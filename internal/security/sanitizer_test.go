package security

import (
	"errors"
	"testing"
)

func TestLogSanitizer_Sanitize(t *testing.T) {
	ls := NewLogSanitizer()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "GitHub personal access token",
			input:    "Using token ghp_1234567890abcdef1234567890abcdef1234",
			expected: "Using token [REDACTED-GITHUB-TOKEN]",
		},
		{
			name:     "GitHub App token",
			input:    "Token: ghs_abcdefghijklmnopqrstuvwxyz1234567890",
			expected: "Token: [REDACTED-GITHUB-TOKEN]",
		},
		{
			name:     "API key in config",
			input:    "api_key = 'sk-1234567890abcdef'",
			expected: "api_key=[REDACTED]",
		},
		{
			name:     "Bearer token",
			input:    "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
			expected: "Authorization: Bearer [REDACTED]",
		},
		{
			name:     "JWT token",
			input:    "token: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c",
			expected: "token: [REDACTED-JWT]",
		},
		{
			name:     "URL with password",
			input:    "Connecting to https://user:secretpassword@example.com",
			expected: "Connecting to https://[REDACTED]@example.com",
		},
		{
			name:     "Private key",
			input:    "key: -----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQ...\n-----END RSA PRIVATE KEY-----",
			expected: "key: [REDACTED-PRIVATE-KEY]",
		},
		{
			name:     "GCP service account",
			input:    `{"private_key": "-----BEGIN PRIVATE KEY-----\nMIIE...", "client_email": "test@project.iam.gserviceaccount.com"}`,
			expected: `{[REDACTED-GCP-CREDENTIALS], [REDACTED-GCP-CREDENTIALS]}`,
		},
		{
			name:     "AWS credentials",
			input:    "aws_access_key_id=AKIAIOSFODNN7EXAMPLE",
			expected: "aws_access_key_id=[REDACTED]",
		},
		{
			name:     "Base64 in auth context",
			input:    "auth_token: YWxhZGRpbjpvcGVuc2VzYW1lYWxhZGRpbjpvcGVuc2VzYW1l",
			expected: "auth_token=[REDACTED-BASE64]",
		},
		{
			name:     "Multiple secrets",
			input:    "api_key='secret123' and token=ghp_abc123def456ghi789jkl012mno345pqr678",
			expected: "api_key=[REDACTED] and token=[REDACTED-GITHUB-TOKEN]",
		},
		{
			name:     "No secrets",
			input:    "This is a normal log message without any secrets",
			expected: "This is a normal log message without any secrets",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ls.Sanitize(tt.input)
			if result != tt.expected {
				t.Errorf("Sanitize() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestLogSanitizer_SanitizeError(t *testing.T) {
	ls := NewLogSanitizer()

	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: "",
		},
		{
			name:     "error with token",
			err:      errors.New("authentication failed: invalid token ghp_1234567890abcdef1234567890abcdef1234"),
			expected: "authentication failed: invalid token [REDACTED-GITHUB-TOKEN]",
		},
		{
			name:     "normal error",
			err:      errors.New("file not found"),
			expected: "file not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ls.SanitizeError(tt.err)
			if result != tt.expected {
				t.Errorf("SanitizeError() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestLogSanitizer_SanitizeMap(t *testing.T) {
	ls := NewLogSanitizer()

	input := map[string]string{
		"user":         "john",
		"api_key":      "secret123456",
		"password":     "mysecretpass",
		"debug":        "true",
		"auth_token":   "Bearer abc123",
		"normal_value": "hello world",
	}

	result := ls.SanitizeMap(input)

	// Check that sensitive values are redacted
	if result["api_key"] != "[REDACTED]" {
		t.Errorf("Expected api_key to be [REDACTED], got %s", result["api_key"])
	}
	if result["password"] != "[REDACTED]" {
		t.Errorf("Expected password to be [REDACTED], got %s", result["password"])
	}
	if result["auth_token"] != "[REDACTED]" {
		t.Errorf("Expected auth_token to be [REDACTED], got %s", result["auth_token"])
	}

	// Check that normal values are preserved
	if result["user"] != "john" {
		t.Errorf("Expected user to be john, got %s", result["user"])
	}
	if result["debug"] != "true" {
		t.Errorf("Expected debug to be true, got %s", result["debug"])
	}
	if result["normal_value"] != "hello world" {
		t.Errorf("Expected normal_value to be 'hello world', got %s", result["normal_value"])
	}
}

func TestPathSanitizer_Sanitize(t *testing.T) {
	ps := NewPathSanitizer()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Linux home directory",
			input:    "/home/username/project/file.go",
			expected: "[HOME]/project/file.go",
		},
		{
			name:     "macOS home directory",
			input:    "/Users/johndoe/Documents/code.py",
			expected: "[HOME]/Documents/code.py",
		},
		{
			name:     "Tilde expansion",
			input:    "~/workspace/agentium",
			expected: "[HOME]/workspace/agentium",
		},
		{
			name:     "Temp directory with session",
			input:    "/tmp/agentium/session-abc123/config.json",
			expected: "/tmp/agentium/[SESSION-ID]/config.json",
		},
		{
			name:     "No sensitive paths",
			input:    "/opt/app/bin/executable",
			expected: "/opt/app/bin/executable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ps.Sanitize(tt.input)
			if result != tt.expected {
				t.Errorf("Sanitize() = %q, want %q", result, tt.expected)
			}
		})
	}
}