package controller

import (
	"testing"

	"github.com/andywolf/agentium/internal/routing"
)

func TestConfigForPhase_UnconfiguredPhases(t *testing.T) {
	orch := &SubTaskOrchestrator{
		config: DelegationConfig{
			Enabled:  true,
			Strategy: "sequential",
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
}

func TestConfigForPhase_ConfiguredPhases(t *testing.T) {
	implModel := &routing.ModelConfig{Adapter: "claude-code", Model: "claude-opus-4-20250514"}
	docsModel := &routing.ModelConfig{Model: "claude-sonnet-4-20250514"}

	orch := &SubTaskOrchestrator{
		config: DelegationConfig{
			Enabled:  true,
			Strategy: "sequential",
			SubAgents: map[SubTaskType]SubTaskConfig{
				SubTaskImplement: {Agent: "claude-code", Model: implModel, Skills: []string{"implement"}},
				SubTaskDocs:      {Agent: "aider", Model: docsModel},
				SubTaskPlan:      {Agent: "claude-code", Skills: []string{"plan"}},
			},
		},
	}

	tests := []struct {
		phase     TaskPhase
		wantAgent string
		wantModel string
	}{
		{PhaseImplement, "claude-code", "claude-opus-4-20250514"},
		{PhaseDocs, "aider", "claude-sonnet-4-20250514"},
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
	// Only configure implement, leaving plan/docs unconfigured
	orch := &SubTaskOrchestrator{
		config: DelegationConfig{
			Enabled:  true,
			Strategy: "sequential",
			SubAgents: map[SubTaskType]SubTaskConfig{
				SubTaskImplement: {Agent: "claude-code"},
			},
		},
	}

	// Configured phases should return config
	if cfg := orch.ConfigForPhase(PhaseImplement); cfg == nil {
		t.Error("ConfigForPhase(IMPLEMENT) should return config")
	}

	// Unconfigured phases should return nil
	if cfg := orch.ConfigForPhase(PhasePlan); cfg != nil {
		t.Errorf("ConfigForPhase(PLAN) = %+v, want nil (plan not configured)", cfg)
	}
	if cfg := orch.ConfigForPhase(PhaseDocs); cfg != nil {
		t.Errorf("ConfigForPhase(DOCS) = %+v, want nil (docs not configured)", cfg)
	}
}
