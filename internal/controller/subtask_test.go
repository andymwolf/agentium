package controller

import (
	"encoding/json"
	"testing"

	"github.com/andywolf/agentium/internal/routing"
)

func TestPhaseToSubTask_AllConfiguredPhases(t *testing.T) {
	expected := map[TaskPhase]SubTaskType{
		PhasePlan:       SubTaskPlan,
		PhaseImplement:  SubTaskImplement,
		PhaseTest:       SubTaskTest,
		PhaseReview:     SubTaskReview,
		PhasePRCreation: SubTaskPRCreation,
		PhaseAnalyze:    SubTaskPlan,
		PhasePush:       SubTaskPush,
	}

	for phase, want := range expected {
		got, ok := phaseToSubTask[phase]
		if !ok {
			t.Errorf("phaseToSubTask missing phase %s", phase)
			continue
		}
		if got != want {
			t.Errorf("phaseToSubTask[%s] = %s, want %s", phase, got, want)
		}
	}
}

func TestPhaseToSubTask_UnconfiguredPhases(t *testing.T) {
	unconfigured := []TaskPhase{PhaseComplete, PhaseBlocked, PhaseNothingToDo}
	for _, phase := range unconfigured {
		if _, ok := phaseToSubTask[phase]; ok {
			t.Errorf("phaseToSubTask should not contain phase %s", phase)
		}
	}
}

func TestDelegationConfig_JSONRoundTrip(t *testing.T) {
	original := DelegationConfig{
		Enabled:  true,
		Strategy: "sequential",
		SubAgents: map[SubTaskType]SubTaskConfig{
			SubTaskImplement: {
				Agent:  "claude-code",
				Model:  &routing.ModelConfig{Adapter: "claude-code", Model: "claude-opus-4-20250514"},
				Skills: []string{"implement", "test"},
			},
			SubTaskTest: {
				Agent: "aider",
			},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded DelegationConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if !decoded.Enabled {
		t.Error("Enabled should be true")
	}
	if decoded.Strategy != "sequential" {
		t.Errorf("Strategy = %q, want %q", decoded.Strategy, "sequential")
	}
	if len(decoded.SubAgents) != 2 {
		t.Fatalf("SubAgents length = %d, want 2", len(decoded.SubAgents))
	}

	implCfg := decoded.SubAgents[SubTaskImplement]
	if implCfg.Agent != "claude-code" {
		t.Errorf("implement agent = %q, want %q", implCfg.Agent, "claude-code")
	}
	if implCfg.Model == nil || implCfg.Model.Model != "claude-opus-4-20250514" {
		t.Errorf("implement model unexpected: %+v", implCfg.Model)
	}
	if len(implCfg.Skills) != 2 || implCfg.Skills[0] != "implement" {
		t.Errorf("implement skills = %v, want [implement test]", implCfg.Skills)
	}

	testCfg := decoded.SubAgents[SubTaskTest]
	if testCfg.Agent != "aider" {
		t.Errorf("test agent = %q, want %q", testCfg.Agent, "aider")
	}
}

func TestPhaseToSubTask_PRCreationDistinctFromReview(t *testing.T) {
	prType, prOk := phaseToSubTask[PhasePRCreation]
	reviewType, reviewOk := phaseToSubTask[PhaseReview]

	if !prOk {
		t.Fatal("phaseToSubTask missing PhasePRCreation")
	}
	if !reviewOk {
		t.Fatal("phaseToSubTask missing PhaseReview")
	}
	if prType == reviewType {
		t.Errorf("PhasePRCreation and PhaseReview should map to distinct subtask types, both map to %s", prType)
	}
	if prType != SubTaskPRCreation {
		t.Errorf("PhasePRCreation maps to %s, want %s", prType, SubTaskPRCreation)
	}
	if reviewType != SubTaskReview {
		t.Errorf("PhaseReview maps to %s, want %s", reviewType, SubTaskReview)
	}
}

func TestPhaseToSubTask_AllPhasesRouteCorrectly(t *testing.T) {
	tests := []struct {
		phase   TaskPhase
		want    SubTaskType
		present bool
	}{
		{PhasePlan, SubTaskPlan, true},
		{PhaseImplement, SubTaskImplement, true},
		{PhaseTest, SubTaskTest, true},
		{PhaseReview, SubTaskReview, true},
		{PhaseDocs, SubTaskDocs, true},
		{PhasePRCreation, SubTaskPRCreation, true},
		{PhaseAnalyze, SubTaskPlan, true},
		{PhasePush, SubTaskPush, true},
		{PhaseComplete, "", false},
		{PhaseBlocked, "", false},
		{PhaseNothingToDo, "", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.phase), func(t *testing.T) {
			got, ok := phaseToSubTask[tt.phase]
			if ok != tt.present {
				t.Errorf("phaseToSubTask[%s] present = %v, want %v", tt.phase, ok, tt.present)
				return
			}
			if tt.present && got != tt.want {
				t.Errorf("phaseToSubTask[%s] = %s, want %s", tt.phase, got, tt.want)
			}
		})
	}
}

func TestPhaseToSubTask_MapCompleteness(t *testing.T) {
	// Verify the map has exactly the expected number of entries
	expectedLen := 8 // Plan, Implement, Test, Review, Docs, PRCreation, Analyze, Push
	if len(phaseToSubTask) != expectedLen {
		t.Errorf("phaseToSubTask has %d entries, want %d", len(phaseToSubTask), expectedLen)
	}
}

func TestSubTaskConfig_NilModel(t *testing.T) {
	cfg := SubTaskConfig{
		Agent:  "claude-code",
		Skills: []string{"safety"},
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded SubTaskConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.Model != nil {
		t.Errorf("Model should be nil, got %+v", decoded.Model)
	}
	if decoded.Agent != "claude-code" {
		t.Errorf("Agent = %q, want %q", decoded.Agent, "claude-code")
	}
}
