// Package events provides a unified event abstraction for all agent adapters.
// It defines a common AgentEvent type that normalizes events from Claude Code,
// Codex, and future adapters into a single format for logging and analysis.
package events

import (
	"time"
)

// EventType identifies the category of an agent event.
type EventType string

const (
	// EventText is plain text output from the agent.
	EventText EventType = "text"
	// EventThinking is the agent's reasoning/thinking content.
	EventThinking EventType = "thinking"
	// EventToolUse is when the agent invokes a tool.
	EventToolUse EventType = "tool_use"
	// EventToolResult is the result returned from a tool invocation.
	EventToolResult EventType = "tool_result"
	// EventCommand is a command execution (e.g., bash).
	EventCommand EventType = "command"
	// EventFileChange is a file modification (create, edit, delete).
	EventFileChange EventType = "file_change"
	// EventError is an error event.
	EventError EventType = "error"
)

// AgentEvent is a unified event structure that normalizes events from all adapters.
// It provides a common schema for logging, debugging, and analysis.
type AgentEvent struct {
	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`

	// SessionID identifies the Agentium session.
	SessionID string `json:"session_id"`

	// Iteration is the agent iteration number (1-indexed).
	Iteration int `json:"iteration"`

	// Adapter is the source adapter (e.g., "claude-code", "codex").
	Adapter string `json:"adapter"`

	// Type categorizes the event (text, thinking, tool_use, etc.).
	Type EventType `json:"type"`

	// Summary is a short human-readable description (for log display).
	Summary string `json:"summary,omitempty"`

	// Content is the full event content (may be large for tool results).
	Content string `json:"content,omitempty"`

	// ToolName is the name of the tool invoked (for tool_use events).
	ToolName string `json:"tool_name,omitempty"`

	// ToolInput is the raw JSON input to the tool (for tool_use events).
	ToolInput string `json:"tool_input,omitempty"`

	// FilePath is the affected file path (for file_change events).
	FilePath string `json:"file_path,omitempty"`

	// Action is the file action (for file_change events: write, edit, delete).
	Action string `json:"action,omitempty"`
}

// ValidEventTypes returns all valid event type values.
func ValidEventTypes() []EventType {
	return []EventType{
		EventText,
		EventThinking,
		EventToolUse,
		EventToolResult,
		EventCommand,
		EventFileChange,
		EventError,
	}
}

// IsValidEventType checks if the given string is a valid event type.
func IsValidEventType(s string) bool {
	for _, t := range ValidEventTypes() {
		if string(t) == s {
			return true
		}
	}
	return false
}
