package codex

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/andywolf/agentium/internal/agent"
)

const (
	// DefaultImage is the default Docker image for Codex CLI
	DefaultImage = "ghcr.io/andymwolf/agentium-codex:latest"
)

// Adapter implements the Agent interface for OpenAI's Codex CLI
type Adapter struct {
	image string
}

// New creates a new Codex adapter
func New() *Adapter {
	return &Adapter{
		image: DefaultImage,
	}
}

// Name returns the agent identifier
func (a *Adapter) Name() string {
	return "codex"
}

// ContainerImage returns the Docker image for Codex CLI
func (a *Adapter) ContainerImage() string {
	return a.image
}

// ContainerEntrypoint returns the entrypoint for docker exec in pooled containers.
func (a *Adapter) ContainerEntrypoint() []string {
	return []string{"/runtime-scripts/agent-wrapper.sh", "codex"}
}

// BuildEnv constructs environment variables for the Codex container
func (a *Adapter) BuildEnv(session *agent.Session, iteration int) map[string]string {
	env := map[string]string{
		"GITHUB_TOKEN":        session.GitHubToken,
		"AGENTIUM_SESSION_ID": session.ID,
		"AGENTIUM_ITERATION":  fmt.Sprintf("%d", iteration),
		"AGENTIUM_REPOSITORY": session.Repository,
		"AGENTIUM_WORKDIR":    "/workspace",
	}

	// Inject OpenAI API key from credentials if available (highest precedence)
	if session.Credentials != nil && session.Credentials.OpenAIAccessToken != "" {
		env["OPENAI_API_KEY"] = session.Credentials.OpenAIAccessToken
	} else if key, ok := session.Metadata["codex_api_key"]; ok {
		// Fall back to metadata: prefer codex_api_key, then openai_api_key
		env["CODEX_API_KEY"] = key
	} else if key, ok := session.Metadata["openai_api_key"]; ok {
		env["OPENAI_API_KEY"] = key
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

// BuildCommand constructs the command to run Codex CLI
func (a *Adapter) BuildCommand(session *agent.Session, iteration int) []string {
	prompt := a.BuildPrompt(session, iteration)

	args := []string{
		"exec",
		"--json",
	}
	if !session.Interactive {
		args = append(args, "--yolo")
	}
	args = append(args,
		"--skip-git-repo-check",
		"--cd", "/workspace",
	)

	// Model override: prefer IterationContext, then metadata
	model := ""
	if session.IterationContext != nil && session.IterationContext.ModelOverride != "" {
		model = session.IterationContext.ModelOverride
	} else if m, ok := session.Metadata["codex_model"]; ok && m != "" {
		model = m
	}
	if model != "" {
		args = append(args, "--model", model)
	}

	// Reasoning level override via config (Codex uses model_reasoning_effort)
	if session.IterationContext != nil && session.IterationContext.ReasoningOverride != "" {
		args = append(args, "-c", fmt.Sprintf("model_reasoning_effort=%s", session.IterationContext.ReasoningOverride))
	}

	// Build developer instructions from system/project prompts + status signal instructions.
	// Escape newlines so the value survives CLI config parsing as a single argument.
	developerInstructions := a.buildDeveloperInstructions(session)
	if developerInstructions != "" {
		escaped := strings.ReplaceAll(developerInstructions, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, "\n", `\n`)
		args = append(args, "-c", fmt.Sprintf("developer_instructions=%s", escaped))
	}

	args = append(args, prompt)
	return args
}

// buildDeveloperInstructions combines system prompt, project prompt, and status signal instructions
func (a *Adapter) buildDeveloperInstructions(session *agent.Session) string {
	var parts []string

	// Prefer phase-aware skills prompt over monolithic system prompt
	systemPrompt := session.SystemPrompt
	if session.IterationContext != nil && session.IterationContext.SkillsPrompt != "" {
		systemPrompt = session.IterationContext.SkillsPrompt
	}
	if systemPrompt != "" {
		parts = append(parts, systemPrompt)
	}

	if session.ProjectPrompt != "" {
		parts = append(parts, session.ProjectPrompt)
	}

	// Always append status signal instructions
	parts = append(parts, statusSignalInstructions)

	return strings.Join(parts, "\n\n")
}

// statusSignalInstructions tells the agent how to emit AGENTIUM_STATUS signals
const statusSignalInstructions = `When you complete a significant milestone, output a status signal on its own line in this format:
AGENTIUM_STATUS: STATUS_NAME optional message

Available status values:
- TESTS_PASSED: All tests pass
- TESTS_FAILED: Tests failed (include details in message)
- PR_CREATED: Pull request created (include URL in message)
- PUSHED: Changes pushed to remote
- COMPLETE: All work finished successfully
- NOTHING_TO_DO: No changes needed
- BLOCKED: Cannot proceed (include reason in message)
- ANALYZING: Currently analyzing the codebase
- TESTS_RUNNING: Currently running tests`

// BuildPrompt constructs the prompt for Codex CLI
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
	sb.WriteString("1. Create a new branch: <prefix>/issue-<number>-<short-description> (prefix based on issue labels, default: feature)\n")
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

// CodexEvent represents a JSONL event from Codex CLI --json output.
// This type is exported for use by the audit and event packages.
type CodexEvent struct {
	Type    string          `json:"type"`
	Item    *EventItem      `json:"item,omitempty"`
	Delta   *EventDelta     `json:"delta,omitempty"`
	Usage   *usage          `json:"usage,omitempty"`
	Error   *EventError     `json:"error,omitempty"`
	Content []contentBlock  `json:"content,omitempty"` // For "message" type events
	Message *messageContent `json:"message,omitempty"` // Alternative message structure
}

// EventItem represents an item within a Codex event.
// This type is exported for use by the audit package.
type EventItem struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Command  string `json:"command,omitempty"`
	Output   string `json:"output,omitempty"`
	FilePath string `json:"file_path,omitempty"`
	Action   string `json:"action,omitempty"`
}

// EventDelta represents a streaming text delta.
// This type is exported for use by the event package.
type EventDelta struct {
	Text string `json:"text,omitempty"`
}

// contentBlock represents a content block within a message event
type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// messageContent represents the message field structure in some events
type messageContent struct {
	Content []contentBlock `json:"content,omitempty"`
}

// usage represents token usage information
type usage struct {
	InputTokens       int `json:"input_tokens"`
	OutputTokens      int `json:"output_tokens"`
	CachedInputTokens int `json:"cached_input_tokens"`
}

// EventError represents an error in a Codex event.
// This type is exported for use by the event package.
type EventError struct {
	Message string `json:"message"`
}

// ParseOutput parses Codex CLI's JSONL output to determine results
func (a *Adapter) ParseOutput(exitCode int, stdout, stderr string) (*agent.IterationResult, error) {
	result := &agent.IterationResult{
		ExitCode: exitCode,
		Success:  exitCode == 0,
	}

	// Parse JSONL events from stdout
	var textParts []string
	var filesChanged []string
	var errors []string
	var totalInput, totalOutput int
	var parsedEvents int
	var events []interface{} // Collect events for audit logging

	// Track event types seen for diagnostic logging
	eventTypeCounts := make(map[string]int)

	lines := strings.Split(stdout, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var event CodexEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			// Skip malformed JSON lines
			continue
		}
		parsedEvents++
		eventTypeCounts[event.Type]++

		// Store completed events for audit logging
		if event.Type == "item.completed" && event.Item != nil {
			events = append(events, event)
		}

		switch event.Type {
		case "item.completed":
			if event.Item != nil {
				switch event.Item.Type {
				case "agent_message":
					if event.Item.Text != "" {
						textParts = append(textParts, event.Item.Text)
					}
				case "command_execution":
					if event.Item.Output != "" {
						textParts = append(textParts, event.Item.Output)
					}
				case "file_change":
					if event.Item.FilePath != "" {
						filesChanged = append(filesChanged, event.Item.FilePath)
					}
				}
			}
		case "item.delta", "response.output_text.delta":
			// Handle streaming delta events that deliver text incrementally
			if event.Delta != nil && event.Delta.Text != "" {
				textParts = append(textParts, event.Delta.Text)
			} else if event.Item != nil && event.Item.Text != "" {
				textParts = append(textParts, event.Item.Text)
			}
		case "message":
			// Handle message events with content array
			textParts = append(textParts, extractTextFromContent(event.Content)...)
			if event.Message != nil {
				textParts = append(textParts, extractTextFromContent(event.Message.Content)...)
			}
		case "response.completed":
			// Handle response.completed events that may contain message content
			textParts = append(textParts, extractTextFromContent(event.Content)...)
			if event.Message != nil {
				textParts = append(textParts, extractTextFromContent(event.Message.Content)...)
			}
		case "turn.completed":
			if event.Usage != nil {
				totalInput += event.Usage.InputTokens
				totalOutput += event.Usage.OutputTokens
			}
		case "turn.failed":
			if event.Error != nil && event.Error.Message != "" {
				errors = append(errors, event.Error.Message)
			}
		case "error":
			if event.Error != nil && event.Error.Message != "" {
				errors = append(errors, event.Error.Message)
			}
		}
	}

	// Log event type distribution for diagnostics
	if len(eventTypeCounts) > 0 {
		log.Printf("[codex] ParseOutput: parsed %d events, types: %v", parsedEvents, eventTypeCounts)
	}

	// Store events for audit logging
	result.Events = events

	// Set token usage
	result.InputTokens = totalInput
	result.OutputTokens = totalOutput
	result.TokensUsed = totalInput + totalOutput

	// Fallback: if no JSONL events were parsed or no text was extracted,
	// use raw stdout for signal/PR detection to handle unexpected output formats.
	if parsedEvents == 0 || (len(textParts) == 0 && stdout != "") {
		textParts = append(textParts, stdout)
	}

	// Combine text content for signal detection
	combined := strings.Join(textParts, "\n") + "\n" + stderr
	result.RawTextContent = strings.Join(textParts, "\n")

	// For Codex, extract assistant text (agent_message items only, excluding command outputs)
	var assistantTextParts []string
	for _, e := range events {
		if evt, ok := e.(CodexEvent); ok && evt.Item != nil && evt.Item.Type == "agent_message" && evt.Item.Text != "" {
			assistantTextParts = append(assistantTextParts, evt.Item.Text)
		}
	}
	result.AssistantText = strings.Join(assistantTextParts, "\n")

	// Log warning if RawTextContent is empty despite having stdout (diagnostic for reviewer issues)
	if result.RawTextContent == "" && stdout != "" {
		preview := stdout
		if len(preview) > 500 {
			preview = preview[:500] + "..."
		}
		log.Printf("[codex] WARNING: RawTextContent empty despite stdout (%d bytes). Preview: %s", len(stdout), preview)
	}

	// Parse AGENTIUM_STATUS signals from output
	statusPattern := regexp.MustCompile(`AGENTIUM_STATUS:[ \t]*(\w+)(?:[ \t]+([^\n]+))?`)
	if matches := statusPattern.FindAllStringSubmatch(combined, -1); len(matches) > 0 {
		last := matches[len(matches)-1]
		result.AgentStatus = last[1]
		if len(last) > 2 && last[2] != "" {
			result.StatusMessage = strings.TrimSpace(last[2])
		}

		switch result.AgentStatus {
		case "PUSHED", "COMPLETE", "PR_CREATED":
			result.PushedChanges = true
		case "NOTHING_TO_DO":
			result.Success = true
		}
	}

	// Look for created PRs in output
	// Use \s* instead of [^\d]* to prevent matching "Created PR for issue #123" as PR #123
	prPattern := regexp.MustCompile(`(?:Created|Opened)\s+(?:pull request|PR)\s*#?(\d+)`)
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

	// Detect successful git push
	pushPattern := regexp.MustCompile(`To (?:github\.com|git@github\.com)[^\n]*\n.*[a-f0-9]+\.\.[a-f0-9]+`)
	if pushPattern.MatchString(combined) {
		result.PushedChanges = true
	}

	// Extract error messages
	if exitCode != 0 {
		if len(errors) > 0 {
			result.Error = errors[len(errors)-1]
		} else {
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
				stderrLines := strings.Split(strings.TrimSpace(stderr), "\n")
				result.Error = stderrLines[len(stderrLines)-1]
			}
		}
	}

	// Generate summary
	if len(result.PRsCreated) > 0 {
		result.Summary = fmt.Sprintf("Created %d PR(s): #%s", len(result.PRsCreated), strings.Join(result.PRsCreated, ", #"))
	} else if len(filesChanged) > 0 {
		result.Summary = fmt.Sprintf("Modified %d file(s)", len(filesChanged))
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

// extractTextFromContent extracts text from content blocks in message events.
func extractTextFromContent(content []contentBlock) []string {
	var texts []string
	for _, c := range content {
		if c.Type == "text" && c.Text != "" {
			texts = append(texts, c.Text)
		}
	}
	return texts
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
	agent.Register("codex", func() agent.Agent {
		return New()
	})
}
