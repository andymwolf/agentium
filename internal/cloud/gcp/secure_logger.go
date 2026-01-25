package gcp

import (
	"context"
	"fmt"

	"github.com/andymwolf/agentium/internal/security"
	"google.golang.org/api/option"
)

// SecureCloudLogger wraps CloudLogger with automatic log sanitization
type SecureCloudLogger struct {
	*CloudLogger
	sanitizer     *security.LogSanitizer
	pathSanitizer *security.PathSanitizer
}

// NewSecureCloudLogger creates a CloudLogger with automatic sanitization
func NewSecureCloudLogger(ctx context.Context, cfg CloudLoggerConfig, opts ...option.ClientOption) (*SecureCloudLogger, error) {
	cloudLogger, err := NewCloudLogger(ctx, cfg, opts...)
	if err != nil {
		return nil, err
	}

	return &SecureCloudLogger{
		CloudLogger:   cloudLogger,
		sanitizer:     security.NewLogSanitizer(),
		pathSanitizer: security.NewPathSanitizer(),
	}, nil
}

// NewSecureCloudLoggerWithWriter creates a SecureCloudLogger with a custom writer (for testing)
func NewSecureCloudLoggerWithWriter(writer LogWriter, sessionID string, labels map[string]string) *SecureCloudLogger {
	return &SecureCloudLogger{
		CloudLogger:   NewCloudLoggerWithWriter(writer, sessionID, labels),
		sanitizer:     security.NewLogSanitizer(),
		pathSanitizer: security.NewPathSanitizer(),
	}
}

// Override all logging methods to add sanitization

// Debug logs a sanitized message at DEBUG severity
func (scl *SecureCloudLogger) Debug(msg string) {
	sanitized := scl.sanitizer.Sanitize(msg)
	scl.CloudLogger.Debug(sanitized)
}

// Debugf logs a sanitized formatted message at DEBUG severity
func (scl *SecureCloudLogger) Debugf(format string, args ...interface{}) {
	// Format first, then sanitize to catch secrets in interpolated values
	msg := fmt.Sprintf(format, args...)
	sanitized := scl.sanitizer.Sanitize(msg)
	scl.CloudLogger.Debug(sanitized)
}

// Info logs a sanitized message at INFO severity
func (scl *SecureCloudLogger) Info(msg string) {
	sanitized := scl.sanitizer.Sanitize(msg)
	scl.CloudLogger.Info(sanitized)
}

// Infof logs a sanitized formatted message at INFO severity
func (scl *SecureCloudLogger) Infof(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	sanitized := scl.sanitizer.Sanitize(msg)
	scl.CloudLogger.Info(sanitized)
}

// Warning logs a sanitized message at WARNING severity
func (scl *SecureCloudLogger) Warning(msg string) {
	sanitized := scl.sanitizer.Sanitize(msg)
	scl.CloudLogger.Warning(sanitized)
}

// Warningf logs a sanitized formatted message at WARNING severity
func (scl *SecureCloudLogger) Warningf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	sanitized := scl.sanitizer.Sanitize(msg)
	scl.CloudLogger.Warning(sanitized)
}

// Error logs a sanitized message at ERROR severity
func (scl *SecureCloudLogger) Error(msg string) {
	sanitized := scl.sanitizer.Sanitize(msg)
	scl.CloudLogger.Error(sanitized)
}

// Errorf logs a sanitized formatted message at ERROR severity
func (scl *SecureCloudLogger) Errorf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	sanitized := scl.sanitizer.Sanitize(msg)
	scl.CloudLogger.Error(sanitized)
}

// LogWithLabels logs a sanitized message with sanitized labels
func (scl *SecureCloudLogger) LogWithLabels(severity Severity, msg string, extraLabels map[string]string) {
	sanitizedMsg := scl.sanitizer.Sanitize(msg)
	sanitizedLabels := scl.sanitizer.SanitizeMap(extraLabels)
	scl.CloudLogger.LogWithLabels(severity, sanitizedMsg, sanitizedLabels)
}

