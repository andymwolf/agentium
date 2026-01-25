// Package security provides utilities for securing sensitive information in logs and output.
package security

import (
	"regexp"
	"strings"
)

// Common patterns for sensitive data that should be scrubbed from logs
var sensitivePatterns = []*regexp.Regexp{
	// Generic tokens and keys
	regexp.MustCompile(`(?i)(api[_-]?key|apikey|api[_-]?token|access[_-]?token|auth[_-]?token|authentication[_-]?token|private[_-]?key|secret[_-]?key)[\s]*[:=][\s]*["']?([a-zA-Z0-9_\-./+=]{20,})["']?`),

	// Bearer tokens
	regexp.MustCompile(`(?i)bearer\s+([a-zA-Z0-9_\-./+=]{20,})`),

	// AWS patterns
	regexp.MustCompile(`(?i)(aws[_-]?access[_-]?key[_-]?id|aws[_-]?secret[_-]?access[_-]?key)[\s]*[:=][\s]*["']?([a-zA-Z0-9/+=]{20,})["']?`),

	// GitHub tokens
	regexp.MustCompile(`ghp_[a-zA-Z0-9]{36}`),
	regexp.MustCompile(`gho_[a-zA-Z0-9]{36}`),
	regexp.MustCompile(`ghs_[a-zA-Z0-9]{36}`),
	regexp.MustCompile(`ghr_[a-zA-Z0-9]{36}`),

	// Google Cloud patterns
	regexp.MustCompile(`(?i)gcp[_-]?key[\s]*[:=][\s]*["']?([a-zA-Z0-9_\-./+=]{20,})["']?`),

	// JWT tokens
	regexp.MustCompile(`eyJ[a-zA-Z0-9_-]+\.eyJ[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+`),

	// SSH private keys
	regexp.MustCompile(`-----BEGIN\s+(?:RSA\s+)?PRIVATE\s+KEY-----[\s\S]+?-----END\s+(?:RSA\s+)?PRIVATE\s+KEY-----`),

	// Generic secret patterns
	regexp.MustCompile(`(?i)(password|passwd|pwd)[\s]*:[\s]*"([^"]{8,})"`),
	regexp.MustCompile(`(?i)(password|passwd|pwd)[\s]*:[\s]*'([^']{8,})'`),
	regexp.MustCompile(`(?i)(password|passwd|pwd)[\s]*[:=][\s]*"([^"]{8,})"`),
	regexp.MustCompile(`(?i)(password|passwd|pwd)[\s]*[:=][\s]*'([^']{8,})'`),
	regexp.MustCompile(`(?i)(password|passwd|pwd)[\s]*[:=][\s]*([^\s"']{8,})`),
	regexp.MustCompile(`(?i)(secret)[\s]*[:=][\s]*["']?([a-zA-Z0-9_\-./+=]{16,})["']?`),

	// Base64 encoded potential secrets (minimum 40 chars to reduce false positives)
	regexp.MustCompile(`(?:[A-Za-z0-9+/]{40,}={0,2})`),
}

// Scrubber provides methods to remove sensitive information from strings
type Scrubber struct {
	patterns []*regexp.Regexp
}

// NewScrubber creates a new Scrubber with default patterns
func NewScrubber() *Scrubber {
	return &Scrubber{
		patterns: sensitivePatterns,
	}
}

// Scrub removes sensitive information from the input string
func (s *Scrubber) Scrub(input string) string {
	scrubbed := input

	for _, pattern := range s.patterns {
		scrubbed = pattern.ReplaceAllStringFunc(scrubbed, func(match string) string {
			// Preserve some context to understand what was redacted
			if strings.Contains(match, "=") {
				parts := strings.SplitN(match, "=", 2)
				if len(parts) == 2 {
					return parts[0] + "=***REDACTED***"
				}
			} else if strings.Contains(match, ":") {
				parts := strings.SplitN(match, ":", 2)
				if len(parts) == 2 {
					return parts[0] + ":***REDACTED***"
				}
			} else if strings.HasPrefix(match, "Bearer ") {
				return "Bearer ***REDACTED***"
			} else if strings.Contains(match, "BEGIN") && strings.Contains(match, "PRIVATE KEY") {
				return "-----BEGIN PRIVATE KEY----- ***REDACTED*** -----END PRIVATE KEY-----"
			}
			// For standalone tokens, show partial match for debugging
			if len(match) > 10 {
				return match[:4] + "***REDACTED***"
			}
			return "***REDACTED***"
		})
	}

	return scrubbed
}

// ScrubSlice applies scrubbing to each string in a slice
func (s *Scrubber) ScrubSlice(inputs []string) []string {
	scrubbed := make([]string, len(inputs))
	for i, input := range inputs {
		scrubbed[i] = s.Scrub(input)
	}
	return scrubbed
}

// AddPattern adds a custom pattern to the scrubber
func (s *Scrubber) AddPattern(pattern *regexp.Regexp) {
	s.patterns = append(s.patterns, pattern)
}

// ContainsSensitive checks if the input contains any sensitive patterns
// This is useful for validation without modifying the string
func (s *Scrubber) ContainsSensitive(input string) bool {
	for _, pattern := range s.patterns {
		if pattern.MatchString(input) {
			return true
		}
	}
	return false
}