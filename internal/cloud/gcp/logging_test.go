package gcp

import (
	"context"
	"sync"
	"testing"
	"time"
)

// mockCloudLogger implements CloudLogger for testing
type mockCloudLogger struct {
	mu      sync.Mutex
	entries []LogEntry
	flushed bool
	closed  bool
}

func newMockCloudLogger() *mockCloudLogger {
	return &mockCloudLogger{
		entries: make([]LogEntry, 0),
	}
}

func (m *mockCloudLogger) Log(entry LogEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, entry)
}

func (m *mockCloudLogger) Flush(timeout time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.flushed = true
	return nil
}

func (m *mockCloudLogger) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *mockCloudLogger) getEntries() []LogEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]LogEntry, len(m.entries))
	copy(result, m.entries)
	return result
}

func TestLogSeverity_String(t *testing.T) {
	tests := []struct {
		severity LogSeverity
		want     string
	}{
		{SeverityDefault, "DEFAULT"},
		{SeverityDebug, "DEBUG"},
		{SeverityInfo, "INFO"},
		{SeverityWarning, "WARNING"},
		{SeverityError, "ERROR"},
		{SeverityCritical, "CRITICAL"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.severity.String()
			if got != tt.want {
				t.Errorf("LogSeverity.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatLogEntry(t *testing.T) {
	sessionID := "test-session-123"
	iteration := 5
	severity := SeverityInfo
	message := "Test log message"
	labels := map[string]string{
		"task":  "issue-42",
		"phase": "IMPLEMENT",
	}

	before := time.Now()
	entry := FormatLogEntry(sessionID, iteration, severity, message, labels)
	after := time.Now()

	if entry.Message != message {
		t.Errorf("entry.Message = %q, want %q", entry.Message, message)
	}
	if entry.Severity != severity {
		t.Errorf("entry.Severity = %v, want %v", entry.Severity, severity)
	}
	if entry.SessionID != sessionID {
		t.Errorf("entry.SessionID = %q, want %q", entry.SessionID, sessionID)
	}
	if entry.Iteration != iteration {
		t.Errorf("entry.Iteration = %d, want %d", entry.Iteration, iteration)
	}
	if entry.Timestamp.Before(before) || entry.Timestamp.After(after) {
		t.Errorf("entry.Timestamp = %v, want between %v and %v", entry.Timestamp, before, after)
	}
	if len(entry.Labels) != len(labels) {
		t.Errorf("len(entry.Labels) = %d, want %d", len(entry.Labels), len(labels))
	}
	for k, v := range labels {
		if entry.Labels[k] != v {
			t.Errorf("entry.Labels[%q] = %q, want %q", k, entry.Labels[k], v)
		}
	}
}

func TestFormatLogEntry_NilLabels(t *testing.T) {
	entry := FormatLogEntry("session-1", 1, SeverityInfo, "test", nil)

	if entry.Labels != nil {
		t.Errorf("entry.Labels = %v, want nil", entry.Labels)
	}
}

func TestFormatLogEntry_EmptyLabels(t *testing.T) {
	labels := map[string]string{}
	entry := FormatLogEntry("session-1", 1, SeverityInfo, "test", labels)

	if len(entry.Labels) != 0 {
		t.Errorf("len(entry.Labels) = %d, want 0", len(entry.Labels))
	}
}

func TestMockCloudLogger_Log(t *testing.T) {
	mock := newMockCloudLogger()

	entry1 := FormatLogEntry("session-1", 1, SeverityInfo, "first log", nil)
	entry2 := FormatLogEntry("session-1", 2, SeverityWarning, "second log", map[string]string{"key": "value"})

	mock.Log(entry1)
	mock.Log(entry2)

	entries := mock.getEntries()
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}

	if entries[0].Message != "first log" {
		t.Errorf("entries[0].Message = %q, want %q", entries[0].Message, "first log")
	}
	if entries[0].Severity != SeverityInfo {
		t.Errorf("entries[0].Severity = %v, want %v", entries[0].Severity, SeverityInfo)
	}
	if entries[1].Message != "second log" {
		t.Errorf("entries[1].Message = %q, want %q", entries[1].Message, "second log")
	}
	if entries[1].Severity != SeverityWarning {
		t.Errorf("entries[1].Severity = %v, want %v", entries[1].Severity, SeverityWarning)
	}
}

func TestMockCloudLogger_Flush(t *testing.T) {
	mock := newMockCloudLogger()

	if mock.flushed {
		t.Errorf("mock.flushed should be false initially")
	}

	err := mock.Flush(5 * time.Second)
	if err != nil {
		t.Errorf("Flush() unexpected error: %v", err)
	}

	if !mock.flushed {
		t.Errorf("mock.flushed should be true after Flush()")
	}
}

func TestMockCloudLogger_Close(t *testing.T) {
	mock := newMockCloudLogger()

	if mock.closed {
		t.Errorf("mock.closed should be false initially")
	}

	err := mock.Close()
	if err != nil {
		t.Errorf("Close() unexpected error: %v", err)
	}

	if !mock.closed {
		t.Errorf("mock.closed should be true after Close()")
	}
}

func TestCloudLoggerInterface(t *testing.T) {
	// Verify that mockCloudLogger implements CloudLogger
	var _ CloudLogger = (*mockCloudLogger)(nil)

	// Verify that CloudLoggingClient implements CloudLogger
	var _ CloudLogger = (*CloudLoggingClient)(nil)
}

func TestMockCloudLogger_ConcurrentLog(t *testing.T) {
	mock := newMockCloudLogger()

	var wg sync.WaitGroup
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(iter int) {
			defer wg.Done()
			entry := FormatLogEntry("session-1", iter, SeverityInfo, "concurrent log", nil)
			mock.Log(entry)
		}(i)
	}

	wg.Wait()

	entries := mock.getEntries()
	if len(entries) != numGoroutines {
		t.Errorf("len(entries) = %d, want %d", len(entries), numGoroutines)
	}
}

func TestCloudLoggingClient_Close_Nil(t *testing.T) {
	// Test that Close handles nil client gracefully
	client := &CloudLoggingClient{
		client: nil,
		logger: nil,
	}

	err := client.Close()
	if err != nil {
		t.Errorf("Close() with nil client unexpected error: %v", err)
	}
}

func TestCloudLoggingClient_Log_NilLogger(t *testing.T) {
	// Test that Log handles nil logger gracefully (no panic)
	client := &CloudLoggingClient{
		logger: nil,
	}

	// Should not panic
	entry := FormatLogEntry("session-1", 1, SeverityInfo, "test", nil)
	client.Log(entry)
}

func TestCloudLoggingClient_Flush_NilLogger(t *testing.T) {
	// Test that Flush handles nil logger gracefully
	client := &CloudLoggingClient{
		logger: nil,
	}

	err := client.Flush(5 * time.Second)
	if err != nil {
		t.Errorf("Flush() with nil logger unexpected error: %v", err)
	}
}

func TestNewCloudLoggingClient_NoProjectID(t *testing.T) {
	// Test that NewCloudLoggingClient fails gracefully when no project ID is available
	// and no metadata server is reachable
	// Clear all project-related env vars
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")
	t.Setenv("GCP_PROJECT", "")
	t.Setenv("GCLOUD_PROJECT", "")

	ctx := context.Background()
	_, err := NewCloudLoggingClient(ctx, "", "test-log")

	// If running on a GCP VM, this might succeed because metadata server is available.
	// In that case, the test should still pass - it just means the env provides a project ID.
	if err != nil {
		// Verify it's the expected error about project ID
		if !containsStr(err.Error(), "project") {
			t.Errorf("NewCloudLoggingClient() unexpected error: %v", err)
		}
	}
	// If err is nil, the metadata server provided a project ID - that's also acceptable
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestDefaultLogName(t *testing.T) {
	if DefaultLogName != "agentium-sessions" {
		t.Errorf("DefaultLogName = %q, want %q", DefaultLogName, "agentium-sessions")
	}
}

func TestLogSeverity_toCloudSeverity(t *testing.T) {
	tests := []struct {
		name     string
		severity LogSeverity
		wantStr  string
	}{
		{"default maps correctly", SeverityDefault, "Default"},
		{"debug maps correctly", SeverityDebug, "Debug"},
		{"info maps correctly", SeverityInfo, "Info"},
		{"warning maps correctly", SeverityWarning, "Warning"},
		{"error maps correctly", SeverityError, "Error"},
		{"critical maps correctly", SeverityCritical, "Critical"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.severity.toCloudSeverity()
			if got.String() != tt.wantStr {
				t.Errorf("toCloudSeverity() = %v, want %v", got.String(), tt.wantStr)
			}
		})
	}
}

func TestLogEntry_Fields(t *testing.T) {
	now := time.Now()
	entry := LogEntry{
		Message:   "test message",
		Severity:  SeverityError,
		Timestamp: now,
		SessionID: "session-abc",
		Iteration: 3,
		Labels: map[string]string{
			"task": "issue-1",
		},
	}

	if entry.Message != "test message" {
		t.Errorf("Message = %q, want %q", entry.Message, "test message")
	}
	if entry.Severity != SeverityError {
		t.Errorf("Severity = %v, want %v", entry.Severity, SeverityError)
	}
	if !entry.Timestamp.Equal(now) {
		t.Errorf("Timestamp = %v, want %v", entry.Timestamp, now)
	}
	if entry.SessionID != "session-abc" {
		t.Errorf("SessionID = %q, want %q", entry.SessionID, "session-abc")
	}
	if entry.Iteration != 3 {
		t.Errorf("Iteration = %d, want %d", entry.Iteration, 3)
	}
	if entry.Labels["task"] != "issue-1" {
		t.Errorf("Labels[task] = %q, want %q", entry.Labels["task"], "issue-1")
	}
}
