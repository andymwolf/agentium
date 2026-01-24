package claudecode

import (
	"strings"
	"testing"

	"github.com/andywolf/agentium/internal/agent"
)

func TestAdapter_Name(t *testing.T) {
	a := New()
	if got := a.Name(); got != "claude-code" {
		t.Errorf("Name() = %q, want %q", got, "claude-code")
	}
}

func TestAdapter_ContainerImage(t *testing.T) {
	a := New()
	if got := a.ContainerImage(); got != DefaultImage {
		t.Errorf("ContainerImage() = %q, want %q", got, DefaultImage)
	}
}

func TestAdapter_BuildEnv(t *testing.T) {
	a := New()
	session := &agent.Session{
		ID:          "test-session",
		Repository:  "github.com/org/repo",
		GitHubToken: "ghp_token123",
		Metadata: map[string]string{
			"custom_key": "custom_value",
		},
	}

	env := a.BuildEnv(session, 1)

	tests := []struct {
		key   string
		value string
	}{
		{"GITHUB_TOKEN", "ghp_token123"},
		{"AGENTIUM_SESSION_ID", "test-session"},
		{"AGENTIUM_ITERATION", "1"},
		{"AGENTIUM_REPOSITORY", "github.com/org/repo"},
		{"AGENTIUM_WORKDIR", "/workspace"},
		{"AGENTIUM_CUSTOM_KEY", "custom_value"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := env[tt.key]; got != tt.value {
				t.Errorf("env[%q] = %q, want %q", tt.key, got, tt.value)
			}
		})
	}
}

func TestAdapter_BuildCommand(t *testing.T) {
	a := New()
	session := &agent.Session{
		Repository: "github.com/org/repo",
		Tasks:      []string{"1", "2"},
	}

	cmd := a.BuildCommand(session, 1)

	if len(cmd) < 3 {
		t.Fatalf("BuildCommand() returned %d args, want at least 3", len(cmd))
	}

	if cmd[0] != "--print" {
		t.Errorf("BuildCommand()[0] = %q, want %q", cmd[0], "--print")
	}

	if cmd[1] != "--dangerously-skip-permissions" {
		t.Errorf("BuildCommand()[1] = %q, want %q", cmd[1], "--dangerously-skip-permissions")
	}
}

func TestAdapter_BuildPrompt(t *testing.T) {
	a := New()

	tests := []struct {
		name      string
		session   *agent.Session
		iteration int
		contains  []string
	}{
		{
			name: "basic prompt",
			session: &agent.Session{
				Repository: "github.com/org/repo",
				Tasks:      []string{"12", "17"},
			},
			iteration: 1,
			contains: []string{
				"github.com/org/repo",
				"Issue #12",
				"Issue #17",
				"Create a new branch",
				"Create a pull request",
			},
		},
		{
			name: "custom prompt",
			session: &agent.Session{
				Repository: "github.com/org/repo",
				Tasks:      []string{"1"},
				Prompt:     "Custom instructions here",
			},
			iteration: 1,
			contains: []string{
				"Custom instructions here",
				"Issue #1",
			},
		},
		{
			name: "iteration > 1",
			session: &agent.Session{
				Repository: "github.com/org/repo",
				Tasks:      []string{"1"},
			},
			iteration: 3,
			contains: []string{
				"iteration 3",
				"Continue from where you left off",
			},
		},
		{
			name: "active task with focused prompt - uses prompt directly",
			session: &agent.Session{
				Repository: "github.com/org/repo",
				Tasks:      []string{"6"},
				ActiveTask: "6",
				Prompt:     "## Your Task: Issue #6\n\nFocused prompt from controller",
			},
			iteration: 1,
			contains: []string{
				"Your Task: Issue #6",
				"Focused prompt from controller",
			},
		},
		{
			name: "active task prompt does not include generic instructions",
			session: &agent.Session{
				Repository: "github.com/org/repo",
				Tasks:      []string{"6"},
				ActiveTask: "6",
				Prompt:     "## Your Task: Issue #6\n\nDo something specific",
			},
			iteration: 1,
			contains: []string{
				"Your Task: Issue #6",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt := a.BuildPrompt(tt.session, tt.iteration)
			for _, substr := range tt.contains {
				if !strings.Contains(prompt, substr) {
					t.Errorf("BuildPrompt() missing %q in:\n%s", substr, prompt)
				}
			}
		})
	}
}

func TestAdapter_BuildPrompt_ActiveTaskSkipsGenericInstructions(t *testing.T) {
	a := New()
	session := &agent.Session{
		Repository: "github.com/org/repo",
		Tasks:      []string{"6", "7"},
		ActiveTask: "6",
		Prompt:     "## Your Task: Issue #6\n\nCustom focused instructions",
	}

	prompt := a.BuildPrompt(session, 2)

	// Should NOT contain generic multi-issue instructions
	genericPhrases := []string{
		"Create a new branch: agentium/issue-<number>",
		"For each issue:",
		"Issue #7", // Only issue #6 should be referenced
	}
	for _, phrase := range genericPhrases {
		if strings.Contains(prompt, phrase) {
			t.Errorf("BuildPrompt() with ActiveTask should not contain generic phrase %q, got:\n%s", phrase, prompt)
		}
	}

	// Should contain the focused prompt
	if !strings.Contains(prompt, "Custom focused instructions") {
		t.Errorf("BuildPrompt() missing focused prompt content")
	}
}

func TestAdapter_BuildCommand_IterationContext(t *testing.T) {
	a := New()

	tests := []struct {
		name                string
		session             *agent.Session
		wantSystemPrompt    string
		notWantSystemPrompt string
	}{
		{
			name: "nil IterationContext uses SystemPrompt",
			session: &agent.Session{
				Repository:   "github.com/org/repo",
				Tasks:        []string{"1"},
				SystemPrompt: "monolithic system prompt",
			},
			wantSystemPrompt: "monolithic system prompt",
		},
		{
			name: "IterationContext with SkillsPrompt overrides SystemPrompt",
			session: &agent.Session{
				Repository:   "github.com/org/repo",
				Tasks:        []string{"1"},
				SystemPrompt: "monolithic system prompt",
				IterationContext: &agent.IterationContext{
					Phase:        "IMPLEMENT",
					SkillsPrompt: "composed skills prompt",
				},
			},
			wantSystemPrompt:    "composed skills prompt",
			notWantSystemPrompt: "monolithic system prompt",
		},
		{
			name: "IterationContext with empty SkillsPrompt falls back to SystemPrompt",
			session: &agent.Session{
				Repository:   "github.com/org/repo",
				Tasks:        []string{"1"},
				SystemPrompt: "monolithic system prompt",
				IterationContext: &agent.IterationContext{
					Phase:        "IMPLEMENT",
					SkillsPrompt: "",
				},
			},
			wantSystemPrompt: "monolithic system prompt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := a.BuildCommand(tt.session, 1)

			// Find the --system-prompt value
			var systemPromptValue string
			for i, arg := range cmd {
				if arg == "--system-prompt" && i+1 < len(cmd) {
					systemPromptValue = cmd[i+1]
					break
				}
			}

			if tt.wantSystemPrompt != "" && systemPromptValue != tt.wantSystemPrompt {
				t.Errorf("system prompt = %q, want %q", systemPromptValue, tt.wantSystemPrompt)
			}

			if tt.notWantSystemPrompt != "" && systemPromptValue == tt.notWantSystemPrompt {
				t.Errorf("system prompt should not be %q", tt.notWantSystemPrompt)
			}
		})
	}
}

func TestAdapter_BuildCommand_ModelOverride(t *testing.T) {
	a := New()

	tests := []struct {
		name      string
		session   *agent.Session
		wantModel string
	}{
		{
			name: "no override - no --model flag",
			session: &agent.Session{
				Repository: "github.com/org/repo",
				Tasks:      []string{"1"},
			},
			wantModel: "",
		},
		{
			name: "with model override",
			session: &agent.Session{
				Repository: "github.com/org/repo",
				Tasks:      []string{"1"},
				IterationContext: &agent.IterationContext{
					Phase:         "IMPLEMENT",
					ModelOverride: "claude-opus-4-20250514",
				},
			},
			wantModel: "claude-opus-4-20250514",
		},
		{
			name: "empty model override - no --model flag",
			session: &agent.Session{
				Repository: "github.com/org/repo",
				Tasks:      []string{"1"},
				IterationContext: &agent.IterationContext{
					Phase:         "TEST",
					ModelOverride: "",
				},
			},
			wantModel: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := a.BuildCommand(tt.session, 1)

			var modelValue string
			for i, arg := range cmd {
				if arg == "--model" && i+1 < len(cmd) {
					modelValue = cmd[i+1]
					break
				}
			}

			if modelValue != tt.wantModel {
				t.Errorf("--model = %q, want %q (cmd: %v)", modelValue, tt.wantModel, cmd)
			}
		})
	}
}

func TestAdapter_ParseOutput_StatusSignals(t *testing.T) {
	a := New()

	tests := []struct {
		name              string
		exitCode          int
		stdout            string
		stderr            string
		wantAgentStatus   string
		wantStatusMessage string
		wantPushedChanges bool
		wantSuccess       bool
	}{
		{
			name:              "TESTS_PASSED status",
			exitCode:          0,
			stdout:            "Running tests...\nAGENTIUM_STATUS: TESTS_PASSED\nAll tests passed",
			stderr:            "",
			wantAgentStatus:   "TESTS_PASSED",
			wantStatusMessage: "",
			wantPushedChanges: false,
			wantSuccess:       true,
		},
		{
			name:              "TESTS_FAILED status with message",
			exitCode:          1,
			stdout:            "Running tests...\nAGENTIUM_STATUS: TESTS_FAILED 3 tests failed in auth module",
			stderr:            "",
			wantAgentStatus:   "TESTS_FAILED",
			wantStatusMessage: "3 tests failed in auth module",
			wantPushedChanges: false,
			wantSuccess:       false,
		},
		{
			name:              "PR_CREATED status with URL",
			exitCode:          0,
			stdout:            "Creating PR...\nAGENTIUM_STATUS: PR_CREATED https://github.com/org/repo/pull/42",
			stderr:            "",
			wantAgentStatus:   "PR_CREATED",
			wantStatusMessage: "https://github.com/org/repo/pull/42",
			wantPushedChanges: true,
			wantSuccess:       true,
		},
		{
			name:              "PUSHED status",
			exitCode:          0,
			stdout:            "Pushing changes...\nAGENTIUM_STATUS: PUSHED",
			stderr:            "",
			wantAgentStatus:   "PUSHED",
			wantStatusMessage: "",
			wantPushedChanges: true,
			wantSuccess:       true,
		},
		{
			name:              "COMPLETE status",
			exitCode:          0,
			stdout:            "All work done\nAGENTIUM_STATUS: COMPLETE",
			stderr:            "",
			wantAgentStatus:   "COMPLETE",
			wantStatusMessage: "",
			wantPushedChanges: true,
			wantSuccess:       true,
		},
		{
			name:              "NOTHING_TO_DO status",
			exitCode:          0,
			stdout:            "Reviewed feedback, no changes needed\nAGENTIUM_STATUS: NOTHING_TO_DO",
			stderr:            "",
			wantAgentStatus:   "NOTHING_TO_DO",
			wantStatusMessage: "",
			wantPushedChanges: false,
			wantSuccess:       true,
		},
		{
			name:              "BLOCKED status with reason",
			exitCode:          0,
			stdout:            "AGENTIUM_STATUS: BLOCKED need clarification on requirements",
			stderr:            "",
			wantAgentStatus:   "BLOCKED",
			wantStatusMessage: "need clarification on requirements",
			wantPushedChanges: false,
			wantSuccess:       true,
		},
		{
			name:              "multiple statuses takes last one",
			exitCode:          0,
			stdout:            "AGENTIUM_STATUS: TESTS_RUNNING\nRunning...\nAGENTIUM_STATUS: TESTS_PASSED\nDone",
			stderr:            "",
			wantAgentStatus:   "TESTS_PASSED",
			wantStatusMessage: "",
			wantPushedChanges: false,
			wantSuccess:       true,
		},
		{
			name:              "status in stderr",
			exitCode:          0,
			stdout:            "",
			stderr:            "AGENTIUM_STATUS: ANALYZING reviewing changes",
			wantAgentStatus:   "ANALYZING",
			wantStatusMessage: "reviewing changes",
			wantPushedChanges: false,
			wantSuccess:       true,
		},
		{
			name:              "no status signal",
			exitCode:          0,
			stdout:            "Just some normal output without status",
			stderr:            "",
			wantAgentStatus:   "",
			wantStatusMessage: "",
			wantPushedChanges: false,
			wantSuccess:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := a.ParseOutput(tt.exitCode, tt.stdout, tt.stderr)
			if err != nil {
				t.Fatalf("ParseOutput() returned error: %v", err)
			}

			if result.AgentStatus != tt.wantAgentStatus {
				t.Errorf("AgentStatus = %q, want %q", result.AgentStatus, tt.wantAgentStatus)
			}

			if result.StatusMessage != tt.wantStatusMessage {
				t.Errorf("StatusMessage = %q, want %q", result.StatusMessage, tt.wantStatusMessage)
			}

			if result.PushedChanges != tt.wantPushedChanges {
				t.Errorf("PushedChanges = %v, want %v", result.PushedChanges, tt.wantPushedChanges)
			}

			if result.Success != tt.wantSuccess {
				t.Errorf("Success = %v, want %v", result.Success, tt.wantSuccess)
			}
		})
	}
}

func TestAdapter_ParseOutput(t *testing.T) {
	a := New()

	tests := []struct {
		name           string
		exitCode       int
		stdout         string
		stderr         string
		wantSuccess    bool
		wantPRs        []string
		wantTasks      []string
		wantErrContain string
	}{
		{
			name:        "successful run with PR",
			exitCode:    0,
			stdout:      "Created PR #42 for the fix\nhttps://github.com/org/repo/pull/42",
			stderr:      "",
			wantSuccess: true,
			wantPRs:     []string{"42"},
		},
		{
			name:        "successful run with multiple PRs",
			exitCode:    0,
			stdout:      "Created pull request #10\nOpened PR #20\nhttps://github.com/org/repo/pull/30",
			stderr:      "",
			wantSuccess: true,
			wantPRs:     []string{"10", "20", "30"},
		},
		{
			name:        "detects completed tasks",
			exitCode:    0,
			stdout:      "Fixes #12\nCloses #17\nresolves #24",
			stderr:      "",
			wantSuccess: true,
			wantTasks:   []string{"12", "17", "24"},
		},
		{
			name:           "failed run with error",
			exitCode:       1,
			stdout:         "",
			stderr:         "error: failed to push to remote",
			wantSuccess:    false,
			wantErrContain: "failed to push to remote",
		},
		{
			name:           "failed run with fatal error",
			exitCode:       1,
			stdout:         "",
			stderr:         "fatal: not a git repository",
			wantSuccess:    false,
			wantErrContain: "not a git repository",
		},
		{
			name:        "empty output success",
			exitCode:    0,
			stdout:      "",
			stderr:      "",
			wantSuccess: true,
		},
		{
			name:        "PR URL without text mention",
			exitCode:    0,
			stdout:      "Done! See https://github.com/org/repo/pull/99",
			stderr:      "",
			wantSuccess: true,
			wantPRs:     []string{"99"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := a.ParseOutput(tt.exitCode, tt.stdout, tt.stderr)
			if err != nil {
				t.Fatalf("ParseOutput() returned error: %v", err)
			}

			if result.Success != tt.wantSuccess {
				t.Errorf("Success = %v, want %v", result.Success, tt.wantSuccess)
			}

			if result.ExitCode != tt.exitCode {
				t.Errorf("ExitCode = %d, want %d", result.ExitCode, tt.exitCode)
			}

			if len(tt.wantPRs) > 0 {
				if len(result.PRsCreated) != len(tt.wantPRs) {
					t.Errorf("PRsCreated = %v, want %v", result.PRsCreated, tt.wantPRs)
				} else {
					for i, pr := range tt.wantPRs {
						if result.PRsCreated[i] != pr {
							t.Errorf("PRsCreated[%d] = %q, want %q", i, result.PRsCreated[i], pr)
						}
					}
				}
			}

			if len(tt.wantTasks) > 0 {
				if len(result.TasksCompleted) != len(tt.wantTasks) {
					t.Errorf("TasksCompleted = %v, want %v", result.TasksCompleted, tt.wantTasks)
				}
			}

			if tt.wantErrContain != "" && !strings.Contains(result.Error, tt.wantErrContain) {
				t.Errorf("Error = %q, want to contain %q", result.Error, tt.wantErrContain)
			}
		})
	}
}

func TestAdapter_Validate(t *testing.T) {
	tests := []struct {
		name    string
		adapter *Adapter
		wantErr bool
	}{
		{
			name:    "valid adapter",
			adapter: New(),
			wantErr: false,
		},
		{
			name:    "empty image",
			adapter: &Adapter{image: ""},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.adapter.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAdapter_Registration(t *testing.T) {
	// Test that the adapter is properly registered
	a, err := agent.Get("claude-code")
	if err != nil {
		t.Fatalf("Get(claude-code) returned error: %v", err)
	}

	if a.Name() != "claude-code" {
		t.Errorf("Registered agent Name() = %q, want %q", a.Name(), "claude-code")
	}
}
