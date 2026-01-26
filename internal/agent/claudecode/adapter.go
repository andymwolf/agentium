package claudecode

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/andywolf/agentium/internal/agent"
)

const (
	// DefaultImage is the default Docker image for Claude Code
	DefaultImage = "ghcr.io/andymwolf/agentium-claudecode:latest"
)

// Adapter implements the Agent interface for Claude Code
type Adapter struct {
	image string
}

// New creates a new Claude Code adapter
func New() *Adapter {
	return &Adapter{
		image: DefaultImage,
	}
}

// Name returns the agent identifier
func (a *Adapter) Name() string {
	return "claude-code"
}

// ContainerImage returns the Docker image for Claude Code
func (a *Adapter) ContainerImage() string {
	return a.image
}

// BuildEnv constructs environment variables for the Claude Code container
func (a *Adapter) BuildEnv(session *agent.Session, iteration int) map[string]string {
	authMode := session.ClaudeAuthMode
	if authMode == "" {
		authMode = "api"
	}

	env := map[string]string{
		"GITHUB_TOKEN":            session.GitHubToken,
		"AGENTIUM_SESSION_ID":     session.ID,
		"AGENTIUM_ITERATION":      fmt.Sprintf("%d", iteration),
		"AGENTIUM_REPOSITORY":     session.Repository,
		"AGENTIUM_WORKDIR":        "/workspace",
		"AGENTIUM_AUTH_MODE":      authMode,
		"CLAUDE_CODE_USE_BEDROCK": "0",
	}

	// Add any custom metadata
	for k, v := range session.Metadata {
		env[fmt.Sprintf("AGENTIUM_%s", strings.ToUpper(k))] = v
	}

	return env
}

// BuildCommand constructs the command to run Claude Code
func (a *Adapter) BuildCommand(session *agent.Session, iteration int) []string {
	var args []string
	if session.Interactive {
		// Interactive mode: run without --print to show TUI for permission approvals
		args = []string{
			"--verbose",
		}
	} else {
		// Non-interactive mode: use --print for structured output and skip permissions
		args = []string{
			"--print",
			"--verbose",
			"--output-format", "stream-json",
			"--dangerously-skip-permissions",
		}
	}

	// Prefer phase-aware skills prompt over monolithic system prompt
	systemPrompt := session.SystemPrompt
	if session.IterationContext != nil && session.IterationContext.SkillsPrompt != "" {
		systemPrompt = session.IterationContext.SkillsPrompt
	}

	if systemPrompt != "" {
		args = append(args, "--system-prompt", systemPrompt)
	}

	if session.ProjectPrompt != "" {
		args = append(args, "--append-system-prompt", session.ProjectPrompt)
	}

	// Pass model override from routing config
	if session.IterationContext != nil && session.IterationContext.ModelOverride != "" {
		args = append(args, "--model", session.IterationContext.ModelOverride)
	}

	// In interactive mode, append prompt as positional argument.
	// In non-interactive mode, prompt is delivered via stdin (see GetStdinPrompt).
	if session.Interactive {
		prompt := a.BuildPrompt(session, iteration)
		args = append(args, prompt)
	}
	return args
}

// GetStdinPrompt implements StdinPromptProvider for stdin-based prompt delivery.
// In non-interactive mode with --print, the prompt is piped via stdin to avoid
// TTY-related issues that can cause the CLI to exit without processing the prompt.
func (a *Adapter) GetStdinPrompt(session *agent.Session, iteration int) string {
	if session.Interactive {
		return "" // Interactive mode uses positional arg
	}
	return a.BuildPrompt(session, iteration)
}

// BuildPrompt constructs the prompt for Claude Code
func (a *Adapter) BuildPrompt(session *agent.Session, iteration int) string {
	// When the controller provides a focused per-task prompt (ActiveTask is set),
	// use it directly — it already contains repository context, issue details,
	// existing work detection, and appropriate instructions.
	if session.ActiveTask != "" && session.Prompt != "" {
		prompt := session.Prompt
		if session.IterationContext != nil {
			// Prefer structured handoff input over accumulated memory context
			if session.IterationContext.PhaseInput != "" {
				prompt += "\n\n" + session.IterationContext.PhaseInput
			} else if session.IterationContext.MemoryContext != "" {
				prompt += "\n\n" + session.IterationContext.MemoryContext
			}
		}
		return prompt
	}

	// Legacy fallback: build a generic multi-issue prompt
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("You are working on repository: %s\n\n", session.Repository))

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
	sb.WriteString("For each issue:\n")
	sb.WriteString("1. Create a new branch: agentium/issue-<number>-<short-description>\n")
	sb.WriteString("2. Implement the fix or feature\n")
	sb.WriteString("3. Run any relevant tests\n")
	sb.WriteString("4. Commit your changes with a descriptive message\n")
	sb.WriteString("5. Push the branch\n")
	sb.WriteString("6. Create a pull request linking to the issue\n\n")

	sb.WriteString("Use 'gh' CLI for GitHub operations and 'git' for version control.\n")
	sb.WriteString("The repository is already cloned at /workspace.\n")

	if iteration > 1 {
		sb.WriteString(fmt.Sprintf("\nThis is iteration %d. Continue from where you left off.\n", iteration))
	}

	return sb.String()
}

// ParseOutput parses Claude Code's output to determine results
func (a *Adapter) ParseOutput(exitCode int, stdout, stderr string) (*agent.IterationResult, error) {
	result := &agent.IterationResult{
		ExitCode: exitCode,
		Success:  exitCode == 0,
	}

	// Parse stream-json from stdout
	parsed := ParseStreamJSON([]byte(stdout))

	// Populate structured fields
	result.RawTextContent = parsed.TextContent
	if parsed.TotalTokens != nil {
		result.TokensUsed = parsed.TotalTokens.InputTokens + parsed.TotalTokens.OutputTokens
	}

	// Convert events for controller access
	events := make([]interface{}, len(parsed.Events))
	for i, e := range parsed.Events {
		events[i] = e
	}
	result.Events = events

	// Signal detection: use aggregated text + stderr
	// Parse AGENTIUM_STATUS signals from output
	// Format: AGENTIUM_STATUS: STATUS_NAME [optional message on same line]
	// The pattern matches status and optional message up to end of line
	// Use [ \t] instead of \s to avoid matching newlines
	statusPattern := regexp.MustCompile(`AGENTIUM_STATUS:[ \t]*(\w+)(?:[ \t]+([^\n]+))?`)
	combined := parsed.TextContent + "\n" + stderr
	if matches := statusPattern.FindAllStringSubmatch(combined, -1); len(matches) > 0 {
		// Use the last status signal (most recent)
		last := matches[len(matches)-1]
		result.AgentStatus = last[1]
		if len(last) > 2 && last[2] != "" {
			result.StatusMessage = strings.TrimSpace(last[2])
		}

		// Set PushedChanges based on status signals for backwards compatibility
		switch result.AgentStatus {
		case "PUSHED", "COMPLETE", "PR_CREATED":
			result.PushedChanges = true
		case "NOTHING_TO_DO":
			// Mark as success even if nothing was pushed
			result.Success = true
		}
	}

	// Look for created PRs in output (require "Created" or "Opened" verb to avoid matching issue references)
	prPattern := regexp.MustCompile(`(?:Created|Opened)\s+(?:pull request|PR)[^\d]*#?(\d+)`)
	prMatches := prPattern.FindAllStringSubmatch(combined, -1)
	for _, match := range prMatches {
		if len(match) > 1 {
			result.PRsCreated = appendUnique(result.PRsCreated, match[1])
		}
	}

	// Look for GitHub PR URLs
	urlPattern := regexp.MustCompile(`https://github\.com/[^/]+/[^/]+/pull/(\d+)`)
	urlMatches := urlPattern.FindAllStringSubmatch(combined, -1)
	for _, match := range urlMatches {
		if len(match) > 1 {
			result.PRsCreated = appendUnique(result.PRsCreated, match[1])
		}
	}

	// Look for completed tasks (issues mentioned in commits/PRs)
	issuePattern := regexp.MustCompile(`(?:fixes?|closes?|resolves?)[^\d]*#(\d+)`)
	issueMatches := issuePattern.FindAllStringSubmatch(strings.ToLower(combined), -1)
	for _, match := range issueMatches {
		if len(match) > 1 {
			result.TasksCompleted = append(result.TasksCompleted, match[1])
		}
	}

	// Detect successful git push (for PR review sessions)
	// Matches patterns like: "To github.com:owner/repo.git" followed by commit hash range
	pushPattern := regexp.MustCompile(`To (?:github\.com|git@github\.com)[^\n]*\n.*[a-f0-9]+\.\.[a-f0-9]+`)
	if pushPattern.MatchString(combined) {
		result.PushedChanges = true
	}

	// Extract error messages
	if exitCode != 0 {
		// Look for common error patterns
		errorPatterns := []string{
			`error:?\s+(.+)`,
			`fatal:?\s+(.+)`,
			`Error:?\s+(.+)`,
		}
		for _, pattern := range errorPatterns {
			re := regexp.MustCompile(pattern)
			if match := re.FindStringSubmatch(stderr); len(match) > 1 {
				result.Error = match[1]
				break
			}
		}
		if result.Error == "" && stderr != "" {
			// Use last non-empty line of stderr
			lines := strings.Split(strings.TrimSpace(stderr), "\n")
			result.Error = lines[len(lines)-1]
		}
	}

	// Generate summary
	if len(result.PRsCreated) > 0 {
		result.Summary = fmt.Sprintf("Created %d PR(s): #%s", len(result.PRsCreated), strings.Join(result.PRsCreated, ", #"))
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

// appendUnique appends value to slice only if not already present.
func appendUnique(slice []string, value string) []string {
	for _, v := range slice {
		if v == value {
			return slice
		}
	}
	return append(slice, value)
}

func init() {
	// Register the adapter
	agent.Register("claude-code", func() agent.Agent {
		return New()
	})
}
