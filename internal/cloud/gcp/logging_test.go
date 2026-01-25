package gcp

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/logging"
)

// mockLogWriter is a test mock for the LogWriter interface
type mockLogWriter struct {
	mu       sync.Mutex
	entries  []logging.Entry
	flushed  int
	closed   bool
	flushErr error
	closeErr error
}

func newMockLogWriter() *mockLogWriter {
	return &mockLogWriter{
		entries: make([]logging.Entry, 0),
	}
}

func (m *mockLogWriter) Log(entry logging.Entry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, entry)
}

func (m *mockLogWriter) Flush() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.flushed++
	return m.flushErr
}

func (m *mockLogWriter) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return m.closeErr
}

func (m *mockLogWriter) getEntries() []logging.Entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]logging.Entry, len(m.entries))
	copy(result, m.entries)
	return result
}

func TestCloudLogger_Info(t *testing.T) {
	mock := newMockLogWriter()
	logger := NewCloudLoggerWithWriter(mock, "test-session-123", nil)

	logger.Info("test message")

	entries := mock.getEntries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	entry := entries[0]
	if entry.Severity != logging.Info {
		t.Errorf("expected severity Info, got %v", entry.Severity)
	}

	payload, ok := entry.Payload.(LogEntry)
	if !ok {
		t.Fatalf("expected LogEntry payload, got %T", entry.Payload)
	}

	if payload.Message != "test message" {
		t.Errorf("expected message 'test message', got %q", payload.Message)
	}
	if payload.SessionID != "test-session-123" {
		t.Errorf("expected session ID 'test-session-123', got %q", payload.SessionID)
	}
	if payload.Severity != SeverityInfo {
		t.Errorf("expected severity INFO, got %q", payload.Severity)
	}
}

func TestCloudLogger_Infof(t *testing.T) {
	mock := newMockLogWriter()
	logger := NewCloudLoggerWithWriter(mock, "sess-1", nil)

	logger.Infof("hello %s, count=%d", "world", 42)

	entries := mock.getEntries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	payload := entries[0].Payload.(LogEntry)
	if payload.Message != "hello world, count=42" {
		t.Errorf("expected formatted message, got %q", payload.Message)
	}
}

func TestCloudLogger_SeverityLevels(t *testing.T) {
	tests := []struct {
		name     string
		logFunc  func(*CloudLogger)
		wantSev  logging.Severity
		wantType Severity
	}{
		{
			name:     "Info",
			logFunc:  func(l *CloudLogger) { l.Info("info msg") },
			wantSev:  logging.Info,
			wantType: SeverityInfo,
		},
		{
			name:     "Warning",
			logFunc:  func(l *CloudLogger) { l.Warning("warn msg") },
			wantSev:  logging.Warning,
			wantType: SeverityWarning,
		},
		{
			name:     "Error",
			logFunc:  func(l *CloudLogger) { l.Error("error msg") },
			wantSev:  logging.Error,
			wantType: SeverityError,
		},
		{
			name:     "Warningf",
			logFunc:  func(l *CloudLogger) { l.Warningf("warn %d", 1) },
			wantSev:  logging.Warning,
			wantType: SeverityWarning,
		},
		{
			name:     "Errorf",
			logFunc:  func(l *CloudLogger) { l.Errorf("err %d", 2) },
			wantSev:  logging.Error,
			wantType: SeverityError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := newMockLogWriter()
			logger := NewCloudLoggerWithWriter(mock, "sess-1", nil)

			tt.logFunc(logger)

			entries := mock.getEntries()
			if len(entries) != 1 {
				t.Fatalf("expected 1 entry, got %d", len(entries))
			}

			if entries[0].Severity != tt.wantSev {
				t.Errorf("expected severity %v, got %v", tt.wantSev, entries[0].Severity)
			}

			payload := entries[0].Payload.(LogEntry)
			if payload.Severity != tt.wantType {
				t.Errorf("expected payload severity %q, got %q", tt.wantType, payload.Severity)
			}
		})
	}
}

func TestCloudLogger_SetIteration(t *testing.T) {
	mock := newMockLogWriter()
	logger := NewCloudLoggerWithWriter(mock, "sess-1", nil)

	// Log with default iteration (0)
	logger.Info("first")

	// Set iteration and log again
	logger.SetIteration(5)
	logger.Info("second")

	entries := mock.getEntries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	payload0 := entries[0].Payload.(LogEntry)
	if payload0.Iteration != 0 {
		t.Errorf("first entry iteration = %d, want 0", payload0.Iteration)
	}

	payload1 := entries[1].Payload.(LogEntry)
	if payload1.Iteration != 5 {
		t.Errorf("second entry iteration = %d, want 5", payload1.Iteration)
	}
}

func TestCloudLogger_Timestamp(t *testing.T) {
	mock := newMockLogWriter()
	logger := NewCloudLoggerWithWriter(mock, "sess-1", nil)

	before := time.Now().UTC()
	logger.Info("timed message")
	after := time.Now().UTC()

	entries := mock.getEntries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	payload := entries[0].Payload.(LogEntry)
	if payload.Timestamp.Before(before) || payload.Timestamp.After(after) {
		t.Errorf("timestamp %v not between %v and %v", payload.Timestamp, before, after)
	}

	// Verify entry-level timestamp matches
	if entries[0].Timestamp.Before(before) || entries[0].Timestamp.After(after) {
		t.Errorf("entry timestamp %v not between %v and %v", entries[0].Timestamp, before, after)
	}
}

func TestCloudLogger_Labels(t *testing.T) {
	mock := newMockLogWriter()
	labels := map[string]string{
		"repository": "github.com/test/repo",
		"env":        "test",
	}
	logger := NewCloudLoggerWithWriter(mock, "sess-labels", labels)

	logger.Info("labeled message")

	entries := mock.getEntries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	// Check entry-level labels
	if entries[0].Labels["session_id"] != "sess-labels" {
		t.Errorf("entry label session_id = %q, want 'sess-labels'", entries[0].Labels["session_id"])
	}
	if entries[0].Labels["repository"] != "github.com/test/repo" {
		t.Errorf("entry label repository = %q, want 'github.com/test/repo'", entries[0].Labels["repository"])
	}

	// Check payload labels
	payload := entries[0].Payload.(LogEntry)
	if payload.Labels["env"] != "test" {
		t.Errorf("payload label env = %q, want 'test'", payload.Labels["env"])
	}
}

func TestCloudLogger_LogWithLabels(t *testing.T) {
	mock := newMockLogWriter()
	logger := NewCloudLoggerWithWriter(mock, "sess-1", map[string]string{
		"base": "value",
	})

	extra := map[string]string{
		"task_id": "42",
		"phase":   "IMPLEMENT",
	}
	logger.LogWithLabels(SeverityWarning, "task update", extra)

	entries := mock.getEntries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	// Verify base labels are preserved
	if entries[0].Labels["base"] != "value" {
		t.Errorf("base label missing, labels = %v", entries[0].Labels)
	}

	// Verify extra labels are added
	if entries[0].Labels["task_id"] != "42" {
		t.Errorf("extra label task_id missing, labels = %v", entries[0].Labels)
	}
	if entries[0].Labels["phase"] != "IMPLEMENT" {
		t.Errorf("extra label phase missing, labels = %v", entries[0].Labels)
	}
}

func TestCloudLogger_Flush(t *testing.T) {
	mock := newMockLogWriter()
	logger := NewCloudLoggerWithWriter(mock, "sess-1", nil)

	logger.Info("before flush")

	if err := logger.Flush(); err != nil {
		t.Errorf("Flush() unexpected error: %v", err)
	}

	mock.mu.Lock()
	flushed := mock.flushed
	mock.mu.Unlock()

	if flushed != 1 {
		t.Errorf("expected 1 flush call, got %d", flushed)
	}
}

func TestCloudLogger_Close(t *testing.T) {
	mock := newMockLogWriter()
	logger := NewCloudLoggerWithWriter(mock, "sess-1", nil)

	logger.Info("before close")

	if err := logger.Close(); err != nil {
		t.Errorf("Close() unexpected error: %v", err)
	}

	mock.mu.Lock()
	flushed := mock.flushed
	closed := mock.closed
	mock.mu.Unlock()

	// Close should flush first
	if flushed != 1 {
		t.Errorf("expected 1 flush call on close, got %d", flushed)
	}
	if !closed {
		t.Error("expected Close() to be called on writer")
	}
}

func TestCloudLogger_FlushError(t *testing.T) {
	mock := newMockLogWriter()
	mock.flushErr = fmt.Errorf("flush failed")
	logger := NewCloudLoggerWithWriter(mock, "sess-1", nil)

	err := logger.Close()
	if err == nil {
		t.Error("expected error from Close() when flush fails")
	}
	if !strings.Contains(err.Error(), "flush failed") {
		t.Errorf("error = %q, want to contain 'flush failed'", err.Error())
	}
}

func TestCloudLogger_ConcurrentWrites(t *testing.T) {
	mock := newMockLogWriter()
	logger := NewCloudLoggerWithWriter(mock, "sess-concurrent", nil)

	var wg sync.WaitGroup
	numWriters := 10
	msgsPerWriter := 50

	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < msgsPerWriter; j++ {
				logger.SetIteration(id*100 + j)
				logger.Infof("writer %d msg %d", id, j)
			}
		}(i)
	}

	wg.Wait()

	entries := mock.getEntries()
	expectedCount := numWriters * msgsPerWriter
	if len(entries) != expectedCount {
		t.Errorf("expected %d entries, got %d", expectedCount, len(entries))
	}

	// Verify all entries have session ID
	for i, entry := range entries {
		payload := entry.Payload.(LogEntry)
		if payload.SessionID != "sess-concurrent" {
			t.Errorf("entry %d session ID = %q, want 'sess-concurrent'", i, payload.SessionID)
		}
	}
}

func TestCloudLogger_LogWithLabels_SeverityMapping(t *testing.T) {
	tests := []struct {
		severity Severity
		want     logging.Severity
	}{
		{SeverityDebug, logging.Debug},
		{SeverityInfo, logging.Info},
		{SeverityWarning, logging.Warning},
		{SeverityError, logging.Error},
		{SeverityCritical, logging.Critical},
		{Severity("UNKNOWN"), logging.Default},
	}

	for _, tt := range tests {
		t.Run(string(tt.severity), func(t *testing.T) {
			mock := newMockLogWriter()
			logger := NewCloudLoggerWithWriter(mock, "sess-1", nil)

			logger.LogWithLabels(tt.severity, "test", nil)

			entries := mock.getEntries()
			if entries[0].Severity != tt.want {
				t.Errorf("severity %q mapped to %v, want %v", tt.severity, entries[0].Severity, tt.want)
			}
		})
	}
}

func TestNewCloudLoggerWithWriter_NilLabels(t *testing.T) {
	mock := newMockLogWriter()
	logger := NewCloudLoggerWithWriter(mock, "sess-nil", nil)

	logger.Info("no labels")

	entries := mock.getEntries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	// Should still have session_id label
	if entries[0].Labels["session_id"] != "sess-nil" {
		t.Errorf("session_id label missing, labels = %v", entries[0].Labels)
	}
}

func TestCloudLoggerConfig_Defaults(t *testing.T) {
	cfg := CloudLoggerConfig{}

	if cfg.LogName != "" {
		t.Errorf("default LogName should be empty (set by NewCloudLogger), got %q", cfg.LogName)
	}
	if cfg.SessionID != "" {
		t.Errorf("default SessionID should be empty, got %q", cfg.SessionID)
	}
}
