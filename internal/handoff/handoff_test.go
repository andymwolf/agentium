package handoff

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStore(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "handoff-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	t.Run("NewStore creates directory", func(t *testing.T) {
		store, err := NewStore(tmpDir)
		if err != nil {
			t.Fatalf("NewStore failed: %v", err)
		}

		// Check .agentium directory was created
		dir := filepath.Join(tmpDir, ".agentium")
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Error(".agentium directory was not created")
		}

		_ = store // use store
	})

	t.Run("SetIssueContext and GetIssueContext", func(t *testing.T) {
		store, _ := NewStore(tmpDir)
		taskID := "issue:123"

		issue := &IssueContext{
			Number:     123,
			Title:      "Test Issue",
			Body:       "Test body",
			Repository: "owner/repo",
		}

		store.SetIssueContext(taskID, issue)
		retrieved := store.GetIssueContext(taskID)

		if retrieved == nil {
			t.Fatal("GetIssueContext returned nil")
		}
		if retrieved.Number != 123 {
			t.Errorf("expected number 123, got %d", retrieved.Number)
		}
		if retrieved.Title != "Test Issue" {
			t.Errorf("expected title 'Test Issue', got %s", retrieved.Title)
		}
	})

	t.Run("StorePhaseOutput and GetPhaseOutput", func(t *testing.T) {
		store, _ := NewStore(tmpDir)
		taskID := "issue:456"

		planOut := &PlanOutput{
			Summary:         "Test summary",
			FilesToModify:   []string{"file1.go"},
			FilesToCreate:   []string{"file2.go"},
			TestingApproach: "unit tests",
			ImplementationSteps: []ImplementationStep{
				{Order: 1, Description: "Step 1"},
			},
		}

		err := store.StorePhaseOutput(taskID, PhasePlan, 1, planOut)
		if err != nil {
			t.Fatalf("StorePhaseOutput failed: %v", err)
		}

		retrieved := store.GetPlanOutput(taskID)
		if retrieved == nil {
			t.Fatal("GetPlanOutput returned nil")
		}
		if retrieved.Summary != "Test summary" {
			t.Errorf("expected summary 'Test summary', got %s", retrieved.Summary)
		}
	})

	t.Run("StorePhaseOutput replaces existing", func(t *testing.T) {
		store, _ := NewStore(tmpDir)
		taskID := "issue:789"

		planOut1 := &PlanOutput{Summary: "First plan"}
		planOut2 := &PlanOutput{Summary: "Second plan"}

		_ = store.StorePhaseOutput(taskID, PhasePlan, 1, planOut1)
		_ = store.StorePhaseOutput(taskID, PhasePlan, 2, planOut2)

		retrieved := store.GetPlanOutput(taskID)
		if retrieved.Summary != "Second plan" {
			t.Errorf("expected 'Second plan', got %s", retrieved.Summary)
		}
	})

	t.Run("ClearFromPhase clears subsequent phases", func(t *testing.T) {
		store, _ := NewStore(tmpDir)
		taskID := "issue:101"

		_ = store.StorePhaseOutput(taskID, PhasePlan, 1, &PlanOutput{Summary: "Plan"})
		_ = store.StorePhaseOutput(taskID, PhaseImplement, 1, &ImplementOutput{BranchName: "feature/test"})
		_ = store.StorePhaseOutput(taskID, PhaseDocs, 1, &DocsOutput{})

		store.ClearFromPhase(taskID, PhaseImplement)

		if store.GetPlanOutput(taskID) == nil {
			t.Error("Plan should still exist after clearing from IMPLEMENT")
		}
		if store.GetImplementOutput(taskID) != nil {
			t.Error("Implement should be cleared")
		}
		if store.GetDocsOutput(taskID) != nil {
			t.Error("Docs should be cleared")
		}
	})

	t.Run("Save and Load persistence", func(t *testing.T) {
		store1, _ := NewStore(tmpDir)
		taskID := "issue:persist"

		store1.SetIssueContext(taskID, &IssueContext{Number: 999, Title: "Persist Test"})
		_ = store1.StorePhaseOutput(taskID, PhasePlan, 1, &PlanOutput{Summary: "Persisted plan"})
		_ = store1.Save()

		// Create new store instance - should load persisted data
		store2, _ := NewStore(tmpDir)
		retrieved := store2.GetIssueContext(taskID)
		if retrieved == nil || retrieved.Number != 999 {
			t.Error("Issue context not persisted")
		}

		plan := store2.GetPlanOutput(taskID)
		if plan == nil || plan.Summary != "Persisted plan" {
			t.Error("Plan output not persisted")
		}
	})
}

func TestBuilder(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "handoff-builder-test")
	defer os.RemoveAll(tmpDir)

	store, _ := NewStore(tmpDir)
	builder := NewBuilder(store)
	taskID := "issue:builder-test"

	// Setup test data
	issue := &IssueContext{
		Number:     42,
		Title:      "Builder Test Issue",
		Body:       "Test body content",
		Repository: "owner/repo",
	}
	store.SetIssueContext(taskID, issue)

	t.Run("BuildInputForPhase PLAN", func(t *testing.T) {
		input, err := builder.BuildInputForPhase(taskID, PhasePlan)
		if err != nil {
			t.Fatalf("BuildInputForPhase failed: %v", err)
		}

		if input == "" {
			t.Error("Input should not be empty")
		}
	})

	t.Run("BuildInputForPhase IMPLEMENT requires plan", func(t *testing.T) {
		_, err := builder.BuildInputForPhase(taskID, PhaseImplement)
		if err == nil {
			t.Error("Expected error when plan output is missing")
		}
	})

	t.Run("BuildInputForPhase IMPLEMENT with plan", func(t *testing.T) {
		_ = store.StorePhaseOutput(taskID, PhasePlan, 1, &PlanOutput{
			Summary:         "Test plan",
			FilesToModify:   []string{"file.go"},
			TestingApproach: "unit tests",
			ImplementationSteps: []ImplementationStep{
				{Order: 1, Description: "Step 1"},
			},
		})

		input, err := builder.BuildInputForPhase(taskID, PhaseImplement)
		if err != nil {
			t.Fatalf("BuildInputForPhase failed: %v", err)
		}

		if input == "" {
			t.Error("Input should not be empty")
		}
	})

	t.Run("BuildMarkdownContext", func(t *testing.T) {
		md, err := builder.BuildMarkdownContext(taskID, PhasePlan)
		if err != nil {
			t.Fatalf("BuildMarkdownContext failed: %v", err)
		}

		if md == "" {
			t.Error("Markdown context should not be empty")
		}
	})
}

func TestParser(t *testing.T) {
	parser := NewParser()

	t.Run("ParseOutput extracts single-line JSON", func(t *testing.T) {
		output := `Some agent output
AGENTIUM_HANDOFF: {"summary":"Test plan","files_to_modify":["file.go"],"files_to_create":[],"implementation_steps":[{"order":1,"description":"Step 1"}],"testing_approach":"unit tests"}
More output`

		result, err := parser.ParseOutput(output, PhasePlan)
		if err != nil {
			t.Fatalf("ParseOutput failed: %v", err)
		}

		plan, ok := result.(*PlanOutput)
		if !ok {
			t.Fatalf("Expected *PlanOutput, got %T", result)
		}

		if plan.Summary != "Test plan" {
			t.Errorf("Expected summary 'Test plan', got %s", plan.Summary)
		}
	})

	t.Run("ParseOutput handles multiline JSON", func(t *testing.T) {
		output := `AGENTIUM_HANDOFF: {
  "summary": "Multi-line plan",
  "files_to_modify": ["file.go"],
  "files_to_create": [],
  "implementation_steps": [
    {"order": 1, "description": "Step 1"}
  ],
  "testing_approach": "unit tests"
}
`

		result, err := parser.ParseOutput(output, PhasePlan)
		if err != nil {
			t.Fatalf("ParseOutput failed: %v", err)
		}

		plan := result.(*PlanOutput)
		if plan.Summary != "Multi-line plan" {
			t.Errorf("Expected summary 'Multi-line plan', got %s", plan.Summary)
		}
	})

	t.Run("ParseOutput returns error when no signal", func(t *testing.T) {
		output := "No handoff signal here"
		_, err := parser.ParseOutput(output, PhasePlan)
		if err == nil {
			t.Error("Expected error when no handoff signal")
		}
	})

	t.Run("HasHandoffSignal", func(t *testing.T) {
		if !parser.HasHandoffSignal("AGENTIUM_HANDOFF: {}") {
			t.Error("Should detect handoff signal")
		}
		if parser.HasHandoffSignal("No signal here") {
			t.Error("Should not detect signal when absent")
		}
	})

	t.Run("ParseOutput for IMPLEMENT phase", func(t *testing.T) {
		output := `AGENTIUM_HANDOFF: {"branch_name":"feature/test","commits":[{"hash":"abc123","message":"Add feature"}],"files_changed":["file.go"],"tests_passed":true}`

		result, err := parser.ParseOutput(output, PhaseImplement)
		if err != nil {
			t.Fatalf("ParseOutput failed: %v", err)
		}

		impl := result.(*ImplementOutput)
		if impl.BranchName != "feature/test" {
			t.Errorf("Expected branch 'feature/test', got %s", impl.BranchName)
		}
		if !impl.TestsPassed {
			t.Error("Expected tests_passed to be true")
		}
	})

	t.Run("ParseAny detects phase from content", func(t *testing.T) {
		output := `AGENTIUM_HANDOFF: {"pr_number":123,"pr_url":"https://github.com/owner/repo/pull/123"}`

		phase, result, err := parser.ParseAny(output)
		if err != nil {
			t.Fatalf("ParseAny failed: %v", err)
		}

		if phase != PhasePRCreation {
			t.Errorf("Expected PR_CREATION phase, got %s", phase)
		}

		prOut := result.(*PRCreationOutput)
		if prOut.PRNumber != 123 {
			t.Errorf("Expected PR number 123, got %d", prOut.PRNumber)
		}
	})
}

func TestValidator(t *testing.T) {
	validator := NewValidator()

	t.Run("ValidatePlanOutput success", func(t *testing.T) {
		out := &PlanOutput{
			Summary:         "Valid plan",
			FilesToModify:   []string{"file.go"},
			TestingApproach: "unit tests",
			ImplementationSteps: []ImplementationStep{
				{Order: 1, Description: "Step 1"},
			},
		}

		errs := validator.ValidatePhaseOutput(PhasePlan, out)
		if errs.HasErrors() {
			t.Errorf("Expected no errors, got: %v", errs)
		}
	})

	t.Run("ValidatePlanOutput missing summary", func(t *testing.T) {
		out := &PlanOutput{
			FilesToModify:   []string{"file.go"},
			TestingApproach: "unit tests",
			ImplementationSteps: []ImplementationStep{
				{Order: 1, Description: "Step 1"},
			},
		}

		errs := validator.ValidatePhaseOutput(PhasePlan, out)
		if !errs.HasErrors() {
			t.Error("Expected validation error for missing summary")
		}
	})

	t.Run("ValidatePlanOutput missing steps", func(t *testing.T) {
		out := &PlanOutput{
			Summary:             "Plan",
			TestingApproach:     "unit tests",
			ImplementationSteps: []ImplementationStep{},
		}

		errs := validator.ValidatePhaseOutput(PhasePlan, out)
		if !errs.HasErrors() {
			t.Error("Expected validation error for missing steps")
		}
	})

	t.Run("ValidatePlanOutput invalid complexity", func(t *testing.T) {
		out := &PlanOutput{
			Summary:         "Plan",
			TestingApproach: "unit tests",
			Complexity:      "INVALID",
			ImplementationSteps: []ImplementationStep{
				{Order: 1, Description: "Step 1"},
			},
		}

		errs := validator.ValidatePhaseOutput(PhasePlan, out)
		if !errs.HasErrors() {
			t.Error("Expected validation error for invalid complexity")
		}
	})

	t.Run("ValidateImplementOutput success", func(t *testing.T) {
		out := &ImplementOutput{
			BranchName:   "feature/test",
			FilesChanged: []string{"file.go"},
			Commits: []Commit{
				{Hash: "abc123", Message: "Add feature"},
			},
			TestsPassed: true,
		}

		errs := validator.ValidatePhaseOutput(PhaseImplement, out)
		if errs.HasErrors() {
			t.Errorf("Expected no errors, got: %v", errs)
		}
	})

	t.Run("ValidateImplementOutput missing branch", func(t *testing.T) {
		out := &ImplementOutput{
			FilesChanged: []string{"file.go"},
		}

		errs := validator.ValidatePhaseOutput(PhaseImplement, out)
		if !errs.HasErrors() {
			t.Error("Expected validation error for missing branch")
		}
	})

	t.Run("ValidatePRCreationOutput success", func(t *testing.T) {
		out := &PRCreationOutput{
			PRNumber: 123,
			PRUrl:    "https://github.com/owner/repo/pull/123",
		}

		errs := validator.ValidatePhaseOutput(PhasePRCreation, out)
		if errs.HasErrors() {
			t.Errorf("Expected no errors, got: %v", errs)
		}
	})

	t.Run("ValidatePRCreationOutput invalid PR number", func(t *testing.T) {
		out := &PRCreationOutput{
			PRNumber: 0,
			PRUrl:    "https://github.com/owner/repo/pull/123",
		}

		errs := validator.ValidatePhaseOutput(PhasePRCreation, out)
		if !errs.HasErrors() {
			t.Error("Expected validation error for invalid PR number")
		}
	})

	t.Run("ValidatePhaseInput", func(t *testing.T) {
		tmpDir, _ := os.MkdirTemp("", "validator-test")
		defer os.RemoveAll(tmpDir)

		store, _ := NewStore(tmpDir)
		taskID := "issue:validator"

		// No issue context - should fail for all phases
		errs := validator.ValidatePhaseInput(store, taskID, PhasePlan)
		if !errs.HasErrors() {
			t.Error("Expected error for missing issue context")
		}

		// Add issue context
		store.SetIssueContext(taskID, &IssueContext{Number: 1})

		// PLAN should now pass
		errs = validator.ValidatePhaseInput(store, taskID, PhasePlan)
		if errs.HasErrors() {
			t.Errorf("PLAN should pass with issue context: %v", errs)
		}

		// IMPLEMENT should fail without plan
		errs = validator.ValidatePhaseInput(store, taskID, PhaseImplement)
		if !errs.HasErrors() {
			t.Error("IMPLEMENT should fail without plan output")
		}

		// Add plan output
		_ = store.StorePhaseOutput(taskID, PhasePlan, 1, &PlanOutput{Summary: "Plan"})

		// IMPLEMENT should now pass
		errs = validator.ValidatePhaseInput(store, taskID, PhaseImplement)
		if errs.HasErrors() {
			t.Errorf("IMPLEMENT should pass with plan: %v", errs)
		}
	})
}
