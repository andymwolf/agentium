package events

import (
	"time"

	"github.com/andywolf/agentium/internal/agent"
	"github.com/andywolf/agentium/internal/agent/claudecode"
	"github.com/andywolf/agentium/internal/agent/codex"
)

// ConvertParams holds parameters for event conversion.
type ConvertParams struct {
	SessionID string
	Iteration int
	Adapter   string
	Timestamp time.Time // Optional: defaults to time.Now() if zero
}

// FromIterationResult converts events from an IterationResult to unified AgentEvents.
// It type-switches on the event payload to handle Claude Code and Codex event formats.
// The Adapter field in params is used to label the resulting events.
func FromIterationResult(result *agent.IterationResult, params ConvertParams) []AgentEvent {
	if result == nil || len(result.Events) == 0 {
		return nil
	}

	ts := params.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}

	var events []AgentEvent

	for _, evt := range result.Events {
		switch e := evt.(type) {
		case claudecode.StreamEvent:
			converted := fromClaudeCodeEvent(e, params, ts)
			if converted != nil {
				events = append(events, *converted)
			}
		case codex.CodexEvent:
			converted := fromCodexEvent(e, params, ts)
			if converted != nil {
				events = append(events, *converted)
			}
		}
	}

	return events
}

// FromClaudeCode converts a slice of Claude Code StreamEvents to unified AgentEvents.
func FromClaudeCode(streamEvents []claudecode.StreamEvent, params ConvertParams) []AgentEvent {
	if len(streamEvents) == 0 {
		return nil
	}

	ts := params.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}

	var events []AgentEvent
	for _, se := range streamEvents {
		converted := fromClaudeCodeEvent(se, params, ts)
		if converted != nil {
			events = append(events, *converted)
		}
	}
	return events
}

// fromClaudeCodeEvent converts a single Claude Code StreamEvent to an AgentEvent.
func fromClaudeCodeEvent(se claudecode.StreamEvent, params ConvertParams, ts time.Time) *AgentEvent {
	event := &AgentEvent{
		Timestamp: ts,
		SessionID: params.SessionID,
		Iteration: params.Iteration,
		Adapter:   params.Adapter,
	}

	switch se.Subtype {
	case claudecode.BlockText:
		event.Type = EventText
		event.Content = se.Content
		event.Summary = truncate(se.Content, 100)

	case claudecode.BlockThinking:
		event.Type = EventThinking
		event.Content = se.Content
		event.Summary = truncate(se.Content, 100)

	case claudecode.BlockToolUse:
		event.Type = EventToolUse
		event.ToolName = se.ToolName
		if se.ToolInput != nil {
			event.ToolInput = string(se.ToolInput)
		}
		event.Summary = "Tool: " + se.ToolName

	case claudecode.BlockToolResult:
		event.Type = EventToolResult
		event.Content = se.Content
		event.Summary = truncate(se.Content, 100)

	default:
		// Skip system events and other unrecognized types
		return nil
	}

	return event
}

// FromCodex converts a slice of Codex CodexEvents to unified AgentEvents.
func FromCodex(codexEvents []codex.CodexEvent, params ConvertParams) []AgentEvent {
	if len(codexEvents) == 0 {
		return nil
	}

	ts := params.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}

	var events []AgentEvent
	for _, ce := range codexEvents {
		converted := fromCodexEvent(ce, params, ts)
		if converted != nil {
			events = append(events, *converted)
		}
	}
	return events
}

// fromCodexEvent converts a single Codex CodexEvent to an AgentEvent.
func fromCodexEvent(ce codex.CodexEvent, params ConvertParams, ts time.Time) *AgentEvent {
	// Only process "item.completed" events which contain structured action info
	if ce.Type != "item.completed" || ce.Item == nil {
		return nil
	}

	event := &AgentEvent{
		Timestamp: ts,
		SessionID: params.SessionID,
		Iteration: params.Iteration,
		Adapter:   params.Adapter,
	}

	switch ce.Item.Type {
	case "agent_message":
		event.Type = EventText
		event.Content = ce.Item.Text
		event.Summary = truncate(ce.Item.Text, 100)

	case "command_execution":
		event.Type = EventCommand
		event.ToolName = "bash"
		event.ToolInput = ce.Item.Command
		event.Content = ce.Item.Output
		event.Summary = "Command: " + truncate(ce.Item.Command, 80)

	case "file_change":
		event.Type = EventFileChange
		event.FilePath = ce.Item.FilePath
		event.Action = ce.Item.Action
		event.Summary = ce.Item.Action + ": " + ce.Item.FilePath

	default:
		return nil
	}

	return event
}

// truncate shortens a string to the specified maximum length, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
