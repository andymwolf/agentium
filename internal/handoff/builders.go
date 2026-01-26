package handoff

import (
	"encoding/json"
	"fmt"
	"strings"
)

// BuildPlanInput constructs the input for the PLAN phase.
// This is the minimal context needed for planning: just the issue details.
func BuildPlanInput(issueCtx *IssueContext) (string, error) {
	if issueCtx == nil {
		return "", fmt.Errorf("issue context is required for PLAN phase")
	}

	var sb strings.Builder
	sb.WriteString("## Issue Context\n\n")
	sb.WriteString(fmt.Sprintf("**Repository:** %s\n", issueCtx.Repository))
	sb.WriteString(fmt.Sprintf("**Issue:** #%s\n", issueCtx.Number))
	sb.WriteString(fmt.Sprintf("**Title:** %s\n\n", issueCtx.Title))

	if issueCtx.Body != "" {
		sb.WriteString("**Description:**\n")
		sb.WriteString(issueCtx.Body)
		sb.WriteString("\n\n")
	}

	return sb.String(), nil
}

// BuildImplementInput constructs the input for the IMPLEMENT phase.
// This includes the issue context, the plan, and any existing work.
func BuildImplementInput(store *Store, taskID string, existingWork *ExistingWorkContext) (string, error) {
	issueCtx := store.GetIssueContext(taskID)
	if issueCtx == nil {
		return "", fmt.Errorf("issue context not found for task %s", taskID)
	}

	plan := store.GetPlanOutput(taskID)
	if plan == nil {
		return "", fmt.Errorf("plan output not found for task %s", taskID)
	}

	var sb strings.Builder

	// Issue summary (minimal)
	sb.WriteString("## Task\n\n")
	sb.WriteString(fmt.Sprintf("**Issue #%s:** %s\n\n", issueCtx.Number, issueCtx.Title))

	// Plan from previous phase
	sb.WriteString("## Implementation Plan\n\n")
	sb.WriteString(fmt.Sprintf("**Summary:** %s\n\n", plan.Summary))

	if len(plan.FilesToModify) > 0 {
		sb.WriteString("**Files to modify:**\n")
		for _, f := range plan.FilesToModify {
			sb.WriteString(fmt.Sprintf("- %s\n", f))
		}
		sb.WriteString("\n")
	}

	if len(plan.FilesToCreate) > 0 {
		sb.WriteString("**Files to create:**\n")
		for _, f := range plan.FilesToCreate {
			sb.WriteString(fmt.Sprintf("- %s\n", f))
		}
		sb.WriteString("\n")
	}

	if len(plan.ImplementationSteps) > 0 {
		sb.WriteString("**Implementation steps:**\n")
		for _, step := range plan.ImplementationSteps {
			if step.File != "" {
				sb.WriteString(fmt.Sprintf("%d. %s (in %s)\n", step.Number, step.Description, step.File))
			} else {
				sb.WriteString(fmt.Sprintf("%d. %s\n", step.Number, step.Description))
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("**Testing approach:** %s\n\n", plan.TestingApproach))

	// Existing work context
	if existingWork != nil {
		sb.WriteString("## Existing Work\n\n")
		if existingWork.PRNumber != "" {
			sb.WriteString(fmt.Sprintf("An existing PR #%s exists on branch `%s`.\n", existingWork.PRNumber, existingWork.Branch))
			sb.WriteString("Check out this branch and continue from where it left off.\n\n")
		} else if existingWork.Branch != "" {
			sb.WriteString(fmt.Sprintf("An existing branch `%s` exists.\n", existingWork.Branch))
			sb.WriteString("Check out this branch and continue from where it left off.\n\n")
		}
	}

	return sb.String(), nil
}

// BuildReviewInput constructs the input for the REVIEW phase.
// This includes the plan summary and implementation output.
func BuildReviewInput(store *Store, taskID string) (string, error) {
	issueCtx := store.GetIssueContext(taskID)
	if issueCtx == nil {
		return "", fmt.Errorf("issue context not found for task %s", taskID)
	}

	plan := store.GetPlanOutput(taskID)
	if plan == nil {
		return "", fmt.Errorf("plan output not found for task %s", taskID)
	}

	impl := store.GetImplementOutput(taskID)
	if impl == nil {
		return "", fmt.Errorf("implement output not found for task %s", taskID)
	}

	var sb strings.Builder

	// Issue summary (minimal)
	sb.WriteString("## Task\n\n")
	sb.WriteString(fmt.Sprintf("**Issue #%s:** %s\n\n", issueCtx.Number, issueCtx.Title))

	// Plan summary (not full plan - just what reviewer needs)
	sb.WriteString("## Plan Summary\n\n")
	sb.WriteString(plan.Summary)
	sb.WriteString("\n\n")

	// Implementation output
	sb.WriteString("## Implementation Completed\n\n")
	sb.WriteString(fmt.Sprintf("**Branch:** %s\n", impl.BranchName))

	if len(impl.FilesChanged) > 0 {
		sb.WriteString(fmt.Sprintf("**Files changed:** %d\n", len(impl.FilesChanged)))
		for _, f := range impl.FilesChanged {
			sb.WriteString(fmt.Sprintf("- %s\n", f))
		}
		sb.WriteString("\n")
	}

	if len(impl.Commits) > 0 {
		sb.WriteString(fmt.Sprintf("**Commits:** %d\n", len(impl.Commits)))
		for _, c := range impl.Commits {
			sb.WriteString(fmt.Sprintf("- %s: %s\n", c.SHA[:7], c.Message))
		}
		sb.WriteString("\n")
	}

	if impl.TestsPassed {
		sb.WriteString("**Tests:** Passed\n\n")
	} else {
		sb.WriteString("**Tests:** Failed or not run\n")
		if impl.TestOutput != "" {
			sb.WriteString("```\n")
			// Truncate test output to avoid bloat
			output := impl.TestOutput
			if len(output) > 1000 {
				output = output[:1000] + "\n... (truncated)"
			}
			sb.WriteString(output)
			sb.WriteString("\n```\n\n")
		}
	}

	return sb.String(), nil
}

// BuildDocsInput constructs the input for the DOCS phase.
// This includes what files were changed for doc update context.
func BuildDocsInput(store *Store, taskID string) (string, error) {
	issueCtx := store.GetIssueContext(taskID)
	if issueCtx == nil {
		return "", fmt.Errorf("issue context not found for task %s", taskID)
	}

	plan := store.GetPlanOutput(taskID)
	if plan == nil {
		return "", fmt.Errorf("plan output not found for task %s", taskID)
	}

	impl := store.GetImplementOutput(taskID)
	if impl == nil {
		return "", fmt.Errorf("implement output not found for task %s", taskID)
	}

	var sb strings.Builder

	sb.WriteString("## Task\n\n")
	sb.WriteString(fmt.Sprintf("**Issue #%s:** %s\n\n", issueCtx.Number, issueCtx.Title))

	sb.WriteString("## Summary\n\n")
	sb.WriteString(plan.Summary)
	sb.WriteString("\n\n")

	sb.WriteString("## Files Changed\n\n")
	for _, f := range impl.FilesChanged {
		sb.WriteString(fmt.Sprintf("- %s\n", f))
	}
	sb.WriteString("\n")

	sb.WriteString("## Instructions\n\n")
	sb.WriteString("Review if any documentation needs to be updated based on the changes above.\n")
	sb.WriteString("If no documentation changes are needed, emit an empty docs output.\n")

	return sb.String(), nil
}

// BuildPRCreationInput constructs the input for the PR_CREATION phase.
func BuildPRCreationInput(store *Store, taskID string) (string, error) {
	issueCtx := store.GetIssueContext(taskID)
	if issueCtx == nil {
		return "", fmt.Errorf("issue context not found for task %s", taskID)
	}

	plan := store.GetPlanOutput(taskID)
	if plan == nil {
		return "", fmt.Errorf("plan output not found for task %s", taskID)
	}

	impl := store.GetImplementOutput(taskID)
	if impl == nil {
		return "", fmt.Errorf("implement output not found for task %s", taskID)
	}

	var sb strings.Builder

	sb.WriteString("## Create Pull Request\n\n")
	sb.WriteString(fmt.Sprintf("**Issue:** #%s - %s\n", issueCtx.Number, issueCtx.Title))
	sb.WriteString(fmt.Sprintf("**Branch:** %s\n", impl.BranchName))
	sb.WriteString(fmt.Sprintf("**Files changed:** %d\n", len(impl.FilesChanged)))

	if impl.TestsPassed {
		sb.WriteString("**Tests:** Passed\n\n")
	} else {
		sb.WriteString("**Tests:** Not passed (PR should note this)\n\n")
	}

	sb.WriteString("## Summary for PR Description\n\n")
	sb.WriteString(plan.Summary)
	sb.WriteString("\n\n")

	sb.WriteString("## Instructions\n\n")
	sb.WriteString("1. Push the branch if not already pushed\n")
	sb.WriteString(fmt.Sprintf("2. Create a PR with title referencing issue #%s\n", issueCtx.Number))
	sb.WriteString(fmt.Sprintf("3. Include 'Closes #%s' in the PR body\n", issueCtx.Number))
	sb.WriteString("4. Emit the PR number and URL in the handoff output\n")

	return sb.String(), nil
}

// BuildInputForPhase constructs the input string for the given phase.
// Returns the input content and any error.
func BuildInputForPhase(store *Store, taskID string, phase string, existingWork *ExistingWorkContext) (string, error) {
	switch phase {
	case "PLAN":
		issueCtx := store.GetIssueContext(taskID)
		return BuildPlanInput(issueCtx)
	case "IMPLEMENT":
		return BuildImplementInput(store, taskID, existingWork)
	case "REVIEW":
		return BuildReviewInput(store, taskID)
	case "DOCS":
		return BuildDocsInput(store, taskID)
	case "PR_CREATION":
		return BuildPRCreationInput(store, taskID)
	default:
		return "", fmt.Errorf("unknown phase: %s", phase)
	}
}

// BuildInputForPhaseJSON constructs a JSON representation of the phase input.
// Useful for structured injection into agent prompts.
func BuildInputForPhaseJSON(store *Store, taskID string, phase string, existingWork *ExistingWorkContext) (string, error) {
	var input interface{}

	switch phase {
	case "PLAN":
		issueCtx := store.GetIssueContext(taskID)
		if issueCtx == nil {
			return "", fmt.Errorf("issue context not found for task %s", taskID)
		}
		input = issueCtx

	case "IMPLEMENT":
		issueCtx := store.GetIssueContext(taskID)
		plan := store.GetPlanOutput(taskID)
		if issueCtx == nil || plan == nil {
			return "", fmt.Errorf("missing required data for IMPLEMENT phase")
		}
		input = ImplementInput{
			IssueContext: *issueCtx,
			Plan:         *plan,
			ExistingWork: existingWork,
		}

	case "REVIEW":
		issueCtx := store.GetIssueContext(taskID)
		plan := store.GetPlanOutput(taskID)
		impl := store.GetImplementOutput(taskID)
		if issueCtx == nil || plan == nil || impl == nil {
			return "", fmt.Errorf("missing required data for REVIEW phase")
		}
		input = ReviewInput{
			IssueContext:   *issueCtx,
			PlanSummary:    plan.Summary,
			Implementation: *impl,
		}

	case "DOCS":
		issueCtx := store.GetIssueContext(taskID)
		plan := store.GetPlanOutput(taskID)
		impl := store.GetImplementOutput(taskID)
		if issueCtx == nil || plan == nil || impl == nil {
			return "", fmt.Errorf("missing required data for DOCS phase")
		}
		input = DocsInput{
			IssueContext: *issueCtx,
			PlanSummary:  plan.Summary,
			FilesChanged: impl.FilesChanged,
		}

	case "PR_CREATION":
		issueCtx := store.GetIssueContext(taskID)
		plan := store.GetPlanOutput(taskID)
		impl := store.GetImplementOutput(taskID)
		if issueCtx == nil || plan == nil || impl == nil {
			return "", fmt.Errorf("missing required data for PR_CREATION phase")
		}
		input = PRCreationInput{
			IssueContext: *issueCtx,
			BranchName:   impl.BranchName,
			PlanSummary:  plan.Summary,
			FilesChanged: impl.FilesChanged,
			TestsPassed:  impl.TestsPassed,
			TestOutput:   impl.TestOutput,
		}

	default:
		return "", fmt.Errorf("unknown phase: %s", phase)
	}

	data, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal phase input: %w", err)
	}
	return string(data), nil
}
