package controller

import (
	"testing"

	"github.com/andywolf/agentium/internal/routing"
)

func TestConfigForPhase_UnconfiguredPhases(t *testing.T) {
	orch := &SubTaskOrchestrator{
		config: DelegationConfig{
			Enabled:   true,
			Strategy:  "sequential",
			SubAgents: map[SubTaskType]SubTaskConfig{
				SubTaskImplement: {Agent: "claude-code"},
			},
		},
	}

	// PhaseComplete is not in phaseToSubTask
	if cfg := orch.ConfigForPhase(PhaseComplete); cfg != nil {
		t.Errorf("ConfigForPhase(COMPLETE) = %+v, want nil", cfg)
	}

	// PhaseBlocked is not in phaseToSubTask
	if cfg := orch.ConfigForPhase(PhaseBlocked); cfg != nil {
		t.Errorf("ConfigForPhase(BLOCKED) = %+v, want nil", cfg)
	}

	// PhaseTest maps to SubTaskTest, but not configured in SubAgents
	if cfg := orch.ConfigForPhase(PhaseTest); cfg != nil {
		t.Errorf("ConfigForPhase(TEST) = %+v, want nil (not in SubAgents)", cfg)
	}
}

func TestConfigForPhase_ConfiguredPhases(t *testing.T) {
	implModel := &routing.ModelConfig{Adapter: "claude-code", Model: "claude-opus-4-20250514"}
	testModel := &routing.ModelConfig{Model: "claude-sonnet-4-20250514"}

	orch := &SubTaskOrchestrator{
		config: DelegationConfig{
			Enabled:  true,
			Strategy: "sequential",
			SubAgents: map[SubTaskType]SubTaskConfig{
				SubTaskImplement: {Agent: "claude-code", Model: implModel, Skills: []string{"implement"}},
				SubTaskTest:      {Agent: "aider", Model: testModel},
				SubTaskReview:    {Agent: "claude-code", Skills: []string{"pr_review"}},
				SubTaskPlan:      {Agent: "claude-code", Skills: []string{"planning"}},
				SubTaskPush:      {Agent: "claude-code", Skills: []string{"push"}},
			},
		},
	}

	tests := []struct {
		phase     TaskPhase
		wantAgent string
		wantModel string
	}{
		{PhaseImplement, "claude-code", "claude-opus-4-20250514"},
		{PhaseTest, "aider", "claude-sonnet-4-20250514"},
		{PhasePRCreation, "claude-code", ""},  // review type
		{PhaseAnalyze, "claude-code", ""},      // plan type
		{PhasePush, "claude-code", ""},          // push type
	}

	for _, tt := range tests {
		t.Run(string(tt.phase), func(t *testing.T) {
			cfg := orch.ConfigForPhase(tt.phase)
			if cfg == nil {
				t.Fatalf("ConfigForPhase(%s) = nil, want config", tt.phase)
			}
			if cfg.Agent != tt.wantAgent {
				t.Errorf("Agent = %q, want %q", cfg.Agent, tt.wantAgent)
			}
			if tt.wantModel != "" {
				if cfg.Model == nil || cfg.Model.Model != tt.wantModel {
					t.Errorf("Model = %+v, want model %q", cfg.Model, tt.wantModel)
				}
			}
		})
	}
}

func TestConfigForPhase_PartialDelegation(t *testing.T) {
	// Only configure implement and test, leaving review/plan unconfigured
	orch := &SubTaskOrchestrator{
		config: DelegationConfig{
			Enabled:  true,
			Strategy: "sequential",
			SubAgents: map[SubTaskType]SubTaskConfig{
				SubTaskImplement: {Agent: "claude-code"},
				SubTaskTest:      {Agent: "aider"},
			},
		},
	}

	// Configured phases should return config
	if cfg := orch.ConfigForPhase(PhaseImplement); cfg == nil {
		t.Error("ConfigForPhase(IMPLEMENT) should return config")
	}
	if cfg := orch.ConfigForPhase(PhaseTest); cfg == nil {
		t.Error("ConfigForPhase(TEST) should return config")
	}

	// Unconfigured phases should return nil
	if cfg := orch.ConfigForPhase(PhasePRCreation); cfg != nil {
		t.Errorf("ConfigForPhase(PR_CREATION) = %+v, want nil (review not configured)", cfg)
	}
	if cfg := orch.ConfigForPhase(PhaseAnalyze); cfg != nil {
		t.Errorf("ConfigForPhase(ANALYZE) = %+v, want nil (plan not configured)", cfg)
	}
	if cfg := orch.ConfigForPhase(PhasePush); cfg != nil {
		t.Errorf("ConfigForPhase(PUSH) = %+v, want nil (push not configured)", cfg)
	}
}
