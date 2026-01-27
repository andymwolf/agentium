package controller

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// instanceSignature returns a signature like "agentium:gcp:agentium-abc123" for debugging.
// This is appended to comments to help identify which instance posted them.
func (c *Controller) instanceSignature() string {
	provider := c.config.CloudProvider
	if provider == "" {
		provider = "unknown"
	}
	return fmt.Sprintf("agentium:%s:%s", provider, c.config.ID)
}

// appendSignature adds the instance signature as an HTML comment to the body.
// The signature is invisible when rendered but helps with debugging.
func (c *Controller) appendSignature(body string) string {
	return fmt.Sprintf("%s\n\n<!-- %s -->", body, c.instanceSignature())
}

// postPhaseComment posts a progress comment on the GitHub issue for the current task.
// This is best-effort: errors are logged but never cause the controller to crash.
func (c *Controller) postPhaseComment(ctx context.Context, phase TaskPhase, iteration int, summary string) {
	if c.activeTaskType != "issue" {
		return
	}

	body := fmt.Sprintf("### Phase: %s (iteration %d)\n\n%s", phase, iteration, summary)
	c.postIssueComment(ctx, body)
}

// postJudgeComment posts a judge verdict comment on the GitHub issue.
// This is best-effort: errors are logged but never cause the controller to crash.
func (c *Controller) postJudgeComment(ctx context.Context, phase TaskPhase, result JudgeResult) {
	if c.activeTaskType != "issue" {
		return
	}

	var body string
	switch result.Verdict {
	case VerdictAdvance:
		body = fmt.Sprintf("**Judge:** Phase `%s` — ADVANCE", phase)
	case VerdictIterate:
		body = fmt.Sprintf("**Judge:** Phase `%s` — ITERATE\n\n> %s", phase, result.Feedback)
	case VerdictBlocked:
		body = fmt.Sprintf("**Judge:** Phase `%s` — BLOCKED\n\n> %s", phase, result.Feedback)
	}

	c.postIssueComment(ctx, body)
}

// postIssueComment posts a comment on the active issue. Best-effort.
func (c *Controller) postIssueComment(ctx context.Context, body string) {
	body = c.appendSignature(body)
	cmd := exec.CommandContext(ctx, "gh", "issue", "comment", c.activeTask,
		"--repo", c.config.Repository,
		"--body", body,
	)
	cmd.Env = append(os.Environ(), fmt.Sprintf("GITHUB_TOKEN=%s", c.gitHubToken))
	cmd.Dir = c.workDir

	if output, err := cmd.CombinedOutput(); err != nil {
		c.logWarning("failed to post issue comment: %v (output: %s)", err, string(output))
	}
}

// postPRComment posts a comment on a pull request. Best-effort.
func (c *Controller) postPRComment(ctx context.Context, prNumber string, body string) {
	if prNumber == "" {
		c.logWarning("postPRComment called with empty PR number")
		return
	}

	body = c.appendSignature(body)
	cmd := exec.CommandContext(ctx, "gh", "pr", "comment", prNumber,
		"--repo", c.config.Repository,
		"--body", body,
	)
	cmd.Env = append(os.Environ(), fmt.Sprintf("GITHUB_TOKEN=%s", c.gitHubToken))
	cmd.Dir = c.workDir

	if output, err := cmd.CombinedOutput(); err != nil {
		c.logWarning("failed to post PR comment: %v (output: %s)", err, string(output))
	}
}

// postReviewerFeedback posts reviewer feedback as a comment on the issue.
// This is best-effort: errors are logged but never cause the controller to crash.
func (c *Controller) postReviewerFeedback(ctx context.Context, phase TaskPhase, iteration int, feedback string) {
	if c.activeTaskType != "issue" {
		return
	}

	body := fmt.Sprintf("### Reviewer Feedback: %s (iteration %d)\n\n%s", phase, iteration, truncateForComment(feedback))
	c.postIssueComment(ctx, body)
}

// postPRReviewSummary posts a review summary comment on the associated PR.
// This includes reviewer feedback and is posted during the IMPLEMENT phase.
func (c *Controller) postPRReviewSummary(ctx context.Context, prNumber string, phase TaskPhase, iteration int, reviewFeedback string) {
	if prNumber == "" {
		return
	}

	body := fmt.Sprintf("### Review Summary: %s (iteration %d)\n\n%s", phase, iteration, truncateForComment(reviewFeedback))
	c.postPRComment(ctx, prNumber, body)
}

// postPRJudgeVerdict posts a judge verdict comment on the associated PR.
// This is called when the verdict is ITERATE or BLOCKED to make it visible on the PR.
func (c *Controller) postPRJudgeVerdict(ctx context.Context, prNumber string, phase TaskPhase, result JudgeResult) {
	if prNumber == "" {
		return
	}

	// Only post for non-ADVANCE verdicts (ITERATE and BLOCKED need visibility)
	if result.Verdict == VerdictAdvance {
		return
	}

	var body string
	switch result.Verdict {
	case VerdictIterate:
		body = fmt.Sprintf("**🔄 Judge Verdict:** Phase `%s` — ITERATE\n\n> %s", phase, result.Feedback)
	case VerdictBlocked:
		body = fmt.Sprintf("**⛔ Judge Verdict:** Phase `%s` — BLOCKED\n\n> %s", phase, result.Feedback)
	}

	c.postPRComment(ctx, prNumber, body)
}

// planMarker is the delimiter used to identify the AGENTIUM PLAN section in issue bodies.
const planMarker = "<!-- #AGENTIUM PLAN# -->"

// updateIssuePlan updates the GitHub issue body to append or update the AGENTIUM PLAN section.
// The original issue description is preserved above the plan marker.
// This is best-effort: errors are logged but never cause the controller to crash.
func (c *Controller) updateIssuePlan(ctx context.Context, plan string) {
	if c.activeTaskType != "issue" {
		return
	}

	// Fetch current issue body
	cmd := exec.CommandContext(ctx, "gh", "issue", "view", c.activeTask,
		"--repo", c.config.Repository,
		"--json", "body",
		"--jq", ".body",
	)
	cmd.Env = append(os.Environ(), fmt.Sprintf("GITHUB_TOKEN=%s", c.gitHubToken))
	cmd.Dir = c.workDir

	output, err := cmd.Output()
	if err != nil {
		c.logWarning("failed to fetch issue body for plan update: %v", err)
		return
	}

	currentBody := strings.TrimSpace(string(output))

	// Build new body: preserve original content, update/add plan section
	var newBody string
	if idx := strings.Index(currentBody, planMarker); idx >= 0 {
		// Replace existing plan section
		newBody = strings.TrimSpace(currentBody[:idx]) + "\n\n" + planMarker + "\n## Implementation Plan\n\n" + plan
	} else {
		// Append new plan section
		newBody = currentBody + "\n\n" + planMarker + "\n## Implementation Plan\n\n" + plan
	}

	// Update issue body
	cmd = exec.CommandContext(ctx, "gh", "issue", "edit", c.activeTask,
		"--repo", c.config.Repository,
		"--body", newBody,
	)
	cmd.Env = append(os.Environ(), fmt.Sprintf("GITHUB_TOKEN=%s", c.gitHubToken))
	cmd.Dir = c.workDir

	if output, err := cmd.CombinedOutput(); err != nil {
		c.logWarning("failed to update issue body with plan: %v (output: %s)", err, string(output))
	} else {
		c.logInfo("Updated issue #%s with implementation plan", c.activeTask)
	}
}

// getPRNumberForTask returns the PR number associated with the current task, if any.
// For PR tasks, returns the active task ID directly.
// For issue tasks, checks existing work (continuation) and task state (newly created PRs).
func (c *Controller) getPRNumberForTask() string {
	// For PR tasks, the active task IS the PR number
	if c.activeTaskType == "pr" {
		return c.activeTask
	}

	// Check existing work first (continuation of existing PR)
	if c.activeTaskExistingWork != nil && c.activeTaskExistingWork.PRNumber != "" {
		return c.activeTaskExistingWork.PRNumber
	}

	// Check task state for newly created PR
	taskID := fmt.Sprintf("%s:%s", c.activeTaskType, c.activeTask)
	if state, ok := c.taskStates[taskID]; ok && state.PRNumber != "" {
		return state.PRNumber
	}

	return ""
}

// postReviewFeedbackForPhase posts reviewer feedback to the appropriate location based on phase.
// - PLAN phase: Posts to the associated issue
// - IMPLEMENT phase: Posts to the associated PR (if one exists)
// - Other phases: Posts to the issue
func (c *Controller) postReviewFeedbackForPhase(ctx context.Context, phase TaskPhase, iteration int, feedback string) {
	if c.activeTaskType != "issue" {
		return
	}

	switch phase {
	case PhasePlan:
		// Plan review feedback goes to the issue
		c.postReviewerFeedback(ctx, phase, iteration, feedback)

	case PhaseImplement:
		// Implementation review feedback goes to the PR (if one exists)
		if prNumber := c.getPRNumberForTask(); prNumber != "" {
			c.postPRReviewSummary(ctx, prNumber, phase, iteration, feedback)
		} else {
			// Fallback to issue if no PR yet
			c.postReviewerFeedback(ctx, phase, iteration, feedback)
		}

	default:
		// Other phases post to the issue
		c.postReviewerFeedback(ctx, phase, iteration, feedback)
	}
}
