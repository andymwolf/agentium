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
	Short: "Regenerate AGENTS.md from project",
	Long: `Regenerate AGENTS.md by rescanning the project.

This command re-analyzes your codebase and updates the auto-generated sections
of AGENTS.md while preserving any custom content you've added.

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
	_, err = os.Stat(configPath)
	if os.IsNotExist(err) {
		return fmt.Errorf(".agentium.yaml not found. Run 'agentium init' first")
	}

	nonInteractive, _ := cmd.Flags().GetBool("non-interactive")
	force, _ := cmd.Flags().GetBool("force")

	// Check for existing AGENTS.md
	agentMDPath := filepath.Join(cwd, agentmd.AgentMDFile)
	var hasCustomContent bool

	existingContent, readErr := os.ReadFile(agentMDPath)
	if readErr == nil {
		parser := &agentmd.Parser{}
		parsed, parseErr := parser.Parse(string(existingContent))
		if parseErr == nil {
			hasCustomContent = parsed.HasCustomContent()
		}
	}

	// Confirm regeneration if custom content exists
	if hasCustomContent && !force && !nonInteractive {
		var confirmed bool
		confirmed, err = wizard.ConfirmRegeneration(hasCustomContent)
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

	// Generate AGENTS.md
	gen, err := agentmd.NewGenerator()
	if err != nil {
		return err
	}

	err = gen.WriteToProject(cwd, info)
	if err != nil {
		return fmt.Errorf("failed to write AGENTS.md: %w", err)
	}

	fmt.Printf("Updated %s\n", agentmd.AgentMDFile)

	if hasCustomContent {
		fmt.Println("Custom sections have been preserved.")
	}

	return nil
}
