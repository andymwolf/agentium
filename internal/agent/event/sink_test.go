package event

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileSink_WriteAndRead(t *testing.T) {
	// Create a temp file
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "events.jsonl")

	sink, err := NewFileSink(path)
	if err != nil {
		t.Fatalf("NewFileSink failed: %v", err)
	}
	defer func() { _ = sink.Close() }()

	// Write some events
	evt1 := &AgentEvent{
		Timestamp: time.Now(),
		SessionID: "session-1",
		Iteration: 1,
		Adapter:   "claude-code",
		Type:      EventText,
		Summary:   "Hello",
		Content:   "Hello, world!",
	}
	evt2 := &AgentEvent{
		Timestamp: time.Now(),
		SessionID: "session-1",
		Iteration: 1,
		Adapter:   "claude-code",
		Type:      EventToolUse,
		Summary:   "Bash",
		Content:   "git status",
		Metadata: map[string]string{
			"tool_name": "Bash",
		},
	}

	err = sink.Write(evt1)
	if err != nil {
		t.Fatalf("Write(evt1) failed: %v", err)
	}
	err = sink.Write(evt2)
	if err != nil {
		t.Fatalf("Write(evt2) failed: %v", err)
	}
	err = sink.Flush()
	if err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	// Read and verify
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	var events []*AgentEvent
	for scanner.Scan() {
		var evt AgentEvent
		if unmarshalErr := json.Unmarshal(scanner.Bytes(), &evt); unmarshalErr != nil {
			t.Fatalf("Unmarshal failed: %v", unmarshalErr)
		}
		events = append(events, &evt)
	}

	if len(events) != 2 {
		t.Fatalf("len(events) = %d, want %d", len(events), 2)
	}
	if events[0].Type != EventText {
		t.Errorf("events[0].Type = %q, want %q", events[0].Type, EventText)
	}
	if events[1].Type != EventToolUse {
		t.Errorf("events[1].Type = %q, want %q", events[1].Type, EventToolUse)
	}
	if events[1].Metadata["tool_name"] != "Bash" {
		t.Errorf("events[1].Metadata[tool_name] = %q, want %q", events[1].Metadata["tool_name"], "Bash")
	}
}

func TestFileSink_WriteBatch(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "batch_events.jsonl")

	sink, err := NewFileSink(path)
	if err != nil {
		t.Fatalf("NewFileSink failed: %v", err)
	}

	events := []*AgentEvent{
		NewEvent("session-1", 1, "codex", EventText, "First", "First message"),
		NewEvent("session-1", 1, "codex", EventText, "Second", "Second message"),
		NewEvent("session-1", 1, "codex", EventText, "Third", "Third message"),
	}

	err = sink.WriteBatch(events)
	if err != nil {
		t.Fatalf("WriteBatch failed: %v", err)
	}
	err = sink.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Count lines in file
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	lineCount := 0
	for scanner.Scan() {
		lineCount++
	}

	if lineCount != 3 {
		t.Errorf("lineCount = %d, want %d", lineCount, 3)
	}
}

func TestFileSink_Path(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test_path.jsonl")

	sink, err := NewFileSink(path)
	if err != nil {
		t.Fatalf("NewFileSink failed: %v", err)
	}
	defer func() { _ = sink.Close() }()

	if sink.Path() != path {
		t.Errorf("Path() = %q, want %q", sink.Path(), path)
	}
}

func TestFileSink_AppendMode(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "append_test.jsonl")

	// First write
	sink1, err := NewFileSink(path)
	if err != nil {
		t.Fatalf("NewFileSink (1) failed: %v", err)
	}
	err = sink1.Write(NewEvent("s1", 1, "a", EventText, "First", "First"))
	if err != nil {
		t.Fatalf("Write (1) failed: %v", err)
	}
	err = sink1.Close()
	if err != nil {
		t.Fatalf("Close (1) failed: %v", err)
	}

	// Second write (should append)
	sink2, err := NewFileSink(path)
	if err != nil {
		t.Fatalf("NewFileSink (2) failed: %v", err)
	}
	err = sink2.Write(NewEvent("s1", 2, "a", EventText, "Second", "Second"))
	if err != nil {
		t.Fatalf("Write (2) failed: %v", err)
	}
	err = sink2.Close()
	if err != nil {
		t.Fatalf("Close (2) failed: %v", err)
	}

	// Verify both events are present
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	lineCount := 0
	for scanner.Scan() {
		lineCount++
	}

	if lineCount != 2 {
		t.Errorf("lineCount = %d, want %d (append mode should preserve first event)", lineCount, 2)
	}
}

func TestFileSink_InvalidPath(t *testing.T) {
	// Try to create sink in non-existent directory
	_, err := NewFileSink("/nonexistent/dir/events.jsonl")
	if err == nil {
		t.Error("NewFileSink should fail for invalid path")
	}
}
