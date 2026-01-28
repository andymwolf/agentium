package controller

import (
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

// parseIssueNumberFromBranch extracts an issue number from an agentium branch name.
// Branch format: agentium/issue-<number>-<description>
// Returns empty string if not an agentium branch or no issue number found.
func parseIssueNumberFromBranch(branch string) string {
	// Handle "origin/" prefix
	branch = strings.TrimPrefix(branch, "origin/")

	if !strings.HasPrefix(branch, "agentium/issue-") {
		return ""
	}

	// Extract the number after "agentium/issue-"
	rest := strings.TrimPrefix(branch, "agentium/issue-")
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
