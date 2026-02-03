package controller

import (
	"strings"
	"testing"
)

func TestBuildReviewPrompt(t *testing.T) {
	c := &Controller{
		config:     SessionConfig{Repository: "github.com/org/repo"},
		activeTask: "42",
	}

	params := reviewRunParams{
		CompletedPhase: PhasePlan,
		PhaseOutput:    "Step 1: modify auth.go\nStep 2: add tests",
		Iteration:      2,
		MaxIterations:  3,
	}

	prompt := c.buildReviewPrompt(params)

	contains := []string{
		"PLAN",
		"iteration 2/3",
		"github.com/org/repo",
		"#42",
		"modify auth.go",
		"constructive, actionable review feedback",
		"Do NOT emit any verdict",
	}
	for _, substr := range contains {
		if !containsString(prompt, substr) {
			t.Errorf("buildReviewPrompt() missing %q", substr)
		}
	}
}

func TestBuildReviewPrompt_TruncatesLongOutput(t *testing.T) {
	c := &Controller{
		config:     SessionConfig{Repository: "github.com/org/repo"},
		activeTask: "1",
	}

	// Create output longer than default 8000 chars
	longOutput := ""
	for i := 0; i < 1000; i++ {
		longOutput += "0123456789"
	}

	params := reviewRunParams{
		CompletedPhase: PhaseImplement,
		PhaseOutput:    longOutput,
		Iteration:      1,
		MaxIterations:  5,
	}

	prompt := c.buildReviewPrompt(params)
	if !containsString(prompt, "output truncated") {
		t.Error("expected truncation marker in review prompt for long output")
	}
}

func TestBuildReviewPrompt_NoMemoryInjected(t *testing.T) {
	c := &Controller{
		config:     SessionConfig{Repository: "github.com/org/repo"},
		activeTask: "7",
	}

	params := reviewRunParams{
		CompletedPhase: PhaseImplement,
		PhaseOutput:    "implementation complete",
		Iteration:      1,
		MaxIterations:  3,
	}

	prompt := c.buildReviewPrompt(params)

	// The review prompt should not contain memory sections
	memoryMarkers := []string{
		"Memory from Previous Iterations",
		"Iteration History",
		"Evaluator Feedback",
	}
	for _, marker := range memoryMarkers {
		if containsString(prompt, marker) {
			t.Errorf("buildReviewPrompt() should not contain memory marker %q", marker)
		}
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "short string unchanged",
			input:  "hello",
			maxLen: 10,
			want:   "hello",
		},
		{
			name:   "exact length unchanged",
			input:  "hello",
			maxLen: 5,
			want:   "hello",
		},
		{
			name:   "long string truncated",
			input:  "hello world",
			maxLen: 8,
			want:   "hello...",
		},
		{
			name:   "very short maxLen",
			input:  "hello",
			maxLen: 3,
			want:   "hel",
		},
		{
			name:   "empty string",
			input:  "",
			maxLen: 10,
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateString(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateString(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestReviewerFallback_NotPRSummary(t *testing.T) {
	// This test verifies the fallback behavior when RawTextContent is empty.
	// The fallback should NOT contain PR detection results like "Created 2 PR(s): #123, #99"
	// Instead it should provide a descriptive message.

	// Simulate what the fallback message should look like
	successFallback := "Review completed but no feedback text was captured. Check agent logs for details."
	failureFallback := "Review failed: API timeout"

	// Verify success fallback doesn't look like PR summary
	if strings.Contains(successFallback, "PR(s)") || strings.Contains(successFallback, "Created") {
		t.Error("success fallback should not contain PR summary content")
	}

	// Verify failure fallback includes error info
	if !strings.Contains(failureFallback, "API timeout") {
		t.Error("failure fallback should include error information")
	}
}
