package events

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileSink(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "events-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	t.Run("create and write events", func(t *testing.T) {
		sink, err := NewFileSink(tmpDir)
		if err != nil {
			t.Fatalf("failed to create file sink: %v", err)
		}

		// Verify path
		expectedPath := filepath.Join(tmpDir, DefaultFilename)
		if sink.Path() != expectedPath {
			t.Errorf("Path() = %q, want %q", sink.Path(), expectedPath)
		}

		// Write events
		testEvents := []AgentEvent{
			{
				Timestamp: time.Now(),
				SessionID: "session-1",
				Iteration: 1,
				Adapter:   "claude-code",
				Type:      EventText,
				Content:   "Hello world",
				Summary:   "Hello world",
			},
			{
				Timestamp: time.Now(),
				SessionID: "session-1",
				Iteration: 1,
				Adapter:   "claude-code",
				Type:      EventToolUse,
				ToolName:  "Bash",
				ToolInput: `{"command": "ls"}`,
				Summary:   "Tool: Bash",
			},
		}

		if writeErr := sink.Write(testEvents); writeErr != nil {
			t.Fatalf("failed to write events: %v", writeErr)
		}

		// Close sink
		if closeErr := sink.Close(); closeErr != nil {
			t.Fatalf("failed to close sink: %v", closeErr)
		}

		// Read back events
		readEvents, readErr := ReadEvents(sink.Path())
		if readErr != nil {
			t.Fatalf("failed to read events: %v", readErr)
		}

		if len(readEvents) != 2 {
			t.Fatalf("expected 2 events, got %d", len(readEvents))
		}

		if readEvents[0].Type != EventText {
			t.Errorf("event[0].Type = %q, want %q", readEvents[0].Type, EventText)
		}
		if readEvents[1].Type != EventToolUse {
			t.Errorf("event[1].Type = %q, want %q", readEvents[1].Type, EventToolUse)
		}
	})

	t.Run("append mode", func(t *testing.T) {
		// Create new temp dir for this test
		dir, dirErr := os.MkdirTemp("", "events-append-*")
		if dirErr != nil {
			t.Fatalf("failed to create temp dir: %v", dirErr)
		}
		t.Cleanup(func() { _ = os.RemoveAll(dir) })

		// First write
		sink1, err1 := NewFileSink(dir)
		if err1 != nil {
			t.Fatalf("failed to create first sink: %v", err1)
		}
		if err := sink1.WriteOne(AgentEvent{Type: EventText, Content: "First"}); err != nil {
			t.Fatalf("failed to write first event: %v", err)
		}
		if err := sink1.Close(); err != nil {
			t.Fatalf("failed to close first sink: %v", err)
		}

		// Second write (append)
		sink2, err2 := NewFileSink(dir)
		if err2 != nil {
			t.Fatalf("failed to create second sink: %v", err2)
		}
		if err := sink2.WriteOne(AgentEvent{Type: EventText, Content: "Second"}); err != nil {
			t.Fatalf("failed to write second event: %v", err)
		}
		if err := sink2.Close(); err != nil {
			t.Fatalf("failed to close second sink: %v", err)
		}

		// Verify both events are present
		readEvents, readErr := ReadEvents(filepath.Join(dir, DefaultFilename))
		if readErr != nil {
			t.Fatalf("failed to read events: %v", readErr)
		}
		if len(readEvents) != 2 {
			t.Errorf("expected 2 events after append, got %d", len(readEvents))
		}
	})

	t.Run("write empty slice", func(t *testing.T) {
		dir, dirErr := os.MkdirTemp("", "events-empty-*")
		if dirErr != nil {
			t.Fatalf("failed to create temp dir: %v", dirErr)
		}
		t.Cleanup(func() { _ = os.RemoveAll(dir) })

		sink, sinkErr := NewFileSink(dir)
		if sinkErr != nil {
			t.Fatalf("failed to create sink: %v", sinkErr)
		}
		t.Cleanup(func() { _ = sink.Close() })

		// Writing empty slice should not error
		if err := sink.Write([]AgentEvent{}); err != nil {
			t.Errorf("Write([]) returned error: %v", err)
		}
	})

	t.Run("double close", func(t *testing.T) {
		dir, dirErr := os.MkdirTemp("", "events-double-*")
		if dirErr != nil {
			t.Fatalf("failed to create temp dir: %v", dirErr)
		}
		t.Cleanup(func() { _ = os.RemoveAll(dir) })

		sink, sinkErr := NewFileSink(dir)
		if sinkErr != nil {
			t.Fatalf("failed to create sink: %v", sinkErr)
		}
		if err := sink.Close(); err != nil {
			t.Fatalf("first Close() returned error: %v", err)
		}

		// Second close should not error
		if err := sink.Close(); err != nil {
			t.Errorf("second Close() returned error: %v", err)
		}
	})
}

func TestFilterByType(t *testing.T) {
	events := []AgentEvent{
		{Type: EventText, Content: "text1"},
		{Type: EventThinking, Content: "thinking1"},
		{Type: EventToolUse, Content: "tool1"},
		{Type: EventText, Content: "text2"},
		{Type: EventError, Content: "error1"},
	}

	t.Run("filter single type", func(t *testing.T) {
		result := FilterByType(events, EventText)
		if len(result) != 2 {
			t.Errorf("expected 2 text events, got %d", len(result))
		}
	})

	t.Run("filter multiple types", func(t *testing.T) {
		result := FilterByType(events, EventText, EventThinking)
		if len(result) != 3 {
			t.Errorf("expected 3 events, got %d", len(result))
		}
	})

	t.Run("filter no types returns all", func(t *testing.T) {
		result := FilterByType(events)
		if len(result) != len(events) {
			t.Errorf("expected %d events, got %d", len(events), len(result))
		}
	})

	t.Run("filter non-existent type", func(t *testing.T) {
		result := FilterByType(events, EventFileChange)
		if len(result) != 0 {
			t.Errorf("expected 0 events, got %d", len(result))
		}
	})
}

func TestFilterByIteration(t *testing.T) {
	testEvents := []AgentEvent{
		{Iteration: 1, Content: "iter1-a"},
		{Iteration: 1, Content: "iter1-b"},
		{Iteration: 2, Content: "iter2-a"},
		{Iteration: 3, Content: "iter3-a"},
	}

	t.Run("filter by iteration 1", func(t *testing.T) {
		result := FilterByIteration(testEvents, 1)
		if len(result) != 2 {
			t.Errorf("expected 2 events for iteration 1, got %d", len(result))
		}
	})

	t.Run("filter by iteration 2", func(t *testing.T) {
		result := FilterByIteration(testEvents, 2)
		if len(result) != 1 {
			t.Errorf("expected 1 event for iteration 2, got %d", len(result))
		}
	})

	t.Run("filter by non-existent iteration", func(t *testing.T) {
		result := FilterByIteration(testEvents, 99)
		if len(result) != 0 {
			t.Errorf("expected 0 events for iteration 99, got %d", len(result))
		}
	})

	t.Run("iteration 0 returns all events", func(t *testing.T) {
		result := FilterByIteration(testEvents, 0)
		if len(result) != len(testEvents) {
			t.Errorf("expected %d events for iteration 0, got %d", len(testEvents), len(result))
		}
	})

	t.Run("negative iteration returns all events", func(t *testing.T) {
		result := FilterByIteration(testEvents, -1)
		if len(result) != len(testEvents) {
			t.Errorf("expected %d events for iteration -1, got %d", len(testEvents), len(result))
		}
	})
}

func TestReadEvents_InvalidFile(t *testing.T) {
	t.Run("non-existent file", func(t *testing.T) {
		_, err := ReadEvents("/non/existent/file.jsonl")
		if err == nil {
			t.Error("expected error for non-existent file")
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		tmpFile, createErr := os.CreateTemp("", "invalid-*.jsonl")
		if createErr != nil {
			t.Fatalf("failed to create temp file: %v", createErr)
		}
		if _, writeErr := tmpFile.WriteString("not valid json\n"); writeErr != nil {
			t.Fatalf("failed to write to temp file: %v", writeErr)
		}
		if closeErr := tmpFile.Close(); closeErr != nil {
			t.Fatalf("failed to close temp file: %v", closeErr)
		}
		t.Cleanup(func() { _ = os.Remove(tmpFile.Name()) })

		_, err := ReadEvents(tmpFile.Name())
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})
}
