package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Severity levels for structured logs
type Severity string

const (
	SeverityDefault  Severity = "DEFAULT"
	SeverityDebug    Severity = "DEBUG"
	SeverityInfo     Severity = "INFO"
	SeverityWarning  Severity = "WARNING"
	SeverityError    Severity = "ERROR"
	SeverityCritical Severity = "CRITICAL"
)

// LogEntry represents a structured log entry for Cloud Logging
type LogEntry struct {
	Severity  Severity               `json:"severity"`
	Message   string                 `json:"message"`
	Timestamp time.Time              `json:"timestamp"`
	SessionID string                 `json:"session_id"`
	Iteration int                    `json:"iteration"`
	Labels    map[string]string      `json:"labels,omitempty"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

// LoggerInterface defines the interface for cloud logging operations
type LoggerInterface interface {
	Log(severity Severity, message string, fields map[string]interface{})
	LogInfo(message string)
	LogWarning(message string)
	LogError(message string)
	SetIteration(iteration int)
	Flush() error
	Close() error
}

// CloudLogger provides structured logging to GCP Cloud Logging
// via structured JSON output compatible with Cloud Logging agent.
// On GCP VMs, the logging agent picks up structured JSON from stdout/stderr
// and forwards it to Cloud Logging with proper severity and labels.
type CloudLogger struct {
	writer    io.Writer
	sessionID string
	iteration int
	labels    map[string]string
	mu        sync.Mutex
	closed    bool
	flushFn   func() error // Optional flush function for buffered writers
}

// CloudLoggerOption allows configuring the CloudLogger
type CloudLoggerOption func(*CloudLogger)

// WithLabels adds custom labels to all log entries
func WithLabels(labels map[string]string) CloudLoggerOption {
	return func(cl *CloudLogger) {
		for k, v := range labels {
			cl.labels[k] = v
		}
	}
}

// WithIteration sets the current iteration number
func WithIteration(iteration int) CloudLoggerOption {
	return func(cl *CloudLogger) {
		cl.iteration = iteration
	}
}

// WithWriter sets a custom writer for log output
func WithWriter(w io.Writer) CloudLoggerOption {
	return func(cl *CloudLogger) {
		cl.writer = w
	}
}

// WithFlushFunc sets a custom flush function
func WithFlushFunc(fn func() error) CloudLoggerOption {
	return func(cl *CloudLogger) {
		cl.flushFn = fn
	}
}

// NewCloudLogger creates a new CloudLogger instance that writes structured
// JSON logs compatible with GCP Cloud Logging.
// On GCP VMs with the logging agent installed, these logs are automatically
// picked up and forwarded to Cloud Logging with proper severity levels.
func NewCloudLogger(sessionID string, opts ...CloudLoggerOption) *CloudLogger {
	cl := &CloudLogger{
		writer:    os.Stderr, // Cloud Logging agent reads from stderr by default
		sessionID: sessionID,
		labels: map[string]string{
			"session_id": sessionID,
			"component":  "agentium-controller",
		},
	}

	for _, opt := range opts {
		opt(cl)
	}

	return cl
}

// Log writes a structured log entry
func (cl *CloudLogger) Log(severity Severity, message string, fields map[string]interface{}) {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	if cl.closed {
		return
	}

	entry := LogEntry{
		Severity:  severity,
		Message:   message,
		Timestamp: time.Now().UTC(),
		SessionID: cl.sessionID,
		Iteration: cl.iteration,
		Labels:    cl.labels,
		Fields:    fields,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		fmt.Fprintf(cl.writer, `{"severity":"ERROR","message":"failed to marshal log entry: %v"}`+"\n", err)
		return
	}
	fmt.Fprintf(cl.writer, "%s\n", data)
}

// LogInfo writes an INFO level log entry
func (cl *CloudLogger) LogInfo(message string) {
	cl.Log(SeverityInfo, message, nil)
}

// LogWarning writes a WARNING level log entry
func (cl *CloudLogger) LogWarning(message string) {
	cl.Log(SeverityWarning, message, nil)
}

// LogError writes an ERROR level log entry
func (cl *CloudLogger) LogError(message string) {
	cl.Log(SeverityError, message, nil)
}

// SetIteration updates the current iteration number for subsequent logs
func (cl *CloudLogger) SetIteration(iteration int) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	cl.iteration = iteration
}

// Flush ensures all buffered logs are written
func (cl *CloudLogger) Flush() error {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	if cl.closed {
		return nil
	}

	if cl.flushFn != nil {
		return cl.flushFn()
	}

	// If the writer implements a Sync/Flush method, call it
	if syncer, ok := cl.writer.(interface{ Sync() error }); ok {
		return syncer.Sync()
	}

	return nil
}

// Close flushes remaining logs and marks the logger as closed
func (cl *CloudLogger) Close() error {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	if cl.closed {
		return nil
	}

	cl.closed = true

	// Flush any remaining buffered data
	if cl.flushFn != nil {
		return cl.flushFn()
	}

	return nil
}

// FormatLogEntry formats a LogEntry as a JSON string for local output
func FormatLogEntry(entry LogEntry) string {
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Sprintf(`{"error": "failed to marshal log entry: %v"}`, err)
	}
	return string(data)
}

// FallbackLogger is a structured logger that writes to a local io.Writer.
// It produces JSON-structured output compatible with Cloud Logging's
// structured log format, but writes to stdout for local debugging.
type FallbackLogger struct {
	writer    io.Writer
	sessionID string
	iteration int
	labels    map[string]string
	mu        sync.Mutex
}

// NewFallbackLogger creates a logger that writes structured JSON to the given writer
func NewFallbackLogger(writer io.Writer, sessionID string) *FallbackLogger {
	return &FallbackLogger{
		writer:    writer,
		sessionID: sessionID,
		labels: map[string]string{
			"session_id": sessionID,
			"component":  "agentium-controller",
		},
	}
}

// Log writes a structured log entry to the writer
func (fl *FallbackLogger) Log(severity Severity, message string, fields map[string]interface{}) {
	fl.mu.Lock()
	defer fl.mu.Unlock()

	entry := LogEntry{
		Severity:  severity,
		Message:   message,
		Timestamp: time.Now().UTC(),
		SessionID: fl.sessionID,
		Iteration: fl.iteration,
		Labels:    fl.labels,
		Fields:    fields,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		fmt.Fprintf(fl.writer, `{"severity":"ERROR","message":"failed to marshal log entry: %v"}`+"\n", err)
		return
	}
	fmt.Fprintf(fl.writer, "%s\n", data)
}

// LogInfo writes an INFO level log entry
func (fl *FallbackLogger) LogInfo(message string) {
	fl.Log(SeverityInfo, message, nil)
}

// LogWarning writes a WARNING level log entry
func (fl *FallbackLogger) LogWarning(message string) {
	fl.Log(SeverityWarning, message, nil)
}

// LogError writes an ERROR level log entry
func (fl *FallbackLogger) LogError(message string) {
	fl.Log(SeverityError, message, nil)
}

// SetIteration updates the current iteration number for subsequent logs
func (fl *FallbackLogger) SetIteration(iteration int) {
	fl.mu.Lock()
	defer fl.mu.Unlock()
	fl.iteration = iteration
}

// Flush is a no-op for the fallback logger (writes are synchronous)
func (fl *FallbackLogger) Flush() error {
	return nil
}

// Close is a no-op for the fallback logger
func (fl *FallbackLogger) Close() error {
	return nil
}

// NewLogger creates the appropriate logger based on environment.
// On GCP VMs (detected via metadata server), it creates a CloudLogger
// that writes structured JSON to stderr (picked up by the Cloud Logging agent).
// Otherwise, it falls back to structured JSON on stdout for local debugging.
func NewLogger(ctx context.Context, sessionID string, opts ...CloudLoggerOption) LoggerInterface {
	// On GCP, use structured logging to stderr (Cloud Logging agent picks it up)
	if isRunningOnGCP() {
		return NewCloudLogger(sessionID, opts...)
	}

	// Fallback: structured JSON to stdout
	return NewFallbackLogger(os.Stdout, sessionID)
}

// isRunningOnGCP checks if the code is running on a GCP environment
// by probing the metadata server
func isRunningOnGCP() bool {
	client := &http.Client{Timeout: 1 * time.Second}
	req, err := http.NewRequest("GET", "http://metadata.google.internal/computeMetadata/v1/", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Metadata-Flavor", "Google")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// Ensure CloudLogger implements LoggerInterface
var _ LoggerInterface = (*CloudLogger)(nil)

// Ensure FallbackLogger implements LoggerInterface
var _ LoggerInterface = (*FallbackLogger)(nil)

// SanitizeForLog removes potentially sensitive data from strings
// before logging. It redacts common patterns like tokens, keys, etc.
func SanitizeForLog(s string) string {
	// Redact GitHub tokens
	if strings.HasPrefix(s, "ghs_") || strings.HasPrefix(s, "ghp_") || strings.HasPrefix(s, "gho_") {
		return "[REDACTED_GITHUB_TOKEN]"
	}
	// Redact Bearer tokens
	if strings.HasPrefix(s, "Bearer ") {
		return "Bearer [REDACTED]"
	}
	return s
}
