package gcp

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"cloud.google.com/go/logging"
	"google.golang.org/api/option"
)

// Severity levels for structured logging
type Severity string

const (
	SeverityDebug    Severity = "DEBUG"
	SeverityInfo     Severity = "INFO"
	SeverityWarning  Severity = "WARNING"
	SeverityError    Severity = "ERROR"
	SeverityCritical Severity = "CRITICAL"
)

// LogEntry represents a structured log entry with session metadata
type LogEntry struct {
	Timestamp time.Time         `json:"timestamp"`
	Severity  Severity          `json:"severity"`
	Message   string            `json:"message"`
	SessionID string            `json:"session_id"`
	Iteration int               `json:"iteration"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// CloudLoggerConfig holds configuration for the CloudLogger
type CloudLoggerConfig struct {
	ProjectID  string
	LogName    string
	SessionID  string
	Repository string
}

// LogWriter defines the interface for writing log entries to a backend.
// This allows for mocking in tests.
type LogWriter interface {
	Log(entry logging.Entry)
	Flush() error
	Close() error
}

// gcpLogWriter wraps the real GCP logging.Logger
type gcpLogWriter struct {
	logger *logging.Logger
	client *logging.Client
}

func (w *gcpLogWriter) Log(entry logging.Entry) {
	w.logger.Log(entry)
}

func (w *gcpLogWriter) Flush() error {
	return w.logger.Flush()
}

func (w *gcpLogWriter) Close() error {
	return w.client.Close()
}

// CloudLogger provides structured logging to GCP Cloud Logging.
// It includes session metadata (session ID, iteration, timestamps) with each log entry
// and supports flushing to ensure logs survive VM termination.
type CloudLogger struct {
	writer    LogWriter
	sessionID string
	iteration atomic.Int64
	labels    map[string]string // immutable after construction; do not modify
}

// NewCloudLogger creates a new CloudLogger that sends logs to GCP Cloud Logging.
// If the Cloud Logging client cannot be created (e.g., not running on GCP),
// it returns an error. Callers should fall back to local logging in that case.
func NewCloudLogger(ctx context.Context, cfg CloudLoggerConfig, opts ...option.ClientOption) (*CloudLogger, error) {
	if cfg.ProjectID == "" {
		// Try to get project ID from environment/metadata
		projectID, err := getProjectID(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to determine project ID: %w", err)
		}
		cfg.ProjectID = projectID
	}

	if cfg.LogName == "" {
		cfg.LogName = "agentium-session"
	}

	client, err := logging.NewClient(ctx, cfg.ProjectID, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create logging client: %w", err)
	}

	logger := client.Logger(cfg.LogName)

	labels := map[string]string{
		"session_id": cfg.SessionID,
	}
	if cfg.Repository != "" {
		labels["repository"] = cfg.Repository
	}

	return &CloudLogger{
		writer:    &gcpLogWriter{logger: logger, client: client},
		sessionID: cfg.SessionID,
		labels:    labels,
	}, nil
}

// NewCloudLoggerWithWriter creates a CloudLogger with a custom LogWriter.
// This is primarily used for testing with mock writers.
func NewCloudLoggerWithWriter(writer LogWriter, sessionID string, labels map[string]string) *CloudLogger {
	if labels == nil {
		labels = map[string]string{}
	}
	labels["session_id"] = sessionID

	return &CloudLogger{
		writer:    writer,
		sessionID: sessionID,
		labels:    labels,
	}
}

// SetIteration updates the current iteration number for subsequent log entries.
// Note: SetIteration and log calls are not atomic as a pair. In concurrent use,
// another goroutine may call SetIteration between this call and a subsequent log call.
func (cl *CloudLogger) SetIteration(iteration int) {
	cl.iteration.Store(int64(iteration))
}

// Info logs a message at INFO severity
func (cl *CloudLogger) Info(msg string) {
	cl.log(SeverityInfo, msg, nil)
}

// Infof logs a formatted message at INFO severity
func (cl *CloudLogger) Infof(format string, args ...interface{}) {
	cl.log(SeverityInfo, fmt.Sprintf(format, args...), nil)
}

// Warning logs a message at WARNING severity
func (cl *CloudLogger) Warning(msg string) {
	cl.log(SeverityWarning, msg, nil)
}

// Warningf logs a formatted message at WARNING severity
func (cl *CloudLogger) Warningf(format string, args ...interface{}) {
	cl.log(SeverityWarning, fmt.Sprintf(format, args...), nil)
}

// Error logs a message at ERROR severity
func (cl *CloudLogger) Error(msg string) {
	cl.log(SeverityError, msg, nil)
}

// Errorf logs a formatted message at ERROR severity
func (cl *CloudLogger) Errorf(format string, args ...interface{}) {
	cl.log(SeverityError, fmt.Sprintf(format, args...), nil)
}

// LogWithLabels logs a message with additional custom labels
func (cl *CloudLogger) LogWithLabels(severity Severity, msg string, extraLabels map[string]string) {
	cl.log(severity, msg, extraLabels)
}

// log writes a structured log entry to Cloud Logging
func (cl *CloudLogger) log(severity Severity, msg string, extraLabels map[string]string) {
	iteration := int(cl.iteration.Load())

	// Build the structured payload
	payload := LogEntry{
		Timestamp: time.Now().UTC(),
		Severity:  severity,
		Message:   msg,
		SessionID: cl.sessionID,
		Iteration: iteration,
	}

	// Merge labels
	labels := make(map[string]string, len(cl.labels)+len(extraLabels))
	for k, v := range cl.labels {
		labels[k] = v
	}
	for k, v := range extraLabels {
		labels[k] = v
	}
	payload.Labels = labels

	// Map severity to Cloud Logging severity
	var gcpSeverity logging.Severity
	switch severity {
	case SeverityDebug:
		gcpSeverity = logging.Debug
	case SeverityInfo:
		gcpSeverity = logging.Info
	case SeverityWarning:
		gcpSeverity = logging.Warning
	case SeverityError:
		gcpSeverity = logging.Error
	case SeverityCritical:
		gcpSeverity = logging.Critical
	default:
		gcpSeverity = logging.Default
	}

	entry := logging.Entry{
		Timestamp: payload.Timestamp,
		Severity:  gcpSeverity,
		Payload:   payload,
		Labels:    labels,
	}

	cl.writer.Log(entry)
}

// Flush ensures all buffered log entries are sent to Cloud Logging.
// This should be called before VM termination to ensure logs survive.
func (cl *CloudLogger) Flush() error {
	return cl.writer.Flush()
}

// Close flushes remaining logs and closes the Cloud Logging client.
func (cl *CloudLogger) Close() error {
	if err := cl.writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush logs: %w", err)
	}
	return cl.writer.Close()
}
