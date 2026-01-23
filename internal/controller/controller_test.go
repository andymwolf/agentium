package controller

import (
	"bytes"
	"errors"
	"log"
	"testing"
	"time"

	"github.com/andywolf/agentium/internal/cloud/gcp"
)

func TestLoadConfigFromEnv_EnvVar(t *testing.T) {
	tests := []struct {
		name      string
		envValue  string
		wantID    string
		wantRepo  string
		wantAgent string
		wantErr   bool
		errMsg    string
	}{
		{
			name: "valid JSON from env",
			envValue: `{
				"id": "test-session",
				"repository": "github.com/org/repo",
				"tasks": ["1", "2"],
				"agent": "claude-code",
				"max_iterations": 30,
				"max_duration": "2h"
			}`,
			wantID:    "test-session",
			wantRepo:  "github.com/org/repo",
			wantAgent: "claude-code",
			wantErr:   false,
		},
		{
			name:     "invalid JSON from env",
			envValue: `{invalid json}`,
			wantErr:  true,
			errMsg:   "failed to parse AGENTIUM_SESSION_CONFIG",
		},
		{
			name: "minimal valid JSON",
			envValue: `{
				"id": "minimal",
				"repository": "github.com/test/repo"
			}`,
			wantID:   "minimal",
			wantRepo: "github.com/test/repo",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(key string) string {
				if key == "AGENTIUM_SESSION_CONFIG" {
					return tt.envValue
				}
				return ""
			}
			readFile := func(path string) ([]byte, error) {
				return nil, errors.New("should not be called")
			}

			config, err := LoadConfigFromEnv(getenv, readFile)

			if tt.wantErr {
				if err == nil {
					t.Errorf("LoadConfigFromEnv() expected error, got nil")
					return
				}
				if tt.errMsg != "" && !containsString(err.Error(), tt.errMsg) {
					t.Errorf("LoadConfigFromEnv() error = %q, want error containing %q", err.Error(), tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("LoadConfigFromEnv() unexpected error: %v", err)
				return
			}

			if config.ID != tt.wantID {
				t.Errorf("config.ID = %q, want %q", config.ID, tt.wantID)
			}
			if config.Repository != tt.wantRepo {
				t.Errorf("config.Repository = %q, want %q", config.Repository, tt.wantRepo)
			}
			if tt.wantAgent != "" && config.Agent != tt.wantAgent {
				t.Errorf("config.Agent = %q, want %q", config.Agent, tt.wantAgent)
			}
		})
	}
}

func TestLoadConfigFromEnv_File(t *testing.T) {
	tests := []struct {
		name        string
		configPath  string
		fileContent string
		fileErr     error
		wantID      string
		wantErr     bool
		errMsg      string
	}{
		{
			name:       "valid JSON from default path",
			configPath: "",
			fileContent: `{
				"id": "file-session",
				"repository": "github.com/org/repo",
				"tasks": ["1"],
				"agent": "aider"
			}`,
			wantID:  "file-session",
			wantErr: false,
		},
		{
			name:       "valid JSON from custom path",
			configPath: "/custom/path/config.json",
			fileContent: `{
				"id": "custom-session",
				"repository": "github.com/org/repo"
			}`,
			wantID:  "custom-session",
			wantErr: false,
		},
		{
			name:       "file not found",
			configPath: "",
			fileErr:    errors.New("file not found"),
			wantErr:    true,
			errMsg:     "failed to read config file",
		},
		{
			name:        "invalid JSON in file",
			configPath:  "",
			fileContent: `{not valid json`,
			wantErr:     true,
			errMsg:      "failed to parse config file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(key string) string {
				if key == "AGENTIUM_CONFIG_PATH" {
					return tt.configPath
				}
				return ""
			}

			expectedPath := tt.configPath
			if expectedPath == "" {
				expectedPath = DefaultConfigPath
			}

			readFile := func(path string) ([]byte, error) {
				if path != expectedPath {
					t.Errorf("readFile called with path %q, want %q", path, expectedPath)
				}
				if tt.fileErr != nil {
					return nil, tt.fileErr
				}
				return []byte(tt.fileContent), nil
			}

			config, err := LoadConfigFromEnv(getenv, readFile)

			if tt.wantErr {
				if err == nil {
					t.Errorf("LoadConfigFromEnv() expected error, got nil")
					return
				}
				if tt.errMsg != "" && !containsString(err.Error(), tt.errMsg) {
					t.Errorf("LoadConfigFromEnv() error = %q, want error containing %q", err.Error(), tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("LoadConfigFromEnv() unexpected error: %v", err)
				return
			}

			if config.ID != tt.wantID {
				t.Errorf("config.ID = %q, want %q", config.ID, tt.wantID)
			}
		})
	}
}

func TestLoadConfigFromEnv_EnvTakesPrecedence(t *testing.T) {
	// When both env var and file are available, env var should win
	getenv := func(key string) string {
		switch key {
		case "AGENTIUM_SESSION_CONFIG":
			return `{"id": "from-env", "repository": "github.com/env/repo"}`
		case "AGENTIUM_CONFIG_PATH":
			return "/some/path.json"
		}
		return ""
	}

	readFile := func(path string) ([]byte, error) {
		t.Error("readFile should not be called when env var is set")
		return []byte(`{"id": "from-file"}`), nil
	}

	config, err := LoadConfigFromEnv(getenv, readFile)
	if err != nil {
		t.Fatalf("LoadConfigFromEnv() unexpected error: %v", err)
	}

	if config.ID != "from-env" {
		t.Errorf("config.ID = %q, want %q (env should take precedence)", config.ID, "from-env")
	}
}

func TestLoadConfigFromEnv_FullConfig(t *testing.T) {
	fullJSON := `{
		"id": "full-session",
		"repository": "github.com/org/repo",
		"tasks": ["1", "2", "3"],
		"agent": "claude-code",
		"max_iterations": 50,
		"max_duration": "4h",
		"prompt": "Custom prompt here",
		"github": {
			"app_id": 123456,
			"installation_id": 789012,
			"private_key_secret": "projects/test/secrets/key"
		}
	}`

	getenv := func(key string) string {
		if key == "AGENTIUM_SESSION_CONFIG" {
			return fullJSON
		}
		return ""
	}
	readFile := func(path string) ([]byte, error) {
		return nil, errors.New("should not be called")
	}

	config, err := LoadConfigFromEnv(getenv, readFile)
	if err != nil {
		t.Fatalf("LoadConfigFromEnv() unexpected error: %v", err)
	}

	if config.ID != "full-session" {
		t.Errorf("ID = %q, want %q", config.ID, "full-session")
	}
	if config.Repository != "github.com/org/repo" {
		t.Errorf("Repository = %q, want %q", config.Repository, "github.com/org/repo")
	}
	if len(config.Tasks) != 3 {
		t.Errorf("len(Tasks) = %d, want 3", len(config.Tasks))
	}
	if config.Agent != "claude-code" {
		t.Errorf("Agent = %q, want %q", config.Agent, "claude-code")
	}
	if config.MaxIterations != 50 {
		t.Errorf("MaxIterations = %d, want 50", config.MaxIterations)
	}
	if config.MaxDuration != "4h" {
		t.Errorf("MaxDuration = %q, want %q", config.MaxDuration, "4h")
	}
	if config.Prompt != "Custom prompt here" {
		t.Errorf("Prompt = %q, want %q", config.Prompt, "Custom prompt here")
	}
	if config.GitHub.AppID != 123456 {
		t.Errorf("GitHub.AppID = %d, want 123456", config.GitHub.AppID)
	}
	if config.GitHub.InstallationID != 789012 {
		t.Errorf("GitHub.InstallationID = %d, want 789012", config.GitHub.InstallationID)
	}
	if config.GitHub.PrivateKeySecret != "projects/test/secrets/key" {
		t.Errorf("GitHub.PrivateKeySecret = %q, want %q", config.GitHub.PrivateKeySecret, "projects/test/secrets/key")
	}
}

func TestDefaultConfigPath(t *testing.T) {
	if DefaultConfigPath != "/etc/agentium/session.json" {
		t.Errorf("DefaultConfigPath = %q, want %q", DefaultConfigPath, "/etc/agentium/session.json")
	}
}

func TestClearSensitiveData(t *testing.T) {
	c := &Controller{
		gitHubToken: "ghs_secret_token_123",
		config: SessionConfig{
			Prompt: "some prompt with sensitive data",
		},
		logger:      log.New(&bytes.Buffer{}, "", 0),
		cloudLogger: &mockLogger{},
	}
	c.config.GitHub.PrivateKeySecret = "projects/test/secrets/key"
	c.config.ClaudeAuth.AuthJSONBase64 = "base64encodedcreds"

	c.clearSensitiveData()

	if c.gitHubToken != "" {
		t.Errorf("gitHubToken not cleared: %q", c.gitHubToken)
	}
	if c.config.GitHub.PrivateKeySecret != "" {
		t.Errorf("PrivateKeySecret not cleared: %q", c.config.GitHub.PrivateKeySecret)
	}
	if c.config.ClaudeAuth.AuthJSONBase64 != "" {
		t.Errorf("AuthJSONBase64 not cleared: %q", c.config.ClaudeAuth.AuthJSONBase64)
	}
	if c.config.Prompt != "" {
		t.Errorf("Prompt not cleared: %q", c.config.Prompt)
	}
}

func TestFlushLogsWithTimeout_Success(t *testing.T) {
	flushed := false
	c := &Controller{
		logger: log.New(&bytes.Buffer{}, "", 0),
		cloudLogger: &mockLogger{
			flushFn: func() error {
				flushed = true
				return nil
			},
		},
	}

	c.flushLogsWithTimeout()

	if !flushed {
		t.Error("flush was not called")
	}
}

func TestFlushLogsWithTimeout_Error(t *testing.T) {
	c := &Controller{
		logger: log.New(&bytes.Buffer{}, "", 0),
		cloudLogger: &mockLogger{
			flushFn: func() error {
				return errors.New("flush error")
			},
		},
	}

	// Should not panic even with error
	c.flushLogsWithTimeout()
}

func TestFlushLogsWithTimeout_NilLogger(t *testing.T) {
	c := &Controller{
		logger:      log.New(&bytes.Buffer{}, "", 0),
		cloudLogger: nil,
	}

	// Should not panic with nil cloud logger
	c.flushLogsWithTimeout()
}

func TestGetTerminationReason_MaxIterations(t *testing.T) {
	c := &Controller{
		iteration:   10,
		config:      SessionConfig{MaxIterations: 10},
		startTime:   time.Now(),
		maxDuration: 2 * time.Hour,
		taskStates:  make(map[string]*TaskState),
	}

	reason := c.getTerminationReason()
	if reason != "max_iterations_reached" {
		t.Errorf("reason = %q, want %q", reason, "max_iterations_reached")
	}
}

func TestGetTerminationReason_AllTasksTerminal(t *testing.T) {
	c := &Controller{
		iteration:   1,
		config:      SessionConfig{MaxIterations: 10},
		startTime:   time.Now(),
		maxDuration: 2 * time.Hour,
		taskStates: map[string]*TaskState{
			"issue:1": {Phase: PhaseComplete},
			"issue:2": {Phase: PhaseBlocked},
		},
	}

	reason := c.getTerminationReason()
	if reason != "all_tasks_terminal" {
		t.Errorf("reason = %q, want %q", reason, "all_tasks_terminal")
	}
}

func TestGetTerminationReason_PRPushed(t *testing.T) {
	c := &Controller{
		iteration:     1,
		config:        SessionConfig{MaxIterations: 10, PRs: []string{"42"}},
		startTime:     time.Now(),
		maxDuration:   2 * time.Hour,
		taskStates:    make(map[string]*TaskState),
		pushedChanges: true,
	}

	reason := c.getTerminationReason()
	if reason != "pr_changes_pushed" {
		t.Errorf("reason = %q, want %q", reason, "pr_changes_pushed")
	}
}

func TestGetTerminationReason_AllCompleted(t *testing.T) {
	c := &Controller{
		iteration:   1,
		config:      SessionConfig{MaxIterations: 10, Tasks: []string{"1", "2"}},
		startTime:   time.Now(),
		maxDuration: 2 * time.Hour,
		taskStates:  make(map[string]*TaskState),
		completed:   map[string]bool{"1": true, "2": true},
	}

	reason := c.getTerminationReason()
	if reason != "all_tasks_completed" {
		t.Errorf("reason = %q, want %q", reason, "all_tasks_completed")
	}
}

// mockLogger implements gcp.LoggerInterface for testing
type mockLogger struct {
	logs     []string
	flushFn  func() error
	closeFn  func() error
}

func (m *mockLogger) Log(severity gcp.Severity, message string, fields map[string]interface{}) {
	m.logs = append(m.logs, string(severity)+": "+message)
}

func (m *mockLogger) LogInfo(message string) {
	m.Log(gcp.SeverityInfo, message, nil)
}

func (m *mockLogger) LogWarning(message string) {
	m.Log(gcp.SeverityWarning, message, nil)
}

func (m *mockLogger) LogError(message string) {
	m.Log(gcp.SeverityError, message, nil)
}

func (m *mockLogger) SetIteration(iteration int) {}

func (m *mockLogger) Flush() error {
	if m.flushFn != nil {
		return m.flushFn()
	}
	return nil
}

func (m *mockLogger) Close() error {
	if m.closeFn != nil {
		return m.closeFn()
	}
	return nil
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
