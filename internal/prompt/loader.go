package prompt

import (
	"fmt"
	"os"
	"path/filepath"
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

// LoadProjectPrompt reads .agentium/AGENT.md from the given workspace directory.
// Returns empty string with nil error if the file does not exist.
func LoadProjectPrompt(workDir string) (string, error) {
	agentMDPath := filepath.Join(workDir, ".agentium", "AGENT.md")

	data, err := os.ReadFile(agentMDPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to read project prompt %s: %w", agentMDPath, err)
	}

	return string(data), nil
}
