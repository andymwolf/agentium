package controller

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// CommentRole identifies which agent or controller component posted a comment.
type CommentRole string

const (
	RoleWorker             CommentRole = "Worker"
	RoleComplexityAssessor CommentRole = "Complexity Assessor"
	RoleReviewer           CommentRole = "Reviewer"
	RoleJudge              CommentRole = "Judge"
	RoleController         CommentRole = "Controller"
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

// postCommentForPhase routes a comment to the correct GitHub target based on the current phase.
// IMPLEMENT and VERIFY phases post to the PR (with fallback to the issue if no PR exists yet).
// All other phases (PLAN, DOCS, etc.) post to the issue.
// This is best-effort: errors are logged but never cause the controller to crash.
func (c *Controller) postCommentForPhase(ctx context.Context, phase TaskPhase, body string) {
	if c.activeTaskType != "issue" {
		return
	}
	switch phase {
	case PhaseImplement, PhaseVerify:
		if prNumber := c.getPRNumberForTask(); prNumber != "" {
			c.postPRComment(ctx, prNumber, body)
			return
		}
		// Fallback to issue if no PR yet (e.g. first IMPLEMENT iteration)
		c.postIssueComment(ctx, body)
	default:
		// PLAN, DOCS, and any other phase → issue
		c.postIssueComment(ctx, body)
	}
}

// postPhaseComment posts a progress comment routed by phase.
// This is best-effort: errors are logged but never cause the controller to crash.
func (c *Controller) postPhaseComment(ctx context.Context, phase TaskPhase, iteration int, role CommentRole, summary string) {
	body := fmt.Sprintf("### Phase: %s — %s (iteration %d)\n\n%s", phase, role, iteration, summary)
	c.postCommentForPhase(ctx, phase, body)
}

// postJudgeComment posts a judge verdict comment routed by phase.
// This is best-effort: errors are logged but never cause the controller to crash.
func (c *Controller) postJudgeComment(ctx context.Context, phase TaskPhase, iteration int, result JudgeResult) {
	header := fmt.Sprintf("### Phase: %s — %s (iteration %d)", phase, RoleJudge, iteration)
	var body string
	switch result.Verdict {
	case VerdictAdvance:
		body = fmt.Sprintf("%s\n\n**Verdict:** ADVANCE", header)
	case VerdictIterate:
		body = fmt.Sprintf("%s\n\n**Verdict:** ITERATE\n\n> %s", header, result.Feedback)
	case VerdictBlocked:
		body = fmt.Sprintf("%s\n\n**Verdict:** BLOCKED\n\n> %s", header, result.Feedback)
	}

	c.postCommentForPhase(ctx, phase, body)
}

// postIssueComment posts a comment on the active issue. Best-effort.
func (c *Controller) postIssueComment(ctx context.Context, body string) {
	body = c.appendSignature(body)
	cmd := exec.CommandContext(ctx, "gh", "issue", "comment", c.activeTask,
		"--repo", c.config.Repository,
		"--body-file", "-",
	)
	cmd.Env = c.envWithGitHubToken()
	cmd.Dir = c.workDir
	cmd.Stdin = strings.NewReader(body)

	if output, err := cmd.CombinedOutput(); err != nil {
		c.logWarning("failed to post issue comment: %v (output: %s)", err, string(output))
	} else {
		c.logInfo("Posted comment to issue #%s", c.activeTask)
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
		"--body-file", "-",
	)
	cmd.Env = c.envWithGitHubToken()
	cmd.Dir = c.workDir
	cmd.Stdin = strings.NewReader(body)

	if output, err := cmd.CombinedOutput(); err != nil {
		c.logWarning("failed to post PR comment: %v (output: %s)", err, string(output))
	} else {
		c.logInfo("Posted comment to PR #%s", prNumber)
	}
}

// postImplementationPlan posts the implementation plan as a comment on the GitHub issue.
// This follows the "append only" principle - we never modify the issue body.
// This is best-effort: errors are logged but never cause the controller to crash.
func (c *Controller) postImplementationPlan(ctx context.Context, plan string) {
	if c.activeTaskType != "issue" {
		return
	}

	body := fmt.Sprintf("## Implementation Plan\n\n%s", plan)
	c.postIssueComment(ctx, body)
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
	taskID := taskKey(c.activeTaskType, c.activeTask)
	if state, ok := c.taskStates[taskID]; ok && state.PRNumber != "" {
		return state.PRNumber
	}

	return ""
}

// postNOMERGEComment posts a warning comment on the PR indicating that it
// requires human review before merging. This is called when the controller
// forced ADVANCE at max iterations or when a NOMERGE verdict was given.
func (c *Controller) postNOMERGEComment(ctx context.Context, prNumber string, reason string) {
	if prNumber == "" {
		return
	}

	body := fmt.Sprintf(`## NOMERGE - Human Review Required

This pull request was completed but **requires human review** before merging.

**Reason:** %s

### What this means

The automated review process did not achieve full confidence in this change.
Please review the changes carefully before merging.

### Recommended actions

1. Review all code changes in this PR
2. Verify tests are passing and adequate
3. Check for edge cases or potential issues
4. Mark as ready for review when satisfied

---
*This PR remains in draft status until a human reviewer approves it.*`, reason)

	c.postPRComment(ctx, prNumber, body)
}

// postReviewFeedbackForPhase posts reviewer feedback routed by phase via postCommentForPhase.
func (c *Controller) postReviewFeedbackForPhase(ctx context.Context, phase TaskPhase, iteration int, feedback string) {
	body := fmt.Sprintf("### Phase: %s — %s (iteration %d)\n\n%s", phase, RoleReviewer, iteration, feedback)
	c.postCommentForPhase(ctx, phase, body)
}
