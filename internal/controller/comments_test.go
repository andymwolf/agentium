package controller

import (
	"context"
	"log"
	"os"
	"testing"

	"github.com/andywolf/agentium/internal/agent"
)

// testLogger returns a logger suitable for tests
func testLogger() *log.Logger {
	return log.New(os.Stdout, "[test] ", log.LstdFlags)
}

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

func TestPostPRComment_EmptyPRNumber(t *testing.T) {
	// postPRComment should be a no-op when PR number is empty
	c := &Controller{
		activeTaskType: "issue",
		activeTask:     "42",
		logger:         testLogger(),
	}

	// Should not panic or crash - just return silently
	c.postPRComment(context.Background(), "", "test comment")
}

func TestPostReviewerFeedback_OnlyForIssues(t *testing.T) {
	// postReviewerFeedback should be a no-op for PR tasks
	c := &Controller{
		activeTaskType: "pr",
		activeTask:     "57",
		logger:         testLogger(),
	}

	// Should not panic or crash - just return silently
	c.postReviewerFeedback(context.Background(), PhaseImplement, 1, "test feedback")
}

func TestPostPRJudgeVerdict_SkipsAdvance(t *testing.T) {
	// postPRJudgeVerdict should be a no-op for ADVANCE verdict
	c := &Controller{
		activeTaskType: "issue",
		activeTask:     "42",
		logger:         testLogger(),
	}

	// Should not attempt to post for ADVANCE verdict
	c.postPRJudgeVerdict(context.Background(), "123", PhaseImplement, JudgeResult{Verdict: VerdictAdvance})
}

func TestPostPRJudgeVerdict_EmptyPRNumber(t *testing.T) {
	// postPRJudgeVerdict should be a no-op when PR number is empty
	c := &Controller{
		activeTaskType: "issue",
		activeTask:     "42",
		logger:         testLogger(),
	}

	// Should not panic or crash - just return silently
	c.postPRJudgeVerdict(context.Background(), "", PhaseImplement, JudgeResult{Verdict: VerdictIterate, Feedback: "needs work"})
}

func TestGetPRNumberForTask(t *testing.T) {
	tests := []struct {
		name           string
		existingWork   *agent.ExistingWork
		taskStates     map[string]*TaskState
		activeTaskType string
		activeTask     string
		wantPRNumber   string
	}{
		{
			name:           "no existing work, no task state",
			existingWork:   nil,
			taskStates:     map[string]*TaskState{},
			activeTaskType: "issue",
			activeTask:     "42",
			wantPRNumber:   "",
		},
		{
			name: "existing work with PR",
			existingWork: &agent.ExistingWork{
				PRNumber: "100",
			},
			taskStates:     map[string]*TaskState{},
			activeTaskType: "issue",
			activeTask:     "42",
			wantPRNumber:   "100",
		},
		{
			name:         "task state with PR",
			existingWork: nil,
			taskStates: map[string]*TaskState{
				"issue:42": {PRNumber: "200"},
			},
			activeTaskType: "issue",
			activeTask:     "42",
			wantPRNumber:   "200",
		},
		{
			name: "existing work takes precedence",
			existingWork: &agent.ExistingWork{
				PRNumber: "100",
			},
			taskStates: map[string]*TaskState{
				"issue:42": {PRNumber: "200"},
			},
			activeTaskType: "issue",
			activeTask:     "42",
			wantPRNumber:   "100",
		},
		{
			name:           "PR task returns active task directly",
			existingWork:   nil,
			taskStates:     map[string]*TaskState{},
			activeTaskType: "pr",
			activeTask:     "57",
			wantPRNumber:   "57",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Controller{
				activeTaskType:         tt.activeTaskType,
				activeTask:             tt.activeTask,
				taskStates:             tt.taskStates,
				activeTaskExistingWork: tt.existingWork,
				logger:                 testLogger(),
			}

			got := c.getPRNumberForTask()
			if got != tt.wantPRNumber {
				t.Errorf("getPRNumberForTask() = %q, want %q", got, tt.wantPRNumber)
			}
		})
	}
}

func TestPostReviewFeedbackForPhase_Routing(t *testing.T) {
	tests := []struct {
		name           string
		phase          TaskPhase
		activeTaskType string
		wantSkipped    bool // True if we expect the function to return early
	}{
		{
			name:           "PR tasks are skipped",
			phase:          PhaseImplement,
			activeTaskType: "pr",
			wantSkipped:    true,
		},
		{
			name:           "PLAN phase for issue",
			phase:          PhasePlan,
			activeTaskType: "issue",
			wantSkipped:    false,
		},
		{
			name:           "IMPLEMENT phase for issue",
			phase:          PhaseImplement,
			activeTaskType: "issue",
			wantSkipped:    false,
		},
		{
			name:           "DOCS phase for issue",
			phase:          PhaseDocs,
			activeTaskType: "issue",
			wantSkipped:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Controller{
				activeTaskType: tt.activeTaskType,
				activeTask:     "42",
				taskStates:     map[string]*TaskState{},
				logger:         testLogger(),
			}

			// This should not panic - we're testing that it handles missing gh CLI gracefully
			c.postReviewFeedbackForPhase(context.Background(), tt.phase, 1, "test feedback")
		})
	}
}
