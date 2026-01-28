package codex

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/andywolf/agentium/internal/agent"
)

// makeJSONL builds a JSONL string from CodexEvent structs
func makeJSONL(events ...CodexEvent) string {
	var lines []string
	for _, e := range events {
		b, _ := json.Marshal(e)
		lines = append(lines, string(b))
	}
	return strings.Join(lines, "\n") + "\n"
}

func TestAdapter_Name(t *testing.T) {
	a := New()
	if got := a.Name(); got != "codex" {
		t.Errorf("Name() = %q, want %q", got, "codex")
	}
}

func TestAdapter_ContainerImage(t *testing.T) {
	a := New()
	if got := a.ContainerImage(); got != DefaultImage {
		t.Errorf("ContainerImage() = %q, want %q", got, DefaultImage)
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
	a, err := agent.Get("codex")
	if err != nil {
		t.Fatalf("Get(codex) returned error: %v", err)
	}

	if a.Name() != "codex" {
		t.Errorf("Registered agent Name() = %q, want %q", a.Name(), "codex")
	}
}

func TestAdapter_BuildEnv(t *testing.T) {
	a := New()

	t.Run("standard variables", func(t *testing.T) {
		session := &agent.Session{
			ID:          "test-session",
			Repository:  "github.com/org/repo",
			GitHubToken: "ghp_token123",
			Metadata:    map[string]string{},
		}

		env := a.BuildEnv(session, 1)

		expected := map[string]string{
			"GITHUB_TOKEN":        "ghp_token123",
			"AGENTIUM_SESSION_ID": "test-session",
			"AGENTIUM_ITERATION":  "1",
			"AGENTIUM_REPOSITORY": "github.com/org/repo",
			"AGENTIUM_WORKDIR":    "/workspace",
		}

		for k, want := range expected {
			if got := env[k]; got != want {
				t.Errorf("env[%q] = %q, want %q", k, got, want)
			}
		}
	})

	t.Run("CODEX_API_KEY from metadata", func(t *testing.T) {
		session := &agent.Session{
			ID:         "test-session",
			Repository: "github.com/org/repo",
			Metadata: map[string]string{
				"codex_api_key": "codex-key-123",
			},
		}

		env := a.BuildEnv(session, 1)

		if got := env["CODEX_API_KEY"]; got != "codex-key-123" {
			t.Errorf("env[CODEX_API_KEY] = %q, want %q", got, "codex-key-123")
		}
		if _, ok := env["OPENAI_API_KEY"]; ok {
			t.Error("OPENAI_API_KEY should not be set when CODEX_API_KEY is present")
		}
	})

	t.Run("OPENAI_API_KEY fallback", func(t *testing.T) {
		session := &agent.Session{
			ID:         "test-session",
			Repository: "github.com/org/repo",
			Metadata: map[string]string{
				"openai_api_key": "openai-key-456",
			},
		}

		env := a.BuildEnv(session, 1)

		if got := env["OPENAI_API_KEY"]; got != "openai-key-456" {
			t.Errorf("env[OPENAI_API_KEY] = %q, want %q", got, "openai-key-456")
		}
		if _, ok := env["CODEX_API_KEY"]; ok {
			t.Error("CODEX_API_KEY should not be set when only openai_api_key is in metadata")
		}
	})

	t.Run("sensitive key filtering", func(t *testing.T) {
		session := &agent.Session{
			ID:         "test-session",
			Repository: "github.com/org/repo",
			Metadata: map[string]string{
				"codex_api_key":  "secret-key",
				"openai_api_key": "another-secret",
				"my_secret":      "hidden",
				"auth_token":     "also-hidden",
				"custom_setting": "visible",
			},
		}

		env := a.BuildEnv(session, 1)

		// Sensitive keys should NOT be passed through as AGENTIUM_*
		sensitiveKeys := []string{
			"AGENTIUM_CODEX_API_KEY",
			"AGENTIUM_OPENAI_API_KEY",
			"AGENTIUM_MY_SECRET",
			"AGENTIUM_AUTH_TOKEN",
		}
		for _, k := range sensitiveKeys {
			if _, ok := env[k]; ok {
				t.Errorf("sensitive key %q should not be in env", k)
			}
		}

		// Non-sensitive keys should be passed through
		if got := env["AGENTIUM_CUSTOM_SETTING"]; got != "visible" {
			t.Errorf("env[AGENTIUM_CUSTOM_SETTING] = %q, want %q", got, "visible")
		}
	})
}

func TestAdapter_BuildCommand(t *testing.T) {
	a := New()

	t.Run("basic command structure", func(t *testing.T) {
		session := &agent.Session{
			Repository: "github.com/org/repo",
			Tasks:      []string{"1"},
			Metadata:   map[string]string{},
		}

		cmd := a.BuildCommand(session, 1)

		// Must contain exec, --json, --yolo, --skip-git-repo-check
		requiredArgs := []string{"exec", "--json", "--yolo", "--skip-git-repo-check"}
		for _, required := range requiredArgs {
			found := false
			for _, arg := range cmd {
				if arg == required {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("BuildCommand() missing required arg %q in %v", required, cmd)
			}
		}

		// Must contain --cd /workspace
		foundCD := false
		for i, arg := range cmd {
			if arg == "--cd" && i+1 < len(cmd) && cmd[i+1] == "/workspace" {
				foundCD = true
				break
			}
		}
		if !foundCD {
			t.Errorf("BuildCommand() missing --cd /workspace in %v", cmd)
		}
	})

	t.Run("model override from IterationContext", func(t *testing.T) {
		session := &agent.Session{
			Repository: "github.com/org/repo",
			Tasks:      []string{"1"},
			Metadata:   map[string]string{},
			IterationContext: &agent.IterationContext{
				ModelOverride: "o3-mini",
			},
		}

		cmd := a.BuildCommand(session, 1)

		var modelValue string
		for i, arg := range cmd {
			if arg == "--model" && i+1 < len(cmd) {
				modelValue = cmd[i+1]
				break
			}
		}
		if modelValue != "o3-mini" {
			t.Errorf("--model = %q, want %q", modelValue, "o3-mini")
		}
	})

	t.Run("model override from metadata", func(t *testing.T) {
		session := &agent.Session{
			Repository: "github.com/org/repo",
			Tasks:      []string{"1"},
			Metadata: map[string]string{
				"codex_model": "gpt-4o",
			},
		}

		cmd := a.BuildCommand(session, 1)

		var modelValue string
		for i, arg := range cmd {
			if arg == "--model" && i+1 < len(cmd) {
				modelValue = cmd[i+1]
				break
			}
		}
		if modelValue != "gpt-4o" {
			t.Errorf("--model = %q, want %q", modelValue, "gpt-4o")
		}
	})

	t.Run("no model flag when not set", func(t *testing.T) {
		session := &agent.Session{
			Repository: "github.com/org/repo",
			Tasks:      []string{"1"},
			Metadata:   map[string]string{},
		}

		cmd := a.BuildCommand(session, 1)

		for _, arg := range cmd {
			if arg == "--model" {
				t.Error("--model flag should not be present when no model is set")
				break
			}
		}
	})

	t.Run("developer_instructions included", func(t *testing.T) {
		session := &agent.Session{
			Repository:   "github.com/org/repo",
			Tasks:        []string{"1"},
			SystemPrompt: "Be helpful",
			Metadata:     map[string]string{},
		}

		cmd := a.BuildCommand(session, 1)

		// Find -c flag
		var configValue string
		for i, arg := range cmd {
			if arg == "-c" && i+1 < len(cmd) {
				configValue = cmd[i+1]
				break
			}
		}

		if !strings.HasPrefix(configValue, "developer_instructions=") {
			t.Errorf("expected -c developer_instructions=..., got %q", configValue)
		}
		if !strings.Contains(configValue, "Be helpful") {
			t.Errorf("developer_instructions should contain system prompt")
		}
		if !strings.Contains(configValue, "AGENTIUM_STATUS") {
			t.Errorf("developer_instructions should contain status signal instructions")
		}
	})

	t.Run("developer_instructions escapes newlines", func(t *testing.T) {
		session := &agent.Session{
			Repository:   "github.com/org/repo",
			Tasks:        []string{"1"},
			SystemPrompt: "Line one\nLine two\nLine three",
			Metadata:     map[string]string{},
		}

		cmd := a.BuildCommand(session, 1)

		var configValue string
		for i, arg := range cmd {
			if arg == "-c" && i+1 < len(cmd) {
				configValue = cmd[i+1]
				break
			}
		}

		// Should not contain raw newlines
		instructionsValue := strings.TrimPrefix(configValue, "developer_instructions=")
		if strings.Contains(instructionsValue, "\n") {
			t.Errorf("developer_instructions should not contain raw newlines, got: %q", instructionsValue)
		}
		// Should contain escaped \n sequences
		if !strings.Contains(instructionsValue, `\n`) {
			t.Errorf("developer_instructions should contain escaped \\n, got: %q", instructionsValue)
		}
	})

	t.Run("developer_instructions escapes backslashes", func(t *testing.T) {
		session := &agent.Session{
			Repository:   "github.com/org/repo",
			Tasks:        []string{"1"},
			SystemPrompt: `path\to\file`,
			Metadata:     map[string]string{},
		}

		cmd := a.BuildCommand(session, 1)

		var configValue string
		for i, arg := range cmd {
			if arg == "-c" && i+1 < len(cmd) {
				configValue = cmd[i+1]
				break
			}
		}

		if !strings.Contains(configValue, `path\\to\\file`) {
			t.Errorf("developer_instructions should escape backslashes, got: %q", configValue)
		}
	})

	t.Run("SkillsPrompt overrides SystemPrompt", func(t *testing.T) {
		session := &agent.Session{
			Repository:   "github.com/org/repo",
			Tasks:        []string{"1"},
			SystemPrompt: "monolithic system prompt",
			Metadata:     map[string]string{},
			IterationContext: &agent.IterationContext{
				SkillsPrompt: "composed skills prompt",
			},
		}

		cmd := a.BuildCommand(session, 1)

		var configValue string
		for i, arg := range cmd {
			if arg == "-c" && i+1 < len(cmd) {
				configValue = cmd[i+1]
				break
			}
		}

		if strings.Contains(configValue, "monolithic system prompt") {
			t.Error("developer_instructions should not contain SystemPrompt when SkillsPrompt is set")
		}
		if !strings.Contains(configValue, "composed skills prompt") {
			t.Error("developer_instructions should contain SkillsPrompt")
		}
	})
}

func TestAdapter_BuildPrompt(t *testing.T) {
	a := New()

	tests := []struct {
		name        string
		session     *agent.Session
		iteration   int
		contains    []string
		notContains []string
	}{
		{
			name: "active task uses prompt directly",
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
			notContains: []string{
				"Create a new branch",
				"For each issue",
			},
		},
		{
			name: "active task with memory context",
			session: &agent.Session{
				Repository: "github.com/org/repo",
				Tasks:      []string{"6"},
				ActiveTask: "6",
				Prompt:     "Fix the bug",
				IterationContext: &agent.IterationContext{
					MemoryContext: "Previous iteration: created branch",
				},
			},
			iteration: 2,
			contains: []string{
				"Fix the bug",
				"Previous iteration: created branch",
			},
		},
		{
			name: "generic multi-issue mode",
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
			name: "custom prompt in legacy mode",
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
			name: "iteration continuation",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt := a.BuildPrompt(tt.session, tt.iteration)
			for _, substr := range tt.contains {
				if !strings.Contains(prompt, substr) {
					t.Errorf("BuildPrompt() missing %q in:\n%s", substr, prompt)
				}
			}
			for _, substr := range tt.notContains {
				if strings.Contains(prompt, substr) {
					t.Errorf("BuildPrompt() should not contain %q in:\n%s", substr, prompt)
				}
			}
		})
	}
}

func TestAdapter_ParseOutput_JSONL(t *testing.T) {
	a := New()

	t.Run("agent message extraction", func(t *testing.T) {
		stdout := makeJSONL(
			CodexEvent{
				Type: "item.completed",
				Item: &EventItem{Type: "agent_message", Text: "I fixed the bug"},
			},
		)

		result, err := a.ParseOutput(0, stdout, "")
		if err != nil {
			t.Fatalf("ParseOutput() returned error: %v", err)
		}

		if !strings.Contains(result.RawTextContent, "I fixed the bug") {
			t.Errorf("RawTextContent = %q, want to contain %q", result.RawTextContent, "I fixed the bug")
		}
	})

	t.Run("command execution output", func(t *testing.T) {
		stdout := makeJSONL(
			CodexEvent{
				Type: "item.completed",
				Item: &EventItem{Type: "command_execution", Command: "git push", Output: "Everything up-to-date"},
			},
		)

		result, err := a.ParseOutput(0, stdout, "")
		if err != nil {
			t.Fatalf("ParseOutput() returned error: %v", err)
		}

		if !strings.Contains(result.RawTextContent, "Everything up-to-date") {
			t.Errorf("RawTextContent should contain command output")
		}
	})

	t.Run("file change detection", func(t *testing.T) {
		stdout := makeJSONL(
			CodexEvent{
				Type: "item.completed",
				Item: &EventItem{Type: "file_change", FilePath: "src/main.go", Action: "modified"},
			},
			CodexEvent{
				Type: "item.completed",
				Item: &EventItem{Type: "file_change", FilePath: "src/util.go", Action: "created"},
			},
		)

		result, err := a.ParseOutput(0, stdout, "")
		if err != nil {
			t.Fatalf("ParseOutput() returned error: %v", err)
		}

		if !strings.Contains(result.Summary, "2 file(s)") {
			t.Errorf("Summary = %q, want to contain file count", result.Summary)
		}
	})

	t.Run("token usage from turn.completed", func(t *testing.T) {
		stdout := makeJSONL(
			CodexEvent{
				Type:  "turn.completed",
				Usage: &usage{InputTokens: 1000, OutputTokens: 500, CachedInputTokens: 200},
			},
			CodexEvent{
				Type:  "turn.completed",
				Usage: &usage{InputTokens: 800, OutputTokens: 300, CachedInputTokens: 100},
			},
		)

		result, err := a.ParseOutput(0, stdout, "")
		if err != nil {
			t.Fatalf("ParseOutput() returned error: %v", err)
		}

		expectedInput := 1000 + 800
		expectedOutput := 500 + 300
		expectedTokens := expectedInput + expectedOutput
		if result.InputTokens != expectedInput {
			t.Errorf("InputTokens = %d, want %d", result.InputTokens, expectedInput)
		}
		if result.OutputTokens != expectedOutput {
			t.Errorf("OutputTokens = %d, want %d", result.OutputTokens, expectedOutput)
		}
		if result.TokensUsed != expectedTokens {
			t.Errorf("TokensUsed = %d, want %d", result.TokensUsed, expectedTokens)
		}
	})

	t.Run("item.delta event with delta field", func(t *testing.T) {
		stdout := makeJSONL(
			CodexEvent{
				Type:  "item.delta",
				Delta: &eventDelta{Text: "Hello "},
			},
			CodexEvent{
				Type:  "item.delta",
				Delta: &eventDelta{Text: "world"},
			},
		)

		result, err := a.ParseOutput(0, stdout, "")
		if err != nil {
			t.Fatalf("ParseOutput() returned error: %v", err)
		}

		if !strings.Contains(result.RawTextContent, "Hello ") || !strings.Contains(result.RawTextContent, "world") {
			t.Errorf("RawTextContent = %q, want to contain delta text fragments", result.RawTextContent)
		}
	})

	t.Run("response.output_text.delta event", func(t *testing.T) {
		stdout := makeJSONL(
			CodexEvent{
				Type:  "response.output_text.delta",
				Delta: &eventDelta{Text: "streaming text here"},
			},
		)

		result, err := a.ParseOutput(0, stdout, "")
		if err != nil {
			t.Fatalf("ParseOutput() returned error: %v", err)
		}

		if !strings.Contains(result.RawTextContent, "streaming text here") {
			t.Errorf("RawTextContent = %q, want to contain delta text", result.RawTextContent)
		}
	})

	t.Run("delta event with item text fallback", func(t *testing.T) {
		stdout := makeJSONL(
			CodexEvent{
				Type: "item.delta",
				Item: &EventItem{Type: "agent_message", Text: "fallback text"},
			},
		)

		result, err := a.ParseOutput(0, stdout, "")
		if err != nil {
			t.Fatalf("ParseOutput() returned error: %v", err)
		}

		if !strings.Contains(result.RawTextContent, "fallback text") {
			t.Errorf("RawTextContent = %q, want to contain item text from delta event", result.RawTextContent)
		}
	})

	t.Run("status signals detected from delta events", func(t *testing.T) {
		stdout := makeJSONL(
			CodexEvent{
				Type:  "item.delta",
				Delta: &eventDelta{Text: "Working on it...\nAGENTIUM_STATUS: COMPLETE"},
			},
		)

		result, err := a.ParseOutput(0, stdout, "")
		if err != nil {
			t.Fatalf("ParseOutput() returned error: %v", err)
		}

		if result.AgentStatus != "COMPLETE" {
			t.Errorf("AgentStatus = %q, want %q", result.AgentStatus, "COMPLETE")
		}
		if !result.PushedChanges {
			t.Error("PushedChanges should be true for COMPLETE status")
		}
	})
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
			name:     "TESTS_PASSED status",
			exitCode: 0,
			stdout: makeJSONL(CodexEvent{
				Type: "item.completed",
				Item: &EventItem{Type: "agent_message", Text: "Running tests...\nAGENTIUM_STATUS: TESTS_PASSED\nAll tests passed"},
			}),
			wantAgentStatus:   "TESTS_PASSED",
			wantPushedChanges: false,
			wantSuccess:       true,
		},
		{
			name:     "TESTS_FAILED with message",
			exitCode: 1,
			stdout: makeJSONL(CodexEvent{
				Type: "item.completed",
				Item: &EventItem{Type: "agent_message", Text: "AGENTIUM_STATUS: TESTS_FAILED 3 tests failed in auth module"},
			}),
			wantAgentStatus:   "TESTS_FAILED",
			wantStatusMessage: "3 tests failed in auth module",
			wantPushedChanges: false,
			wantSuccess:       false,
		},
		{
			name:     "PR_CREATED with URL",
			exitCode: 0,
			stdout: makeJSONL(CodexEvent{
				Type: "item.completed",
				Item: &EventItem{Type: "agent_message", Text: "AGENTIUM_STATUS: PR_CREATED https://github.com/org/repo/pull/42"},
			}),
			wantAgentStatus:   "PR_CREATED",
			wantStatusMessage: "https://github.com/org/repo/pull/42",
			wantPushedChanges: true,
			wantSuccess:       true,
		},
		{
			name:     "PUSHED status",
			exitCode: 0,
			stdout: makeJSONL(CodexEvent{
				Type: "item.completed",
				Item: &EventItem{Type: "agent_message", Text: "AGENTIUM_STATUS: PUSHED"},
			}),
			wantAgentStatus:   "PUSHED",
			wantPushedChanges: true,
			wantSuccess:       true,
		},
		{
			name:     "COMPLETE status",
			exitCode: 0,
			stdout: makeJSONL(CodexEvent{
				Type: "item.completed",
				Item: &EventItem{Type: "agent_message", Text: "AGENTIUM_STATUS: COMPLETE"},
			}),
			wantAgentStatus:   "COMPLETE",
			wantPushedChanges: true,
			wantSuccess:       true,
		},
		{
			name:     "NOTHING_TO_DO status",
			exitCode: 0,
			stdout: makeJSONL(CodexEvent{
				Type: "item.completed",
				Item: &EventItem{Type: "agent_message", Text: "AGENTIUM_STATUS: NOTHING_TO_DO"},
			}),
			wantAgentStatus:   "NOTHING_TO_DO",
			wantPushedChanges: false,
			wantSuccess:       true,
		},
		{
			name:     "BLOCKED with reason",
			exitCode: 0,
			stdout: makeJSONL(CodexEvent{
				Type: "item.completed",
				Item: &EventItem{Type: "agent_message", Text: "AGENTIUM_STATUS: BLOCKED need clarification on requirements"},
			}),
			wantAgentStatus:   "BLOCKED",
			wantStatusMessage: "need clarification on requirements",
			wantPushedChanges: false,
			wantSuccess:       true,
		},
		{
			name:     "multiple statuses takes last one",
			exitCode: 0,
			stdout: makeJSONL(
				CodexEvent{
					Type: "item.completed",
					Item: &EventItem{Type: "agent_message", Text: "AGENTIUM_STATUS: TESTS_RUNNING"},
				},
				CodexEvent{
					Type: "item.completed",
					Item: &EventItem{Type: "agent_message", Text: "AGENTIUM_STATUS: TESTS_PASSED"},
				},
			),
			wantAgentStatus:   "TESTS_PASSED",
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

func TestAdapter_ParseOutput_PRDetection(t *testing.T) {
	a := New()

	tests := []struct {
		name    string
		stdout  string
		wantPRs []string
	}{
		{
			name: "PR URL in agent message",
			stdout: makeJSONL(CodexEvent{
				Type: "item.completed",
				Item: &EventItem{Type: "agent_message", Text: "Done! See https://github.com/org/repo/pull/99"},
			}),
			wantPRs: []string{"99"},
		},
		{
			name: "Created pull request text",
			stdout: makeJSONL(CodexEvent{
				Type: "item.completed",
				Item: &EventItem{Type: "agent_message", Text: "Created pull request #42"},
			}),
			wantPRs: []string{"42"},
		},
		{
			name: "Opened PR text",
			stdout: makeJSONL(CodexEvent{
				Type: "item.completed",
				Item: &EventItem{Type: "agent_message", Text: "Opened PR #55"},
			}),
			wantPRs: []string{"55"},
		},
		{
			name: "bare PR reference not detected",
			stdout: makeJSONL(CodexEvent{
				Type: "item.completed",
				Item: &EventItem{Type: "agent_message", Text: "This PR closes #24"},
			}),
			wantPRs: nil,
		},
		{
			name: "duplicate PRs deduplicated",
			stdout: makeJSONL(CodexEvent{
				Type: "item.completed",
				Item: &EventItem{Type: "agent_message", Text: "Created PR #110\nhttps://github.com/org/repo/pull/110"},
			}),
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

func TestAdapter_ParseOutput_Errors(t *testing.T) {
	a := New()

	t.Run("turn.failed event", func(t *testing.T) {
		stdout := makeJSONL(CodexEvent{
			Type:  "turn.failed",
			Error: &eventError{Message: "API rate limit exceeded"},
		})

		result, err := a.ParseOutput(1, stdout, "")
		if err != nil {
			t.Fatalf("ParseOutput() returned error: %v", err)
		}

		if result.Success {
			t.Error("Success should be false for exit code 1")
		}
		if result.Error != "API rate limit exceeded" {
			t.Errorf("Error = %q, want %q", result.Error, "API rate limit exceeded")
		}
	})

	t.Run("error event", func(t *testing.T) {
		stdout := makeJSONL(CodexEvent{
			Type:  "error",
			Error: &eventError{Message: "Connection timeout"},
		})

		result, err := a.ParseOutput(1, stdout, "")
		if err != nil {
			t.Fatalf("ParseOutput() returned error: %v", err)
		}

		if result.Error != "Connection timeout" {
			t.Errorf("Error = %q, want %q", result.Error, "Connection timeout")
		}
	})

	t.Run("non-zero exit with stderr", func(t *testing.T) {
		result, err := a.ParseOutput(1, "", "fatal: not a git repository")
		if err != nil {
			t.Fatalf("ParseOutput() returned error: %v", err)
		}

		if !strings.Contains(result.Error, "not a git repository") {
			t.Errorf("Error = %q, want to contain %q", result.Error, "not a git repository")
		}
	})

	t.Run("summary reflects failure", func(t *testing.T) {
		result, err := a.ParseOutput(1, "", "error: something went wrong")
		if err != nil {
			t.Fatalf("ParseOutput() returned error: %v", err)
		}

		if !strings.Contains(result.Summary, "failed") {
			t.Errorf("Summary = %q, want to contain 'failed'", result.Summary)
		}
	})
}

func TestAdapter_ParseOutput_EdgeCases(t *testing.T) {
	a := New()

	t.Run("malformed JSON lines skipped", func(t *testing.T) {
		stdout := "not json at all\n" + makeJSONL(CodexEvent{
			Type: "item.completed",
			Item: &EventItem{Type: "agent_message", Text: "valid output"},
		}) + "another bad line\n"

		result, err := a.ParseOutput(0, stdout, "")
		if err != nil {
			t.Fatalf("ParseOutput() returned error: %v", err)
		}

		if !result.Success {
			t.Error("Success should be true even with malformed lines")
		}
		if !strings.Contains(result.RawTextContent, "valid output") {
			t.Errorf("RawTextContent should contain valid parsed output")
		}
	})

	t.Run("empty stdout and stderr", func(t *testing.T) {
		result, err := a.ParseOutput(0, "", "")
		if err != nil {
			t.Fatalf("ParseOutput() returned error: %v", err)
		}

		if !result.Success {
			t.Error("Success should be true for exit code 0")
		}
		if result.Summary != "Iteration completed successfully" {
			t.Errorf("Summary = %q, want %q", result.Summary, "Iteration completed successfully")
		}
	})

	t.Run("task completion detection", func(t *testing.T) {
		stdout := makeJSONL(CodexEvent{
			Type: "item.completed",
			Item: &EventItem{Type: "agent_message", Text: "Fixes #12\nCloses #17"},
		})

		result, err := a.ParseOutput(0, stdout, "")
		if err != nil {
			t.Fatalf("ParseOutput() returned error: %v", err)
		}

		if len(result.TasksCompleted) != 2 {
			t.Errorf("TasksCompleted = %v, want 2 items", result.TasksCompleted)
		}
	})

	t.Run("git push detection", func(t *testing.T) {
		stdout := makeJSONL(CodexEvent{
			Type: "item.completed",
			Item: &EventItem{
				Type:   "command_execution",
				Output: "To github.com:org/repo.git\n   abc1234..def5678  main -> main",
			},
		})

		result, err := a.ParseOutput(0, stdout, "")
		if err != nil {
			t.Fatalf("ParseOutput() returned error: %v", err)
		}

		if !result.PushedChanges {
			t.Error("PushedChanges should be true when git push output is detected")
		}
	})

	t.Run("raw stdout fallback when no JSONL parsed", func(t *testing.T) {
		// Simulate output that is plain text (not JSONL at all)
		stdout := "Working on issue #12\nAGENTIUM_STATUS: COMPLETE\nCreated pull request #42\nhttps://github.com/org/repo/pull/42"

		result, err := a.ParseOutput(0, stdout, "")
		if err != nil {
			t.Fatalf("ParseOutput() returned error: %v", err)
		}

		// Status should be detected from raw stdout
		if result.AgentStatus != "COMPLETE" {
			t.Errorf("AgentStatus = %q, want %q (raw stdout fallback)", result.AgentStatus, "COMPLETE")
		}
		// PRs should be detected from raw stdout
		if len(result.PRsCreated) == 0 {
			t.Error("PRsCreated should be detected from raw stdout fallback")
		}
		if !result.PushedChanges {
			t.Error("PushedChanges should be true from COMPLETE status in raw fallback")
		}
	})

	t.Run("raw stdout fallback when JSONL parsed but no text extracted", func(t *testing.T) {
		// Simulate events that don't produce text (e.g., only file_change events)
		stdout := makeJSONL(
			CodexEvent{
				Type: "item.completed",
				Item: &EventItem{Type: "file_change", FilePath: "main.go", Action: "modified"},
			},
		) + "AGENTIUM_STATUS: PUSHED\n"

		result, err := a.ParseOutput(0, stdout, "")
		if err != nil {
			t.Fatalf("ParseOutput() returned error: %v", err)
		}

		// The non-JSON line "AGENTIUM_STATUS: PUSHED" should be picked up via raw fallback
		if result.AgentStatus != "PUSHED" {
			t.Errorf("AgentStatus = %q, want %q (fallback for non-text JSONL)", result.AgentStatus, "PUSHED")
		}
	})
}
