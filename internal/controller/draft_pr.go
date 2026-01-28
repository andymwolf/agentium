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

// maybeCreateDraftPR ensures a draft PR exists for the current branch.
// It first checks if a PR already exists (from worker, previous run, etc.),
// and if so, updates state to track it. Otherwise, it creates a new draft PR.
// Returns nil if no action needed or PR exists/creation succeeds.
func (c *Controller) maybeCreateDraftPR(ctx context.Context, taskID string) error {
	state := c.taskStates[taskID]
	if state == nil {
		return fmt.Errorf("no task state for %s", taskID)
	}

	// Skip if draft PR already tracked in state
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
		// Only create draft PR for branches matching */issue-*-* pattern
		if !strings.Contains(detected, "/issue-") {
			c.logInfo("Skipping draft PR creation: branch %q does not match */issue-* pattern", detected)
			return nil
		}
		branchName = detected
	}

	// Check if a PR already exists for this branch (from worker, previous run, etc.)
	existingPR, findErr := c.findExistingPRForBranch(ctx, branchName)
	if findErr != nil {
		c.logWarning("Failed to check for existing PR: %v", findErr)
		// Continue to try creating one
	}
	if existingPR != nil {
		c.logInfo("Found existing PR #%s for branch %s", existingPR.Number, branchName)
		state.DraftPRCreated = true
		state.PRNumber = existingPR.Number
		c.updateHandoffWithPRInfo(taskID, existingPR.Number, existingPR.URL, state.PhaseIteration)
		return nil
	}

	// No existing PR - push branch if needed and create draft PR
	// Push the branch (handles both unpushed commits and already-pushed branches)
	if pushErr := c.ensureBranchPushed(ctx, branchName); pushErr != nil {
		return fmt.Errorf("failed to push branch: %w", pushErr)
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
*This draft PR was automatically created by Agentium during the IMPLEMENT phase.*
*Instance: %s*`, issueNumber, c.instanceSignature())

	c.logInfo("Creating draft PR for issue #%s", issueNumber)
	createCmd := exec.CommandContext(ctx, "gh", "pr", "create",
		"--draft",
		"--title", prTitle,
		"--body", prBody,
		"--repo", c.config.Repository,
	)
	createCmd.Dir = c.workDir
	createCmd.Env = append(os.Environ(), fmt.Sprintf("GITHUB_TOKEN=%s", c.gitHubToken))

	createOutput, createErr := createCmd.CombinedOutput()
	if createErr != nil {
		// Check if error is due to PR already existing (race condition or worker created it)
		if strings.Contains(string(createOutput), "already exists") {
			c.logInfo("PR already exists for branch (created concurrently), checking again")
			existingPR, retryErr := c.findExistingPRForBranch(ctx, branchName)
			if retryErr == nil && existingPR != nil {
				state.DraftPRCreated = true
				state.PRNumber = existingPR.Number
				c.updateHandoffWithPRInfo(taskID, existingPR.Number, existingPR.URL, state.PhaseIteration)
				return nil
			}
		}
		return fmt.Errorf("failed to create draft PR: %w (output: %s)", createErr, string(createOutput))
	}

	// Parse PR number from output (gh pr create outputs the PR URL)
	prNumber, prURL := parsePRCreateOutput(string(createOutput))
	if prNumber == "" {
		c.logWarning("Could not parse PR number from gh output: %s", string(createOutput))
		// Still mark as created since the command succeeded
	}

	// Update state
	state.DraftPRCreated = true
	state.PRNumber = prNumber
	c.updateHandoffWithPRInfo(taskID, prNumber, prURL, state.PhaseIteration)

	c.logInfo("Draft PR #%s created successfully: %s", prNumber, prURL)
	return nil
}

// existingPRInfo holds information about an existing PR.
type existingPRInfo struct {
	Number string
	URL    string
}

// findExistingPRForBranch checks if a PR already exists for the given branch.
func (c *Controller) findExistingPRForBranch(ctx context.Context, branchName string) (*existingPRInfo, error) {
	// Use gh pr view to check for existing PR on this branch
	cmd := exec.CommandContext(ctx, "gh", "pr", "view", branchName,
		"--repo", c.config.Repository,
		"--json", "number,url",
	)
	cmd.Dir = c.workDir
	cmd.Env = append(os.Environ(), fmt.Sprintf("GITHUB_TOKEN=%s", c.gitHubToken))

	output, cmdErr := cmd.Output()
	if cmdErr != nil {
		// No PR exists for this branch (gh pr view exits non-zero)
		return nil, nil
	}

	var prInfo struct {
		Number int    `json:"number"`
		URL    string `json:"url"`
	}
	if unmarshalErr := json.Unmarshal(output, &prInfo); unmarshalErr != nil {
		return nil, fmt.Errorf("failed to parse PR info: %w", unmarshalErr)
	}

	return &existingPRInfo{
		Number: fmt.Sprintf("%d", prInfo.Number),
		URL:    prInfo.URL,
	}, nil
}

// ensureBranchPushed pushes the branch to origin if it has unpushed commits,
// or if the remote branch doesn't exist yet.
func (c *Controller) ensureBranchPushed(ctx context.Context, branchName string) error {
	// Check if remote branch exists
	checkCmd := exec.CommandContext(ctx, "git", "ls-remote", "--heads", "origin", branchName)
	checkCmd.Dir = c.workDir
	checkOutput, checkErr := checkCmd.Output()
	if checkErr != nil {
		return fmt.Errorf("failed to check remote branch: %w", checkErr)
	}

	remoteExists := strings.TrimSpace(string(checkOutput)) != ""

	// Check for unpushed commits if remote exists
	hasUnpushed := false
	if remoteExists {
		var unpushedErr error
		hasUnpushed, unpushedErr = c.branchHasUnpushedCommits(ctx, branchName)
		if unpushedErr != nil {
			c.logWarning("Failed to check for unpushed commits: %v", unpushedErr)
		}
	}

	// Push if remote doesn't exist or we have unpushed commits
	if !remoteExists || hasUnpushed {
		c.logInfo("Pushing branch %s to origin", branchName)
		pushCmd := exec.CommandContext(ctx, "git", "push", "-u", "origin", branchName)
		pushCmd.Dir = c.workDir
		pushCmd.Env = append(os.Environ(), fmt.Sprintf("GITHUB_TOKEN=%s", c.gitHubToken))
		pushOutput, pushErr := pushCmd.CombinedOutput()
		if pushErr != nil {
			return fmt.Errorf("push failed: %w (output: %s)", pushErr, string(pushOutput))
		}
	} else {
		c.logInfo("Branch %s already pushed and up to date", branchName)
	}

	return nil
}

// updateHandoffWithPRInfo updates the handoff store with PR information.
func (c *Controller) updateHandoffWithPRInfo(taskID, prNumber, prURL string, iteration int) {
	if !c.isHandoffEnabled() || prNumber == "" {
		return
	}

	implOutput := c.handoffStore.GetImplementOutput(taskID)
	if implOutput != nil {
		implOutput.DraftPRNumber = parseIntOrZero(prNumber)
		implOutput.DraftPRUrl = prURL
		if err := c.handoffStore.StorePhaseOutput(taskID, handoff.PhaseImplement, iteration, implOutput); err != nil {
			c.logWarning("Failed to update handoff store with PR info: %v", err)
			return // Don't try to save if store failed
		}
		// Persist to disk (following pattern from phase_loop.go)
		if err := c.handoffStore.Save(); err != nil {
			c.logWarning("Failed to persist handoff store after PR info update: %v", err)
		}
	}
}

// finalizeDraftPR marks the draft PR as ready for review, or posts a NOMERGE
// comment if the controller forced advancement at max iterations.
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

	// Check if NOMERGE handling is needed (controller forced ADVANCE at max iterations)
	if state.ControllerOverrode {
		reason := "Controller forced ADVANCE at max iterations"
		c.logWarning("PR #%s requires human review: %s", state.PRNumber, reason)
		c.postNOMERGEComment(ctx, state.PRNumber, reason)
		// Keep PR as draft - do not mark as ready
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

// extractIssueNumber extracts the issue number from a branch name like "<prefix>/issue-123-description".
// Supports any prefix (feature, bug, enhancement, agentium, etc.).
func extractIssueNumber(branchName string) string {
	re := regexp.MustCompile(`\w+/issue-(\d+)`)
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
