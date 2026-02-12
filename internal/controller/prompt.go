package controller

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/andywolf/agentium/internal/agent"
	"github.com/andywolf/agentium/internal/handoff"
	"github.com/andywolf/agentium/internal/memory"
	"github.com/andywolf/agentium/internal/template"
)

// renderWithParameters applies template variable substitution to a prompt string.
// It merges built-in variables (repository, issue_url, etc.) with user-provided parameters,
// where user parameters take precedence on name collision.
func (c *Controller) renderWithParameters(prompt string) string {
	// Build built-in variables from session config
	builtins := map[string]string{
		"repository": c.config.Repository,
	}

	// Add issue_url: prefer explicit PromptContext value, fall back to derived URL
	if c.config.PromptContext != nil && c.config.PromptContext.IssueURL != "" {
		builtins["issue_url"] = c.config.PromptContext.IssueURL
	} else if c.activeTask != "" && c.activeTaskType == "issue" && c.config.Repository != "" {
		builtins["issue_url"] = fmt.Sprintf("https://github.com/%s/issues/%s", c.config.Repository, c.activeTask)
	}

	// Add issue_number only for issue tasks (not PR tasks)
	if c.activeTask != "" && c.activeTaskType == "issue" {
		builtins["issue_number"] = c.activeTask
	}

	// Merge with user parameters (user params override builtins)
	var userParams map[string]string
	if c.config.PromptContext != nil {
		userParams = c.config.PromptContext.Parameters
	}

	merged := template.MergeVariables(builtins, userParams)
	return template.RenderPrompt(prompt, merged)
}

// buildPromptForTask builds a focused prompt for a single issue, incorporating existing work context.
// The phase parameter controls whether implementation instructions are included:
// - For IMPLEMENT phase (or empty phase): include full implementation instructions
// - For other phases (PLAN, DOCS, etc.): defer to the phase-specific system prompt
func (c *Controller) buildPromptForTask(issueNumber string, existingWork *agent.ExistingWork, phase TaskPhase) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("You are working on repository: %s\n\n", c.config.Repository))

	// O(1) lookup for issue detail
	issue := c.issueDetailsByNumber[issueNumber]

	sb.WriteString(fmt.Sprintf("## Your Task: Issue #%s\n\n", issueNumber))
	if issue != nil {
		sb.WriteString(fmt.Sprintf("**Title:** %s\n\n", issue.Title))
		if issue.Body != "" {
			sb.WriteString(fmt.Sprintf("**Description:**\n%s\n\n", issue.Body))
		}
		if len(issue.Comments) > 0 {
			if formatted := formatExternalComments(issue.Comments); formatted != "" {
				sb.WriteString("**Prior Discussion:**\n\n")
				sb.WriteString(formatted)
			}
		}
	}

	// Always include existing work context (branch/PR info) regardless of phase
	if existingWork != nil {
		sb.WriteString("## Existing Work Detected\n\n")
		if existingWork.PRNumber != "" {
			sb.WriteString(fmt.Sprintf("An open PR already exists for this issue: **PR #%s** (%s)\n",
				existingWork.PRNumber, existingWork.PRTitle))
			sb.WriteString(fmt.Sprintf("Branch: `%s`\n\n", existingWork.Branch))
		} else {
			sb.WriteString(fmt.Sprintf("An existing branch was found for this issue: `%s`\n\n", existingWork.Branch))
		}
	}

	// Only include detailed implementation instructions for IMPLEMENT phase (or when phase is empty/unspecified)
	// For PLAN, DOCS, and other phases, defer to the phase-specific system prompt
	switch phase {
	case PhaseImplement, "":
		if existingWork != nil {
			if existingWork.PRNumber != "" {
				sb.WriteString("### Instructions\n\n")
				sb.WriteString(fmt.Sprintf("1. Check out the existing branch: `git checkout %s`\n", existingWork.Branch))
				sb.WriteString("2. Review the current state of the code on this branch\n")
				sb.WriteString("3. Continue implementation or fix any issues found\n")
				sb.WriteString("4. Run tests to verify correctness\n")
				sb.WriteString(fmt.Sprintf("5. Push updates to the existing branch: `git push origin %s`\n", existingWork.Branch))
				sb.WriteString("6. The existing PR will update automatically\n\n")
				sb.WriteString("### DO NOT\n\n")
				sb.WriteString("- Do NOT create a new branch\n")
				sb.WriteString("- Do NOT create a new PR\n")
				sb.WriteString("- Do NOT close or delete the existing PR\n")
			} else {
				sb.WriteString("### Instructions\n\n")
				sb.WriteString(fmt.Sprintf("1. Check out the existing branch: `git checkout %s`\n", existingWork.Branch))
				sb.WriteString("2. Review what's already been done on this branch\n")
				sb.WriteString("3. Continue implementation or fix issues\n")
				sb.WriteString("4. Run tests to verify correctness\n")
				sb.WriteString("5. Commit and push your changes\n")
				sb.WriteString("6. Create a PR linking to the issue (if one doesn't exist yet)\n\n")
				sb.WriteString("### DO NOT\n\n")
				sb.WriteString("- Do NOT create a new branch (use the existing one)\n")
			}
		} else {
			// No existing work — fresh start
			// Check if this issue depends on a parent issue's branch
			taskID := taskKey("issue", issueNumber)
			parentBranch := ""
			if state, ok := c.taskStates[taskID]; ok && state.ParentBranch != "" {
				parentBranch = state.ParentBranch
			}

			// Determine branch prefix from issue labels
			branchPrefix := "feature" // Default
			if issue != nil {
				branchPrefix = branchPrefixForLabels(issue.Labels)
			}

			sb.WriteString("### Instructions\n\n")
			if parentBranch != "" {
				sb.WriteString(fmt.Sprintf("**NOTE:** This issue depends on work from another issue. You must branch from: `%s`\n\n", parentBranch))
				sb.WriteString(fmt.Sprintf("1. First, check out the parent branch: `git checkout %s`\n", parentBranch))
				sb.WriteString(fmt.Sprintf("2. Create your new branch from it: `git checkout -b %s/issue-%s-<short-description>`\n", branchPrefix, issueNumber))
				sb.WriteString("3. Implement the fix or feature\n")
				sb.WriteString("4. Run tests to verify correctness\n")
				sb.WriteString("5. Commit your changes with a descriptive message\n")
				sb.WriteString("6. Push the branch\n")
				sb.WriteString("7. Create a pull request targeting `main` (NOT the parent branch)\n\n")
				sb.WriteString("### IMPORTANT\n\n")
				sb.WriteString("- Your PR must target `main`, not the parent branch\n")
				sb.WriteString("- The PR diff will include parent changes until the parent PR is merged\n")
				sb.WriteString("- After the parent PR merges, GitHub will auto-resolve the diff\n")
			} else {
				sb.WriteString(fmt.Sprintf("1. Create a new branch: `%s/issue-%s-<short-description>`\n", branchPrefix, issueNumber))
				sb.WriteString("2. Implement the fix or feature\n")
				sb.WriteString("3. Run tests to verify correctness\n")
				sb.WriteString("4. Commit your changes with a descriptive message\n")
				sb.WriteString("5. Push the branch\n")
				sb.WriteString("6. Create a pull request linking to the issue\n\n")
			}
		}
		sb.WriteString("Use 'gh' CLI for GitHub operations and 'git' for version control.\n")
		sb.WriteString(fmt.Sprintf("The repository is already cloned at %s.\n", c.workDir))
	case PhaseVerify:
		// VERIFY phase: provide PR number and repo context for CI checking and merging
		taskID := taskKey("issue", issueNumber)
		state := c.taskStates[taskID]
		sb.WriteString("### Instructions\n\n")
		sb.WriteString("Follow the instructions in your system prompt to verify CI checks and merge the PR.\n\n")
		if state != nil && state.PRNumber != "" {
			sb.WriteString(fmt.Sprintf("**PR Number:** %s\n", state.PRNumber))
		}
		sb.WriteString(fmt.Sprintf("**Repository:** %s\n", c.config.Repository))
		sb.WriteString(fmt.Sprintf("The repository is cloned at %s.\n", c.workDir))
	default:
		// For PLAN, DOCS, and other phases: defer to the phase-specific system prompt
		sb.WriteString("### Instructions\n\n")
		sb.WriteString("Follow the instructions in your system prompt to complete this phase.\n")
		sb.WriteString(fmt.Sprintf("The repository is cloned at %s.\n", c.workDir))
	}

	// Apply template variable substitution
	return c.renderWithParameters(sb.String())
}

// buildIssueContext creates a handoff.IssueContext from the active issue details.
func (c *Controller) buildIssueContext() *handoff.IssueContext {
	if c.activeTask == "" || c.activeTaskType != "issue" {
		return nil
	}

	// O(1) lookup for issue in issueDetails
	issue := c.issueDetailsByNumber[c.activeTask]
	if issue == nil {
		return nil
	}
	return &handoff.IssueContext{
		Number:     issue.Number,
		Title:      issue.Title,
		Body:       issue.Body,
		Repository: c.config.Repository,
	}
}

// buildIterateFeedbackSection constructs the feedback section for ITERATE prompts.
// It retrieves the previous iteration's reviewer feedback (EvalFeedback) and judge
// directives (JudgeDirective) from memory, formatting them into a structured section
// that guides the worker on what must be addressed.
//
// The returned section contains:
// - Phase-specific required actions (re-emit AGENTIUM_HANDOFF, make code changes, etc.)
// - Current plan content (for PLAN phase, so the worker knows what to update)
// - Guidance on how to interpret feedback types
// - Judge directives (REQUIRED action items)
// - Reviewer analysis (detailed context)
//
// Returns empty string if no feedback is available for the previous iteration.
func (c *Controller) buildIterateFeedbackSection(taskID string, phaseIteration int, parentBranch string, phase TaskPhase) string {
	if c.memoryStore == nil {
		return ""
	}

	entries := c.memoryStore.GetPreviousIterationFeedback(taskID, phaseIteration)
	if len(entries) == 0 {
		return ""
	}

	var sb strings.Builder

	sb.WriteString("## Feedback from Previous Iteration\n\n")

	// Phase-specific required actions — tells the worker what it MUST produce
	switch phase {
	case PhasePlan:
		sb.WriteString("### Required Actions\n\n")
		sb.WriteString("You MUST emit an updated `AGENTIUM_HANDOFF` signal with your revised plan that addresses the feedback below.\n")
		sb.WriteString("Do NOT skip the handoff signal — the reviewer evaluates your plan from the `AGENTIUM_HANDOFF` output, not from conversation text.\n\n")
		// Include current plan so the worker can see what to update
		if c.handoffStore != nil {
			if planOutput := c.handoffStore.GetPlanOutput(taskID); planOutput != nil {
				planJSON, err := json.Marshal(planOutput)
				if err == nil {
					sb.WriteString("**Your current plan (update this to address the feedback):**\n")
					sb.WriteString("```json\n")
					sb.WriteString(string(planJSON))
					sb.WriteString("\n```\n\n")
				}
			}
		}
	case PhaseImplement:
		sb.WriteString("### Required Actions\n\n")
		sb.WriteString("You MUST make code changes to address the feedback below, commit them, push, and emit an updated `AGENTIUM_HANDOFF` signal.\n")
		sb.WriteString("Do NOT re-emit the same handoff with unchanged commits — the reviewer will detect no progress and request another iteration.\n\n")
	case PhaseDocs:
		sb.WriteString("### Required Actions\n\n")
		sb.WriteString("You MUST update the documentation to address the feedback below and emit an updated `AGENTIUM_HANDOFF` signal.\n\n")
	case PhaseVerify:
		sb.WriteString("### Required Actions\n\n")
		sb.WriteString("You MUST address the verification issues described in the feedback below and emit an updated `AGENTIUM_HANDOFF` signal.\n\n")
	}

	// Guidance on how to interpret feedback
	sb.WriteString("**How to use this feedback:**\n")
	sb.WriteString("- **Reviewer feedback**: Detailed analysis - consider all points as context\n")
	sb.WriteString("- **Judge directives**: Required action items - you MUST address these\n\n")
	sb.WriteString("**Approach:**\n")
	diffBase := "main"
	if parentBranch != "" {
		diffBase = parentBranch
	}
	sb.WriteString(fmt.Sprintf("- Your implementation already exists on this branch. Run `git log --oneline %s..HEAD` and `git diff %s..HEAD` to review your existing work before making changes.\n", diffBase, diffBase))
	sb.WriteString("- Make **targeted, surgical fixes** to address the feedback. Do not rewrite or start over unless the judge directive explicitly says to take a different approach.\n")
	sb.WriteString("- If a directive asks for a specific fix, make that fix and nothing else. If it asks to reconsider the approach, then a broader change is warranted.\n\n")

	// Separate reviewer feedback and judge directives
	var reviewerFeedback, judgeDirectives []string
	for _, e := range entries {
		switch e.Type {
		case memory.EvalFeedback:
			reviewerFeedback = append(reviewerFeedback, e.Content)
		case memory.JudgeDirective:
			judgeDirectives = append(judgeDirectives, e.Content)
		}
	}

	// Judge directives first (required actions)
	if len(judgeDirectives) > 0 {
		sb.WriteString("### Judge Directives (REQUIRED)\n\n")
		for _, d := range judgeDirectives {
			sb.WriteString(d)
			sb.WriteString("\n\n")
		}
	}

	// Reviewer feedback second (context)
	if len(reviewerFeedback) > 0 {
		sb.WriteString("### Reviewer Analysis (Context)\n\n")
		for _, f := range reviewerFeedback {
			sb.WriteString(f)
			sb.WriteString("\n\n")
		}
	}

	// Instructions for responding to feedback
	sb.WriteString("### Responding to Feedback\n\n")
	sb.WriteString("For each point in the reviewer analysis above, emit a `FEEDBACK_RESPONSE` memory signal indicating how you handled it.\n\n")
	sb.WriteString("Format: `AGENTIUM_MEMORY: FEEDBACK_RESPONSE [STATUS] <feedback summary> - <your response>`\n\n")
	sb.WriteString("STATUS values:\n")
	sb.WriteString("- `[ADDRESSED]` — You fixed or implemented the feedback point\n")
	sb.WriteString("- `[DECLINED]` — You chose not to act on it (explain why)\n")
	sb.WriteString("- `[PARTIAL]` — You partially addressed it (explain what remains)\n\n")
	sb.WriteString("Emit one signal per feedback point. This is expected for every reviewer feedback item.\n\n")

	return sb.String()
}
