// Package event provides a unified agent event abstraction for capturing and streaming
// structured events from all agent adapters (Claude Code, Codex, Aider).
package event

import (
	"encoding/json"
	"time"
)

// EventType enumerates the types of events that agents can emit.
type EventType string

const (
	// EventText represents text output from the agent.
	EventText EventType = "text"
	// EventThinking represents the agent's reasoning/thinking process.
	EventThinking EventType = "thinking"
	// EventToolUse represents a tool invocation by the agent.
	EventToolUse EventType = "tool_use"
	// EventToolResult represents the result of a tool invocation.
	EventToolResult EventType = "tool_result"
	// EventCommand represents a command execution (shell, git, etc).
	EventCommand EventType = "command"
	// EventFileChange represents a file modification (create, update, delete).
	EventFileChange EventType = "file_change"
	// EventError represents an error event.
	EventError EventType = "error"
	// EventSystem represents system-level events (session start, iteration, etc).
	EventSystem EventType = "system"
)

// AgentEvent is the unified event structure that all adapter events are converted to.
// This provides a common format for logging, storage, and analysis across all adapters.
type AgentEvent struct {
	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`
	// SessionID identifies the session that produced this event.
	SessionID string `json:"session_id"`
	// Iteration is the iteration number within the session.
	Iteration int `json:"iteration"`
	// Adapter is the name of the agent adapter that produced this event (e.g., "claude-code", "codex").
	Adapter string `json:"adapter"`
	// Type is the event type (text, thinking, tool_use, etc).
	Type EventType `json:"type"`
	// Summary is a short human-readable description of the event.
	Summary string `json:"summary"`
	// Content is the full event content (may be large for tool results, thinking).
	Content string `json:"content"`
	// Metadata contains optional key-value pairs for additional context.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// MarshalJSONL marshals the event to a JSON line (no trailing newline).
func (e *AgentEvent) MarshalJSONL() ([]byte, error) {
	return json.Marshal(e)
}

// NewEvent creates a new AgentEvent with the given parameters.
func NewEvent(sessionID string, iteration int, adapter string, eventType EventType, summary, content string) *AgentEvent {
	return &AgentEvent{
		Timestamp: time.Now(),
		SessionID: sessionID,
		Iteration: iteration,
		Adapter:   adapter,
		Type:      eventType,
		Summary:   summary,
		Content:   content,
	}
}

// WithMetadata adds metadata to the event and returns it for chaining.
func (e *AgentEvent) WithMetadata(key, value string) *AgentEvent {
	if e.Metadata == nil {
		e.Metadata = make(map[string]string)
	}
	e.Metadata[key] = value
	return e
}

// MaxSummaryLen is the maximum length for event summaries.
const MaxSummaryLen = 200

// TruncateSummary returns a truncated version of content suitable for the Summary field.
func TruncateSummary(content string) string {
	if len(content) <= MaxSummaryLen {
		return content
	}
	return content[:MaxSummaryLen-3] + "..."
}
