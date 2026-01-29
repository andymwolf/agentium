package controller

import "testing"

func TestParseComplexityVerdict(t *testing.T) {
	tests := []struct {
		name         string
		output       string
		wantVerdict  ComplexityVerdict
		wantFeedback string
		wantSignal   bool
	}{
		{
			name:         "SIMPLE verdict",
			output:       "Analysis...\nAGENTIUM_EVAL: SIMPLE single file change",
			wantVerdict:  ComplexitySimple,
			wantFeedback: "single file change",
			wantSignal:   true,
		},
		{
			name:         "COMPLEX verdict",
			output:       "AGENTIUM_EVAL: COMPLEX multiple components and architectural decisions",
			wantVerdict:  ComplexityComplex,
			wantFeedback: "multiple components and architectural decisions",
			wantSignal:   true,
		},
		{
			name:         "SIMPLE without feedback",
			output:       "AGENTIUM_EVAL: SIMPLE",
			wantVerdict:  ComplexitySimple,
			wantFeedback: "",
			wantSignal:   true,
		},
		{
			name:         "COMPLEX without feedback",
			output:       "AGENTIUM_EVAL: COMPLEX",
			wantVerdict:  ComplexityComplex,
			wantFeedback: "",
			wantSignal:   true,
		},
		{
			name:         "no signal defaults to COMPLEX (fail-closed)",
			output:       "Some output without any eval signal",
			wantVerdict:  ComplexityComplex,
			wantFeedback: "",
			wantSignal:   false,
		},
		{
			name:         "empty output defaults to COMPLEX",
			output:       "",
			wantVerdict:  ComplexityComplex,
			wantFeedback: "",
			wantSignal:   false,
		},
		{
			name:         "ADVANCE is not a complexity verdict",
			output:       "AGENTIUM_EVAL: ADVANCE",
			wantVerdict:  ComplexityComplex,
			wantFeedback: "",
			wantSignal:   false,
		},
		{
			name:         "ITERATE is not a complexity verdict",
			output:       "AGENTIUM_EVAL: ITERATE",
			wantVerdict:  ComplexityComplex,
			wantFeedback: "",
			wantSignal:   false,
		},
		{
			name:         "BLOCKED is not a complexity verdict",
			output:       "AGENTIUM_EVAL: BLOCKED",
			wantVerdict:  ComplexityComplex,
			wantFeedback: "",
			wantSignal:   false,
		},
		{
			name:         "multiple verdicts - first wins",
			output:       "AGENTIUM_EVAL: SIMPLE quick fix\nAGENTIUM_EVAL: COMPLEX",
			wantVerdict:  ComplexitySimple,
			wantFeedback: "quick fix",
			wantSignal:   true,
		},
		{
			name:         "verdict not at start of line is ignored",
			output:       "prefix AGENTIUM_EVAL: SIMPLE\nAGENTIUM_EVAL: COMPLEX real verdict",
			wantVerdict:  ComplexityComplex,
			wantFeedback: "real verdict",
			wantSignal:   true,
		},
		{
			name:         "SIMPLE with trailing whitespace",
			output:       "AGENTIUM_EVAL: SIMPLE   ",
			wantVerdict:  ComplexitySimple,
			wantFeedback: "",
			wantSignal:   true,
		},
		// Markdown fence stripping tests
		{
			name:         "verdict inside markdown code fence is detected",
			output:       "Here is my assessment:\n```\nAGENTIUM_EVAL: SIMPLE one file change\n```",
			wantVerdict:  ComplexitySimple,
			wantFeedback: "one file change",
			wantSignal:   true,
		},
		{
			name:         "verdict inside markdown fence with language tag",
			output:       "```text\nAGENTIUM_EVAL: COMPLEX multiple components\n```",
			wantVerdict:  ComplexityComplex,
			wantFeedback: "multiple components",
			wantSignal:   true,
		},
		{
			name:         "raw verdict preferred over fenced verdict",
			output:       "AGENTIUM_EVAL: SIMPLE quick fix\n```\nAGENTIUM_EVAL: COMPLEX should not match\n```",
			wantVerdict:  ComplexitySimple,
			wantFeedback: "quick fix",
			wantSignal:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseComplexityVerdict(tt.output)
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

func TestBuildComplexityPrompt(t *testing.T) {
	c := &Controller{
		config:     SessionConfig{Repository: "github.com/org/repo"},
		activeTask: "42",
	}

	params := complexityRunParams{
		PlanOutput:    "plan content here",
		Iteration:     1,
		MaxIterations: 3,
	}

	prompt := c.buildComplexityPrompt(params)

	contains := []string{
		"complexity assessor",
		"PLAN",
		"github.com/org/repo",
		"#42",
		"plan content here",
		"AGENTIUM_EVAL:",
		"SIMPLE",
		"COMPLEX",
		"When in doubt, choose COMPLEX",
	}
	for _, substr := range contains {
		if !containsString(prompt, substr) {
			t.Errorf("buildComplexityPrompt() missing %q", substr)
		}
	}
}
