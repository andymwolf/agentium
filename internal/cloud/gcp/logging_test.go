package gcp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestNewCloudLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := NewCloudLogger(
		WithWriter(&buf),
		WithSessionID("test-session-123"),
		WithIteration(5),
		WithLabels(map[string]string{"env": "test"}),
	)

	if logger == nil {
		t.Fatal("NewCloudLogger() returned nil")
	}
	if logger.sessionID != "test-session-123" {
		t.Errorf("sessionID = %q, want %q", logger.sessionID, "test-session-123")
	}
	if logger.iteration != 5 {
		t.Errorf("iteration = %d, want %d", logger.iteration, 5)
	}
	if logger.labels["env"] != "test" {
		t.Errorf("labels[env] = %q, want %q", logger.labels["env"], "test")
	}
}

func TestCloudLogger_Info(t *testing.T) {
	var buf bytes.Buffer
	logger := NewCloudLogger(
		WithWriter(&buf),
		WithSessionID("sess-1"),
		WithIteration(3),
	)

	logger.Info("Starting iteration")

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse log entry: %v", err)
	}

	if entry.Severity != SeverityInfo {
		t.Errorf("Severity = %q, want %q", entry.Severity, SeverityInfo)
	}
	if entry.Message != "Starting iteration" {
		t.Errorf("Message = %q, want %q", entry.Message, "Starting iteration")
	}
	if entry.SessionID != "sess-1" {
		t.Errorf("SessionID = %q, want %q", entry.SessionID, "sess-1")
	}
	if entry.Iteration != 3 {
		t.Errorf("Iteration = %d, want %d", entry.Iteration, 3)
	}
	if entry.Timestamp == "" {
		t.Error("Timestamp is empty")
	}
}

func TestCloudLogger_Infof(t *testing.T) {
	var buf bytes.Buffer
	logger := NewCloudLogger(WithWriter(&buf), WithSessionID("sess-1"))

	logger.Infof("Task %d of %d", 2, 5)

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse log entry: %v", err)
	}

	if entry.Message != "Task 2 of 5" {
		t.Errorf("Message = %q, want %q", entry.Message, "Task 2 of 5")
	}
}

func TestCloudLogger_Warning(t *testing.T) {
	var buf bytes.Buffer
	logger := NewCloudLogger(WithWriter(&buf))

	logger.Warning("Something might be wrong")

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse log entry: %v", err)
	}

	if entry.Severity != SeverityWarning {
		t.Errorf("Severity = %q, want %q", entry.Severity, SeverityWarning)
	}
}

func TestCloudLogger_Warningf(t *testing.T) {
	var buf bytes.Buffer
	logger := NewCloudLogger(WithWriter(&buf))

	logger.Warningf("Retrying after %d failures", 3)

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse log entry: %v", err)
	}

	if entry.Severity != SeverityWarning {
		t.Errorf("Severity = %q, want %q", entry.Severity, SeverityWarning)
	}
	if entry.Message != "Retrying after 3 failures" {
		t.Errorf("Message = %q, want %q", entry.Message, "Retrying after 3 failures")
	}
}

func TestCloudLogger_Error(t *testing.T) {
	var buf bytes.Buffer
	logger := NewCloudLogger(WithWriter(&buf))

	logger.Error("Something went wrong")

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse log entry: %v", err)
	}

	if entry.Severity != SeverityError {
		t.Errorf("Severity = %q, want %q", entry.Severity, SeverityError)
	}
}

func TestCloudLogger_Errorf(t *testing.T) {
	var buf bytes.Buffer
	logger := NewCloudLogger(WithWriter(&buf))

	logger.Errorf("Failed to connect: %s", "timeout")

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse log entry: %v", err)
	}

	if entry.Severity != SeverityError {
		t.Errorf("Severity = %q, want %q", entry.Severity, SeverityError)
	}
	if entry.Message != "Failed to connect: timeout" {
		t.Errorf("Message = %q, want %q", entry.Message, "Failed to connect: timeout")
	}
}

func TestCloudLogger_Write(t *testing.T) {
	var buf bytes.Buffer
	logger := NewCloudLogger(
		WithWriter(&buf),
		WithSessionID("sess-2"),
		WithIteration(1),
	)

	// Simulate log.Logger writing to the cloud logger
	msg := "[controller] Starting session sess-2\n"
	n, err := logger.Write([]byte(msg))

	if err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	if n != len(msg) {
		t.Errorf("Write() returned %d, want %d", n, len(msg))
	}

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse log entry: %v", err)
	}

	// Should strip the [controller] prefix
	if entry.Message != "Starting session sess-2" {
		t.Errorf("Message = %q, want %q", entry.Message, "Starting session sess-2")
	}
	if entry.Severity != SeverityInfo {
		t.Errorf("Severity = %q, want %q", entry.Severity, SeverityInfo)
	}
}

func TestCloudLogger_Write_DetectsSeverity(t *testing.T) {
	tests := []struct {
		name         string
		message      string
		wantSeverity string
	}{
		{
			name:         "info message",
			message:      "Starting iteration 5",
			wantSeverity: SeverityInfo,
		},
		{
			name:         "warning message",
			message:      "Warning: failed to connect",
			wantSeverity: SeverityWarning,
		},
		{
			name:         "error message prefix",
			message:      "Error: something broke",
			wantSeverity: SeverityError,
		},
		{
			name:         "failed keyword",
			message:      "Operation failed: timeout",
			wantSeverity: SeverityError,
		},
		{
			name:         "warn prefix",
			message:      "Warn: possible issue",
			wantSeverity: SeverityWarning,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := NewCloudLogger(WithWriter(&buf))

			logger.Write([]byte(tt.message))

			var entry LogEntry
			if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
				t.Fatalf("Failed to parse log entry: %v", err)
			}

			if entry.Severity != tt.wantSeverity {
				t.Errorf("Severity = %q, want %q for message %q", entry.Severity, tt.wantSeverity, tt.message)
			}
		})
	}
}

func TestCloudLogger_SetIteration(t *testing.T) {
	var buf bytes.Buffer
	logger := NewCloudLogger(WithWriter(&buf), WithIteration(1))

	logger.SetIteration(7)
	logger.Info("After update")

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse log entry: %v", err)
	}

	if entry.Iteration != 7 {
		t.Errorf("Iteration = %d, want %d", entry.Iteration, 7)
	}
}

func TestCloudLogger_Labels(t *testing.T) {
	var buf bytes.Buffer
	logger := NewCloudLogger(
		WithWriter(&buf),
		WithLabels(map[string]string{
			"agent":      "claude-code",
			"repository": "github.com/org/repo",
		}),
	)

	logger.Info("Test with labels")

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse log entry: %v", err)
	}

	if entry.Labels["agent"] != "claude-code" {
		t.Errorf("Labels[agent] = %q, want %q", entry.Labels["agent"], "claude-code")
	}
	if entry.Labels["repository"] != "github.com/org/repo" {
		t.Errorf("Labels[repository] = %q, want %q", entry.Labels["repository"], "github.com/org/repo")
	}
}

func TestCloudLogger_NilWriter(t *testing.T) {
	// Should not panic with nil writer
	logger := NewCloudLogger(WithSessionID("test"))
	logger.Info("Should not panic")
	logger.Write([]byte("Also should not panic"))
}

func TestCloudLogger_MultipleEntries(t *testing.T) {
	var buf bytes.Buffer
	logger := NewCloudLogger(WithWriter(&buf), WithSessionID("multi-test"))

	logger.Info("First entry")
	logger.Warning("Second entry")
	logger.Error("Third entry")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("Expected 3 log lines, got %d", len(lines))
	}

	var entry1, entry2, entry3 LogEntry
	json.Unmarshal([]byte(lines[0]), &entry1)
	json.Unmarshal([]byte(lines[1]), &entry2)
	json.Unmarshal([]byte(lines[2]), &entry3)

	if entry1.Severity != SeverityInfo {
		t.Errorf("Entry 1 severity = %q, want %q", entry1.Severity, SeverityInfo)
	}
	if entry2.Severity != SeverityWarning {
		t.Errorf("Entry 2 severity = %q, want %q", entry2.Severity, SeverityWarning)
	}
	if entry3.Severity != SeverityError {
		t.Errorf("Entry 3 severity = %q, want %q", entry3.Severity, SeverityError)
	}
}

func TestCloudLogger_TimestampFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := NewCloudLogger(WithWriter(&buf))

	logger.Info("Check timestamp")

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse log entry: %v", err)
	}

	// Verify timestamp is in RFC3339Nano format
	if !strings.Contains(entry.Timestamp, "T") || !strings.HasSuffix(entry.Timestamp, "Z") {
		t.Errorf("Timestamp %q does not appear to be RFC3339 format", entry.Timestamp)
	}
}

func TestCloudLogger_FlushAndClose(t *testing.T) {
	var buf bytes.Buffer
	logger := NewCloudLogger(WithWriter(&buf))

	if err := logger.Flush(); err != nil {
		t.Errorf("Flush() returned error: %v", err)
	}

	if err := logger.Close(); err != nil {
		t.Errorf("Close() returned error: %v", err)
	}
}

func TestFormatEntry(t *testing.T) {
	entry := LogEntry{
		Severity:  SeverityInfo,
		Message:   "Test message",
		Timestamp: "2024-01-15T10:30:00.000000000Z",
		SessionID: "sess-123",
		Iteration: 5,
		Labels: map[string]string{
			"agent": "claude-code",
		},
	}

	result, err := FormatEntry(entry)
	if err != nil {
		t.Fatalf("FormatEntry() error: %v", err)
	}

	// Verify it's valid JSON
	var parsed LogEntry
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("FormatEntry() result is not valid JSON: %v", err)
	}

	if parsed.Severity != SeverityInfo {
		t.Errorf("parsed.Severity = %q, want %q", parsed.Severity, SeverityInfo)
	}
	if parsed.Message != "Test message" {
		t.Errorf("parsed.Message = %q, want %q", parsed.Message, "Test message")
	}
	if parsed.SessionID != "sess-123" {
		t.Errorf("parsed.SessionID = %q, want %q", parsed.SessionID, "sess-123")
	}
	if parsed.Iteration != 5 {
		t.Errorf("parsed.Iteration = %d, want %d", parsed.Iteration, 5)
	}
}

func TestFormatEntry_StructuredFields(t *testing.T) {
	entry := LogEntry{
		Severity:  SeverityWarning,
		Message:   "Task blocked",
		Timestamp: "2024-01-15T10:30:00.000000000Z",
		SessionID: "sess-456",
		Iteration: 10,
	}

	result, err := FormatEntry(entry)
	if err != nil {
		t.Fatalf("FormatEntry() error: %v", err)
	}

	// Verify specific JSON fields
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(result), &raw); err != nil {
		t.Fatalf("Failed to parse as map: %v", err)
	}

	if raw["severity"] != "WARNING" {
		t.Errorf("severity = %v, want %q", raw["severity"], "WARNING")
	}
	if raw["sessionId"] != "sess-456" {
		t.Errorf("sessionId = %v, want %q", raw["sessionId"], "sess-456")
	}
	if raw["iteration"] != float64(10) {
		t.Errorf("iteration = %v, want %v", raw["iteration"], 10)
	}
}

func TestDetectSeverity(t *testing.T) {
	logger := NewCloudLogger()

	tests := []struct {
		message  string
		expected string
	}{
		{"Normal info message", SeverityInfo},
		{"Error: something wrong", SeverityError},
		{"error at position 5", SeverityError},
		{"Warning: disk space low", SeverityWarning},
		{"warning about memory", SeverityWarning},
		{"warn: deprecated function", SeverityWarning},
		{"Operation failed: timeout", SeverityError},
		{"Task completed successfully", SeverityInfo},
		{"Failed to connect", SeverityError},
	}

	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			got := logger.detectSeverity(tt.message)
			if got != tt.expected {
				t.Errorf("detectSeverity(%q) = %q, want %q", tt.message, got, tt.expected)
			}
		})
	}
}

func TestCloudLogger_ConcurrentWrites(t *testing.T) {
	var buf bytes.Buffer
	logger := NewCloudLogger(WithWriter(&buf), WithSessionID("concurrent-test"))

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(n int) {
			logger.Infof("Message %d", n)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 10 {
		t.Errorf("Expected 10 log lines, got %d", len(lines))
	}

	// Verify each line is valid JSON
	for i, line := range lines {
		var entry LogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Errorf("Line %d is not valid JSON: %v", i, err)
		}
	}
}

func TestLoggingAPIClient_WriteEntry(t *testing.T) {
	// Test that writeEntry doesn't panic with nil client
	var nilClient *loggingAPIClient
	nilClient.writeEntry(nil, LogEntry{Message: "test"})

	// Test with a client (will fail to connect but shouldn't panic)
	client := &loggingAPIClient{
		projectID: "test-project",
		logName:   "agentium",
		client:    &http.Client{Timeout: 1 * time.Millisecond},
	}

	// This will fail to connect but should not panic
	entry := LogEntry{
		Severity:  SeverityInfo,
		Message:   "test entry",
		SessionID: "test-session",
		Iteration: 1,
		Timestamp: "2024-01-15T10:30:00Z",
	}
	client.writeEntry(nil, entry)
}

func TestCloudLogger_Write_StripsPrefix(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantMsg string
	}{
		{
			name:    "controller prefix",
			input:   "[controller] Starting session",
			wantMsg: "Starting session",
		},
		{
			name:    "custom prefix",
			input:   "[myapp] Doing something",
			wantMsg: "Doing something",
		},
		{
			name:    "no prefix",
			input:   "Plain message",
			wantMsg: "Plain message",
		},
		{
			name:    "bracket in middle not stripped",
			input:   "Message with [bracket] in it",
			wantMsg: "Message with [bracket] in it",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := NewCloudLogger(WithWriter(&buf))

			logger.Write([]byte(tt.input))

			var entry LogEntry
			if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
				t.Fatalf("Failed to parse: %v", err)
			}

			if entry.Message != tt.wantMsg {
				t.Errorf("Message = %q, want %q", entry.Message, tt.wantMsg)
			}
		})
	}
}
