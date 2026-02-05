package event

import (
	"encoding/json"

	"github.com/andywolf/agentium/internal/agent/claudecode"
)

// FromClaudeCode converts a Claude Code StreamEvent to a unified AgentEvent.
func FromClaudeCode(se claudecode.StreamEvent, sessionID string, iteration int) *AgentEvent {
	evt := &AgentEvent{
		SessionID: sessionID,
		Iteration: iteration,
		Adapter:   "claude-code",
	}

	// Map Claude Code content block types to unified event types
	switch se.Subtype {
	case claudecode.BlockText:
		evt.Type = EventText
		evt.Content = se.Content
		evt.Summary = TruncateSummary(se.Content)

	case claudecode.BlockThinking:
		evt.Type = EventThinking
		evt.Content = se.Content
		evt.Summary = TruncateSummary(se.Content)

	case claudecode.BlockToolUse:
		evt.Type = EventToolUse
		evt.Content = string(se.ToolInput)
		evt.Summary = se.ToolName
		evt.Metadata = map[string]string{
			"tool_name": se.ToolName,
		}
		// Include tool input as formatted JSON if possible
		if len(se.ToolInput) > 0 {
			var inputMap map[string]interface{}
			if err := json.Unmarshal(se.ToolInput, &inputMap); err == nil {
				if formatted, err := json.MarshalIndent(inputMap, "", "  "); err == nil {
					evt.Content = string(formatted)
				}
			}
		}

	case claudecode.BlockToolResult:
		evt.Type = EventToolResult
		evt.Content = se.Content
		evt.Summary = TruncateSummary(se.Content)

	default:
		// For system events or unknown types
		evt.Type = EventSystem
		evt.Content = se.Content
		evt.Summary = string(se.Type) + ":" + string(se.Subtype)
	}

	return evt
}

// FromClaudeCodeBatch converts a slice of Claude Code StreamEvents to AgentEvents.
func FromClaudeCodeBatch(events []claudecode.StreamEvent, sessionID string, iteration int) []*AgentEvent {
	result := make([]*AgentEvent, 0, len(events))
	for _, se := range events {
		result = append(result, FromClaudeCode(se, sessionID, iteration))
	}
	return result
}
