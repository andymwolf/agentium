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
	defer os.RemoveAll(tmpDir)

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
		events := []AgentEvent{
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

		if err := sink.Write(events); err != nil {
			t.Fatalf("failed to write events: %v", err)
		}

		// Close sink
		if err := sink.Close(); err != nil {
			t.Fatalf("failed to close sink: %v", err)
		}

		// Read back events
		readEvents, err := ReadEvents(sink.Path())
		if err != nil {
			t.Fatalf("failed to read events: %v", err)
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
		dir, err := os.MkdirTemp("", "events-append-*")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(dir)

		// First write
		sink1, _ := NewFileSink(dir)
		sink1.WriteOne(AgentEvent{Type: EventText, Content: "First"})
		sink1.Close()

		// Second write (append)
		sink2, _ := NewFileSink(dir)
		sink2.WriteOne(AgentEvent{Type: EventText, Content: "Second"})
		sink2.Close()

		// Verify both events are present
		events, _ := ReadEvents(filepath.Join(dir, DefaultFilename))
		if len(events) != 2 {
			t.Errorf("expected 2 events after append, got %d", len(events))
		}
	})

	t.Run("write empty slice", func(t *testing.T) {
		dir, _ := os.MkdirTemp("", "events-empty-*")
		defer os.RemoveAll(dir)

		sink, _ := NewFileSink(dir)
		defer sink.Close()

		// Writing empty slice should not error
		if err := sink.Write([]AgentEvent{}); err != nil {
			t.Errorf("Write([]) returned error: %v", err)
		}
	})

	t.Run("double close", func(t *testing.T) {
		dir, _ := os.MkdirTemp("", "events-double-*")
		defer os.RemoveAll(dir)

		sink, _ := NewFileSink(dir)
		sink.Close()

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
	events := []AgentEvent{
		{Iteration: 1, Content: "iter1-a"},
		{Iteration: 1, Content: "iter1-b"},
		{Iteration: 2, Content: "iter2-a"},
		{Iteration: 3, Content: "iter3-a"},
	}

	t.Run("filter by iteration 1", func(t *testing.T) {
		result := FilterByIteration(events, 1)
		if len(result) != 2 {
			t.Errorf("expected 2 events for iteration 1, got %d", len(result))
		}
	})

	t.Run("filter by iteration 2", func(t *testing.T) {
		result := FilterByIteration(events, 2)
		if len(result) != 1 {
			t.Errorf("expected 1 event for iteration 2, got %d", len(result))
		}
	})

	t.Run("filter by non-existent iteration", func(t *testing.T) {
		result := FilterByIteration(events, 99)
		if len(result) != 0 {
			t.Errorf("expected 0 events for iteration 99, got %d", len(result))
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
		tmpFile, _ := os.CreateTemp("", "invalid-*.jsonl")
		tmpFile.WriteString("not valid json\n")
		tmpFile.Close()
		defer os.Remove(tmpFile.Name())

		_, err := ReadEvents(tmpFile.Name())
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})
}
