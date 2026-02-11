package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// subIssuesGraphQLResponse represents the GraphQL response for sub-issues.
type subIssuesGraphQLResponse struct {
	Data struct {
		Repository struct {
			Issue struct {
				SubIssues struct {
					Nodes []struct {
						Number int    `json:"number"`
						State  string `json:"state"`
					} `json:"nodes"`
				} `json:"subIssues"`
			} `json:"issue"`
		} `json:"repository"`
	} `json:"data"`
}

// parseRepoOwnerName extracts owner and name from a repository string.
// Supports formats: "owner/repo", "github.com/owner/repo", "https://github.com/owner/repo".
func parseRepoOwnerName(repo string) (owner, name string, err error) {
	repo = strings.TrimPrefix(repo, "https://")
	repo = strings.TrimPrefix(repo, "http://")
	repo = strings.TrimPrefix(repo, "github.com/")
	repo = strings.TrimSuffix(repo, ".git")

	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid repository format: %q", repo)
	}
	return parts[0], parts[1], nil
}

// fetchOpenSubIssues queries the GitHub GraphQL API for open sub-issues of the given issue.
// Returns a slice of open sub-issue number strings, or an error if the API call fails.
func (c *Controller) fetchOpenSubIssues(ctx context.Context, issueID string) ([]string, error) {
	owner, name, err := parseRepoOwnerName(c.config.Repository)
	if err != nil {
		return nil, fmt.Errorf("cannot parse repository: %w", err)
	}

	issueNum, err := strconv.Atoi(issueID)
	if err != nil {
		return nil, fmt.Errorf("invalid issue number %q: %w", issueID, err)
	}

	query := fmt.Sprintf(`{ repository(owner: %q, name: %q) { issue(number: %d) { subIssues(first: 50) { nodes { number state } } } } }`,
		owner, name, issueNum)

	cmd := c.execCommand(ctx, "gh", "api", "graphql", "-f", "query="+query)
	cmd.Env = c.envWithGitHubToken()

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("GraphQL query failed: %w", err)
	}

	var resp subIssuesGraphQLResponse
	if err := json.Unmarshal(output, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse GraphQL response: %w", err)
	}

	var ids []string
	for _, node := range resp.Data.Repository.Issue.SubIssues.Nodes {
		if strings.EqualFold(node.State, "OPEN") {
			ids = append(ids, strconv.Itoa(node.Number))
		}
	}
	return ids, nil
}

// detectSubIssues queries the GitHub sub-issues API with caching and exponential
// backoff retry (1s, 2s, 4s, 8s, 16s). Returns an error after 6 failed attempts.
func (c *Controller) detectSubIssues(ctx context.Context, issueID string) ([]string, error) {
	if cached, ok := c.subIssueCache[issueID]; ok {
		return cached, nil
	}

	var ids []string
	var lastErr error
	delays := []time.Duration{0, 1 * time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second}
	for attempt, delay := range delays {
		if attempt > 0 {
			c.logWarning("Sub-issues API failed for #%s (attempt %d/6), retrying in %s: %v",
				issueID, attempt, delay, lastErr)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		ids, lastErr = c.fetchOpenSubIssues(ctx, issueID)
		if lastErr == nil {
			c.subIssueCache[issueID] = ids
			return ids, nil
		}
	}
	return nil, fmt.Errorf("sub-issues API failed for #%s after 6 attempts: %w", issueID, lastErr)
}

// expandParentIssue expands a parent issue's sub-issues into the task queue.
// The caller provides the sub-issue IDs (from the API or regex fallback).
func (c *Controller) expandParentIssue(ctx context.Context, parentID string, subIssueIDs []string) error {
	c.logInfo("Issue #%s: expanding %d sub-issues: %v", parentID, len(subIssueIDs), subIssueIDs)

	// Fetch details for sub-issues not already in cache
	if err := c.fetchSubIssueDetails(ctx, subIssueIDs); err != nil {
		return fmt.Errorf("failed to fetch sub-issue details: %w", err)
	}

	// Create TaskState entries and queue items for each sub-issue
	initialPhase := PhaseImplement
	if c.isPhaseLoopEnabled() {
		initialPhase = PhasePlan
	}

	var newItems []TaskQueueItem
	for _, id := range subIssueIDs {
		tk := taskKey("issue", id)
		if _, exists := c.taskStates[tk]; !exists {
			c.taskStates[tk] = &TaskState{
				ID:    id,
				Type:  "issue",
				Phase: initialPhase,
			}
		}
		newItems = append(newItems, TaskQueueItem{Type: "issue", ID: id})
	}

	// Insert sub-issues into queue after the parent
	c.insertAfterTask(parentID, newItems)

	// Rebuild dependency graph and reorder sub-issues within their block
	c.rebuildDependencyGraphWithSubIssues(subIssueIDs)

	// Track parent -> sub-issue mapping
	c.parentSubIssues[parentID] = subIssueIDs

	// Post expansion comment on parent
	c.postParentStatusComment(ctx, parentID, subIssueIDs, "expanded")

	// Recursively expand sub-issues that themselves have sub-issues
	for _, id := range subIssueIDs {
		grandchildIDs, err := c.detectSubIssues(ctx, id)
		if err != nil {
			return err
		}
		if len(grandchildIDs) > 0 {
			tk := taskKey("issue", id)
			if state, ok := c.taskStates[tk]; ok {
				state.Phase = PhaseNothingToDo
			}
			if err := c.expandParentIssue(ctx, id, grandchildIDs); err != nil {
				return err
			}
		}
	}

	return nil
}

// fetchSubIssueDetails fetches issue details for IDs not already cached.
// Note: on partial failure, successfully fetched issues remain in
// issueDetails/issueDetailsByNumber. This is harmless since no tasks
// are queued for them when the caller returns an error.
func (c *Controller) fetchSubIssueDetails(ctx context.Context, issueIDs []string) error {
	for _, id := range issueIDs {
		if c.issueDetailsByNumber[id] != nil {
			continue
		}

		cmd := c.execCommand(ctx, "gh", "issue", "view", id,
			"--repo", c.config.Repository,
			"--json", "number,title,body,labels",
		)
		cmd.Env = c.envWithGitHubToken()

		output, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("failed to fetch issue #%s: %w", id, err)
		}

		var issue issueDetail
		if err := json.Unmarshal(output, &issue); err != nil {
			return fmt.Errorf("failed to parse issue #%s: %w", id, err)
		}

		c.issueDetails = append(c.issueDetails, issue)
		issueNumStr := strconv.Itoa(issue.Number)
		c.issueDetailsByNumber[issueNumStr] = &c.issueDetails[len(c.issueDetails)-1]
	}
	return nil
}

// insertAfterTask inserts queue items after the task with the given ID.
func (c *Controller) insertAfterTask(afterID string, items []TaskQueueItem) {
	if len(items) == 0 {
		return
	}

	// Find position of the afterID task
	insertIdx := -1
	for i, item := range c.taskQueue {
		if item.ID == afterID {
			insertIdx = i + 1
			break
		}
	}

	if insertIdx == -1 {
		// Task not found — append to end
		c.taskQueue = append(c.taskQueue, items...)
		return
	}

	// Insert items at the position after the parent
	newQueue := make([]TaskQueueItem, 0, len(c.taskQueue)+len(items))
	newQueue = append(newQueue, c.taskQueue[:insertIdx]...)
	newQueue = append(newQueue, items...)
	newQueue = append(newQueue, c.taskQueue[insertIdx:]...)
	c.taskQueue = newQueue
}

// rebuildDependencyGraphWithSubIssues rebuilds the dependency graph including sub-issues.
// Unlike a full queue reorder, this only reorders sub-issues within their block
// so that they appear before non-dependent siblings.
func (c *Controller) rebuildDependencyGraphWithSubIssues(subIssueIDs []string) {
	// Build batch IDs from the full task queue
	batchIDs := make(map[string]bool)
	for _, item := range c.taskQueue {
		batchIDs[item.ID] = true
	}

	// Parse dependencies for newly added sub-issues
	subSet := make(map[string]bool, len(subIssueIDs))
	for _, id := range subIssueIDs {
		subSet[id] = true
	}
	for i := range c.issueDetails {
		id := strconv.Itoa(c.issueDetails[i].Number)
		if subSet[id] {
			c.issueDetails[i].DependsOn = parseDependencies(c.issueDetails[i].Body)
		}
	}

	// Rebuild the full dependency graph
	c.depGraph = NewDependencyGraph(c.issueDetails, batchIDs)

	if brokenEdges := c.depGraph.BrokenEdges(); len(brokenEdges) > 0 {
		for _, edge := range brokenEdges {
			c.logWarning("Cycle detected in sub-issues: edge from #%s to #%s was removed", edge.ParentID, edge.ChildID)
		}
	}

	// Only reorder sub-issues within their block — not the entire queue
	if c.depGraph.HasDependencies() {
		c.reorderSubIssuesInQueue(subIssueIDs)
		c.logInfo("Sub-issues reordered within block after expansion: %v", subIssueIDs)
	}
}

// reorderSubIssuesInQueue reorders only the sub-issue block within the task queue
// according to their topological ordering, without affecting sibling positions.
// This ensures sub-issues are processed before non-dependent siblings.
func (c *Controller) reorderSubIssuesInQueue(subIssueIDs []string) {
	subSet := make(map[string]bool, len(subIssueIDs))
	for _, id := range subIssueIDs {
		subSet[id] = true
	}

	// Get the topological order filtered to just sub-issues
	sortedAll := c.depGraph.SortedIssueIDs()
	var sortedSubs []string
	for _, id := range sortedAll {
		if subSet[id] {
			sortedSubs = append(sortedSubs, id)
		}
	}

	// Find positions of sub-issues in the queue
	var positions []int
	for i, item := range c.taskQueue {
		if subSet[item.ID] {
			positions = append(positions, i)
		}
	}

	// Replace sub-issues at their existing positions in sorted order
	for i, pos := range positions {
		if i < len(sortedSubs) {
			c.taskQueue[pos] = TaskQueueItem{Type: "issue", ID: sortedSubs[i]}
		}
	}
}

// postParentStatusComment posts a status comment on a parent issue.
func (c *Controller) postParentStatusComment(ctx context.Context, parentID string, subIssueIDs []string, event string) {
	var body string
	switch event {
	case "expanded":
		var lines []string
		lines = append(lines, fmt.Sprintf("**Parent expanded** — %d sub-issues queued for processing:\n", len(subIssueIDs)))
		for _, id := range subIssueIDs {
			lines = append(lines, fmt.Sprintf("- #%s", id))
		}
		body = strings.Join(lines, "\n")

	case "completed":
		var lines []string
		lines = append(lines, fmt.Sprintf("**Parent completed** — %d sub-issues processed:\n", len(subIssueIDs)))
		for _, id := range subIssueIDs {
			tk := taskKey("issue", id)
			state := c.taskStates[tk]
			phase := "UNKNOWN"
			pr := ""
			if state != nil {
				phase = string(state.Phase)
				if state.PRNumber != "" {
					pr = fmt.Sprintf(" (PR #%s)", state.PRNumber)
				}
			}
			lines = append(lines, fmt.Sprintf("- #%s: %s%s", id, phase, pr))
		}
		body = strings.Join(lines, "\n")

	default:
		return
	}

	// Post the comment using the standard issue comment mechanism.
	// Temporarily set activeTask to the parent ID for postIssueComment.
	savedActive := c.activeTask
	c.activeTask = parentID
	defer func() { c.activeTask = savedActive }()
	c.postIssueComment(ctx, body)
}
