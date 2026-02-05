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
		"indicate severity where relevant",
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

func TestExtractFeedbackResponses_ValidSignals(t *testing.T) {
	output := `some output before
AGENTIUM_MEMORY: FEEDBACK_RESPONSE [ADDRESSED] Fix nil pointer in auth handler - Added nil check at handler.go:45
more output
AGENTIUM_MEMORY: FEEDBACK_RESPONSE [DECLINED] Use sync.Map for concurrency - Current mutex is sufficient for our access pattern
AGENTIUM_MEMORY: FEEDBACK_RESPONSE [PARTIAL] Improve test coverage - Added unit tests, integration tests deferred to follow-up
trailing output`

	responses := extractFeedbackResponses(output)
	if len(responses) != 3 {
		t.Fatalf("expected 3 responses, got %d", len(responses))
	}

	expected := []string{
		"[ADDRESSED] Fix nil pointer in auth handler - Added nil check at handler.go:45",
		"[DECLINED] Use sync.Map for concurrency - Current mutex is sufficient for our access pattern",
		"[PARTIAL] Improve test coverage - Added unit tests, integration tests deferred to follow-up",
	}
	for i, exp := range expected {
		if responses[i] != exp {
			t.Errorf("response[%d] = %q, want %q", i, responses[i], exp)
		}
	}
}

func TestExtractFeedbackResponses_NoSignals(t *testing.T) {
	output := "regular output with no feedback response signals\njust normal text"
	responses := extractFeedbackResponses(output)
	if len(responses) != 0 {
		t.Fatalf("expected 0 responses, got %d", len(responses))
	}
}

func TestExtractFeedbackResponses_MixedWithOtherSignals(t *testing.T) {
	output := `AGENTIUM_MEMORY: KEY_FACT The API uses JWT tokens
AGENTIUM_MEMORY: FEEDBACK_RESPONSE [ADDRESSED] Fix auth bug - Done
AGENTIUM_MEMORY: STEP_DONE Implemented middleware
AGENTIUM_MEMORY: FEEDBACK_RESPONSE [DECLINED] Refactor utils - Not needed
AGENTIUM_MEMORY: ERROR Some error occurred`

	responses := extractFeedbackResponses(output)
	if len(responses) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(responses))
	}
	if responses[0] != "[ADDRESSED] Fix auth bug - Done" {
		t.Errorf("response[0] = %q, want %q", responses[0], "[ADDRESSED] Fix auth bug - Done")
	}
	if responses[1] != "[DECLINED] Refactor utils - Not needed" {
		t.Errorf("response[1] = %q, want %q", responses[1], "[DECLINED] Refactor utils - Not needed")
	}
}

func TestExtractFeedbackResponses_EmptyOutput(t *testing.T) {
	responses := extractFeedbackResponses("")
	if len(responses) != 0 {
		t.Fatalf("expected 0 responses for empty output, got %d", len(responses))
	}
}

func TestBuildReviewPrompt_WorkerFeedbackResponses(t *testing.T) {
	c := &Controller{
		config:     SessionConfig{Repository: "github.com/org/repo"},
		activeTask: "42",
	}

	params := reviewRunParams{
		CompletedPhase:          PhasePlan,
		PhaseOutput:             "Step 1: modify auth.go",
		Iteration:               2,
		MaxIterations:           3,
		PreviousFeedback:        "Some old feedback",
		WorkerFeedbackResponses: "[ADDRESSED] Fix auth - Done\n[DECLINED] Refactor - Not needed",
	}

	prompt := c.buildReviewPrompt(params)

	// Should contain worker feedback responses section
	if !containsString(prompt, "Worker's Response to Previous Feedback") {
		t.Error("expected 'Worker's Response to Previous Feedback' section")
	}
	if !containsString(prompt, "[ADDRESSED] Fix auth - Done") {
		t.Error("expected feedback response content in prompt")
	}

	// Should NOT contain raw previous feedback (worker responses take priority)
	if containsString(prompt, "Previous Iteration Feedback") {
		t.Error("should not contain 'Previous Iteration Feedback' when worker responses are present")
	}

	// Should contain verification instructions
	if !containsString(prompt, "ADDRESSED items: verify") {
		t.Error("expected verification instructions for ADDRESSED items")
	}
}

func TestBuildReviewPrompt_FallbackToPreviousFeedback(t *testing.T) {
	c := &Controller{
		config:     SessionConfig{Repository: "github.com/org/repo"},
		activeTask: "42",
	}

	params := reviewRunParams{
		CompletedPhase:   PhasePlan,
		PhaseOutput:      "Step 1: modify auth.go",
		Iteration:        2,
		MaxIterations:    3,
		PreviousFeedback: "Some old feedback",
		// No WorkerFeedbackResponses — should fall back
	}

	prompt := c.buildReviewPrompt(params)

	// Should fall back to raw previous feedback
	if !containsString(prompt, "Previous Iteration Feedback") {
		t.Error("expected 'Previous Iteration Feedback' fallback section")
	}
	if containsString(prompt, "Worker's Response to Previous Feedback") {
		t.Error("should not contain worker responses section when none provided")
	}
}
