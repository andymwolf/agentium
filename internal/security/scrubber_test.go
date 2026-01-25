package security

import (
	"regexp"
	"strings"
	"testing"
)

func TestScrubber_Scrub(t *testing.T) {
	scrubber := NewScrubber()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "GitHub personal access token",
			input:    "Use token ghp_1234567890abcdefghijklmnopqrstuvwxyz for auth",
			expected: "Use token ghp_***REDACTED*** for auth",
		},
		{
			name:     "Bearer token",
			input:    "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkw",
			expected: "Authorization: Bearer ***REDACTED***",
		},
		{
			name:     "API key with equals",
			input:    "api_key=sk-1234567890abcdefghijklmnopqrstuvwxyz",
			expected: "api_key=***REDACTED***",
		},
		{
			name:     "Password in configuration",
			input:    `password=supersecretpassword123`,
			expected: `password=***REDACTED***`,
		},
		{
			name:     "AWS access key",
			input:    "aws_access_key_id=AKIAIOSFODNN7EXAMPLE",
			expected: "aws_access_key_id=***REDACTED***",
		},
		{
			name:     "Multiple secrets",
			input:    "api_key=verylongsecretkey12345678901234567890 and password=pass456789",
			expected: "api_key=***REDACTED*** and password=***REDACTED***",
		},
		{
			name:     "SSH private key",
			input:    "-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA...\n-----END RSA PRIVATE KEY-----",
			expected: "-----BEGIN PRIVATE KEY----- ***REDACTED*** -----END PRIVATE KEY-----",
		},
		{
			name:     "JWT token",
			input:    "token=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c",
			expected: "token=eyJh***REDACTED***",
		},
		{
			name:     "No secrets",
			input:    "This is a normal log message without any secrets",
			expected: "This is a normal log message without any secrets",
		},
		{
			name:     "Environment variable format",
			input:    "GITHUB_TOKEN='ghp_abcdefghijklmnopqrstuvwxyz1234567890'",
			expected: "GITHUB_TOKEN='ghp_***REDACTED***'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := scrubber.Scrub(tt.input)
			if result != tt.expected {
				t.Errorf("Scrub() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestScrubber_ScrubSlice(t *testing.T) {
	scrubber := NewScrubber()

	input := []string{
		"normal log line",
		"api_key=secret1234567890abcdefghij",
		"another normal line",
		"password: mysecretpassword",
	}

	expected := []string{
		"normal log line",
		"api_key=***REDACTED***",
		"another normal line",
		"password:***REDACTED***",
	}

	result := scrubber.ScrubSlice(input)

	if len(result) != len(expected) {
		t.Fatalf("ScrubSlice() returned %d items, want %d", len(result), len(expected))
	}

	for i := range result {
		if result[i] != expected[i] {
			t.Errorf("ScrubSlice()[%d] = %v, want %v", i, result[i], expected[i])
		}
	}
}

func TestScrubber_ContainsSensitive(t *testing.T) {
	scrubber := NewScrubber()

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "Contains GitHub token",
			input:    "token is ghp_1234567890abcdefghijklmnopqrstuvwxyz",
			expected: true,
		},
		{
			name:     "Contains password",
			input:    "password=mysecret123",
			expected: true,
		},
		{
			name:     "No sensitive data",
			input:    "This is a normal message",
			expected: false,
		},
		{
			name:     "Contains API key",
			input:    "Using api_key=verylongsecretkey123456789",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := scrubber.ContainsSensitive(tt.input)
			if result != tt.expected {
				t.Errorf("ContainsSensitive() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestScrubber_AddPattern(t *testing.T) {
	scrubber := NewScrubber()

	// Add custom pattern for a specific token format
	customPattern := regexp.MustCompile(`custom_token_[a-z0-9]{16}`)
	scrubber.AddPattern(customPattern)

	input := "Found custom_token_abcdef1234567890 in config"
	result := scrubber.Scrub(input)
	if !strings.Contains(result, "***REDACTED***") {
		t.Errorf("Custom pattern not scrubbed: %v", result)
	}
}

func TestScrubber_EdgeCases(t *testing.T) {
	scrubber := NewScrubber()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "Very short secret",
			input:    "pwd=abc",
			expected: "pwd=abc", // Too short to be considered a real password
		},
		{
			name:     "Secret at start of line",
			input:    "ghp_1234567890abcdefghijklmnopqrstuvwxyz is the token",
			expected: "ghp_***REDACTED*** is the token",
		},
		{
			name:     "Secret at end of line",
			input:    "The token is ghp_1234567890abcdefghijklmnopqrstuvwxyz",
			expected: "The token is ghp_***REDACTED***",
		},
		{
			name:     "Case insensitive API key",
			input:    "API_KEY=secretkey1234567890abcdefgh or api_key=anotherkey1234567890abcdefgh",
			expected: "API_KEY=***REDACTED*** or api_key=***REDACTED***",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := scrubber.Scrub(tt.input)
			if result != tt.expected {
				t.Errorf("Scrub() = %v, want %v", result, tt.expected)
			}
		})
	}
}