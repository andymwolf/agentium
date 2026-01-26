package handoff

import (
	"strings"
	"testing"
)

func TestBuildPlanInput(t *testing.T) {
	ctx := &IssueContext{
		Number:     "42",
		Title:      "Add authentication",
		Body:       "Implement OAuth2 login",
		Repository: "owner/repo",
	}

	input, err := BuildPlanInput(ctx)
	if err != nil {
		t.Fatalf("BuildPlanInput() error = %v", err)
	}

	if !strings.Contains(input, "Repository:** owner/repo") {
		t.Error("input should contain repository")
	}
	if !strings.Contains(input, "Issue:** #42") {
		t.Error("input should contain issue number")
	}
	if !strings.Contains(input, "Title:** Add authentication") {
		t.Error("input should contain title")
	}
	if !strings.Contains(input, "OAuth2 login") {
		t.Error("input should contain body")
	}
}

func TestBuildPlanInput_NilContext(t *testing.T) {
	_, err := BuildPlanInput(nil)
	if err == nil {
		t.Error("expected error for nil context")
	}
}

func TestBuildImplementInput(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	taskID := "issue:42"

	store.SetIssueContext(taskID, &IssueContext{
		Number: "42",
		Title:  "Add feature",
	})
	store.SetPlanOutput(taskID, &PlanOutput{
		Summary:       "Add the feature",
		FilesToModify: []string{"main.go"},
		FilesToCreate: []string{"feature.go"},
		ImplementationSteps: []ImplementationStep{
			{Number: 1, Description: "Create file", File: "feature.go"},
			{Number: 2, Description: "Update main"},
		},
		TestingApproach: "Unit tests",
	})

	input, err := BuildImplementInput(store, taskID, nil)
	if err != nil {
		t.Fatalf("BuildImplementInput() error = %v", err)
	}

	if !strings.Contains(input, "Issue #42") {
		t.Error("input should contain issue number")
	}
	if !strings.Contains(input, "Add the feature") {
		t.Error("input should contain plan summary")
	}
	if !strings.Contains(input, "main.go") {
		t.Error("input should contain files to modify")
	}
	if !strings.Contains(input, "feature.go") {
		t.Error("input should contain files to create")
	}
	if !strings.Contains(input, "Create file") {
		t.Error("input should contain implementation steps")
	}
}

func TestBuildImplementInput_WithExistingWork(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	taskID := "issue:42"

	store.SetIssueContext(taskID, &IssueContext{Number: "42", Title: "Test"})
	store.SetPlanOutput(taskID, &PlanOutput{Summary: "test"})

	existingWork := &ExistingWorkContext{
		Branch:   "agentium/issue-42-test",
		PRNumber: "123",
		PRTitle:  "Test PR",
	}

	input, err := BuildImplementInput(store, taskID, existingWork)
	if err != nil {
		t.Fatalf("BuildImplementInput() error = %v", err)
	}

	if !strings.Contains(input, "Existing Work") {
		t.Error("input should contain existing work section")
	}
	if !strings.Contains(input, "PR #123") {
		t.Error("input should contain PR number")
	}
	if !strings.Contains(input, "agentium/issue-42-test") {
		t.Error("input should contain branch name")
	}
}

func TestBuildReviewInput(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	taskID := "issue:42"

	store.SetIssueContext(taskID, &IssueContext{Number: "42", Title: "Test"})
	store.SetPlanOutput(taskID, &PlanOutput{Summary: "Test plan summary"})
	store.SetImplementOutput(taskID, &ImplementOutput{
		BranchName: "agentium/issue-42-test",
		FilesChanged: []string{"file1.go", "file2.go"},
		Commits: []CommitInfo{
			{SHA: "abc1234567890", Message: "Add feature"},
		},
		TestsPassed: true,
	})

	input, err := BuildReviewInput(store, taskID)
	if err != nil {
		t.Fatalf("BuildReviewInput() error = %v", err)
	}

	if !strings.Contains(input, "Issue #42") {
		t.Error("input should contain issue number")
	}
	if !strings.Contains(input, "Test plan summary") {
		t.Error("input should contain plan summary")
	}
	if !strings.Contains(input, "agentium/issue-42-test") {
		t.Error("input should contain branch name")
	}
	if !strings.Contains(input, "file1.go") {
		t.Error("input should contain changed files")
	}
	if !strings.Contains(input, "abc1234") {
		t.Error("input should contain commit SHA (truncated)")
	}
	if !strings.Contains(input, "Passed") {
		t.Error("input should indicate tests passed")
	}
}

func TestBuildPRCreationInput(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	taskID := "issue:42"

	store.SetIssueContext(taskID, &IssueContext{
		Number: "42",
		Title:  "Add feature",
	})
	store.SetPlanOutput(taskID, &PlanOutput{Summary: "Plan summary"})
	store.SetImplementOutput(taskID, &ImplementOutput{
		BranchName:   "agentium/issue-42-feature",
		FilesChanged: []string{"main.go", "feature.go"},
		TestsPassed:  true,
	})

	input, err := BuildPRCreationInput(store, taskID)
	if err != nil {
		t.Fatalf("BuildPRCreationInput() error = %v", err)
	}

	if !strings.Contains(input, "#42") {
		t.Error("input should contain issue number")
	}
	if !strings.Contains(input, "agentium/issue-42-feature") {
		t.Error("input should contain branch name")
	}
	if !strings.Contains(input, "Closes #42") {
		t.Error("input should mention closing issue")
	}
}

func TestBuildInputForPhase(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	taskID := "issue:42"

	store.SetIssueContext(taskID, &IssueContext{Number: "42", Title: "Test"})
	store.SetPlanOutput(taskID, &PlanOutput{Summary: "plan"})
	store.SetImplementOutput(taskID, &ImplementOutput{BranchName: "branch"})

	tests := []struct {
		phase     string
		shouldErr bool
	}{
		{"PLAN", false},
		{"IMPLEMENT", false},
		{"REVIEW", false},
		{"DOCS", false},
		{"PR_CREATION", false},
		{"UNKNOWN", true},
	}

	for _, tt := range tests {
		t.Run(tt.phase, func(t *testing.T) {
			_, err := BuildInputForPhase(store, taskID, tt.phase, nil)
			if (err != nil) != tt.shouldErr {
				t.Errorf("BuildInputForPhase(%s) error = %v, shouldErr = %v", tt.phase, err, tt.shouldErr)
			}
		})
	}
}

func TestBuildInputForPhaseJSON(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	taskID := "issue:42"

	store.SetIssueContext(taskID, &IssueContext{Number: "42", Title: "Test"})

	json, err := BuildInputForPhaseJSON(store, taskID, "PLAN", nil)
	if err != nil {
		t.Fatalf("BuildInputForPhaseJSON() error = %v", err)
	}

	if !strings.Contains(json, `"number": "42"`) {
		t.Error("JSON should contain issue number")
	}
	if !strings.Contains(json, `"title": "Test"`) {
		t.Error("JSON should contain title")
	}
}
