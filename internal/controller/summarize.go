package controller

import (
	"fmt"
	"regexp"
	"strings"
)

// agentiumSignalPattern matches single-line AGENTIUM_STATUS and AGENTIUM_MEMORY signals.
var agentiumSignalPattern = regexp.MustCompile(`(?m)^AGENTIUM_(?:STATUS|MEMORY):\s+.*$`)

// agentiumHandoffPattern matches multi-line AGENTIUM_HANDOFF: { ... } blocks.
var agentiumHandoffPattern = regexp.MustCompile(`(?ms)^AGENTIUM_HANDOFF:\s*\{.*?\n\}`)

// StripAgentiumSignals removes AGENTIUM_STATUS, AGENTIUM_MEMORY, and
// multi-line AGENTIUM_HANDOFF blocks from content. These are internal
// protocol signals not intended for human readers.
func StripAgentiumSignals(content string) string {
	if content == "" {
		return ""
	}
	content = agentiumHandoffPattern.ReplaceAllString(content, "")
	content = agentiumSignalPattern.ReplaceAllString(content, "")
	// Collapse runs of blank lines left by removed signals
	content = collapseBlankLines(content)
	return strings.TrimSpace(content)
}

// preamblePatterns match common stream-of-thought leading lines that agents
// produce before their actual output (e.g., "Let me examine...", "I'll review...").
var preamblePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^(let me|i'll|i will|i need to|i should|i want to|i'm going to)\b`),
	regexp.MustCompile(`(?i)^(excellent|great|good|perfect|okay|alright|now i|now let)\b`),
	regexp.MustCompile(`(?i)^(looking at|examining|reviewing|reading|checking|analyzing|starting)\b`),
	regexp.MustCompile(`(?i)^(first,?\s|next,?\s|to begin|to start)\b`),
	regexp.MustCompile(`(?i)^(i now have|i have a|i can see|i understand)\b`),
	regexp.MustCompile(`(?i)^(file created|file updated|file written|file saved|changes saved)\b`),
	regexp.MustCompile(`(?i)^(based on|given the|considering)\b`),
}

// StripPreamble removes leading conversational/stream-of-thought lines from
// content. It stops at the first non-blank, non-preamble line to preserve the
// agent's substantive output.
func StripPreamble(content string) string {
	if content == "" {
		return ""
	}
	lines := strings.Split(content, "\n")
	startIdx := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if isPreambleLine(trimmed) {
			startIdx = i + 1
			continue
		}
		// First non-blank, non-preamble line — stop stripping
		break
	}
	if startIdx >= len(lines) {
		return ""
	}
	result := strings.Join(lines[startIdx:], "\n")
	return strings.TrimSpace(result)
}

// isPreambleLine returns true if the line matches a stream-of-thought pattern.
func isPreambleLine(line string) bool {
	for _, p := range preamblePatterns {
		if p.MatchString(line) {
			return true
		}
	}
	return false
}

// collapseBlankLines replaces runs of 3+ consecutive blank lines with a single blank line.
func collapseBlankLines(s string) string {
	prev := ""
	for {
		next := strings.ReplaceAll(s, "\n\n\n", "\n\n")
		if next == prev {
			return next
		}
		prev = next
		s = next
	}
}

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
