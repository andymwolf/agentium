// Package wizard provides interactive prompts for CLI commands.
package wizard

import (
	"fmt"
	"strings"

	"github.com/andywolf/agentium/internal/scanner"
	"github.com/charmbracelet/huh"
)

// ConfirmProjectInfo presents the detected project info for user confirmation.
func ConfirmProjectInfo(info *scanner.ProjectInfo) (*scanner.ProjectInfo, error) {
	// Prepare display values
	languages := formatLanguages(info.Languages)
	buildCmds := strings.Join(info.BuildCommands, ", ")
	testCmds := strings.Join(info.TestCommands, ", ")
	lintCmds := strings.Join(info.LintCommands, ", ")

	var confirmed bool

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Detected Project Configuration").
				Description(fmt.Sprintf(
					"Project: %s\nLanguages: %s\nBuild System: %s\nFramework: %s",
					info.Name, languages, info.BuildSystem, info.Framework,
				)),

			huh.NewConfirm().
				Title("Is this correct?").
				Value(&confirmed),
		),
	)

	if err := form.Run(); err != nil {
		return nil, fmt.Errorf("prompt cancelled: %w", err)
	}

	if confirmed {
		return info, nil
	}

	// Allow editing
	return editProjectInfo(info, buildCmds, testCmds, lintCmds)
}

func editProjectInfo(info *scanner.ProjectInfo, buildCmds, testCmds, lintCmds string) (*scanner.ProjectInfo, error) {
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Project Name").
				Value(&info.Name),

			huh.NewInput().
				Title("Build System").
				Value(&info.BuildSystem),

			huh.NewInput().
				Title("Framework (optional)").
				Value(&info.Framework),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("Build Commands (comma-separated)").
				Value(&buildCmds),

			huh.NewInput().
				Title("Test Commands (comma-separated)").
				Value(&testCmds),

			huh.NewInput().
				Title("Lint Commands (comma-separated)").
				Value(&lintCmds),
		),
	)

	if err := form.Run(); err != nil {
		return nil, fmt.Errorf("prompt cancelled: %w", err)
	}

	// Parse commands back
	info.BuildCommands = parseCommands(buildCmds)
	info.TestCommands = parseCommands(testCmds)
	info.LintCommands = parseCommands(lintCmds)

	return info, nil
}

// PromptGreenfield prompts for minimal project configuration when no code exists.
func PromptGreenfield() (*scanner.ProjectInfo, error) {
	info := &scanner.ProjectInfo{}

	var language string
	var buildCmds, testCmds string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Project Name").
				Value(&info.Name).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("project name is required")
					}
					return nil
				}),

			huh.NewSelect[string]().
				Title("Primary Language").
				Options(
					huh.NewOption("Go", "Go"),
					huh.NewOption("TypeScript", "TypeScript"),
					huh.NewOption("JavaScript", "JavaScript"),
					huh.NewOption("Python", "Python"),
					huh.NewOption("Rust", "Rust"),
					huh.NewOption("Java", "Java"),
					huh.NewOption("Other", "Other"),
				).
				Value(&language),

			huh.NewInput().
				Title("Framework (optional)").
				Value(&info.Framework),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("Build Commands (comma-separated, optional)").
				Value(&buildCmds),

			huh.NewInput().
				Title("Test Commands (comma-separated, optional)").
				Value(&testCmds),
		),
	)

	if err := form.Run(); err != nil {
		return nil, fmt.Errorf("prompt cancelled: %w", err)
	}

	info.Languages = []scanner.LanguageInfo{{Name: language, Percentage: 100}}
	info.BuildCommands = parseCommands(buildCmds)
	info.TestCommands = parseCommands(testCmds)

	return info, nil
}

// ConfirmRegeneration asks user to confirm regeneration when custom content exists.
func ConfirmRegeneration(hasCustomContent bool) (bool, error) {
	if !hasCustomContent {
		return true, nil
	}

	var confirmed bool
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Existing AGENT.md Found").
				Description("Custom sections will be preserved. Generated sections will be updated."),

			huh.NewConfirm().
				Title("Continue with regeneration?").
				Value(&confirmed),
		),
	)

	if err := form.Run(); err != nil {
		return false, err
	}

	return confirmed, nil
}

func formatLanguages(languages []scanner.LanguageInfo) string {
	if len(languages) == 0 {
		return "Unknown"
	}
	var parts []string
	for _, lang := range languages {
		parts = append(parts, fmt.Sprintf("%s (%.0f%%)", lang.Name, lang.Percentage))
	}
	return strings.Join(parts, ", ")
}

func parseCommands(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
