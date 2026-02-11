package controller

import (
	"context"
	"strings"
	"testing"

	"github.com/andywolf/agentium/internal/handoff"
)

func TestAdvancePhase(t *testing.T) {
	c := &Controller{config: SessionConfig{}}
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
			got := c.advancePhase(tt.current)
			if got != tt.want {
				t.Errorf("advancePhase(%q) = %q, want %q", tt.current, got, tt.want)
			}
		})
	}
}

func TestAdvancePhase_WithAutoMerge(t *testing.T) {
	c := &Controller{config: SessionConfig{AutoMerge: true}}
	tests := []struct {
		name    string
		current TaskPhase
		want    TaskPhase
	}{
		{"PLAN advances to IMPLEMENT", PhasePlan, PhaseImplement},
		{"IMPLEMENT advances to DOCS", PhaseImplement, PhaseDocs},
		{"DOCS advances to VERIFY", PhaseDocs, PhaseVerify},
		{"VERIFY advances to COMPLETE", PhaseVerify, PhaseComplete},
		{"unknown phase advances to COMPLETE", TaskPhase("UNKNOWN"), PhaseComplete},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.advancePhase(tt.current)
			if got != tt.want {
				t.Errorf("advancePhase(%q) = %q, want %q", tt.current, got, tt.want)
			}
		})
	}
}

func TestPhaseOrder_WithAutoMerge(t *testing.T) {
	c := &Controller{config: SessionConfig{AutoMerge: true}}
	order := c.phaseOrder()
	expected := []TaskPhase{PhasePlan, PhaseImplement, PhaseDocs, PhaseVerify}
	if len(order) != len(expected) {
		t.Fatalf("phaseOrder() length = %d, want %d", len(order), len(expected))
	}
	for i, phase := range expected {
		if order[i] != phase {
			t.Errorf("phaseOrder()[%d] = %q, want %q", i, order[i], phase)
		}
	}
}

func TestPhaseOrder_WithoutAutoMerge(t *testing.T) {
	c := &Controller{config: SessionConfig{}}
	order := c.phaseOrder()
	expected := []TaskPhase{PhasePlan, PhaseImplement, PhaseDocs}
	if len(order) != len(expected) {
		t.Fatalf("phaseOrder() length = %d, want %d", len(order), len(expected))
	}
	for i, phase := range expected {
		if order[i] != phase {
			t.Errorf("phaseOrder()[%d] = %q, want %q", i, order[i], phase)
		}
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
		{PhaseVerify, defaultVerifyMaxIter},
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
		{PhaseVerify, simpleVerifyMaxIter},
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

func TestPhaseMaxIterations_VerifyCustomConfig(t *testing.T) {
	c := &Controller{
		config: SessionConfig{
			PhaseLoop: &PhaseLoopConfig{
				VerifyMaxIterations: 5,
			},
		},
	}

	got := c.phaseMaxIterations(PhaseVerify, WorkflowPathComplex)
	if got != 5 {
		t.Errorf("phaseMaxIterations(VERIFY, COMPLEX) = %d, want 5", got)
	}
}

func TestPhaseMaxIterations_VerifyDefault(t *testing.T) {
	c := &Controller{
		config: SessionConfig{
			PhaseLoop: &PhaseLoopConfig{},
		},
	}

	got := c.phaseMaxIterations(PhaseVerify, WorkflowPathUnset)
	if got != defaultVerifyMaxIter {
		t.Errorf("phaseMaxIterations(VERIFY, UNSET) = %d, want %d", got, defaultVerifyMaxIter)
	}
}

func TestExtractPlanFromIssueBody_FullPlan(t *testing.T) {
	body := `## Summary
Add authentication middleware to the API server.

## Files to Modify
- ` + "`" + `internal/server/router.go` + "`" + `
- ` + "`" + `internal/middleware/auth.go` + "`" + `

## Files to Create
- ` + "`" + `internal/middleware/jwt.go` + "`" + `

## Implementation Steps
1. Add JWT validation helper in jwt.go
2. Create auth middleware function in auth.go
3. Wire middleware into router.go

## Testing
Run go test ./internal/middleware/... and verify 401 on unauthenticated requests.
`

	plan := extractPlanFromIssueBody(body)
	if plan == nil {
		t.Fatal("expected non-nil plan")
	}

	if plan.Summary != "Add authentication middleware to the API server." {
		t.Errorf("Summary = %q", plan.Summary)
	}

	if len(plan.FilesToModify) != 2 {
		t.Errorf("FilesToModify count = %d, want 2", len(plan.FilesToModify))
	} else {
		if plan.FilesToModify[0] != "internal/server/router.go" {
			t.Errorf("FilesToModify[0] = %q", plan.FilesToModify[0])
		}
	}

	if len(plan.FilesToCreate) != 1 {
		t.Errorf("FilesToCreate count = %d, want 1", len(plan.FilesToCreate))
	} else if plan.FilesToCreate[0] != "internal/middleware/jwt.go" {
		t.Errorf("FilesToCreate[0] = %q", plan.FilesToCreate[0])
	}

	if len(plan.ImplementationSteps) != 3 {
		t.Errorf("ImplementationSteps count = %d, want 3", len(plan.ImplementationSteps))
	} else {
		if plan.ImplementationSteps[0].Order != 1 {
			t.Errorf("Step[0].Order = %d, want 1", plan.ImplementationSteps[0].Order)
		}
		if plan.ImplementationSteps[0].Description != "Add JWT validation helper in jwt.go" {
			t.Errorf("Step[0].Description = %q", plan.ImplementationSteps[0].Description)
		}
	}

	if plan.TestingApproach == "" {
		t.Error("expected non-empty TestingApproach")
	}
}

func TestExtractPlanFromIssueBody_EmptyBody(t *testing.T) {
	plan := extractPlanFromIssueBody("")
	if plan != nil {
		t.Errorf("expected nil plan for empty body, got %+v", plan)
	}
}

func TestExtractPlanFromIssueBody_NoPlanStructure(t *testing.T) {
	body := "Please fix the login button. It doesn't work when clicked."
	plan := extractPlanFromIssueBody(body)
	if plan != nil {
		t.Errorf("expected nil plan for unstructured body, got %+v", plan)
	}
}

func TestExtractPlanFromIssueBody_PartialPlan(t *testing.T) {
	body := `## Summary
Fix the login button click handler.

## Implementation Steps
1. Update the onClick handler in LoginButton.tsx
2. Add error handling for failed auth requests
`

	plan := extractPlanFromIssueBody(body)
	if plan == nil {
		t.Fatal("expected non-nil plan for partial plan")
	}
	if plan.Summary == "" {
		t.Error("expected non-empty Summary")
	}
	if len(plan.ImplementationSteps) != 2 {
		t.Errorf("ImplementationSteps count = %d, want 2", len(plan.ImplementationSteps))
	}
}

func TestFormatPlanForComment_FallbackCapsLength(t *testing.T) {
	// Generate a long raw output (500 lines)
	var lines []string
	for i := 0; i < 500; i++ {
		lines = append(lines, "This is a line of agent output that needs to be capped")
	}
	longOutput := strings.Join(lines, "\n")

	c := &Controller{
		config:       SessionConfig{},
		handoffStore: nil, // no handoff store → fallback path
	}

	result := c.formatPlanForComment("issue:123", longOutput)

	// Result should be shorter than the input (capped at 200 lines)
	resultLines := strings.Split(result, "\n")
	if len(resultLines) > 210 { // allow some margin for truncation message
		t.Errorf("formatPlanForComment fallback produced %d lines, expected ≤ ~200", len(resultLines))
	}
}

func TestExtractFilePaths(t *testing.T) {
	content := `
- ` + "`" + `internal/server/router.go` + "`" + `
- ` + "`" + `internal/middleware/auth.go` + "`" + `
- internal/config/config.go
- ` + "`" + `internal/server/router.go` + "`" + ` (duplicate)
`

	paths := extractFilePaths(content)

	// Should deduplicate
	if len(paths) != 3 {
		t.Errorf("extractFilePaths() returned %d paths, want 3: %v", len(paths), paths)
	}
}

func TestExtractSteps(t *testing.T) {
	content := `
1. First step description
2. Second step description
3. Third step with more detail
`

	steps := extractSteps(content)
	if len(steps) != 3 {
		t.Fatalf("extractSteps() returned %d steps, want 3", len(steps))
	}
	if steps[0].Order != 1 || steps[0].Description != "First step description" {
		t.Errorf("steps[0] = %+v", steps[0])
	}
	if steps[2].Order != 3 || steps[2].Description != "Third step with more detail" {
		t.Errorf("steps[2] = %+v", steps[2])
	}
}

func TestParseStepNumber(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"1", 1},
		{"10", 10},
		{"0", 0},
		{"abc", 0},
		{"", 0},
	}
	for _, tt := range tests {
		got := parseStepNumber(tt.input)
		if got != tt.want {
			t.Errorf("parseStepNumber(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestSplitMarkdownSections(t *testing.T) {
	body := `Some preamble text

## Summary
This is the summary.

## Files to Modify
- file1.go
- file2.go

# Top-Level Heading
Content under top-level heading.

## Implementation Steps
1. Do something
2. Do another thing
`

	sections := splitMarkdownSections(body)

	if len(sections) != 4 {
		t.Errorf("splitMarkdownSections() returned %d sections, want 4: %v", len(sections), sections)
	}

	if summary, ok := sections["Summary"]; !ok {
		t.Error("missing 'Summary' section")
	} else if !strings.Contains(summary, "This is the summary.") {
		t.Errorf("Summary content = %q", summary)
	}

	if files, ok := sections["Files to Modify"]; !ok {
		t.Error("missing 'Files to Modify' section")
	} else if !strings.Contains(files, "file1.go") {
		t.Errorf("Files to Modify content = %q", files)
	}

	if topLevel, ok := sections["Top-Level Heading"]; !ok {
		t.Error("missing 'Top-Level Heading' section")
	} else if !strings.Contains(topLevel, "Content under top-level heading.") {
		t.Errorf("Top-Level Heading content = %q", topLevel)
	}

	if steps, ok := sections["Implementation Steps"]; !ok {
		t.Error("missing 'Implementation Steps' section")
	} else if !strings.Contains(steps, "Do something") {
		t.Errorf("Implementation Steps content = %q", steps)
	}

	// Preamble text (before any heading) should not appear in any section
	for heading, content := range sections {
		if strings.Contains(content, "Some preamble text") {
			t.Errorf("preamble text leaked into section %q", heading)
		}
	}
}

func TestTryVerifyMerge_HandoffMergeSuccessful(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := handoff.NewStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create handoff store: %v", err)
	}

	taskID := "issue:123"
	_ = store.StorePhaseOutput(taskID, handoff.PhaseVerify, 1, &handoff.VerifyOutput{
		MergeSuccessful: true,
		MergeSHA:        "abc123",
	})

	c := &Controller{
		config:       SessionConfig{},
		handoffStore: store,
		logger:       newTestLogger(),
	}

	state := &TaskState{PRNumber: "42"}
	got, failures := c.tryVerifyMerge(context.Background(), taskID, state)
	if !got {
		t.Error("tryVerifyMerge() merged = false, want true when handoff reports merge successful")
	}
	if len(failures) != 0 {
		t.Errorf("tryVerifyMerge() remainingFailures = %v, want nil on successful merge", failures)
	}
}

func TestTryVerifyMerge_HandoffChecksPassed_ControllerMerge(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := handoff.NewStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create handoff store: %v", err)
	}

	taskID := "issue:456"
	_ = store.StorePhaseOutput(taskID, handoff.PhaseVerify, 1, &handoff.VerifyOutput{
		ChecksPassed:    true,
		MergeSuccessful: false,
	})

	c := &Controller{
		config: SessionConfig{
			Repository: "nonexistent-org/nonexistent-repo-xyzzy",
		},
		handoffStore: store,
		logger:       newTestLogger(),
	}

	// Controller merge will fail (nonexistent repo),
	// so tryVerifyMerge returns false — this is the expected fallback behavior.
	state := &TaskState{PRNumber: "99999"}
	got, failures := c.tryVerifyMerge(context.Background(), taskID, state)
	if got {
		t.Error("tryVerifyMerge() merged = true, want false when controller merge fails")
	}
	if len(failures) != 0 {
		t.Errorf("tryVerifyMerge() remainingFailures = %v, want nil when checks passed", failures)
	}
}

func TestTryVerifyMerge_HandoffChecksFailed(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := handoff.NewStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create handoff store: %v", err)
	}

	taskID := "issue:789"
	_ = store.StorePhaseOutput(taskID, handoff.PhaseVerify, 1, &handoff.VerifyOutput{
		ChecksPassed:      false,
		MergeSuccessful:   false,
		RemainingFailures: []string{"lint", "test"},
	})

	c := &Controller{
		config:       SessionConfig{},
		handoffStore: store,
		logger:       newTestLogger(),
	}

	state := &TaskState{PRNumber: "42"}
	got, failures := c.tryVerifyMerge(context.Background(), taskID, state)
	if got {
		t.Error("tryVerifyMerge() merged = true, want false when CI checks have not passed")
	}
	if len(failures) != 2 || failures[0] != "lint" || failures[1] != "test" {
		t.Errorf("tryVerifyMerge() remainingFailures = %v, want [lint test]", failures)
	}
}

func TestTryVerifyMerge_NoHandoffData(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := handoff.NewStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create handoff store: %v", err)
	}

	c := &Controller{
		config: SessionConfig{
			Repository: "nonexistent-org/nonexistent-repo-xyzzy",
		},
		handoffStore: store,
		logger:       newTestLogger(),
	}

	// No handoff data stored — controller tries merge directly, which fails (nonexistent repo).
	state := &TaskState{PRNumber: "99999"}
	got, failures := c.tryVerifyMerge(context.Background(), "issue:999", state)
	if got {
		t.Error("tryVerifyMerge() merged = true, want false when no handoff data and merge fails")
	}
	if len(failures) != 0 {
		t.Errorf("tryVerifyMerge() remainingFailures = %v, want nil when no handoff data", failures)
	}
}

// Tests for skip_on condition evaluation (issue #420)

func TestEvaluateSkipCondition_EmptyOutput(t *testing.T) {
	c := &Controller{
		config: SessionConfig{},
		logger: newTestLogger(),
	}

	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{"empty string", "", true},
		{"whitespace only", "   \n\t  \n  ", true},
		{"has content", "Some output", false},
		{"newlines with content", "\n\nSome content\n\n", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.evaluateSkipCondition(SkipConditionEmptyOutput, tt.output, "issue:123")
			if got != tt.want {
				t.Errorf("evaluateSkipCondition(empty_output, %q) = %v, want %v", tt.output, got, tt.want)
			}
		})
	}
}

func TestEvaluateSkipCondition_SimpleOutput(t *testing.T) {
	c := &Controller{
		config: SessionConfig{},
		logger: newTestLogger(),
	}

	// Generate output with exactly simpleOutputLineThreshold non-empty lines
	var shortLines []string
	for i := 0; i < simpleOutputLineThreshold-1; i++ {
		shortLines = append(shortLines, "Line content")
	}
	shortOutput := strings.Join(shortLines, "\n")

	// Generate output with more than simpleOutputLineThreshold non-empty lines
	var longLines []string
	for i := 0; i < simpleOutputLineThreshold+5; i++ {
		longLines = append(longLines, "Line content")
	}
	longOutput := strings.Join(longLines, "\n")

	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{"empty string", "", true},
		{"single line", "Just one line", true},
		{"few lines", "Line 1\nLine 2\nLine 3", true},
		{"short output", shortOutput, true},
		{"long output", longOutput, false},
		{"many empty lines with few content lines", "\n\n\nLine 1\n\n\nLine 2\n\n\n", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.evaluateSkipCondition(SkipConditionSimpleOutput, tt.output, "issue:123")
			if got != tt.want {
				t.Errorf("evaluateSkipCondition(simple_output, %q) = %v, want %v", tt.output[:min(50, len(tt.output))], got, tt.want)
			}
		})
	}
}

func TestEvaluateSkipCondition_NoCodeChanges(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := handoff.NewStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create handoff store: %v", err)
	}

	// Store handoff data with no files changed
	taskIDNoChanges := "issue:100"
	_ = store.StorePhaseOutput(taskIDNoChanges, handoff.PhaseImplement, 1, &handoff.ImplementOutput{
		BranchName:   "feature/test",
		FilesChanged: []string{},
		TestsPassed:  true,
	})

	// Store handoff data with files changed
	taskIDWithChanges := "issue:200"
	_ = store.StorePhaseOutput(taskIDWithChanges, handoff.PhaseImplement, 1, &handoff.ImplementOutput{
		BranchName:   "feature/test2",
		FilesChanged: []string{"file1.go", "file2.go"},
		TestsPassed:  true,
	})

	c := &Controller{
		config:       SessionConfig{},
		handoffStore: store,
		logger:       newTestLogger(),
	}

	tests := []struct {
		name   string
		taskID string
		want   bool
	}{
		{"no files changed", taskIDNoChanges, true},
		{"files changed", taskIDWithChanges, false},
		{"no handoff data", "issue:999", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.evaluateSkipCondition(SkipConditionNoCodeChanges, "some output", tt.taskID)
			if got != tt.want {
				t.Errorf("evaluateSkipCondition(no_code_changes, task=%s) = %v, want %v", tt.taskID, got, tt.want)
			}
		})
	}
}

func TestEvaluateSkipCondition_UnrecognizedCondition(t *testing.T) {
	c := &Controller{
		config: SessionConfig{},
		logger: newTestLogger(),
	}

	// Unrecognized conditions should return false (safe default: don't skip)
	got := c.evaluateSkipCondition("unknown_condition", "some output", "issue:123")
	if got != false {
		t.Errorf("evaluateSkipCondition(unknown_condition) = %v, want false", got)
	}
}

func TestEvaluateSkipCondition_EmptyCondition(t *testing.T) {
	c := &Controller{
		config: SessionConfig{},
		logger: newTestLogger(),
	}

	// Empty condition should return false (don't skip)
	got := c.evaluateSkipCondition("", "some output", "issue:123")
	if got != false {
		t.Errorf("evaluateSkipCondition('') = %v, want false", got)
	}
}

func TestShouldSkipReviewer_NilConfig(t *testing.T) {
	c := &Controller{
		config: SessionConfig{PhaseLoop: nil},
		logger: newTestLogger(),
	}

	got, reason := c.shouldSkipReviewer("some output", "issue:123")
	if got != false {
		t.Errorf("shouldSkipReviewer with nil PhaseLoop = %v, want false", got)
	}
	if reason != "" {
		t.Errorf("shouldSkipReviewer reason = %q, want empty", reason)
	}
}

func TestShouldSkipReviewer_NoCondition(t *testing.T) {
	c := &Controller{
		config: SessionConfig{
			PhaseLoop: &PhaseLoopConfig{
				ReviewerSkipOn: "",
			},
		},
		logger: newTestLogger(),
	}

	got, reason := c.shouldSkipReviewer("some output", "issue:123")
	if got != false {
		t.Errorf("shouldSkipReviewer with empty ReviewerSkipOn = %v, want false", got)
	}
	if reason != "" {
		t.Errorf("shouldSkipReviewer reason = %q, want empty", reason)
	}
}

func TestShouldSkipReviewer_WithCondition(t *testing.T) {
	c := &Controller{
		config: SessionConfig{
			PhaseLoop: &PhaseLoopConfig{
				ReviewerSkipOn: SkipConditionEmptyOutput,
			},
		},
		logger: newTestLogger(),
	}

	// Empty output should trigger skip
	got, reason := c.shouldSkipReviewer("", "issue:123")
	if got != true {
		t.Errorf("shouldSkipReviewer with empty output = %v, want true", got)
	}
	if reason != SkipConditionEmptyOutput {
		t.Errorf("shouldSkipReviewer reason = %q, want %q", reason, SkipConditionEmptyOutput)
	}

	// Non-empty output should not trigger skip
	got, reason = c.shouldSkipReviewer("some content", "issue:123")
	if got != false {
		t.Errorf("shouldSkipReviewer with content = %v, want false", got)
	}
	if reason != "" {
		t.Errorf("shouldSkipReviewer reason = %q, want empty", reason)
	}
}

func TestShouldSkipReviewer_BooleanSkip(t *testing.T) {
	c := &Controller{
		config: SessionConfig{
			PhaseLoop: &PhaseLoopConfig{
				ReviewerSkip:   true,
				ReviewerSkipOn: SkipConditionEmptyOutput, // Both set, skip=true takes precedence
			},
		},
		logger: newTestLogger(),
	}

	// Boolean skip=true should always skip, regardless of output content
	got, reason := c.shouldSkipReviewer("non-empty content", "issue:123")
	if got != true {
		t.Errorf("shouldSkipReviewer with ReviewerSkip=true = %v, want true", got)
	}
	if reason != "reviewer_skip=true" {
		t.Errorf("shouldSkipReviewer reason = %q, want %q", reason, "reviewer_skip=true")
	}
}

func TestShouldSkipReviewer_BooleanPrecedence(t *testing.T) {
	// Test that skip=true takes precedence over skip_on
	c := &Controller{
		config: SessionConfig{
			PhaseLoop: &PhaseLoopConfig{
				ReviewerSkip:   true,
				ReviewerSkipOn: SkipConditionEmptyOutput,
			},
		},
		logger: newTestLogger(),
	}

	// Even with non-empty output (which wouldn't trigger skip_on), skip=true should skip
	got, reason := c.shouldSkipReviewer("lots of content here", "issue:123")
	if got != true {
		t.Errorf("shouldSkipReviewer: skip=true should take precedence, got %v", got)
	}
	if reason != "reviewer_skip=true" {
		t.Errorf("shouldSkipReviewer reason should indicate boolean skip, got %q", reason)
	}
}

func TestShouldSkipJudge_NilConfig(t *testing.T) {
	c := &Controller{
		config: SessionConfig{PhaseLoop: nil},
		logger: newTestLogger(),
	}

	got, reason := c.shouldSkipJudge("some output", "issue:123")
	if got != false {
		t.Errorf("shouldSkipJudge with nil PhaseLoop = %v, want false", got)
	}
	if reason != "" {
		t.Errorf("shouldSkipJudge reason = %q, want empty", reason)
	}
}

func TestShouldSkipJudge_WithCondition(t *testing.T) {
	c := &Controller{
		config: SessionConfig{
			PhaseLoop: &PhaseLoopConfig{
				JudgeSkipOn: SkipConditionSimpleOutput,
			},
		},
		logger: newTestLogger(),
	}

	// Short output should trigger skip
	got, reason := c.shouldSkipJudge("Line 1\nLine 2", "issue:123")
	if got != true {
		t.Errorf("shouldSkipJudge with short output = %v, want true", got)
	}
	if reason != SkipConditionSimpleOutput {
		t.Errorf("shouldSkipJudge reason = %q, want %q", reason, SkipConditionSimpleOutput)
	}

	// Long output should not trigger skip
	var longLines []string
	for i := 0; i < simpleOutputLineThreshold+5; i++ {
		longLines = append(longLines, "Line content")
	}
	got, reason = c.shouldSkipJudge(strings.Join(longLines, "\n"), "issue:123")
	if got != false {
		t.Errorf("shouldSkipJudge with long output = %v, want false", got)
	}
	if reason != "" {
		t.Errorf("shouldSkipJudge reason = %q, want empty", reason)
	}
}

func TestShouldSkipJudge_BooleanSkip(t *testing.T) {
	c := &Controller{
		config: SessionConfig{
			PhaseLoop: &PhaseLoopConfig{
				JudgeSkip:   true,
				JudgeSkipOn: SkipConditionSimpleOutput, // Both set, skip=true takes precedence
			},
		},
		logger: newTestLogger(),
	}

	// Boolean skip=true should always skip, regardless of output length
	var longLines []string
	for i := 0; i < simpleOutputLineThreshold+5; i++ {
		longLines = append(longLines, "Line content")
	}
	got, reason := c.shouldSkipJudge(strings.Join(longLines, "\n"), "issue:123")
	if got != true {
		t.Errorf("shouldSkipJudge with JudgeSkip=true = %v, want true", got)
	}
	if reason != "judge_skip=true" {
		t.Errorf("shouldSkipJudge reason = %q, want %q", reason, "judge_skip=true")
	}
}

func TestIsOutputEmpty(t *testing.T) {
	c := &Controller{}

	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{"empty", "", true},
		{"spaces", "   ", true},
		{"tabs", "\t\t", true},
		{"newlines", "\n\n\n", true},
		{"mixed whitespace", "  \t\n  \t ", true},
		{"single char", "a", false},
		{"content with whitespace", "  hello  ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.isOutputEmpty(tt.output)
			if got != tt.want {
				t.Errorf("isOutputEmpty(%q) = %v, want %v", tt.output, got, tt.want)
			}
		})
	}
}

func TestIsOutputSimple(t *testing.T) {
	c := &Controller{}

	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{"empty", "", true},
		{"single line", "one line", true},
		{"exactly threshold minus one", strings.Repeat("line\n", simpleOutputLineThreshold-1), true},
		{"exactly threshold", strings.Repeat("line\n", simpleOutputLineThreshold), false},
		{"many empty lines few content", "\n\n\nline1\n\n\nline2\n\n\n", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.isOutputSimple(tt.output)
			if got != tt.want {
				t.Errorf("isOutputSimple(%q...) = %v, want %v", tt.output[:min(20, len(tt.output))], got, tt.want)
			}
		})
	}
}

func TestImplementOutputHasNoCodeChanges_NoHandoffStore(t *testing.T) {
	c := &Controller{
		config:       SessionConfig{},
		handoffStore: nil,
	}

	got := c.implementOutputHasNoCodeChanges("issue:123")
	if got != false {
		t.Errorf("implementOutputHasNoCodeChanges with nil store = %v, want false", got)
	}
}

func TestImplementOutputHasNoCodeChanges_NoHandoffData(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := handoff.NewStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create handoff store: %v", err)
	}

	c := &Controller{
		config:       SessionConfig{},
		handoffStore: store,
	}

	got := c.implementOutputHasNoCodeChanges("issue:999")
	if got != false {
		t.Errorf("implementOutputHasNoCodeChanges with no data = %v, want false", got)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestValidatePhases(t *testing.T) {
	tests := []struct {
		name    string
		phases  []PhaseStepConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "known phases without overrides OK",
			phases: []PhaseStepConfig{
				{Name: "PLAN"},
				{Name: "IMPLEMENT"},
			},
			wantErr: false,
		},
		{
			name: "known phase with overrides OK",
			phases: []PhaseStepConfig{
				{Name: "PLAN", Worker: &StepPromptConfig{Prompt: "custom plan prompt"}},
			},
			wantErr: false,
		},
		{
			name: "unknown phase with worker prompt OK",
			phases: []PhaseStepConfig{
				{Name: "LINT", Worker: &StepPromptConfig{Prompt: "run linting"}},
			},
			wantErr: false,
		},
		{
			name: "unknown phase without worker prompt errors",
			phases: []PhaseStepConfig{
				{Name: "LINT"},
			},
			wantErr: true,
			errMsg:  "unknown phase \"LINT\" requires worker.prompt",
		},
		{
			name: "unknown phase with empty worker prompt errors",
			phases: []PhaseStepConfig{
				{Name: "LINT", Worker: &StepPromptConfig{Prompt: ""}},
			},
			wantErr: true,
			errMsg:  "unknown phase \"LINT\" requires worker.prompt",
		},
		{
			name: "empty phase name errors",
			phases: []PhaseStepConfig{
				{Name: ""},
			},
			wantErr: true,
			errMsg:  "phase name must not be empty",
		},
		{
			name: "duplicate phase name errors",
			phases: []PhaseStepConfig{
				{Name: "PLAN"},
				{Name: "PLAN"},
			},
			wantErr: true,
			errMsg:  "duplicate phase name: PLAN",
		},
		{
			name:    "empty phases list OK",
			phases:  []PhaseStepConfig{},
			wantErr: false,
		},
		{
			name: "mixed known and unknown phases OK",
			phases: []PhaseStepConfig{
				{Name: "PLAN"},
				{Name: "LINT", Worker: &StepPromptConfig{Prompt: "run lint"}},
				{Name: "IMPLEMENT"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePhases(tt.phases)
			if tt.wantErr {
				if err == nil {
					t.Error("validatePhases() expected error, got nil")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("validatePhases() error = %q, want containing %q", err.Error(), tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("validatePhases() unexpected error: %v", err)
			}
		})
	}
}

func TestPhaseOrder_CustomPhases(t *testing.T) {
	tests := []struct {
		name      string
		phases    []PhaseStepConfig
		autoMerge bool
		want      []TaskPhase
	}{
		{
			name: "custom order",
			phases: []PhaseStepConfig{
				{Name: "IMPLEMENT"},
				{Name: "DOCS"},
			},
			want: []TaskPhase{PhaseImplement, PhaseDocs},
		},
		{
			name: "custom with unknown phases",
			phases: []PhaseStepConfig{
				{Name: "LINT", Worker: &StepPromptConfig{Prompt: "lint"}},
				{Name: "IMPLEMENT"},
				{Name: "DEPLOY", Worker: &StepPromptConfig{Prompt: "deploy"}},
			},
			want: []TaskPhase{"LINT", PhaseImplement, "DEPLOY"},
		},
		{
			name: "auto-merge appends VERIFY if not present",
			phases: []PhaseStepConfig{
				{Name: "PLAN"},
				{Name: "IMPLEMENT"},
			},
			autoMerge: true,
			want:      []TaskPhase{PhasePlan, PhaseImplement, PhaseVerify},
		},
		{
			name: "auto-merge does not duplicate VERIFY",
			phases: []PhaseStepConfig{
				{Name: "PLAN"},
				{Name: "VERIFY"},
			},
			autoMerge: true,
			want:      []TaskPhase{PhasePlan, PhaseVerify},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Controller{
				config: SessionConfig{
					Phases:    tt.phases,
					AutoMerge: tt.autoMerge,
				},
			}
			got := c.phaseOrder()
			if len(got) != len(tt.want) {
				t.Fatalf("phaseOrder() length = %d, want %d (got %v)", len(got), len(tt.want), got)
			}
			for i, p := range tt.want {
				if got[i] != p {
					t.Errorf("phaseOrder()[%d] = %q, want %q", i, got[i], p)
				}
			}
		})
	}
}

func TestPhaseOrder_EmptyPhasesFallback(t *testing.T) {
	c := &Controller{config: SessionConfig{}}
	got := c.phaseOrder()
	expected := issuePhaseOrder
	if len(got) != len(expected) {
		t.Fatalf("phaseOrder() with empty phases = %v, want %v", got, expected)
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("phaseOrder()[%d] = %q, want %q", i, got[i], expected[i])
		}
	}
}

func TestPhaseMaxIterations_CustomPhaseOverride(t *testing.T) {
	c := &Controller{
		config: SessionConfig{
			PhaseLoop: &PhaseLoopConfig{
				PlanMaxIterations:      3,
				ImplementMaxIterations: 5,
			},
			Phases: []PhaseStepConfig{
				{Name: "PLAN", MaxIterations: 7},
				{Name: "IMPLEMENT"}, // No override, falls back to PhaseLoopConfig
			},
		},
		phaseConfigs: map[TaskPhase]*PhaseStepConfig{},
	}
	// Build phaseConfigs like New() does
	for i := range c.config.Phases {
		c.phaseConfigs[TaskPhase(c.config.Phases[i].Name)] = &c.config.Phases[i]
	}

	// API override (7) takes precedence over PhaseLoopConfig (3)
	if got := c.phaseMaxIterations(PhasePlan, WorkflowPathUnset); got != 7 {
		t.Errorf("phaseMaxIterations(PLAN) = %d, want 7", got)
	}

	// No API override: falls back to PhaseLoopConfig (5)
	if got := c.phaseMaxIterations(PhaseImplement, WorkflowPathUnset); got != 5 {
		t.Errorf("phaseMaxIterations(IMPLEMENT) = %d, want 5", got)
	}

	// SIMPLE path still takes highest priority
	if got := c.phaseMaxIterations(PhasePlan, WorkflowPathSimple); got != simplePlanMaxIter {
		t.Errorf("phaseMaxIterations(PLAN, SIMPLE) = %d, want %d", got, simplePlanMaxIter)
	}
}

func TestPhaseWorkerPrompt(t *testing.T) {
	c := &Controller{
		phaseConfigs: map[TaskPhase]*PhaseStepConfig{
			PhasePlan: {
				Name:   "PLAN",
				Worker: &StepPromptConfig{Prompt: "custom plan worker prompt"},
			},
			PhaseImplement: {
				Name: "IMPLEMENT",
				// No worker override
			},
		},
	}

	if got := c.phaseWorkerPrompt(PhasePlan); got != "custom plan worker prompt" {
		t.Errorf("phaseWorkerPrompt(PLAN) = %q, want %q", got, "custom plan worker prompt")
	}
	if got := c.phaseWorkerPrompt(PhaseImplement); got != "" {
		t.Errorf("phaseWorkerPrompt(IMPLEMENT) = %q, want empty", got)
	}
	if got := c.phaseWorkerPrompt(PhaseDocs); got != "" {
		t.Errorf("phaseWorkerPrompt(DOCS) = %q, want empty (phase not in map)", got)
	}
}

func TestPhaseReviewerPrompt(t *testing.T) {
	c := &Controller{
		phaseConfigs: map[TaskPhase]*PhaseStepConfig{
			PhasePlan: {
				Name:     "PLAN",
				Reviewer: &StepPromptConfig{Prompt: "custom reviewer prompt"},
			},
		},
	}

	if got := c.phaseReviewerPrompt(PhasePlan); got != "custom reviewer prompt" {
		t.Errorf("phaseReviewerPrompt(PLAN) = %q, want %q", got, "custom reviewer prompt")
	}
	if got := c.phaseReviewerPrompt(PhaseImplement); got != "" {
		t.Errorf("phaseReviewerPrompt(IMPLEMENT) = %q, want empty", got)
	}
}

func TestPhaseJudgeCriteria(t *testing.T) {
	c := &Controller{
		phaseConfigs: map[TaskPhase]*PhaseStepConfig{
			PhasePlan: {
				Name:  "PLAN",
				Judge: &JudgePromptConfig{Criteria: "custom judge criteria"},
			},
		},
	}

	if got := c.phaseJudgeCriteria(PhasePlan); got != "custom judge criteria" {
		t.Errorf("phaseJudgeCriteria(PLAN) = %q, want %q", got, "custom judge criteria")
	}
	if got := c.phaseJudgeCriteria(PhaseImplement); got != "" {
		t.Errorf("phaseJudgeCriteria(IMPLEMENT) = %q, want empty", got)
	}
}

func TestContainsPhase(t *testing.T) {
	phases := []TaskPhase{PhasePlan, PhaseImplement, PhaseDocs}
	if !containsPhase(phases, PhasePlan) {
		t.Error("containsPhase should find PLAN")
	}
	if containsPhase(phases, PhaseVerify) {
		t.Error("containsPhase should not find VERIFY")
	}
	if containsPhase(nil, PhasePlan) {
		t.Error("containsPhase should return false for nil slice")
	}
}

func TestPhaseHelpers_NilPhaseConfigs(t *testing.T) {
	c := &Controller{
		phaseConfigs: nil,
	}

	if got := c.phaseWorkerPrompt(PhasePlan); got != "" {
		t.Errorf("phaseWorkerPrompt with nil map = %q, want empty", got)
	}
	if got := c.phaseReviewerPrompt(PhasePlan); got != "" {
		t.Errorf("phaseReviewerPrompt with nil map = %q, want empty", got)
	}
	if got := c.phaseJudgeCriteria(PhasePlan); got != "" {
		t.Errorf("phaseJudgeCriteria with nil map = %q, want empty", got)
	}
}
