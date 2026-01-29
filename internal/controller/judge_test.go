package controller

import "testing"

func TestParseJudgeVerdict(t *testing.T) {
	tests := []struct {
		name         string
		output       string
		wantVerdict  JudgeVerdict
		wantFeedback string
		wantSignal   bool
	}{
		{
			name:         "ADVANCE verdict",
			output:       "Some analysis...\nAGENTIUM_EVAL: ADVANCE\nDone.",
			wantVerdict:  VerdictAdvance,
			wantFeedback: "",
			wantSignal:   true,
		},
		{
			name:         "ITERATE with feedback",
			output:       "Analysis...\nAGENTIUM_EVAL: ITERATE Tests failed in auth/handler_test.go\nDone.",
			wantVerdict:  VerdictIterate,
			wantFeedback: "Tests failed in auth/handler_test.go",
			wantSignal:   true,
		},
		{
			name:         "BLOCKED with reason",
			output:       "AGENTIUM_EVAL: BLOCKED Need API credentials for integration test",
			wantVerdict:  VerdictBlocked,
			wantFeedback: "Need API credentials for integration test",
			wantSignal:   true,
		},
		{
			name:         "no signal defaults to BLOCKED (fail-closed)",
			output:       "Some output without any eval signal",
			wantVerdict:  VerdictBlocked,
			wantFeedback: "",
			wantSignal:   false,
		},
		{
			name:         "empty output defaults to BLOCKED",
			output:       "",
			wantVerdict:  VerdictBlocked,
			wantFeedback: "",
			wantSignal:   false,
		},
		{
			name:         "ADVANCE with trailing whitespace",
			output:       "AGENTIUM_EVAL: ADVANCE   ",
			wantVerdict:  VerdictAdvance,
			wantFeedback: "",
			wantSignal:   true,
		},
		{
			name:         "ITERATE with multi-word feedback",
			output:       "AGENTIUM_EVAL: ITERATE fix the nil pointer in TestLogin and add error handling",
			wantVerdict:  VerdictIterate,
			wantFeedback: "fix the nil pointer in TestLogin and add error handling",
			wantSignal:   true,
		},
		{
			name:         "malformed - wrong prefix",
			output:       "AGENTIUM_STATUS: ADVANCE",
			wantVerdict:  VerdictBlocked,
			wantFeedback: "",
			wantSignal:   false,
		},
		{
			name:         "malformed - invalid verdict",
			output:       "AGENTIUM_EVAL: UNKNOWN something",
			wantVerdict:  VerdictBlocked,
			wantFeedback: "",
			wantSignal:   false,
		},
		{
			name:         "SIMPLE is not a valid judge verdict",
			output:       "AGENTIUM_EVAL: SIMPLE straightforward change",
			wantVerdict:  VerdictBlocked,
			wantFeedback: "",
			wantSignal:   false,
		},
		{
			name:         "COMPLEX is not a valid judge verdict",
			output:       "AGENTIUM_EVAL: COMPLEX multiple components",
			wantVerdict:  VerdictBlocked,
			wantFeedback: "",
			wantSignal:   false,
		},
		{
			name:         "NOMERGE is not a valid judge verdict",
			output:       "AGENTIUM_EVAL: NOMERGE needs human review",
			wantVerdict:  VerdictBlocked,
			wantFeedback: "",
			wantSignal:   false,
		},
		{
			name:         "multiple verdicts - first wins",
			output:       "AGENTIUM_EVAL: ITERATE fix tests\nAGENTIUM_EVAL: ADVANCE",
			wantVerdict:  VerdictIterate,
			wantFeedback: "fix tests",
			wantSignal:   true,
		},
		{
			name:         "verdict not at start of line is ignored",
			output:       "prefix AGENTIUM_EVAL: ADVANCE\nAGENTIUM_EVAL: BLOCKED real issue",
			wantVerdict:  VerdictBlocked,
			wantFeedback: "real issue",
			wantSignal:   true,
		},
		// Markdown fence stripping tests
		{
			name:         "verdict inside markdown code fence is detected",
			output:       "Here is my verdict:\n```\nAGENTIUM_EVAL: ADVANCE\n```",
			wantVerdict:  VerdictAdvance,
			wantFeedback: "",
			wantSignal:   true,
		},
		{
			name:         "verdict inside markdown fence with language tag",
			output:       "```text\nAGENTIUM_EVAL: ITERATE fix the tests\n```",
			wantVerdict:  VerdictIterate,
			wantFeedback: "fix the tests",
			wantSignal:   true,
		},
		{
			name:         "verdict inside triple backticks with surrounding text",
			output:       "Analysis complete.\n```\nAGENTIUM_EVAL: BLOCKED need credentials\n```\nEnd of response.",
			wantVerdict:  VerdictBlocked,
			wantFeedback: "need credentials",
			wantSignal:   true,
		},
		{
			name:         "raw verdict preferred over fenced verdict",
			output:       "AGENTIUM_EVAL: ADVANCE\n```\nAGENTIUM_EVAL: BLOCKED should not match\n```",
			wantVerdict:  VerdictAdvance,
			wantFeedback: "",
			wantSignal:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseJudgeVerdict(tt.output)
			if result.Verdict != tt.wantVerdict {
				t.Errorf("Verdict = %q, want %q", result.Verdict, tt.wantVerdict)
			}
			if result.Feedback != tt.wantFeedback {
				t.Errorf("Feedback = %q, want %q", result.Feedback, tt.wantFeedback)
			}
			if result.SignalFound != tt.wantSignal {
				t.Errorf("SignalFound = %v, want %v", result.SignalFound, tt.wantSignal)
			}
		})
	}
}

func TestJudgeContextBudget_Default(t *testing.T) {
	c := &Controller{config: SessionConfig{}}
	if got := c.judgeContextBudget(); got != defaultJudgeContextBudget {
		t.Errorf("judgeContextBudget() = %d, want %d", got, defaultJudgeContextBudget)
	}
}

func TestJudgeContextBudget_Custom(t *testing.T) {
	c := &Controller{
		config: SessionConfig{
			PhaseLoop: &PhaseLoopConfig{
				Enabled:            true,
				JudgeContextBudget: 20000,
			},
		},
	}
	if got := c.judgeContextBudget(); got != 20000 {
		t.Errorf("judgeContextBudget() = %d, want 20000", got)
	}
}

func TestBuildJudgePrompt(t *testing.T) {
	c := &Controller{
		config:     SessionConfig{Repository: "github.com/org/repo"},
		activeTask: "42",
	}

	params := judgeRunParams{
		CompletedPhase: PhaseImplement,
		PhaseOutput:    "code changes here",
		ReviewFeedback: "Missing error handling in auth.go",
		Iteration:      2,
		MaxIterations:  5,
	}

	prompt := c.buildJudgePrompt(params)

	contains := []string{
		"IMPLEMENT",
		"judge",
		"github.com/org/repo",
		"#42",
		"2/5",
		"Missing error handling in auth.go",
		"code changes here",
		"AGENTIUM_EVAL:",
	}
	for _, substr := range contains {
		if !containsString(prompt, substr) {
			t.Errorf("buildJudgePrompt() missing %q", substr)
		}
	}

	// Should NOT contain final iteration note
	if containsString(prompt, "FINAL iteration") {
		t.Error("buildJudgePrompt() should not mention final iteration when iteration < max")
	}

	// Should NOT mention REGRESS for non-REVIEW phase
	if containsString(prompt, "REGRESS") {
		t.Error("buildJudgePrompt() should not mention REGRESS for IMPLEMENT phase")
	}
}

func TestBuildJudgePrompt_FinalIteration(t *testing.T) {
	c := &Controller{
		config:     SessionConfig{Repository: "github.com/org/repo"},
		activeTask: "1",
	}

	params := judgeRunParams{
		CompletedPhase: PhasePlan,
		PhaseOutput:    "plan output",
		ReviewFeedback: "some feedback",
		Iteration:      3,
		MaxIterations:  3,
	}

	prompt := c.buildJudgePrompt(params)

	if !containsString(prompt, "FINAL iteration") {
		t.Error("buildJudgePrompt() should mention FINAL iteration when iteration == max")
	}
	if !containsString(prompt, "Prefer ADVANCE") {
		t.Error("buildJudgePrompt() should tell judge to prefer ADVANCE on final iteration")
	}
}

func TestBuildJudgePrompt_EmptyReviewFeedback(t *testing.T) {
	c := &Controller{
		config:     SessionConfig{Repository: "github.com/org/repo"},
		activeTask: "1",
	}

	params := judgeRunParams{
		CompletedPhase: PhaseImplement,
		PhaseOutput:    "test output",
		ReviewFeedback: "",
		Iteration:      1,
		MaxIterations:  3,
	}

	prompt := c.buildJudgePrompt(params)

	if !containsString(prompt, "No feedback provided") {
		t.Error("buildJudgePrompt() should indicate no feedback when ReviewFeedback is empty")
	}
}

func TestBuildJudgePrompt_TruncatesLongOutput(t *testing.T) {
	c := &Controller{
		config: SessionConfig{
			Repository: "github.com/org/repo",
			PhaseLoop:  &PhaseLoopConfig{Enabled: true, JudgeContextBudget: 100},
		},
		activeTask: "1",
	}

	// Create output longer than custom 100 chars
	longOutput := ""
	for i := 0; i < 20; i++ {
		longOutput += "0123456789"
	}

	params := judgeRunParams{
		CompletedPhase: PhaseImplement,
		PhaseOutput:    longOutput,
		Iteration:      1,
		MaxIterations:  3,
	}

	prompt := c.buildJudgePrompt(params)
	if !containsString(prompt, "output truncated") {
		t.Error("expected truncation marker with custom budget")
	}
}
