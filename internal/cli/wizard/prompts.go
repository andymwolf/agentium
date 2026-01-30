// Package wizard provides interactive prompts for CLI commands.
package wizard

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/andywolf/agentium/internal/scanner"
)

// reader is the input source (can be replaced in tests)
var reader = bufio.NewReader(os.Stdin)

// ConfirmProjectInfo presents the detected project info for user confirmation.
func ConfirmProjectInfo(info *scanner.ProjectInfo) (*scanner.ProjectInfo, error) {
	// Display detected values
	fmt.Println("\n--- Detected Project Configuration ---")
	fmt.Printf("Project:      %s\n", info.Name)
	fmt.Printf("Languages:    %s\n", formatLanguages(info.Languages))
	fmt.Printf("Build System: %s\n", info.BuildSystem)
	fmt.Printf("Framework:    %s\n", info.Framework)
	fmt.Printf("Build Cmds:   %s\n", strings.Join(info.BuildCommands, ", "))
	fmt.Printf("Test Cmds:    %s\n", strings.Join(info.TestCommands, ", "))
	fmt.Printf("Lint Cmds:    %s\n", strings.Join(info.LintCommands, ", "))
	fmt.Println("--------------------------------------")

	confirmed, err := promptYesNo("Is this correct?", true)
	if err != nil {
		return nil, err
	}

	if confirmed {
		return info, nil
	}

	// Allow editing
	return editProjectInfo(info)
}

func editProjectInfo(info *scanner.ProjectInfo) (*scanner.ProjectInfo, error) {
	var err error

	info.Name, err = promptString("Project Name", info.Name)
	if err != nil {
		return nil, err
	}

	info.BuildSystem, err = promptString("Build System", info.BuildSystem)
	if err != nil {
		return nil, err
	}

	info.Framework, err = promptString("Framework (optional)", info.Framework)
	if err != nil {
		return nil, err
	}

	buildCmds, err := promptString("Build Commands (comma-separated)", strings.Join(info.BuildCommands, ", "))
	if err != nil {
		return nil, err
	}
	info.BuildCommands = parseCommands(buildCmds)

	testCmds, err := promptString("Test Commands (comma-separated)", strings.Join(info.TestCommands, ", "))
	if err != nil {
		return nil, err
	}
	info.TestCommands = parseCommands(testCmds)

	lintCmds, err := promptString("Lint Commands (comma-separated)", strings.Join(info.LintCommands, ", "))
	if err != nil {
		return nil, err
	}
	info.LintCommands = parseCommands(lintCmds)

	return info, nil
}

// PromptGreenfield prompts for minimal project configuration when no code exists.
func PromptGreenfield() (*scanner.ProjectInfo, error) {
	info := &scanner.ProjectInfo{}
	var err error

	fmt.Println("\n--- Greenfield Project Setup ---")

	info.Name, err = promptString("Project Name", "")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(info.Name) == "" {
		return nil, fmt.Errorf("project name is required")
	}

	language, err := promptSelect("Primary Language", []string{
		"Go", "TypeScript", "JavaScript", "Python", "Rust", "Java", "Other",
	}, "Go")
	if err != nil {
		return nil, err
	}
	info.Languages = []scanner.LanguageInfo{{Name: language, Percentage: 100}}

	info.Framework, err = promptString("Framework (optional)", "")
	if err != nil {
		return nil, err
	}

	buildCmds, err := promptString("Build Commands (comma-separated, optional)", "")
	if err != nil {
		return nil, err
	}
	info.BuildCommands = parseCommands(buildCmds)

	testCmds, err := promptString("Test Commands (comma-separated, optional)", "")
	if err != nil {
		return nil, err
	}
	info.TestCommands = parseCommands(testCmds)

	return info, nil
}

// ConfirmRegeneration asks user to confirm regeneration when custom content exists.
func ConfirmRegeneration(hasCustomContent bool) (bool, error) {
	if !hasCustomContent {
		return true, nil
	}

	fmt.Println("\n--- Existing AGENTS.md Found ---")
	fmt.Println("Custom sections will be preserved. Generated sections will be updated.")

	return promptYesNo("Continue with regeneration?", true)
}

// promptYesNo prompts for a yes/no response
func promptYesNo(question string, defaultYes bool) (bool, error) {
	defaultStr := "Y/n"
	if !defaultYes {
		defaultStr = "y/N"
	}

	fmt.Printf("%s [%s]: ", question, defaultStr)
	input, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("failed to read input: %w", err)
	}

	input = strings.TrimSpace(strings.ToLower(input))
	if input == "" {
		return defaultYes, nil
	}

	return input == "y" || input == "yes", nil
}

// promptString prompts for a string value with a default
func promptString(question, defaultValue string) (string, error) {
	if defaultValue != "" {
		fmt.Printf("%s [%s]: ", question, defaultValue)
	} else {
		fmt.Printf("%s: ", question)
	}

	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read input: %w", err)
	}

	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue, nil
	}

	return input, nil
}

// promptSelect prompts for a selection from options
func promptSelect(question string, options []string, defaultValue string) (string, error) {
	fmt.Printf("%s:\n", question)
	for i, opt := range options {
		marker := " "
		if opt == defaultValue {
			marker = "*"
		}
		fmt.Printf("  %s %d. %s\n", marker, i+1, opt)
	}

	fmt.Printf("Enter number [default: %s]: ", defaultValue)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read input: %w", err)
	}

	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue, nil
	}

	// Try parsing as number
	var idx int
	if _, err := fmt.Sscanf(input, "%d", &idx); err == nil {
		if idx >= 1 && idx <= len(options) {
			return options[idx-1], nil
		}
	}

	// Try matching option name
	inputLower := strings.ToLower(input)
	for _, opt := range options {
		if strings.ToLower(opt) == inputLower {
			return opt, nil
		}
	}

	return defaultValue, nil
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
