package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/andywolf/agentium/internal/agent"
)

func (c *Controller) fetchIssueDetails(ctx context.Context) []issueDetail {
	c.logInfo("Fetching issue details")

	issues := make([]issueDetail, 0, len(c.config.Tasks))
	c.issueDetailsByNumber = make(map[string]*issueDetail, len(c.config.Tasks))

	for _, taskID := range c.config.Tasks {
		// Use gh CLI to fetch issue
		cmd := c.execCommand(ctx, "gh", "issue", "view", taskID,
			"--repo", c.config.Repository,
			"--json", "number,title,body,labels,comments",
		)
		cmd.Env = c.envWithGitHubToken()

		output, err := cmd.Output()
		if err != nil {
			c.logWarning("failed to fetch issue #%s: %v", taskID, err)
			continue
		}

		var issue issueDetail
		if err := json.Unmarshal(output, &issue); err != nil {
			c.logWarning("failed to parse issue #%s: %v", taskID, err)
			continue
		}

		issues = append(issues, issue)
	}

	// Build O(1) lookup map after collecting all issues
	for i := range issues {
		issueNumStr := fmt.Sprintf("%d", issues[i].Number)
		c.issueDetailsByNumber[issueNumStr] = &issues[i]
	}

	return issues
}

// formatExternalComments formats non-Agentium issue comments as structured markdown.
// Comments containing the Agentium signature (<!-- agentium:) are filtered out.
// Returns an empty string if no external comments exist.
func formatExternalComments(comments []issueComment) string {
	var sb strings.Builder
	for _, comment := range comments {
		if strings.Contains(comment.Body, "<!-- agentium:") {
			continue
		}
		date := comment.CreatedAt
		if len(date) >= 10 {
			date = date[:10] // trim to YYYY-MM-DD
		}
		sb.WriteString(fmt.Sprintf("**@%s** (%s):\n> %s\n\n",
			comment.Author.Login, date,
			strings.ReplaceAll(strings.TrimSpace(comment.Body), "\n", "\n> ")))
	}
	return sb.String()
}

// detectExistingWork checks GitHub for existing branches and PRs related to an issue.
// It searches for branches matching the pattern */issue-<N>-* (any prefix).
func (c *Controller) detectExistingWork(ctx context.Context, issueNumber string) *agent.ExistingWork {
	// Check for existing open PRs with branch matching */issue-<N>-*
	// Use --limit to ensure we scan enough PRs in repos with many open PRs
	// Search pattern matches any prefix (feature, bug, enhancement, agentium, etc.)
	branchPattern := fmt.Sprintf("/issue-%s-", issueNumber)
	cmd := c.execCommand(ctx, "gh", "pr", "list",
		"--repo", c.config.Repository,
		"--state", "open",
		"--limit", "200",
		"--json", "number,title,headRefName",
	)
	cmd.Dir = c.workDir
	cmd.Env = c.envWithGitHubToken()

	if output, err := cmd.Output(); err == nil {
		var prs []struct {
			Number      int    `json:"number"`
			Title       string `json:"title"`
			HeadRefName string `json:"headRefName"`
		}
		if unmarshalErr := json.Unmarshal(output, &prs); unmarshalErr == nil {
			// Filter for branches matching */issue-<N>-*
			for _, pr := range prs {
				if strings.Contains(pr.HeadRefName, branchPattern) {
					c.logInfo("Found existing PR #%d for issue #%s on branch %s",
						pr.Number, issueNumber, pr.HeadRefName)
					return &agent.ExistingWork{
						PRNumber: fmt.Sprintf("%d", pr.Number),
						PRTitle:  pr.Title,
						Branch:   pr.HeadRefName,
					}
				}
			}
		}
	} else {
		c.logWarning("failed to list PRs for existing work detection on issue #%s: %v", issueNumber, err)
	}

	// No PR found; check for existing remote branches matching */issue-<N>-*
	// First, list all remote branches
	cmd = c.execCommand(ctx, "git", "branch", "-r")
	cmd.Dir = c.workDir

	if output, err := cmd.Output(); err == nil {
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, line := range lines {
			branch := strings.TrimSpace(line)
			branch = strings.TrimPrefix(branch, "origin/")
			// Match pattern: */issue-<N>-*
			if strings.Contains(branch, branchPattern) {
				c.logInfo("Found existing branch for issue #%s: %s", issueNumber, branch)
				return &agent.ExistingWork{
					Branch: branch,
				}
			}
		}
	} else {
		c.logWarning("failed to list remote branches for existing work detection on issue #%s: %v", issueNumber, err)
	}

	c.logInfo("No existing work found for issue #%s", issueNumber)
	return nil
}

// branchPrefixForLabels returns the branch prefix based on the first issue label.
// Returns "feature" as default when no labels are present or if sanitization yields empty string.
func branchPrefixForLabels(labels []issueLabel) string {
	if len(labels) > 0 {
		prefix := sanitizeBranchPrefix(labels[0].Name)
		if prefix != "" {
			return prefix
		}
	}
	return "feature" // Default when no labels or invalid label
}

// sanitizeBranchPrefix converts a label name to a valid git branch prefix.
// It handles characters that are invalid in git refs: ~ ^ : ? * [ \ space and more.
func sanitizeBranchPrefix(label string) string {
	// Lowercase first
	result := strings.ToLower(label)

	// Replace any character that's not alphanumeric or hyphen with hyphen
	var sanitized strings.Builder
	for _, r := range result {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			sanitized.WriteRune(r)
		} else {
			sanitized.WriteRune('-')
		}
	}
	result = sanitized.String()

	// Collapse consecutive hyphens
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}

	// Trim leading and trailing hyphens
	result = strings.Trim(result, "-")

	return result
}
