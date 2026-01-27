package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/andywolf/agentium/internal/agentmd"
	"github.com/andywolf/agentium/internal/cli/wizard"
	"github.com/andywolf/agentium/internal/scanner"
	"github.com/spf13/cobra"
)

var refreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Regenerate AGENT.md from project",
	Long: `Regenerate .agentium/AGENT.md by rescanning the project.

This command re-analyzes your codebase and updates the auto-generated sections
of AGENT.md while preserving any custom content you've added.

Requires .agentium.yaml to exist (run 'agentium init' first).

Example:
  agentium refresh
  agentium refresh --non-interactive`,
	RunE: refreshAgentMD,
}

func init() {
	rootCmd.AddCommand(refreshCmd)

	refreshCmd.Flags().Bool("non-interactive", false, "Use detected values without prompting")
	refreshCmd.Flags().Bool("force", false, "Regenerate without confirmation")
}

func refreshAgentMD(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// Check that .agentium.yaml exists
	configPath := filepath.Join(cwd, ".agentium.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return fmt.Errorf(".agentium.yaml not found. Run 'agentium init' first")
	}

	nonInteractive, _ := cmd.Flags().GetBool("non-interactive")
	force, _ := cmd.Flags().GetBool("force")

	// Check for existing AGENT.md
	agentMDPath := filepath.Join(cwd, agentmd.AgentiumDir, agentmd.AgentMDFile)
	var hasCustomContent bool

	if existingContent, err := os.ReadFile(agentMDPath); err == nil {
		parser := &agentmd.Parser{}
		parsed, err := parser.Parse(string(existingContent))
		if err == nil {
			hasCustomContent = parsed.HasCustomContent()
		}
	}

	// Confirm regeneration if custom content exists
	if hasCustomContent && !force && !nonInteractive {
		confirmed, err := wizard.ConfirmRegeneration(hasCustomContent)
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Println("Refresh cancelled.")
			return nil
		}
	}

	// Scan project
	fmt.Println("Scanning project...")
	s := scanner.New(cwd)
	info, err := s.Scan()
	if err != nil {
		return fmt.Errorf("failed to scan project: %w", err)
	}

	// Interactive confirmation of detected values
	if !nonInteractive {
		info, err = wizard.ConfirmProjectInfo(info)
		if err != nil {
			return err
		}
	}

	// Generate AGENT.md
	gen, err := agentmd.NewGenerator()
	if err != nil {
		return err
	}

	if err := gen.WriteToProject(cwd, info); err != nil {
		return fmt.Errorf("failed to write AGENT.md: %w", err)
	}

	fmt.Printf("Updated %s/%s\n", agentmd.AgentiumDir, agentmd.AgentMDFile)

	if hasCustomContent {
		fmt.Println("Custom sections have been preserved.")
	}

	return nil
}
