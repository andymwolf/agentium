package gcp

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"cloud.google.com/go/logging"
)

// mockLogger implements Logger interface for testing
type mockLogger struct {
	entries []logEntry
	closed  bool
	flushed bool
}

type logEntry struct {
	severity logging.Severity
	message  string
	labels   map[string]string
}

func (m *mockLogger) Log(severity logging.Severity, message string, labels map[string]string) {
	m.entries = append(m.entries, logEntry{
		severity: severity,
		message:  message,
		labels:   labels,
	})
}

func (m *mockLogger) Info(message string) {
	m.Log(logging.Info, message, nil)
}

func (m *mockLogger) Warn(message string) {
	m.Log(logging.Warning, message, nil)
}

func (m *mockLogger) Error(message string) {
	m.Log(logging.Error, message, nil)
}

func (m *mockLogger) Flush() error {
	m.flushed = true
	return nil
}

func (m *mockLogger) Close() error {
	m.closed = true
	return nil
}

func TestMockLogger_Info(t *testing.T) {
	mock := &mockLogger{}

	mock.Info("test message")

	if len(mock.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(mock.entries))
	}

	entry := mock.entries[0]
	if entry.severity != logging.Info {
		t.Errorf("severity = %v, want %v", entry.severity, logging.Info)
	}
	if entry.message != "test message" {
		t.Errorf("message = %q, want %q", entry.message, "test message")
	}
}

func TestMockLogger_Warn(t *testing.T) {
	mock := &mockLogger{}

	mock.Warn("warning message")

	if len(mock.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(mock.entries))
	}

	entry := mock.entries[0]
	if entry.severity != logging.Warning {
		t.Errorf("severity = %v, want %v", entry.severity, logging.Warning)
	}
	if entry.message != "warning message" {
		t.Errorf("message = %q, want %q", entry.message, "warning message")
	}
}

func TestMockLogger_Error(t *testing.T) {
	mock := &mockLogger{}

	mock.Error("error message")

	if len(mock.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(mock.entries))
	}

	entry := mock.entries[0]
	if entry.severity != logging.Error {
		t.Errorf("severity = %v, want %v", entry.severity, logging.Error)
	}
	if entry.message != "error message" {
		t.Errorf("message = %q, want %q", entry.message, "error message")
	}
}

func TestMockLogger_Log_WithLabels(t *testing.T) {
	mock := &mockLogger{}

	labels := map[string]string{
		"session_id": "test-session",
		"iteration":  "5",
	}

	mock.Log(logging.Info, "test with labels", labels)

	if len(mock.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(mock.entries))
	}

	entry := mock.entries[0]
	if entry.labels == nil {
		t.Fatal("labels should not be nil")
	}

	if entry.labels["session_id"] != "test-session" {
		t.Errorf("session_id = %q, want %q", entry.labels["session_id"], "test-session")
	}
	if entry.labels["iteration"] != "5" {
		t.Errorf("iteration = %q, want %q", entry.labels["iteration"], "5")
	}
}

func TestMockLogger_MultipleEntries(t *testing.T) {
	mock := &mockLogger{}

	mock.Info("info 1")
	mock.Warn("warning 1")
	mock.Error("error 1")
	mock.Info("info 2")

	if len(mock.entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(mock.entries))
	}

	// Verify order and content
	expectedEntries := []struct {
		severity logging.Severity
		message  string
	}{
		{logging.Info, "info 1"},
		{logging.Warning, "warning 1"},
		{logging.Error, "error 1"},
		{logging.Info, "info 2"},
	}

	for i, expected := range expectedEntries {
		entry := mock.entries[i]
		if entry.severity != expected.severity {
			t.Errorf("entry[%d] severity = %v, want %v", i, entry.severity, expected.severity)
		}
		if entry.message != expected.message {
			t.Errorf("entry[%d] message = %q, want %q", i, entry.message, expected.message)
		}
	}
}

func TestMockLogger_Flush(t *testing.T) {
	mock := &mockLogger{}

	mock.Info("test")

	if mock.flushed {
		t.Error("flushed should be false before Flush() is called")
	}

	err := mock.Flush()
	if err != nil {
		t.Errorf("Flush() unexpected error: %v", err)
	}

	if !mock.flushed {
		t.Error("flushed should be true after Flush() is called")
	}
}

func TestMockLogger_Close(t *testing.T) {
	mock := &mockLogger{}

	if mock.closed {
		t.Error("closed should be false before Close() is called")
	}

	err := mock.Close()
	if err != nil {
		t.Errorf("Close() unexpected error: %v", err)
	}

	if !mock.closed {
		t.Error("closed should be true after Close() is called")
	}
}

func TestStdLogger_Info(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStdLogger(&buf, "[test] ")

	logger.Info("test info message")

	output := buf.String()
	if !strings.Contains(output, "[test]") {
		t.Errorf("output should contain prefix [test], got: %q", output)
	}
	if !strings.Contains(output, "[Info]") {
		t.Errorf("output should contain [Info], got: %q", output)
	}
	if !strings.Contains(output, "test info message") {
		t.Errorf("output should contain message, got: %q", output)
	}
}

func TestStdLogger_Warn(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStdLogger(&buf, "[test] ")

	logger.Warn("test warning message")

	output := buf.String()
	if !strings.Contains(output, "[Warning]") {
		t.Errorf("output should contain [Warning], got: %q", output)
	}
	if !strings.Contains(output, "test warning message") {
		t.Errorf("output should contain message, got: %q", output)
	}
}

func TestStdLogger_Error(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStdLogger(&buf, "[test] ")

	logger.Error("test error message")

	output := buf.String()
	if !strings.Contains(output, "[Error]") {
		t.Errorf("output should contain [Error], got: %q", output)
	}
	if !strings.Contains(output, "test error message") {
		t.Errorf("output should contain message, got: %q", output)
	}
}

func TestStdLogger_Log_WithLabels(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStdLogger(&buf, "[test] ")

	labels := map[string]string{
		"session_id": "test-session",
		"iteration":  "3",
	}

	logger.Log(logging.Info, "test with labels", labels)

	output := buf.String()
	if !strings.Contains(output, "test with labels") {
		t.Errorf("output should contain message, got: %q", output)
	}
	if !strings.Contains(output, "session_id") {
		t.Errorf("output should contain session_id label, got: %q", output)
	}
	if !strings.Contains(output, "test-session") {
		t.Errorf("output should contain session_id value, got: %q", output)
	}
}

func TestStdLogger_Flush(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStdLogger(&buf, "[test] ")

	logger.Info("test")

	// Flush should be a no-op but shouldn't error
	err := logger.Flush()
	if err != nil {
		t.Errorf("Flush() unexpected error: %v", err)
	}
}

func TestStdLogger_Close(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStdLogger(&buf, "[test] ")

	// Close should be a no-op but shouldn't error
	err := logger.Close()
	if err != nil {
		t.Errorf("Close() unexpected error: %v", err)
	}
}

func TestLoggerInterface(t *testing.T) {
	// Verify that CloudLogger implements Logger
	var _ Logger = (*CloudLogger)(nil)

	// Verify that StdLogger implements Logger
	var _ Logger = (*StdLogger)(nil)

	// Verify that mockLogger implements Logger
	var _ Logger = (*mockLogger)(nil)
}

func TestCloudLogger_SetIteration(t *testing.T) {
	// We can't easily test the actual CloudLogger without GCP credentials,
	// but we can test that the interface is correctly defined
	// This test ensures the SetIteration method exists and can be called
	// The actual behavior will be tested in integration tests

	// For now, just verify the method signature exists by checking
	// that CloudLogger has the expected methods
	var cl *CloudLogger
	if cl != nil {
		cl.SetIteration(5) // This won't execute but verifies the method exists
	}
}

func TestCloudLogger_LogStructure(t *testing.T) {
	// Test that we can create a CloudLogger structure
	// (without actually connecting to GCP)
	cl := &CloudLogger{
		sessionID: "test-session",
		iteration: 5,
		projectID: "test-project",
	}

	if cl.sessionID != "test-session" {
		t.Errorf("sessionID = %q, want %q", cl.sessionID, "test-session")
	}
	if cl.iteration != 5 {
		t.Errorf("iteration = %d, want %d", cl.iteration, 5)
	}
	if cl.projectID != "test-project" {
		t.Errorf("projectID = %q, want %q", cl.projectID, "test-project")
	}
}

// TestNewCloudLogger_ProjectIDDetection tests that project ID detection fails gracefully
func TestNewCloudLogger_ProjectIDDetection(t *testing.T) {
	// Save and restore environment
	oldEnv := map[string]string{
		"GOOGLE_CLOUD_PROJECT": os.Getenv("GOOGLE_CLOUD_PROJECT"),
		"GCP_PROJECT":          os.Getenv("GCP_PROJECT"),
		"GCLOUD_PROJECT":       os.Getenv("GCLOUD_PROJECT"),
	}
	defer func() {
		for k, v := range oldEnv {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	}()

	// Clear all project env vars
	os.Unsetenv("GOOGLE_CLOUD_PROJECT")
	os.Unsetenv("GCP_PROJECT")
	os.Unsetenv("GCLOUD_PROJECT")

	ctx := context.Background()

	// This should fail because we're not on GCP and have no env vars set
	// However, if we're running in a GCP environment, it might succeed by using metadata server
	_, err := NewCloudLogger(ctx, "test-session", "test-log")

	// If we're not on GCP and have no env vars, we expect an error
	// But if we ARE on GCP (like in CI), it's OK to succeed
	// This test just ensures the function doesn't panic
	if err != nil {
		// Error is expected when not on GCP - this is the correct behavior
		t.Logf("NewCloudLogger() returned expected error when project ID cannot be detected: %v", err)
	} else {
		// Success means we're on GCP or have credentials available - also valid
		t.Logf("NewCloudLogger() succeeded (likely running on GCP or with credentials)")
	}
}
