// Package security provides security utilities for the Agentium project
package security

import (
	"regexp"
	"strings"
)

// Common patterns for sensitive data
var (
	// GitHub tokens
	githubTokenPattern = regexp.MustCompile(`(gh[ps]_[a-zA-Z0-9]{36}|github_pat_[a-zA-Z0-9]{22}_[a-zA-Z0-9]{59})`)

	// Generic API keys
	apiKeyPattern = regexp.MustCompile(`(?i)(api[_-]?key|apikey|api[_-]?secret|api[_-]?token)[[:space:]]*[:=][[:space:]]*['"` + "`" + `]?([a-zA-Z0-9_\-]{16,})`)

	// Bearer tokens
	bearerTokenPattern = regexp.MustCompile(`(?i)bearer[[:space:]]+([a-zA-Z0-9_\-\.]+)`)

	// Base64 encoded secrets (minimum 20 chars)
	base64SecretPattern = regexp.MustCompile(`(?:[A-Za-z0-9+/]{20,}={0,2})`)

	// Private keys
	privateKeyPattern = regexp.MustCompile(`(?s)-----BEGIN[[:space:]]+(?:RSA[[:space:]]+)?PRIVATE[[:space:]]+KEY-----.*?-----END[[:space:]]+(?:RSA[[:space:]]+)?PRIVATE[[:space:]]+KEY-----`)

	// Passwords in URLs
	urlPasswordPattern = regexp.MustCompile(`(?i)(https?|ftp)://[^:]+:([^@]+)@`)

	// JSON Web Tokens
	jwtPattern = regexp.MustCompile(`eyJ[a-zA-Z0-9_-]*\.eyJ[a-zA-Z0-9_-]*\.[a-zA-Z0-9_-]*`)

	// Cloud provider patterns
	gcpServiceAccountPattern = regexp.MustCompile(`"private_key":\s*"[^"]+"|"client_email":\s*"[^"]+@[^"]+\.iam\.gserviceaccount\.com"`)
	awsAccessKeyPattern     = regexp.MustCompile(`(?i)(aws[_-]?access[_-]?key[_-]?id|aws[_-]?secret[_-]?access[_-]?key)[[:space:]]*[:=][[:space:]]*['"` + "`" + `]?([a-zA-Z0-9/+=]{16,})`)
)

// LogSanitizer provides methods for sanitizing logs
type LogSanitizer struct {
	customPatterns []*regexp.Regexp
}

// NewLogSanitizer creates a new log sanitizer
func NewLogSanitizer() *LogSanitizer {
	return &LogSanitizer{
		customPatterns: make([]*regexp.Regexp, 0),
	}
}

// AddCustomPattern adds a custom pattern to sanitize
func (ls *LogSanitizer) AddCustomPattern(pattern *regexp.Regexp) {
	ls.customPatterns = append(ls.customPatterns, pattern)
}

// Sanitize removes or masks sensitive information from log messages
func (ls *LogSanitizer) Sanitize(message string) string {
	// Replace GitHub tokens
	message = githubTokenPattern.ReplaceAllString(message, "[REDACTED-GITHUB-TOKEN]")

	// Replace API keys
	message = apiKeyPattern.ReplaceAllString(message, "${1}=[REDACTED]")

	// Replace bearer tokens
	message = bearerTokenPattern.ReplaceAllString(message, "Bearer [REDACTED]")

	// Replace private keys
	message = privateKeyPattern.ReplaceAllString(message, "[REDACTED-PRIVATE-KEY]")

	// Replace passwords in URLs
	message = urlPasswordPattern.ReplaceAllString(message, "${1}://[REDACTED]@")

	// Replace JWTs
	message = jwtPattern.ReplaceAllString(message, "[REDACTED-JWT]")

	// Replace GCP service account info
	message = gcpServiceAccountPattern.ReplaceAllString(message, "[REDACTED-GCP-CREDENTIALS]")

	// Replace AWS credentials
	message = awsAccessKeyPattern.ReplaceAllString(message, "${1}=[REDACTED]")

	// Apply custom patterns
	for _, pattern := range ls.customPatterns {
		message = pattern.ReplaceAllString(message, "[REDACTED]")
	}

	// Sanitize potential base64 encoded secrets (only in specific contexts)
	message = sanitizeBase64InContext(message)

	return message
}

// sanitizeBase64InContext only redacts base64 that appears to be secrets
func sanitizeBase64InContext(message string) string {
	// Look for base64 in specific contexts (after = or : in config/auth contexts)
	contextPattern := regexp.MustCompile(`(?i)(auth|token|key|secret|password|credential)[^=:]*[:=]\s*["'` + "`" + `]?([A-Za-z0-9+/]{20,}={0,2})`)
	return contextPattern.ReplaceAllString(message, "${1}=[REDACTED-BASE64]")
}

// SanitizeError sanitizes error messages that might contain sensitive info
func (ls *LogSanitizer) SanitizeError(err error) string {
	if err == nil {
		return ""
	}
	return ls.Sanitize(err.Error())
}

// SanitizeMap sanitizes all values in a map (useful for labels/metadata)
func (ls *LogSanitizer) SanitizeMap(m map[string]string) map[string]string {
	sanitized := make(map[string]string)
	for k, v := range m {
		// Sanitize both keys and values
		sanitizedKey := ls.Sanitize(k)
		sanitizedValue := ls.Sanitize(v)

		// Extra check for sensitive key names
		if isSensitiveKey(k) {
			sanitizedValue = "[REDACTED]"
		}

		sanitized[sanitizedKey] = sanitizedValue
	}
	return sanitized
}

// isSensitiveKey checks if a key name suggests sensitive content
func isSensitiveKey(key string) bool {
	lowerKey := strings.ToLower(key)
	sensitiveKeywords := []string{
		"password", "passwd", "pwd",
		"secret", "token", "key",
		"auth", "credential", "cred",
		"private", "api", "bearer",
	}

	for _, keyword := range sensitiveKeywords {
		if strings.Contains(lowerKey, keyword) {
			return true
		}
	}
	return false
}

// PathSanitizer sanitizes file paths that might expose sensitive info
type PathSanitizer struct {
	homeDir string
}

// NewPathSanitizer creates a new path sanitizer
func NewPathSanitizer() *PathSanitizer {
	return &PathSanitizer{
		homeDir: "[HOME]",
	}
}

// Sanitize replaces sensitive path components
func (ps *PathSanitizer) Sanitize(path string) string {
	// Replace home directory references
	path = regexp.MustCompile(`/home/[^/]+`).ReplaceAllString(path, ps.homeDir)
	path = regexp.MustCompile(`/Users/[^/]+`).ReplaceAllString(path, ps.homeDir)
	path = strings.Replace(path, "~", ps.homeDir, 1)

	// Sanitize temp directories that might contain session IDs
	path = regexp.MustCompile(`/tmp/agentium/[^/]+`).ReplaceAllString(path, "/tmp/agentium/[SESSION-ID]")

	return path
}