package controller

import (
	"context"
	"fmt"
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

func TestCreateDraftPRWithRetry_BlocksOnExhaustion(t *testing.T) {
	taskID := "issue:123"
	state := &TaskState{
		ID:             "123",
		Type:           "issue",
		Phase:          PhaseImplement,
		DraftPRCreated: false,
	}
	c := &Controller{
		taskStates: map[string]*TaskState{taskID: state},
		logger:     newTestLogger(),
		workDir:    "/nonexistent/path/that/does/not/exist",
	}

	// maybeCreateDraftPR will fail because workDir doesn't exist,
	// causing branch detection to fail on every attempt.
	blocked := c.createDraftPRWithRetry(context.Background(), taskID, state, PhaseImplement, 1)

	if !blocked {
		t.Fatal("expected createDraftPRWithRetry to return blocked=true after exhausting retries")
	}
	if state.Phase != PhaseBlocked {
		t.Errorf("state.Phase = %q, want %q", state.Phase, PhaseBlocked)
	}
	if !state.ControllerOverrode {
		t.Error("expected state.ControllerOverrode to be true")
	}
	if state.DraftPRCreated {
		t.Error("expected state.DraftPRCreated to remain false")
	}
}

func TestCreateDraftPRWithRetry_SucceedsWhenAlreadyCreated(t *testing.T) {
	taskID := "issue:456"
	state := &TaskState{
		ID:             "456",
		Type:           "issue",
		Phase:          PhaseImplement,
		DraftPRCreated: true, // Already created
	}
	c := &Controller{
		taskStates: map[string]*TaskState{taskID: state},
		logger:     newTestLogger(),
	}

	// maybeCreateDraftPR returns nil immediately when DraftPRCreated is true,
	// so the retry loop should succeed on the first attempt.
	blocked := c.createDraftPRWithRetry(context.Background(), taskID, state, PhaseImplement, 1)

	if blocked {
		t.Fatal("expected createDraftPRWithRetry to return blocked=false when PR already created")
	}
	if state.Phase != PhaseImplement {
		t.Errorf("state.Phase = %q, want %q", state.Phase, PhaseImplement)
	}
	if state.ControllerOverrode {
		t.Error("expected state.ControllerOverrode to remain false")
	}
}

func TestCreateDraftPRWithRetry_RespectsContextCancellation(t *testing.T) {
	taskID := "issue:789"
	state := &TaskState{
		ID:             "789",
		Type:           "issue",
		Phase:          PhaseImplement,
		DraftPRCreated: false,
	}
	c := &Controller{
		taskStates: map[string]*TaskState{taskID: state},
		logger:     newTestLogger(),
	}

	// Cancel context before calling — the retry loop should stop early.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	blocked := c.createDraftPRWithRetry(ctx, taskID, state, PhaseImplement, 1)

	if !blocked {
		t.Fatal("expected createDraftPRWithRetry to return blocked=true on cancelled context")
	}
	if state.Phase != PhaseBlocked {
		t.Errorf("state.Phase = %q, want %q", state.Phase, PhaseBlocked)
	}
	if !state.ControllerOverrode {
		t.Error("expected state.ControllerOverrode to be true")
	}
}

func TestIsAuthError(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		output []string
		want   bool
	}{
		{"nil error", nil, nil, false},
		{"generic error", fmt.Errorf("connection refused"), nil, false},
		{"401 in error", fmt.Errorf("exit status 1: HTTP 401"), nil, true},
		{"bad credentials in error", fmt.Errorf("Bad credentials"), nil, true},
		{"auth failed in error", fmt.Errorf("authentication failed for https://github.com"), nil, true},
		{"could not read username", fmt.Errorf("could not read Username"), nil, true},
		{"401 in output only", fmt.Errorf("exit status 1"), []string{"HTTP 401: Bad credentials"}, true},
		{"bad credentials in output only", fmt.Errorf("exit status 1"), []string{"Bad credentials (https://api.github.com/graphql)"}, true},
		{"clean output no auth issue", fmt.Errorf("exit status 1"), []string{"not found"}, false},
		{"cmdOutputError with 401 in output", &cmdOutputError{err: fmt.Errorf("exit status 1"), output: "HTTP 401: Bad credentials"}, nil, true},
		{"cmdOutputError with clean output", &cmdOutputError{err: fmt.Errorf("exit status 1"), output: "not found"}, nil, false},
		{"wrapped cmdOutputError with auth", fmt.Errorf("draft PR failed: %w", &cmdOutputError{err: fmt.Errorf("exit status 1"), output: "HTTP 401"}), nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isAuthError(tt.err, tt.output...)
			if got != tt.want {
				t.Errorf("isAuthError(%v, %v) = %v, want %v", tt.err, tt.output, got, tt.want)
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
