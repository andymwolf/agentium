package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

func TestBranchPrefixForLabels(t *testing.T) {
	tests := []struct {
		name   string
		labels []issueLabel
		want   string
	}{
		{
			name:   "no labels - default to feature",
			labels: nil,
			want:   "feature",
		},
		{
			name:   "empty labels - default to feature",
			labels: []issueLabel{},
			want:   "feature",
		},
		{
			name:   "bug label",
			labels: []issueLabel{{Name: "bug"}},
			want:   "bug",
		},
		{
			name:   "enhancement label",
			labels: []issueLabel{{Name: "enhancement"}},
			want:   "enhancement",
		},
		{
			name:   "multiple labels - use first",
			labels: []issueLabel{{Name: "bug"}, {Name: "urgent"}},
			want:   "bug",
		},
		{
			name:   "label with space - sanitized",
			labels: []issueLabel{{Name: "good first issue"}},
			want:   "good-first-issue",
		},
		{
			name:   "uppercase label - lowercased",
			labels: []issueLabel{{Name: "Feature"}},
			want:   "feature",
		},
		{
			name:   "mixed case with space",
			labels: []issueLabel{{Name: "Help Wanted"}},
			want:   "help-wanted",
		},
		{
			name:   "label with colon - sanitized",
			labels: []issueLabel{{Name: "type: bug"}},
			want:   "type-bug",
		},
		{
			name:   "label with question mark - sanitized",
			labels: []issueLabel{{Name: "priority?high"}},
			want:   "priority-high",
		},
		{
			name:   "label with slash - sanitized",
			labels: []issueLabel{{Name: "ui/ux"}},
			want:   "ui-ux",
		},
		{
			name:   "label with multiple special chars - sanitized",
			labels: []issueLabel{{Name: "type: bug [critical]"}},
			want:   "type-bug-critical",
		},
		{
			name:   "label with consecutive special chars - collapsed",
			labels: []issueLabel{{Name: "type::bug"}},
			want:   "type-bug",
		},
		{
			name:   "label starting with special char - trimmed",
			labels: []issueLabel{{Name: ":bug"}},
			want:   "bug",
		},
		{
			name:   "label ending with special char - trimmed",
			labels: []issueLabel{{Name: "bug:"}},
			want:   "bug",
		},
		{
			name:   "label that becomes empty after sanitization - default to feature",
			labels: []issueLabel{{Name: ":::"}},
			want:   "feature",
		},
		{
			name:   "label with numbers",
			labels: []issueLabel{{Name: "priority-1"}},
			want:   "priority-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := branchPrefixForLabels(tt.labels)
			if got != tt.want {
				t.Errorf("branchPrefixForLabels() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSanitizeBranchPrefix(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"bug", "bug"},
		{"Bug", "bug"},
		{"type: bug", "type-bug"},
		{"priority?high", "priority-high"},
		{"ui/ux", "ui-ux"},
		{"good first issue", "good-first-issue"},
		{"type::bug", "type-bug"},
		{":bug", "bug"},
		{"bug:", "bug"},
		{":::", ""},
		{"a~b^c", "a-b-c"},
		{"test*case", "test-case"},
		{"feature[1]", "feature-1"},
		{"path\\name", "path-name"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeBranchPrefix(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeBranchPrefix(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatExternalComments(t *testing.T) {
	tests := []struct {
		name     string
		comments []issueComment
		want     string
	}{
		{
			name:     "no comments",
			comments: nil,
			want:     "",
		},
		{
			name: "single external comment",
			comments: []issueComment{
				{
					Author:    issueCommentAuthor{Login: "alice"},
					Body:      "This approach looks wrong.",
					CreatedAt: "2025-01-15T10:30:00Z",
				},
			},
			want: "**@alice** (2025-01-15):\n> This approach looks wrong.\n\n",
		},
		{
			name: "filters agentium comments",
			comments: []issueComment{
				{
					Author:    issueCommentAuthor{Login: "alice"},
					Body:      "Please fix the tests.",
					CreatedAt: "2025-01-15T10:30:00Z",
				},
				{
					Author:    issueCommentAuthor{Login: "agentium-bot"},
					Body:      "Phase complete.\n\n<!-- agentium:gcp:agentium-abc123 -->",
					CreatedAt: "2025-01-15T11:00:00Z",
				},
			},
			want: "**@alice** (2025-01-15):\n> Please fix the tests.\n\n",
		},
		{
			name: "all agentium comments returns empty",
			comments: []issueComment{
				{
					Author:    issueCommentAuthor{Login: "agentium-bot"},
					Body:      "Status update.\n\n<!-- agentium:gcp:agentium-abc123 -->",
					CreatedAt: "2025-01-15T11:00:00Z",
				},
			},
			want: "",
		},
		{
			name: "multiline comment body",
			comments: []issueComment{
				{
					Author:    issueCommentAuthor{Login: "bob"},
					Body:      "Line one.\nLine two.\nLine three.",
					CreatedAt: "2025-02-01T08:00:00Z",
				},
			},
			want: "**@bob** (2025-02-01):\n> Line one.\n> Line two.\n> Line three.\n\n",
		},
		{
			name: "short createdAt preserved as-is",
			comments: []issueComment{
				{
					Author:    issueCommentAuthor{Login: "carol"},
					Body:      "Short date.",
					CreatedAt: "2025-03",
				},
			},
			want: "**@carol** (2025-03):\n> Short date.\n\n",
		},
		{
			name: "multiple external comments in order",
			comments: []issueComment{
				{
					Author:    issueCommentAuthor{Login: "alice"},
					Body:      "First comment.",
					CreatedAt: "2025-01-10T09:00:00Z",
				},
				{
					Author:    issueCommentAuthor{Login: "bob"},
					Body:      "Second comment.",
					CreatedAt: "2025-01-11T10:00:00Z",
				},
			},
			want: "**@alice** (2025-01-10):\n> First comment.\n\n**@bob** (2025-01-11):\n> Second comment.\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatExternalComments(tt.comments)
			if got != tt.want {
				t.Errorf("formatExternalComments() =\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}

func TestFetchIssueDetails_IncludesState(t *testing.T) {
	// Verify that fetchIssueDetails populates the State field from gh output
	issue := issueDetail{
		Number: 42,
		Title:  "Open issue",
		Body:   "body",
		State:  "OPEN",
		Labels: []issueLabel{{Name: "bug"}},
	}
	closedIssue := issueDetail{
		Number: 43,
		Title:  "Closed issue",
		Body:   "done",
		State:  "CLOSED",
		Labels: []issueLabel{{Name: "bug"}},
	}

	responses := map[string]issueDetail{
		"42": issue,
		"43": closedIssue,
	}

	c := &Controller{
		config: SessionConfig{
			Repository: "org/repo",
			Tasks:      []string{"42", "43"},
		},
		logger: newTestLogger(),
		cmdRunner: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			// Find the issue number from args (it follows "view")
			var issueNum string
			for i, arg := range args {
				if arg == "view" && i+1 < len(args) {
					issueNum = args[i+1]
					break
				}
			}
			resp, ok := responses[issueNum]
			if !ok {
				return exec.CommandContext(ctx, "false")
			}
			data, _ := json.Marshal(resp)
			return exec.CommandContext(ctx, "echo", string(data))
		},
	}

	issues := c.fetchIssueDetails(context.Background())

	if len(issues) != 2 {
		t.Fatalf("got %d issues, want 2", len(issues))
	}

	for _, iss := range issues {
		want := responses[strconv.Itoa(iss.Number)]
		if iss.State != want.State {
			t.Errorf("issue #%d State = %q, want %q", iss.Number, iss.State, want.State)
		}
	}
}

func TestFilterClosedIssues(t *testing.T) {
	// Simulate what initSession does after fetchIssueDetails:
	// closed issues should be removed from issueDetails, taskStates, and taskQueue.
	c := &Controller{
		config: SessionConfig{
			Repository: "org/repo",
			Tasks:      []string{"10", "11", "12"},
		},
		taskStates: map[string]*TaskState{
			"issue:10": {ID: "10", Type: "issue", Phase: PhaseImplement},
			"issue:11": {ID: "11", Type: "issue", Phase: PhaseImplement},
			"issue:12": {ID: "12", Type: "issue", Phase: PhaseImplement},
		},
		taskQueue: []TaskQueueItem{
			{Type: "issue", ID: "10"},
			{Type: "issue", ID: "11"},
			{Type: "issue", ID: "12"},
		},
		issueDetails: []issueDetail{
			{Number: 10, Title: "Open", State: "OPEN"},
			{Number: 11, Title: "Closed", State: "CLOSED"},
			{Number: 12, Title: "Also open", State: "OPEN"},
		},
	}

	// Apply the same filtering logic as initSession
	var openIssues []issueDetail
	for _, issue := range c.issueDetails {
		id := strconv.Itoa(issue.Number)
		if strings.EqualFold(issue.State, "CLOSED") {
			delete(c.taskStates, taskKey("issue", id))
			continue
		}
		openIssues = append(openIssues, issue)
	}
	c.issueDetails = openIssues

	c.issueDetailsByNumber = make(map[string]*issueDetail, len(c.issueDetails))
	for i := range c.issueDetails {
		c.issueDetailsByNumber[fmt.Sprintf("%d", c.issueDetails[i].Number)] = &c.issueDetails[i]
	}

	var filteredQueue []TaskQueueItem
	for _, item := range c.taskQueue {
		if _, exists := c.taskStates[taskKey(item.Type, item.ID)]; exists {
			filteredQueue = append(filteredQueue, item)
		}
	}
	c.taskQueue = filteredQueue

	// Verify: only open issues remain
	if len(c.issueDetails) != 2 {
		t.Fatalf("issueDetails length = %d, want 2", len(c.issueDetails))
	}
	for _, iss := range c.issueDetails {
		if iss.Number == 11 {
			t.Errorf("closed issue #11 should have been filtered out")
		}
	}

	// Verify task states
	if _, ok := c.taskStates["issue:11"]; ok {
		t.Error("taskStates should not contain closed issue:11")
	}
	if _, ok := c.taskStates["issue:10"]; !ok {
		t.Error("taskStates should contain open issue:10")
	}
	if _, ok := c.taskStates["issue:12"]; !ok {
		t.Error("taskStates should contain open issue:12")
	}

	// Verify task queue
	if len(c.taskQueue) != 2 {
		t.Fatalf("taskQueue length = %d, want 2", len(c.taskQueue))
	}
	wantIDs := []string{"10", "12"}
	for i, want := range wantIDs {
		if c.taskQueue[i].ID != want {
			t.Errorf("taskQueue[%d].ID = %q, want %q", i, c.taskQueue[i].ID, want)
		}
	}

	// Verify issueDetailsByNumber
	if _, ok := c.issueDetailsByNumber["11"]; ok {
		t.Error("issueDetailsByNumber should not contain closed issue #11")
	}
	if _, ok := c.issueDetailsByNumber["10"]; !ok {
		t.Error("issueDetailsByNumber should contain open issue #10")
	}
}

func TestFilterClosedIssues_AllClosed(t *testing.T) {
	c := &Controller{
		config: SessionConfig{
			Repository: "org/repo",
			Tasks:      []string{"1"},
		},
		taskStates: map[string]*TaskState{
			"issue:1": {ID: "1", Type: "issue", Phase: PhaseImplement},
		},
		taskQueue: []TaskQueueItem{
			{Type: "issue", ID: "1"},
		},
		issueDetails: []issueDetail{
			{Number: 1, Title: "Done", State: "CLOSED"},
		},
	}

	// Apply filtering
	var openIssues []issueDetail
	for _, issue := range c.issueDetails {
		id := strconv.Itoa(issue.Number)
		if strings.EqualFold(issue.State, "CLOSED") {
			delete(c.taskStates, taskKey("issue", id))
			continue
		}
		openIssues = append(openIssues, issue)
	}
	c.issueDetails = openIssues

	var filteredQueue []TaskQueueItem
	for _, item := range c.taskQueue {
		if _, exists := c.taskStates[taskKey(item.Type, item.ID)]; exists {
			filteredQueue = append(filteredQueue, item)
		}
	}
	c.taskQueue = filteredQueue

	if len(c.issueDetails) != 0 {
		t.Errorf("issueDetails length = %d, want 0", len(c.issueDetails))
	}
	if len(c.taskStates) != 0 {
		t.Errorf("taskStates length = %d, want 0", len(c.taskStates))
	}
	if len(c.taskQueue) != 0 {
		t.Errorf("taskQueue length = %d, want 0", len(c.taskQueue))
	}
}
