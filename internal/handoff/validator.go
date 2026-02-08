package handoff

import (
	"fmt"
	"strings"
)

// ValidationError contains details about validation failures.
type ValidationError struct {
	Phase   Phase
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s validation error for %s: %s", e.Phase, e.Field, e.Message)
}

// ValidationErrors is a collection of validation errors.
type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return ""
	}
	var msgs []string
	for _, err := range e {
		msgs = append(msgs, err.Error())
	}
	return strings.Join(msgs, "; ")
}

// HasErrors returns true if there are validation errors.
func (e ValidationErrors) HasErrors() bool {
	return len(e) > 0
}

// Validator validates handoff data for completeness and correctness.
type Validator struct{}

// NewValidator creates a new handoff validator.
func NewValidator() *Validator {
	return &Validator{}
}

// ValidatePhaseOutput validates the output for a given phase.
func (v *Validator) ValidatePhaseOutput(phase Phase, output interface{}) ValidationErrors {
	var errs ValidationErrors

	switch phase {
	case PhasePlan:
		out, ok := output.(*PlanOutput)
		if !ok {
			errs = append(errs, ValidationError{Phase: phase, Field: "type", Message: "expected *PlanOutput"})
			return errs
		}
		errs = append(errs, v.validatePlanOutput(out)...)

	case PhaseImplement:
		out, ok := output.(*ImplementOutput)
		if !ok {
			errs = append(errs, ValidationError{Phase: phase, Field: "type", Message: "expected *ImplementOutput"})
			return errs
		}
		errs = append(errs, v.validateImplementOutput(out)...)

	case PhaseDocs:
		out, ok := output.(*DocsOutput)
		if !ok {
			errs = append(errs, ValidationError{Phase: phase, Field: "type", Message: "expected *DocsOutput"})
			return errs
		}
		errs = append(errs, v.validateDocsOutput(out)...)

	case PhaseVerify:
		out, ok := output.(*VerifyOutput)
		if !ok {
			errs = append(errs, ValidationError{Phase: phase, Field: "type", Message: "expected *VerifyOutput"})
			return errs
		}
		errs = append(errs, v.validateVerifyOutput(out)...)

	default:
		errs = append(errs, ValidationError{Phase: phase, Field: "phase", Message: "unknown phase"})
	}

	return errs
}

// validatePlanOutput validates PLAN phase output.
func (v *Validator) validatePlanOutput(out *PlanOutput) ValidationErrors {
	var errs ValidationErrors

	if out == nil {
		errs = append(errs, ValidationError{Phase: PhasePlan, Field: "output", Message: "output is nil"})
		return errs
	}

	if strings.TrimSpace(out.Summary) == "" {
		errs = append(errs, ValidationError{Phase: PhasePlan, Field: "summary", Message: "summary is required"})
	}

	if len(out.ImplementationSteps) == 0 {
		errs = append(errs, ValidationError{Phase: PhasePlan, Field: "implementation_steps", Message: "at least one implementation step is required"})
	}

	// Validate step ordering
	for i, step := range out.ImplementationSteps {
		if step.Order <= 0 {
			errs = append(errs, ValidationError{
				Phase:   PhasePlan,
				Field:   fmt.Sprintf("implementation_steps[%d].order", i),
				Message: "step order must be positive",
			})
		}
		if strings.TrimSpace(step.Description) == "" {
			errs = append(errs, ValidationError{
				Phase:   PhasePlan,
				Field:   fmt.Sprintf("implementation_steps[%d].description", i),
				Message: "step description is required",
			})
		}
	}

	if strings.TrimSpace(out.TestingApproach) == "" {
		errs = append(errs, ValidationError{Phase: PhasePlan, Field: "testing_approach", Message: "testing approach is required"})
	}

	return errs
}

// validateImplementOutput validates IMPLEMENT phase output.
func (v *Validator) validateImplementOutput(out *ImplementOutput) ValidationErrors {
	var errs ValidationErrors

	if out == nil {
		errs = append(errs, ValidationError{Phase: PhaseImplement, Field: "output", Message: "output is nil"})
		return errs
	}

	if strings.TrimSpace(out.BranchName) == "" {
		errs = append(errs, ValidationError{Phase: PhaseImplement, Field: "branch_name", Message: "branch name is required"})
	}

	if len(out.FilesChanged) == 0 {
		errs = append(errs, ValidationError{Phase: PhaseImplement, Field: "files_changed", Message: "at least one file must be changed"})
	}

	// Validate commits
	for i, commit := range out.Commits {
		if strings.TrimSpace(commit.Hash) == "" {
			errs = append(errs, ValidationError{
				Phase:   PhaseImplement,
				Field:   fmt.Sprintf("commits[%d].hash", i),
				Message: "commit hash is required",
			})
		}
		if strings.TrimSpace(commit.Message) == "" {
			errs = append(errs, ValidationError{
				Phase:   PhaseImplement,
				Field:   fmt.Sprintf("commits[%d].message", i),
				Message: "commit message is required",
			})
		}
	}

	return errs
}

// validateDocsOutput validates DOCS phase output.
func (v *Validator) validateDocsOutput(out *DocsOutput) ValidationErrors {
	var errs ValidationErrors

	if out == nil {
		errs = append(errs, ValidationError{Phase: PhaseDocs, Field: "output", Message: "output is nil"})
		return errs
	}

	// DocsOutput can legitimately have no updates if no docs needed
	// Just validate the structure is present

	return errs
}

// validateVerifyOutput validates VERIFY phase output.
func (v *Validator) validateVerifyOutput(out *VerifyOutput) ValidationErrors {
	var errs ValidationErrors

	if out == nil {
		errs = append(errs, ValidationError{Phase: PhaseVerify, Field: "output", Message: "output is nil"})
		return errs
	}

	// VerifyOutput is valid as long as it's present - checks_passed and merge_successful
	// are booleans that convey the result regardless of value

	return errs
}

// ValidatePhaseInput validates that required inputs are present for a phase.
func (v *Validator) ValidatePhaseInput(store *Store, taskID string, phase Phase) ValidationErrors {
	var errs ValidationErrors

	// All phases require issue context
	if store.GetIssueContext(taskID) == nil {
		errs = append(errs, ValidationError{Phase: phase, Field: "issue_context", Message: "issue context is required"})
	}

	// Check phase-specific requirements
	switch phase {
	case PhasePlan:
		// Only needs issue context

	case PhaseImplement:
		if store.GetPlanOutput(taskID) == nil {
			errs = append(errs, ValidationError{Phase: phase, Field: "plan_output", Message: "plan output is required for IMPLEMENT phase"})
		}

	case PhaseDocs:
		if store.GetPlanOutput(taskID) == nil {
			errs = append(errs, ValidationError{Phase: phase, Field: "plan_output", Message: "plan output is required for DOCS phase"})
		}
		if store.GetImplementOutput(taskID) == nil {
			errs = append(errs, ValidationError{Phase: phase, Field: "implement_output", Message: "implement output is required for DOCS phase"})
		}

	case PhaseVerify:
		if store.GetImplementOutput(taskID) == nil {
			errs = append(errs, ValidationError{Phase: phase, Field: "implement_output", Message: "implement output is required for VERIFY phase"})
		}
	}

	return errs
}
