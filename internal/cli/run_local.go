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
func runLocalSession(cmd *cobra.Command, args []string) error {
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

	// Verify GITHUB_TOKEN is set
	if os.Getenv("GITHUB_TOKEN") == "" {
		return fmt.Errorf("GITHUB_TOKEN environment variable is required for --local mode\n\nSet it with:\n  export GITHUB_TOKEN=<your-github-token>")
	}

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
		cfg.Session.Tasks = issues
	}
	if prs := viper.GetStringSlice("session.prs"); len(prs) > 0 {
		cfg.Session.PRs = prs
	}
	if agent := viper.GetString("session.agent"); agent != "" {
		cfg.Session.Agent = agent
	}
	if maxIter := viper.GetInt("session.max_iterations"); maxIter > 0 {
		cfg.Session.MaxIterations = maxIter
	}
	if maxDur := viper.GetString("session.max_duration"); maxDur != "" {
		cfg.Session.MaxDuration = maxDur
	}
	if prompt, _ := cmd.Flags().GetString("prompt"); prompt != "" {
		cfg.Session.Prompt = prompt
	}
	if authMode := viper.GetString("claude.auth_mode"); authMode != "" {
		cfg.Claude.AuthMode = authMode
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
		os.RemoveAll(workDir)
	}()

	// Set workspace environment variable for the controller
	os.Setenv("AGENTIUM_WORKDIR", workDir)

	fmt.Printf("Session ID: %s\n", sessionID)
	fmt.Printf("Repository: %s\n", cfg.Session.Repository)
	if len(cfg.Session.Tasks) > 0 {
		fmt.Printf("Issues: %s\n", strings.Join(cfg.Session.Tasks, ", "))
	}
	if len(cfg.Session.PRs) > 0 {
		fmt.Printf("PRs: %s\n", strings.Join(cfg.Session.PRs, ", "))
	}
	fmt.Printf("Agent: %s\n", cfg.Session.Agent)
	fmt.Printf("Workspace: %s\n", workDir)
	fmt.Printf("Max iterations: %d\n", cfg.Session.MaxIterations)
	fmt.Printf("Max duration: %s\n", cfg.Session.MaxDuration)
	fmt.Println()
	fmt.Println("Running in local interactive mode - agent will prompt for permission approvals")
	fmt.Println()

	// Handle Claude OAuth authentication
	var claudeAuthBase64 string
	if cfg.Claude.AuthMode == "oauth" {
		authJSON, err := readAuthJSON(cfg.Claude.AuthJSONPath)
		if err != nil {
			return fmt.Errorf("failed to read Claude auth.json: %w", err)
		}
		claudeAuthBase64 = base64.StdEncoding.EncodeToString(authJSON)
		fmt.Println("Using Claude Max OAuth authentication")
	}

	// Handle Codex OAuth authentication
	var codexAuthBase64 string
	if cfg.Session.Agent == "codex" {
		authJSON, err := readCodexAuthJSON(cfg.Codex.AuthJSONPath)
		if err != nil {
			return fmt.Errorf("failed to read Codex auth.json: %w", err)
		}
		codexAuthBase64 = base64.StdEncoding.EncodeToString(authJSON)
		fmt.Println("Using Codex OAuth authentication")
	}

	// Build controller session config
	sessionConfig := controller.SessionConfig{
		ID:            sessionID,
		Repository:    cfg.Session.Repository,
		Tasks:         cfg.Session.Tasks,
		PRs:           cfg.Session.PRs,
		Agent:         cfg.Session.Agent,
		MaxIterations: cfg.Session.MaxIterations,
		MaxDuration:   cfg.Session.MaxDuration,
		Prompt:        cfg.Session.Prompt,
		Interactive:   true, // Enable interactive mode
		Verbose:       viper.GetBool("verbose"),
	}

	// Set Claude auth config
	sessionConfig.ClaudeAuth.AuthMode = cfg.Claude.AuthMode
	sessionConfig.ClaudeAuth.AuthJSONBase64 = claudeAuthBase64

	// Set Codex auth config
	sessionConfig.CodexAuth.AuthJSONBase64 = codexAuthBase64

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
