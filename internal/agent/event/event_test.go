package event

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewEvent(t *testing.T) {
	evt := NewEvent("session-123", 2, "claude-code", EventToolUse, "Using Bash", "git status")

	if evt.SessionID != "session-123" {
		t.Errorf("SessionID = %q, want %q", evt.SessionID, "session-123")
	}
	if evt.Iteration != 2 {
		t.Errorf("Iteration = %d, want %d", evt.Iteration, 2)
	}
	if evt.Adapter != "claude-code" {
		t.Errorf("Adapter = %q, want %q", evt.Adapter, "claude-code")
	}
	if evt.Type != EventToolUse {
		t.Errorf("Type = %q, want %q", evt.Type, EventToolUse)
	}
	if evt.Summary != "Using Bash" {
		t.Errorf("Summary = %q, want %q", evt.Summary, "Using Bash")
	}
	if evt.Content != "git status" {
		t.Errorf("Content = %q, want %q", evt.Content, "git status")
	}
	if evt.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
}

func TestWithMetadata(t *testing.T) {
	evt := NewEvent("session-123", 1, "codex", EventCommand, "command", "ls -la")
	evt.WithMetadata("tool_name", "Bash").WithMetadata("file_path", "/workspace")

	if evt.Metadata["tool_name"] != "Bash" {
		t.Errorf("Metadata[tool_name] = %q, want %q", evt.Metadata["tool_name"], "Bash")
	}
	if evt.Metadata["file_path"] != "/workspace" {
		t.Errorf("Metadata[file_path] = %q, want %q", evt.Metadata["file_path"], "/workspace")
	}
}

func TestMarshalJSONL(t *testing.T) {
	evt := &AgentEvent{
		Timestamp: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		SessionID: "test-session",
		Iteration: 1,
		Adapter:   "claude-code",
		Type:      EventText,
		Summary:   "Hello",
		Content:   "Hello, world!",
		Metadata: map[string]string{
			"key": "value",
		},
	}

	data, err := evt.MarshalJSONL()
	if err != nil {
		t.Fatalf("MarshalJSONL failed: %v", err)
	}

	// Verify it's valid JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Result is not valid JSON: %v", err)
	}

	// Verify key fields
	if parsed["session_id"] != "test-session" {
		t.Errorf("session_id = %v, want %v", parsed["session_id"], "test-session")
	}
	if parsed["type"] != "text" {
		t.Errorf("type = %v, want %v", parsed["type"], "text")
	}
	if parsed["adapter"] != "claude-code" {
		t.Errorf("adapter = %v, want %v", parsed["adapter"], "claude-code")
	}
}

func TestTruncateSummary(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantLen int
	}{
		{
			name:    "short string",
			input:   "Hello, world!",
			wantLen: 13,
		},
		{
			name:    "exactly max length",
			input:   string(make([]byte, MaxSummaryLen)),
			wantLen: MaxSummaryLen,
		},
		{
			name:    "over max length",
			input:   string(make([]byte, MaxSummaryLen+100)),
			wantLen: MaxSummaryLen,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TruncateSummary(tt.input)
			if len(result) != tt.wantLen {
				t.Errorf("len(TruncateSummary(%d chars)) = %d, want %d",
					len(tt.input), len(result), tt.wantLen)
			}
			if len(tt.input) > MaxSummaryLen && result[len(result)-3:] != "..." {
				t.Error("Truncated summary should end with '...'")
			}
		})
	}
}

func TestEventTypes(t *testing.T) {
	// Verify all event types have expected string values
	tests := []struct {
		eventType EventType
		want      string
	}{
		{EventText, "text"},
		{EventThinking, "thinking"},
		{EventToolUse, "tool_use"},
		{EventToolResult, "tool_result"},
		{EventCommand, "command"},
		{EventFileChange, "file_change"},
		{EventError, "error"},
		{EventSystem, "system"},
	}

	for _, tt := range tests {
		if string(tt.eventType) != tt.want {
			t.Errorf("EventType %v = %q, want %q", tt.eventType, string(tt.eventType), tt.want)
		}
	}
}
