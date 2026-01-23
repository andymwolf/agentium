package aider

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/andywolf/agentium/internal/agent"
)

const (
	// DefaultImage is the default Docker image for Aider
	DefaultImage = "ghcr.io/andymwolf/agentium-aider:latest"
)

// Adapter implements the Agent interface for Aider
type Adapter struct {
	image string
	model string
}

// New creates a new Aider adapter
func New() *Adapter {
	return &Adapter{
		image: DefaultImage,
		model: "claude-3-5-sonnet-20241022",
	}
}

// Name returns the agent identifier
func (a *Adapter) Name() string {
	return "aider"
}

// ContainerImage returns the Docker image for Aider
func (a *Adapter) ContainerImage() string {
	return a.image
}

// BuildEnv constructs environment variables for the Aider container
func (a *Adapter) BuildEnv(session *agent.Session, iteration int) map[string]string {
	env := map[string]string{
		"GITHUB_TOKEN":        session.GitHubToken,
		"AGENTIUM_SESSION_ID": session.ID,
		"AGENTIUM_ITERATION":  fmt.Sprintf("%d", iteration),
		"AGENTIUM_REPOSITORY": session.Repository,
		"AGENTIUM_WORKDIR":    "/workspace",
	}

	// Aider needs ANTHROPIC_API_KEY for Claude models
	if key, ok := session.Metadata["anthropic_api_key"]; ok {
		env["ANTHROPIC_API_KEY"] = key
	}

	// Add any custom metadata (exclude sensitive keys)
	for k, v := range session.Metadata {
		lowerKey := strings.ToLower(k)
		if !strings.Contains(lowerKey, "api_key") && !strings.Contains(lowerKey, "secret") && !strings.Contains(lowerKey, "token") {
			env[fmt.Sprintf("AGENTIUM_%s", strings.ToUpper(k))] = v
		}
	}

	return env
}

// BuildCommand constructs the command to run Aider
func (a *Adapter) BuildCommand(session *agent.Session, iteration int) []string {
	prompt := a.BuildPrompt(session, iteration)

	return []string{
		"--model", a.model,
		"--yes-always",
		"--no-git",
		"--message", prompt,
	}
}

// BuildPrompt constructs the prompt for Aider
func (a *Adapter) BuildPrompt(session *agent.Session, iteration int) string {
	var sb strings.Builder

	// Prepend system prompt if available (Aider has no --system-prompt flag)
	if session.SystemPrompt != "" {
		sb.WriteString("=== SYSTEM INSTRUCTIONS ===\n\n")
		sb.WriteString(session.SystemPrompt)
		sb.WriteString("\n\n=== END SYSTEM INSTRUCTIONS ===\n\n")
	}

	// Append project-specific instructions if available
	if session.ProjectPrompt != "" {
		sb.WriteString("=== PROJECT INSTRUCTIONS ===\n\n")
		sb.WriteString(session.ProjectPrompt)
		sb.WriteString("\n\n=== END PROJECT INSTRUCTIONS ===\n\n")
	}

	sb.WriteString(fmt.Sprintf("Working on repository: %s\n\n", session.Repository))

	if session.Prompt != "" {
		sb.WriteString(session.Prompt)
		sb.WriteString("\n\n")
	} else {
		sb.WriteString("Complete the following GitHub issues:\n\n")
	}

	for _, task := range session.Tasks {
		sb.WriteString(fmt.Sprintf("- Issue #%s\n", task))
	}

	sb.WriteString("\n")
	sb.WriteString("For each issue, make the necessary code changes.\n")
	sb.WriteString("Focus on implementing working solutions.\n")

	if iteration > 1 {
		sb.WriteString(fmt.Sprintf("\nThis is iteration %d. Continue from where you left off.\n", iteration))
	}

	return sb.String()
}

// ParseOutput parses Aider's output to determine results
func (a *Adapter) ParseOutput(exitCode int, stdout, stderr string) (*agent.IterationResult, error) {
	result := &agent.IterationResult{
		ExitCode: exitCode,
		Success:  exitCode == 0,
	}

	combined := stdout + stderr

	// Look for file changes
	filePattern := regexp.MustCompile(`(?:Wrote|Updated|Created|Modified)\s+(\S+)`)
	fileMatches := filePattern.FindAllStringSubmatch(combined, -1)

	filesChanged := make([]string, 0)
	for _, match := range fileMatches {
		if len(match) > 1 {
			filesChanged = append(filesChanged, match[1])
		}
	}

	// Extract error messages
	if exitCode != 0 {
		errorPatterns := []string{
			`error:?\s+(.+)`,
			`Error:?\s+(.+)`,
			`failed:?\s+(.+)`,
		}
		for _, pattern := range errorPatterns {
			re := regexp.MustCompile(pattern)
			if match := re.FindStringSubmatch(stderr); len(match) > 1 {
				result.Error = match[1]
				break
			}
		}
		if result.Error == "" && stderr != "" {
			lines := strings.Split(strings.TrimSpace(stderr), "\n")
			result.Error = lines[len(lines)-1]
		}
	}

	// Generate summary
	if len(filesChanged) > 0 {
		result.Summary = fmt.Sprintf("Modified %d file(s): %s", len(filesChanged), strings.Join(filesChanged, ", "))
	} else if result.Success {
		result.Summary = "Iteration completed successfully"
	} else {
		result.Summary = fmt.Sprintf("Iteration failed: %s", result.Error)
	}

	return result, nil
}

// Validate checks if the adapter configuration is valid
func (a *Adapter) Validate() error {
	if a.image == "" {
		return fmt.Errorf("container image is required")
	}
	return nil
}

func init() {
	// Register the adapter
	agent.Register("aider", func() agent.Agent {
		return New()
	})
}
