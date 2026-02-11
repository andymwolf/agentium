package controller

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

func TestHasTrackerLabel(t *testing.T) {
	tests := []struct {
		name   string
		labels []issueLabel
		want   bool
	}{
		{
			name:   "tracker label present",
			labels: []issueLabel{{Name: "tracker"}, {Name: "bug"}},
			want:   true,
		},
		{
			name:   "tracker label uppercase",
			labels: []issueLabel{{Name: "Tracker"}},
			want:   true,
		},
		{
			name:   "tracker label mixed case",
			labels: []issueLabel{{Name: "TRACKER"}},
			want:   true,
		},
		{
			name:   "no tracker label",
			labels: []issueLabel{{Name: "bug"}, {Name: "enhancement"}},
			want:   false,
		},
		{
			name:   "empty labels",
			labels: nil,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := &issueDetail{Labels: tt.labels}
			got := hasTrackerLabel(issue)
			if got != tt.want {
				t.Errorf("hasTrackerLabel() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseSubIssues(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
	}{
		{
			name: "tasklist checkboxes",
			body: `## Sub-issues
- [ ] #10
- [x] #11
- [ ] #12`,
			want: []string{"10", "11", "12"},
		},
		{
			name: "full URLs",
			body: `## Tasks
- [ ] https://github.com/org/repo/issues/20
- [ ] https://github.com/org/repo/issues/21`,
			want: []string{"20", "21"},
		},
		{
			name: "plain list items",
			body: `- #30 Implement auth
- #31 Add tests`,
			want: []string{"30", "31"},
		},
		{
			name: "table cells",
			body: `| Issue | Status |
| #40 | TODO |
| #41 | DONE |`,
			want: []string{"40", "41"},
		},
		{
			name: "mixed formats dedup",
			body: `- [ ] #50
- #50 duplicate
- [ ] #51`,
			want: []string{"50", "51"},
		},
		{
			name: "empty body",
			body: "",
			want: nil,
		},
		{
			name: "prose references not matched",
			body: "See issue #99 for context. We discussed #100 in the meeting.",
			want: nil,
		},
		{
			name: "asterisk list items",
			body: `* [ ] #60
* #61 some task`,
			want: []string{"60", "61"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSubIssues(tt.body)
			if len(got) != len(tt.want) {
				t.Fatalf("parseSubIssues() got %v (len %d), want %v (len %d)", got, len(got), tt.want, len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseSubIssues()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestInsertAfterTask(t *testing.T) {
	tests := []struct {
		name    string
		queue   []TaskQueueItem
		afterID string
		items   []TaskQueueItem
		wantIDs []string
	}{
		{
			name: "insert after first",
			queue: []TaskQueueItem{
				{Type: "issue", ID: "1"},
				{Type: "issue", ID: "5"},
			},
			afterID: "1",
			items: []TaskQueueItem{
				{Type: "issue", ID: "2"},
				{Type: "issue", ID: "3"},
			},
			wantIDs: []string{"1", "2", "3", "5"},
		},
		{
			name: "insert after last",
			queue: []TaskQueueItem{
				{Type: "issue", ID: "1"},
				{Type: "issue", ID: "5"},
			},
			afterID: "5",
			items: []TaskQueueItem{
				{Type: "issue", ID: "6"},
			},
			wantIDs: []string{"1", "5", "6"},
		},
		{
			name: "task not found appends",
			queue: []TaskQueueItem{
				{Type: "issue", ID: "1"},
			},
			afterID: "99",
			items: []TaskQueueItem{
				{Type: "issue", ID: "2"},
			},
			wantIDs: []string{"1", "2"},
		},
		{
			name: "empty items no-op",
			queue: []TaskQueueItem{
				{Type: "issue", ID: "1"},
			},
			afterID: "1",
			items:   nil,
			wantIDs: []string{"1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Controller{
				taskQueue: make([]TaskQueueItem, len(tt.queue)),
			}
			copy(c.taskQueue, tt.queue)

			c.insertAfterTask(tt.afterID, tt.items)

			if len(c.taskQueue) != len(tt.wantIDs) {
				t.Fatalf("queue length = %d, want %d; queue: %v", len(c.taskQueue), len(tt.wantIDs), c.taskQueue)
			}
			for i, wantID := range tt.wantIDs {
				if c.taskQueue[i].ID != wantID {
					t.Errorf("queue[%d].ID = %q, want %q", i, c.taskQueue[i].ID, wantID)
				}
			}
		})
	}
}

func TestExpandParentIssue_WithSubIssues(t *testing.T) {
	// Pre-populate sub-issue details so fetchSubIssueDetails skips the gh call
	sub10 := issueDetail{
		Number: 10,
		Title:  "Sub-issue 10",
		Body:   "Implement feature A",
		Labels: []issueLabel{{Name: "enhancement"}},
	}
	sub11 := issueDetail{
		Number: 11,
		Title:  "Sub-issue 11",
		Body:   "Implement feature B\n\nDepends on #10",
		Labels: []issueLabel{{Name: "enhancement"}},
	}

	c := &Controller{
		config: SessionConfig{
			Repository: "org/repo",
		},
		taskStates: map[string]*TaskState{
			"issue:100": {ID: "100", Type: "issue", Phase: PhaseImplement},
		},
		taskQueue: []TaskQueueItem{
			{Type: "issue", ID: "100"},
			{Type: "issue", ID: "200"},
		},
		issueDetails: []issueDetail{sub10, sub11},
		issueDetailsByNumber: map[string]*issueDetail{
			"10": &sub10,
			"11": &sub11,
		},
		parentSubIssues: make(map[string][]string),
		logger:          newTestLogger(),
	}

	err := c.expandParentIssue(context.TODO(), "100", []string{"10", "11"})
	if err != nil {
		t.Fatalf("expandParentIssue() error = %v", err)
	}

	// Verify sub-issues were queued and dependency-ordered (#10 before #11).
	if len(c.taskQueue) != 4 {
		t.Fatalf("task queue length = %d, want 4; queue: %v", len(c.taskQueue), c.taskQueue)
	}
	// Sub-issues must appear in dependency order: #10 before #11
	idx10, idx11 := -1, -1
	queueIDs := make(map[string]bool)
	for i, item := range c.taskQueue {
		queueIDs[item.ID] = true
		if item.ID == "10" {
			idx10 = i
		}
		if item.ID == "11" {
			idx11 = i
		}
	}
	if idx10 == -1 || idx11 == -1 {
		t.Fatalf("sub-issues not found in queue: %v", c.taskQueue)
	}
	if idx10 >= idx11 {
		t.Errorf("#10 (idx %d) should appear before #11 (idx %d) in queue", idx10, idx11)
	}
	// Original tasks still present
	for _, id := range []string{"100", "200"} {
		if !queueIDs[id] {
			t.Errorf("task #%s missing from queue", id)
		}
	}

	// Verify task states created for sub-issues
	for _, id := range []string{"10", "11"} {
		tk := taskKey("issue", id)
		state, ok := c.taskStates[tk]
		if !ok {
			t.Errorf("task state not created for sub-issue #%s", id)
			continue
		}
		if state.Phase != PhaseImplement {
			t.Errorf("sub-issue #%s phase = %q, want %q", id, state.Phase, PhaseImplement)
		}
	}

	// Verify parent -> sub-issue mapping
	subIDs, ok := c.parentSubIssues["100"]
	if !ok {
		t.Fatal("parentSubIssues missing entry for parent #100")
	}
	if len(subIDs) != 2 || subIDs[0] != "10" || subIDs[1] != "11" {
		t.Errorf("parentSubIssues[100] = %v, want [10, 11]", subIDs)
	}
}

func TestSubIssueQueueOrdering_SubIssuesBeforeSiblings(t *testing.T) {
	// Parent=#100, subs=#200,#201 (201 depends on 200), sibling=#150.
	// After expansion, queue should be: [100, 200, 201, 150]
	// NOT [100, 150, 200, 201] as a full topological sort would produce.
	sub200 := issueDetail{
		Number: 200,
		Title:  "Sub-issue 200",
		Body:   "First sub-task",
		Labels: []issueLabel{{Name: "enhancement"}},
	}
	sub201 := issueDetail{
		Number: 201,
		Title:  "Sub-issue 201",
		Body:   "Second sub-task\n\nDepends on #200",
		Labels: []issueLabel{{Name: "enhancement"}},
	}

	c := &Controller{
		config: SessionConfig{
			Repository: "org/repo",
		},
		taskStates: map[string]*TaskState{
			"issue:100": {ID: "100", Type: "issue", Phase: PhaseImplement},
			"issue:150": {ID: "150", Type: "issue", Phase: PhaseImplement},
		},
		taskQueue: []TaskQueueItem{
			{Type: "issue", ID: "100"},
			{Type: "issue", ID: "150"},
		},
		issueDetails: []issueDetail{sub200, sub201},
		issueDetailsByNumber: map[string]*issueDetail{
			"200": &sub200,
			"201": &sub201,
		},
		parentSubIssues: make(map[string][]string),
		logger:          newTestLogger(),
	}

	err := c.expandParentIssue(context.TODO(), "100", []string{"200", "201"})
	if err != nil {
		t.Fatalf("expandParentIssue() error = %v", err)
	}

	// Expected order: [100, 200, 201, 150]
	wantOrder := []string{"100", "200", "201", "150"}
	if len(c.taskQueue) != len(wantOrder) {
		t.Fatalf("queue length = %d, want %d; queue: %v", len(c.taskQueue), len(wantOrder), queueIDs(c.taskQueue))
	}
	for i, want := range wantOrder {
		if c.taskQueue[i].ID != want {
			t.Errorf("queue[%d].ID = %q, want %q (full queue: %v)", i, c.taskQueue[i].ID, want, queueIDs(c.taskQueue))
		}
	}
}

func TestParseSubIssuesGraphQLResponse(t *testing.T) {
	tests := []struct {
		name     string
		jsonResp string
		wantIDs  []string
	}{
		{
			name: "mixed open and closed",
			jsonResp: `{
				"data": {
					"repository": {
						"issue": {
							"subIssues": {
								"nodes": [
									{"number": 10, "state": "OPEN"},
									{"number": 11, "state": "CLOSED"},
									{"number": 12, "state": "OPEN"}
								]
							}
						}
					}
				}
			}`,
			wantIDs: []string{"10", "12"},
		},
		{
			name: "all closed",
			jsonResp: `{
				"data": {
					"repository": {
						"issue": {
							"subIssues": {
								"nodes": [
									{"number": 10, "state": "CLOSED"},
									{"number": 11, "state": "CLOSED"}
								]
							}
						}
					}
				}
			}`,
			wantIDs: nil,
		},
		{
			name: "no sub-issues",
			jsonResp: `{
				"data": {
					"repository": {
						"issue": {
							"subIssues": {
								"nodes": []
							}
						}
					}
				}
			}`,
			wantIDs: nil,
		},
		{
			name: "all open",
			jsonResp: `{
				"data": {
					"repository": {
						"issue": {
							"subIssues": {
								"nodes": [
									{"number": 363, "state": "OPEN"},
									{"number": 364, "state": "OPEN"}
								]
							}
						}
					}
				}
			}`,
			wantIDs: []string{"363", "364"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resp subIssuesGraphQLResponse
			if err := json.Unmarshal([]byte(tt.jsonResp), &resp); err != nil {
				t.Fatalf("failed to parse test JSON: %v", err)
			}

			// Filter open sub-issues using the same logic as production code
			var ids []string
			for _, node := range resp.Data.Repository.Issue.SubIssues.Nodes {
				if strings.EqualFold(node.State, "OPEN") {
					ids = append(ids, strconv.Itoa(node.Number))
				}
			}

			if len(ids) != len(tt.wantIDs) {
				t.Fatalf("got %v (len %d), want %v (len %d)", ids, len(ids), tt.wantIDs, len(tt.wantIDs))
			}
			for i := range ids {
				if ids[i] != tt.wantIDs[i] {
					t.Errorf("[%d] = %q, want %q", i, ids[i], tt.wantIDs[i])
				}
			}
		})
	}
}

func TestParseRepoOwnerName(t *testing.T) {
	tests := []struct {
		name      string
		repo      string
		wantOwner string
		wantName  string
		wantErr   bool
	}{
		{
			name:      "simple owner/repo",
			repo:      "org/repo",
			wantOwner: "org",
			wantName:  "repo",
		},
		{
			name:      "with github.com prefix",
			repo:      "github.com/org/repo",
			wantOwner: "org",
			wantName:  "repo",
		},
		{
			name:      "with https prefix",
			repo:      "https://github.com/org/repo",
			wantOwner: "org",
			wantName:  "repo",
		},
		{
			name:      "with .git suffix",
			repo:      "org/repo.git",
			wantOwner: "org",
			wantName:  "repo",
		},
		{
			name:    "invalid single part",
			repo:    "just-a-name",
			wantErr: true,
		},
		{
			name:    "empty string",
			repo:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, name, err := parseRepoOwnerName(tt.repo)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseRepoOwnerName(%q) error = %v, wantErr %v", tt.repo, err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if owner != tt.wantOwner {
				t.Errorf("owner = %q, want %q", owner, tt.wantOwner)
			}
			if name != tt.wantName {
				t.Errorf("name = %q, want %q", name, tt.wantName)
			}
		})
	}
}

// queueIDs extracts IDs from a task queue for test output.
func queueIDs(queue []TaskQueueItem) []string {
	ids := make([]string, len(queue))
	for i, item := range queue {
		ids[i] = item.ID
	}
	return ids
}
