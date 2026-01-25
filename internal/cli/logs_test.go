package cli

import (
	"bytes"
	"io"
	"os"
	"testing"
	"time"

	"github.com/andywolf/agentium/internal/provisioner"
)

func captureOutput(fn func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

func TestFormatLogEntry_PlainMessage(t *testing.T) {
	entry := provisioner.LogEntry{
		Message: "hello world",
	}
	got := captureOutput(func() { formatLogEntry(entry) })
	if got != "hello world\n" {
		t.Errorf("got %q, want %q", got, "hello world\n")
	}
}

func TestFormatLogEntry_WithTimestamp(t *testing.T) {
	ts := time.Date(2024, 6, 15, 14, 30, 45, 0, time.UTC)
	entry := provisioner.LogEntry{
		Timestamp: ts,
		Message:   "msg",
	}
	got := captureOutput(func() { formatLogEntry(entry) })
	want := "[14:30:45] msg\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatLogEntry_ToolUseWithName(t *testing.T) {
	entry := provisioner.LogEntry{
		Message:   "ls -la",
		EventType: "tool_use",
		ToolName:  "Bash",
	}
	got := captureOutput(func() { formatLogEntry(entry) })
	want := "[TOOL:Bash] ls -la\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatLogEntry_ToolUseWithoutName(t *testing.T) {
	entry := provisioner.LogEntry{
		Message:   "some tool call",
		EventType: "tool_use",
	}
	got := captureOutput(func() { formatLogEntry(entry) })
	want := "[TOOL] some tool call\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatLogEntry_ToolResult(t *testing.T) {
	entry := provisioner.LogEntry{
		Message:   "file1.go",
		EventType: "tool_result",
	}
	got := captureOutput(func() { formatLogEntry(entry) })
	want := "[RESULT] file1.go\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatLogEntry_Thinking(t *testing.T) {
	entry := provisioner.LogEntry{
		Message:   "analyzing the problem",
		EventType: "thinking",
	}
	got := captureOutput(func() { formatLogEntry(entry) })
	want := "[THINKING] analyzing the problem\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatLogEntry_Text(t *testing.T) {
	entry := provisioner.LogEntry{
		Message:   "I will fix this bug",
		EventType: "text",
	}
	got := captureOutput(func() { formatLogEntry(entry) })
	want := "[AGENT] I will fix this bug\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatLogEntry_UnknownEventType(t *testing.T) {
	entry := provisioner.LogEntry{
		Message:   "something",
		EventType: "custom_event",
	}
	got := captureOutput(func() { formatLogEntry(entry) })
	want := "[custom_event] something\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatLogEntry_EventWithTimestamp(t *testing.T) {
	ts := time.Date(2024, 6, 15, 9, 5, 3, 0, time.UTC)
	entry := provisioner.LogEntry{
		Timestamp: ts,
		Message:   "reading file",
		EventType: "tool_use",
		ToolName:  "Read",
	}
	got := captureOutput(func() { formatLogEntry(entry) })
	want := "[09:05:03] [TOOL:Read] reading file\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
