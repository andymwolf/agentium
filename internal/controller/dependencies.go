package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// IssueDependency represents a parent-child relationship between two issues.
type IssueDependency struct {
	ChildID  string
	ParentID string
}

// DependencyGraph manages inter-issue dependencies for topological ordering.
type DependencyGraph struct {
	parents     map[string][]string // issue -> its parent issues
	children    map[string][]string // issue -> its child issues
	sortedOrder []string            // topologically sorted issue IDs
	brokenEdges []IssueDependency   // edges removed to break cycles
}

// dependencyPatterns matches common dependency phrases in issue bodies.
// Supports: "depends on #123", "blocked by #456", "after #789", "requires #101"
var dependencyPatterns = regexp.MustCompile(`(?i)(?:depends\s+on|blocked\s+by|after|requires)\s+#(\d+)`)

// parseDependencies extracts issue IDs from dependency phrases in a body text.
// Returns deduplicated issue IDs.
func parseDependencies(body string) []string {
	matches := dependencyPatterns.FindAllStringSubmatch(body, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var ids []string
	for _, match := range matches {
		if len(match) >= 2 {
			id := match[1]
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// NewDependencyGraph builds a dependency graph from issue details.
// Only issues within batchIDs are included; external dependencies are tracked but
// issues outside the batch are not nodes in the graph.
// Multi-parent dependencies are chained: if issue X depends on {A, B, C},
// the parents are sorted numerically and implicit edges are added so A→B→C→X.
func NewDependencyGraph(issues []issueDetail, batchIDs map[string]bool) *DependencyGraph {
	g := &DependencyGraph{
		parents:  make(map[string][]string),
		children: make(map[string][]string),
	}

	// Initialize nodes for all batch issues
	for _, issue := range issues {
		id := strconv.Itoa(issue.Number)
		if batchIDs[id] {
			if g.parents[id] == nil {
				g.parents[id] = []string{}
			}
			if g.children[id] == nil {
				g.children[id] = []string{}
			}
		}
	}

	// Parse dependencies and build edges
	for _, issue := range issues {
		childID := strconv.Itoa(issue.Number)
		if !batchIDs[childID] {
			continue
		}

		deps := parseDependencies(issue.Body)
		if len(deps) == 0 {
			continue
		}

		// Filter to only in-batch parents and sort numerically (lowest first)
		var inBatchParents []string
		for _, parentID := range deps {
			if batchIDs[parentID] && parentID != childID {
				inBatchParents = append(inBatchParents, parentID)
			}
		}

		if len(inBatchParents) == 0 {
			continue
		}

		// Sort parents numerically for deterministic chaining
		sort.Slice(inBatchParents, func(i, j int) bool {
			numI, _ := strconv.Atoi(inBatchParents[i])
			numJ, _ := strconv.Atoi(inBatchParents[j])
			return numI < numJ
		})

		// Chain multi-parent: A→B→C→child
		// Add implicit edges between consecutive parents
		for i := 0; i < len(inBatchParents)-1; i++ {
			g.addEdge(inBatchParents[i], inBatchParents[i+1])
		}

		// Connect last parent to child
		lastParent := inBatchParents[len(inBatchParents)-1]
		g.addEdge(lastParent, childID)
	}

	// Detect and break cycles, then sort
	g.detectAndBreakCycles()
	g.topologicalSort()

	return g
}

// addEdge adds a parent→child edge to the graph.
func (g *DependencyGraph) addEdge(parentID, childID string) {
	// Avoid duplicate edges
	for _, existing := range g.children[parentID] {
		if existing == childID {
			return
		}
	}
	g.children[parentID] = append(g.children[parentID], childID)
	g.parents[childID] = append(g.parents[childID], parentID)
}

// removeEdge removes a parent→child edge from the graph.
func (g *DependencyGraph) removeEdge(parentID, childID string) {
	// Remove from children list
	children := g.children[parentID]
	for i, c := range children {
		if c == childID {
			g.children[parentID] = append(children[:i], children[i+1:]...)
			break
		}
	}

	// Remove from parents list
	parents := g.parents[childID]
	for i, p := range parents {
		if p == parentID {
			g.parents[childID] = append(parents[:i], parents[i+1:]...)
			break
		}
	}
}

// detectAndBreakCycles finds cycles using DFS and removes edges to break them.
// When a cycle is detected, the edge to the highest-numbered child is removed.
// Returns the edges that were removed.
func (g *DependencyGraph) detectAndBreakCycles() []IssueDependency {
	const (
		white = 0 // unvisited
		gray  = 1 // in current DFS path
		black = 2 // finished
	)

	color := make(map[string]int)
	var backEdges []IssueDependency

	// Get all nodes sorted for deterministic traversal
	var nodes []string
	for node := range g.parents {
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(i, j int) bool {
		numI, _ := strconv.Atoi(nodes[i])
		numJ, _ := strconv.Atoi(nodes[j])
		return numI < numJ
	})

	var dfs func(node string)
	dfs = func(node string) {
		color[node] = gray

		// Sort children for deterministic traversal
		children := make([]string, len(g.children[node]))
		copy(children, g.children[node])
		sort.Slice(children, func(i, j int) bool {
			numI, _ := strconv.Atoi(children[i])
			numJ, _ := strconv.Atoi(children[j])
			return numI < numJ
		})

		for _, child := range children {
			switch color[child] {
			case white:
				dfs(child)
			case gray:
				// Back edge found - this creates a cycle
				backEdges = append(backEdges, IssueDependency{
					ParentID: node,
					ChildID:  child,
				})
			}
		}
		color[node] = black
	}

	for _, node := range nodes {
		if color[node] == white {
			dfs(node)
		}
	}

	// Remove back edges to break cycles
	for _, edge := range backEdges {
		g.removeEdge(edge.ParentID, edge.ChildID)
	}

	g.brokenEdges = backEdges
	return backEdges
}

// topologicalSort performs Kahn's algorithm to produce a topological ordering.
// Must be called after detectAndBreakCycles to ensure the graph is a DAG.
func (g *DependencyGraph) topologicalSort() {
	// Calculate in-degree for each node
	inDegree := make(map[string]int)
	for node := range g.parents {
		inDegree[node] = len(g.parents[node])
	}

	// Find all nodes with no incoming edges (in-degree = 0)
	var queue []string
	for node, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, node)
		}
	}

	// Sort queue for deterministic ordering (lower numbers first)
	sort.Slice(queue, func(i, j int) bool {
		numI, _ := strconv.Atoi(queue[i])
		numJ, _ := strconv.Atoi(queue[j])
		return numI < numJ
	})

	var sorted []string
	for len(queue) > 0 {
		// Pop from queue (already sorted)
		node := queue[0]
		queue = queue[1:]
		sorted = append(sorted, node)

		// For each child, decrease in-degree
		children := make([]string, len(g.children[node]))
		copy(children, g.children[node])
		sort.Slice(children, func(i, j int) bool {
			numI, _ := strconv.Atoi(children[i])
			numJ, _ := strconv.Atoi(children[j])
			return numI < numJ
		})

		for _, child := range children {
			inDegree[child]--
			if inDegree[child] == 0 {
				// Insert in sorted order
				inserted := false
				for i, q := range queue {
					numChild, _ := strconv.Atoi(child)
					numQ, _ := strconv.Atoi(q)
					if numChild < numQ {
						queue = append(queue[:i], append([]string{child}, queue[i:]...)...)
						inserted = true
						break
					}
				}
				if !inserted {
					queue = append(queue, child)
				}
			}
		}
	}

	g.sortedOrder = sorted
}

// SortedIssueIDs returns the topologically sorted list of issue IDs.
func (g *DependencyGraph) SortedIssueIDs() []string {
	return g.sortedOrder
}

// ParentsOf returns the parent issue IDs for a given issue.
func (g *DependencyGraph) ParentsOf(issueID string) []string {
	return g.parents[issueID]
}

// ChildrenOf returns the child issue IDs for a given issue.
func (g *DependencyGraph) ChildrenOf(issueID string) []string {
	return g.children[issueID]
}

// BrokenEdges returns the edges that were removed to break cycles.
func (g *DependencyGraph) BrokenEdges() []IssueDependency {
	return g.brokenEdges
}

// HasDependencies returns true if the graph has any dependencies.
func (g *DependencyGraph) HasDependencies() bool {
	for _, parents := range g.parents {
		if len(parents) > 0 {
			return true
		}
	}
	return false
}

// parseIssueNumberFromBranch extracts an issue number from a branch name.
// Branch format: <prefix>/issue-<number>-<description>
// Supports any prefix (feature, bug, enhancement, agentium, etc.).
// Returns empty string if the branch doesn't match the pattern or no issue number found.
func parseIssueNumberFromBranch(branch string) string {
	// Handle "origin/" prefix
	branch = strings.TrimPrefix(branch, "origin/")

	// Look for /issue-<number>- pattern anywhere in the branch name
	idx := strings.Index(branch, "/issue-")
	if idx == -1 {
		return ""
	}

	// Extract the number after "/issue-"
	rest := branch[idx+len("/issue-"):]
	parts := strings.SplitN(rest, "-", 2)
	if len(parts) == 0 {
		return ""
	}

	// Validate it's a number
	if _, err := strconv.Atoi(parts[0]); err != nil {
		return ""
	}

	return parts[0]
}

// buildDependencyGraph constructs an inter-issue dependency graph from issue bodies.
// It parses dependencies, detects and breaks cycles (with warnings), and reorders the task queue.
// This method is only called when there are multiple issues in the batch.
func (c *Controller) buildDependencyGraph() {
	if len(c.issueDetails) <= 1 {
		return
	}

	// Build set of batch issue IDs
	batchIDs := make(map[string]bool)
	for _, issue := range c.issueDetails {
		batchIDs[strconv.Itoa(issue.Number)] = true
	}

	// Parse dependencies and populate DependsOn field
	for i := range c.issueDetails {
		deps := parseDependencies(c.issueDetails[i].Body)
		c.issueDetails[i].DependsOn = deps
	}

	// Build the dependency graph
	c.depGraph = NewDependencyGraph(c.issueDetails, batchIDs)

	// Log any cycles that were broken
	if brokenEdges := c.depGraph.BrokenEdges(); len(brokenEdges) > 0 {
		for _, edge := range brokenEdges {
			c.logWarning("Cycle detected: edge from #%s to #%s was removed", edge.ParentID, edge.ChildID)
		}
	}

	// Reorder task queue if graph has dependencies
	if c.depGraph.HasDependencies() {
		c.reorderTaskQueue(c.depGraph.SortedIssueIDs())
		c.logInfo("Task queue reordered based on dependencies: %v", c.depGraph.SortedIssueIDs())
	}
}

// reorderTaskQueue reorders the task queue to match the topologically sorted issue order.
func (c *Controller) reorderTaskQueue(sortedIDs []string) {
	issueMap := make(map[string]TaskQueueItem)

	for _, item := range c.taskQueue {
		issueMap[item.ID] = item
	}

	// Rebuild queue in topological order
	newQueue := make([]TaskQueueItem, 0, len(c.taskQueue))

	for _, id := range sortedIDs {
		if item, ok := issueMap[id]; ok {
			newQueue = append(newQueue, item)
			delete(issueMap, id)
		}
	}

	// Append any remaining issues not in the sorted list (shouldn't happen, but be safe)
	for _, item := range issueMap {
		newQueue = append(newQueue, item)
	}

	c.taskQueue = newQueue
}

// resolveParentBranch determines the branch a child issue should be based on.
// Returns the parent's branch name if the child depends on a parent issue, or "" to use main.
// Returns an error if the child should be marked BLOCKED (e.g., parent failed or has no branch).
func (c *Controller) resolveParentBranch(ctx context.Context, childID string) (string, error) {
	if c.depGraph == nil {
		return "", nil
	}

	parents := c.depGraph.ParentsOf(childID)
	if len(parents) == 0 {
		return "", nil
	}

	// For chained dependencies, only the immediate parent matters
	// (multi-parent chaining already linearized the graph)
	parentID := parents[0]

	// Check if parent is in our batch
	parentTaskID := taskKey("issue", parentID)
	parentState, inBatch := c.taskStates[parentTaskID]

	if inBatch {
		// In-batch parent: check its completion state
		switch parentState.Phase {
		case PhaseComplete:
			// Parent completed successfully, find its branch
			existingWork := c.detectExistingWork(ctx, parentID)
			if existingWork != nil && existingWork.Branch != "" {
				c.logInfo("Issue #%s will branch from parent #%s's branch: %s", childID, parentID, existingWork.Branch)
				return existingWork.Branch, nil
			}
			// Parent complete but no branch found (maybe it was merged?)
			c.logInfo("Issue #%s: parent #%s complete but no branch found, using main", childID, parentID)
			return "", nil

		case PhaseNothingToDo:
			// Parent had nothing to do, no dependency effect
			c.logInfo("Issue #%s: parent #%s had nothing to do, using main", childID, parentID)
			return "", nil

		case PhaseBlocked:
			// Parent is blocked, child should also be blocked
			return "", fmt.Errorf("parent issue #%s is blocked", parentID)

		default:
			// Parent not yet complete, child should wait (block for now, controller will re-check)
			return "", fmt.Errorf("parent issue #%s not yet complete (phase: %s)", parentID, parentState.Phase)
		}
	}

	// External parent (not in batch): check for existing PR
	return c.resolveExternalParentBranch(ctx, parentID, childID)
}

// resolveExternalParentBranch resolves the branch for an external (not in batch) parent issue.
func (c *Controller) resolveExternalParentBranch(ctx context.Context, parentID, childID string) (string, error) {
	// Look for an existing PR for the external parent
	existingWork := c.detectExistingWork(ctx, parentID)
	if existingWork == nil {
		// No work found for external parent - child is blocked
		return "", fmt.Errorf("external parent issue #%s has no branch or PR", parentID)
	}

	if existingWork.PRNumber != "" {
		// External parent has a PR - check if it's merged
		merged, err := c.isPRMerged(ctx, existingWork.PRNumber)
		if err != nil {
			c.logWarning("Failed to check merge status of PR #%s: %v", existingWork.PRNumber, err)
			// Assume not merged, use the branch
			c.logInfo("Issue #%s will branch from external parent #%s's PR branch: %s", childID, parentID, existingWork.Branch)
			return existingWork.Branch, nil
		}

		if merged {
			// PR is merged, code is in main
			c.logInfo("Issue #%s: external parent #%s's PR merged, using main", childID, parentID)
			return "", nil
		}

		// PR is open, use its branch
		c.logInfo("Issue #%s will branch from external parent #%s's open PR branch: %s", childID, parentID, existingWork.Branch)
		return existingWork.Branch, nil
	}

	// External parent has a branch but no PR
	c.logInfo("Issue #%s will branch from external parent #%s's branch: %s", childID, parentID, existingWork.Branch)
	return existingWork.Branch, nil
}

// isPRMerged checks if a PR has been merged.
func (c *Controller) isPRMerged(ctx context.Context, prNumber string) (bool, error) {
	cmd := exec.CommandContext(ctx, "gh", "pr", "view", prNumber,
		"--repo", c.config.Repository,
		"--json", "state",
	)
	cmd.Dir = c.workDir
	cmd.Env = c.envWithGitHubToken()

	output, err := cmd.Output()
	if err != nil {
		return false, err
	}

	var result struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return false, err
	}

	return result.State == "MERGED", nil
}

// propagateBlocked marks all children of a blocked issue as BLOCKED via BFS.
func (c *Controller) propagateBlocked(issueID string) {
	if c.depGraph == nil {
		return
	}

	// BFS through children
	queue := []string{issueID}
	visited := make(map[string]bool)

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if visited[current] {
			continue
		}
		visited[current] = true

		children := c.depGraph.ChildrenOf(current)
		for _, childID := range children {
			taskID := taskKey("issue", childID)
			if state, ok := c.taskStates[taskID]; ok {
				if state.Phase != PhaseBlocked && state.Phase != PhaseComplete && state.Phase != PhaseNothingToDo {
					state.Phase = PhaseBlocked
					c.logInfo("Issue #%s marked BLOCKED (parent #%s blocked)", childID, current)
					queue = append(queue, childID)
				}
			}
		}
	}
}
