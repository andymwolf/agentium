package cli

import (
	"context"
	"fmt"

	"github.com/andywolf/agentium/internal/config"
	"github.com/andywolf/agentium/internal/provisioner"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var destroyCmd = &cobra.Command{
	Use:   "destroy [session-id]",
	Short: "Force-terminate a session",
	Long: `Force-terminate an Agentium session and destroy all associated resources.

This will immediately terminate the VM and clean up all cloud resources.
Any work in progress will be lost unless already committed and pushed.

Example:
  agentium destroy agentium-abc12345`,
	Args: cobra.ExactArgs(1),
	RunE: destroySession,
}

func init() {
	rootCmd.AddCommand(destroyCmd)

	destroyCmd.Flags().BoolP("force", "f", false, "Skip confirmation prompt")
}

func destroySession(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	sessionID := args[0]

	force, _ := cmd.Flags().GetBool("force")

	if !force {
		fmt.Printf("This will terminate session %s and destroy all resources.\n", sessionID)
		fmt.Printf("Any uncommitted work will be lost.\n\n")
		fmt.Print("Are you sure? [y/N]: ")

		var confirm string
		fmt.Scanln(&confirm)
		if confirm != "y" && confirm != "Y" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	provider := cfg.Cloud.Provider
	if provider == "" {
		return fmt.Errorf("cloud provider not configured")
	}

	verbose := viper.GetBool("verbose")
	prov, err := provisioner.New(provider, verbose)
	if err != nil {
		return fmt.Errorf("failed to create provisioner: %w", err)
	}

	fmt.Printf("Destroying session %s...\n", sessionID)

	if err := prov.Destroy(ctx, sessionID); err != nil {
		return fmt.Errorf("failed to destroy session: %w", err)
	}

	fmt.Println("Session destroyed successfully.")
	return nil
}
