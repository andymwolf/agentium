package claudecode

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseStreamJSON_EmptyInput(t *testing.T) {
	result := ParseStreamJSON([]byte(""))
	if len(result.Events) != 0 {
		t.Errorf("expected 0 events, got %d", len(result.Events))
	}
	if result.TextContent != "" {
		t.Errorf("expected empty TextContent, got %q", result.TextContent)
	}
	if result.TotalTokens != nil {
		t.Errorf("expected nil TotalTokens, got %+v", result.TotalTokens)
	}
}

func TestParseStreamJSON_TextMessage(t *testing.T) {
	input := `{"type":"assistant","message":{"content":[{"type":"text","text":"Hello, world!"}]}}` + "\n"
	result := ParseStreamJSON([]byte(input))

	if len(result.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(result.Events))
	}

	evt := result.Events[0]
	if evt.Type != EventAssistant {
		t.Errorf("event type = %q, want %q", evt.Type, EventAssistant)
	}
	if evt.Subtype != BlockText {
		t.Errorf("event subtype = %q, want %q", evt.Subtype, BlockText)
	}
	if evt.Content != "Hello, world!" {
		t.Errorf("event content = %q, want %q", evt.Content, "Hello, world!")
	}
	if result.TextContent != "Hello, world!" {
		t.Errorf("TextContent = %q, want %q", result.TextContent, "Hello, world!")
	}
}

func TestParseStreamJSON_ToolUseEvent(t *testing.T) {
	input := `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"ls -la"}}]}}` + "\n"
	result := ParseStreamJSON([]byte(input))

	if len(result.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(result.Events))
	}

	evt := result.Events[0]
	if evt.Type != EventAssistant {
		t.Errorf("event type = %q, want %q", evt.Type, EventAssistant)
	}
	if evt.Subtype != BlockToolUse {
		t.Errorf("event subtype = %q, want %q", evt.Subtype, BlockToolUse)
	}
	if evt.ToolName != "Bash" {
		t.Errorf("tool name = %q, want %q", evt.ToolName, "Bash")
	}
	if evt.ToolInput == nil {
		t.Fatal("expected non-nil ToolInput")
	}

	var input_map map[string]string
	if err := json.Unmarshal(evt.ToolInput, &input_map); err != nil {
		t.Fatalf("failed to unmarshal ToolInput: %v", err)
	}
	if input_map["command"] != "ls -la" {
		t.Errorf("ToolInput.command = %q, want %q", input_map["command"], "ls -la")
	}
}

func TestParseStreamJSON_ToolResultEvent(t *testing.T) {
	input := `{"type":"user","message":{"content":[{"type":"tool_result","content":"file1.go\nfile2.go"}]}}` + "\n"
	result := ParseStreamJSON([]byte(input))

	if len(result.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(result.Events))
	}

	evt := result.Events[0]
	if evt.Type != EventUser {
		t.Errorf("event type = %q, want %q", evt.Type, EventUser)
	}
	if evt.Subtype != BlockToolResult {
		t.Errorf("event subtype = %q, want %q", evt.Subtype, BlockToolResult)
	}
	if evt.Content != "file1.go\nfile2.go" {
		t.Errorf("event content = %q, want %q", evt.Content, "file1.go\nfile2.go")
	}
	// Tool result content should be included in TextContent
	if !strings.Contains(result.TextContent, "file1.go") {
		t.Errorf("TextContent should contain tool result content, got %q", result.TextContent)
	}
}

func TestParseStreamJSON_ThinkingBlock(t *testing.T) {
	thinking := "Let me analyze this problem step by step..."
	input := `{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"` + thinking + `"}]}}` + "\n"
	result := ParseStreamJSON([]byte(input))

	if len(result.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(result.Events))
	}

	evt := result.Events[0]
	if evt.Subtype != BlockThinking {
		t.Errorf("event subtype = %q, want %q", evt.Subtype, BlockThinking)
	}
	if evt.Content != thinking {
		t.Errorf("event content = %q, want %q", evt.Content, thinking)
	}
}

func TestParseStreamJSON_ThinkingTruncation(t *testing.T) {
	// Create thinking content that exceeds MaxThinkingBytes
	longThinking := strings.Repeat("x", MaxThinkingBytes+1000)
	input := `{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"` + longThinking + `"}]}}` + "\n"
	result := ParseStreamJSON([]byte(input))

	if len(result.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(result.Events))
	}

	if len(result.Events[0].Content) != MaxThinkingBytes {
		t.Errorf("thinking content length = %d, want %d (truncated)", len(result.Events[0].Content), MaxThinkingBytes)
	}
}

func TestParseStreamJSON_ResultWithTokens(t *testing.T) {
	input := `{"type":"result","result":{"content":[{"type":"text","text":"Done."}],"usage":{"input_tokens":1500,"output_tokens":300},"stop_reason":"end_turn"}}` + "\n"
	result := ParseStreamJSON([]byte(input))

	if result.TotalTokens == nil {
		t.Fatal("expected non-nil TotalTokens")
	}
	if result.TotalTokens.InputTokens != 1500 {
		t.Errorf("InputTokens = %d, want %d", result.TotalTokens.InputTokens, 1500)
	}
	if result.TotalTokens.OutputTokens != 300 {
		t.Errorf("OutputTokens = %d, want %d", result.TotalTokens.OutputTokens, 300)
	}
	if result.StopReason != "end_turn" {
		t.Errorf("StopReason = %q, want %q", result.StopReason, "end_turn")
	}
	if result.TextContent != "Done." {
		t.Errorf("TextContent = %q, want %q", result.TextContent, "Done.")
	}
}

func TestParseStreamJSON_ResultWithTopLevelUsage(t *testing.T) {
	// Current Claude Code format: usage and stop_reason at the top level, result is a plain string.
	input := `{"type":"result","subtype":"success","is_error":false,"result":"4","stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":5,"cache_creation_input_tokens":23101}}` + "\n"
	result := ParseStreamJSON([]byte(input))

	if result.TotalTokens == nil {
		t.Fatal("expected non-nil TotalTokens")
	}
	if result.TotalTokens.InputTokens != 2 {
		t.Errorf("InputTokens = %d, want %d", result.TotalTokens.InputTokens, 2)
	}
	if result.TotalTokens.OutputTokens != 5 {
		t.Errorf("OutputTokens = %d, want %d", result.TotalTokens.OutputTokens, 5)
	}
	if result.StopReason != "end_turn" {
		t.Errorf("StopReason = %q, want %q", result.StopReason, "end_turn")
	}
}

func TestParseStreamJSON_MalformedLineSkipped(t *testing.T) {
	input := "not valid json\n" +
		`{"type":"assistant","message":{"content":[{"type":"text","text":"valid"}]}}` + "\n" +
		"another bad line\n"

	result := ParseStreamJSON([]byte(input))

	if len(result.Events) != 1 {
		t.Fatalf("expected 1 event (malformed skipped), got %d", len(result.Events))
	}
	if result.Events[0].Content != "valid" {
		t.Errorf("event content = %q, want %q", result.Events[0].Content, "valid")
	}
}

func TestParseStreamJSON_SignalsInTextContent(t *testing.T) {
	input := `{"type":"assistant","message":{"content":[{"type":"text","text":"Work complete\nAGENTIUM_STATUS: PR_CREATED https://github.com/org/repo/pull/42"}]}}` + "\n"
	result := ParseStreamJSON([]byte(input))

	if !strings.Contains(result.TextContent, "AGENTIUM_STATUS: PR_CREATED") {
		t.Errorf("TextContent should contain AGENTIUM_STATUS signal, got %q", result.TextContent)
	}
}

func TestParseStreamJSON_MultipleEvents(t *testing.T) {
	input := `{"type":"system","subtype":"init"}` + "\n" +
		`{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"Analyzing..."},{"type":"text","text":"I'll check the files."},{"type":"tool_use","name":"Read","input":{"path":"/foo/bar.go"}}]}}` + "\n" +
		`{"type":"user","message":{"content":[{"type":"tool_result","content":"package foo"}]}}` + "\n" +
		`{"type":"assistant","message":{"content":[{"type":"text","text":"The file contains package foo."}]}}` + "\n" +
		`{"type":"result","result":{"content":[{"type":"text","text":"Task complete."}],"usage":{"input_tokens":2000,"output_tokens":500},"stop_reason":"end_turn"}}` + "\n"

	result := ParseStreamJSON([]byte(input))

	// system(1) + thinking(1) + text(1) + tool_use(1) + tool_result(1) + text(1) + result_text(1) = 7
	if len(result.Events) != 7 {
		t.Fatalf("expected 7 events, got %d", len(result.Events))
	}

	// Verify event types
	expectedTypes := []struct {
		evtType StreamEventType
		subtype ContentBlockType
	}{
		{EventSystem, "init"},
		{EventAssistant, BlockThinking},
		{EventAssistant, BlockText},
		{EventAssistant, BlockToolUse},
		{EventUser, BlockToolResult},
		{EventAssistant, BlockText},
		{EventResult, BlockText},
	}

	for i, exp := range expectedTypes {
		if result.Events[i].Type != exp.evtType {
			t.Errorf("event[%d].Type = %q, want %q", i, result.Events[i].Type, exp.evtType)
		}
		if result.Events[i].Subtype != exp.subtype {
			t.Errorf("event[%d].Subtype = %q, want %q", i, result.Events[i].Subtype, exp.subtype)
		}
	}

	// TextContent should aggregate all text blocks and tool results
	if !strings.Contains(result.TextContent, "I'll check the files.") {
		t.Error("TextContent missing first text block")
	}
	if !strings.Contains(result.TextContent, "package foo") {
		t.Error("TextContent missing tool result content")
	}
	if !strings.Contains(result.TextContent, "Task complete.") {
		t.Error("TextContent missing result text block")
	}

	// Token usage from result
	if result.TotalTokens.InputTokens != 2000 {
		t.Errorf("InputTokens = %d, want 2000", result.TotalTokens.InputTokens)
	}
	if result.StopReason != "end_turn" {
		t.Errorf("StopReason = %q, want %q", result.StopReason, "end_turn")
	}
}

func TestParseStreamJSON_SystemEvent(t *testing.T) {
	input := `{"type":"system","subtype":"init"}` + "\n"
	result := ParseStreamJSON([]byte(input))

	if len(result.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(result.Events))
	}
	if result.Events[0].Type != EventSystem {
		t.Errorf("event type = %q, want %q", result.Events[0].Type, EventSystem)
	}
}

func TestParseStreamJSON_ToolResultArrayContent(t *testing.T) {
	// Tool result with content as an array of text blocks
	input := `{"type":"user","message":{"content":[{"type":"tool_result","content":[{"type":"text","text":"first part"},{"type":"text","text":"second part"}]}]}}` + "\n"
	result := ParseStreamJSON([]byte(input))

	if len(result.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(result.Events))
	}

	evt := result.Events[0]
	if evt.Content != "first part\nsecond part" {
		t.Errorf("event content = %q, want %q", evt.Content, "first part\nsecond part")
	}
	if !strings.Contains(result.TextContent, "first part") {
		t.Error("TextContent missing first part of array content")
	}
	if !strings.Contains(result.TextContent, "second part") {
		t.Error("TextContent missing second part of array content")
	}
}

func TestParseStreamJSON_ToolResultNilContent(t *testing.T) {
	input := `{"type":"user","message":{"content":[{"type":"tool_result"}]}}` + "\n"
	result := ParseStreamJSON([]byte(input))

	if len(result.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(result.Events))
	}
	if result.Events[0].Content != "" {
		t.Errorf("expected empty content for nil tool_result, got %q", result.Events[0].Content)
	}
}

func TestParseStreamJSON_EmptyLines(t *testing.T) {
	input := "\n\n" +
		`{"type":"assistant","message":{"content":[{"type":"text","text":"hello"}]}}` + "\n\n\n"
	result := ParseStreamJSON([]byte(input))

	if len(result.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(result.Events))
	}
	if result.TextContent != "hello" {
		t.Errorf("TextContent = %q, want %q", result.TextContent, "hello")
	}
}
