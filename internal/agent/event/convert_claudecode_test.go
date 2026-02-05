package event

import (
	"encoding/json"
	"testing"

	"github.com/andywolf/agentium/internal/agent/claudecode"
)

func TestFromClaudeCode_Text(t *testing.T) {
	se := claudecode.StreamEvent{
		Type:    claudecode.EventAssistant,
		Subtype: claudecode.BlockText,
		Content: "Hello, I can help you with that.",
	}

	evt := FromClaudeCode(se, "session-123", 1)

	if evt.Type != EventText {
		t.Errorf("Type = %q, want %q", evt.Type, EventText)
	}
	if evt.Content != "Hello, I can help you with that." {
		t.Errorf("Content = %q, want %q", evt.Content, "Hello, I can help you with that.")
	}
	if evt.Adapter != "claude-code" {
		t.Errorf("Adapter = %q, want %q", evt.Adapter, "claude-code")
	}
	if evt.SessionID != "session-123" {
		t.Errorf("SessionID = %q, want %q", evt.SessionID, "session-123")
	}
	if evt.Iteration != 1 {
		t.Errorf("Iteration = %d, want %d", evt.Iteration, 1)
	}
}

func TestFromClaudeCode_Thinking(t *testing.T) {
	se := claudecode.StreamEvent{
		Type:    claudecode.EventAssistant,
		Subtype: claudecode.BlockThinking,
		Content: "Let me analyze this code...",
	}

	evt := FromClaudeCode(se, "session-456", 2)

	if evt.Type != EventThinking {
		t.Errorf("Type = %q, want %q", evt.Type, EventThinking)
	}
	if evt.Content != "Let me analyze this code..." {
		t.Errorf("Content = %q, want %q", evt.Content, "Let me analyze this code...")
	}
}

func TestFromClaudeCode_ToolUse(t *testing.T) {
	toolInput := json.RawMessage(`{"command": "git status", "description": "Check git status"}`)
	se := claudecode.StreamEvent{
		Type:      claudecode.EventAssistant,
		Subtype:   claudecode.BlockToolUse,
		ToolName:  "Bash",
		ToolInput: toolInput,
	}

	evt := FromClaudeCode(se, "session-789", 3)

	if evt.Type != EventToolUse {
		t.Errorf("Type = %q, want %q", evt.Type, EventToolUse)
	}
	if evt.Summary != "Bash" {
		t.Errorf("Summary = %q, want %q", evt.Summary, "Bash")
	}
	if evt.Metadata["tool_name"] != "Bash" {
		t.Errorf("Metadata[tool_name] = %q, want %q", evt.Metadata["tool_name"], "Bash")
	}
	// Content should be formatted JSON
	if evt.Content == "" {
		t.Error("Content should not be empty for tool use")
	}
}

func TestFromClaudeCode_ToolResult(t *testing.T) {
	se := claudecode.StreamEvent{
		Type:    claudecode.EventUser,
		Subtype: claudecode.BlockToolResult,
		Content: "On branch main\nnothing to commit",
	}

	evt := FromClaudeCode(se, "session-abc", 1)

	if evt.Type != EventToolResult {
		t.Errorf("Type = %q, want %q", evt.Type, EventToolResult)
	}
	if evt.Content != "On branch main\nnothing to commit" {
		t.Errorf("Content = %q, want %q", evt.Content, "On branch main\nnothing to commit")
	}
}

func TestFromClaudeCode_System(t *testing.T) {
	se := claudecode.StreamEvent{
		Type:    claudecode.EventSystem,
		Subtype: "init",
	}

	evt := FromClaudeCode(se, "session-xyz", 1)

	if evt.Type != EventSystem {
		t.Errorf("Type = %q, want %q", evt.Type, EventSystem)
	}
}

func TestFromClaudeCodeBatch(t *testing.T) {
	events := []claudecode.StreamEvent{
		{Type: claudecode.EventAssistant, Subtype: claudecode.BlockText, Content: "First"},
		{Type: claudecode.EventAssistant, Subtype: claudecode.BlockThinking, Content: "Thinking..."},
		{Type: claudecode.EventAssistant, Subtype: claudecode.BlockText, Content: "Second"},
	}

	result := FromClaudeCodeBatch(events, "session-batch", 5)

	if len(result) != 3 {
		t.Fatalf("len(result) = %d, want %d", len(result), 3)
	}
	if result[0].Content != "First" {
		t.Errorf("result[0].Content = %q, want %q", result[0].Content, "First")
	}
	if result[1].Type != EventThinking {
		t.Errorf("result[1].Type = %q, want %q", result[1].Type, EventThinking)
	}
	if result[2].Content != "Second" {
		t.Errorf("result[2].Content = %q, want %q", result[2].Content, "Second")
	}
}
