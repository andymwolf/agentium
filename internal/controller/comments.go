package controller

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

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
