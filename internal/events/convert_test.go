package events

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/andywolf/agentium/internal/agent"
	"github.com/andywolf/agentium/internal/agent/claudecode"
	"github.com/andywolf/agentium/internal/agent/codex"
)

func TestFromClaudeCode(t *testing.T) {
	now := time.Now()
	params := ConvertParams{
		SessionID: "test-session",
		Iteration: 1,
		Adapter:   "claude-code",
		Timestamp: now,
	}

	tests := []struct {
		name     string
		input    []claudecode.StreamEvent
		expected []AgentEvent
	}{
		{
			name: "text event",
			input: []claudecode.StreamEvent{
				{Type: claudecode.EventAssistant, Subtype: claudecode.BlockText, Content: "Hello world"},
			},
			expected: []AgentEvent{
				{
					Timestamp: now,
					SessionID: "test-session",
					Iteration: 1,
					Adapter:   "claude-code",
					Type:      EventText,
					Content:   "Hello world",
					Summary:   "Hello world",
				},
			},
		},
		{
			name: "thinking event",
			input: []claudecode.StreamEvent{
				{Type: claudecode.EventAssistant, Subtype: claudecode.BlockThinking, Content: "Let me think..."},
			},
			expected: []AgentEvent{
				{
					Timestamp: now,
					SessionID: "test-session",
					Iteration: 1,
					Adapter:   "claude-code",
					Type:      EventThinking,
					Content:   "Let me think...",
					Summary:   "Let me think...",
				},
			},
		},
		{
			name: "tool use event",
			input: []claudecode.StreamEvent{
				{
					Type:      claudecode.EventAssistant,
					Subtype:   claudecode.BlockToolUse,
					ToolName:  "Bash",
					ToolInput: json.RawMessage(`{"command": "ls -la"}`),
				},
			},
			expected: []AgentEvent{
				{
					Timestamp: now,
					SessionID: "test-session",
					Iteration: 1,
					Adapter:   "claude-code",
					Type:      EventToolUse,
					ToolName:  "Bash",
					ToolInput: `{"command": "ls -la"}`,
					Summary:   "Tool: Bash",
				},
			},
		},
		{
			name: "tool result event",
			input: []claudecode.StreamEvent{
				{Type: claudecode.EventUser, Subtype: claudecode.BlockToolResult, Content: "file1.txt\nfile2.txt"},
			},
			expected: []AgentEvent{
				{
					Timestamp: now,
					SessionID: "test-session",
					Iteration: 1,
					Adapter:   "claude-code",
					Type:      EventToolResult,
					Content:   "file1.txt\nfile2.txt",
					Summary:   "file1.txt\nfile2.txt",
				},
			},
		},
		{
			name:     "empty input",
			input:    []claudecode.StreamEvent{},
			expected: nil,
		},
		{
			name:     "nil input",
			input:    nil,
			expected: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := FromClaudeCode(tc.input, params)

			if tc.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}

			if len(result) != len(tc.expected) {
				t.Fatalf("expected %d events, got %d", len(tc.expected), len(result))
			}

			for i, exp := range tc.expected {
				got := result[i]
				if got.Type != exp.Type {
					t.Errorf("event[%d].Type = %q, want %q", i, got.Type, exp.Type)
				}
				if got.SessionID != exp.SessionID {
					t.Errorf("event[%d].SessionID = %q, want %q", i, got.SessionID, exp.SessionID)
				}
				if got.Iteration != exp.Iteration {
					t.Errorf("event[%d].Iteration = %d, want %d", i, got.Iteration, exp.Iteration)
				}
				if got.Adapter != exp.Adapter {
					t.Errorf("event[%d].Adapter = %q, want %q", i, got.Adapter, exp.Adapter)
				}
				if got.Content != exp.Content {
					t.Errorf("event[%d].Content = %q, want %q", i, got.Content, exp.Content)
				}
				if got.ToolName != exp.ToolName {
					t.Errorf("event[%d].ToolName = %q, want %q", i, got.ToolName, exp.ToolName)
				}
				if got.ToolInput != exp.ToolInput {
					t.Errorf("event[%d].ToolInput = %q, want %q", i, got.ToolInput, exp.ToolInput)
				}
			}
		})
	}
}

func TestFromCodex(t *testing.T) {
	now := time.Now()
	params := ConvertParams{
		SessionID: "test-session",
		Iteration: 2,
		Adapter:   "codex",
		Timestamp: now,
	}

	tests := []struct {
		name     string
		input    []codex.CodexEvent
		expected []AgentEvent
	}{
		{
			name: "agent message event",
			input: []codex.CodexEvent{
				{
					Type: "item.completed",
					Item: &codex.EventItem{Type: "agent_message", Text: "I'll help you with that."},
				},
			},
			expected: []AgentEvent{
				{
					Timestamp: now,
					SessionID: "test-session",
					Iteration: 2,
					Adapter:   "codex",
					Type:      EventText,
					Content:   "I'll help you with that.",
					Summary:   "I'll help you with that.",
				},
			},
		},
		{
			name: "command execution event",
			input: []codex.CodexEvent{
				{
					Type: "item.completed",
					Item: &codex.EventItem{
						Type:    "command_execution",
						Command: "ls -la",
						Output:  "total 8\ndrwxr-xr-x 2 user user 4096 Jan 1 00:00 .",
					},
				},
			},
			expected: []AgentEvent{
				{
					Timestamp: now,
					SessionID: "test-session",
					Iteration: 2,
					Adapter:   "codex",
					Type:      EventCommand,
					ToolName:  "bash",
					ToolInput: "ls -la",
					Content:   "total 8\ndrwxr-xr-x 2 user user 4096 Jan 1 00:00 .",
					Summary:   "Command: ls -la",
				},
			},
		},
		{
			name: "file change event",
			input: []codex.CodexEvent{
				{
					Type: "item.completed",
					Item: &codex.EventItem{
						Type:     "file_change",
						FilePath: "src/main.go",
						Action:   "write",
					},
				},
			},
			expected: []AgentEvent{
				{
					Timestamp: now,
					SessionID: "test-session",
					Iteration: 2,
					Adapter:   "codex",
					Type:      EventFileChange,
					FilePath:  "src/main.go",
					Action:    "write",
					Summary:   "write: src/main.go",
				},
			},
		},
		{
			name: "non-item.completed events are skipped",
			input: []codex.CodexEvent{
				{Type: "item.delta"},
				{Type: "turn.completed"},
			},
			expected: nil,
		},
		{
			name: "events with nil Item are skipped",
			input: []codex.CodexEvent{
				{Type: "item.completed", Item: nil},
			},
			expected: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := FromCodex(tc.input, params)

			if tc.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}

			if len(result) != len(tc.expected) {
				t.Fatalf("expected %d events, got %d", len(tc.expected), len(result))
			}

			for i, exp := range tc.expected {
				got := result[i]
				if got.Type != exp.Type {
					t.Errorf("event[%d].Type = %q, want %q", i, got.Type, exp.Type)
				}
				if got.ToolName != exp.ToolName {
					t.Errorf("event[%d].ToolName = %q, want %q", i, got.ToolName, exp.ToolName)
				}
				if got.FilePath != exp.FilePath {
					t.Errorf("event[%d].FilePath = %q, want %q", i, got.FilePath, exp.FilePath)
				}
				if got.Action != exp.Action {
					t.Errorf("event[%d].Action = %q, want %q", i, got.Action, exp.Action)
				}
			}
		})
	}
}

func TestFromIterationResult(t *testing.T) {
	now := time.Now()
	params := ConvertParams{
		SessionID: "test-session",
		Iteration: 1,
		Adapter:   "claude-code",
		Timestamp: now,
	}

	t.Run("mixed adapter events", func(t *testing.T) {
		result := &agent.IterationResult{
			Events: []interface{}{
				claudecode.StreamEvent{
					Type:    claudecode.EventAssistant,
					Subtype: claudecode.BlockText,
					Content: "Hello from Claude",
				},
				codex.CodexEvent{
					Type: "item.completed",
					Item: &codex.EventItem{Type: "agent_message", Text: "Hello from Codex"},
				},
			},
		}

		events := FromIterationResult(result, params)
		if len(events) != 2 {
			t.Fatalf("expected 2 events, got %d", len(events))
		}

		if events[0].Type != EventText || events[0].Content != "Hello from Claude" {
			t.Errorf("first event not converted correctly: %+v", events[0])
		}
		if events[1].Type != EventText || events[1].Content != "Hello from Codex" {
			t.Errorf("second event not converted correctly: %+v", events[1])
		}
	})

	t.Run("nil result", func(t *testing.T) {
		events := FromIterationResult(nil, params)
		if events != nil {
			t.Errorf("expected nil, got %v", events)
		}
	})

	t.Run("empty events", func(t *testing.T) {
		result := &agent.IterationResult{Events: []interface{}{}}
		events := FromIterationResult(result, params)
		if events != nil {
			t.Errorf("expected nil, got %v", events)
		}
	})
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello world", 8, "hello..."},
		{"abc", 3, "abc"},
		{"abcd", 3, "abc"},
		{"", 10, ""},
		{"hello", 0, ""},
	}

	for _, tc := range tests {
		result := truncate(tc.input, tc.maxLen)
		if result != tc.expected {
			t.Errorf("truncate(%q, %d) = %q, want %q", tc.input, tc.maxLen, result, tc.expected)
		}
	}
}
