package gcp

import (
	"bytes"
	"log"
	"strings"
	"sync"
	"testing"
)

func TestSeverity_String(t *testing.T) {
	tests := []struct {
		severity Severity
		want     string
	}{
		{SeverityDefault, "DEFAULT"},
		{SeverityDebug, "DEBUG"},
		{SeverityInfo, "INFO"},
		{SeverityWarning, "WARNING"},
		{SeverityError, "ERROR"},
		{SeverityCritical, "CRITICAL"},
		{Severity(999), "DEFAULT"}, // Unknown severity
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.severity.String()
			if got != tt.want {
				t.Errorf("Severity(%d).String() = %q, want %q", tt.severity, got, tt.want)
			}
		})
	}
}

func TestSeverity_ToGCPSeverity(t *testing.T) {
	tests := []struct {
		severity Severity
		wantName string
	}{
		{SeverityDebug, "Debug"},
		{SeverityInfo, "Info"},
		{SeverityWarning, "Warning"},
		{SeverityError, "Error"},
		{SeverityCritical, "Critical"},
		{SeverityDefault, "Default"},
	}

	for _, tt := range tests {
		t.Run(tt.wantName, func(t *testing.T) {
			got := tt.severity.toGCPSeverity()
			if got.String() != tt.wantName {
				t.Errorf("Severity(%d).toGCPSeverity() = %q, want %q", tt.severity, got.String(), tt.wantName)
			}
		})
	}
}

func TestFormatLogEntry(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		iteration int
		event     string
		details   map[string]string
		wantParts []string
	}{
		{
			name:      "basic entry",
			sessionID: "test-session",
			iteration: 3,
			event:     "session started",
			details:   nil,
			wantParts: []string{"[session=test-session iter=3]", "session started"},
		},
		{
			name:      "entry with details",
			sessionID: "sess-123",
			iteration: 1,
			event:     "task completed",
			details: map[string]string{
				"task_id": "42",
				"status":  "success",
			},
			wantParts: []string{"[session=sess-123 iter=1]", "task completed", "task_id=", "status="},
		},
		{
			name:      "empty details map",
			sessionID: "s1",
			iteration: 0,
			event:     "init",
			details:   map[string]string{},
			wantParts: []string{"[session=s1 iter=0]", "init"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatLogEntry(tt.sessionID, tt.iteration, tt.event, tt.details)
			for _, part := range tt.wantParts {
				if !strings.Contains(result, part) {
					t.Errorf("FormatLogEntry() = %q, want to contain %q", result, part)
				}
			}
		})
	}
}

func TestFormatLogEntry_NoBracesWithoutDetails(t *testing.T) {
	result := FormatLogEntry("s1", 0, "test", nil)
	if strings.Contains(result, "{") {
		t.Errorf("FormatLogEntry() with nil details should not contain braces, got %q", result)
	}

	result = FormatLogEntry("s1", 0, "test", map[string]string{})
	if strings.Contains(result, "{") {
		t.Errorf("FormatLogEntry() with empty details should not contain braces, got %q", result)
	}
}

func TestStdLogger_Info(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStdLogger(&buf, "[test] ", "test-session")

	logger.Info("test info message")

	output := buf.String()
	if !strings.Contains(output, "[test]") {
		t.Errorf("output should contain prefix [test], got: %q", output)
	}
	if !strings.Contains(output, "[INFO]") {
		t.Errorf("output should contain [INFO], got: %q", output)
	}
	if !strings.Contains(output, "test info message") {
		t.Errorf("output should contain message, got: %q", output)
	}
	if !strings.Contains(output, "session=test-session") {
		t.Errorf("output should contain session ID, got: %q", output)
	}
}

func TestStdLogger_Warn(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStdLogger(&buf, "[test] ", "test-session")

	logger.Warn("test warning")

	output := buf.String()
	if !strings.Contains(output, "[WARNING]") {
		t.Errorf("output should contain [WARNING], got: %q", output)
	}
	if !strings.Contains(output, "test warning") {
		t.Errorf("output should contain message, got: %q", output)
	}
}

func TestStdLogger_Error(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStdLogger(&buf, "[test] ", "test-session")

	logger.Error("test error")

	output := buf.String()
	if !strings.Contains(output, "[ERROR]") {
		t.Errorf("output should contain [ERROR], got: %q", output)
	}
	if !strings.Contains(output, "test error") {
		t.Errorf("output should contain message, got: %q", output)
	}
}

func TestStdLogger_Log_WithLabels(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStdLogger(&buf, "[test] ", "test-session")

	labels := map[string]string{
		"task_id": "42",
		"phase":   "test",
	}

	logger.Log(SeverityInfo, "test with labels", labels)

	output := buf.String()
	if !strings.Contains(output, "test with labels") {
		t.Errorf("output should contain message, got: %q", output)
	}
	if !strings.Contains(output, "task_id") {
		t.Errorf("output should contain task_id label, got: %q", output)
	}
}

func TestStdLogger_SetIteration(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStdLogger(&buf, "[test] ", "test-session")

	logger.SetIteration(5)
	logger.Info("after set iteration")

	output := buf.String()
	if !strings.Contains(output, "iter=5") {
		t.Errorf("output should contain iter=5, got: %q", output)
	}
}

func TestStdLogger_Flush(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStdLogger(&buf, "[test] ", "test-session")

	err := logger.Flush()
	if err != nil {
		t.Errorf("Flush() unexpected error: %v", err)
	}
}

func TestStdLogger_Close(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStdLogger(&buf, "[test] ", "test-session")

	err := logger.Close()
	if err != nil {
		t.Errorf("Close() unexpected error: %v", err)
	}
}

func TestStdLogger_ConcurrentAccess(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStdLogger(&buf, "[test] ", "test-session")

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			logger.SetIteration(n)
			logger.Info("concurrent message")
			logger.Warn("concurrent warning")
		}(i)
	}
	wg.Wait()

	// Should not panic and should have produced output
	output := buf.String()
	if output == "" {
		t.Error("expected output from concurrent logging")
	}
}

func TestStdLogger_NilLabels(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStdLogger(&buf, "[test] ", "test-session")

	logger.Log(SeverityInfo, "no labels", nil)

	output := buf.String()
	if !strings.Contains(output, "no labels") {
		t.Errorf("output should contain message, got: %q", output)
	}
	// Should not contain trailing space from empty labels
	if strings.Contains(output, " map[") {
		t.Errorf("output should not contain map[] for nil labels, got: %q", output)
	}
}

func TestCloudLoggingClient_Structure(t *testing.T) {
	cl := &CloudLoggingClient{
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

func TestCloudLoggingClient_SetIteration(t *testing.T) {
	cl := &CloudLoggingClient{
		sessionID: "test-session",
		iteration: 0,
	}

	cl.SetIteration(10)

	cl.mu.Lock()
	if cl.iteration != 10 {
		t.Errorf("iteration = %d, want 10", cl.iteration)
	}
	cl.mu.Unlock()
}

func TestCloudLoggingClient_ClosedLogging(t *testing.T) {
	var buf bytes.Buffer
	cl := &CloudLoggingClient{
		sessionID: "test-session",
		iteration: 1,
		closed:    true,
		fallback:  log.New(&buf, "[cloudlogger] ", 0),
	}

	// Should not panic, should write to fallback
	cl.Log(SeverityInfo, "test after close", nil)

	output := buf.String()
	if !strings.Contains(output, "logger closed") {
		t.Errorf("expected 'logger closed' in fallback output, got: %q", output)
	}
	if !strings.Contains(output, "test after close") {
		t.Errorf("expected message in fallback output, got: %q", output)
	}
}

func TestCloudLoggingClient_FlushClosed(t *testing.T) {
	cl := &CloudLoggingClient{
		closed: true,
	}

	err := cl.Flush()
	if err != nil {
		t.Errorf("Flush() on closed client should return nil, got: %v", err)
	}
}

func TestCloudLoggingClient_CloseTwice(t *testing.T) {
	cl := &CloudLoggingClient{
		closed: true,
	}

	err := cl.Close()
	if err != nil {
		t.Errorf("Close() on already closed client should return nil, got: %v", err)
	}
}

func TestCloudLoggerInterface(t *testing.T) {
	// Verify that CloudLoggingClient implements CloudLogger
	var _ CloudLogger = (*CloudLoggingClient)(nil)

	// Verify that StdLogger implements CloudLogger
	var _ CloudLogger = (*StdLogger)(nil)
}

func TestCloudLoggingClient_ConcurrentSetIteration(t *testing.T) {
	cl := &CloudLoggingClient{
		sessionID: "test-session",
		iteration: 0,
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			cl.SetIteration(n)
		}(i)
	}
	wg.Wait()

	// Should not have any race conditions
}

func TestCloudLoggingClient_LogAfterClose_AllSeverities(t *testing.T) {
	var buf bytes.Buffer
	cl := &CloudLoggingClient{
		sessionID: "test-session",
		iteration: 1,
		closed:    true,
		fallback:  log.New(&buf, "", 0),
	}

	cl.Info("info after close")
	cl.Warn("warn after close")
	cl.Error("error after close")

	output := buf.String()
	if !strings.Contains(output, "info after close") {
		t.Errorf("expected info message in output, got: %q", output)
	}
	if !strings.Contains(output, "warn after close") {
		t.Errorf("expected warn message in output, got: %q", output)
	}
	if !strings.Contains(output, "error after close") {
		t.Errorf("expected error message in output, got: %q", output)
	}
}
