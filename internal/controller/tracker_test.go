package controller

import (
	"context"
	"testing"
)

func TestIsTrackerIssue(t *testing.T) {
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
			got := isTrackerIssue(issue)
			if got != tt.want {
				t.Errorf("isTrackerIssue() = %v, want %v", got, tt.want)
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

func TestExpandTrackerIssue_NoSubIssues(t *testing.T) {
	c := &Controller{
		config: SessionConfig{
			Repository: "org/repo",
		},
		taskStates:       make(map[string]*TaskState),
		trackerSubIssues: make(map[string][]string),
		logger:           newTestLogger(),
	}

	tracker := &issueDetail{
		Number: 100,
		Title:  "Tracker issue",
		Body:   "This is just a regular description with no sub-issues.",
		Labels: []issueLabel{{Name: "tracker"}},
	}

	expanded, err := c.expandTrackerIssue(context.TODO(), "100", tracker)
	if err != nil {
		t.Fatalf("expandTrackerIssue() error = %v", err)
	}
	if expanded {
		t.Error("expandTrackerIssue() = true, want false for empty body")
	}
}
