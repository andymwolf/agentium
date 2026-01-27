package handoff

import (
	"encoding/json"
	"fmt"
)

// Builder constructs phase inputs from the handoff store.
type Builder struct {
	store *Store
}

// NewBuilder creates a new input builder.
func NewBuilder(store *Store) *Builder {
	return &Builder{store: store}
}

// BuildInputForPhase constructs the appropriate input for a phase.
// Returns JSON string suitable for injection into agent context.
func (b *Builder) BuildInputForPhase(taskID string, phase Phase) (string, error) {
	var input interface{}
	var err error

	switch phase {
	case PhasePlan:
		input, err = b.buildPlanInput(taskID)
	case PhaseImplement:
		input, err = b.buildImplementInput(taskID)
	case PhaseDocs:
		input, err = b.buildDocsInput(taskID)
	case PhasePRCreation:
		input, err = b.buildPRCreationInput(taskID)
	default:
		return "", fmt.Errorf("unknown phase: %s", phase)
	}

	if err != nil {
		return "", err
	}

	// Marshal to JSON with nice formatting
	data, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal %s input: %w", phase, err)
	}

	return string(data), nil
}

// buildPlanInput constructs input for the PLAN phase.
func (b *Builder) buildPlanInput(taskID string) (*PlanInput, error) {
	issue := b.store.GetIssueContext(taskID)
	if issue == nil {
		return nil, fmt.Errorf("no issue context found for task %s", taskID)
	}

	return &PlanInput{
		Issue: *issue,
	}, nil
}

// buildImplementInput constructs input for the IMPLEMENT phase.
func (b *Builder) buildImplementInput(taskID string) (*ImplementInput, error) {
	issue := b.store.GetIssueContext(taskID)
	if issue == nil {
		return nil, fmt.Errorf("no issue context found for task %s", taskID)
	}

	plan := b.store.GetPlanOutput(taskID)
	if plan == nil {
		return nil, fmt.Errorf("no plan output found for task %s", taskID)
	}

	input := &ImplementInput{
		Issue: *issue,
		Plan:  *plan,
	}

	// Check for existing work from previous implementation attempts
	impl := b.store.GetImplementOutput(taskID)
	if impl != nil && impl.BranchName != "" {
		input.ExistingWork = &ExistingWork{
			BranchName:    impl.BranchName,
			FilesModified: impl.FilesChanged,
			Commits:       extractCommitMessages(impl.Commits),
		}
	}

	return input, nil
}

// buildReviewInput constructs input for the REVIEW phase.
func (b *Builder) buildReviewInput(taskID string) (*ReviewInput, error) {
	issue := b.store.GetIssueContext(taskID)
	if issue == nil {
		return nil, fmt.Errorf("no issue context found for task %s", taskID)
	}

	plan := b.store.GetPlanOutput(taskID)
	if plan == nil {
		return nil, fmt.Errorf("no plan output found for task %s", taskID)
	}

	impl := b.store.GetImplementOutput(taskID)
	if impl == nil {
		return nil, fmt.Errorf("no implementation output found for task %s", taskID)
	}

	return &ReviewInput{
		Issue:          *issue,
		PlanSummary:    plan.Summary,
		Implementation: *impl,
	}, nil
}

// buildDocsInput constructs input for the DOCS phase.
func (b *Builder) buildDocsInput(taskID string) (*DocsInput, error) {
	issue := b.store.GetIssueContext(taskID)
	if issue == nil {
		return nil, fmt.Errorf("no issue context found for task %s", taskID)
	}

	plan := b.store.GetPlanOutput(taskID)
	if plan == nil {
		return nil, fmt.Errorf("no plan output found for task %s", taskID)
	}

	impl := b.store.GetImplementOutput(taskID)
	if impl == nil {
		return nil, fmt.Errorf("no implementation output found for task %s", taskID)
	}

	return &DocsInput{
		Issue:        *issue,
		PlanSummary:  plan.Summary,
		FilesChanged: impl.FilesChanged,
	}, nil
}

// buildPRCreationInput constructs input for the PR_CREATION phase.
func (b *Builder) buildPRCreationInput(taskID string) (*PRCreationInput, error) {
	issue := b.store.GetIssueContext(taskID)
	if issue == nil {
		return nil, fmt.Errorf("no issue context found for task %s", taskID)
	}

	plan := b.store.GetPlanOutput(taskID)
	if plan == nil {
		return nil, fmt.Errorf("no plan output found for task %s", taskID)
	}

	impl := b.store.GetImplementOutput(taskID)
	if impl == nil {
		return nil, fmt.Errorf("no implementation output found for task %s", taskID)
	}

	input := &PRCreationInput{
		Issue:        *issue,
		BranchName:   impl.BranchName,
		PlanSummary:  plan.Summary,
		FilesChanged: impl.FilesChanged,
	}

	// Include test results if available
	if impl.TestOutput != "" {
		input.TestResults = impl.TestOutput
	}

	return input, nil
}

// BuildMarkdownContext creates a human-readable markdown representation
// of the phase input, suitable for injection into agent prompts.
func (b *Builder) BuildMarkdownContext(taskID string, phase Phase) (string, error) {
	input, err := b.BuildInputForPhase(taskID, phase)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("## Phase Input: %s\n\nThe following structured data has been provided for this phase:\n\n```json\n%s\n```\n\nUse this context to guide your work. When you complete this phase, emit an AGENTIUM_HANDOFF signal with your structured output.\n", phase, input), nil
}

// extractCommitMessages extracts just the messages from commits.
func extractCommitMessages(commits []Commit) []string {
	messages := make([]string, len(commits))
	for i, c := range commits {
		messages[i] = c.Message
	}
	return messages
}
