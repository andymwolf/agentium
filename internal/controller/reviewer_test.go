package controller

import "testing"

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
		CompletedPhase: PhaseTest,
		PhaseOutput:    "tests passed",
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
