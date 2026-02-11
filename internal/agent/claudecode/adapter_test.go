package claudecode

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/andywolf/agentium/internal/agent"
)

// wrapInStreamJSON wraps plain text as an assistant text block in NDJSON format
// for use in ParseOutput tests that previously passed raw text as stdout.
func wrapInStreamJSON(text string) string {
	escaped, _ := json.Marshal(text)
	return fmt.Sprintf(`{"type":"assistant","message":{"content":[{"type":"text","text":%s}]}}`, string(escaped)) + "\n"
}

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

	t.Run("non-interactive mode", func(t *testing.T) {
		session := &agent.Session{
			Repository:  "github.com/org/repo",
			Tasks:       []string{"1", "2"},
			Interactive: false,
		}

		cmd := a.BuildCommand(session, 1)

		// Non-interactive mode: prompt is delivered via stdin, not as positional arg
		// Should have exactly 5 args: --print, --verbose, --output-format, stream-json, --dangerously-skip-permissions
		if len(cmd) != 5 {
			t.Fatalf("BuildCommand() returned %d args, want 5", len(cmd))
		}

		if cmd[0] != "--print" {
			t.Errorf("BuildCommand()[0] = %q, want %q", cmd[0], "--print")
		}

		if cmd[1] != "--verbose" {
			t.Errorf("BuildCommand()[1] = %q, want %q", cmd[1], "--verbose")
		}

		if cmd[2] != "--output-format" {
			t.Errorf("BuildCommand()[2] = %q, want %q", cmd[2], "--output-format")
		}

		if cmd[3] != "stream-json" {
			t.Errorf("BuildCommand()[3] = %q, want %q", cmd[3], "stream-json")
		}

		if cmd[4] != "--dangerously-skip-permissions" {
			t.Errorf("BuildCommand()[4] = %q, want %q", cmd[4], "--dangerously-skip-permissions")
		}
	})

	t.Run("interactive mode", func(t *testing.T) {
		session := &agent.Session{
			Repository:  "github.com/org/repo",
			Tasks:       []string{"1"},
			Interactive: true,
		}

		cmd := a.BuildCommand(session, 1)

		// Interactive mode should have --verbose but NOT --print or --dangerously-skip-permissions
		if cmd[0] != "--verbose" {
			t.Errorf("BuildCommand()[0] = %q, want %q", cmd[0], "--verbose")
		}

		for _, arg := range cmd {
			if arg == "--print" {
				t.Error("Interactive mode should not have --print flag")
			}
			if arg == "--dangerously-skip-permissions" {
				t.Error("Interactive mode should not have --dangerously-skip-permissions flag")
			}
		}
	})

	t.Run("non-interactive plan mode", func(t *testing.T) {
		session := &agent.Session{
			Repository:  "github.com/org/repo",
			Tasks:       []string{"1"},
			Interactive: false,
			IterationContext: &agent.IterationContext{
				Phase: "PLAN",
			},
		}

		cmd := a.BuildCommand(session, 1)

		// Non-interactive plan mode should use --permission-mode plan
		// without --dangerously-skip-permissions (they conflict per anthropics/claude-code#17544).
		if len(cmd) != 6 {
			t.Fatalf("BuildCommand() returned %d args, want 6: %v", len(cmd), cmd)
		}

		if cmd[4] != "--permission-mode" {
			t.Errorf("BuildCommand()[4] = %q, want %q", cmd[4], "--permission-mode")
		}

		if cmd[5] != "plan" {
			t.Errorf("BuildCommand()[5] = %q, want %q", cmd[5], "plan")
		}

		// Verify --dangerously-skip-permissions is NOT present
		for _, arg := range cmd {
			if arg == "--dangerously-skip-permissions" {
				t.Fatal("BuildCommand() should not include --dangerously-skip-permissions in PLAN phase")
			}
		}
	})

	t.Run("non-interactive implement phase does not enable plan mode", func(t *testing.T) {
		session := &agent.Session{
			Repository:  "github.com/org/repo",
			Tasks:       []string{"1"},
			Interactive: false,
			IterationContext: &agent.IterationContext{
				Phase: "IMPLEMENT",
			},
		}

		cmd := a.BuildCommand(session, 1)
		for _, arg := range cmd {
			if arg == "--permission-mode" {
				t.Fatal("BuildCommand() should not include --permission-mode outside PLAN phase")
			}
		}
	})
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
				"prefix based on issue labels",
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
		"<prefix>/issue-<number>",
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
			stdout:            wrapInStreamJSON("Running tests...\nAGENTIUM_STATUS: TESTS_PASSED\nAll tests passed"),
			stderr:            "",
			wantAgentStatus:   "TESTS_PASSED",
			wantStatusMessage: "",
			wantPushedChanges: false,
			wantSuccess:       true,
		},
		{
			name:              "TESTS_FAILED status with message",
			exitCode:          1,
			stdout:            wrapInStreamJSON("Running tests...\nAGENTIUM_STATUS: TESTS_FAILED 3 tests failed in auth module"),
			stderr:            "",
			wantAgentStatus:   "TESTS_FAILED",
			wantStatusMessage: "3 tests failed in auth module",
			wantPushedChanges: false,
			wantSuccess:       false,
		},
		{
			name:              "PR_CREATED status with URL",
			exitCode:          0,
			stdout:            wrapInStreamJSON("Creating PR...\nAGENTIUM_STATUS: PR_CREATED https://github.com/org/repo/pull/42"),
			stderr:            "",
			wantAgentStatus:   "PR_CREATED",
			wantStatusMessage: "https://github.com/org/repo/pull/42",
			wantPushedChanges: true,
			wantSuccess:       true,
		},
		{
			name:              "PUSHED status",
			exitCode:          0,
			stdout:            wrapInStreamJSON("Pushing changes...\nAGENTIUM_STATUS: PUSHED"),
			stderr:            "",
			wantAgentStatus:   "PUSHED",
			wantStatusMessage: "",
			wantPushedChanges: true,
			wantSuccess:       true,
		},
		{
			name:              "COMPLETE status",
			exitCode:          0,
			stdout:            wrapInStreamJSON("All work done\nAGENTIUM_STATUS: COMPLETE"),
			stderr:            "",
			wantAgentStatus:   "COMPLETE",
			wantStatusMessage: "",
			wantPushedChanges: true,
			wantSuccess:       true,
		},
		{
			name:              "NOTHING_TO_DO status",
			exitCode:          0,
			stdout:            wrapInStreamJSON("Reviewed feedback, no changes needed\nAGENTIUM_STATUS: NOTHING_TO_DO"),
			stderr:            "",
			wantAgentStatus:   "NOTHING_TO_DO",
			wantStatusMessage: "",
			wantPushedChanges: false,
			wantSuccess:       true,
		},
		{
			name:              "BLOCKED status with reason",
			exitCode:          0,
			stdout:            wrapInStreamJSON("AGENTIUM_STATUS: BLOCKED need clarification on requirements"),
			stderr:            "",
			wantAgentStatus:   "BLOCKED",
			wantStatusMessage: "need clarification on requirements",
			wantPushedChanges: false,
			wantSuccess:       true,
		},
		{
			name:              "multiple statuses takes last one",
			exitCode:          0,
			stdout:            wrapInStreamJSON("AGENTIUM_STATUS: TESTS_RUNNING\nRunning...\nAGENTIUM_STATUS: TESTS_PASSED\nDone"),
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
			stdout:            wrapInStreamJSON("Just some normal output without status"),
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
			stdout:      wrapInStreamJSON("Created PR #42 for the fix\nhttps://github.com/org/repo/pull/42"),
			stderr:      "",
			wantSuccess: true,
			wantPRs:     []string{"42"},
		},
		{
			name:        "successful run with multiple PRs",
			exitCode:    0,
			stdout:      wrapInStreamJSON("Created pull request #10\nOpened PR #20\nhttps://github.com/org/repo/pull/30"),
			stderr:      "",
			wantSuccess: true,
			wantPRs:     []string{"10", "20", "30"},
		},
		{
			name:        "detects completed tasks",
			exitCode:    0,
			stdout:      wrapInStreamJSON("Fixes #12\nCloses #17\nresolves #24"),
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
			stdout:      wrapInStreamJSON("Done! See https://github.com/org/repo/pull/99"),
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

func TestAdapter_ParseOutput_PRDetectionRegex(t *testing.T) {
	a := New()

	tests := []struct {
		name    string
		stdout  string
		wantPRs []string
	}{
		{
			name:    "Created pull request detected",
			stdout:  wrapInStreamJSON("Created pull request #110"),
			wantPRs: []string{"110"},
		},
		{
			name:    "Opened PR detected",
			stdout:  wrapInStreamJSON("Opened PR #42"),
			wantPRs: []string{"42"},
		},
		{
			name:    "bare PR reference not detected",
			stdout:  wrapInStreamJSON("This PR closes #24"),
			wantPRs: nil,
		},
		{
			name:    "issue reference after PR keyword not detected",
			stdout:  wrapInStreamJSON("PR fixes #9\nUpdated PR for issue #24"),
			wantPRs: nil,
		},
		{
			name:    "PR URL still detected",
			stdout:  wrapInStreamJSON("See https://github.com/org/repo/pull/110"),
			wantPRs: []string{"110"},
		},
		{
			name:    "duplicate PRs deduplicated",
			stdout:  wrapInStreamJSON("Created PR #110\nCreated pull request #110\nhttps://github.com/org/repo/pull/110"),
			wantPRs: []string{"110"},
		},
		{
			name:    "mixed real and issue refs - only real PRs",
			stdout:  wrapInStreamJSON("Created PR #110 that closes #24\nPR references issue #9"),
			wantPRs: []string{"110"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := a.ParseOutput(0, tt.stdout, "")
			if err != nil {
				t.Fatalf("ParseOutput() returned error: %v", err)
			}

			if len(tt.wantPRs) == 0 {
				if len(result.PRsCreated) != 0 {
					t.Errorf("PRsCreated = %v, want empty", result.PRsCreated)
				}
			} else {
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
		})
	}
}

func TestAdapter_ParseOutput_TokenUsage(t *testing.T) {
	a := New()

	t.Run("populates InputTokens and OutputTokens from result event", func(t *testing.T) {
		stdout := `{"type":"result","result":{"content":[{"type":"text","text":"Done."}],"usage":{"input_tokens":1500,"output_tokens":300},"stop_reason":"end_turn"}}` + "\n"

		result, err := a.ParseOutput(0, stdout, "")
		if err != nil {
			t.Fatalf("ParseOutput() returned error: %v", err)
		}

		if result.InputTokens != 1500 {
			t.Errorf("InputTokens = %d, want %d", result.InputTokens, 1500)
		}
		if result.OutputTokens != 300 {
			t.Errorf("OutputTokens = %d, want %d", result.OutputTokens, 300)
		}
		if result.TokensUsed != 1800 {
			t.Errorf("TokensUsed = %d, want %d", result.TokensUsed, 1800)
		}
	})

	t.Run("zero tokens when no result event", func(t *testing.T) {
		stdout := wrapInStreamJSON("Just some text without token info")

		result, err := a.ParseOutput(0, stdout, "")
		if err != nil {
			t.Fatalf("ParseOutput() returned error: %v", err)
		}

		if result.InputTokens != 0 {
			t.Errorf("InputTokens = %d, want 0", result.InputTokens)
		}
		if result.OutputTokens != 0 {
			t.Errorf("OutputTokens = %d, want 0", result.OutputTokens)
		}
		if result.TokensUsed != 0 {
			t.Errorf("TokensUsed = %d, want 0", result.TokensUsed)
		}
	})
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

func TestAdapter_GetStdinPrompt(t *testing.T) {
	a := New()

	t.Run("non-interactive mode returns prompt for stdin", func(t *testing.T) {
		session := &agent.Session{
			Repository:  "github.com/org/repo",
			Tasks:       []string{"1"},
			Interactive: false,
		}

		prompt := a.GetStdinPrompt(session, 1)

		if prompt == "" {
			t.Error("GetStdinPrompt() returned empty string for non-interactive mode")
		}

		if !strings.Contains(prompt, "github.com/org/repo") {
			t.Errorf("GetStdinPrompt() missing repository in prompt: %q", prompt)
		}
	})

	t.Run("interactive mode returns empty string", func(t *testing.T) {
		session := &agent.Session{
			Repository:  "github.com/org/repo",
			Tasks:       []string{"1"},
			Interactive: true,
		}

		prompt := a.GetStdinPrompt(session, 1)

		if prompt != "" {
			t.Errorf("GetStdinPrompt() = %q, want empty string for interactive mode", prompt)
		}
	})

	t.Run("implements StdinPromptProvider interface", func(t *testing.T) {
		var _ agent.StdinPromptProvider = a
	})

	t.Run("implements PlanModeCapable interface", func(t *testing.T) {
		var _ agent.PlanModeCapable = a
	})

	t.Run("implements ContinuationCapable interface", func(t *testing.T) {
		var _ agent.ContinuationCapable = a
	})
}

func TestAdapter_SupportsContinuation(t *testing.T) {
	a := New()
	if !a.SupportsContinuation() {
		t.Error("SupportsContinuation() should return true")
	}
}

func TestAdapter_BuildContinueCommand(t *testing.T) {
	a := New()

	t.Run("non-interactive implement mode", func(t *testing.T) {
		session := &agent.Session{
			Repository:  "github.com/org/repo",
			Interactive: false,
			IterationContext: &agent.IterationContext{
				Phase: "IMPLEMENT",
			},
		}

		cmd := a.BuildContinueCommand(session, 2)

		// Should have: --print, --verbose, --output-format, stream-json, --dangerously-skip-permissions, --continue
		if len(cmd) != 6 {
			t.Fatalf("BuildContinueCommand() returned %d args, want 6: %v", len(cmd), cmd)
		}

		// Verify --continue is present
		hasFlag := false
		for _, arg := range cmd {
			if arg == "--continue" {
				hasFlag = true
			}
		}
		if !hasFlag {
			t.Error("BuildContinueCommand() should include --continue flag")
		}

		// Verify no --system-prompt or --append-system-prompt
		for _, arg := range cmd {
			if arg == "--system-prompt" {
				t.Error("BuildContinueCommand() should not include --system-prompt")
			}
			if arg == "--append-system-prompt" {
				t.Error("BuildContinueCommand() should not include --append-system-prompt")
			}
		}
	})

	t.Run("plan mode includes --continue", func(t *testing.T) {
		session := &agent.Session{
			Repository:  "github.com/org/repo",
			Interactive: false,
			IterationContext: &agent.IterationContext{
				Phase: "PLAN",
			},
		}

		cmd := a.BuildContinueCommand(session, 2)

		hasFlag := false
		hasPlanMode := false
		for i, arg := range cmd {
			if arg == "--continue" {
				hasFlag = true
			}
			if arg == "--permission-mode" && i+1 < len(cmd) && cmd[i+1] == "plan" {
				hasPlanMode = true
			}
		}
		if !hasFlag {
			t.Error("BuildContinueCommand() should include --continue in PLAN mode")
		}
		if !hasPlanMode {
			t.Error("BuildContinueCommand() should include --permission-mode plan in PLAN mode")
		}

		// Verify no --dangerously-skip-permissions in plan mode
		for _, arg := range cmd {
			if arg == "--dangerously-skip-permissions" {
				t.Error("BuildContinueCommand() should not include --dangerously-skip-permissions in PLAN mode")
			}
		}
	})

	t.Run("with model override", func(t *testing.T) {
		session := &agent.Session{
			Repository:  "github.com/org/repo",
			Interactive: false,
			IterationContext: &agent.IterationContext{
				Phase:         "IMPLEMENT",
				ModelOverride: "claude-sonnet-4-5-20250929",
			},
		}

		cmd := a.BuildContinueCommand(session, 2)

		hasModel := false
		for i, arg := range cmd {
			if arg == "--model" && i+1 < len(cmd) && cmd[i+1] == "claude-sonnet-4-5-20250929" {
				hasModel = true
			}
		}
		if !hasModel {
			t.Error("BuildContinueCommand() should include --model flag with override")
		}
	})

	t.Run("interactive mode falls back to BuildCommand", func(t *testing.T) {
		session := &agent.Session{
			Repository:  "github.com/org/repo",
			Interactive: true,
		}

		cmd := a.BuildContinueCommand(session, 2)
		origCmd := a.BuildCommand(session, 2)

		if len(cmd) != len(origCmd) {
			t.Errorf("Interactive BuildContinueCommand() should match BuildCommand(): got %d args, want %d", len(cmd), len(origCmd))
		}
	})
}
