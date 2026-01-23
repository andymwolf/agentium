package gcp

import (
	"context"
	"fmt"
	"sync"
	"time"

	"cloud.google.com/go/logging"
	"google.golang.org/api/option"
)

// LogSeverity represents the severity of a log entry
type LogSeverity int

const (
	SeverityDefault  LogSeverity = iota
	SeverityDebug    LogSeverity = 100
	SeverityInfo     LogSeverity = 200
	SeverityWarning  LogSeverity = 400
	SeverityError    LogSeverity = 500
	SeverityCritical LogSeverity = 600
)

// String returns the string representation of the log severity
func (s LogSeverity) String() string {
	switch s {
	case SeverityDebug:
		return "DEBUG"
	case SeverityInfo:
		return "INFO"
	case SeverityWarning:
		return "WARNING"
	case SeverityError:
		return "ERROR"
	case SeverityCritical:
		return "CRITICAL"
	default:
		return "DEFAULT"
	}
}

// toCloudSeverity converts our LogSeverity to Cloud Logging's Severity type
func (s LogSeverity) toCloudSeverity() logging.Severity {
	switch s {
	case SeverityDebug:
		return logging.Debug
	case SeverityInfo:
		return logging.Info
	case SeverityWarning:
		return logging.Warning
	case SeverityError:
		return logging.Error
	case SeverityCritical:
		return logging.Critical
	default:
		return logging.Default
	}
}

// LogEntry represents a structured log entry to be sent to Cloud Logging
type LogEntry struct {
	Message   string            `json:"message"`
	Severity  LogSeverity       `json:"severity"`
	Timestamp time.Time         `json:"timestamp"`
	SessionID string            `json:"session_id"`
	Iteration int               `json:"iteration"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// CloudLogger defines the interface for cloud logging operations
type CloudLogger interface {
	// Log sends a structured log entry to Cloud Logging
	Log(entry LogEntry)
	// Flush flushes any buffered log entries with a timeout
	Flush(timeout time.Duration) error
	// Close closes the logger and releases resources
	Close() error
}

// CloudLoggingClient implements CloudLogger using GCP Cloud Logging
type CloudLoggingClient struct {
	client    *logging.Client
	logger    *logging.Logger
	projectID string
	logName   string
	mu        sync.Mutex
}

// DefaultLogName is the default log name for Agentium sessions
const DefaultLogName = "agentium-sessions"

// NewCloudLoggingClient creates a new Cloud Logging client
func NewCloudLoggingClient(ctx context.Context, projectID, logName string, opts ...option.ClientOption) (*CloudLoggingClient, error) {
	if projectID == "" {
		// Try to detect project ID from environment or metadata
		var err error
		projectID, err = getProjectID(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to determine project ID: %w", err)
		}
	}

	if logName == "" {
		logName = DefaultLogName
	}

	client, err := logging.NewClient(ctx, "projects/"+projectID, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create cloud logging client: %w", err)
	}

	logger := client.Logger(logName)

	return &CloudLoggingClient{
		client:    client,
		logger:    logger,
		projectID: projectID,
		logName:   logName,
	}, nil
}

// Log sends a structured log entry to Cloud Logging
func (c *CloudLoggingClient) Log(entry LogEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.logger == nil {
		return
	}

	// Build structured payload
	payload := map[string]interface{}{
		"message":    entry.Message,
		"session_id": entry.SessionID,
		"iteration":  entry.Iteration,
	}

	// Add custom labels to payload
	if len(entry.Labels) > 0 {
		payload["labels"] = entry.Labels
	}

	// Set timestamp if not provided
	ts := entry.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}

	c.logger.Log(logging.Entry{
		Severity:  entry.Severity.toCloudSeverity(),
		Timestamp: ts,
		Payload:   payload,
		Labels: map[string]string{
			"session_id": entry.SessionID,
			"component":  "controller",
		},
	})
}

// Flush flushes any buffered log entries, waiting up to the specified timeout
func (c *CloudLoggingClient) Flush(timeout time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.logger == nil {
		return nil
	}

	// Create a context with timeout for the flush operation
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Flush in a goroutine so we can respect the timeout
	done := make(chan error, 1)
	go func() {
		done <- c.logger.Flush()
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return fmt.Errorf("flush timed out after %s", timeout)
	}
}

// Close closes the Cloud Logging client and releases resources
func (c *CloudLoggingClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client == nil {
		return nil
	}

	err := c.client.Close()
	c.client = nil
	c.logger = nil
	return err
}

// FormatLogEntry creates a LogEntry with common fields populated
func FormatLogEntry(sessionID string, iteration int, severity LogSeverity, message string, labels map[string]string) LogEntry {
	return LogEntry{
		Message:   message,
		Severity:  severity,
		Timestamp: time.Now(),
		SessionID: sessionID,
		Iteration: iteration,
		Labels:    labels,
	}
}
