package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/andywolf/agentium/internal/handoff"
)

// maybeCreateDraftPR creates a draft PR if this is the first iteration with commits.
// It checks if a draft PR has already been created for this task and skips if so.
// Returns nil if no action needed or PR creation succeeds.
func (c *Controller) maybeCreateDraftPR(ctx context.Context, taskID string) error {
	state := c.taskStates[taskID]
	if state == nil {
		return fmt.Errorf("no task state for %s", taskID)
	}

	// Skip if draft PR already created
	if state.DraftPRCreated {
		return nil
	}

	// Get the branch name from handoff store if available
	branchName := ""
	if c.isHandoffEnabled() {
		implOutput := c.handoffStore.GetImplementOutput(taskID)
		if implOutput != nil && implOutput.BranchName != "" {
			branchName = implOutput.BranchName
		}
	}

	// If no branch name from handoff, try to detect it
	if branchName == "" {
		detected, err := c.detectCurrentBranch(ctx)
		if err != nil {
			return fmt.Errorf("failed to detect branch: %w", err)
		}
		// Only create draft PR for agentium/ branches
		if !strings.HasPrefix(detected, "agentium/") {
			c.logInfo("Skipping draft PR creation: branch %q is not an agentium branch", detected)
			return nil
		}
		branchName = detected
	}

	// Check if branch has any commits to push
	hasCommits, err := c.branchHasUnpushedCommits(ctx, branchName)
	if err != nil {
		c.logWarning("Failed to check for unpushed commits: %v", err)
		// Continue anyway - push will fail if there's nothing to push
	}
	if !hasCommits {
		c.logInfo("Skipping draft PR creation: no unpushed commits on branch %q", branchName)
		return nil
	}

	// Push the branch
	c.logInfo("Pushing branch %s for draft PR", branchName)
	pushCmd := exec.CommandContext(ctx, "git", "push", "-u", "origin", branchName)
	pushCmd.Dir = c.workDir
	pushCmd.Env = append(os.Environ(), fmt.Sprintf("GITHUB_TOKEN=%s", c.gitHubToken))
	if output, err := pushCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to push branch: %w (output: %s)", err, string(output))
	}

	// Extract issue number from branch name (agentium/issue-123-description)
	issueNumber := extractIssueNumber(branchName)
	if issueNumber == "" {
		issueNumber = state.ID // Fallback to task ID
	}

	// Get issue title for PR title
	prTitle := fmt.Sprintf("Issue #%s: Draft implementation", issueNumber)
	for _, issue := range c.issueDetails {
		if fmt.Sprintf("%d", issue.Number) == issueNumber {
			prTitle = fmt.Sprintf("Issue #%s: %s", issueNumber, issue.Title)
			break
		}
	}

	// Create draft PR
	prBody := fmt.Sprintf(`Closes #%s

## Summary
This is a draft PR - implementation is in progress.

## Status
- [ ] Implementation complete
- [ ] Tests passing
- [ ] Documentation updated

---
*This draft PR was automatically created by Agentium during the IMPLEMENT phase.*`, issueNumber)

	c.logInfo("Creating draft PR for issue #%s", issueNumber)
	createCmd := exec.CommandContext(ctx, "gh", "pr", "create",
		"--draft",
		"--title", prTitle,
		"--body", prBody,
		"--repo", c.config.Repository,
	)
	createCmd.Dir = c.workDir
	createCmd.Env = append(os.Environ(), fmt.Sprintf("GITHUB_TOKEN=%s", c.gitHubToken))

	output, err := createCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create draft PR: %w (output: %s)", err, string(output))
	}

	// Parse PR number from output (gh pr create outputs the PR URL)
	prNumber, prURL := parsePRCreateOutput(string(output))
	if prNumber == "" {
		c.logWarning("Could not parse PR number from gh output: %s", string(output))
		// Still mark as created since the command succeeded
	}

	// Update state
	state.DraftPRCreated = true
	state.PRNumber = prNumber

	// Store draft PR info in handoff store if available
	if c.isHandoffEnabled() && prNumber != "" {
		// Update the implement output with draft PR info
		implOutput := c.handoffStore.GetImplementOutput(taskID)
		if implOutput != nil {
			implOutput.DraftPRNumber = parseIntOrZero(prNumber)
			implOutput.DraftPRUrl = prURL
			if err := c.handoffStore.StorePhaseOutput(taskID, handoff.PhaseImplement, state.PhaseIteration, implOutput); err != nil {
				c.logWarning("Failed to update handoff store with draft PR info: %v", err)
			}
		}
	}

	c.logInfo("Draft PR #%s created successfully: %s", prNumber, prURL)
	return nil
}

// finalizeDraftPR marks the draft PR as ready for review.
// This is called when the workflow reaches PhaseComplete.
func (c *Controller) finalizeDraftPR(ctx context.Context, taskID string) error {
	state := c.taskStates[taskID]
	if state == nil {
		return fmt.Errorf("no task state for %s", taskID)
	}

	// Skip if no PR number
	if state.PRNumber == "" {
		c.logInfo("Skipping PR finalization: no PR number in state")
		return nil
	}

	c.logInfo("Marking PR #%s as ready for review", state.PRNumber)

	readyCmd := exec.CommandContext(ctx, "gh", "pr", "ready", state.PRNumber,
		"--repo", c.config.Repository,
	)
	readyCmd.Dir = c.workDir
	readyCmd.Env = append(os.Environ(), fmt.Sprintf("GITHUB_TOKEN=%s", c.gitHubToken))

	output, err := readyCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to mark PR as ready: %w (output: %s)", err, string(output))
	}

	c.logInfo("PR #%s is now ready for review", state.PRNumber)
	return nil
}

// detectCurrentBranch returns the current git branch name.
func (c *Controller) detectCurrentBranch(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = c.workDir
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// branchHasUnpushedCommits checks if the branch has commits not yet pushed to origin.
func (c *Controller) branchHasUnpushedCommits(ctx context.Context, branch string) (bool, error) {
	// Check if remote tracking branch exists
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", fmt.Sprintf("origin/%s", branch))
	cmd.Dir = c.workDir
	if err := cmd.Run(); err != nil {
		// Remote branch doesn't exist, so all local commits are unpushed
		return true, nil
	}

	// Count commits ahead of origin
	cmd = exec.CommandContext(ctx, "git", "rev-list", "--count", fmt.Sprintf("origin/%s..%s", branch, branch))
	cmd.Dir = c.workDir
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}

	count := strings.TrimSpace(string(output))
	return count != "0", nil
}

// extractIssueNumber extracts the issue number from a branch name like "agentium/issue-123-description".
func extractIssueNumber(branchName string) string {
	re := regexp.MustCompile(`agentium/issue-(\d+)`)
	matches := re.FindStringSubmatch(branchName)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// parsePRCreateOutput extracts the PR number and URL from gh pr create output.
// The output typically looks like: "https://github.com/owner/repo/pull/123\n"
func parsePRCreateOutput(output string) (number string, url string) {
	output = strings.TrimSpace(output)
	// gh pr create outputs the PR URL on success
	re := regexp.MustCompile(`https://github\.com/[^/]+/[^/]+/pull/(\d+)`)
	matches := re.FindStringSubmatch(output)
	if len(matches) >= 2 {
		return matches[1], matches[0]
	}
	return "", output
}

// parseIntOrZero parses a string as int, returning 0 on error.
func parseIntOrZero(s string) int {
	var n int
	if err := json.Unmarshal([]byte(s), &n); err != nil {
		return 0
	}
	return n
}
