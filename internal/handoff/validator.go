package handoff

import (
	"fmt"
	"strings"
)

// ValidationError represents a validation failure with details.
type ValidationError struct {
	Phase   string
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s validation error: %s - %s", e.Phase, e.Field, e.Message)
}

// ValidationResult contains the result of validating a phase output.
type ValidationResult struct {
	Valid    bool
	Errors   []ValidationError
	Warnings []string
}

// ValidatePlanOutput validates a PlanOutput has required fields.
func ValidatePlanOutput(output *PlanOutput) ValidationResult {
	result := ValidationResult{Valid: true}

	if output == nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Phase:   "PLAN",
			Field:   "output",
			Message: "output is nil",
		})
		return result
	}

	if strings.TrimSpace(output.Summary) == "" {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Phase:   "PLAN",
			Field:   "summary",
			Message: "summary is required",
		})
	}

	// At least one file should be modified or created
	if len(output.FilesToModify) == 0 && len(output.FilesToCreate) == 0 {
		result.Warnings = append(result.Warnings,
			"no files to modify or create specified (might indicate no code changes needed)")
	}

	// Implementation steps are strongly recommended
	if len(output.ImplementationSteps) == 0 {
		result.Warnings = append(result.Warnings,
			"no implementation steps specified")
	}

	return result
}

// ValidateImplementOutput validates an ImplementOutput has required fields.
func ValidateImplementOutput(output *ImplementOutput) ValidationResult {
	result := ValidationResult{Valid: true}

	if output == nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Phase:   "IMPLEMENT",
			Field:   "output",
			Message: "output is nil",
		})
		return result
	}

	if strings.TrimSpace(output.BranchName) == "" {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Phase:   "IMPLEMENT",
			Field:   "branch_name",
			Message: "branch name is required",
		})
	}

	// Commits should exist unless this is a continuation
	if len(output.Commits) == 0 {
		result.Warnings = append(result.Warnings,
			"no commits recorded (might indicate continuation of existing work)")
	}

	// Files changed should typically exist
	if len(output.FilesChanged) == 0 {
		result.Warnings = append(result.Warnings,
			"no files changed recorded")
	}

	// Tests passing is important but not blocking
	if !output.TestsPassed {
		result.Warnings = append(result.Warnings,
			"tests did not pass - review phase should address this")
	}

	return result
}

// ValidateReviewOutput validates a ReviewOutput has required fields.
func ValidateReviewOutput(output *ReviewOutput) ValidationResult {
	result := ValidationResult{Valid: true}

	if output == nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Phase:   "REVIEW",
			Field:   "output",
			Message: "output is nil",
		})
		return result
	}

	// If regression is needed, reason should be provided
	if output.RegressionNeeded && strings.TrimSpace(output.RegressionReason) == "" {
		result.Warnings = append(result.Warnings,
			"regression requested but no reason provided")
	}

	// Check that issues marked as fixed have corresponding fixes applied
	fixedCount := 0
	for _, issue := range output.IssuesFound {
		if issue.Fixed {
			fixedCount++
		}
	}
	if fixedCount > 0 && len(output.FixesApplied) == 0 {
		result.Warnings = append(result.Warnings,
			"issues marked as fixed but no fixes recorded in fixes_applied")
	}

	return result
}

// ValidateDocsOutput validates a DocsOutput.
func ValidateDocsOutput(output *DocsOutput) ValidationResult {
	result := ValidationResult{Valid: true}

	if output == nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Phase:   "DOCS",
			Field:   "output",
			Message: "output is nil",
		})
		return result
	}

	// Docs output can be empty (no docs needed), which is valid
	return result
}

// ValidatePRCreationOutput validates a PRCreationOutput has required fields.
func ValidatePRCreationOutput(output *PRCreationOutput) ValidationResult {
	result := ValidationResult{Valid: true}

	if output == nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Phase:   "PR_CREATION",
			Field:   "output",
			Message: "output is nil",
		})
		return result
	}

	if output.PRNumber <= 0 {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Phase:   "PR_CREATION",
			Field:   "pr_number",
			Message: "valid PR number is required",
		})
	}

	if strings.TrimSpace(output.PRURL) == "" {
		result.Warnings = append(result.Warnings,
			"PR URL not provided")
	}

	return result
}

// ValidatePhaseOutput validates the output for a given phase.
func ValidatePhaseOutput(phase string, output interface{}) ValidationResult {
	switch phase {
	case "PLAN":
		if o, ok := output.(*PlanOutput); ok {
			return ValidatePlanOutput(o)
		}
		return ValidationResult{
			Valid:  false,
			Errors: []ValidationError{{Phase: "PLAN", Field: "type", Message: "expected *PlanOutput"}},
		}

	case "IMPLEMENT":
		if o, ok := output.(*ImplementOutput); ok {
			return ValidateImplementOutput(o)
		}
		return ValidationResult{
			Valid:  false,
			Errors: []ValidationError{{Phase: "IMPLEMENT", Field: "type", Message: "expected *ImplementOutput"}},
		}

	case "REVIEW":
		if o, ok := output.(*ReviewOutput); ok {
			return ValidateReviewOutput(o)
		}
		return ValidationResult{
			Valid:  false,
			Errors: []ValidationError{{Phase: "REVIEW", Field: "type", Message: "expected *ReviewOutput"}},
		}

	case "DOCS":
		if o, ok := output.(*DocsOutput); ok {
			return ValidateDocsOutput(o)
		}
		return ValidationResult{
			Valid:  false,
			Errors: []ValidationError{{Phase: "DOCS", Field: "type", Message: "expected *DocsOutput"}},
		}

	case "PR_CREATION":
		if o, ok := output.(*PRCreationOutput); ok {
			return ValidatePRCreationOutput(o)
		}
		return ValidationResult{
			Valid:  false,
			Errors: []ValidationError{{Phase: "PR_CREATION", Field: "type", Message: "expected *PRCreationOutput"}},
		}

	default:
		return ValidationResult{
			Valid:  false,
			Errors: []ValidationError{{Phase: phase, Field: "phase", Message: "unknown phase"}},
		}
	}
}

// CanAdvanceToPhase checks if the handoff store has sufficient data to start the given phase.
func CanAdvanceToPhase(store *Store, taskID string, phase string) (bool, string) {
	switch phase {
	case "PLAN":
		// PLAN only needs issue context
		if store.GetIssueContext(taskID) == nil {
			return false, "issue context required for PLAN phase"
		}
		return true, ""

	case "IMPLEMENT":
		// IMPLEMENT needs issue context and plan
		if store.GetIssueContext(taskID) == nil {
			return false, "issue context required for IMPLEMENT phase"
		}
		if store.GetPlanOutput(taskID) == nil {
			return false, "plan output required for IMPLEMENT phase"
		}
		return true, ""

	case "REVIEW":
		// REVIEW needs issue context, plan, and implementation
		if store.GetIssueContext(taskID) == nil {
			return false, "issue context required for REVIEW phase"
		}
		if store.GetPlanOutput(taskID) == nil {
			return false, "plan output required for REVIEW phase"
		}
		if store.GetImplementOutput(taskID) == nil {
			return false, "implement output required for REVIEW phase"
		}
		return true, ""

	case "DOCS":
		// DOCS needs issue context, plan, and implementation
		if store.GetIssueContext(taskID) == nil {
			return false, "issue context required for DOCS phase"
		}
		if store.GetPlanOutput(taskID) == nil {
			return false, "plan output required for DOCS phase"
		}
		if store.GetImplementOutput(taskID) == nil {
			return false, "implement output required for DOCS phase"
		}
		return true, ""

	case "PR_CREATION":
		// PR_CREATION needs issue context, plan, and implementation
		if store.GetIssueContext(taskID) == nil {
			return false, "issue context required for PR_CREATION phase"
		}
		if store.GetPlanOutput(taskID) == nil {
			return false, "plan output required for PR_CREATION phase"
		}
		if store.GetImplementOutput(taskID) == nil {
			return false, "implement output required for PR_CREATION phase"
		}
		return true, ""

	default:
		return false, fmt.Sprintf("unknown phase: %s", phase)
	}
}
