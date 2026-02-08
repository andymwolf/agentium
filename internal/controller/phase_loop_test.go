package controller

import (
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
