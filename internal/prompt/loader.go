package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadSystemPrompt reads SYSTEM.md from the repository's prompts directory.
func LoadSystemPrompt(workDir string) (string, error) {
	systemMDPath := filepath.Join(workDir, "prompts", "SYSTEM.md")
	data, err := os.ReadFile(systemMDPath)
	if err != nil {
		return "", fmt.Errorf("failed to read system prompt %s: %w", systemMDPath, err)
	}
	return string(data), nil
}

// LoadProjectPrompt reads AGENT.md from the given workspace directory root.
// Returns empty string with nil error if the file does not exist.
func LoadProjectPrompt(workDir string) (string, error) {
	agentMDPath := filepath.Join(workDir, "AGENT.md")

	data, err := os.ReadFile(agentMDPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to read project prompt %s: %w", agentMDPath, err)
	}

	return string(data), nil
}

// LoadProjectPromptWithPackage loads and merges root and package-specific AGENT.md files.
// For monorepo projects, this combines the repository-level instructions with package-specific
// instructions, providing hierarchical context to the agent.
//
// The returned prompt has the format:
//
//	## Repository Instructions
//	<root AGENT.md content>
//
//	---
//
//	## Package Instructions (<packagePath>)
//	<package AGENT.md content>
//
// If either file is missing, it is silently skipped. If both are missing, returns empty string.
func LoadProjectPromptWithPackage(workDir, packagePath string) (string, error) {
	var parts []string

	// Load root AGENT.md (optional)
	rootPrompt, err := LoadProjectPrompt(workDir)
	if err != nil {
		return "", fmt.Errorf("failed to load root project prompt: %w", err)
	}
	if rootPrompt != "" {
		parts = append(parts, "## Repository Instructions\n\n"+rootPrompt)
	}

	// Load package AGENT.md (optional)
	if packagePath != "" {
		pkgDir := filepath.Join(workDir, packagePath)
		pkgPrompt, err := LoadProjectPrompt(pkgDir)
		if err != nil {
			// Only return error for actual read failures, not missing files
			return "", fmt.Errorf("failed to load package prompt: %w", err)
		}
		if pkgPrompt != "" {
			parts = append(parts, fmt.Sprintf("## Package Instructions (%s)\n\n%s", packagePath, pkgPrompt))
		}
	}

	if len(parts) == 0 {
		return "", nil
	}

	return strings.Join(parts, "\n\n---\n\n"), nil
}
