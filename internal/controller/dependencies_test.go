package controller

import (
	"reflect"
	"sort"
	"testing"
)

func TestParseDependencies(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected []string
	}{
		{
			name:     "depends on",
			body:     "This issue depends on #123",
			expected: []string{"123"},
		},
		{
			name:     "blocked by",
			body:     "Blocked by #456",
			expected: []string{"456"},
		},
		{
			name:     "after",
			body:     "Should be done after #789",
			expected: []string{"789"},
		},
		{
			name:     "requires",
			body:     "This requires #101",
			expected: []string{"101"},
		},
		{
			name:     "multiple dependencies",
			body:     "Depends on #100, blocked by #200, and requires #300",
			expected: []string{"100", "200", "300"},
		},
		{
			name:     "case insensitive",
			body:     "DEPENDS ON #111, Blocked By #222",
			expected: []string{"111", "222"},
		},
		{
			name:     "with extra whitespace",
			body:     "depends  on  #333",
			expected: []string{"333"},
		},
		{
			name:     "no dependencies",
			body:     "This is just a regular issue body",
			expected: nil,
		},
		{
			name:     "empty body",
			body:     "",
			expected: nil,
		},
		{
			name:     "deduplicated",
			body:     "Depends on #100, also depends on #100",
			expected: []string{"100"},
		},
		{
			name:     "mixed with other hashtags",
			body:     "Depends on #123. Related to #456 (not a dependency). Blocked by #789.",
			expected: []string{"123", "789"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseDependencies(tt.body)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("parseDependencies(%q) = %v, want %v", tt.body, result, tt.expected)
			}
		})
	}
}

func TestNewDependencyGraph_Simple(t *testing.T) {
	// Issue 102 depends on 101
	issues := []issueDetail{
		{Number: 101, Title: "Parent", Body: "No dependencies"},
		{Number: 102, Title: "Child", Body: "Depends on #101"},
	}
	batch := map[string]bool{"101": true, "102": true}

	g := NewDependencyGraph(issues, batch)

	// Check parents
	if parents := g.ParentsOf("102"); len(parents) != 1 || parents[0] != "101" {
		t.Errorf("ParentsOf(102) = %v, want [101]", parents)
	}
	if parents := g.ParentsOf("101"); len(parents) != 0 {
		t.Errorf("ParentsOf(101) = %v, want []", parents)
	}

	// Check children
	if children := g.ChildrenOf("101"); len(children) != 1 || children[0] != "102" {
		t.Errorf("ChildrenOf(101) = %v, want [102]", children)
	}

	// Check sorted order: 101 should come before 102
	sorted := g.SortedIssueIDs()
	if len(sorted) != 2 || sorted[0] != "101" || sorted[1] != "102" {
		t.Errorf("SortedIssueIDs() = %v, want [101, 102]", sorted)
	}
}

func TestNewDependencyGraph_MultiParentChaining(t *testing.T) {
	// Issue 104 depends on both 101 and 102
	// Should chain: 101 → 102 → 104 (sorted numerically)
	issues := []issueDetail{
		{Number: 101, Title: "First", Body: "No deps"},
		{Number: 102, Title: "Second", Body: "No deps"},
		{Number: 104, Title: "Child", Body: "Depends on #102, depends on #101"},
	}
	batch := map[string]bool{"101": true, "102": true, "104": true}

	g := NewDependencyGraph(issues, batch)

	// 102 should have implicit dependency on 101 (multi-parent chaining)
	parents102 := g.ParentsOf("102")
	if len(parents102) != 1 || parents102[0] != "101" {
		t.Errorf("ParentsOf(102) = %v, want [101] (implicit chain)", parents102)
	}

	// 104 should only depend on 102 (the last in the chain)
	parents104 := g.ParentsOf("104")
	if len(parents104) != 1 || parents104[0] != "102" {
		t.Errorf("ParentsOf(104) = %v, want [102]", parents104)
	}

	// Sorted order should be 101 → 102 → 104
	sorted := g.SortedIssueIDs()
	if len(sorted) != 3 {
		t.Fatalf("SortedIssueIDs() has %d elements, want 3", len(sorted))
	}
	if sorted[0] != "101" || sorted[1] != "102" || sorted[2] != "104" {
		t.Errorf("SortedIssueIDs() = %v, want [101, 102, 104]", sorted)
	}
}

func TestNewDependencyGraph_ExternalDependenciesIgnored(t *testing.T) {
	// Issue 102 depends on 101 (in batch) and 999 (not in batch)
	issues := []issueDetail{
		{Number: 101, Title: "Parent", Body: "No deps"},
		{Number: 102, Title: "Child", Body: "Depends on #101, depends on #999"},
	}
	batch := map[string]bool{"101": true, "102": true}

	g := NewDependencyGraph(issues, batch)

	// Only 101 should be a parent (999 is external)
	parents := g.ParentsOf("102")
	if len(parents) != 1 || parents[0] != "101" {
		t.Errorf("ParentsOf(102) = %v, want [101]", parents)
	}

	// 999 should not be in the graph
	if children := g.ChildrenOf("999"); len(children) != 0 {
		t.Errorf("ChildrenOf(999) = %v, want [] (external issue)", children)
	}
}

func TestNewDependencyGraph_NoDependencies(t *testing.T) {
	issues := []issueDetail{
		{Number: 101, Title: "First", Body: "No deps"},
		{Number: 102, Title: "Second", Body: "Also no deps"},
	}
	batch := map[string]bool{"101": true, "102": true}

	g := NewDependencyGraph(issues, batch)

	if g.HasDependencies() {
		t.Error("HasDependencies() = true, want false")
	}

	// Both should be in sorted order (numerically)
	sorted := g.SortedIssueIDs()
	if len(sorted) != 2 || sorted[0] != "101" || sorted[1] != "102" {
		t.Errorf("SortedIssueIDs() = %v, want [101, 102]", sorted)
	}
}

func TestNewDependencyGraph_CycleDetection(t *testing.T) {
	// Create a cycle: 101 → 102 → 103 → 101
	issues := []issueDetail{
		{Number: 101, Title: "A", Body: "Depends on #103"},
		{Number: 102, Title: "B", Body: "Depends on #101"},
		{Number: 103, Title: "C", Body: "Depends on #102"},
	}
	batch := map[string]bool{"101": true, "102": true, "103": true}

	g := NewDependencyGraph(issues, batch)

	// Should have broken at least one edge
	if len(g.BrokenEdges()) == 0 {
		t.Error("BrokenEdges() is empty, expected at least one edge to be broken")
	}

	// Should still produce a valid topological sort
	sorted := g.SortedIssueIDs()
	if len(sorted) != 3 {
		t.Errorf("SortedIssueIDs() has %d elements, want 3", len(sorted))
	}

	// Verify no duplicates
	seen := make(map[string]bool)
	for _, id := range sorted {
		if seen[id] {
			t.Errorf("SortedIssueIDs() contains duplicate: %s", id)
		}
		seen[id] = true
	}
}

func TestNewDependencyGraph_SelfDependencyIgnored(t *testing.T) {
	// Issue 101 "depends on" itself
	issues := []issueDetail{
		{Number: 101, Title: "Self", Body: "Depends on #101"},
	}
	batch := map[string]bool{"101": true}

	g := NewDependencyGraph(issues, batch)

	// Should ignore self-dependency
	if parents := g.ParentsOf("101"); len(parents) != 0 {
		t.Errorf("ParentsOf(101) = %v, want [] (self-dep ignored)", parents)
	}
}

func TestNewDependencyGraph_DiamondDependency(t *testing.T) {
	// Diamond: 101 → 102, 101 → 103, 102 → 104, 103 → 104
	// Issue 104 depends on both 102 and 103
	issues := []issueDetail{
		{Number: 101, Title: "Root", Body: "No deps"},
		{Number: 102, Title: "Left", Body: "Depends on #101"},
		{Number: 103, Title: "Right", Body: "Depends on #101"},
		{Number: 104, Title: "Bottom", Body: "Depends on #102, depends on #103"},
	}
	batch := map[string]bool{"101": true, "102": true, "103": true, "104": true}

	g := NewDependencyGraph(issues, batch)

	// Sorted order should have 101 first, then 102 and 103, then 104
	sorted := g.SortedIssueIDs()
	if len(sorted) != 4 {
		t.Fatalf("SortedIssueIDs() has %d elements, want 4", len(sorted))
	}

	// 101 must be first
	if sorted[0] != "101" {
		t.Errorf("SortedIssueIDs()[0] = %s, want 101", sorted[0])
	}

	// 104 must be last
	if sorted[3] != "104" {
		t.Errorf("SortedIssueIDs()[3] = %s, want 104", sorted[3])
	}

	// 102 and 103 should be in the middle (in some order)
	middle := []string{sorted[1], sorted[2]}
	sort.Strings(middle)
	if middle[0] != "102" || middle[1] != "103" {
		t.Errorf("Middle elements = %v, want [102, 103]", middle)
	}
}

func TestParseIssueNumberFromBranch(t *testing.T) {
	tests := []struct {
		branch   string
		expected string
	}{
		{"agentium/issue-123-feature", "123"},
		{"agentium/issue-456-bug-fix", "456"},
		{"origin/agentium/issue-789-test", "789"},
		{"feature/something", ""},
		{"agentium/other-123", ""},
		{"main", ""},
		{"", ""},
		{"agentium/issue-abc-invalid", ""},
		{"agentium/issue--empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.branch, func(t *testing.T) {
			result := parseIssueNumberFromBranch(tt.branch)
			if result != tt.expected {
				t.Errorf("parseIssueNumberFromBranch(%q) = %q, want %q", tt.branch, result, tt.expected)
			}
		})
	}
}

func TestDependencyGraph_HasDependencies(t *testing.T) {
	// With dependencies
	issuesWithDeps := []issueDetail{
		{Number: 101, Body: ""},
		{Number: 102, Body: "Depends on #101"},
	}
	batch := map[string]bool{"101": true, "102": true}
	g := NewDependencyGraph(issuesWithDeps, batch)
	if !g.HasDependencies() {
		t.Error("HasDependencies() = false, want true")
	}

	// Without dependencies
	issuesNoDeps := []issueDetail{
		{Number: 101, Body: ""},
		{Number: 102, Body: "No deps here"},
	}
	g2 := NewDependencyGraph(issuesNoDeps, batch)
	if g2.HasDependencies() {
		t.Error("HasDependencies() = true, want false")
	}
}

func TestDependencyGraph_ComplexChain(t *testing.T) {
	// Linear chain: 101 → 102 → 103 → 104
	issues := []issueDetail{
		{Number: 101, Body: ""},
		{Number: 102, Body: "Depends on #101"},
		{Number: 103, Body: "Depends on #102"},
		{Number: 104, Body: "Depends on #103"},
	}
	batch := map[string]bool{"101": true, "102": true, "103": true, "104": true}

	g := NewDependencyGraph(issues, batch)

	// Verify the chain
	sorted := g.SortedIssueIDs()
	expected := []string{"101", "102", "103", "104"}
	if !reflect.DeepEqual(sorted, expected) {
		t.Errorf("SortedIssueIDs() = %v, want %v", sorted, expected)
	}
}
