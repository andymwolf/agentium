package controller

import (
	"fmt"
	"regexp"
	"strings"
)

// diffStartPatterns match the beginning of unified diff format
var diffStartPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^diff --git\s+`),
	regexp.MustCompile(`^@@\s+.*\s+@@`),
}

// diffLinePatterns match lines that are part of diff content
var diffLinePatterns = []*regexp.Regexp{
	regexp.MustCompile(`^\+\+\+\s+`),
	regexp.MustCompile(`^---\s+`),
	regexp.MustCompile(`^@@\s+.*\s+@@`),
}

// SummarizeForComment filters agent output for readable GitHub comments.
// - Excludes diff content
// - Limits to maxLines lines
// - Preserves status signals
func SummarizeForComment(content string, maxLines int) string {
	if content == "" {
		return ""
	}

	lines := strings.Split(content, "\n")
	var filtered []string
	inDiff := false

	for _, line := range lines {
		// Detect diff block start
		if isDiffStart(line) {
			inDiff = true
			continue
		}

		// Handle lines inside diff blocks
		if inDiff {
			if isDiffEnd(line) {
				inDiff = false
				// This line ends the diff - include it as content
				filtered = append(filtered, line)
			}
			// Skip diff content lines
			continue
		}

		// Keep non-diff lines
		filtered = append(filtered, line)
	}

	// Truncate if needed, keeping beginning and end for context
	if maxLines > 0 && len(filtered) > maxLines {
		mid := maxLines / 2
		omitted := len(filtered) - maxLines
		filtered = append(
			filtered[:mid],
			append([]string{fmt.Sprintf("... (%d lines omitted) ...", omitted)},
				filtered[len(filtered)-mid:]...)...,
		)
	}

	return strings.Join(filtered, "\n")
}

// isDiffStart returns true if the line starts a diff block
func isDiffStart(line string) bool {
	for _, pattern := range diffStartPatterns {
		if pattern.MatchString(line) {
			return true
		}
	}
	return false
}

// isDiffEnd returns true if the line ends a diff block.
// Diffs end when we see a non-diff line that isn't prefixed with +, -, or space.
func isDiffEnd(line string) bool {
	// Empty line could be end of diff or part of context
	if line == "" {
		return false
	}
	// Lines starting with +, -, or space (context) are still in the diff
	if len(line) > 0 {
		switch line[0] {
		case '+', '-', ' ', '@':
			return false
		}
	}
	// Check for diff metadata lines
	for _, pattern := range diffLinePatterns {
		if pattern.MatchString(line) {
			return false
		}
	}
	// Any other non-empty line ends the diff
	return true
}
