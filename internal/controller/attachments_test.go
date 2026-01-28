package controller

import (
	"strings"
	"testing"
)

func TestContentNeedsAttachment(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "short content",
			content:  "Hello, world!",
			expected: false,
		},
		{
			name:     "exactly at threshold",
			content:  strings.Repeat("a", AttachmentThreshold),
			expected: false,
		},
		{
			name:     "just over threshold",
			content:  strings.Repeat("a", AttachmentThreshold+1),
			expected: true,
		},
		{
			name:     "well over threshold",
			content:  strings.Repeat("a", 1000),
			expected: true,
		},
		{
			name:     "empty content",
			content:  "",
			expected: false,
		},
		{
			name:     "unicode content under threshold",
			content:  strings.Repeat("日", 100),
			expected: false,
		},
		{
			name:     "unicode content over threshold",
			content:  strings.Repeat("日", AttachmentThreshold+1),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := contentNeedsAttachment(tt.content)
			if result != tt.expected {
				t.Errorf("contentNeedsAttachment() = %v, want %v (content length: %d runes)",
					result, tt.expected, len([]rune(tt.content)))
			}
		})
	}
}

func TestExtractSummary(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		maxRunes  int
		wantLen   int
		wantEnds  string
	}{
		{
			name:     "short content unchanged",
			content:  "Hello, world!",
			maxRunes: 200,
			wantLen:  13,
			wantEnds: "Hello, world!",
		},
		{
			name:     "long content truncated with ellipsis",
			content:  strings.Repeat("a", 300),
			maxRunes: 100,
			wantEnds: "...",
		},
		{
			name:     "breaks at newline when possible",
			content:  "First line\nSecond line\nThird line that is very long and continues",
			maxRunes: 50,
			wantEnds: "...",
		},
		{
			name:     "empty content",
			content:  "",
			maxRunes: 100,
			wantLen:  0,
			wantEnds: "",
		},
		{
			name:     "unicode content",
			content:  strings.Repeat("日本語", 100),
			maxRunes: 50,
			wantEnds: "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractSummary(tt.content, tt.maxRunes)

			// Check ending
			if tt.wantEnds != "" && !strings.HasSuffix(result, tt.wantEnds) {
				t.Errorf("extractSummary() should end with %q, got %q", tt.wantEnds, result)
			}

			// Check exact length if specified
			if tt.wantLen > 0 && len(result) != tt.wantLen {
				t.Errorf("extractSummary() length = %d, want %d", len(result), tt.wantLen)
			}

			// Check that result doesn't exceed maxRunes + ellipsis length
			resultRunes := len([]rune(result))
			maxExpected := tt.maxRunes + 3 // "..." is 3 runes
			if resultRunes > maxExpected {
				t.Errorf("extractSummary() result has %d runes, should be <= %d", resultRunes, maxExpected)
			}
		})
	}
}

func TestExtractSummary_BreaksAtNewline(t *testing.T) {
	// Content with a newline in the middle half
	content := "Line one is here.\nLine two is longer and continues past the limit."
	result := extractSummary(content, 40)

	// Should break at the newline (position 17) since it's after position 20 (40/2)
	// Actually position 17 is less than 20, so it won't break there
	// Let's use a different test case
	content = strings.Repeat("a", 30) + "\n" + strings.Repeat("b", 50)
	result = extractSummary(content, 50)

	// The newline is at position 30, which is > 25 (50/2), so it should break there
	if !strings.HasSuffix(result, "...") {
		t.Errorf("expected ellipsis suffix, got: %q", result)
	}
	if strings.Contains(result, "b") {
		t.Errorf("should have broken at newline before 'b's, got: %q", result)
	}
}

func TestGistFilename(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		phase       TaskPhase
		iteration   int
		taskID      string
		expected    string
	}{
		{
			name:        "phase output",
			contentType: "phase",
			phase:       PhaseImplement,
			iteration:   2,
			taskID:      "123",
			expected:    "phase_IMPLEMENT_iter_2_issue_123.md",
		},
		{
			name:        "review feedback",
			contentType: "review",
			phase:       PhasePlan,
			iteration:   1,
			taskID:      "456",
			expected:    "review_PLAN_iter_1_issue_456.md",
		},
		{
			name:        "judge feedback",
			contentType: "judge",
			phase:       PhaseDocs,
			iteration:   0,
			taskID:      "789",
			expected:    "judge_DOCS_issue_789.md",
		},
		{
			name:        "plan",
			contentType: "plan",
			phase:       PhasePlan,
			iteration:   0,
			taskID:      "100",
			expected:    "plan_issue_100.md",
		},
		{
			name:        "unknown type uses default",
			contentType: "unknown",
			phase:       PhaseImplement,
			iteration:   1,
			taskID:      "200",
			expected:    "agentium_unknown_issue_200.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := gistFilename(tt.contentType, tt.phase, tt.iteration, tt.taskID)
			if result != tt.expected {
				t.Errorf("gistFilename() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestAttachmentThreshold(t *testing.T) {
	// Verify the threshold constant is set correctly
	if AttachmentThreshold != 500 {
		t.Errorf("AttachmentThreshold = %d, want 500", AttachmentThreshold)
	}
}
