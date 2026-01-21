package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/andywolf/agentium/internal/config"
	"github.com/andywolf/agentium/internal/provisioner"
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
	runCmd.MarkFlagRequired("repo")
	runCmd.Flags().StringSlice("issues", nil, "Issue numbers to work on (comma-separated)")
	runCmd.Flags().String("agent", "claude-code", "Agent to use (claude-code, aider)")
	runCmd.Flags().Int("max-iterations", 30, "Maximum number of iterations")
	runCmd.Flags().String("max-duration", "2h", "Maximum session duration")
	runCmd.Flags().String("provider", "", "Cloud provider (gcp, aws, azure)")
	runCmd.Flags().String("region", "", "Cloud region")
	runCmd.Flags().Bool("dry-run", false, "Show what would be provisioned without creating resources")
	runCmd.Flags().String("prompt", "", "Custom prompt for the agent")

	viper.BindPFlag("session.repo", runCmd.Flags().Lookup("repo"))
	viper.BindPFlag("session.issues", runCmd.Flags().Lookup("issues"))
	viper.BindPFlag("session.agent", runCmd.Flags().Lookup("agent"))
	viper.BindPFlag("session.max_iterations", runCmd.Flags().Lookup("max-iterations"))
	viper.BindPFlag("session.max_duration", runCmd.Flags().Lookup("max-duration"))
	viper.BindPFlag("cloud.provider", runCmd.Flags().Lookup("provider"))
	viper.BindPFlag("cloud.region", runCmd.Flags().Lookup("region"))
}

func runSession(cmd *cobra.Command, args []string) error {
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
		cfg.Session.Tasks = issues
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
	if provider := viper.GetString("cloud.provider"); provider != "" {
		cfg.Cloud.Provider = provider
	}
	if region := viper.GetString("cloud.region"); region != "" {
		cfg.Cloud.Region = region
	}
	if prompt, _ := cmd.Flags().GetString("prompt"); prompt != "" {
		cfg.Session.Prompt = prompt
	}

	// Validate required fields
	if cfg.Session.Repository == "" {
		return fmt.Errorf("repository is required (use --repo or set in config)")
	}
	if len(cfg.Session.Tasks) == 0 {
		return fmt.Errorf("at least one issue is required (use --issues)")
	}
	if cfg.Cloud.Provider == "" {
		return fmt.Errorf("cloud provider is required (use --provider or set in config)")
	}

	// Generate session ID
	sessionID := fmt.Sprintf("agentium-%s", uuid.New().String()[:8])

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	verbose := viper.GetBool("verbose")

	fmt.Printf("Session ID: %s\n", sessionID)
	fmt.Printf("Repository: %s\n", cfg.Session.Repository)
	fmt.Printf("Issues: %s\n", strings.Join(cfg.Session.Tasks, ", "))
	fmt.Printf("Agent: %s\n", cfg.Session.Agent)
	fmt.Printf("Provider: %s\n", cfg.Cloud.Provider)
	fmt.Printf("Max iterations: %d\n", cfg.Session.MaxIterations)
	fmt.Printf("Max duration: %s\n", cfg.Session.MaxDuration)
	fmt.Println()

	if dryRun {
		fmt.Println("Dry run - no resources will be created")
		return nil
	}

	// Create provisioner
	prov, err := provisioner.New(cfg.Cloud.Provider, verbose)
	if err != nil {
		return fmt.Errorf("failed to create provisioner: %w", err)
	}

	// Build session config for the VM
	sessionConfig := provisioner.SessionConfig{
		ID:            sessionID,
		Repository:    cfg.Session.Repository,
		Tasks:         cfg.Session.Tasks,
		Agent:         cfg.Session.Agent,
		MaxIterations: cfg.Session.MaxIterations,
		MaxDuration:   cfg.Session.MaxDuration,
		Prompt:        cfg.Session.Prompt,
		GitHub: provisioner.GitHubConfig{
			AppID:            cfg.GitHub.AppID,
			InstallationID:   cfg.GitHub.InstallationID,
			PrivateKeySecret: cfg.GitHub.PrivateKeySecret,
		},
	}

	// Build VM config
	vmConfig := provisioner.VMConfig{
		Region:       cfg.Cloud.Region,
		MachineType:  cfg.Cloud.MachineType,
		UseSpot:      cfg.Cloud.UseSpot,
		DiskSizeGB:   cfg.Cloud.DiskSizeGB,
		Session:      sessionConfig,
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
