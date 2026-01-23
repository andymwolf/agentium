package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Severity levels for structured logs
const (
	SeverityDefault  = "DEFAULT"
	SeverityDebug    = "DEBUG"
	SeverityInfo     = "INFO"
	SeverityWarning  = "WARNING"
	SeverityError    = "ERROR"
	SeverityCritical = "CRITICAL"
)

// LogEntry represents a structured log entry for Cloud Logging
type LogEntry struct {
	Severity  string            `json:"severity"`
	Message   string            `json:"message"`
	Timestamp string            `json:"timestamp"`
	Labels    map[string]string `json:"logging.googleapis.com/labels,omitempty"`
	SessionID string            `json:"sessionId,omitempty"`
	Iteration int               `json:"iteration,omitempty"`
}

// CloudLogger writes structured logs compatible with GCP Cloud Logging.
// When running on GCE, structured JSON written to stdout is automatically
// ingested by the Cloud Logging agent. This logger also supports writing
// directly to the Cloud Logging API for environments without the agent.
type CloudLogger struct {
	mu        sync.Mutex
	writer    io.Writer
	sessionID string
	iteration int
	labels    map[string]string
	apiClient *loggingAPIClient
}

// loggingAPIClient handles direct writes to the Cloud Logging API
type loggingAPIClient struct {
	projectID string
	logName   string
	client    *http.Client
}

// CloudLoggerOption configures a CloudLogger
type CloudLoggerOption func(*CloudLogger)

// WithWriter sets the underlying writer for structured log output
func WithWriter(w io.Writer) CloudLoggerOption {
	return func(l *CloudLogger) {
		l.writer = w
	}
}

// WithSessionID sets the session ID label on all log entries
func WithSessionID(id string) CloudLoggerOption {
	return func(l *CloudLogger) {
		l.sessionID = id
	}
}

// WithIteration sets the current iteration number on log entries
func WithIteration(iter int) CloudLoggerOption {
	return func(l *CloudLogger) {
		l.iteration = iter
	}
}

// WithLabels sets additional labels on all log entries
func WithLabels(labels map[string]string) CloudLoggerOption {
	return func(l *CloudLogger) {
		for k, v := range labels {
			l.labels[k] = v
		}
	}
}

// WithAPIClient enables direct Cloud Logging API writes
func WithAPIClient(projectID, logName string) CloudLoggerOption {
	return func(l *CloudLogger) {
		l.apiClient = &loggingAPIClient{
			projectID: projectID,
			logName:   logName,
			client: &http.Client{
				Timeout: 5 * time.Second,
			},
		}
	}
}

// NewCloudLogger creates a new structured logger for GCP Cloud Logging.
// By default it writes JSON-structured logs to the provided writer.
// On GCE instances, these are automatically picked up by the logging agent.
func NewCloudLogger(opts ...CloudLoggerOption) *CloudLogger {
	l := &CloudLogger{
		labels: make(map[string]string),
	}

	for _, opt := range opts {
		opt(l)
	}

	return l
}

// SetIteration updates the current iteration number for subsequent log entries
func (l *CloudLogger) SetIteration(iter int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.iteration = iter
}

// Log writes a structured log entry at the given severity level
func (l *CloudLogger) Log(severity, message string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry := l.buildEntry(severity, message)
	l.writeEntry(entry)
}

// Info writes an INFO-level structured log entry
func (l *CloudLogger) Info(message string) {
	l.Log(SeverityInfo, message)
}

// Infof writes a formatted INFO-level structured log entry
func (l *CloudLogger) Infof(format string, args ...interface{}) {
	l.Log(SeverityInfo, fmt.Sprintf(format, args...))
}

// Warning writes a WARNING-level structured log entry
func (l *CloudLogger) Warning(message string) {
	l.Log(SeverityWarning, message)
}

// Warningf writes a formatted WARNING-level structured log entry
func (l *CloudLogger) Warningf(format string, args ...interface{}) {
	l.Log(SeverityWarning, fmt.Sprintf(format, args...))
}

// Error writes an ERROR-level structured log entry
func (l *CloudLogger) Error(message string) {
	l.Log(SeverityError, message)
}

// Errorf writes a formatted ERROR-level structured log entry
func (l *CloudLogger) Errorf(format string, args ...interface{}) {
	l.Log(SeverityError, fmt.Sprintf(format, args...))
}

// Write implements io.Writer so CloudLogger can be used as the output
// for a standard log.Logger. Messages written this way are logged at INFO level.
func (l *CloudLogger) Write(p []byte) (n int, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	message := strings.TrimSpace(string(p))
	// Strip standard log prefix if present (e.g., "[controller] ")
	// Only strip if it's a short prefix tag at the start of the message
	if strings.HasPrefix(message, "[") {
		if idx := strings.Index(message, "] "); idx >= 0 && idx <= 20 {
			message = message[idx+2:]
		}
	}

	severity := l.detectSeverity(message)
	entry := l.buildEntry(severity, message)
	l.writeEntry(entry)

	return len(p), nil
}

// Flush ensures any buffered log entries are written.
// For the API client, this sends any pending entries.
func (l *CloudLogger) Flush() error {
	// Currently writes are synchronous, so flush is a no-op
	return nil
}

// Close flushes remaining logs and releases resources
func (l *CloudLogger) Close() error {
	return l.Flush()
}

// buildEntry creates a LogEntry from the current logger state
func (l *CloudLogger) buildEntry(severity, message string) LogEntry {
	entry := LogEntry{
		Severity:  severity,
		Message:   message,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		SessionID: l.sessionID,
		Iteration: l.iteration,
	}

	if len(l.labels) > 0 {
		entry.Labels = make(map[string]string, len(l.labels))
		for k, v := range l.labels {
			entry.Labels[k] = v
		}
	}

	return entry
}

// writeEntry serializes and writes a log entry
func (l *CloudLogger) writeEntry(entry LogEntry) {
	if l.writer == nil {
		return
	}

	data, err := json.Marshal(entry)
	if err != nil {
		// Fallback to plain text if JSON marshaling fails
		fmt.Fprintf(l.writer, "%s [%s] %s\n", entry.Timestamp, entry.Severity, entry.Message)
		return
	}

	l.writer.Write(data)
	l.writer.Write([]byte("\n"))

	// Also write to Cloud Logging API if configured
	if l.apiClient != nil {
		l.apiClient.writeEntry(context.Background(), entry)
	}
}

// detectSeverity attempts to determine log severity from message content
func (l *CloudLogger) detectSeverity(message string) string {
	lower := strings.ToLower(message)
	switch {
	case strings.HasPrefix(lower, "warning") || strings.HasPrefix(lower, "warn:") || strings.HasPrefix(lower, "warn "):
		return SeverityWarning
	case strings.HasPrefix(lower, "error") || strings.HasPrefix(lower, "failed") || strings.Contains(lower, "failed"):
		return SeverityError
	default:
		return SeverityInfo
	}
}

// writeEntry sends a log entry to the Cloud Logging API
func (c *loggingAPIClient) writeEntry(ctx context.Context, entry LogEntry) {
	if c == nil {
		return
	}

	// Build the Cloud Logging API request body
	payload := map[string]interface{}{
		"entries": []map[string]interface{}{
			{
				"logName":  fmt.Sprintf("projects/%s/logs/%s", c.projectID, c.logName),
				"severity": entry.Severity,
				"jsonPayload": map[string]interface{}{
					"message":   entry.Message,
					"sessionId": entry.SessionID,
					"iteration": entry.Iteration,
				},
				"timestamp": entry.Timestamp,
				"labels":    entry.Labels,
			},
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return
	}

	url := "https://logging.googleapis.com/v2/entries:write"
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(data)))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	// On GCE, the default credentials are available via metadata server
	resp, err := c.client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
}

// FormatEntry formats a LogEntry as a JSON string for testing/debugging
func FormatEntry(entry LogEntry) (string, error) {
	data, err := json.Marshal(entry)
	if err != nil {
		return "", fmt.Errorf("failed to marshal log entry: %w", err)
	}
	return string(data), nil
}
