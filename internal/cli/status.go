package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/andywolf/agentium/internal/config"
	"github.com/andywolf/agentium/internal/provisioner"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var statusCmd = &cobra.Command{
	Use:   "status [session-id]",
	Short: "Check session status",
	Long: `Check the status of Agentium sessions.

Without arguments, lists all active sessions.
With a session ID, shows detailed status for that session.

Examples:
  agentium status                    # List all sessions
  agentium status agentium-abc12345  # Show specific session`,
	Args: cobra.MaximumNArgs(1),
	RunE: checkStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)

	statusCmd.Flags().Bool("watch", false, "Watch for status changes")
	statusCmd.Flags().Duration("interval", 10*time.Second, "Watch interval")
}

func checkStatus(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

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

	// If no session ID provided, list all sessions
	if len(args) == 0 {
		return listSessions(ctx, prov)
	}

	// Otherwise show detailed status for specific session
	sessionID := args[0]
	return showSessionStatus(ctx, cmd, prov, sessionID)
}

func listSessions(ctx context.Context, prov provisioner.Provisioner) error {
	sessions, err := prov.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	if len(sessions) == 0 {
		fmt.Println("No active sessions found.")
		return nil
	}

	fmt.Printf("%-25s %-12s %-16s %-15s %s\n", "SESSION", "STATE", "IP", "ZONE", "UPTIME")
	fmt.Println(strings.Repeat("-", 80))

	for _, s := range sessions {
		uptime := ""
		if !s.StartTime.IsZero() {
			uptime = time.Since(s.StartTime).Round(time.Second).String()
		}
		fmt.Printf("%-25s %-12s %-16s %-15s %s\n",
			s.SessionID,
			s.State,
			s.PublicIP,
			s.Zone,
			uptime,
		)
	}

	fmt.Printf("\n%d session(s) found.\n", len(sessions))
	return nil
}

func showSessionStatus(ctx context.Context, cmd *cobra.Command, prov provisioner.Provisioner, sessionID string) error {
	watch, _ := cmd.Flags().GetBool("watch")
	interval, _ := cmd.Flags().GetDuration("interval")

	for {
		status, err := prov.Status(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("failed to get status: %w", err)
		}

		fmt.Printf("Session: %s\n", sessionID)
		fmt.Printf("State: %s\n", status.State)
		fmt.Printf("Instance: %s\n", status.InstanceID)
		if status.PublicIP != "" {
			fmt.Printf("IP: %s\n", status.PublicIP)
		}
		if !status.StartTime.IsZero() {
			fmt.Printf("Started: %s\n", status.StartTime.Format(time.RFC3339))
		}
		if !status.EndTime.IsZero() {
			fmt.Printf("Ended: %s\n", status.EndTime.Format(time.RFC3339))
			fmt.Printf("Duration: %s\n", status.EndTime.Sub(status.StartTime).Round(time.Second))
		} else if !status.StartTime.IsZero() {
			fmt.Printf("Uptime: %s\n", time.Since(status.StartTime).Round(time.Second))
		}
		if status.MaxIterations > 0 {
			fmt.Printf("Iteration: %d/%d\n", status.CurrentIteration, status.MaxIterations)
		}
		if len(status.CompletedTasks) > 0 {
			fmt.Printf("Completed tasks: %v\n", status.CompletedTasks)
		}
		if len(status.PendingTasks) > 0 {
			fmt.Printf("Pending tasks: %v\n", status.PendingTasks)
		}

		if !watch {
			break
		}

		if status.State == "terminated" || status.State == "completed" || status.State == "failed" {
			break
		}

		fmt.Println("\n---")
		time.Sleep(interval)
	}

	return nil
}
