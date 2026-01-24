package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/andywolf/agentium/internal/config"
	"github.com/andywolf/agentium/internal/provisioner"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var logsCmd = &cobra.Command{
	Use:   "logs [session-id]",
	Short: "Retrieve session logs",
	Long: `Retrieve logs from an Agentium session.

Example:
  agentium logs agentium-abc12345
  agentium logs agentium-abc12345 --follow`,
	Args: cobra.ExactArgs(1),
	RunE: getLogs,
}

func init() {
	rootCmd.AddCommand(logsCmd)

	logsCmd.Flags().BoolP("follow", "f", false, "Follow log output")
	logsCmd.Flags().Int("tail", 100, "Number of lines to show from the end")
	logsCmd.Flags().String("since", "", "Show logs since timestamp (e.g., 2024-01-01T00:00:00Z) or duration (e.g., 1h)")
}

func getLogs(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	sessionID := args[0]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	provider := cfg.Cloud.Provider
	if provider == "" {
		return fmt.Errorf("cloud provider not configured")
	}

	verbose := viper.GetBool("verbose")
	prov, err := provisioner.New(provider, verbose, cfg.Cloud.Project)
	if err != nil {
		return fmt.Errorf("failed to create provisioner: %w", err)
	}

	follow, _ := cmd.Flags().GetBool("follow")
	tail, _ := cmd.Flags().GetInt("tail")
	sinceStr, _ := cmd.Flags().GetString("since")

	var since time.Time
	if sinceStr != "" {
		// Try parsing as duration first
		if dur, err := time.ParseDuration(sinceStr); err == nil {
			since = time.Now().Add(-dur)
		} else {
			// Try parsing as timestamp
			since, err = time.Parse(time.RFC3339, sinceStr)
			if err != nil {
				return fmt.Errorf("invalid --since value: %s", sinceStr)
			}
		}
	}

	logsOpts := provisioner.LogsOptions{
		Follow: follow,
		Tail:   tail,
		Since:  since,
	}

	logCh, errCh := prov.Logs(ctx, sessionID, logsOpts)

	for entry := range logCh {
		if entry.Timestamp.IsZero() {
			fmt.Println(entry.Message)
		} else {
			fmt.Printf("[%s] %s\n", entry.Timestamp.Format("15:04:05"), entry.Message)
		}
	}

	if err := <-errCh; err != nil {
		return fmt.Errorf("error reading logs: %w", err)
	}
	return nil
}
