// Package handoff provides structured phase input/output contracts for the
// sub-agents model. Each phase receives curated inputs and produces structured
// outputs that the orchestrator routes forward to the next phase.
package handoff

// IssueContext provides the minimal context about an issue being worked on.
// This is passed as input to the PLAN phase.
type IssueContext struct {
	Number     string `json:"number"`
	Title      string `json:"title"`
	Body       string `json:"body"`
	Repository string `json:"repository"`
}

// ImplementationStep describes a single step in an implementation plan.
type ImplementationStep struct {
	Number      int    `json:"number"`
	Description string `json:"description"`
	File        string `json:"file,omitempty"` // Primary file affected (optional)
}

// PlanOutput is the structured output from the PLAN phase.
type PlanOutput struct {
	Summary             string               `json:"summary"`
	FilesToModify       []string             `json:"files_to_modify"`
	FilesToCreate       []string             `json:"files_to_create"`
	ImplementationSteps []ImplementationStep `json:"implementation_steps"`
	TestingApproach     string               `json:"testing_approach"`
}

// ExistingWorkContext describes prior work detected for an issue (branch/PR).
type ExistingWorkContext struct {
	Branch   string `json:"branch,omitempty"`
	PRNumber string `json:"pr_number,omitempty"`
	PRTitle  string `json:"pr_title,omitempty"`
}

// ImplementInput is the input for the IMPLEMENT phase.
type ImplementInput struct {
	IssueContext IssueContext         `json:"issue_context"`
	Plan         PlanOutput           `json:"plan"`
	ExistingWork *ExistingWorkContext `json:"existing_work,omitempty"`
}

// CommitInfo describes a git commit made during implementation.
type CommitInfo struct {
	SHA     string `json:"sha"`
	Message string `json:"message"`
}

// ImplementOutput is the structured output from the IMPLEMENT phase.
type ImplementOutput struct {
	BranchName   string       `json:"branch_name"`
	Commits      []CommitInfo `json:"commits"`
	FilesChanged []string     `json:"files_changed"`
	TestsPassed  bool         `json:"tests_passed"`
	TestOutput   string       `json:"test_output,omitempty"`
}

// ReviewInput is the input for the REVIEW phase.
type ReviewInput struct {
	IssueContext   IssueContext    `json:"issue_context"`
	PlanSummary    string          `json:"plan_summary"`
	Implementation ImplementOutput `json:"implementation"`
}

// ReviewIssue describes an issue found during code review.
type ReviewIssue struct {
	File        string `json:"file"`
	Line        int    `json:"line,omitempty"`
	Description string `json:"description"`
	Severity    string `json:"severity"` // "error", "warning", "suggestion"
	Fixed       bool   `json:"fixed"`
}

// ReviewOutput is the structured output from the REVIEW phase.
type ReviewOutput struct {
	IssuesFound      []ReviewIssue `json:"issues_found"`
	FixesApplied     []string      `json:"fixes_applied"`
	RegressionNeeded bool          `json:"regression_needed"`
	RegressionReason string        `json:"regression_reason,omitempty"`
}

// DocsInput is the input for the DOCS phase.
type DocsInput struct {
	IssueContext IssueContext `json:"issue_context"`
	PlanSummary  string       `json:"plan_summary"`
	FilesChanged []string     `json:"files_changed"`
}

// DocsOutput is the structured output from the DOCS phase.
type DocsOutput struct {
	DocsUpdated   []string `json:"docs_updated"`
	ReadmeChanged bool     `json:"readme_changed"`
}

// PRCreationInput is the input for the PR_CREATION phase.
type PRCreationInput struct {
	IssueContext IssueContext `json:"issue_context"`
	BranchName   string       `json:"branch_name"`
	PlanSummary  string       `json:"plan_summary"`
	FilesChanged []string     `json:"files_changed"`
	TestsPassed  bool         `json:"tests_passed"`
	TestOutput   string       `json:"test_output,omitempty"`
}

// PRCreationOutput is the structured output from the PR_CREATION phase.
type PRCreationOutput struct {
	PRNumber int    `json:"pr_number"`
	PRURL    string `json:"pr_url"`
}

// Config holds handoff feature configuration.
type Config struct {
	Enabled bool `json:"enabled"`
}
