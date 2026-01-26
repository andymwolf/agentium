package handoff

import (
	"testing"
)

func TestValidatePlanOutput(t *testing.T) {
	tests := []struct {
		name       string
		output     *PlanOutput
		wantValid  bool
		wantErrors int
	}{
		{
			name:       "nil output",
			output:     nil,
			wantValid:  false,
			wantErrors: 1,
		},
		{
			name:       "empty summary",
			output:     &PlanOutput{Summary: ""},
			wantValid:  false,
			wantErrors: 1,
		},
		{
			name: "valid output",
			output: &PlanOutput{
				Summary:       "Test plan",
				FilesToModify: []string{"file.go"},
				ImplementationSteps: []ImplementationStep{
					{Number: 1, Description: "Step 1"},
				},
			},
			wantValid:  true,
			wantErrors: 0,
		},
		{
			name: "no files - warning only",
			output: &PlanOutput{
				Summary: "Test plan",
			},
			wantValid:  true, // Warning, not error
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidatePlanOutput(tt.output)
			if result.Valid != tt.wantValid {
				t.Errorf("Valid = %v, want %v", result.Valid, tt.wantValid)
			}
			if len(result.Errors) != tt.wantErrors {
				t.Errorf("Errors = %d, want %d", len(result.Errors), tt.wantErrors)
			}
		})
	}
}

func TestValidateImplementOutput(t *testing.T) {
	tests := []struct {
		name       string
		output     *ImplementOutput
		wantValid  bool
		wantErrors int
	}{
		{
			name:       "nil output",
			output:     nil,
			wantValid:  false,
			wantErrors: 1,
		},
		{
			name:       "empty branch name",
			output:     &ImplementOutput{BranchName: ""},
			wantValid:  false,
			wantErrors: 1,
		},
		{
			name: "valid output",
			output: &ImplementOutput{
				BranchName:   "agentium/issue-42",
				Commits:      []CommitInfo{{SHA: "abc", Message: "test"}},
				FilesChanged: []string{"file.go"},
				TestsPassed:  true,
			},
			wantValid:  true,
			wantErrors: 0,
		},
		{
			name: "tests failed - warning only",
			output: &ImplementOutput{
				BranchName:  "agentium/issue-42",
				TestsPassed: false,
			},
			wantValid:  true, // Warning, not error
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateImplementOutput(tt.output)
			if result.Valid != tt.wantValid {
				t.Errorf("Valid = %v, want %v", result.Valid, tt.wantValid)
			}
			if len(result.Errors) != tt.wantErrors {
				t.Errorf("Errors = %d, want %d", len(result.Errors), tt.wantErrors)
			}
		})
	}
}

func TestValidatePRCreationOutput(t *testing.T) {
	tests := []struct {
		name       string
		output     *PRCreationOutput
		wantValid  bool
		wantErrors int
	}{
		{
			name:       "nil output",
			output:     nil,
			wantValid:  false,
			wantErrors: 1,
		},
		{
			name:       "zero PR number",
			output:     &PRCreationOutput{PRNumber: 0},
			wantValid:  false,
			wantErrors: 1,
		},
		{
			name: "valid output",
			output: &PRCreationOutput{
				PRNumber: 123,
				PRURL:    "https://github.com/owner/repo/pull/123",
			},
			wantValid:  true,
			wantErrors: 0,
		},
		{
			name: "missing URL - warning only",
			output: &PRCreationOutput{
				PRNumber: 123,
			},
			wantValid:  true, // Warning, not error
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidatePRCreationOutput(tt.output)
			if result.Valid != tt.wantValid {
				t.Errorf("Valid = %v, want %v", result.Valid, tt.wantValid)
			}
			if len(result.Errors) != tt.wantErrors {
				t.Errorf("Errors = %d, want %d", len(result.Errors), tt.wantErrors)
			}
		})
	}
}

func TestCanAdvanceToPhase(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	taskID := "issue:42"

	// PLAN only needs issue context
	store.SetIssueContext(taskID, &IssueContext{Number: "42"})

	canAdvance, reason := CanAdvanceToPhase(store, taskID, "PLAN")
	if !canAdvance {
		t.Errorf("should be able to advance to PLAN: %s", reason)
	}

	// IMPLEMENT needs plan
	canAdvance, reason = CanAdvanceToPhase(store, taskID, "IMPLEMENT")
	if canAdvance {
		t.Error("should not be able to advance to IMPLEMENT without plan")
	}

	store.SetPlanOutput(taskID, &PlanOutput{Summary: "test"})

	canAdvance, reason = CanAdvanceToPhase(store, taskID, "IMPLEMENT")
	if !canAdvance {
		t.Errorf("should be able to advance to IMPLEMENT: %s", reason)
	}

	// REVIEW needs implementation
	canAdvance, reason = CanAdvanceToPhase(store, taskID, "REVIEW")
	if canAdvance {
		t.Error("should not be able to advance to REVIEW without implementation")
	}

	store.SetImplementOutput(taskID, &ImplementOutput{BranchName: "test"})

	canAdvance, reason = CanAdvanceToPhase(store, taskID, "REVIEW")
	if !canAdvance {
		t.Errorf("should be able to advance to REVIEW: %s", reason)
	}
}

func TestValidateReviewOutput(t *testing.T) {
	tests := []struct {
		name       string
		output     *ReviewOutput
		wantValid  bool
		wantErrors int
	}{
		{
			name:       "nil output",
			output:     nil,
			wantValid:  false,
			wantErrors: 1,
		},
		{
			name: "valid empty output",
			output: &ReviewOutput{
				IssuesFound: []ReviewIssue{},
			},
			wantValid:  true,
			wantErrors: 0,
		},
		{
			name: "regression without reason - warning",
			output: &ReviewOutput{
				RegressionNeeded: true,
				RegressionReason: "",
			},
			wantValid:  true, // Warning, not error
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateReviewOutput(tt.output)
			if result.Valid != tt.wantValid {
				t.Errorf("Valid = %v, want %v", result.Valid, tt.wantValid)
			}
			if len(result.Errors) != tt.wantErrors {
				t.Errorf("Errors = %d, want %d", len(result.Errors), tt.wantErrors)
			}
		})
	}
}
