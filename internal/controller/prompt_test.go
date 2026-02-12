package controller

import (
	"fmt"
	"io"
	"log"
	"testing"

	"github.com/andywolf/agentium/internal/agent"
	"github.com/andywolf/agentium/internal/handoff"
	"github.com/andywolf/agentium/internal/memory"
)

func TestBuildPromptForTask(t *testing.T) {
	tests := []struct {
		name         string
		issueNumber  string
		issueDetails []issueDetail
		existingWork *agent.ExistingWork
		phase        TaskPhase
		contains     []string
		notContains  []string
	}{
		{
			name:        "fresh start - no existing work (IMPLEMENT phase) - default prefix",
			issueNumber: "42",
			issueDetails: []issueDetail{
				{Number: 42, Title: "Fix login bug", Body: "The login page crashes"},
			},
			existingWork: nil,
			phase:        PhaseImplement,
			contains: []string{
				"Issue #42",
				"Fix login bug",
				"login page crashes",
				"Create a new branch",
				"feature/issue-42", // Default prefix when no labels
				"Create a pull request",
			},
			notContains: []string{
				"Existing Work Detected",
				"Do NOT create a new branch",
				"Follow the instructions in your system prompt",
			},
		},
		{
			name:        "fresh start - uses bug label prefix",
			issueNumber: "42",
			issueDetails: []issueDetail{
				{Number: 42, Title: "Fix login bug", Body: "The login page crashes", Labels: []issueLabel{{Name: "bug"}}},
			},
			existingWork: nil,
			phase:        PhaseImplement,
			contains: []string{
				"Issue #42",
				"bug/issue-42", // Uses label-based prefix
			},
			notContains: []string{
				"feature/issue-42",
			},
		},
		{
			name:        "fresh start - empty phase defaults to IMPLEMENT behavior",
			issueNumber: "42",
			issueDetails: []issueDetail{
				{Number: 42, Title: "Fix login bug", Body: "The login page crashes"},
			},
			existingWork: nil,
			phase:        "",
			contains: []string{
				"Issue #42",
				"Create a new branch",
			},
			notContains: []string{
				"Follow the instructions in your system prompt",
			},
		},
		{
			name:        "existing PR found (IMPLEMENT phase)",
			issueNumber: "6",
			issueDetails: []issueDetail{
				{Number: 6, Title: "Add cloud logging", Body: "Integrate GCP logging"},
			},
			existingWork: &agent.ExistingWork{
				Branch:   "feature/issue-6-cloud-logging",
				PRNumber: "87",
				PRTitle:  "Add Cloud Logging integration",
			},
			phase: PhaseImplement,
			contains: []string{
				"Issue #6",
				"Existing Work Detected",
				"PR #87",
				"feature/issue-6-cloud-logging",
				"Do NOT create a new branch",
				"Do NOT create a new PR",
			},
			notContains: []string{
				"Create a new branch",
				"Follow the instructions in your system prompt",
			},
		},
		{
			name:        "existing branch only (no PR) - IMPLEMENT phase",
			issueNumber: "7",
			issueDetails: []issueDetail{
				{Number: 7, Title: "Graceful shutdown", Body: "Implement shutdown"},
			},
			existingWork: &agent.ExistingWork{
				Branch: "enhancement/issue-7-graceful-shutdown",
			},
			phase: PhaseImplement,
			contains: []string{
				"Issue #7",
				"Existing Work Detected",
				"enhancement/issue-7-graceful-shutdown",
				"Do NOT create a new branch",
				"Create a PR linking to the issue",
			},
			notContains: []string{
				"Create a new branch",
				"Do NOT create a new PR",
				"Follow the instructions in your system prompt",
			},
		},
		{
			name:         "issue not in details (IMPLEMENT phase) - default prefix",
			issueNumber:  "99",
			issueDetails: []issueDetail{},
			existingWork: nil,
			phase:        PhaseImplement,
			contains: []string{
				"Issue #99",
				"Create a new branch",
				"feature/issue-99", // Default prefix when issue not found
			},
		},
		{
			name:        "PLAN phase - defers to system prompt",
			issueNumber: "42",
			issueDetails: []issueDetail{
				{Number: 42, Title: "Fix login bug", Body: "The login page crashes"},
			},
			existingWork: nil,
			phase:        PhasePlan,
			contains: []string{
				"Issue #42",
				"Fix login bug",
				"Follow the instructions in your system prompt",
			},
			notContains: []string{
				"Create a new branch",
				"Create a pull request",
				"Run tests",
			},
		},
		{
			name:        "DOCS phase - defers to system prompt",
			issueNumber: "42",
			issueDetails: []issueDetail{
				{Number: 42, Title: "Fix login bug", Body: "The login page crashes"},
			},
			existingWork: nil,
			phase:        PhaseDocs,
			contains: []string{
				"Issue #42",
				"Follow the instructions in your system prompt",
			},
			notContains: []string{
				"Create a new branch",
				"Create a pull request",
			},
		},
		{
			name:        "PLAN phase with existing work - includes existing work context but defers instructions",
			issueNumber: "6",
			issueDetails: []issueDetail{
				{Number: 6, Title: "Add cloud logging", Body: "Integrate GCP logging"},
			},
			existingWork: &agent.ExistingWork{
				Branch:   "feature/issue-6-cloud-logging",
				PRNumber: "87",
				PRTitle:  "Add Cloud Logging integration",
			},
			phase: PhasePlan,
			contains: []string{
				"Issue #6",
				"Existing Work Detected",
				"PR #87",
				"feature/issue-6-cloud-logging",
				"Follow the instructions in your system prompt",
			},
			notContains: []string{
				"Check out the existing branch",
				"Do NOT create a new branch",
				"Run tests",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build issueDetailsByNumber map for O(1) lookup
			issueDetailsByNumber := make(map[string]*issueDetail, len(tt.issueDetails))
			for i := range tt.issueDetails {
				issueDetailsByNumber[fmt.Sprintf("%d", tt.issueDetails[i].Number)] = &tt.issueDetails[i]
			}

			c := &Controller{
				config:               SessionConfig{Repository: "github.com/org/repo"},
				issueDetails:         tt.issueDetails,
				issueDetailsByNumber: issueDetailsByNumber,
				workDir:              "/workspace",
			}
			got := c.buildPromptForTask(tt.issueNumber, tt.existingWork, tt.phase)

			for _, substr := range tt.contains {
				if !containsString(got, substr) {
					t.Errorf("buildPromptForTask() missing %q in:\n%s", substr, got)
				}
			}
			for _, substr := range tt.notContains {
				if containsString(got, substr) {
					t.Errorf("buildPromptForTask() should not contain %q in:\n%s", substr, got)
				}
			}
		})
	}
}

func TestRenderWithParameters(t *testing.T) {
	tests := []struct {
		name           string
		repository     string
		activeTask     string
		activeTaskType string
		promptContext  *PromptContext
		prompt         string
		want           string
	}{
		{
			name:           "issue_url derived from repo and issue number",
			repository:     "org/repo",
			activeTask:     "42",
			activeTaskType: "issue",
			prompt:         "Fix {{issue_url}}",
			want:           "Fix https://github.com/org/repo/issues/42",
		},
		{
			name:           "explicit issue_url takes precedence over derivation",
			repository:     "org/repo",
			activeTask:     "42",
			activeTaskType: "issue",
			promptContext:  &PromptContext{IssueURL: "https://custom.example.com/issue/42"},
			prompt:         "Fix {{issue_url}}",
			want:           "Fix https://custom.example.com/issue/42",
		},
		{
			name:           "issue_url not derived for PR tasks",
			repository:     "org/repo",
			activeTask:     "99",
			activeTaskType: "pr",
			prompt:         "Review {{issue_url}}",
			want:           "Review {{issue_url}}",
		},
		{
			name:           "issue_number set for issue tasks",
			repository:     "org/repo",
			activeTask:     "42",
			activeTaskType: "issue",
			prompt:         "Working on #{{issue_number}}",
			want:           "Working on #42",
		},
		{
			name:           "issue_number not set for PR tasks",
			repository:     "org/repo",
			activeTask:     "99",
			activeTaskType: "pr",
			prompt:         "Working on #{{issue_number}}",
			want:           "Working on #{{issue_number}}",
		},
		{
			name:           "user parameters override derived issue_url",
			repository:     "org/repo",
			activeTask:     "42",
			activeTaskType: "issue",
			promptContext:  &PromptContext{Parameters: map[string]string{"issue_url": "user-override"}},
			prompt:         "Fix {{issue_url}}",
			want:           "Fix user-override",
		},
		{
			name:           "repository always available",
			repository:     "org/repo",
			activeTask:     "",
			activeTaskType: "",
			prompt:         "Repo: {{repository}}",
			want:           "Repo: org/repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Controller{
				config: SessionConfig{
					Repository:    tt.repository,
					PromptContext: tt.promptContext,
				},
				activeTask:     tt.activeTask,
				activeTaskType: tt.activeTaskType,
			}
			got := c.renderWithParameters(tt.prompt)
			if got != tt.want {
				t.Errorf("renderWithParameters() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildPromptForTask_ImplementWithPlan(t *testing.T) {
	// When a handoff plan exists for the issue, IMPLEMENT phase should
	// omit the issue body and comments, deferring to the phase input.
	handoffDir := t.TempDir()
	hStore, err := handoff.NewStore(handoffDir)
	if err != nil {
		t.Fatalf("failed to create handoff store: %v", err)
	}

	taskID := "issue:42"
	if err := hStore.StorePhaseOutput(taskID, handoff.PhasePlan, 1, &handoff.PlanOutput{
		Summary:       "Implement login fix",
		FilesToModify: []string{"auth.go"},
		ImplementationSteps: []handoff.ImplementationStep{
			{Order: 1, Description: "Fix auth handler"},
		},
		TestingApproach: "unit tests",
	}); err != nil {
		t.Fatalf("failed to store plan output: %v", err)
	}

	issueDetails := []issueDetail{
		{Number: 42, Title: "Fix login bug", Body: "The login page crashes when you click submit", Comments: []issueComment{
			{Body: "This is urgent", Author: issueCommentAuthor{Login: "user1"}},
		}},
	}
	issueDetailsByNumber := map[string]*issueDetail{"42": &issueDetails[0]}

	c := &Controller{
		config:               SessionConfig{Repository: "org/repo"},
		issueDetails:         issueDetails,
		issueDetailsByNumber: issueDetailsByNumber,
		workDir:              "/workspace",
		handoffStore:         hStore,
	}

	got := c.buildPromptForTask("42", nil, PhaseImplement)

	// Title should still be present
	if !containsString(got, "Fix login bug") {
		t.Errorf("expected title in output, got:\n%s", got)
	}
	// Body should be omitted
	if containsString(got, "login page crashes") {
		t.Errorf("expected issue body to be omitted when plan exists, got:\n%s", got)
	}
	// Comments should be omitted
	if containsString(got, "This is urgent") {
		t.Errorf("expected comments to be omitted when plan exists, got:\n%s", got)
	}
	// Should reference phase input
	if !containsString(got, "Phase Input") {
		t.Errorf("expected reference to Phase Input, got:\n%s", got)
	}
}

func TestBuildPromptForTask_ImplementWithoutPlan(t *testing.T) {
	// When no handoff plan exists, IMPLEMENT phase should include
	// the full issue body and comments (fallback behavior).
	issueDetails := []issueDetail{
		{Number: 42, Title: "Fix login bug", Body: "The login page crashes when you click submit", Comments: []issueComment{
			{Body: "This is urgent", Author: issueCommentAuthor{Login: "user1"}},
		}},
	}
	issueDetailsByNumber := map[string]*issueDetail{"42": &issueDetails[0]}

	c := &Controller{
		config:               SessionConfig{Repository: "org/repo"},
		issueDetails:         issueDetails,
		issueDetailsByNumber: issueDetailsByNumber,
		workDir:              "/workspace",
		// No handoffStore — plan doesn't exist
	}

	got := c.buildPromptForTask("42", nil, PhaseImplement)

	// Full body should be present
	if !containsString(got, "login page crashes") {
		t.Errorf("expected issue body when no plan exists, got:\n%s", got)
	}
	// Comments should be present
	if !containsString(got, "This is urgent") {
		t.Errorf("expected comments when no plan exists, got:\n%s", got)
	}
	// Should NOT reference phase input
	if containsString(got, "Phase Input") {
		t.Errorf("should not reference Phase Input when no plan exists, got:\n%s", got)
	}
}

func TestBuildPromptForTask_PlanPhaseAlwaysIncludesBody(t *testing.T) {
	// PLAN phase should always include the full body even when a plan exists
	// (plan might exist from a previous ITERATE cycle).
	handoffDir := t.TempDir()
	hStore, err := handoff.NewStore(handoffDir)
	if err != nil {
		t.Fatalf("failed to create handoff store: %v", err)
	}

	taskID := "issue:42"
	if err := hStore.StorePhaseOutput(taskID, handoff.PhasePlan, 1, &handoff.PlanOutput{
		Summary: "Old plan",
	}); err != nil {
		t.Fatalf("failed to store plan output: %v", err)
	}

	issueDetails := []issueDetail{
		{Number: 42, Title: "Fix login bug", Body: "The login page crashes"},
	}
	issueDetailsByNumber := map[string]*issueDetail{"42": &issueDetails[0]}

	c := &Controller{
		config:               SessionConfig{Repository: "org/repo"},
		issueDetails:         issueDetails,
		issueDetailsByNumber: issueDetailsByNumber,
		workDir:              "/workspace",
		handoffStore:         hStore,
	}

	got := c.buildPromptForTask("42", nil, PhasePlan)

	// PLAN should still include the full body
	if !containsString(got, "login page crashes") {
		t.Errorf("PLAN phase should include issue body even when plan exists, got:\n%s", got)
	}
}

func TestBuildIterateFeedbackSection(t *testing.T) {
	tests := []struct {
		name        string
		memoryStore bool
		entries     []struct {
			Type           memory.SignalType
			Content        string
			PhaseIteration int
			TaskID         string
		}
		phaseIteration int
		taskID         string
		phase          TaskPhase
		handoffStore   *handoff.Store
		wantEmpty      bool
		wantContains   []string
		wantNotContain []string
	}{
		{
			name:        "nil memory store returns empty",
			memoryStore: false,
			phase:       PhaseImplement,
			wantEmpty:   true,
		},
		{
			name:           "first iteration returns empty",
			memoryStore:    true,
			phaseIteration: 1,
			phase:          PhaseImplement,
			wantEmpty:      true,
		},
		{
			name:        "no feedback for previous iteration returns empty",
			memoryStore: true,
			entries: []struct {
				Type           memory.SignalType
				Content        string
				PhaseIteration int
				TaskID         string
			}{
				{Type: memory.EvalFeedback, Content: "some feedback", PhaseIteration: 1, TaskID: "issue:42"},
			},
			phaseIteration: 3, // Looking for iteration 2, but only iteration 1 exists
			taskID:         "issue:42",
			phase:          PhaseImplement,
			wantEmpty:      true,
		},
		{
			name:        "returns reviewer feedback from previous iteration",
			memoryStore: true,
			entries: []struct {
				Type           memory.SignalType
				Content        string
				PhaseIteration int
				TaskID         string
			}{
				{Type: memory.EvalFeedback, Content: "Address the test coverage gap", PhaseIteration: 1, TaskID: "issue:42"},
			},
			phaseIteration: 2,
			taskID:         "issue:42",
			phase:          PhaseImplement,
			wantContains: []string{
				"code changes were reviewed",
				"## The reviewer also noted:",
				"Address the test coverage gap",
				"AGENTIUM_HANDOFF",
			},
			wantNotContain: []string{
				"## Feedback from Previous Iteration",
				"**How to use this feedback:**",
			},
		},
		{
			name:        "returns judge directive from previous iteration",
			memoryStore: true,
			entries: []struct {
				Type           memory.SignalType
				Content        string
				PhaseIteration int
				TaskID         string
			}{
				{Type: memory.JudgeDirective, Content: "Add unit tests for edge cases", PhaseIteration: 1, TaskID: "issue:42"},
			},
			phaseIteration: 2,
			taskID:         "issue:42",
			phase:          PhaseImplement,
			wantContains: []string{
				"code changes were reviewed",
				"## Here's what you need to fix:",
				"Add unit tests for edge cases",
			},
			wantNotContain: []string{
				"## Feedback from Previous Iteration",
			},
		},
		{
			name:        "returns both reviewer feedback and judge directive",
			memoryStore: true,
			entries: []struct {
				Type           memory.SignalType
				Content        string
				PhaseIteration int
				TaskID         string
			}{
				{Type: memory.EvalFeedback, Content: "Detailed analysis of the implementation", PhaseIteration: 1, TaskID: "issue:42"},
				{Type: memory.JudgeDirective, Content: "Fix the validation logic", PhaseIteration: 1, TaskID: "issue:42"},
			},
			phaseIteration: 2,
			taskID:         "issue:42",
			phase:          PhaseImplement,
			wantContains: []string{
				"## The reviewer also noted:",
				"Detailed analysis of the implementation",
				"## Here's what you need to fix:",
				"Fix the validation logic",
			},
			wantNotContain: []string{
				"## Feedback from Previous Iteration",
			},
		},
		{
			name:        "filters by task ID",
			memoryStore: true,
			entries: []struct {
				Type           memory.SignalType
				Content        string
				PhaseIteration int
				TaskID         string
			}{
				{Type: memory.EvalFeedback, Content: "Feedback for issue 42", PhaseIteration: 1, TaskID: "issue:42"},
				{Type: memory.EvalFeedback, Content: "Feedback for issue 99", PhaseIteration: 1, TaskID: "issue:99"},
			},
			phaseIteration: 2,
			taskID:         "issue:42",
			phase:          PhaseImplement,
			wantContains:   []string{"Feedback for issue 42"},
			wantNotContain: []string{"Feedback for issue 99"},
		},
		{
			name:        "PLAN phase includes handoff template and narrative",
			memoryStore: true,
			entries: []struct {
				Type           memory.SignalType
				Content        string
				PhaseIteration int
				TaskID         string
			}{
				{Type: memory.EvalFeedback, Content: "Add API endpoints to plan", PhaseIteration: 1, TaskID: "issue:42"},
			},
			phaseIteration: 2,
			taskID:         "issue:42",
			phase:          PhasePlan,
			wantContains: []string{
				"plan was reviewed",
				"AGENTIUM_HANDOFF",
				"revised plan",
				"Add API endpoints to plan",
			},
			wantNotContain: []string{
				"requesting fixes",
			},
		},
		{
			name:        "IMPLEMENT phase includes handoff template with code change narrative",
			memoryStore: true,
			entries: []struct {
				Type           memory.SignalType
				Content        string
				PhaseIteration int
				TaskID         string
			}{
				{Type: memory.JudgeDirective, Content: "Fix the test failures", PhaseIteration: 1, TaskID: "issue:42"},
			},
			phaseIteration: 2,
			taskID:         "issue:42",
			phase:          PhaseImplement,
			wantContains: []string{
				"code changes were reviewed",
				"requesting fixes",
				"AGENTIUM_HANDOFF",
				"Fix the test failures",
			},
			wantNotContain: []string{
				"revised plan",
			},
		},
		{
			name:        "DOCS phase includes handoff template and narrative",
			memoryStore: true,
			entries: []struct {
				Type           memory.SignalType
				Content        string
				PhaseIteration int
				TaskID         string
			}{
				{Type: memory.EvalFeedback, Content: "Update README", PhaseIteration: 1, TaskID: "issue:42"},
			},
			phaseIteration: 2,
			taskID:         "issue:42",
			phase:          PhaseDocs,
			wantContains: []string{
				"documentation updates were reviewed",
				"AGENTIUM_HANDOFF",
				"docs_updated",
			},
		},
		{
			name:        "VERIFY phase includes handoff template and narrative",
			memoryStore: true,
			entries: []struct {
				Type           memory.SignalType
				Content        string
				PhaseIteration int
				TaskID         string
			}{
				{Type: memory.EvalFeedback, Content: "CI still failing", PhaseIteration: 1, TaskID: "issue:42"},
			},
			phaseIteration: 2,
			taskID:         "issue:42",
			phase:          PhaseVerify,
			wantContains: []string{
				"verification attempt needs further work",
				"AGENTIUM_HANDOFF",
				"checks_passed",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Controller{
				logger:       log.New(io.Discard, "", 0),
				handoffStore: tt.handoffStore,
			}

			if tt.memoryStore {
				// Create a temp directory for the memory store
				tempDir := t.TempDir()
				store := memory.NewStore(tempDir, memory.Config{})

				// Add entries
				for _, e := range tt.entries {
					store.UpdateWithPhaseIteration([]memory.Signal{
						{Type: e.Type, Content: e.Content},
					}, 1, e.PhaseIteration, e.TaskID)
				}
				c.memoryStore = store
			}

			got := c.buildIterateFeedbackSection(tt.taskID, tt.phaseIteration, "", tt.phase)

			if tt.wantEmpty {
				if got != "" {
					t.Errorf("buildIterateFeedbackSection() = %q, want empty", got)
				}
				return
			}

			for _, substr := range tt.wantContains {
				if !containsString(got, substr) {
					t.Errorf("buildIterateFeedbackSection() missing %q in:\n%s", substr, got)
				}
			}
			for _, substr := range tt.wantNotContain {
				if containsString(got, substr) {
					t.Errorf("buildIterateFeedbackSection() should not contain %q in:\n%s", substr, got)
				}
			}
		})
	}
}

func TestBuildIterateFeedbackSectionWithPlan(t *testing.T) {
	// Test that PLAN phase includes current plan from handoff store
	tempDir := t.TempDir()
	memStore := memory.NewStore(tempDir, memory.Config{})
	memStore.UpdateWithPhaseIteration([]memory.Signal{
		{Type: memory.EvalFeedback, Content: "Add more detail to plan"},
	}, 1, 1, "issue:42")

	handoffDir := t.TempDir()
	hStore, err := handoff.NewStore(handoffDir)
	if err != nil {
		t.Fatalf("failed to create handoff store: %v", err)
	}
	planOutput := &handoff.PlanOutput{
		Summary:         "Implement user auth",
		FilesToModify:   []string{"auth.go"},
		TestingApproach: "Unit tests",
		ImplementationSteps: []handoff.ImplementationStep{
			{Order: 1, Description: "Add login endpoint"},
		},
	}
	if err := hStore.StorePhaseOutput("issue:42", handoff.PhasePlan, 1, planOutput); err != nil {
		t.Fatalf("failed to store plan output: %v", err)
	}

	c := &Controller{
		logger:       log.New(io.Discard, "", 0),
		memoryStore:  memStore,
		handoffStore: hStore,
	}

	got := c.buildIterateFeedbackSection("issue:42", 2, "", PhasePlan)

	wantContains := []string{
		"plan was reviewed",
		"AGENTIUM_HANDOFF",
		"Your current plan",
		"Implement user auth",
		"Add login endpoint",
		"Submit your revised plan",
	}
	for _, substr := range wantContains {
		if !containsString(got, substr) {
			t.Errorf("buildIterateFeedbackSection() with plan missing %q in:\n%s", substr, got)
		}
	}
}
