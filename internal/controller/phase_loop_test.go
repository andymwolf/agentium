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
			PhaseLoop: &PhaseLoopConfig{Enabled: true},
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
				Enabled:                true,
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
				Enabled:                true,
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
		{"disabled", &PhaseLoopConfig{Enabled: false}, false},
		{"enabled", &PhaseLoopConfig{Enabled: true}, true},
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
				Enabled:            true,
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

func TestTruncateForComment(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantRunes int // expected rune count of result
	}{
		{"short string", "hello", 5},
		{"exactly 500 runes", string(make([]byte, 500)), 500},
		{"over 500 runes", string(make([]byte, 600)), 503}, // 500 + "..."
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateForComment(tt.input)
			if len([]rune(result)) != tt.wantRunes {
				t.Errorf("truncateForComment() rune count = %d, want %d", len([]rune(result)), tt.wantRunes)
			}
		})
	}
}

func TestTruncateForComment_UTF8Safety(t *testing.T) {
	// Build a string of 600 multi-byte runes (each is 3 bytes in UTF-8)
	runes := make([]rune, 600)
	for i := range runes {
		runes[i] = '\u4e16' // 世 (3 bytes per rune)
	}
	input := string(runes)

	result := truncateForComment(input)

	// Should have 500 runes + "..." (503 runes total), not split mid-character
	resultRunes := []rune(result)
	if len(resultRunes) != 503 {
		t.Errorf("rune count = %d, want 503", len(resultRunes))
	}
	// First 500 runes should all be 世
	for i := 0; i < 500; i++ {
		if resultRunes[i] != '\u4e16' {
			t.Errorf("rune[%d] = %U, want U+4E16", i, resultRunes[i])
			break
		}
	}
	// Verify valid UTF-8 (Go strings are always valid, but check no replacement chars)
	if resultRunes[500] != '.' {
		t.Errorf("expected '.' at position 500, got %U", resultRunes[500])
	}
}

func TestTruncateForPlan(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantRunes int // expected rune count of result
	}{
		{"short string", "hello", 5},
		{"exactly 4000 runes", string(make([]byte, 4000)), 4000},
		{"over 4000 runes", string(make([]byte, 5000)), 4000 + len("\n\n... (plan truncated)")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateForPlan(tt.input)
			if len([]rune(result)) != tt.wantRunes {
				t.Errorf("truncateForPlan() rune count = %d, want %d", len([]rune(result)), tt.wantRunes)
			}
		})
	}
}

func TestTruncateForPlan_UTF8Safety(t *testing.T) {
	// Build a string of 5000 multi-byte runes (each is 3 bytes in UTF-8)
	runes := make([]rune, 5000)
	for i := range runes {
		runes[i] = '\u4e16' // 世 (3 bytes per rune)
	}
	input := string(runes)

	result := truncateForPlan(input)

	// Should have 4000 runes + truncation message, not split mid-character
	resultRunes := []rune(result)
	truncationMsg := "\n\n... (plan truncated)"
	expectedLen := 4000 + len([]rune(truncationMsg))
	if len(resultRunes) != expectedLen {
		t.Errorf("rune count = %d, want %d", len(resultRunes), expectedLen)
	}
	// First 4000 runes should all be 世
	for i := 0; i < 4000; i++ {
		if resultRunes[i] != '\u4e16' {
			t.Errorf("rune[%d] = %U, want U+4E16", i, resultRunes[i])
			break
		}
	}
}

func TestBuildWorkerHandoffSummary_DisabledHandoff(t *testing.T) {
	c := &Controller{
		config: SessionConfig{
			Handoff: struct {
				Enabled bool `json:"enabled,omitempty"`
			}{Enabled: false},
		},
		handoffStore: nil,
	}

	result := c.buildWorkerHandoffSummary("issue:123", PhasePlan, 1)
	if result != "" {
		t.Errorf("expected empty result when handoff disabled, got %q", result)
	}
}

func TestBuildWorkerHandoffSummary_NoHandoffStore(t *testing.T) {
	c := &Controller{
		config: SessionConfig{
			Handoff: struct {
				Enabled bool `json:"enabled,omitempty"`
			}{Enabled: true},
		},
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
		config: SessionConfig{
			Handoff: struct {
				Enabled bool `json:"enabled,omitempty"`
			}{Enabled: true},
		},
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
		{"disabled phase loop", &PhaseLoopConfig{Enabled: false, SkipPlanIfExists: true}, false},
		{"enabled but skip disabled", &PhaseLoopConfig{Enabled: true, SkipPlanIfExists: false}, false},
		{"both enabled", &PhaseLoopConfig{Enabled: true, SkipPlanIfExists: true}, true},
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
			name:      "PLAN phase, iter 1, config enabled, has plan - should skip",
			phase:     PhasePlan,
			iter:      1,
			config:    &PhaseLoopConfig{Enabled: true, SkipPlanIfExists: true},
			issueBody: issueWithPlan,
			want:      true,
		},
		// Iteration 2+ should NEVER skip, even with plan
		{
			name:      "PLAN phase, iter 2, config enabled, has plan - should NOT skip",
			phase:     PhasePlan,
			iter:      2,
			config:    &PhaseLoopConfig{Enabled: true, SkipPlanIfExists: true},
			issueBody: issueWithPlan,
			want:      false,
		},
		{
			name:      "PLAN phase, iter 3, config enabled, has plan - should NOT skip",
			phase:     PhasePlan,
			iter:      3,
			config:    &PhaseLoopConfig{Enabled: true, SkipPlanIfExists: true},
			issueBody: issueWithPlan,
			want:      false,
		},
		// Non-PLAN phases should never skip
		{
			name:      "IMPLEMENT phase, iter 1, config enabled, has plan - should NOT skip",
			phase:     PhaseImplement,
			iter:      1,
			config:    &PhaseLoopConfig{Enabled: true, SkipPlanIfExists: true},
			issueBody: issueWithPlan,
			want:      false,
		},
		{
			name:      "DOCS phase, iter 1, config enabled, has plan - should NOT skip",
			phase:     PhaseDocs,
			iter:      1,
			config:    &PhaseLoopConfig{Enabled: true, SkipPlanIfExists: true},
			issueBody: issueWithPlan,
			want:      false,
		},
		// Config disabled should not skip
		{
			name:      "PLAN phase, iter 1, skip disabled, has plan - should NOT skip",
			phase:     PhasePlan,
			iter:      1,
			config:    &PhaseLoopConfig{Enabled: true, SkipPlanIfExists: false},
			issueBody: issueWithPlan,
			want:      false,
		},
		{
			name:      "PLAN phase, iter 1, phase loop disabled, has plan - should NOT skip",
			phase:     PhasePlan,
			iter:      1,
			config:    &PhaseLoopConfig{Enabled: false, SkipPlanIfExists: true},
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
			name:      "PLAN phase, iter 1, config enabled, no plan - should NOT skip",
			phase:     PhasePlan,
			iter:      1,
			config:    &PhaseLoopConfig{Enabled: true, SkipPlanIfExists: true},
			issueBody: issueWithoutPlan,
			want:      false,
		},
		// Empty issue body should not skip
		{
			name:      "PLAN phase, iter 1, config enabled, empty body - should NOT skip",
			phase:     PhasePlan,
			iter:      1,
			config:    &PhaseLoopConfig{Enabled: true, SkipPlanIfExists: true},
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
