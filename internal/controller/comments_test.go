package controller

import "testing"

func TestPostPhaseComment_OnlyForIssues(t *testing.T) {
	// postPhaseComment should be a no-op for PR tasks
	c := &Controller{
		activeTaskType: "pr",
		activeTask:     "57",
	}

	// Should not panic or crash - just return silently
	c.postPhaseComment(nil, PhaseImplement, 1, "test summary")
}

func TestPostEvalComment_OnlyForIssues(t *testing.T) {
	// postEvalComment should be a no-op for PR tasks
	c := &Controller{
		activeTaskType: "pr",
		activeTask:     "57",
	}

	// Should not panic or crash - just return silently
	c.postEvalComment(nil, PhaseTest, EvalResult{Verdict: VerdictAdvance})
}

func TestEvalResultFormatting(t *testing.T) {
	// Verify EvalResult struct fields are correctly populated
	tests := []struct {
		name     string
		result   EvalResult
		wantStr  string
	}{
		{
			name:    "advance verdict",
			result:  EvalResult{Verdict: VerdictAdvance},
			wantStr: "ADVANCE",
		},
		{
			name:    "iterate with feedback",
			result:  EvalResult{Verdict: VerdictIterate, Feedback: "fix tests"},
			wantStr: "ITERATE",
		},
		{
			name:    "blocked with reason",
			result:  EvalResult{Verdict: VerdictBlocked, Feedback: "need credentials"},
			wantStr: "BLOCKED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.result.Verdict) != tt.wantStr {
				t.Errorf("Verdict = %q, want %q", tt.result.Verdict, tt.wantStr)
			}
		})
	}
}
