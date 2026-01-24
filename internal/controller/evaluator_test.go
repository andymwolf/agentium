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
