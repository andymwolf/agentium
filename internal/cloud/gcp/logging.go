package gcp

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"cloud.google.com/go/logging"
	"google.golang.org/api/option"
)

// CloudLogger defines the interface for structured cloud logging operations
type CloudLogger interface {
	// Log writes a log entry with the specified severity and optional labels
	Log(severity Severity, message string, labels map[string]string)
	// Info writes an info-level log entry
	Info(message string)
	// Warn writes a warning-level log entry
	Warn(message string)
	// Error writes an error-level log entry
	Error(message string)
	// SetIteration updates the current iteration number for log entries
	SetIteration(iteration int)
	// Flush ensures all pending log entries are written
	Flush() error
	// Close closes the logger and releases resources
	Close() error
}

// Severity represents log severity levels
type Severity int

const (
	SeverityDefault  Severity = iota
	SeverityDebug    Severity = 100
	SeverityInfo     Severity = 200
	SeverityWarning  Severity = 400
	SeverityError    Severity = 500
	SeverityCritical Severity = 600
)

// String returns the string representation of a severity level
func (s Severity) String() string {
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

// toGCPSeverity converts our Severity to GCP logging.Severity
func (s Severity) toGCPSeverity() logging.Severity {
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

// CloudLoggingClient wraps the GCP Cloud Logging client and provides structured logging
type CloudLoggingClient struct {
	client    *logging.Client
	logger    *logging.Logger
	projectID string
	sessionID string
	iteration int
	fallback  *log.Logger
	mu        sync.Mutex
	closed    bool
}

// NewCloudLoggingClient creates a new Cloud Logging client.
// It automatically detects the project ID from environment or metadata server.
// The logName parameter specifies the Cloud Logging log name (e.g., "agentium-sessions").
func NewCloudLoggingClient(ctx context.Context, sessionID string, logName string, opts ...option.ClientOption) (*CloudLoggingClient, error) {
	// Get the project ID
	projectID, err := getProjectID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get project ID: %w", err)
	}

	// Create logging client
	client, err := logging.NewClient(ctx, projectID, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create logging client: %w", err)
	}

	// Create logger for specific log name with common resource labels
	logger := client.Logger(logName, logging.CommonLabels(map[string]string{
		"session_id": sessionID,
	}))

	return &CloudLoggingClient{
		client:    client,
		logger:    logger,
		projectID: projectID,
		sessionID: sessionID,
		iteration: 0,
		fallback:  log.New(os.Stdout, "[cloudlogger] ", log.LstdFlags),
	}, nil
}

// SetIteration updates the current iteration number for log entries
func (cl *CloudLoggingClient) SetIteration(iteration int) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	cl.iteration = iteration
}

// Log writes a structured log entry with the specified severity
func (cl *CloudLoggingClient) Log(severity Severity, message string, labels map[string]string) {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	if cl.closed {
		cl.fallback.Printf("[%s] %s (logger closed)", severity, message)
		return
	}

	// Build entry labels including session context
	entryLabels := map[string]string{
		"session_id": cl.sessionID,
		"iteration":  fmt.Sprintf("%d", cl.iteration),
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	}

	// Merge with custom labels
	if labels != nil {
		for k, v := range labels {
			entryLabels[k] = v
		}
	}

	// Create log entry
	entry := logging.Entry{
		Severity: severity.toGCPSeverity(),
		Payload:  message,
		Labels:   entryLabels,
	}

	// Log asynchronously (non-blocking, buffered by GCP client)
	cl.logger.Log(entry)
}

// Info writes an info-level log entry
func (cl *CloudLoggingClient) Info(message string) {
	cl.Log(SeverityInfo, message, nil)
}

// Warn writes a warning-level log entry
func (cl *CloudLoggingClient) Warn(message string) {
	cl.Log(SeverityWarning, message, nil)
}

// Error writes an error-level log entry
func (cl *CloudLoggingClient) Error(message string) {
	cl.Log(SeverityError, message, nil)
}

// Flush ensures all pending log entries are written to Cloud Logging.
// This should be called before VM termination to ensure logs survive.
func (cl *CloudLoggingClient) Flush() error {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	if cl.closed {
		return nil
	}

	if err := cl.logger.Flush(); err != nil {
		return fmt.Errorf("failed to flush logs: %w", err)
	}

	return nil
}

// Close flushes pending logs and closes the logger
func (cl *CloudLoggingClient) Close() error {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	if cl.closed {
		return nil
	}

	cl.closed = true

	// Flush any pending logs
	if err := cl.logger.Flush(); err != nil {
		cl.fallback.Printf("Warning: failed to flush logs on close: %v", err)
	}

	// Close the client
	if err := cl.client.Close(); err != nil {
		return fmt.Errorf("failed to close logging client: %w", err)
	}

	return nil
}

// FormatLogEntry creates a formatted log message with structured context.
// This is a utility function for creating consistent log messages.
func FormatLogEntry(sessionID string, iteration int, event string, details map[string]string) string {
	msg := fmt.Sprintf("[session=%s iter=%d] %s", sessionID, iteration, event)
	if len(details) > 0 {
		msg += " {"
		first := true
		for k, v := range details {
			if !first {
				msg += ", "
			}
			msg += fmt.Sprintf("%s=%q", k, v)
			first = false
		}
		msg += "}"
	}
	return msg
}

// StdLogger wraps a standard library logger to implement the CloudLogger interface.
// This is used as a fallback when Cloud Logging is not available (e.g., local development).
type StdLogger struct {
	logger    *log.Logger
	sessionID string
	iteration int
	mu        sync.Mutex
}

// NewStdLogger creates a new standard library logger wrapper
func NewStdLogger(out io.Writer, prefix string, sessionID string) *StdLogger {
	return &StdLogger{
		logger:    log.New(out, prefix, log.LstdFlags),
		sessionID: sessionID,
	}
}

// SetIteration updates the current iteration number for log entries
func (sl *StdLogger) SetIteration(iteration int) {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	sl.iteration = iteration
}

// Log writes a log entry with the specified severity
func (sl *StdLogger) Log(severity Severity, message string, labels map[string]string) {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	labelStr := ""
	if labels != nil && len(labels) > 0 {
		labelStr = fmt.Sprintf(" %v", labels)
	}
	sl.logger.Printf("[%s] [session=%s iter=%d] %s%s", severity, sl.sessionID, sl.iteration, message, labelStr)
}

// Info writes an info-level log entry
func (sl *StdLogger) Info(message string) {
	sl.Log(SeverityInfo, message, nil)
}

// Warn writes a warning-level log entry
func (sl *StdLogger) Warn(message string) {
	sl.Log(SeverityWarning, message, nil)
}

// Error writes an error-level log entry
func (sl *StdLogger) Error(message string) {
	sl.Log(SeverityError, message, nil)
}

// Flush is a no-op for standard logger
func (sl *StdLogger) Flush() error {
	return nil
}

// Close is a no-op for standard logger
func (sl *StdLogger) Close() error {
	return nil
}
