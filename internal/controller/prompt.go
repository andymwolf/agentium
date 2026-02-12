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

		// For IMPLEMENT with handoff plan: skip body/comments (plan replaces them)
		if phase == PhaseImplement && c.hasPlanForTask(issueNumber) {
			sb.WriteString("Refer to the **Phase Input** section below for the implementation plan and requirements.\n\n")
		} else {
			// Full context for PLAN, DOCS, VERIFY, or fallback when no plan exists
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

// hasPlanForTask checks whether a handoff plan exists for the given issue.
func (c *Controller) hasPlanForTask(issueNumber string) bool {
	if !c.isHandoffEnabled() || c.handoffStore == nil {
		return false
	}
	taskID := taskKey("issue", issueNumber)
	return c.handoffStore.GetPlanOutput(taskID) != nil
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

// buildIterateFeedbackSection constructs the feedback prompt for continuation iterations.
// It retrieves the previous iteration's reviewer feedback (EvalFeedback) and judge
// directives (JudgeDirective) from memory, formatting them into a conversational prompt
// that leads with what matters most: the required fixes, then supporting context,
// then the handoff signal template so the worker can submit its work.
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

	// Opening narrative — one sentence telling the agent what happened
	switch phase {
	case PhasePlan:
		sb.WriteString("Your implementation plan was reviewed. The judge is requesting changes before it can advance to implementation.\n\n")
	case PhaseImplement:
		sb.WriteString("Your code changes were reviewed. The judge is requesting fixes before this phase can advance.\n\n")
	case PhaseDocs:
		sb.WriteString("Your documentation updates were reviewed. The judge is requesting changes.\n\n")
	case PhaseVerify:
		sb.WriteString("Your verification attempt needs further work.\n\n")
	default:
		sb.WriteString("Your previous iteration was reviewed and needs changes.\n\n")
	}

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

	// Judge directives first — the most important part, up front
	if len(judgeDirectives) > 0 {
		sb.WriteString("## Here's what you need to fix:\n\n")
		for _, d := range judgeDirectives {
			sb.WriteString(d)
			sb.WriteString("\n\n")
		}
	}

	// Reviewer analysis — supporting context, placed after directives
	if len(reviewerFeedback) > 0 {
		sb.WriteString("## The reviewer also noted:\n\n")
		for _, f := range reviewerFeedback {
			sb.WriteString(f)
			sb.WriteString("\n\n")
		}
	}

	// Phase-specific completion section with handoff template
	switch phase {
	case PhasePlan:
		// Include current plan so the worker can see what to update
		if c.handoffStore != nil {
			if planOutput := c.handoffStore.GetPlanOutput(taskID); planOutput != nil {
				planJSON, err := json.Marshal(planOutput)
				if err == nil {
					sb.WriteString("## Your current plan\n\n")
					sb.WriteString("Update this to address the feedback above:\n\n")
					sb.WriteString("```json\n")
					sb.WriteString(string(planJSON))
					sb.WriteString("\n```\n\n")
				}
			}
		}

		sb.WriteString("## Submit your revised plan\n\n")
		sb.WriteString("When you've addressed the feedback, emit your updated plan. The reviewer evaluates your plan from this signal, not from conversation text.\n\n")
		sb.WriteString("```\nAGENTIUM_HANDOFF: {\n")
		sb.WriteString("  \"summary\": \"...\",\n")
		sb.WriteString("  \"files_to_modify\": [\"...\"],\n")
		sb.WriteString("  \"files_to_create\": [\"...\"],\n")
		sb.WriteString("  \"implementation_steps\": [{\"order\": 1, \"description\": \"...\", \"file\": \"...\"}],\n")
		sb.WriteString("  \"testing_approach\": \"...\"\n")
		sb.WriteString("}\n```\n\n")

	case PhaseImplement:
		diffBase := "main"
		if parentBranch != "" {
			diffBase = parentBranch
		}
		sb.WriteString("## Submit your changes\n\n")
		sb.WriteString(fmt.Sprintf("Review your existing work (`git diff %s..HEAD`), make targeted fixes to address the feedback, then commit and push.\n\n", diffBase))
		sb.WriteString("When done, emit the handoff signal:\n\n")
		sb.WriteString("```\nAGENTIUM_HANDOFF: {\n")
		sb.WriteString("  \"branch_name\": \"...\",\n")
		sb.WriteString("  \"commits\": [{\"hash\": \"...\", \"message\": \"...\"}],\n")
		sb.WriteString("  \"files_changed\": [\"...\"],\n")
		sb.WriteString("  \"tests_passed\": true\n")
		sb.WriteString("}\n```\n\n")

	case PhaseDocs:
		sb.WriteString("## Submit your changes\n\n")
		sb.WriteString("When you've updated the documentation, emit the handoff signal:\n\n")
		sb.WriteString("```\nAGENTIUM_HANDOFF: {\n")
		sb.WriteString("  \"docs_updated\": [\"...\"],\n")
		sb.WriteString("  \"readme_changed\": true\n")
		sb.WriteString("}\n```\n\n")

	case PhaseVerify:
		sb.WriteString("## Submit your results\n\n")
		sb.WriteString("When verification issues are resolved, emit the handoff signal:\n\n")
		sb.WriteString("```\nAGENTIUM_HANDOFF: {\n")
		sb.WriteString("  \"checks_passed\": true,\n")
		sb.WriteString("  \"merge_successful\": true,\n")
		sb.WriteString("  \"merge_sha\": \"...\"\n")
		sb.WriteString("}\n```\n\n")
	}

	// Brief FEEDBACK_RESPONSE note — one line
	sb.WriteString("For each reviewer point above, emit `AGENTIUM_MEMORY: FEEDBACK_RESPONSE [ADDRESSED|DECLINED|PARTIAL] <summary> - <response>` to log how you handled it.\n\n")

	return sb.String()
}
