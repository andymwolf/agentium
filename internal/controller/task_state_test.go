package controller

import (
	"io"
	"log"
	"testing"

	"github.com/andywolf/agentium/internal/agent"
)

func TestUpdateTaskPhase_PRDetectionFallback(t *testing.T) {
	tests := []struct {
		name         string
		taskType     string
		agentStatus  string
		prsCreated   []string
		initialPhase TaskPhase
		wantPhase    TaskPhase
		wantPR       string
	}{
		{
			name:         "no status signal but PR detected in IMPLEMENT - advances to DOCS",
			taskType:     "issue",
			agentStatus:  "",
			prsCreated:   []string{"110"},
			initialPhase: PhaseImplement,
			wantPhase:    PhaseDocs,
			wantPR:       "110",
		},
		{
			name:         "no status signal but PR detected in DOCS - completes",
			taskType:     "issue",
			agentStatus:  "",
			prsCreated:   []string{"110"},
			initialPhase: PhaseDocs,
			wantPhase:    PhaseComplete,
			wantPR:       "110",
		},
		{
			name:         "no status signal and no PRs - stays in current phase",
			taskType:     "issue",
			agentStatus:  "",
			prsCreated:   nil,
			initialPhase: PhaseImplement,
			wantPhase:    PhaseImplement,
			wantPR:       "",
		},
		{
			name:         "explicit PR_CREATED status takes precedence",
			taskType:     "issue",
			agentStatus:  "PR_CREATED",
			prsCreated:   []string{"110"},
			initialPhase: PhaseDocs,
			wantPhase:    PhaseComplete,
			wantPR:       "", // StatusMessage is used, not PRsCreated
		},
		{
			name:         "explicit COMPLETE status - no fallback needed",
			taskType:     "issue",
			agentStatus:  "COMPLETE",
			prsCreated:   nil,
			initialPhase: PhaseDocs,
			wantPhase:    PhaseComplete,
			wantPR:       "",
		},
		{
			name:         "fallback in IMPLEMENT uses first PR number and advances to DOCS",
			taskType:     "issue",
			agentStatus:  "",
			prsCreated:   []string{"110", "111"},
			initialPhase: PhaseImplement,
			wantPhase:    PhaseDocs,
			wantPR:       "110",
		},
		{
			name:         "PR detected in PLAN phase - no state change",
			taskType:     "issue",
			agentStatus:  "",
			prsCreated:   []string{"110"},
			initialPhase: PhasePlan,
			wantPhase:    PhasePlan,
			wantPR:       "110",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taskID := taskKey(tt.taskType, "24")
			c := &Controller{
				taskStates: map[string]*TaskState{
					taskID: {ID: "24", Type: tt.taskType, Phase: tt.initialPhase},
				},
				logger: log.New(io.Discard, "", 0),
			}

			result := &agent.IterationResult{
				AgentStatus: tt.agentStatus,
				PRsCreated:  tt.prsCreated,
			}

			c.updateTaskPhase(taskID, result)

			state := c.taskStates[taskID]
			if state.Phase != tt.wantPhase {
				t.Errorf("Phase = %q, want %q", state.Phase, tt.wantPhase)
			}
			if state.PRNumber != tt.wantPR {
				t.Errorf("PRNumber = %q, want %q", state.PRNumber, tt.wantPR)
			}
		})
	}
}
