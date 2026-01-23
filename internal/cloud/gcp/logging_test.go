package gcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestFallbackLogger_Log(t *testing.T) {
	var buf bytes.Buffer
	logger := NewFallbackLogger(&buf, "test-session-123")

	logger.Log(SeverityInfo, "test message", map[string]interface{}{
		"key": "value",
	})

	output := buf.String()
	if output == "" {
		t.Fatal("expected output, got empty string")
	}

	var entry LogEntry
	if err := json.Unmarshal([]byte(output), &entry); err != nil {
		t.Fatalf("failed to unmarshal log entry: %v", err)
	}

	if entry.Severity != SeverityInfo {
		t.Errorf("severity = %q, want %q", entry.Severity, SeverityInfo)
	}
	if entry.Message != "test message" {
		t.Errorf("message = %q, want %q", entry.Message, "test message")
	}
	if entry.SessionID != "test-session-123" {
		t.Errorf("session_id = %q, want %q", entry.SessionID, "test-session-123")
	}
	if entry.Fields["key"] != "value" {
		t.Errorf("fields[key] = %v, want %q", entry.Fields["key"], "value")
	}
}

func TestFallbackLogger_LogInfo(t *testing.T) {
	var buf bytes.Buffer
	logger := NewFallbackLogger(&buf, "test-session")

	logger.LogInfo("info message")

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if entry.Severity != SeverityInfo {
		t.Errorf("severity = %q, want %q", entry.Severity, SeverityInfo)
	}
	if entry.Message != "info message" {
		t.Errorf("message = %q, want %q", entry.Message, "info message")
	}
}

func TestFallbackLogger_LogWarning(t *testing.T) {
	var buf bytes.Buffer
	logger := NewFallbackLogger(&buf, "test-session")

	logger.LogWarning("warning message")

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if entry.Severity != SeverityWarning {
		t.Errorf("severity = %q, want %q", entry.Severity, SeverityWarning)
	}
}

func TestFallbackLogger_LogError(t *testing.T) {
	var buf bytes.Buffer
	logger := NewFallbackLogger(&buf, "test-session")

	logger.LogError("error message")

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if entry.Severity != SeverityError {
		t.Errorf("severity = %q, want %q", entry.Severity, SeverityError)
	}
}

func TestFallbackLogger_SetIteration(t *testing.T) {
	var buf bytes.Buffer
	logger := NewFallbackLogger(&buf, "test-session")

	logger.SetIteration(5)
	logger.LogInfo("iteration test")

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if entry.Iteration != 5 {
		t.Errorf("iteration = %d, want 5", entry.Iteration)
	}
}

func TestFallbackLogger_Flush(t *testing.T) {
	var buf bytes.Buffer
	logger := NewFallbackLogger(&buf, "test-session")

	// Flush should be a no-op and return nil
	if err := logger.Flush(); err != nil {
		t.Errorf("Flush() unexpected error: %v", err)
	}
}

func TestFallbackLogger_Close(t *testing.T) {
	var buf bytes.Buffer
	logger := NewFallbackLogger(&buf, "test-session")

	// Close should be a no-op and return nil
	if err := logger.Close(); err != nil {
		t.Errorf("Close() unexpected error: %v", err)
	}
}

func TestFallbackLogger_Labels(t *testing.T) {
	var buf bytes.Buffer
	logger := NewFallbackLogger(&buf, "test-session")

	logger.LogInfo("label test")

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if entry.Labels["session_id"] != "test-session" {
		t.Errorf("labels[session_id] = %q, want %q", entry.Labels["session_id"], "test-session")
	}
	if entry.Labels["component"] != "agentium-controller" {
		t.Errorf("labels[component] = %q, want %q", entry.Labels["component"], "agentium-controller")
	}
}

func TestFallbackLogger_Timestamp(t *testing.T) {
	var buf bytes.Buffer
	logger := NewFallbackLogger(&buf, "test-session")

	before := time.Now().UTC()
	logger.LogInfo("timestamp test")
	after := time.Now().UTC()

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if entry.Timestamp.Before(before) || entry.Timestamp.After(after) {
		t.Errorf("timestamp %v not between %v and %v", entry.Timestamp, before, after)
	}
}

func TestFallbackLogger_MultipleEntries(t *testing.T) {
	var buf bytes.Buffer
	logger := NewFallbackLogger(&buf, "test-session")

	logger.LogInfo("first")
	logger.LogWarning("second")
	logger.LogError("third")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}

	severities := []Severity{SeverityInfo, SeverityWarning, SeverityError}
	messages := []string{"first", "second", "third"}

	for i, line := range lines {
		var entry LogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("line %d: failed to unmarshal: %v", i, err)
		}
		if entry.Severity != severities[i] {
			t.Errorf("line %d: severity = %q, want %q", i, entry.Severity, severities[i])
		}
		if entry.Message != messages[i] {
			t.Errorf("line %d: message = %q, want %q", i, entry.Message, messages[i])
		}
	}
}

func TestFormatLogEntry(t *testing.T) {
	entry := LogEntry{
		Severity:  SeverityInfo,
		Message:   "test format",
		Timestamp: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		SessionID: "session-abc",
		Iteration: 3,
		Labels: map[string]string{
			"key": "value",
		},
	}

	result := FormatLogEntry(entry)

	// Verify it's valid JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("FormatLogEntry produced invalid JSON: %v", err)
	}

	if parsed["severity"] != "INFO" {
		t.Errorf("severity = %v, want INFO", parsed["severity"])
	}
	if parsed["message"] != "test format" {
		t.Errorf("message = %v, want 'test format'", parsed["message"])
	}
	if parsed["session_id"] != "session-abc" {
		t.Errorf("session_id = %v, want 'session-abc'", parsed["session_id"])
	}
}

func TestSanitizeForLog(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "GitHub token ghs_",
			input: "ghs_abc123def456",
			want:  "[REDACTED_GITHUB_TOKEN]",
		},
		{
			name:  "GitHub token ghp_",
			input: "ghp_personaltoken123",
			want:  "[REDACTED_GITHUB_TOKEN]",
		},
		{
			name:  "GitHub token gho_",
			input: "gho_oauthtoken123",
			want:  "[REDACTED_GITHUB_TOKEN]",
		},
		{
			name:  "Bearer token",
			input: "Bearer eyJhbGciOiJSUzI1NiI",
			want:  "Bearer [REDACTED]",
		},
		{
			name:  "Normal string",
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "Empty string",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeForLog(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeForLog(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestLoggerInterface(t *testing.T) {
	// Verify that CloudLogger implements LoggerInterface
	var _ LoggerInterface = (*CloudLogger)(nil)

	// Verify that FallbackLogger implements LoggerInterface
	var _ LoggerInterface = (*FallbackLogger)(nil)
}

func TestCloudLogger_Log(t *testing.T) {
	var buf bytes.Buffer
	logger := NewCloudLogger("test-session", WithWriter(&buf))

	logger.Log(SeverityInfo, "cloud log test", map[string]interface{}{
		"action": "test",
	})

	output := buf.String()
	if output == "" {
		t.Fatal("expected output, got empty string")
	}

	var entry LogEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &entry); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if entry.Severity != SeverityInfo {
		t.Errorf("severity = %q, want %q", entry.Severity, SeverityInfo)
	}
	if entry.Message != "cloud log test" {
		t.Errorf("message = %q, want %q", entry.Message, "cloud log test")
	}
	if entry.SessionID != "test-session" {
		t.Errorf("session_id = %q, want %q", entry.SessionID, "test-session")
	}
	if entry.Fields["action"] != "test" {
		t.Errorf("fields[action] = %v, want %q", entry.Fields["action"], "test")
	}
}

func TestCloudLogger_LogInfo(t *testing.T) {
	var buf bytes.Buffer
	logger := NewCloudLogger("test-session", WithWriter(&buf))

	logger.LogInfo("info message")

	var entry LogEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &entry); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if entry.Severity != SeverityInfo {
		t.Errorf("severity = %q, want %q", entry.Severity, SeverityInfo)
	}
}

func TestCloudLogger_LogWarning(t *testing.T) {
	var buf bytes.Buffer
	logger := NewCloudLogger("test-session", WithWriter(&buf))

	logger.LogWarning("warning message")

	var entry LogEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &entry); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if entry.Severity != SeverityWarning {
		t.Errorf("severity = %q, want %q", entry.Severity, SeverityWarning)
	}
}

func TestCloudLogger_LogError(t *testing.T) {
	var buf bytes.Buffer
	logger := NewCloudLogger("test-session", WithWriter(&buf))

	logger.LogError("error message")

	var entry LogEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &entry); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if entry.Severity != SeverityError {
		t.Errorf("severity = %q, want %q", entry.Severity, SeverityError)
	}
}

func TestCloudLogger_SetIteration(t *testing.T) {
	var buf bytes.Buffer
	logger := NewCloudLogger("test-session", WithWriter(&buf))

	logger.SetIteration(7)
	logger.LogInfo("iteration test")

	var entry LogEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &entry); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if entry.Iteration != 7 {
		t.Errorf("iteration = %d, want 7", entry.Iteration)
	}
}

func TestCloudLogger_LogAfterClose(t *testing.T) {
	var buf bytes.Buffer
	logger := NewCloudLogger("test-session", WithWriter(&buf))

	logger.Close()
	logger.Log(SeverityInfo, "after close", nil)

	// Should produce no output after close
	if buf.String() != "" {
		t.Errorf("expected no output after close, got %q", buf.String())
	}
}

func TestCloudLogger_FlushAfterClose(t *testing.T) {
	var buf bytes.Buffer
	logger := NewCloudLogger("test-session", WithWriter(&buf))

	logger.Close()
	err := logger.Flush()
	if err != nil {
		t.Errorf("Flush() after close should return nil, got: %v", err)
	}
}

func TestCloudLogger_CloseIdempotent(t *testing.T) {
	var buf bytes.Buffer
	logger := NewCloudLogger("test-session", WithWriter(&buf))

	// Close twice - should not error
	if err := logger.Close(); err != nil {
		t.Errorf("first Close() error: %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Errorf("second Close() error: %v", err)
	}
}

func TestCloudLogger_FlushWithCustomFlushFn(t *testing.T) {
	flushed := false
	var buf bytes.Buffer
	logger := NewCloudLogger("test-session",
		WithWriter(&buf),
		WithFlushFunc(func() error {
			flushed = true
			return nil
		}),
	)

	logger.LogInfo("before flush")
	if err := logger.Flush(); err != nil {
		t.Errorf("Flush() error: %v", err)
	}

	if !flushed {
		t.Error("custom flush function was not called")
	}
}

func TestCloudLogger_FlushFnError(t *testing.T) {
	var buf bytes.Buffer
	expectedErr := errors.New("flush failed")
	logger := NewCloudLogger("test-session",
		WithWriter(&buf),
		WithFlushFunc(func() error {
			return expectedErr
		}),
	)

	err := logger.Flush()
	if err != expectedErr {
		t.Errorf("Flush() error = %v, want %v", err, expectedErr)
	}
}

func TestWithLabels(t *testing.T) {
	var buf bytes.Buffer
	logger := NewCloudLogger("test-session",
		WithWriter(&buf),
		WithLabels(map[string]string{
			"env":    "production",
			"region": "us-east1",
		}),
	)

	logger.LogInfo("labels test")

	var entry LogEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &entry); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if entry.Labels["env"] != "production" {
		t.Errorf("labels[env] = %q, want %q", entry.Labels["env"], "production")
	}
	if entry.Labels["region"] != "us-east1" {
		t.Errorf("labels[region] = %q, want %q", entry.Labels["region"], "us-east1")
	}
	// Default labels should still be present
	if entry.Labels["session_id"] != "test-session" {
		t.Errorf("labels[session_id] = %q, want %q", entry.Labels["session_id"], "test-session")
	}
}

func TestWithIteration(t *testing.T) {
	var buf bytes.Buffer
	logger := NewCloudLogger("test-session",
		WithWriter(&buf),
		WithIteration(42),
	)

	logger.LogInfo("iteration test")

	var entry LogEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &entry); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if entry.Iteration != 42 {
		t.Errorf("iteration = %d, want 42", entry.Iteration)
	}
}

func TestCloudLogger_ConcurrentWrites(t *testing.T) {
	var buf bytes.Buffer
	logger := NewCloudLogger("concurrent-test", WithWriter(&buf))

	// Write from multiple goroutines
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(n int) {
			logger.LogInfo("concurrent message")
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all entries were written
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 10 {
		t.Errorf("expected 10 lines, got %d", len(lines))
	}

	// Verify each line is valid JSON
	for i, line := range lines {
		var entry LogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Errorf("line %d: invalid JSON: %v", i, err)
		}
	}
}

func TestLogEntry_JSONRoundTrip(t *testing.T) {
	original := LogEntry{
		Severity:  SeverityWarning,
		Message:   "test roundtrip",
		Timestamp: time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC),
		SessionID: "roundtrip-session",
		Iteration: 10,
		Labels: map[string]string{
			"env": "test",
		},
		Fields: map[string]interface{}{
			"count": float64(42),
			"name":  "test",
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded LogEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Severity != original.Severity {
		t.Errorf("severity = %q, want %q", decoded.Severity, original.Severity)
	}
	if decoded.Message != original.Message {
		t.Errorf("message = %q, want %q", decoded.Message, original.Message)
	}
	if decoded.SessionID != original.SessionID {
		t.Errorf("session_id = %q, want %q", decoded.SessionID, original.SessionID)
	}
	if decoded.Iteration != original.Iteration {
		t.Errorf("iteration = %d, want %d", decoded.Iteration, original.Iteration)
	}
	if !decoded.Timestamp.Equal(original.Timestamp) {
		t.Errorf("timestamp = %v, want %v", decoded.Timestamp, original.Timestamp)
	}
}

func TestNewFallbackLogger_Defaults(t *testing.T) {
	var buf bytes.Buffer
	logger := NewFallbackLogger(&buf, "default-test")

	if logger.sessionID != "default-test" {
		t.Errorf("sessionID = %q, want %q", logger.sessionID, "default-test")
	}
	if logger.iteration != 0 {
		t.Errorf("iteration = %d, want 0", logger.iteration)
	}
	if logger.labels["session_id"] != "default-test" {
		t.Errorf("labels[session_id] = %q, want %q", logger.labels["session_id"], "default-test")
	}
	if logger.labels["component"] != "agentium-controller" {
		t.Errorf("labels[component] = %q, want %q", logger.labels["component"], "agentium-controller")
	}
}

func TestFallbackLogger_NilFields(t *testing.T) {
	var buf bytes.Buffer
	logger := NewFallbackLogger(&buf, "test-session")

	// Log with nil fields
	logger.Log(SeverityInfo, "nil fields", nil)

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if entry.Message != "nil fields" {
		t.Errorf("message = %q, want %q", entry.Message, "nil fields")
	}
}

func TestNewCloudLogger_DefaultWriter(t *testing.T) {
	logger := NewCloudLogger("test-session")

	// Default writer should be os.Stderr
	if logger.writer == nil {
		t.Error("writer should not be nil")
	}
	if logger.sessionID != "test-session" {
		t.Errorf("sessionID = %q, want %q", logger.sessionID, "test-session")
	}
	if logger.labels["session_id"] != "test-session" {
		t.Errorf("labels[session_id] = %q, want %q", logger.labels["session_id"], "test-session")
	}
	if logger.labels["component"] != "agentium-controller" {
		t.Errorf("labels[component] = %q, want %q", logger.labels["component"], "agentium-controller")
	}
}

func TestCloudLogger_FieldsWithMultipleTypes(t *testing.T) {
	var buf bytes.Buffer
	logger := NewCloudLogger("test-session", WithWriter(&buf))

	logger.Log(SeverityInfo, "typed fields", map[string]interface{}{
		"string_val": "hello",
		"int_val":    42,
		"float_val":  3.14,
		"bool_val":   true,
		"slice_val":  []string{"a", "b"},
	})

	var entry LogEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &entry); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if entry.Fields["string_val"] != "hello" {
		t.Errorf("fields[string_val] = %v, want %q", entry.Fields["string_val"], "hello")
	}
	// JSON numbers are decoded as float64
	if entry.Fields["int_val"] != float64(42) {
		t.Errorf("fields[int_val] = %v, want 42", entry.Fields["int_val"])
	}
	if entry.Fields["bool_val"] != true {
		t.Errorf("fields[bool_val] = %v, want true", entry.Fields["bool_val"])
	}
}

func TestFallbackLogger_ConcurrentWrites(t *testing.T) {
	var buf bytes.Buffer
	logger := NewFallbackLogger(&buf, "concurrent-test")

	// Write from multiple goroutines
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(n int) {
			logger.LogInfo("concurrent message")
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all entries were written
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 10 {
		t.Errorf("expected 10 lines, got %d", len(lines))
	}

	// Verify each line is valid JSON
	for i, line := range lines {
		var entry LogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Errorf("line %d: invalid JSON: %v", i, err)
		}
	}
}
