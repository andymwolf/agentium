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

var logsCmd = &cobra.Command{
	Use:   "logs [session-id]",
	Short: "Retrieve session logs",
	Long: `Retrieve logs from an Agentium session.

Example:
  agentium logs agentium-abc12345
  agentium logs agentium-abc12345 --follow
  agentium logs agentium-abc12345 --events
  agentium logs agentium-abc12345 --events --level debug
  agentium logs agentium-abc12345 --events --type tool_use,thinking
  agentium logs agentium-abc12345 --events --iteration 3`,
	Args: cobra.ExactArgs(1),
	RunE: getLogs,
}

func init() {
	rootCmd.AddCommand(logsCmd)

	logsCmd.Flags().BoolP("follow", "f", false, "Follow log output")
	logsCmd.Flags().Int("tail", 100, "Number of lines to show from the end")
	logsCmd.Flags().String("since", "", "Show logs since timestamp (e.g., 2024-01-01T00:00:00Z) or duration (e.g., 1h)")
	logsCmd.Flags().Bool("events", false, "Show agent events (tool calls, decisions); implies --level=debug")
	logsCmd.Flags().String("level", "info", "Minimum log level: debug, info, warning, error")
	logsCmd.Flags().String("type", "", "Filter by event types (comma-separated, e.g., tool_use,thinking)")
	logsCmd.Flags().Int("iteration", 0, "Filter by iteration number (0 = all iterations)")
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
	showEvents, _ := cmd.Flags().GetBool("events")
	level, _ := cmd.Flags().GetString("level")
	typeFilter, _ := cmd.Flags().GetString("type")
	iteration, _ := cmd.Flags().GetInt("iteration")

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

	// Parse type filter into slice
	var types []string
	if typeFilter != "" {
		types = strings.Split(typeFilter, ",")
		for i, t := range types {
			types[i] = strings.TrimSpace(t)
		}
	}

	logsOpts := provisioner.LogsOptions{
		Follow:     follow,
		Tail:       tail,
		Since:      since,
		ShowEvents: showEvents,
		MinLevel:   level,
		TypeFilter: types,
		Iteration:  iteration,
	}

	logCh, errCh := prov.Logs(ctx, sessionID, logsOpts)

	for {
		select {
		case entry, ok := <-logCh:
			if !ok {
				if err := <-errCh; err != nil {
					return fmt.Errorf("error reading logs: %w", err)
				}
				return nil
			}
			formatLogEntry(entry)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// formatLogEntry prints a log entry with appropriate formatting based on event type.
func formatLogEntry(entry provisioner.LogEntry) {
	ts := ""
	if !entry.Timestamp.IsZero() {
		ts = fmt.Sprintf("[%s] ", entry.Timestamp.Format("15:04:05"))
	}

	if entry.EventType != "" {
		// Format structured events
		switch entry.EventType {
		case "tool_use":
			toolLabel := "TOOL"
			if entry.ToolName != "" {
				toolLabel = fmt.Sprintf("TOOL:%s", entry.ToolName)
			}
			fmt.Printf("%s[%s] %s\n", ts, toolLabel, entry.Message)
		case "tool_result":
			fmt.Printf("%s[RESULT] %s\n", ts, entry.Message)
		case "thinking":
			fmt.Printf("%s[THINKING] %s\n", ts, entry.Message)
		case "text":
			fmt.Printf("%s[AGENT] %s\n", ts, entry.Message)
		default:
			fmt.Printf("%s[%s] %s\n", ts, entry.EventType, entry.Message)
		}
	} else {
		fmt.Printf("%s%s\n", ts, entry.Message)
	}
}
