package handoff

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStore_SetGetIssueContext(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	taskID := "issue:42"
	ctx := &IssueContext{
		Number:     "42",
		Title:      "Test Issue",
		Body:       "Test body",
		Repository: "owner/repo",
	}

	store.SetIssueContext(taskID, ctx)
	got := store.GetIssueContext(taskID)

	if got == nil {
		t.Fatal("expected issue context, got nil")
	}
	if got.Number != "42" {
		t.Errorf("expected number 42, got %s", got.Number)
	}
	if got.Title != "Test Issue" {
		t.Errorf("expected title 'Test Issue', got %s", got.Title)
	}
}

func TestStore_SetGetPlanOutput(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	taskID := "issue:42"
	plan := &PlanOutput{
		Summary:       "Test plan",
		FilesToModify: []string{"file1.go", "file2.go"},
		FilesToCreate: []string{"file3.go"},
		ImplementationSteps: []ImplementationStep{
			{Number: 1, Description: "Step 1", File: "file1.go"},
			{Number: 2, Description: "Step 2"},
		},
		TestingApproach: "Unit tests",
	}

	store.SetPlanOutput(taskID, plan)
	got := store.GetPlanOutput(taskID)

	if got == nil {
		t.Fatal("expected plan output, got nil")
	}
	if got.Summary != "Test plan" {
		t.Errorf("expected summary 'Test plan', got %s", got.Summary)
	}
	if len(got.FilesToModify) != 2 {
		t.Errorf("expected 2 files to modify, got %d", len(got.FilesToModify))
	}
	if len(got.ImplementationSteps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(got.ImplementationSteps))
	}
}

func TestStore_ClearFromPhase(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	taskID := "issue:42"

	// Set up all phases
	store.SetIssueContext(taskID, &IssueContext{Number: "42"})
	store.SetPlanOutput(taskID, &PlanOutput{Summary: "plan"})
	store.SetImplementOutput(taskID, &ImplementOutput{BranchName: "branch"})
	store.SetReviewOutput(taskID, &ReviewOutput{IssuesFound: []ReviewIssue{}})
	store.SetDocsOutput(taskID, &DocsOutput{DocsUpdated: []string{}})
	store.SetPRCreationOutput(taskID, &PRCreationOutput{PRNumber: 123})

	// Clear from IMPLEMENT onwards
	store.ClearFromPhase(taskID, "IMPLEMENT")

	// Issue context and plan should still exist
	if store.GetIssueContext(taskID) == nil {
		t.Error("expected issue context to remain")
	}
	if store.GetPlanOutput(taskID) == nil {
		t.Error("expected plan output to remain")
	}

	// Everything from IMPLEMENT onwards should be cleared
	if store.GetImplementOutput(taskID) != nil {
		t.Error("expected implement output to be cleared")
	}
	if store.GetReviewOutput(taskID) != nil {
		t.Error("expected review output to be cleared")
	}
	if store.GetDocsOutput(taskID) != nil {
		t.Error("expected docs output to be cleared")
	}
	if store.GetPRCreationOutput(taskID) != nil {
		t.Error("expected PR creation output to be cleared")
	}
}

func TestStore_SaveLoad(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	taskID := "issue:42"
	store.SetIssueContext(taskID, &IssueContext{
		Number: "42",
		Title:  "Test Issue",
	})
	store.SetPlanOutput(taskID, &PlanOutput{
		Summary: "Test plan",
	})

	// Save
	if err := store.Save(); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	// Verify file exists
	filePath := filepath.Join(dir, ".agentium", "handoff.json")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Fatal("handoff.json file not created")
	}

	// Load into new store
	store2 := NewStore(dir)
	if err := store2.Load(); err != nil {
		t.Fatalf("failed to load: %v", err)
	}

	// Verify data
	ctx := store2.GetIssueContext(taskID)
	if ctx == nil {
		t.Fatal("expected issue context after load")
	}
	if ctx.Number != "42" {
		t.Errorf("expected number 42, got %s", ctx.Number)
	}

	plan := store2.GetPlanOutput(taskID)
	if plan == nil {
		t.Fatal("expected plan output after load")
	}
	if plan.Summary != "Test plan" {
		t.Errorf("expected summary 'Test plan', got %s", plan.Summary)
	}
}

func TestStore_HasPlanOutput(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	taskID := "issue:42"

	if store.HasPlanOutput(taskID) {
		t.Error("expected HasPlanOutput to return false initially")
	}

	store.SetPlanOutput(taskID, &PlanOutput{Summary: "test"})

	if !store.HasPlanOutput(taskID) {
		t.Error("expected HasPlanOutput to return true after setting")
	}
}

func TestStore_Summary(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	taskID := "issue:42"

	// Empty task
	summary := store.Summary(taskID)
	if summary != "Task issue:42: no handoff data" {
		t.Errorf("unexpected summary: %s", summary)
	}

	// With some data
	store.SetIssueContext(taskID, &IssueContext{Number: "42"})
	store.SetPlanOutput(taskID, &PlanOutput{Summary: "test"})

	summary = store.Summary(taskID)
	if summary != "Task issue:42: [IssueContext Plan]" {
		t.Errorf("unexpected summary: %s", summary)
	}
}
