package controller

import "testing"

func TestParseEvalVerdict(t *testing.T) {
	tests := []struct {
		name         string
		output       string
		wantVerdict  EvalVerdict
		wantFeedback string
	}{
		{
			name:         "ADVANCE verdict",
			output:       "Some analysis...\nAGENTIUM_EVAL: ADVANCE\nDone.",
			wantVerdict:  VerdictAdvance,
			wantFeedback: "",
		},
		{
			name:         "ITERATE with feedback",
			output:       "Analysis...\nAGENTIUM_EVAL: ITERATE Tests failed in auth/handler_test.go\nDone.",
			wantVerdict:  VerdictIterate,
			wantFeedback: "Tests failed in auth/handler_test.go",
		},
		{
			name:         "BLOCKED with reason",
			output:       "AGENTIUM_EVAL: BLOCKED Need API credentials for integration test",
			wantVerdict:  VerdictBlocked,
			wantFeedback: "Need API credentials for integration test",
		},
		{
			name:         "no signal defaults to ADVANCE",
			output:       "Some output without any eval signal",
			wantVerdict:  VerdictAdvance,
			wantFeedback: "",
		},
		{
			name:         "empty output defaults to ADVANCE",
			output:       "",
			wantVerdict:  VerdictAdvance,
			wantFeedback: "",
		},
		{
			name:         "ADVANCE with trailing whitespace",
			output:       "AGENTIUM_EVAL: ADVANCE   ",
			wantVerdict:  VerdictAdvance,
			wantFeedback: "",
		},
		{
			name:         "ITERATE with multi-word feedback",
			output:       "AGENTIUM_EVAL: ITERATE fix the nil pointer in TestLogin and add error handling",
			wantVerdict:  VerdictIterate,
			wantFeedback: "fix the nil pointer in TestLogin and add error handling",
		},
		{
			name:         "malformed - wrong prefix",
			output:       "AGENTIUM_STATUS: ADVANCE",
			wantVerdict:  VerdictAdvance,
			wantFeedback: "",
		},
		{
			name:         "malformed - invalid verdict",
			output:       "AGENTIUM_EVAL: UNKNOWN something",
			wantVerdict:  VerdictAdvance,
			wantFeedback: "",
		},
		{
			name:         "multiple verdicts - first wins",
			output:       "AGENTIUM_EVAL: ITERATE fix tests\nAGENTIUM_EVAL: ADVANCE",
			wantVerdict:  VerdictIterate,
			wantFeedback: "fix tests",
		},
		{
			name:         "verdict not at start of line is ignored",
			output:       "prefix AGENTIUM_EVAL: ADVANCE\nAGENTIUM_EVAL: BLOCKED real issue",
			wantVerdict:  VerdictBlocked,
			wantFeedback: "real issue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseEvalVerdict(tt.output)
			if result.Verdict != tt.wantVerdict {
				t.Errorf("Verdict = %q, want %q", result.Verdict, tt.wantVerdict)
			}
			if result.Feedback != tt.wantFeedback {
				t.Errorf("Feedback = %q, want %q", result.Feedback, tt.wantFeedback)
			}
		})
	}
}

func TestBuildEvalPrompt(t *testing.T) {
	c := &Controller{
		config:     SessionConfig{Repository: "github.com/org/repo"},
		activeTask: "42",
	}

	prompt := c.buildEvalPrompt(PhaseImplement, "some output here")

	contains := []string{
		"IMPLEMENT",
		"github.com/org/repo",
		"#42",
		"some output here",
		"AGENTIUM_EVAL:",
	}
	for _, substr := range contains {
		if !containsString(prompt, substr) {
			t.Errorf("buildEvalPrompt() missing %q", substr)
		}
	}
}

func TestBuildEvalPrompt_TruncatesLongOutput(t *testing.T) {
	c := &Controller{
		config:     SessionConfig{Repository: "github.com/org/repo"},
		activeTask: "1",
	}

	// Create output longer than default 8000 chars
	longOutput := ""
	for i := 0; i < 1000; i++ {
		longOutput += "0123456789"
	}

	prompt := c.buildEvalPrompt(PhaseTest, longOutput)
	if !containsString(prompt, "output truncated") {
		t.Error("expected truncation marker in prompt for long output")
	}
}

func TestEvalContextBudget_Default(t *testing.T) {
	c := &Controller{config: SessionConfig{}}
	if got := c.evalContextBudget(); got != defaultEvalContextBudget {
		t.Errorf("evalContextBudget() = %d, want %d", got, defaultEvalContextBudget)
	}
}

func TestEvalContextBudget_Custom(t *testing.T) {
	c := &Controller{
		config: SessionConfig{
			PhaseLoop: &PhaseLoopConfig{
				Enabled:           true,
				EvalContextBudget: 20000,
			},
		},
	}
	if got := c.evalContextBudget(); got != 20000 {
		t.Errorf("evalContextBudget() = %d, want 20000", got)
	}
}

func TestParseJudgeVerdict_FailClosed(t *testing.T) {
	tests := []struct {
		name         string
		output       string
		wantVerdict  EvalVerdict
		wantSignal   bool
		wantFeedback string
	}{
		{
			name:        "no signal defaults to ITERATE (fail-closed)",
			output:      "Some output without any eval signal",
			wantVerdict: VerdictIterate,
			wantSignal:  false,
		},
		{
			name:        "empty output defaults to ITERATE",
			output:      "",
			wantVerdict: VerdictIterate,
			wantSignal:  false,
		},
		{
			name:         "ADVANCE with signal",
			output:       "AGENTIUM_EVAL: ADVANCE",
			wantVerdict:  VerdictAdvance,
			wantSignal:   true,
			wantFeedback: "",
		},
		{
			name:         "ITERATE with feedback",
			output:       "AGENTIUM_EVAL: ITERATE fix compilation errors",
			wantVerdict:  VerdictIterate,
			wantSignal:   true,
			wantFeedback: "fix compilation errors",
		},
		{
			name:         "BLOCKED with reason",
			output:       "AGENTIUM_EVAL: BLOCKED needs human review",
			wantVerdict:  VerdictBlocked,
			wantSignal:   true,
			wantFeedback: "needs human review",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseJudgeVerdict(tt.output)
			if result.Verdict != tt.wantVerdict {
				t.Errorf("Verdict = %q, want %q", result.Verdict, tt.wantVerdict)
			}
			if result.SignalFound != tt.wantSignal {
				t.Errorf("SignalFound = %v, want %v", result.SignalFound, tt.wantSignal)
			}
			if result.Feedback != tt.wantFeedback {
				t.Errorf("Feedback = %q, want %q", result.Feedback, tt.wantFeedback)
			}
		})
	}
}

func TestParseEvalVerdict_SignalFound(t *testing.T) {
	// parseEvalVerdict (legacy) should also set SignalFound
	result := parseEvalVerdict("AGENTIUM_EVAL: ADVANCE")
	if !result.SignalFound {
		t.Error("parseEvalVerdict with signal should set SignalFound=true")
	}

	result = parseEvalVerdict("no signal here")
	if result.SignalFound {
		t.Error("parseEvalVerdict without signal should set SignalFound=false")
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
		CompletedPhase: PhaseTest,
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

func TestParseReviewModeSignal(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "FULL signal",
			output: "Some analysis\nAGENTIUM_REVIEW_MODE: FULL\nDone.",
			want:   "FULL",
		},
		{
			name:   "SIMPLE signal",
			output: "AGENTIUM_REVIEW_MODE: SIMPLE\n",
			want:   "SIMPLE",
		},
		{
			name:   "no signal",
			output: "Just some output without the signal",
			want:   "",
		},
		{
			name:   "empty output",
			output: "",
			want:   "",
		},
		{
			name:   "invalid value",
			output: "AGENTIUM_REVIEW_MODE: UNKNOWN",
			want:   "",
		},
		{
			name:   "not at start of line",
			output: "prefix AGENTIUM_REVIEW_MODE: FULL",
			want:   "",
		},
		{
			name:   "trailing whitespace",
			output: "AGENTIUM_REVIEW_MODE: FULL   \n",
			want:   "FULL",
		},
		{
			name:   "mixed with eval signal",
			output: "AGENTIUM_EVAL: ADVANCE\nAGENTIUM_REVIEW_MODE: SIMPLE\n",
			want:   "SIMPLE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseReviewModeSignal(tt.output)
			if got != tt.want {
				t.Errorf("parseReviewModeSignal() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildJudgePrompt_AssessComplexity(t *testing.T) {
	c := &Controller{
		config:     SessionConfig{Repository: "github.com/org/repo"},
		activeTask: "42",
	}

	params := judgeRunParams{
		CompletedPhase:   PhasePlan,
		PhaseOutput:      "plan output",
		ReviewFeedback:   "feedback here",
		Iteration:        1,
		MaxIterations:    3,
		AssessComplexity: true,
	}

	prompt := c.buildJudgePrompt(params)

	contains := []string{
		"Complexity Assessment",
		"AGENTIUM_REVIEW_MODE: FULL",
		"AGENTIUM_REVIEW_MODE: SIMPLE",
		"multiple files",
		"single-file changes",
	}
	for _, substr := range contains {
		if !containsString(prompt, substr) {
			t.Errorf("buildJudgePrompt(AssessComplexity=true) missing %q", substr)
		}
	}
}

func TestBuildJudgePrompt_NoAssessComplexity(t *testing.T) {
	c := &Controller{
		config:     SessionConfig{Repository: "github.com/org/repo"},
		activeTask: "42",
	}

	params := judgeRunParams{
		CompletedPhase:   PhasePlan,
		PhaseOutput:      "plan output",
		ReviewFeedback:   "feedback here",
		Iteration:        1,
		MaxIterations:    3,
		AssessComplexity: false,
	}

	prompt := c.buildJudgePrompt(params)

	if containsString(prompt, "Complexity Assessment") {
		t.Error("buildJudgePrompt(AssessComplexity=false) should NOT contain complexity section")
	}
	if containsString(prompt, "AGENTIUM_REVIEW_MODE") {
		t.Error("buildJudgePrompt(AssessComplexity=false) should NOT mention AGENTIUM_REVIEW_MODE")
	}
}

func TestBuildEvalPrompt_CustomBudget(t *testing.T) {
	c := &Controller{
		config: SessionConfig{
			Repository: "github.com/org/repo",
			PhaseLoop:  &PhaseLoopConfig{Enabled: true, EvalContextBudget: 100},
		},
		activeTask: "1",
	}

	// Create output longer than custom 100 chars
	longOutput := ""
	for i := 0; i < 20; i++ {
		longOutput += "0123456789"
	}

	prompt := c.buildEvalPrompt(PhaseImplement, longOutput)
	if !containsString(prompt, "output truncated") {
		t.Error("expected truncation marker with custom budget")
	}
}
