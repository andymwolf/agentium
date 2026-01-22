package gcp

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"sync"

	"cloud.google.com/go/logging"
	"google.golang.org/api/option"
)

// Logger defines the interface for logging operations
type Logger interface {
	// Log writes a log entry with the specified severity
	Log(severity logging.Severity, message string, labels map[string]string)
	// Info writes an info-level log entry
	Info(message string)
	// Warn writes a warning-level log entry
	Warn(message string)
	// Error writes an error-level log entry
	Error(message string)
	// Flush ensures all pending log entries are written
	Flush() error
	// Close closes the logger and releases resources
	Close() error
}

// CloudLogger wraps the GCP Cloud Logging client and provides structured logging
type CloudLogger struct {
	client      *logging.Client
	logger      *logging.Logger
	projectID   string
	sessionID   string
	iteration   int
	fallback    *log.Logger
	mu          sync.Mutex
	closed      bool
}

// NewCloudLogger creates a new Cloud Logging client
// It automatically detects the project ID from environment or metadata server
func NewCloudLogger(ctx context.Context, sessionID string, logName string, opts ...option.ClientOption) (*CloudLogger, error) {
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

	// Create logger for specific log name
	logger := client.Logger(logName)

	return &CloudLogger{
		client:    client,
		logger:    logger,
		projectID: projectID,
		sessionID: sessionID,
		iteration: 0,
		fallback:  log.New(os.Stdout, "[cloudlogger] ", log.LstdFlags),
	}, nil
}

// SetIteration updates the current iteration number for log entries
func (cl *CloudLogger) SetIteration(iteration int) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	cl.iteration = iteration
}

// Log writes a structured log entry with the specified severity
func (cl *CloudLogger) Log(severity logging.Severity, message string, labels map[string]string) {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	if cl.closed {
		cl.fallback.Printf("[%s] %s (logger closed)", severity, message)
		return
	}

	// Build common labels
	commonLabels := map[string]string{
		"session_id": cl.sessionID,
		"iteration":  fmt.Sprintf("%d", cl.iteration),
		"project_id": cl.projectID,
	}

	// Merge with custom labels
	if labels != nil {
		for k, v := range labels {
			commonLabels[k] = v
		}
	}

	// Create log entry
	entry := logging.Entry{
		Severity: severity,
		Payload:  message,
		Labels:   commonLabels,
	}

	// Log asynchronously (non-blocking)
	cl.logger.Log(entry)
}

// Info writes an info-level log entry
func (cl *CloudLogger) Info(message string) {
	cl.Log(logging.Info, message, nil)
}

// Warn writes a warning-level log entry
func (cl *CloudLogger) Warn(message string) {
	cl.Log(logging.Warning, message, nil)
}

// Error writes an error-level log entry
func (cl *CloudLogger) Error(message string) {
	cl.Log(logging.Error, message, nil)
}

// Flush ensures all pending log entries are written to Cloud Logging
// This should be called before VM termination to ensure logs persist
func (cl *CloudLogger) Flush() error {
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
func (cl *CloudLogger) Close() error {
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

// StdLogger wraps a standard library logger to implement the Logger interface
// This is used as a fallback when Cloud Logging is not available
type StdLogger struct {
	logger *log.Logger
}

// NewStdLogger creates a new standard library logger wrapper
func NewStdLogger(out io.Writer, prefix string) *StdLogger {
	return &StdLogger{
		logger: log.New(out, prefix, log.LstdFlags),
	}
}

// Log writes a log entry with the specified severity
func (sl *StdLogger) Log(severity logging.Severity, message string, labels map[string]string) {
	labelStr := ""
	if labels != nil && len(labels) > 0 {
		labelStr = fmt.Sprintf(" %v", labels)
	}
	sl.logger.Printf("[%s] %s%s", severity, message, labelStr)
}

// Info writes an info-level log entry
func (sl *StdLogger) Info(message string) {
	sl.Log(logging.Info, message, nil)
}

// Warn writes a warning-level log entry
func (sl *StdLogger) Warn(message string) {
	sl.Log(logging.Warning, message, nil)
}

// Error writes an error-level log entry
func (sl *StdLogger) Error(message string) {
	sl.Log(logging.Error, message, nil)
}

// Flush is a no-op for standard logger
func (sl *StdLogger) Flush() error {
	return nil
}

// Close is a no-op for standard logger
func (sl *StdLogger) Close() error {
	return nil
}
