package controller

import (
	"testing"

	"github.com/andywolf/agentium/internal/handoff"
)

func TestAdvancePhase(t *testing.T) {
	tests := []struct {
		name    string
		current TaskPhase
		want    TaskPhase
	}{
		{"PLAN advances to IMPLEMENT", PhasePlan, PhaseImplement},
		{"IMPLEMENT advances to DOCS", PhaseImplement, PhaseDocs},
		{"DOCS advances to COMPLETE", PhaseDocs, PhaseComplete},
		{"unknown phase advances to COMPLETE", TaskPhase("UNKNOWN"), PhaseComplete},
		{"COMPLETE stays COMPLETE", PhaseComplete, PhaseComplete},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := advancePhase(tt.current)
			if got != tt.want {
				t.Errorf("advancePhase(%q) = %q, want %q", tt.current, got, tt.want)
			}
		})
	}
}

func TestPhaseMaxIterations_Defaults(t *testing.T) {
	c := &Controller{
		config: SessionConfig{
			PhaseLoop: &PhaseLoopConfig{},
		},
	}

	tests := []struct {
		phase TaskPhase
		want  int
	}{
		{PhasePlan, defaultPlanMaxIter},
		{PhaseImplement, defaultImplementMaxIter},
		{PhaseDocs, defaultDocsMaxIter},
		{TaskPhase("UNKNOWN"), 1},
	}

	for _, tt := range tests {
		t.Run(string(tt.phase), func(t *testing.T) {
			got := c.phaseMaxIterations(tt.phase, WorkflowPathUnset)
			if got != tt.want {
				t.Errorf("phaseMaxIterations(%q, UNSET) = %d, want %d", tt.phase, got, tt.want)
			}
		})
	}
}

func TestPhaseMaxIterations_CustomConfig(t *testing.T) {
	c := &Controller{
		config: SessionConfig{
			PhaseLoop: &PhaseLoopConfig{
				PlanMaxIterations:      2,
				ImplementMaxIterations: 10,
				DocsMaxIterations:      4,
			},
		},
	}

	tests := []struct {
		phase TaskPhase
		want  int
	}{
		{PhasePlan, 2},
		{PhaseImplement, 10},
		{PhaseDocs, 4},
	}

	for _, tt := range tests {
		t.Run(string(tt.phase), func(t *testing.T) {
			got := c.phaseMaxIterations(tt.phase, WorkflowPathComplex)
			if got != tt.want {
				t.Errorf("phaseMaxIterations(%q, COMPLEX) = %d, want %d", tt.phase, got, tt.want)
			}
		})
	}
}

func TestPhaseLoopConfig_Nil(t *testing.T) {
	c := &Controller{
		config: SessionConfig{
			PhaseLoop: nil,
		},
	}

	// Should use defaults when config is nil
	if got := c.phaseMaxIterations(PhasePlan, WorkflowPathUnset); got != defaultPlanMaxIter {
		t.Errorf("phaseMaxIterations(PLAN, UNSET) with nil config = %d, want %d", got, defaultPlanMaxIter)
	}
}

func TestPhaseMaxIterations_SimplePath(t *testing.T) {
	c := &Controller{
		config: SessionConfig{
			PhaseLoop: &PhaseLoopConfig{
				PlanMaxIterations:      5, // Should be ignored for SIMPLE path
				ImplementMaxIterations: 10,
				DocsMaxIterations:      4,
			},
		},
	}

	tests := []struct {
		phase TaskPhase
		want  int
	}{
		{PhasePlan, simplePlanMaxIter},
		{PhaseImplement, simpleImplementMaxIter},
		{PhaseDocs, simpleDocsMaxIter},
		{TaskPhase("UNKNOWN"), 1},
	}

	for _, tt := range tests {
		t.Run(string(tt.phase), func(t *testing.T) {
			got := c.phaseMaxIterations(tt.phase, WorkflowPathSimple)
			if got != tt.want {
				t.Errorf("phaseMaxIterations(%q, SIMPLE) = %d, want %d", tt.phase, got, tt.want)
			}
		})
	}
}

func TestIsPhaseLoopEnabled(t *testing.T) {
	tests := []struct {
		name   string
		config *PhaseLoopConfig
		want   bool
	}{
		{"nil config", nil, false},
		{"empty config", &PhaseLoopConfig{}, true},
		{"config with values", &PhaseLoopConfig{PlanMaxIterations: 3}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Controller{config: SessionConfig{PhaseLoop: tt.config}}
			if got := c.isPhaseLoopEnabled(); got != tt.want {
				t.Errorf("isPhaseLoopEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIssuePhaseOrder(t *testing.T) {
	// Verify the expected phase order (REVIEW and PR_CREATION phases removed)
	// Draft PRs are created during IMPLEMENT and finalized at PhaseComplete
	expected := []TaskPhase{PhasePlan, PhaseImplement, PhaseDocs}
	if len(issuePhaseOrder) != len(expected) {
		t.Fatalf("issuePhaseOrder length = %d, want %d", len(issuePhaseOrder), len(expected))
	}
	for i, phase := range expected {
		if issuePhaseOrder[i] != phase {
			t.Errorf("issuePhaseOrder[%d] = %q, want %q", i, issuePhaseOrder[i], phase)
		}
	}
}

func TestJudgeNoSignalLimit_Default(t *testing.T) {
	c := &Controller{config: SessionConfig{}}
	if got := c.judgeNoSignalLimit(); got != defaultJudgeNoSignalLimit {
		t.Errorf("judgeNoSignalLimit() = %d, want %d", got, defaultJudgeNoSignalLimit)
	}
}

func TestJudgeNoSignalLimit_Custom(t *testing.T) {
	c := &Controller{
		config: SessionConfig{
			PhaseLoop: &PhaseLoopConfig{
				JudgeNoSignalLimit: 5,
			},
		},
	}
	if got := c.judgeNoSignalLimit(); got != 5 {
		t.Errorf("judgeNoSignalLimit() = %d, want 5", got)
	}
}

func TestJudgeNoSignalLimit_NilConfig(t *testing.T) {
	c := &Controller{config: SessionConfig{PhaseLoop: nil}}
	if got := c.judgeNoSignalLimit(); got != defaultJudgeNoSignalLimit {
		t.Errorf("judgeNoSignalLimit() with nil config = %d, want %d", got, defaultJudgeNoSignalLimit)
	}
}

func TestBuildWorkerHandoffSummary_NoHandoffStore(t *testing.T) {
	c := &Controller{
		config:       SessionConfig{},
		handoffStore: nil,
	}

	result := c.buildWorkerHandoffSummary("issue:123", PhasePlan, 1)
	if result != "" {
		t.Errorf("expected empty result when handoff store is nil, got %q", result)
	}
}

func TestBuildWorkerHandoffSummary_SkipsStaleIteration(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := handoff.NewStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create handoff store: %v", err)
	}

	// Store handoff data from iteration 1
	taskID := "issue:123"
	_ = store.StorePhaseOutput(taskID, handoff.PhasePlan, 1, &handoff.PlanOutput{
		Summary: "Plan from iteration 1",
	})

	c := &Controller{
		config:       SessionConfig{},
		handoffStore: store,
	}

	// Request summary for iteration 2 - should return empty since data is from iteration 1
	result := c.buildWorkerHandoffSummary(taskID, PhasePlan, 2)
	if result != "" {
		t.Errorf("expected empty result for stale iteration, got %q", result)
	}

	// Request summary for iteration 1 - should return the data
	result = c.buildWorkerHandoffSummary(taskID, PhasePlan, 1)
	if result == "" {
		t.Error("expected non-empty result for current iteration")
	}
	if result != "Summary: Plan from iteration 1" {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestHasExistingPlan(t *testing.T) {
	tests := []struct {
		name      string
		issueBody string
		want      bool
	}{
		{"empty body", "", false},
		{"no indicators", "This is a simple issue description.", false},
		{"has Files to Create/Modify", "## Plan\n\n| File | Action |\n|------|--------|\nFiles to Create/Modify\n| foo.go | Add |", true},
		{"has Files to Modify", "Some text\n\nFiles to Modify:\n- foo.go", true},
		{"has Implementation Steps", "## Implementation Steps\n1. Do this\n2. Do that", true},
		{"has Implementation Plan header", "## Implementation Plan\nDetailed plan here...", true},
		{"case sensitive - lowercase", "files to modify", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Controller{
				activeTask: "123",
				issueDetails: []issueDetail{
					{Number: 123, Title: "Test Issue", Body: tt.issueBody},
				},
			}
			got := c.hasExistingPlan()
			if got != tt.want {
				t.Errorf("hasExistingPlan() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractExistingPlan(t *testing.T) {
	tests := []struct {
		name      string
		issueBody string
		wantEmpty bool
	}{
		{"no plan", "Simple issue", true},
		{"has plan", "## Implementation Plan\nDo stuff", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Controller{
				activeTask: "456",
				issueDetails: []issueDetail{
					{Number: 456, Title: "Test", Body: tt.issueBody},
				},
			}
			got := c.extractExistingPlan()
			if tt.wantEmpty && got != "" {
				t.Errorf("extractExistingPlan() = %q, want empty", got)
			}
			if !tt.wantEmpty && got != tt.issueBody {
				t.Errorf("extractExistingPlan() = %q, want %q", got, tt.issueBody)
			}
		})
	}
}

func TestGetActiveIssueBody(t *testing.T) {
	c := &Controller{
		activeTask: "789",
		issueDetails: []issueDetail{
			{Number: 123, Title: "Other Issue", Body: "Other body"},
			{Number: 789, Title: "Active Issue", Body: "Active body"},
		},
	}

	got := c.getActiveIssueBody()
	if got != "Active body" {
		t.Errorf("getActiveIssueBody() = %q, want %q", got, "Active body")
	}

	// Test with non-matching active task
	c.activeTask = "999"
	got = c.getActiveIssueBody()
	if got != "" {
		t.Errorf("getActiveIssueBody() for non-existent task = %q, want empty", got)
	}
}

func TestIsPlanSkipEnabled(t *testing.T) {
	tests := []struct {
		name   string
		config *PhaseLoopConfig
		want   bool
	}{
		{"nil config", nil, false},
		{"skip disabled", &PhaseLoopConfig{SkipPlanIfExists: false}, false},
		{"skip enabled", &PhaseLoopConfig{SkipPlanIfExists: true}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Controller{config: SessionConfig{PhaseLoop: tt.config}}
			if got := c.isPlanSkipEnabled(); got != tt.want {
				t.Errorf("isPlanSkipEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExistingPlanIndicators(t *testing.T) {
	// Verify all expected indicators are present
	expectedIndicators := []string{
		"Files to Create/Modify",
		"Files to Modify",
		"Implementation Steps",
		"## Implementation Plan",
	}

	if len(existingPlanIndicators) != len(expectedIndicators) {
		t.Errorf("existingPlanIndicators has %d items, want %d", len(existingPlanIndicators), len(expectedIndicators))
	}

	for _, expected := range expectedIndicators {
		found := false
		for _, actual := range existingPlanIndicators {
			if actual == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing expected indicator: %q", expected)
		}
	}
}

func TestShouldSkipPlanIteration(t *testing.T) {
	// Issue body with plan indicator
	issueWithPlan := "## Implementation Plan\nStep 1: Do this\nStep 2: Do that"
	// Issue body without plan indicator
	issueWithoutPlan := "Please implement feature X"

	tests := []struct {
		name      string
		phase     TaskPhase
		iter      int
		config    *PhaseLoopConfig
		issueBody string
		want      bool
	}{
		// Iteration 1 with plan should skip
		{
			name:      "PLAN phase, iter 1, skip enabled, has plan - should skip",
			phase:     PhasePlan,
			iter:      1,
			config:    &PhaseLoopConfig{SkipPlanIfExists: true},
			issueBody: issueWithPlan,
			want:      true,
		},
		// Iteration 2+ should NEVER skip, even with plan
		{
			name:      "PLAN phase, iter 2, skip enabled, has plan - should NOT skip",
			phase:     PhasePlan,
			iter:      2,
			config:    &PhaseLoopConfig{SkipPlanIfExists: true},
			issueBody: issueWithPlan,
			want:      false,
		},
		{
			name:      "PLAN phase, iter 3, skip enabled, has plan - should NOT skip",
			phase:     PhasePlan,
			iter:      3,
			config:    &PhaseLoopConfig{SkipPlanIfExists: true},
			issueBody: issueWithPlan,
			want:      false,
		},
		// Non-PLAN phases should never skip
		{
			name:      "IMPLEMENT phase, iter 1, skip enabled, has plan - should NOT skip",
			phase:     PhaseImplement,
			iter:      1,
			config:    &PhaseLoopConfig{SkipPlanIfExists: true},
			issueBody: issueWithPlan,
			want:      false,
		},
		{
			name:      "DOCS phase, iter 1, skip enabled, has plan - should NOT skip",
			phase:     PhaseDocs,
			iter:      1,
			config:    &PhaseLoopConfig{SkipPlanIfExists: true},
			issueBody: issueWithPlan,
			want:      false,
		},
		// Config disabled should not skip
		{
			name:      "PLAN phase, iter 1, skip disabled, has plan - should NOT skip",
			phase:     PhasePlan,
			iter:      1,
			config:    &PhaseLoopConfig{SkipPlanIfExists: false},
			issueBody: issueWithPlan,
			want:      false,
		},
		{
			name:      "PLAN phase, iter 1, nil config, has plan - should NOT skip",
			phase:     PhasePlan,
			iter:      1,
			config:    nil,
			issueBody: issueWithPlan,
			want:      false,
		},
		// No plan in issue should not skip
		{
			name:      "PLAN phase, iter 1, skip enabled, no plan - should NOT skip",
			phase:     PhasePlan,
			iter:      1,
			config:    &PhaseLoopConfig{SkipPlanIfExists: true},
			issueBody: issueWithoutPlan,
			want:      false,
		},
		// Empty issue body should not skip
		{
			name:      "PLAN phase, iter 1, skip enabled, empty body - should NOT skip",
			phase:     PhasePlan,
			iter:      1,
			config:    &PhaseLoopConfig{SkipPlanIfExists: true},
			issueBody: "",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Controller{
				activeTask: "123",
				issueDetails: []issueDetail{
					{Number: 123, Title: "Test Issue", Body: tt.issueBody},
				},
				config: SessionConfig{PhaseLoop: tt.config},
			}
			got := c.shouldSkipPlanIteration(tt.phase, tt.iter)
			if got != tt.want {
				t.Errorf("shouldSkipPlanIteration(%s, %d) = %v, want %v", tt.phase, tt.iter, got, tt.want)
			}
		})
	}
}
