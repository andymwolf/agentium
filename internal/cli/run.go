package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/andywolf/agentium/internal/config"
	"github.com/andywolf/agentium/internal/provisioner"
	"github.com/andywolf/agentium/internal/routing"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Launch an agent session",
	Long: `Launch an ephemeral AI agent session to work on GitHub issues.

This command provisions a cloud VM, clones the repository, and runs the
configured AI agent to complete the specified tasks.

Example:
  agentium run --repo github.com/org/myapp --issues 12,17,24 --max-iterations 30`,
	RunE: runSession,
}

func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.Flags().String("repo", "", "GitHub repository (e.g., github.com/org/repo)")
	runCmd.Flags().StringSlice("issues", nil, "Issue numbers to work on (comma-separated)")
	runCmd.Flags().String("agent", "claude-code", "Agent to use (claude-code, aider, codex)")
	runCmd.Flags().Int("max-iterations", 30, "Maximum number of iterations")
	runCmd.Flags().String("max-duration", "2h", "Maximum session duration")
	runCmd.Flags().String("provider", "", "Cloud provider (gcp, aws, azure)")
	runCmd.Flags().String("region", "", "Cloud region")
	runCmd.Flags().Bool("dry-run", false, "Show what would be provisioned without creating resources")
	runCmd.Flags().String("prompt", "", "Custom prompt for the agent")
	runCmd.Flags().String("claude-auth-mode", "", "Claude auth mode: api (default) or oauth")
	runCmd.Flags().String("model", "", "Override model for all phases (format: adapter:model)")
	runCmd.Flags().StringSlice("phase-model", nil, "Per-phase model override (format: PHASE=adapter:model)")
	runCmd.Flags().Bool("local", false, "Run locally for interactive debugging (no VM provisioning)")
	runCmd.Flags().Bool("auto-merge", false, "Automatically merge PR after CI checks pass")

	_ = viper.BindPFlag("session.repo", runCmd.Flags().Lookup("repo"))
	_ = viper.BindPFlag("session.issues", runCmd.Flags().Lookup("issues"))
	_ = viper.BindPFlag("session.agent", runCmd.Flags().Lookup("agent"))
	_ = viper.BindPFlag("session.max_iterations", runCmd.Flags().Lookup("max-iterations"))
	_ = viper.BindPFlag("session.max_duration", runCmd.Flags().Lookup("max-duration"))
	_ = viper.BindPFlag("cloud.provider", runCmd.Flags().Lookup("provider"))
	_ = viper.BindPFlag("cloud.region", runCmd.Flags().Lookup("region"))
	_ = viper.BindPFlag("claude.auth_mode", runCmd.Flags().Lookup("claude-auth-mode"))
}

func runSession(cmd *cobra.Command, args []string) error {
	// Check if --local flag is set
	local, _ := cmd.Flags().GetBool("local")
	if local {
		return runLocalSession(cmd, args)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nReceived interrupt signal, cleaning up...")
		cancel()
	}()

	// Load and validate configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Apply CLI flags
	if repo := viper.GetString("session.repo"); repo != "" {
		cfg.Session.Repository = repo
	}
	if issues := viper.GetStringSlice("session.issues"); len(issues) > 0 {
		expandedIssues, expandErr := ExpandRanges(issues)
		if expandErr != nil {
			return fmt.Errorf("invalid --issues value: %w", expandErr)
		}
		cfg.Session.Tasks = expandedIssues
	}
	if agent := viper.GetString("session.agent"); agent != "" {
		cfg.Session.Agent = agent
	}
	if cmd.Flags().Changed("max-iterations") {
		maxIter, _ := cmd.Flags().GetInt("max-iterations")
		cfg.Session.MaxIterations = maxIter
	}
	if cmd.Flags().Changed("max-duration") {
		maxDur, _ := cmd.Flags().GetString("max-duration")
		cfg.Session.MaxDuration = maxDur
	}
	if provider := viper.GetString("cloud.provider"); provider != "" {
		cfg.Cloud.Provider = provider
	}
	if region := viper.GetString("cloud.region"); region != "" {
		cfg.Cloud.Region = region
	}
	if prompt, _ := cmd.Flags().GetString("prompt"); prompt != "" {
		cfg.Session.Prompt = prompt
	}
	if authMode := viper.GetString("claude.auth_mode"); authMode != "" {
		cfg.Claude.AuthMode = authMode
	}
	if cmd.Flags().Changed("auto-merge") {
		autoMerge, _ := cmd.Flags().GetBool("auto-merge")
		cfg.Session.AutoMerge = autoMerge
	}

	// Validate configuration after applying CLI flags
	if err = cfg.ValidateForRun(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// Handle Claude OAuth authentication
	var claudeAuthBase64 string
	if cfg.Claude.AuthMode == "oauth" {
		var authJSON []byte
		authJSON, err = readAuthJSON(cfg.Claude.AuthJSONPath)
		if err != nil {
			return fmt.Errorf("failed to read Claude auth.json: %w", err)
		}
		claudeAuthBase64 = base64.StdEncoding.EncodeToString(authJSON)
		fmt.Printf("Using Claude Max OAuth authentication (%d bytes from %s)\n", len(authJSON), cfg.Claude.AuthJSONPath)
	} else {
		fmt.Printf("Claude auth mode: %q (no OAuth credentials will be mounted)\n", cfg.Claude.AuthMode)
	}

	// Generate session ID
	sessionID := fmt.Sprintf("agentium-%s", uuid.New().String()[:8])

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	verbose := viper.GetBool("verbose")

	fmt.Printf("Session ID: %s\n", sessionID)
	fmt.Printf("Repository: %s\n", cfg.Session.Repository)
	if len(cfg.Session.Tasks) > 0 {
		fmt.Printf("Issues: %s\n", strings.Join(cfg.Session.Tasks, ", "))
	}
	fmt.Printf("Agent: %s\n", cfg.Session.Agent)
	fmt.Printf("Provider: %s\n", cfg.Cloud.Provider)
	fmt.Printf("Max iterations: %d\n", cfg.Session.MaxIterations)
	fmt.Printf("Max duration: %s\n", cfg.Session.MaxDuration)
	if cfg.Session.AutoMerge {
		fmt.Println("Auto-merge: enabled")
	}
	fmt.Println()

	if dryRun {
		fmt.Println("Dry run - no resources will be created")
		return nil
	}

	// Create provisioner
	prov, err := provisioner.New(cfg.Cloud.Provider, verbose, cfg.Cloud.Project, cfg.Cloud.ServiceAccountKey)
	if err != nil {
		return fmt.Errorf("failed to create provisioner: %w", err)
	}

	// Build session config for the VM
	sessionConfig := provisioner.SessionConfig{
		ID:            sessionID,
		CloudProvider: cfg.Cloud.Provider,
		Repository:    cfg.Session.Repository,
		Tasks:         cfg.Session.Tasks,
		Agent:         cfg.Session.Agent,
		MaxIterations: cfg.Session.MaxIterations,
		MaxDuration:   cfg.Session.MaxDuration,
		Prompt:        cfg.Session.Prompt,
		AutoMerge:     cfg.Session.AutoMerge,
		GitHub: provisioner.GitHubConfig{
			AppID:            cfg.GitHub.AppID,
			InstallationID:   cfg.GitHub.InstallationID,
			PrivateKeySecret: cfg.GitHub.PrivateKeySecret,
		},
		ClaudeAuth: provisioner.ClaudeAuthConfig{
			AuthMode:       cfg.Claude.AuthMode,
			AuthJSONBase64: claudeAuthBase64,
		},
	}

	// Handle --model (overrides default for all phases)
	if model, _ := cmd.Flags().GetString("model"); model != "" {
		spec := routing.ParseModelSpec(model)
		sessionConfig.Routing = &routing.PhaseRouting{
			Default: spec,
		}
	}

	// Handle --phase-model (per-phase overrides)
	if phaseModels, _ := cmd.Flags().GetStringSlice("phase-model"); len(phaseModels) > 0 {
		if sessionConfig.Routing == nil {
			sessionConfig.Routing = &routing.PhaseRouting{}
		}
		if sessionConfig.Routing.Overrides == nil {
			sessionConfig.Routing.Overrides = make(map[string]routing.ModelConfig)
		}
		for _, pm := range phaseModels {
			parts := strings.SplitN(pm, "=", 2)
			if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
				return fmt.Errorf("invalid --phase-model value %q: expected format PHASE=adapter:model", pm)
			}
			sessionConfig.Routing.Overrides[parts[0]] = routing.ParseModelSpec(parts[1])
		}
	}

	// Merge config file routing when CLI didn't provide overrides
	if cfg.Routing.Default.Model != "" || len(cfg.Routing.Overrides) > 0 {
		if sessionConfig.Routing == nil {
			// No CLI routing at all: use config file entirely
			cfgRouting := cfg.Routing // copy
			sessionConfig.Routing = &cfgRouting
		} else if sessionConfig.Routing.Overrides == nil && len(cfg.Routing.Overrides) > 0 {
			// CLI set --model default but no --phase-model: merge config file overrides
			sessionConfig.Routing.Overrides = make(map[string]routing.ModelConfig, len(cfg.Routing.Overrides))
			for phase, spec := range cfg.Routing.Overrides {
				sessionConfig.Routing.Overrides[phase] = spec
			}
		}
	}

	// Handle Codex OAuth authentication
	// Check after routing merge so CLI overrides are considered
	needsCodexAuth := cfg.Session.Agent == "codex" || routing.NewRouter(sessionConfig.Routing).UsesAdapter("codex")
	if needsCodexAuth {
		var authJSON []byte
		authJSON, err = readCodexAuthJSON(cfg.Codex.AuthJSONPath)
		if err != nil {
			return fmt.Errorf("failed to read Codex auth.json: %w", err)
		}
		sessionConfig.CodexAuth.AuthJSONBase64 = base64.StdEncoding.EncodeToString(authJSON)
		fmt.Println("Using Codex OAuth authentication")
	}

	// Validate auth requirements match routing config before provisioning
	if err = validateAuthForRouting(sessionConfig, cfg); err != nil {
		return err
	}

	// Propagate delegation config from config file
	if cfg.Delegation.Enabled {
		subAgents := make(map[string]provisioner.SubAgentConfig, len(cfg.Delegation.SubAgents))
		for name, sa := range cfg.Delegation.SubAgents {
			subAgents[name] = provisioner.SubAgentConfig{
				Agent:  sa.Agent,
				Model:  sa.Model,
				Skills: sa.Skills,
			}
		}
		sessionConfig.Delegation = &provisioner.ProvDelegationConfig{
			Enabled:   true,
			Strategy:  cfg.Delegation.Strategy,
			SubAgents: subAgents,
		}
	}

	// Propagate phase loop config from config file
	// Phase loop is always enabled - the config just customizes iteration counts
	sessionConfig.PhaseLoop = &provisioner.ProvPhaseLoopConfig{
		SkipPlanIfExists:       cfg.PhaseLoop.SkipPlanIfExists,
		PlanMaxIterations:      cfg.PhaseLoop.PlanMaxIterations,
		ImplementMaxIterations: cfg.PhaseLoop.ImplementMaxIterations,
		ReviewMaxIterations:    cfg.PhaseLoop.ReviewMaxIterations,
		DocsMaxIterations:      cfg.PhaseLoop.DocsMaxIterations,
		VerifyMaxIterations:    cfg.PhaseLoop.VerifyMaxIterations,
		JudgeContextBudget:     cfg.PhaseLoop.JudgeContextBudget,
		JudgeNoSignalLimit:     cfg.PhaseLoop.JudgeNoSignalLimit,
	}

	// Propagate monorepo config from config file
	if cfg.Monorepo.Enabled {
		sessionConfig.Monorepo = &provisioner.ProvMonorepoConfig{
			Enabled:     cfg.Monorepo.Enabled,
			LabelPrefix: cfg.Monorepo.LabelPrefix,
			Tiers:       cfg.Monorepo.Tiers,
		}
	}

	// Propagate fallback config from routing
	if cfg.Routing.Default.FallbackEnabled {
		sessionConfig.Fallback = &provisioner.ProvFallbackConfig{
			Enabled: true,
		}
	}

	// Build VM config
	vmConfig := provisioner.VMConfig{
		Project:         cfg.Cloud.Project,
		Region:          cfg.Cloud.Region,
		MachineType:     cfg.Cloud.MachineType,
		UseSpot:         cfg.Cloud.UseSpot,
		DiskSizeGB:      cfg.Cloud.DiskSizeGB,
		Session:         sessionConfig,
		ControllerImage: cfg.Controller.Image,
	}

	fmt.Println("Provisioning VM...")
	result, err := prov.Provision(ctx, vmConfig)
	if err != nil {
		return fmt.Errorf("failed to provision VM: %w", err)
	}

	fmt.Printf("\nSession started successfully!\n")
	fmt.Printf("  Instance: %s\n", result.InstanceID)
	fmt.Printf("  IP: %s\n", result.PublicIP)
	fmt.Printf("  Zone: %s\n", result.Zone)
	fmt.Println()
	fmt.Printf("To check status: agentium status %s\n", sessionID)
	fmt.Printf("To view logs: agentium logs %s\n", sessionID)
	fmt.Printf("To terminate: agentium destroy %s\n", sessionID)

	return nil
}

// tryAutoDetectOAuth attempts to find OAuth credentials from Keychain (macOS)
// Returns nil if no credentials found (allows fallback to interactive auth)
func tryAutoDetectOAuth() []byte {
	if runtime.GOOS != "darwin" {
		return nil
	}

	data, err := readAuthFromKeychain()
	if err != nil {
		return nil // Not an error - just means no cached credentials
	}

	return data
}

// tryAutoDetectCodexOAuth attempts to find Codex OAuth credentials from Keychain (macOS)
// Returns nil if no credentials found (allows fallback to interactive auth)
func tryAutoDetectCodexOAuth() []byte {
	if runtime.GOOS != "darwin" {
		return nil
	}

	data, err := readCodexAuthFromKeychain()
	if err != nil {
		return nil // Not an error - just means no cached credentials
	}

	return data
}

// readAuthJSON reads Claude OAuth credentials from file or macOS Keychain
func readAuthJSON(path string) ([]byte, error) {
	// Expand ~ to home directory
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to resolve home directory: %w", err)
		}
		path = filepath.Join(home, path[2:])
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// On macOS, try reading from Keychain
			if runtime.GOOS == "darwin" {
				keychainData, keychainErr := readAuthFromKeychain()
				if keychainErr == nil {
					return keychainData, nil
				}
			}
			return nil, fmt.Errorf("auth.json not found at %s and not in macOS Keychain\n\nTo use OAuth authentication:\n  1. Install Claude Code: bun add -g @anthropic-ai/claude-code\n  2. Run: claude login\n  3. Try again", path)
		}
		return nil, fmt.Errorf("failed to read auth.json: %w", err)
	}

	if len(data) < 10 {
		return nil, fmt.Errorf("auth.json appears to be empty or too small")
	}

	if !json.Valid(data) {
		return nil, fmt.Errorf("auth.json is not valid JSON")
	}

	return data, nil
}

// readAuthFromKeychain reads Claude Code OAuth credentials from the macOS Keychain
func readAuthFromKeychain() ([]byte, error) {
	// Determine the account name (macOS username)
	u, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("failed to get current user: %w", err)
	}

	cmd := exec.Command("security", "find-generic-password",
		"-s", "Claude Code-credentials",
		"-a", u.Username,
		"-w",
	)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to read from Keychain: %w", err)
	}

	data := []byte(strings.TrimSpace(string(output)))

	if !json.Valid(data) {
		return nil, fmt.Errorf("keychain credential is not valid JSON")
	}

	return data, nil
}

// readCodexAuthJSON reads Codex OAuth credentials from file or macOS Keychain
func readCodexAuthJSON(path string) ([]byte, error) {
	// Expand ~ to home directory
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to resolve home directory: %w", err)
		}
		path = filepath.Join(home, path[2:])
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// On macOS, try reading from Keychain
			if runtime.GOOS == "darwin" {
				keychainData, keychainErr := readCodexAuthFromKeychain()
				if keychainErr == nil {
					return keychainData, nil
				}
			}
			return nil, fmt.Errorf("codex auth.json not found at %s and not in macOS Keychain\n\nTo use Codex authentication:\n  1. Install Codex: bun add -g @openai/codex\n  2. Run: codex --login\n  3. Try again", path)
		}
		return nil, fmt.Errorf("failed to read Codex auth.json: %w", err)
	}

	if len(data) < 10 {
		return nil, fmt.Errorf("codex auth.json appears to be empty or too small")
	}

	if !json.Valid(data) {
		return nil, fmt.Errorf("codex auth.json is not valid JSON")
	}

	return data, nil
}

// readCodexAuthFromKeychain reads Codex OAuth credentials from the macOS Keychain
func readCodexAuthFromKeychain() ([]byte, error) {
	u, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("failed to get current user: %w", err)
	}

	cmd := exec.Command("security", "find-generic-password",
		"-s", "Codex-credentials",
		"-a", u.Username,
		"-w",
	)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to read Codex credentials from Keychain: %w", err)
	}

	data := []byte(strings.TrimSpace(string(output)))

	if !json.Valid(data) {
		return nil, fmt.Errorf("codex keychain credential is not valid JSON")
	}

	return data, nil
}

// validateAuthForRouting checks that required authentication is available for all adapters in routing.
// Call this after routing merge and auth loading, before provisioning.
func validateAuthForRouting(sessionConfig provisioner.SessionConfig, cfg *config.Config) error {
	router := routing.NewRouter(sessionConfig.Routing)

	// Check Codex auth requirements
	if router.UsesAdapter("codex") && sessionConfig.CodexAuth.AuthJSONBase64 == "" {
		return fmt.Errorf("codex adapter is in routing but auth credentials are missing\n\n" +
			"Run 'codex --login' to authenticate before using codex adapter")
	}

	// Check Claude OAuth requirements
	if router.UsesAdapter("claude-code") && cfg.Claude.AuthMode == "oauth" {
		if sessionConfig.ClaudeAuth.AuthJSONBase64 == "" {
			return fmt.Errorf("claude-code adapter with OAuth mode requires authentication\n\n" +
				"Run 'claude login' to authenticate or use --claude-auth-mode=api")
		}
	}

	return nil
}
