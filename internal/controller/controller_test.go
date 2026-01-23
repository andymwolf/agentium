package controller

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"testing"
	"time"
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

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// mockLogFlusher implements LogFlusher for testing
type mockLogFlusher struct {
	flushed    bool
	closed     bool
	flushErr   error
	closeErr   error
	flushDelay time.Duration
}

func (m *mockLogFlusher) Flush() error {
	if m.flushDelay > 0 {
		time.Sleep(m.flushDelay)
	}
	m.flushed = true
	return m.flushErr
}

func (m *mockLogFlusher) Close() error {
	m.closed = true
	return m.closeErr
}

func TestDefaultShutdownConfig(t *testing.T) {
	cfg := DefaultShutdownConfig()

	if cfg.FlushTimeout != 10*time.Second {
		t.Errorf("FlushTimeout = %v, want %v", cfg.FlushTimeout, 10*time.Second)
	}
	if cfg.ShutdownTimeout != 30*time.Second {
		t.Errorf("ShutdownTimeout = %v, want %v", cfg.ShutdownTimeout, 30*time.Second)
	}
}

func TestController_RegisterLogWriter(t *testing.T) {
	c := &Controller{
		logWriters:     make([]LogFlusher, 0),
		shutdownConfig: DefaultShutdownConfig(),
		logger:         log.New(io.Discard, "", 0),
		completed:      make(map[string]bool),
		taskStates:     make(map[string]*TaskState),
	}

	writer1 := &mockLogFlusher{}
	writer2 := &mockLogFlusher{}

	c.RegisterLogWriter(writer1)
	c.RegisterLogWriter(writer2)

	if len(c.logWriters) != 2 {
		t.Errorf("len(logWriters) = %d, want 2", len(c.logWriters))
	}
}

func TestController_Shutdown_FlushesLogs(t *testing.T) {
	c := &Controller{
		config:         SessionConfig{ID: "test-session"},
		logWriters:     make([]LogFlusher, 0),
		shutdownConfig: DefaultShutdownConfig(),
		logger:         log.New(io.Discard, "", 0),
		completed:      make(map[string]bool),
		taskStates:     make(map[string]*TaskState),
		startTime:      time.Now(),
	}

	writer1 := &mockLogFlusher{}
	writer2 := &mockLogFlusher{}
	c.RegisterLogWriter(writer1)
	c.RegisterLogWriter(writer2)

	c.Shutdown()

	if !writer1.flushed {
		t.Error("writer1 should have been flushed")
	}
	if !writer2.flushed {
		t.Error("writer2 should have been flushed")
	}
}

func TestController_Shutdown_ClosesLogWriters(t *testing.T) {
	c := &Controller{
		config:         SessionConfig{ID: "test-session"},
		logWriters:     make([]LogFlusher, 0),
		shutdownConfig: DefaultShutdownConfig(),
		logger:         log.New(io.Discard, "", 0),
		completed:      make(map[string]bool),
		taskStates:     make(map[string]*TaskState),
		startTime:      time.Now(),
	}

	writer := &mockLogFlusher{}
	c.RegisterLogWriter(writer)

	c.Shutdown()

	if !writer.closed {
		t.Error("writer should have been closed")
	}
}

func TestController_Shutdown_ClearsSensitiveData(t *testing.T) {
	c := &Controller{
		config: SessionConfig{
			ID: "test-session",
			GitHub: struct {
				AppID            int64  `json:"app_id"`
				InstallationID   int64  `json:"installation_id"`
				PrivateKeySecret string `json:"private_key_secret"`
			}{
				PrivateKeySecret: "projects/test/secrets/key",
			},
			ClaudeAuth: struct {
				AuthMode       string `json:"auth_mode"`
				AuthJSONBase64 string `json:"auth_json_base64,omitempty"`
			}{
				AuthJSONBase64: "base64-encoded-credentials",
			},
			Prompt: "sensitive-prompt-content",
		},
		gitHubToken:    "ghp_test-token-12345",
		logWriters:     make([]LogFlusher, 0),
		shutdownConfig: DefaultShutdownConfig(),
		logger:         log.New(io.Discard, "", 0),
		completed:      make(map[string]bool),
		taskStates:     make(map[string]*TaskState),
		startTime:      time.Now(),
	}

	c.Shutdown()

	if c.gitHubToken != "" {
		t.Errorf("gitHubToken should be cleared, got %q", c.gitHubToken)
	}
	if c.config.GitHub.PrivateKeySecret != "" {
		t.Errorf("PrivateKeySecret should be cleared, got %q", c.config.GitHub.PrivateKeySecret)
	}
	if c.config.ClaudeAuth.AuthJSONBase64 != "" {
		t.Errorf("AuthJSONBase64 should be cleared, got %q", c.config.ClaudeAuth.AuthJSONBase64)
	}
	if c.config.Prompt != "" {
		t.Errorf("Prompt should be cleared, got %q", c.config.Prompt)
	}
}

func TestController_Shutdown_FlushTimeout(t *testing.T) {
	c := &Controller{
		config: SessionConfig{ID: "test-session"},
		logWriters: []LogFlusher{
			&mockLogFlusher{flushDelay: 5 * time.Second}, // Will take longer than timeout
		},
		shutdownConfig: ShutdownConfig{
			FlushTimeout:    100 * time.Millisecond, // Very short timeout
			ShutdownTimeout: 1 * time.Second,
		},
		logger:     log.New(io.Discard, "", 0),
		completed:  make(map[string]bool),
		taskStates: make(map[string]*TaskState),
		startTime:  time.Now(),
	}

	start := time.Now()
	c.Shutdown()
	elapsed := time.Since(start)

	// Should complete within the shutdown timeout, not wait for the full flush delay
	if elapsed > 2*time.Second {
		t.Errorf("Shutdown took %v, expected to timeout within 2s", elapsed)
	}
}

func TestController_Shutdown_FlushError(t *testing.T) {
	c := &Controller{
		config: SessionConfig{ID: "test-session"},
		logWriters: []LogFlusher{
			&mockLogFlusher{flushErr: errors.New("flush failed")},
		},
		shutdownConfig: DefaultShutdownConfig(),
		logger:         log.New(io.Discard, "", 0),
		completed:      make(map[string]bool),
		taskStates:     make(map[string]*TaskState),
		startTime:      time.Now(),
	}

	// Should not panic even with flush errors
	c.Shutdown()
}

func TestController_Shutdown_CloseError(t *testing.T) {
	c := &Controller{
		config: SessionConfig{ID: "test-session"},
		logWriters: []LogFlusher{
			&mockLogFlusher{closeErr: errors.New("close failed")},
		},
		shutdownConfig: DefaultShutdownConfig(),
		logger:         log.New(io.Discard, "", 0),
		completed:      make(map[string]bool),
		taskStates:     make(map[string]*TaskState),
		startTime:      time.Now(),
	}

	// Should not panic even with close errors
	c.Shutdown()
}

func TestController_Shutdown_EmitsSessionSummary(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "[controller] ", 0)

	c := &Controller{
		config: SessionConfig{
			ID:            "test-session-123",
			MaxIterations: 10,
			Tasks:         []string{"1", "2"},
		},
		iteration:      5,
		logWriters:     make([]LogFlusher, 0),
		shutdownConfig: DefaultShutdownConfig(),
		logger:         logger,
		completed:      make(map[string]bool),
		taskStates: map[string]*TaskState{
			"issue:1": {ID: "1", Type: "issue", Phase: PhaseComplete, LastStatus: "COMPLETE"},
			"issue:2": {ID: "2", Type: "issue", Phase: PhaseBlocked, LastStatus: "BLOCKED"},
		},
		startTime: time.Now().Add(-5 * time.Minute),
	}

	c.Shutdown()

	output := buf.String()

	// Verify session summary was emitted
	if !containsString(output, "Final Session Summary") {
		t.Error("output should contain 'Final Session Summary'")
	}
	if !containsString(output, "test-session-123") {
		t.Error("output should contain session ID")
	}
	if !containsString(output, "5/10") {
		t.Error("output should contain iterations '5/10'")
	}
	if !containsString(output, "blocked: 1") {
		t.Error("output should contain blocked count")
	}
}

func TestController_Shutdown_NoLogWriters(t *testing.T) {
	c := &Controller{
		config:         SessionConfig{ID: "test-session"},
		logWriters:     make([]LogFlusher, 0),
		shutdownConfig: DefaultShutdownConfig(),
		logger:         log.New(io.Discard, "", 0),
		completed:      make(map[string]bool),
		taskStates:     make(map[string]*TaskState),
		startTime:      time.Now(),
	}

	// Should not panic with no log writers
	c.Shutdown()
}

func TestController_Shutdown_ClosesSecretManager(t *testing.T) {
	mockSM := &mockSecretManager{}

	c := &Controller{
		config:         SessionConfig{ID: "test-session"},
		logWriters:     make([]LogFlusher, 0),
		shutdownConfig: DefaultShutdownConfig(),
		logger:         log.New(io.Discard, "", 0),
		completed:      make(map[string]bool),
		taskStates:     make(map[string]*TaskState),
		startTime:      time.Now(),
		secretManager:  mockSM,
	}

	c.Shutdown()

	if !mockSM.closed {
		t.Error("secret manager should have been closed")
	}
}

func TestController_Shutdown_NilSecretManager(t *testing.T) {
	c := &Controller{
		config:         SessionConfig{ID: "test-session"},
		logWriters:     make([]LogFlusher, 0),
		shutdownConfig: DefaultShutdownConfig(),
		logger:         log.New(io.Discard, "", 0),
		completed:      make(map[string]bool),
		taskStates:     make(map[string]*TaskState),
		startTime:      time.Now(),
		secretManager:  nil,
	}

	// Should not panic with nil secret manager
	c.Shutdown()
}

func TestController_Cleanup_CallsShutdown(t *testing.T) {
	writer := &mockLogFlusher{}

	c := &Controller{
		config:         SessionConfig{ID: "test-session"},
		logWriters:     []LogFlusher{writer},
		shutdownConfig: DefaultShutdownConfig(),
		logger:         log.New(io.Discard, "", 0),
		completed:      make(map[string]bool),
		taskStates:     make(map[string]*TaskState),
		startTime:      time.Now(),
	}

	c.cleanup()

	// cleanup should invoke Shutdown which flushes and closes
	if !writer.flushed {
		t.Error("writer should have been flushed via cleanup -> Shutdown")
	}
	if !writer.closed {
		t.Error("writer should have been closed via cleanup -> Shutdown")
	}
}

func TestLogFlusherInterface(t *testing.T) {
	// Verify that mockLogFlusher implements LogFlusher
	var _ LogFlusher = (*mockLogFlusher)(nil)
}

// mockSecretManager implements SecretFetcher for testing
type mockSecretManager struct {
	closed bool
}

func (m *mockSecretManager) FetchSecret(ctx context.Context, secretPath string) (string, error) {
	return "mock-secret", nil
}

func (m *mockSecretManager) Close() error {
	m.closed = true
	return nil
}
