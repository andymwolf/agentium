package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// isTrackerIssue checks whether an issue has the "tracker" label (case-insensitive).
func isTrackerIssue(issue *issueDetail) bool {
	for _, label := range issue.Labels {
		if strings.EqualFold(label.Name, "tracker") {
			return true
		}
	}
	return false
}

// subIssuePatterns matches issue references in tracker bodies.
// Supports:
//   - GitHub tasklist: "- [ ] #123", "- [x] #124"
//   - Full URL: "- [ ] https://github.com/.../issues/125"
//   - Plain list: "- #123 description"
//   - Table cell: "| #123 |"
var subIssuePatterns = []*regexp.Regexp{
	// Tasklist with checkbox: - [ ] #123 or - [x] #123
	regexp.MustCompile(`(?m)^[-*]\s+\[[ xX]\]\s+#(\d+)`),
	// Tasklist with checkbox + full URL: - [ ] https://github.com/.../issues/123
	regexp.MustCompile(`(?m)^[-*]\s+\[[ xX]\]\s+https://github\.com/[^/]+/[^/]+/issues/(\d+)`),
	// Plain list item: - #123
	regexp.MustCompile(`(?m)^[-*]\s+#(\d+)`),
	// Full URL in plain list: - https://github.com/.../issues/123
	regexp.MustCompile(`(?m)^[-*]\s+https://github\.com/[^/]+/[^/]+/issues/(\d+)`),
	// Table cell: | #123 |
	regexp.MustCompile(`\|\s*#(\d+)\s*\|`),
}

// parseSubIssues extracts issue numbers from a tracker issue body.
// Returns deduplicated IDs in order of first appearance.
func parseSubIssues(body string) []string {
	seen := make(map[string]bool)
	var ids []string

	for _, pattern := range subIssuePatterns {
		matches := pattern.FindAllStringSubmatch(body, -1)
		for _, match := range matches {
			if len(match) >= 2 {
				id := match[1]
				if !seen[id] {
					seen[id] = true
					ids = append(ids, id)
				}
			}
		}
	}
	return ids
}

// expandTrackerIssue expands a tracker issue into sub-issues in the task queue.
// Returns true if sub-issues were found and expanded, false if the tracker had no sub-issues.
func (c *Controller) expandTrackerIssue(ctx context.Context, trackerID string, tracker *issueDetail) (bool, error) {
	subIssueIDs := parseSubIssues(tracker.Body)
	if len(subIssueIDs) == 0 {
		c.logInfo("Tracker #%s has no sub-issues", trackerID)
		return false, nil
	}

	c.logInfo("Tracker #%s: expanding %d sub-issues: %v", trackerID, len(subIssueIDs), subIssueIDs)

	// Fetch details for sub-issues not already in cache
	if err := c.fetchSubIssueDetails(ctx, subIssueIDs); err != nil {
		return false, fmt.Errorf("failed to fetch sub-issue details: %w", err)
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

	// Insert sub-issues into queue after the tracker
	c.insertAfterTask(trackerID, newItems)

	// Rebuild dependency graph with the expanded sub-issues
	c.rebuildDependencyGraphWithSubIssues(subIssueIDs)

	// Track tracker -> sub-issue mapping
	if c.trackerSubIssues == nil {
		c.trackerSubIssues = make(map[string][]string)
	}
	c.trackerSubIssues[trackerID] = subIssueIDs

	// Mark tracker as NOTHING_TO_DO
	trackerTK := taskKey("issue", trackerID)
	if state, ok := c.taskStates[trackerTK]; ok {
		state.Phase = PhaseNothingToDo
	}

	// Post expansion comment on tracker
	c.postTrackerStatusComment(ctx, trackerID, subIssueIDs, "expanded")

	return true, nil
}

// fetchSubIssueDetails fetches issue details for IDs not already cached.
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

	// Insert items at the position after the tracker
	newQueue := make([]TaskQueueItem, 0, len(c.taskQueue)+len(items))
	newQueue = append(newQueue, c.taskQueue[:insertIdx]...)
	newQueue = append(newQueue, items...)
	newQueue = append(newQueue, c.taskQueue[insertIdx:]...)
	c.taskQueue = newQueue
}

// rebuildDependencyGraphWithSubIssues rebuilds the dependency graph including sub-issues.
func (c *Controller) rebuildDependencyGraphWithSubIssues(subIssueIDs []string) {
	// Build batch IDs from the full task queue
	batchIDs := make(map[string]bool)
	for _, item := range c.taskQueue {
		batchIDs[item.ID] = true
	}

	// Parse dependencies for newly added sub-issues
	for i := range c.issueDetails {
		id := strconv.Itoa(c.issueDetails[i].Number)
		for _, subID := range subIssueIDs {
			if id == subID {
				deps := parseDependencies(c.issueDetails[i].Body)
				c.issueDetails[i].DependsOn = deps
				break
			}
		}
	}

	// Rebuild the full dependency graph
	c.depGraph = NewDependencyGraph(c.issueDetails, batchIDs)

	if brokenEdges := c.depGraph.BrokenEdges(); len(brokenEdges) > 0 {
		for _, edge := range brokenEdges {
			c.logWarning("Cycle detected in sub-issues: edge from #%s to #%s was removed", edge.ParentID, edge.ChildID)
		}
	}

	if c.depGraph.HasDependencies() {
		c.reorderTaskQueue(c.depGraph.SortedIssueIDs())
		c.logInfo("Task queue reordered after tracker expansion: %v", c.depGraph.SortedIssueIDs())
	}
}

// postTrackerStatusComment posts a status comment on a tracker issue.
func (c *Controller) postTrackerStatusComment(ctx context.Context, trackerID string, subIssueIDs []string, event string) {
	var body string
	switch event {
	case "expanded":
		var lines []string
		lines = append(lines, fmt.Sprintf("**Tracker expanded** — %d sub-issues queued for processing:\n", len(subIssueIDs)))
		for _, id := range subIssueIDs {
			lines = append(lines, fmt.Sprintf("- #%s", id))
		}
		body = strings.Join(lines, "\n")

	case "completed":
		var lines []string
		lines = append(lines, fmt.Sprintf("**Tracker completed** — %d sub-issues processed:\n", len(subIssueIDs)))
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

	// Post the comment using the standard issue comment mechanism
	// Temporarily set activeTask to the tracker ID for postIssueComment
	savedActive := c.activeTask
	c.activeTask = trackerID
	c.postIssueComment(ctx, body)
	c.activeTask = savedActive
}
