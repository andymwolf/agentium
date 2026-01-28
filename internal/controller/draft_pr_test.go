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
