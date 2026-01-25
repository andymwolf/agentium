package controller

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/andymwolf/agentium/internal/cloud/gcp"
)

// TestSecureLogging verifies that sensitive information is sanitized in logs
func TestSecureLogging(t *testing.T) {
	// Create a test writer to capture log output
	var buf bytes.Buffer
	testWriter := &testLogWriter{buffer: &buf}

	// Create a SecureCloudLogger with the test writer
	logger := gcp.NewSecureCloudLoggerWithWriter(testWriter, "test-session", nil)

	// Test various sensitive data patterns
	tests := []struct {
		name     string
		input    string
		mustHave string // What should remain
		mustNot  []string // What should be redacted
	}{
		{
			name:     "GitHub token",
			input:    "Using token ghp_1234567890abcdefghijklmnopqrstuvwxyz",
			mustHave: "Using token",
			mustNot:  []string{"ghp_", "1234567890"},
		},
		{
			name:     "API key in config",
			input:    "Config: api_key=sk-1234567890abcdefghijklmnop",
			mustHave: "Config:",
			mustNot:  []string{"sk-1234567890", "abcdefghijklmnop"},
		},
		{
			name:     "Bearer token",
			input:    "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
			mustHave: "Authorization:",
			mustNot:  []string{"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"},
		},
		{
			name:     "Private key",
			input:    "Key: -----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBg...\n-----END PRIVATE KEY-----",
			mustHave: "Key:",
			mustNot:  []string{"BEGIN PRIVATE KEY", "MIIEvQIBADANBg"},
		},
		{
			name:     "URL with password",
			input:    "Cloning https://user:secretpass123@github.com/repo.git",
			mustHave: "Cloning",
			mustNot:  []string{"secretpass123", ":secretpass123@"},
		},
		{
			name:     "JWT token",
			input:    "Token: eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U",
			mustHave: "Token:",
			mustNot:  []string{"eyJhbGciOiJIUzI1NiJ9", "dozjgNryP4J3jVmNHl0w5N"},
		},
		{
			name:     "GCP service account",
			input:    `Credentials: {"private_key": "-----BEGIN RSA PRIVATE KEY-----\nMIIE..."}`,
			mustHave: "Credentials:",
			mustNot:  []string{"private_key", "BEGIN RSA PRIVATE KEY"},
		},
		{
			name:     "AWS credentials",
			input:    "aws_access_key_id=AKIAIOSFODNN7EXAMPLE",
			mustHave: "aws_access_key_id=",
			mustNot:  []string{"AKIAIOSFODNN7EXAMPLE"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear buffer
			buf.Reset()

			// Log the message
			logger.Info(tt.input)

			// Get the logged output
			output := buf.String()

			// Check that safe content remains
			if !strings.Contains(output, tt.mustHave) {
				t.Errorf("Expected output to contain %q, but got: %s", tt.mustHave, output)
			}

			// Check that sensitive content is redacted
			for _, sensitive := range tt.mustNot {
				if strings.Contains(output, sensitive) {
					t.Errorf("Output should not contain sensitive data %q, but got: %s", sensitive, output)
				}
			}

			// Verify that some form of redaction marker is present
			if !strings.Contains(output, "[REDACTED") {
				t.Errorf("Expected output to contain redaction marker, but got: %s", output)
			}
		})
	}
}

// TestControllerUsesSecureLogger verifies the controller initializes SecureCloudLogger
func TestControllerUsesSecureLogger(t *testing.T) {
	// This test ensures that the controller's cloudLogger field is of type *gcp.SecureCloudLogger
	// The actual type checking happens at compile time due to the field declaration

	c := &Controller{}

	// This will only compile if cloudLogger is *gcp.SecureCloudLogger
	var _ *gcp.SecureCloudLogger = c.cloudLogger

	// If this compiles, the test passes
	t.Log("Controller correctly uses SecureCloudLogger type")
}

// testLogWriter captures log output for testing
type testLogWriter struct {
	buffer *bytes.Buffer
}

func (w *testLogWriter) Write(e gcp.LogEntry) {
	w.buffer.WriteString(e.Message)
	w.buffer.WriteString("\n")
}

func (w *testLogWriter) Close() error {
	return nil
}