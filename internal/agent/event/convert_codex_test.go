package event

import (
	"testing"
	"time"

	"github.com/andywolf/agentium/internal/agent/codex"
)

func TestFromCodex_AgentMessage(t *testing.T) {
	before := time.Now().UTC()
	ce := codex.CodexEvent{
		Type: "item.completed",
		Item: &codex.EventItem{
			Type: "agent_message",
			Text: "I'll help you with that task.",
		},
	}

	evt := FromCodex(ce, "session-123", 1)
	after := time.Now().UTC()

	if evt.Type != EventText {
		t.Errorf("Type = %q, want %q", evt.Type, EventText)
	}
	if evt.Content != "I'll help you with that task." {
		t.Errorf("Content = %q, want %q", evt.Content, "I'll help you with that task.")
	}
	if evt.Adapter != "codex" {
		t.Errorf("Adapter = %q, want %q", evt.Adapter, "codex")
	}
	if evt.SessionID != "session-123" {
		t.Errorf("SessionID = %q, want %q", evt.SessionID, "session-123")
	}
	// Verify Timestamp is set and within expected range
	if evt.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
	if evt.Timestamp.Before(before) || evt.Timestamp.After(after) {
		t.Errorf("Timestamp = %v, want between %v and %v", evt.Timestamp, before, after)
	}
}

func TestFromCodex_CommandExecution(t *testing.T) {
	ce := codex.CodexEvent{
		Type: "item.completed",
		Item: &codex.EventItem{
			Type:    "command_execution",
			Command: "git status",
			Output:  "On branch main\nnothing to commit",
		},
	}

	evt := FromCodex(ce, "session-456", 2)

	if evt.Type != EventCommand {
		t.Errorf("Type = %q, want %q", evt.Type, EventCommand)
	}
	if evt.Content != "On branch main\nnothing to commit" {
		t.Errorf("Content = %q, want %q", evt.Content, "On branch main\nnothing to commit")
	}
	if evt.Metadata["action"] != "command_execution" {
		t.Errorf("Metadata[action] = %q, want %q", evt.Metadata["action"], "command_execution")
	}
	if evt.Metadata["command"] != "git status" {
		t.Errorf("Metadata[command] = %q, want %q", evt.Metadata["command"], "git status")
	}
}

func TestFromCodex_FileChange(t *testing.T) {
	ce := codex.CodexEvent{
		Type: "item.completed",
		Item: &codex.EventItem{
			Type:     "file_change",
			FilePath: "src/main.go",
			Action:   "modified",
		},
	}

	evt := FromCodex(ce, "session-789", 3)

	if evt.Type != EventFileChange {
		t.Errorf("Type = %q, want %q", evt.Type, EventFileChange)
	}
	if evt.Summary != "modified: src/main.go" {
		t.Errorf("Summary = %q, want %q", evt.Summary, "modified: src/main.go")
	}
	if evt.Metadata["action"] != "modified" {
		t.Errorf("Metadata[action] = %q, want %q", evt.Metadata["action"], "modified")
	}
	if evt.Metadata["file_path"] != "src/main.go" {
		t.Errorf("Metadata[file_path] = %q, want %q", evt.Metadata["file_path"], "src/main.go")
	}
	if evt.Content != "src/main.go" {
		t.Errorf("Content = %q, want %q", evt.Content, "src/main.go")
	}
}

func TestFromCodex_Delta(t *testing.T) {
	ce := codex.CodexEvent{
		Type: "item.delta",
		Delta: &codex.EventDelta{
			Text: "Streaming text...",
		},
	}

	evt := FromCodex(ce, "session-abc", 1)

	if evt.Type != EventText {
		t.Errorf("Type = %q, want %q", evt.Type, EventText)
	}
	if evt.Content != "Streaming text..." {
		t.Errorf("Content = %q, want %q", evt.Content, "Streaming text...")
	}
}

func TestFromCodex_Error(t *testing.T) {
	ce := codex.CodexEvent{
		Type: "error",
		Error: &codex.EventError{
			Message: "Something went wrong",
		},
	}

	evt := FromCodex(ce, "session-err", 1)

	if evt.Type != EventError {
		t.Errorf("Type = %q, want %q", evt.Type, EventError)
	}
	if evt.Content != "Something went wrong" {
		t.Errorf("Content = %q, want %q", evt.Content, "Something went wrong")
	}
}

func TestFromCodex_TurnFailed(t *testing.T) {
	ce := codex.CodexEvent{
		Type: "turn.failed",
		Error: &codex.EventError{
			Message: "API rate limit exceeded",
		},
	}

	evt := FromCodex(ce, "session-fail", 2)

	if evt.Type != EventError {
		t.Errorf("Type = %q, want %q", evt.Type, EventError)
	}
	if evt.Content != "API rate limit exceeded" {
		t.Errorf("Content = %q, want %q", evt.Content, "API rate limit exceeded")
	}
}

func TestFromCodex_SystemEvent(t *testing.T) {
	ce := codex.CodexEvent{
		Type: "turn.completed",
	}

	evt := FromCodex(ce, "session-sys", 1)

	if evt.Type != EventSystem {
		t.Errorf("Type = %q, want %q", evt.Type, EventSystem)
	}
	if evt.Summary != "turn.completed" {
		t.Errorf("Summary = %q, want %q", evt.Summary, "turn.completed")
	}
}

func TestFromCodexBatch(t *testing.T) {
	events := []codex.CodexEvent{
		{
			Type: "item.completed",
			Item: &codex.EventItem{Type: "agent_message", Text: "Hello"},
		},
		{
			Type: "turn.completed", // System event - has summary but no content
		},
		{
			Type: "item.completed",
			Item: &codex.EventItem{Type: "agent_message", Text: "World"},
		},
	}

	result := FromCodexBatch(events, "session-batch", 1)

	// All 3 events are included (system events have summary)
	if len(result) != 3 {
		t.Fatalf("len(result) = %d, want %d", len(result), 3)
	}
	if result[0].Content != "Hello" {
		t.Errorf("result[0].Content = %q, want %q", result[0].Content, "Hello")
	}
	if result[1].Type != EventSystem {
		t.Errorf("result[1].Type = %q, want %q", result[1].Type, EventSystem)
	}
	if result[2].Content != "World" {
		t.Errorf("result[2].Content = %q, want %q", result[2].Content, "World")
	}
}

func TestFromCodexBatch_SystemEvents(t *testing.T) {
	// System events have summary set to event type but no content
	events := []codex.CodexEvent{
		{Type: "turn.started"},
		{Type: "turn.completed"},
		{Type: "response.completed"},
	}

	result := FromCodexBatch(events, "session-sys", 1)

	// System events are kept because they have a summary
	if len(result) != 3 {
		t.Fatalf("len(result) = %d, want %d", len(result), 3)
	}
	for i, evt := range result {
		if evt.Type != EventSystem {
			t.Errorf("result[%d].Type = %q, want %q", i, evt.Type, EventSystem)
		}
		if evt.Summary == "" {
			t.Errorf("result[%d].Summary should not be empty", i)
		}
	}
}

func TestFromCodex_ItemCompletedNilItem(t *testing.T) {
	// Ensure item.completed with nil Item still produces a meaningful event
	ce := codex.CodexEvent{
		Type: "item.completed",
		Item: nil,
	}

	evt := FromCodex(ce, "session-nil", 1)

	if evt.Type != EventSystem {
		t.Errorf("Type = %q, want %q", evt.Type, EventSystem)
	}
	if evt.Summary != "item.completed" {
		t.Errorf("Summary = %q, want %q", evt.Summary, "item.completed")
	}
	if evt.Content != "item.completed (missing item)" {
		t.Errorf("Content = %q, want %q", evt.Content, "item.completed (missing item)")
	}
	if evt.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
}
