package prompt

import (
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

//go:embed system.md
var embeddedSystemMD string

// DefaultSystemMDURL is the default URL to fetch the latest SYSTEM.md
const DefaultSystemMDURL = "https://raw.githubusercontent.com/andymwolf/agentium/main/prompts/SYSTEM.md"

// DefaultFetchTimeout is the default timeout for fetching remote prompts
const DefaultFetchTimeout = 5 * time.Second

// LoadSystemPrompt attempts to fetch the latest SYSTEM.md from fetchURL,
// falling back to the embedded version on failure.
// If fetchURL is empty, DefaultSystemMDURL is used.
func LoadSystemPrompt(fetchURL string) (string, error) {
	if fetchURL == "" {
		fetchURL = DefaultSystemMDURL
	}

	content, err := fetchRemotePrompt(fetchURL, DefaultFetchTimeout)
	if err == nil && content != "" {
		return content, nil
	}

	// Fall back to embedded version
	if embeddedSystemMD == "" {
		return "", fmt.Errorf("no system prompt available: fetch failed (%v) and no embedded fallback", err)
	}

	return embeddedSystemMD, nil
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

// fetchRemotePrompt fetches content from a URL with the given timeout.
func fetchRemotePrompt(url string, timeout time.Duration) (string, error) {
	client := &http.Client{Timeout: timeout}

	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch %s returned status %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response from %s: %w", url, err)
	}

	return string(body), nil
}
