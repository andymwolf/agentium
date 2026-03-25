package controller

import (
	"strings"
	"testing"
	"time"
)

func TestBuildSynthesisPrompt(t *testing.T) {
	c := &Controller{
		config:     SessionConfig{Repository: "github.com/org/repo"},
		activeTask: "42",
	}

	results := []NamedReviewResult{
		{
			ReviewResult: ReviewResult{
				Feedback: "### [1] BLOCKER — `auth.go:42` (confidence: 95)\n\nNil pointer dereference.",
			},
			Name: "correctness",
		},
		{
			ReviewResult: ReviewResult{
				Feedback: "### [1] WARNING — `auth.go:50` (confidence: 85)\n\nError swallowed silently.",
			},
			Name: "errors",
		},
	}

	params := reviewRunParams{
		CompletedPhase: PhaseImplement,
		Iteration:      2,
		MaxIterations:  5,
	}

	prompt := c.buildSynthesisPrompt(PhaseImplement, results, params)

	contains := []string{
		"IMPLEMENT",
		"iteration 2/5",
		"github.com/org/repo",
		"#42",
		"2 specialized reviewers",
		"### Reviewer: correctness",
		"### Reviewer: errors",
		"Nil pointer dereference",
		"Error swallowed silently",
		"Synthesize",
	}

	for _, substr := range contains {
		if !strings.Contains(prompt, substr) {
			t.Errorf("buildSynthesisPrompt() missing %q", substr)
		}
	}
}

func TestBuildSynthesisPrompt_SingleReviewer(t *testing.T) {
	c := &Controller{
		config:     SessionConfig{Repository: "github.com/org/repo"},
		activeTask: "10",
	}

	results := []NamedReviewResult{
		{
			ReviewResult: ReviewResult{Feedback: "No issues found."},
			Name:         "tests",
		},
	}

	params := reviewRunParams{
		CompletedPhase: PhaseImplement,
		Iteration:      1,
		MaxIterations:  3,
	}

	prompt := c.buildSynthesisPrompt(PhaseImplement, results, params)

	if !strings.Contains(prompt, "1 specialized reviewers") {
		t.Error("buildSynthesisPrompt() should show reviewer count")
	}
	if !strings.Contains(prompt, "### Reviewer: tests") {
		t.Error("buildSynthesisPrompt() missing reviewer section")
	}
}

func TestNamedReviewResult_EmbedsReviewResult(t *testing.T) {
	now := time.Now()
	nr := NamedReviewResult{
		ReviewResult: ReviewResult{
			Feedback:     "some feedback",
			InputTokens:  100,
			OutputTokens: 200,
			StartTime:    now,
			EndTime:      now.Add(time.Second),
		},
		Name: "correctness",
	}

	if nr.Feedback != "some feedback" {
		t.Error("NamedReviewResult should expose embedded ReviewResult.Feedback")
	}
	if nr.InputTokens != 100 {
		t.Error("NamedReviewResult should expose embedded ReviewResult.InputTokens")
	}
	if nr.Name != "correctness" {
		t.Error("NamedReviewResult.Name should be set")
	}
}
