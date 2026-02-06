package event

import (
	"time"

	"github.com/andywolf/agentium/internal/agent/codex"
)

// FromCodex converts a Codex CodexEvent to a unified AgentEvent.
func FromCodex(ce codex.CodexEvent, sessionID string, iteration int) *AgentEvent {
	evt := &AgentEvent{
		Timestamp: time.Now().UTC(),
		SessionID: sessionID,
		Iteration: iteration,
		Adapter:   "codex",
	}

	// Map Codex event types to unified event types
	switch ce.Type {
	case "item.completed":
		if ce.Item != nil {
			switch ce.Item.Type {
			case "agent_message":
				evt.Type = EventText
				evt.Content = ce.Item.Text
				evt.Summary = TruncateSummary(ce.Item.Text)

			case "command_execution":
				evt.Type = EventCommand
				evt.Content = ce.Item.Output
				evt.Summary = TruncateSummary(ce.Item.Command)
				evt.Metadata = map[string]string{
					"action":  "command_execution",
					"command": ce.Item.Command,
				}

			case "file_change":
				evt.Type = EventFileChange
				evt.Summary = ce.Item.Action + ": " + ce.Item.FilePath
				evt.Content = ce.Item.FilePath
				evt.Metadata = map[string]string{
					"action":    ce.Item.Action,
					"file_path": ce.Item.FilePath,
				}

			default:
				evt.Type = EventSystem
				evt.Summary = "item.completed:" + ce.Item.Type
				if ce.Item.Text != "" {
					evt.Content = ce.Item.Text
				}
			}
		} else {
			// item.completed with nil Item - provide meaningful fallback
			evt.Type = EventSystem
			evt.Summary = "item.completed"
			evt.Content = "item.completed (missing item)"
		}

	case "item.delta", "response.output_text.delta":
		// Streaming text delta
		evt.Type = EventText
		if ce.Delta != nil && ce.Delta.Text != "" {
			evt.Content = ce.Delta.Text
			evt.Summary = TruncateSummary(ce.Delta.Text)
		} else if ce.Item != nil && ce.Item.Text != "" {
			evt.Content = ce.Item.Text
			evt.Summary = TruncateSummary(ce.Item.Text)
		}

	case "error", "turn.failed":
		evt.Type = EventError
		if ce.Error != nil {
			evt.Content = ce.Error.Message
			evt.Summary = TruncateSummary(ce.Error.Message)
		}

	default:
		// For message, response.completed, turn.completed, etc.
		evt.Type = EventSystem
		evt.Summary = ce.Type
	}

	return evt
}

// FromCodexBatch converts a slice of Codex CodexEvents to AgentEvents.
// It filters out events that don't produce meaningful content.
func FromCodexBatch(events []codex.CodexEvent, sessionID string, iteration int) []*AgentEvent {
	result := make([]*AgentEvent, 0, len(events))
	for _, ce := range events {
		evt := FromCodex(ce, sessionID, iteration)
		// Skip empty events
		if evt.Content == "" && evt.Summary == "" {
			continue
		}
		result = append(result, evt)
	}
	return result
}
