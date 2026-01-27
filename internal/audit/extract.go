package audit

import (
	"encoding/json"
)

// BashInput represents the JSON input structure for the Bash tool.
type BashInput struct {
	Command string `json:"command"`
}

// WriteInput represents the JSON input structure for the Write tool.
type WriteInput struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

// EditInput represents the JSON input structure for the Edit tool.
type EditInput struct {
	FilePath  string `json:"file_path"`
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
}

// WebFetchInput represents the JSON input structure for the WebFetch tool.
type WebFetchInput struct {
	URL    string `json:"url"`
	Prompt string `json:"prompt"`
}

// WebSearchInput represents the JSON input structure for the WebSearch tool.
type WebSearchInput struct {
	Query string `json:"query"`
}

// ClaudeCodeEvent represents a tool_use event from Claude Code's stream-json output.
// This interface allows the extractor to work with the actual StreamEvent type
// without creating an import cycle.
type ClaudeCodeEvent interface {
	GetToolName() string
	GetToolInput() json.RawMessage
	IsToolUse() bool
}

// CodexEventData represents the data needed to extract audit events from Codex output.
type CodexEventData struct {
	Type     string // e.g., "command_execution", "file_change"
	Command  string // For command_execution
	FilePath string // For file_change
	Action   string // For file_change (e.g., "write", "edit")
}

// ExtractFromClaudeCode inspects Claude Code StreamEvent tool_use blocks and
// returns security audit events.
func ExtractFromClaudeCode(events []interface{}, agent, taskID string) []Event {
	var auditEvents []Event

	for _, evt := range events {
		// Type assert to get tool information
		// We expect StreamEvent from claudecode package
		se, ok := evt.(interface {
			GetToolName() string
			GetToolInput() json.RawMessage
			IsToolUse() bool
		})
		if !ok {
			// Try direct field access for struct types
			auditEvents = append(auditEvents, extractFromClaudeCodeStruct(evt, agent, taskID)...)
			continue
		}

		if !se.IsToolUse() {
			continue
		}

		toolName := se.GetToolName()
		toolInput := se.GetToolInput()

		auditEvents = append(auditEvents, extractFromTool(toolName, toolInput, agent, taskID)...)
	}

	return auditEvents
}

// extractFromClaudeCodeStruct handles struct-based StreamEvent without interface methods
func extractFromClaudeCodeStruct(evt interface{}, agent, taskID string) []Event {
	// Use reflection-free approach: marshal to JSON and unmarshal to a known structure
	data, err := json.Marshal(evt)
	if err != nil {
		return nil
	}

	var se struct {
		Subtype   string          `json:"subtype"`
		ToolName  string          `json:"tool_name"`
		ToolInput json.RawMessage `json:"tool_input"`
	}
	if err := json.Unmarshal(data, &se); err != nil {
		return nil
	}

	if se.Subtype != "tool_use" {
		return nil
	}

	return extractFromTool(se.ToolName, se.ToolInput, agent, taskID)
}

// extractFromTool extracts audit events from a single tool invocation
func extractFromTool(toolName string, toolInput json.RawMessage, agent, taskID string) []Event {
	var auditEvents []Event

	switch toolName {
	case "Bash":
		var input BashInput
		if err := json.Unmarshal(toolInput, &input); err == nil && input.Command != "" {
			categories := ClassifyBashCommand(input.Command)
			for _, cat := range categories {
				auditEvents = append(auditEvents, Event{
					Category: cat,
					ToolName: toolName,
					Agent:    agent,
					TaskID:   taskID,
					Message:  input.Command,
				})
			}
		}

	case "Write":
		var input WriteInput
		if err := json.Unmarshal(toolInput, &input); err == nil && input.FilePath != "" {
			if IsSensitivePath(input.FilePath) {
				auditEvents = append(auditEvents, Event{
					Category: SensitiveFileWrite,
					ToolName: toolName,
					Agent:    agent,
					TaskID:   taskID,
					Message:  input.FilePath,
				})
			}
		}

	case "Edit":
		var input EditInput
		if err := json.Unmarshal(toolInput, &input); err == nil && input.FilePath != "" {
			if IsSensitivePath(input.FilePath) {
				auditEvents = append(auditEvents, Event{
					Category: SensitiveFileWrite,
					ToolName: toolName,
					Agent:    agent,
					TaskID:   taskID,
					Message:  input.FilePath,
				})
			}
		}

	case "WebFetch":
		var input WebFetchInput
		if err := json.Unmarshal(toolInput, &input); err == nil && input.URL != "" {
			auditEvents = append(auditEvents, Event{
				Category: URLBrowsed,
				ToolName: toolName,
				Agent:    agent,
				TaskID:   taskID,
				Message:  input.URL,
			})
		}

	case "WebSearch":
		var input WebSearchInput
		if err := json.Unmarshal(toolInput, &input); err == nil && input.Query != "" {
			auditEvents = append(auditEvents, Event{
				Category: URLBrowsed,
				ToolName: toolName,
				Agent:    agent,
				TaskID:   taskID,
				Message:  input.Query,
			})
		}
	}

	return auditEvents
}

// ExtractFromCodexEvents inspects Codex event items and returns security audit events.
func ExtractFromCodexEvents(events []CodexEventData, agent, taskID string) []Event {
	var auditEvents []Event

	for _, evt := range events {
		switch evt.Type {
		case "command_execution":
			if evt.Command != "" {
				categories := ClassifyBashCommand(evt.Command)
				for _, cat := range categories {
					auditEvents = append(auditEvents, Event{
						Category: cat,
						ToolName: "command_execution",
						Agent:    agent,
						TaskID:   taskID,
						Message:  evt.Command,
					})
				}
			}

		case "file_change":
			if evt.FilePath != "" && IsSensitivePath(evt.FilePath) {
				auditEvents = append(auditEvents, Event{
					Category: SensitiveFileWrite,
					ToolName: "file_change",
					Agent:    agent,
					TaskID:   taskID,
					Message:  evt.FilePath,
				})
			}
		}
	}

	return auditEvents
}
