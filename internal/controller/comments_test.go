package controller

import (
	"context"
	"testing"
)

func TestPostPhaseComment_OnlyForIssues(t *testing.T) {
	// postPhaseComment should be a no-op for PR tasks
	c := &Controller{
		activeTaskType: "pr",
		activeTask:     "57",
	}

	// Should not panic or crash - just return silently
	c.postPhaseComment(context.Background(), PhaseImplement, 1, "test summary")
}

func TestPostJudgeComment_OnlyForIssues(t *testing.T) {
	// postJudgeComment should be a no-op for PR tasks
	c := &Controller{
		activeTaskType: "pr",
		activeTask:     "57",
	}

	// Should not panic or crash - just return silently
	c.postJudgeComment(context.Background(), PhaseImplement, JudgeResult{Verdict: VerdictAdvance})
}

func TestJudgeResultFormatting(t *testing.T) {
	// Verify JudgeResult struct fields are correctly populated
	tests := []struct {
		name    string
		result  JudgeResult
		wantStr string
	}{
		{
			name:    "advance verdict",
			result:  JudgeResult{Verdict: VerdictAdvance},
			wantStr: "ADVANCE",
		},
		{
			name:    "iterate with feedback",
			result:  JudgeResult{Verdict: VerdictIterate, Feedback: "fix tests"},
			wantStr: "ITERATE",
		},
		{
			name:    "blocked with reason",
			result:  JudgeResult{Verdict: VerdictBlocked, Feedback: "need credentials"},
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
