// Package handoff provides structured data contracts for phase transitions.
// It implements a sub-agents model where each phase receives curated inputs
// and produces structured outputs for the next phase.
package handoff

import "time"

// Phase represents the execution phases in the pipeline.
type Phase string

const (
	PhasePlan       Phase = "PLAN"
	PhaseImplement  Phase = "IMPLEMENT"
	PhaseDocs       Phase = "DOCS"
	PhasePRCreation Phase = "PR_CREATION"
)

// IssueContext contains the minimal context about the issue being worked on.
// This is the common input shared across all phases.
type IssueContext struct {
	Number     int      `json:"number"`
	Title      string   `json:"title"`
	Body       string   `json:"body"`
	Repository string   `json:"repository"`
	Labels     []string `json:"labels,omitempty"`
}

// -----------------------------------------------------------------------------
// PLAN Phase
// -----------------------------------------------------------------------------

// PlanInput is the curated input for the PLAN phase.
type PlanInput struct {
	Issue IssueContext `json:"issue"`
}

// ImplementationStep describes a single step in the implementation plan.
type ImplementationStep struct {
	Order       int    `json:"order"`
	Description string `json:"description"`
	File        string `json:"file,omitempty"`
	Notes       string `json:"notes,omitempty"`
}

// PlanOutput is the structured output from the PLAN phase.
type PlanOutput struct {
	Summary             string               `json:"summary"`
	FilesToModify       []string             `json:"files_to_modify"`
	FilesToCreate       []string             `json:"files_to_create"`
	ImplementationSteps []ImplementationStep `json:"implementation_steps"`
	TestingApproach     string               `json:"testing_approach"`
	Complexity          string               `json:"complexity,omitempty"` // SIMPLE or COMPLEX
}

// -----------------------------------------------------------------------------
// IMPLEMENT Phase
// -----------------------------------------------------------------------------

// ImplementInput is the curated input for the IMPLEMENT phase.
type ImplementInput struct {
	Issue        IssueContext  `json:"issue"`
	Plan         PlanOutput    `json:"plan"`
	ExistingWork *ExistingWork `json:"existing_work,omitempty"`
}

// ExistingWork captures any prior implementation state (e.g., from regression).
type ExistingWork struct {
	BranchName    string   `json:"branch_name,omitempty"`
	FilesModified []string `json:"files_modified,omitempty"`
	Commits       []string `json:"commits,omitempty"`
}

// Commit represents a single git commit made during implementation.
type Commit struct {
	Hash    string `json:"hash"`
	Message string `json:"message"`
}

// ImplementOutput is the structured output from the IMPLEMENT phase.
type ImplementOutput struct {
	BranchName     string   `json:"branch_name"`
	Commits        []Commit `json:"commits"`
	FilesChanged   []string `json:"files_changed"`
	TestsPassed    bool     `json:"tests_passed"`
	TestOutput     string   `json:"test_output,omitempty"`
	DraftPRNumber  int      `json:"draft_pr_number,omitempty"`
	DraftPRUrl     string   `json:"draft_pr_url,omitempty"`
}

// -----------------------------------------------------------------------------
// REVIEW Phase
// -----------------------------------------------------------------------------

// ReviewInput is the curated input for the REVIEW phase.
type ReviewInput struct {
	Issue          IssueContext    `json:"issue"`
	PlanSummary    string          `json:"plan_summary"`
	Implementation ImplementOutput `json:"implementation"`
}

// ReviewIssue describes a single issue found during code review.
type ReviewIssue struct {
	Severity    string `json:"severity"` // ERROR, WARNING, SUGGESTION
	File        string `json:"file"`
	Line        int    `json:"line,omitempty"`
	Description string `json:"description"`
	Suggestion  string `json:"suggestion,omitempty"`
}

// ReviewOutput is the structured output from the REVIEW phase.
type ReviewOutput struct {
	IssuesFound      []ReviewIssue `json:"issues_found"`
	FixesApplied     []string      `json:"fixes_applied"`
	RegressionNeeded bool          `json:"regression_needed"`
	RegressionReason string        `json:"regression_reason,omitempty"`
}

// -----------------------------------------------------------------------------
// DOCS Phase
// -----------------------------------------------------------------------------

// DocsInput is the curated input for the DOCS phase.
type DocsInput struct {
	Issue        IssueContext `json:"issue"`
	PlanSummary  string       `json:"plan_summary"`
	FilesChanged []string     `json:"files_changed"`
}

// DocsOutput is the structured output from the DOCS phase.
type DocsOutput struct {
	DocsUpdated   []string `json:"docs_updated"`
	ReadmeChanged bool     `json:"readme_changed"`
}

// -----------------------------------------------------------------------------
// PR_CREATION Phase
// -----------------------------------------------------------------------------

// PRCreationInput is the curated input for the PR_CREATION phase.
type PRCreationInput struct {
	Issue        IssueContext `json:"issue"`
	BranchName   string       `json:"branch_name"`
	PlanSummary  string       `json:"plan_summary"`
	FilesChanged []string     `json:"files_changed"`
	TestResults  string       `json:"test_results,omitempty"`
}

// PRCreationOutput is the structured output from the PR_CREATION phase.
type PRCreationOutput struct {
	PRNumber int    `json:"pr_number"`
	PRUrl    string `json:"pr_url"`
}

// -----------------------------------------------------------------------------
// Handoff Envelope
// -----------------------------------------------------------------------------

// HandoffData is the envelope for all phase outputs, stored in the handoff store.
type HandoffData struct {
	TaskID    string    `json:"task_id"`
	Phase     Phase     `json:"phase"`
	Timestamp time.Time `json:"timestamp"`
	Iteration int       `json:"iteration"`

	// Only one of these will be populated based on Phase
	PlanOutput       *PlanOutput       `json:"plan_output,omitempty"`
	ImplementOutput  *ImplementOutput  `json:"implement_output,omitempty"`
	ReviewOutput     *ReviewOutput     `json:"review_output,omitempty"`
	DocsOutput       *DocsOutput       `json:"docs_output,omitempty"`
	PRCreationOutput *PRCreationOutput `json:"pr_creation_output,omitempty"`
}

// GetOutput returns the populated output based on the phase, or nil if none.
func (h *HandoffData) GetOutput() interface{} {
	switch h.Phase {
	case PhasePlan:
		return h.PlanOutput
	case PhaseImplement:
		return h.ImplementOutput
	case PhaseDocs:
		return h.DocsOutput
	case PhasePRCreation:
		return h.PRCreationOutput
	default:
		return nil
	}
}
