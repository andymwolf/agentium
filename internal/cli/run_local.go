package cli

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/andywolf/agentium/internal/config"
	"github.com/andywolf/agentium/internal/controller"
	"github.com/andywolf/agentium/internal/routing"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// runLocalSession runs the controller locally in interactive mode for debugging.
// This bypasses VM provisioning and runs the agent directly on the local machine.
func runLocalSession(cmd *cobra.Command, _ []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signals for cleanup
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nReceived interrupt signal, cleaning up...")
		cancel()
	}()

	// Note: GITHUB_TOKEN is optional - if not set, auth happens inside the container via gh auth login

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

	// Validate configuration for local run (relaxed validation)
	if err = cfg.ValidateForLocalRun(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// Generate session ID
	sessionID := fmt.Sprintf("agentium-local-%s", uuid.New().String()[:8])

	// Create temp workspace directory
	workDir, err := os.MkdirTemp("", "agentium-local-*")
	if err != nil {
		return fmt.Errorf("failed to create temp workspace: %w", err)
	}
	defer func() {
		// Clean up workspace on exit
		fmt.Printf("Cleaning up workspace: %s\n", workDir)
		_ = os.RemoveAll(workDir)
	}()

	// Set workspace environment variable for the controller
	_ = os.Setenv("AGENTIUM_WORKDIR", workDir)

	fmt.Printf("Session ID: %s\n", sessionID)
	fmt.Printf("Repository: %s\n", cfg.Session.Repository)
	if len(cfg.Session.Tasks) > 0 {
		fmt.Printf("Issues: %s\n", strings.Join(cfg.Session.Tasks, ", "))
	}
	fmt.Printf("Agent: %s\n", cfg.Session.Agent)
	fmt.Printf("Workspace: %s\n", workDir)
	fmt.Printf("Max iterations: %d\n", cfg.Session.MaxIterations)
	fmt.Printf("Max duration: %s\n", cfg.Session.MaxDuration)
	if cfg.Session.AutoMerge {
		fmt.Println("Auto-merge: enabled")
	}
	fmt.Println()
	fmt.Println("Running in local interactive mode - agent will prompt for permission approvals")
	fmt.Println()

	// Handle Claude OAuth authentication
	var claudeAuthBase64 string
	var claudeAuthMode string
	switch cfg.Claude.AuthMode {
	case "":
		// Auto-detect OAuth credentials if auth mode not explicitly set
		if autoAuth := tryAutoDetectOAuth(); autoAuth != nil {
			claudeAuthBase64 = base64.StdEncoding.EncodeToString(autoAuth)
			claudeAuthMode = "oauth" // Set mode so Docker mount happens
			fmt.Printf("Auto-detected Claude OAuth credentials from macOS Keychain (%d bytes)\n", len(autoAuth))
		} else {
			fmt.Println("No OAuth credentials found - will use interactive browser auth in container")
		}
	case "oauth":
		// Explicit oauth mode - error if credentials not found
		var authJSON []byte
		authJSON, err = readAuthJSON(cfg.Claude.AuthJSONPath)
		if err != nil {
			return fmt.Errorf("failed to read Claude auth.json: %w", err)
		}
		claudeAuthBase64 = base64.StdEncoding.EncodeToString(authJSON)
		claudeAuthMode = "oauth"
		fmt.Printf("Using Claude Max OAuth authentication (%d bytes)\n", len(authJSON))
	}

	// Build controller session config
	sessionConfig := controller.SessionConfig{
		ID:                   sessionID,
		CloudProvider:        "local",
		Repository:           cfg.Session.Repository,
		Tasks:                cfg.Session.Tasks,
		Agent:                cfg.Session.Agent,
		MaxIterations:        cfg.Session.MaxIterations,
		MaxDuration:          cfg.Session.MaxDuration,
		Prompt:               cfg.Session.Prompt,
		Interactive:          true, // Enable interactive mode
		CloneInsideContainer: true, // Clone inside Docker container for reliable auth
		Verbose:              viper.GetBool("verbose"),
		AutoMerge:            cfg.Session.AutoMerge,
	}

	// Set Claude auth config
	// Use claudeAuthMode which is set to "oauth" when auto-detect succeeds
	sessionConfig.ClaudeAuth.AuthMode = claudeAuthMode
	sessionConfig.ClaudeAuth.AuthJSONBase64 = claudeAuthBase64

	// Enable phase loop (PLAN → IMPLEMENT → DOCS → PR workflow)
	// Phase loop is always enabled - the config just customizes iteration counts
	sessionConfig.PhaseLoop = &controller.PhaseLoopConfig{
		SkipPlanIfExists:       cfg.PhaseLoop.SkipPlanIfExists,
		PlanMaxIterations:      cfg.PhaseLoop.PlanMaxIterations,
		ImplementMaxIterations: cfg.PhaseLoop.ImplementMaxIterations,
		DocsMaxIterations:      cfg.PhaseLoop.DocsMaxIterations,
		VerifyMaxIterations:    cfg.PhaseLoop.VerifyMaxIterations,
		JudgeContextBudget:     cfg.PhaseLoop.JudgeContextBudget,
		JudgeNoSignalLimit:     cfg.PhaseLoop.JudgeNoSignalLimit,
	}

	// Enable monorepo support if configured
	if cfg.Monorepo.Enabled {
		sessionConfig.Monorepo = &controller.MonorepoSessionConfig{
			Enabled:     cfg.Monorepo.Enabled,
			LabelPrefix: cfg.Monorepo.LabelPrefix,
		}
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
			cfgRouting := cfg.Routing
			sessionConfig.Routing = &cfgRouting
		} else if sessionConfig.Routing.Overrides == nil && len(cfg.Routing.Overrides) > 0 {
			sessionConfig.Routing.Overrides = make(map[string]routing.ModelConfig, len(cfg.Routing.Overrides))
			for phase, spec := range cfg.Routing.Overrides {
				sessionConfig.Routing.Overrides[phase] = spec
			}
		}
	}

	// Handle Codex OAuth authentication
	// Check after routing merge so CLI overrides are considered
	router := routing.NewRouter(sessionConfig.Routing)
	needsCodexAuth := cfg.Session.Agent == "codex" || router.UsesAdapter("codex")
	if needsCodexAuth {
		// Try auto-detect from Keychain first
		if autoAuth := tryAutoDetectCodexOAuth(); autoAuth != nil {
			sessionConfig.CodexAuth.AuthJSONBase64 = base64.StdEncoding.EncodeToString(autoAuth)
			fmt.Println("Auto-detected Codex OAuth credentials from macOS Keychain")
		} else {
			fmt.Println("Warning: Codex auth not found - will prompt for auth inside container")
		}
	}

	// Create and run the controller
	ctrl, err := controller.New(sessionConfig)
	if err != nil {
		return fmt.Errorf("failed to create controller: %w", err)
	}

	if err := ctrl.Run(ctx); err != nil {
		if ctx.Err() != nil {
			fmt.Println("Session interrupted by user")
			return nil
		}
		return fmt.Errorf("session failed: %w", err)
	}

	fmt.Println("\nLocal session completed successfully")
	return nil
}
