package aider

import (
	"strings"
	"testing"

	"github.com/andywolf/agentium/internal/agent"
)

func TestAdapter_Name(t *testing.T) {
	a := New()
	if got := a.Name(); got != "aider" {
		t.Errorf("Name() = %q, want %q", got, "aider")
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
			"anthropic_api_key": "sk-ant-api-key",
			"custom_key":        "custom_value",
		},
	}

	env := a.BuildEnv(session, 1)

	tests := []struct {
		key      string
		value    string
		required bool
	}{
		{"GITHUB_TOKEN", "ghp_token123", true},
		{"AGENTIUM_SESSION_ID", "test-session", true},
		{"AGENTIUM_ITERATION", "1", true},
		{"AGENTIUM_REPOSITORY", "github.com/org/repo", true},
		{"AGENTIUM_WORKDIR", "/workspace", true},
		{"ANTHROPIC_API_KEY", "sk-ant-api-key", true},
		{"AGENTIUM_CUSTOM_KEY", "custom_value", true},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := env[tt.key]
			if tt.required && got != tt.value {
				t.Errorf("env[%q] = %q, want %q", tt.key, got, tt.value)
			}
		})
	}

	// API keys should not be duplicated in AGENTIUM_ prefix
	if _, exists := env["AGENTIUM_ANTHROPIC_API_KEY"]; exists {
		t.Error("API key should not be duplicated with AGENTIUM_ prefix")
	}
}

func TestAdapter_BuildCommand(t *testing.T) {
	a := New()
	session := &agent.Session{
		Repository: "github.com/org/repo",
		Tasks:      []string{"1", "2"},
	}

	cmd := a.BuildCommand(session, 1)

	if len(cmd) < 1 {
		t.Fatalf("BuildCommand() returned empty command")
	}

	if cmd[0] != "--model" {
		t.Errorf("BuildCommand()[0] = %q, want %q", cmd[0], "--model")
	}

	// Check for expected flags
	hasModel := false
	hasYesAlways := false
	hasMessage := false

	for i, arg := range cmd {
		if arg == "--model" {
			hasModel = true
		}
		if arg == "--yes-always" {
			hasYesAlways = true
		}
		if arg == "--message" && i+1 < len(cmd) {
			hasMessage = true
		}
	}

	if !hasModel {
		t.Error("BuildCommand() missing --model flag")
	}
	if !hasYesAlways {
		t.Error("BuildCommand() missing --yes-always flag")
	}
	if !hasMessage {
		t.Error("BuildCommand() missing --message flag")
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
			},
		},
		{
			name: "custom prompt",
			session: &agent.Session{
				Repository: "github.com/org/repo",
				Tasks:      []string{"1"},
				Prompt:     "Custom instructions for aider",
			},
			iteration: 1,
			contains: []string{
				"Custom instructions for aider",
				"Issue #1",
			},
		},
		{
			name: "iteration > 1",
			session: &agent.Session{
				Repository: "github.com/org/repo",
				Tasks:      []string{"1"},
			},
			iteration: 5,
			contains: []string{
				"iteration 5",
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
		wantSummary    string
		wantErrContain string
	}{
		{
			name:        "successful run with file changes",
			exitCode:    0,
			stdout:      "Wrote src/main.go\nUpdated tests/main_test.go\nModified README.md",
			stderr:      "",
			wantSuccess: true,
			wantSummary: "Modified 3 file(s)",
		},
		{
			name:        "successful run no changes",
			exitCode:    0,
			stdout:      "No changes needed",
			stderr:      "",
			wantSuccess: true,
			wantSummary: "Iteration completed successfully",
		},
		{
			name:           "failed run with error",
			exitCode:       1,
			stdout:         "",
			stderr:         "Error: API key invalid",
			wantSuccess:    false,
			wantErrContain: "API key invalid",
		},
		{
			name:           "failed run generic",
			exitCode:       1,
			stdout:         "",
			stderr:         "Something went wrong\nfailed: connection timeout",
			wantSuccess:    false,
			wantErrContain: "connection timeout",
		},
		{
			name:        "created file",
			exitCode:    0,
			stdout:      "Created new_file.py",
			stderr:      "",
			wantSuccess: true,
			wantSummary: "Modified 1 file(s)",
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

			if tt.wantSummary != "" && !strings.Contains(result.Summary, tt.wantSummary) {
				t.Errorf("Summary = %q, want to contain %q", result.Summary, tt.wantSummary)
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
			adapter: &Adapter{image: "", model: "claude-3-5-sonnet"},
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
	a, err := agent.Get("aider")
	if err != nil {
		t.Fatalf("Get(aider) returned error: %v", err)
	}

	if a.Name() != "aider" {
		t.Errorf("Registered agent Name() = %q, want %q", a.Name(), "aider")
	}
}
