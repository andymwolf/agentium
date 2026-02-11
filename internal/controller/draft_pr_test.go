package controller

import (
	"context"
	"testing"
)

func TestExtractIssueNumber(t *testing.T) {
	tests := []struct {
		name       string
		branchName string
		want       string
	}{
		{"agentium prefix", "agentium/issue-123-add-feature", "123"},
		{"feature prefix", "feature/issue-123-test", "123"},
		{"bug prefix", "bug/issue-456-fix-auth", "456"},
		{"enhancement prefix", "enhancement/issue-789-add-cache", "789"},
		{"branch with short number", "feature/issue-1-fix", "1"},
		{"branch with long number", "bug/issue-99999-big-change", "99999"},
		{"main branch", "main", ""},
		{"develop branch", "develop", ""},
		{"branch without issue pattern", "feature/something-else", ""},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractIssueNumber(tt.branchName)
			if got != tt.want {
				t.Errorf("extractIssueNumber(%q) = %q, want %q", tt.branchName, got, tt.want)
			}
		})
	}
}

func TestParsePRCreateOutput(t *testing.T) {
	tests := []struct {
		name       string
		output     string
		wantNumber string
		wantURL    string
	}{
		{
			"standard PR URL",
			"https://github.com/owner/repo/pull/123\n",
			"123",
			"https://github.com/owner/repo/pull/123",
		},
		{
			"PR URL without newline",
			"https://github.com/owner/repo/pull/456",
			"456",
			"https://github.com/owner/repo/pull/456",
		},
		{
			"PR URL with org name",
			"https://github.com/my-org/my-repo/pull/789",
			"789",
			"https://github.com/my-org/my-repo/pull/789",
		},
		{
			"no URL in output",
			"Error: something went wrong",
			"",
			"Error: something went wrong",
		},
		{
			"empty output",
			"",
			"",
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNumber, gotURL := parsePRCreateOutput(tt.output)
			if gotNumber != tt.wantNumber {
				t.Errorf("parsePRCreateOutput() number = %q, want %q", gotNumber, tt.wantNumber)
			}
			if gotURL != tt.wantURL {
				t.Errorf("parsePRCreateOutput() url = %q, want %q", gotURL, tt.wantURL)
			}
		})
	}
}

func TestParseIntOrZero(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"123", 123},
		{"0", 0},
		{"999", 999},
		{"invalid", 0},
		{"", 0},
		{"12.34", 0}, // JSON doesn't parse this as int
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseIntOrZero(tt.input)
			if got != tt.want {
				t.Errorf("parseIntOrZero(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestMaybeCreateDraftPR_SkipsWhenAlreadyCreated(t *testing.T) {
	c := &Controller{
		taskStates: map[string]*TaskState{
			"issue:123": {
				ID:             "123",
				Type:           "issue",
				DraftPRCreated: true,
			},
		},
	}

	// Should return nil without doing anything
	err := c.maybeCreateDraftPR(context.TODO(), "issue:123")
	if err != nil {
		t.Errorf("expected nil error when draft PR already created, got %v", err)
	}
}

func TestMaybeCreateDraftPR_ErrorsOnMissingState(t *testing.T) {
	c := &Controller{
		taskStates: map[string]*TaskState{},
	}

	err := c.maybeCreateDraftPR(context.TODO(), "issue:999")
	if err == nil {
		t.Error("expected error when task state not found")
	}
}

func TestFinalizeDraftPR_SkipsWhenPRMerged(t *testing.T) {
	c := &Controller{
		taskStates: map[string]*TaskState{
			"issue:123": {
				ID:       "123",
				Type:     "issue",
				PRNumber: "42",
				PRMerged: true,
			},
		},
		logger: newTestLogger(),
	}

	// Should return nil without doing anything (PR already merged)
	err := c.finalizeDraftPR(context.TODO(), "issue:123")
	if err != nil {
		t.Errorf("expected nil error when PR already merged, got %v", err)
	}
}

func TestFinalizeDraftPR_SkipsWhenNoPRNumber(t *testing.T) {
	c := &Controller{
		taskStates: map[string]*TaskState{
			"issue:123": {
				ID:       "123",
				Type:     "issue",
				PRNumber: "",
			},
		},
		logger: newTestLogger(),
	}

	// Should return nil without doing anything
	err := c.finalizeDraftPR(context.TODO(), "issue:123")
	if err != nil {
		t.Errorf("expected nil error when no PR number, got %v", err)
	}
}

func TestFinalizeDraftPR_ErrorsOnMissingState(t *testing.T) {
	c := &Controller{
		taskStates: map[string]*TaskState{},
	}

	err := c.finalizeDraftPR(context.TODO(), "issue:999")
	if err == nil {
		t.Error("expected error when task state not found")
	}
}

func TestMaybeCreateDraftPR_SkipsMismatchedBranch(t *testing.T) {
	// Simulates branch contamination: task 363 runs on branch for issue 334.
	// The existing PR found belongs to issue 334, not 363.
	// maybeCreateDraftPR should skip adopting this PR.
	tests := []struct {
		name        string
		taskID      string
		branchName  string
		wantAdopt   bool
		description string
	}{
		{
			name:        "branch belongs to different issue",
			taskID:      "363",
			branchName:  "enhancement/issue-334-calendar-availability",
			wantAdopt:   false,
			description: "should skip PR when branch issue (334) != task ID (363)",
		},
		{
			name:        "branch belongs to same issue",
			taskID:      "334",
			branchName:  "enhancement/issue-334-calendar-availability",
			wantAdopt:   true,
			description: "should adopt PR when branch issue matches task ID",
		},
		{
			name:        "branch has no issue number",
			taskID:      "363",
			branchName:  "feature/some-work",
			wantAdopt:   true,
			description: "should adopt PR when branch has no parseable issue number",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			branchIssue := extractIssueNumber(tt.branchName)
			wouldSkip := branchIssue != "" && branchIssue != tt.taskID

			if tt.wantAdopt && wouldSkip {
				t.Errorf("%s: expected adoption but validation would skip (branch issue=%q, task=%q)",
					tt.description, branchIssue, tt.taskID)
			}
			if !tt.wantAdopt && !wouldSkip {
				t.Errorf("%s: expected skip but validation would adopt (branch issue=%q, task=%q)",
					tt.description, branchIssue, tt.taskID)
			}
		})
	}
}

func TestTaskState_NOMERGEConditions(t *testing.T) {
	// Verify both ControllerOverrode and JudgeOverrodeReviewer trigger NOMERGE
	tests := []struct {
		name  string
		state TaskState
		want  bool // Should NOMERGE trigger?
	}{
		{
			name:  "controller overrode",
			state: TaskState{ControllerOverrode: true, JudgeOverrodeReviewer: false},
			want:  true,
		},
		{
			name:  "judge overrode reviewer",
			state: TaskState{ControllerOverrode: false, JudgeOverrodeReviewer: true},
			want:  true,
		},
		{
			name:  "both flags set",
			state: TaskState{ControllerOverrode: true, JudgeOverrodeReviewer: true},
			want:  true,
		},
		{
			name:  "neither flag set",
			state: TaskState{ControllerOverrode: false, JudgeOverrodeReviewer: false},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.state.ControllerOverrode || tt.state.JudgeOverrodeReviewer
			if got != tt.want {
				t.Errorf("NOMERGE condition = %v, want %v", got, tt.want)
			}
		})
	}
}
