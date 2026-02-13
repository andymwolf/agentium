package controller

import (
	"testing"

	"github.com/andywolf/agentium/internal/agent"
	"github.com/andywolf/agentium/internal/routing"
)

func TestDelegation_AdapterSelection_Default(t *testing.T) {
	defaultAgent := &mockAgent{name: "default-agent"}
	c := &Controller{
		agent:    defaultAgent,
		adapters: map[string]agent.Agent{"default-agent": defaultAgent},
	}

	// SubTaskConfig with empty Agent should use default
	cfg := &SubTaskConfig{Agent: ""}
	orch := NewSubTaskOrchestrator(DelegationConfig{
		Enabled:   true,
		SubAgents: map[SubTaskType]SubTaskConfig{SubTaskImplement: *cfg},
	}, c)

	subCfg := orch.ConfigForPhase(PhaseImplement)
	if subCfg == nil {
		t.Fatal("expected config for IMPLEMENT phase")
	}
	if subCfg.Agent != "" {
		t.Errorf("expected empty agent (use default), got %q", subCfg.Agent)
	}
}

func TestDelegation_AdapterSelection_Override(t *testing.T) {
	defaultAgent := &mockAgent{name: "default-agent"}
	overrideAgent := &mockAgent{name: "override-agent"}
	c := &Controller{
		agent: defaultAgent,
		adapters: map[string]agent.Agent{
			"default-agent":  defaultAgent,
			"override-agent": overrideAgent,
		},
	}

	cfg := &SubTaskConfig{Agent: "override-agent"}
	orch := NewSubTaskOrchestrator(DelegationConfig{
		Enabled:   true,
		SubAgents: map[SubTaskType]SubTaskConfig{SubTaskImplement: *cfg},
	}, c)

	subCfg := orch.ConfigForPhase(PhaseImplement)
	if subCfg == nil {
		t.Fatal("expected config for IMPLEMENT phase")
	}
	if subCfg.Agent != "override-agent" {
		t.Errorf("agent = %q, want %q", subCfg.Agent, "override-agent")
	}
}

func TestDelegation_ModelOverride(t *testing.T) {
	model := &routing.ModelConfig{Adapter: "claude-code", Model: "claude-opus-4-20250514"}
	cfg := SubTaskConfig{
		Model: model,
	}

	if cfg.Model == nil || cfg.Model.Model != "claude-opus-4-20250514" {
		t.Errorf("Model override not set correctly: %+v", cfg.Model)
	}
	if cfg.Model.Adapter != "claude-code" {
		t.Errorf("Model adapter = %q, want %q", cfg.Model.Adapter, "claude-code")
	}
}

func TestDelegation_ModelOverride_Nil(t *testing.T) {
	cfg := SubTaskConfig{}
	if cfg.Model != nil {
		t.Errorf("Model should be nil, got %+v", cfg.Model)
	}
}

func TestDelegation_ControllerRoutesCorrectly(t *testing.T) {
	defaultAgent := &mockAgent{name: "default"}
	delegatedAgent := &mockAgent{name: "delegated"}

	c := &Controller{
		agent: defaultAgent,
		adapters: map[string]agent.Agent{
			"default":   defaultAgent,
			"delegated": delegatedAgent,
		},
		config: SessionConfig{
			Delegation: &DelegationConfig{
				Enabled:  true,
				Strategy: "sequential",
				SubAgents: map[SubTaskType]SubTaskConfig{
					SubTaskImplement: {Agent: "delegated"},
				},
			},
		},
		taskStates: map[string]*TaskState{
			"issue:1": {ID: "1", Type: "issue", Phase: PhaseImplement},
		},
		activeTask:     "1",
		activeTaskType: "issue",
	}
	c.orchestrator = NewSubTaskOrchestrator(*c.config.Delegation, c)

	// Phase IMPLEMENT should be routed to delegation
	phase := c.determineActivePhase()
	subCfg := c.orchestrator.ConfigForPhase(phase)
	if subCfg == nil {
		t.Fatal("expected delegation config for IMPLEMENT")
	}
	if subCfg.Agent != "delegated" {
		t.Errorf("delegated agent = %q, want %q", subCfg.Agent, "delegated")
	}

	// Phase DOCS should NOT be routed (not configured)
	c.taskStates["issue:1"].Phase = PhaseDocs
	phase = c.determineActivePhase()
	subCfg = c.orchestrator.ConfigForPhase(phase)
	if subCfg != nil {
		t.Errorf("expected nil config for DOCS phase, got %+v", subCfg)
	}
}
