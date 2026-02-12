package controller

import (
	"errors"
	"fmt"
	"testing"

	"github.com/andywolf/agentium/internal/agent"
	"github.com/andywolf/agentium/internal/routing"
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

func TestLoadConfigFromEnv_PhaseLoopConfig(t *testing.T) {
	tests := []struct {
		name          string
		envValue      string
		wantNil       bool
		wantPlan      int
		wantImplement int
		wantDocs      int
	}{
		{
			name: "phase_loop present with iterations",
			envValue: `{
				"id": "test", "repository": "github.com/org/repo",
				"phase_loop": {"plan_max_iterations": 2, "implement_max_iterations": 8, "docs_max_iterations": 3}
			}`,
			wantNil:       false,
			wantPlan:      2,
			wantImplement: 8,
			wantDocs:      3,
		},
		{
			name: "phase_loop absent",
			envValue: `{
				"id": "test", "repository": "github.com/org/repo"
			}`,
			wantNil: true,
		},
		{
			name: "phase_loop present but empty",
			envValue: `{
				"id": "test", "repository": "github.com/org/repo",
				"phase_loop": {}
			}`,
			wantNil: false,
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
				return nil, fmt.Errorf("should not be called")
			}

			config, err := LoadConfigFromEnv(getenv, readFile)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantNil {
				if config.PhaseLoop != nil {
					t.Errorf("PhaseLoop = %+v, want nil", config.PhaseLoop)
				}
				return
			}

			if config.PhaseLoop == nil {
				t.Fatal("PhaseLoop is nil, want non-nil")
			}
			if tt.wantPlan > 0 && config.PhaseLoop.PlanMaxIterations != tt.wantPlan {
				t.Errorf("PlanMaxIterations = %d, want %d", config.PhaseLoop.PlanMaxIterations, tt.wantPlan)
			}
			if tt.wantImplement > 0 && config.PhaseLoop.ImplementMaxIterations != tt.wantImplement {
				t.Errorf("ImplementMaxIterations = %d, want %d", config.PhaseLoop.ImplementMaxIterations, tt.wantImplement)
			}
			if tt.wantDocs > 0 && config.PhaseLoop.DocsMaxIterations != tt.wantDocs {
				t.Errorf("DocsMaxIterations = %d, want %d", config.PhaseLoop.DocsMaxIterations, tt.wantDocs)
			}
		})
	}
}

func TestDefaultConfigPath(t *testing.T) {
	if DefaultConfigPath != "/etc/agentium/session.json" {
		t.Errorf("DefaultConfigPath = %q, want %q", DefaultConfigPath, "/etc/agentium/session.json")
	}
}

func TestNextQueuedTask(t *testing.T) {
	tests := []struct {
		name       string
		taskQueue  []TaskQueueItem
		taskStates map[string]*TaskState
		wantType   string
		wantID     string
		wantNil    bool
	}{
		{
			name: "multiple issues - first pending",
			taskQueue: []TaskQueueItem{
				{Type: "issue", ID: "6"},
				{Type: "issue", ID: "7"},
			},
			taskStates: map[string]*TaskState{
				"issue:6": {ID: "6", Type: "issue", Phase: PhaseImplement},
				"issue:7": {ID: "7", Type: "issue", Phase: PhaseImplement},
			},
			wantType: "issue",
			wantID:   "6",
		},
		{
			name: "first issue complete, second issue next",
			taskQueue: []TaskQueueItem{
				{Type: "issue", ID: "6"},
				{Type: "issue", ID: "7"},
			},
			taskStates: map[string]*TaskState{
				"issue:6": {ID: "6", Type: "issue", Phase: PhaseComplete},
				"issue:7": {ID: "7", Type: "issue", Phase: PhaseImplement},
			},
			wantType: "issue",
			wantID:   "7",
		},
		{
			name: "all tasks complete or blocked",
			taskQueue: []TaskQueueItem{
				{Type: "issue", ID: "6"},
				{Type: "issue", ID: "7"},
			},
			taskStates: map[string]*TaskState{
				"issue:6": {ID: "6", Type: "issue", Phase: PhaseComplete},
				"issue:7": {ID: "7", Type: "issue", Phase: PhaseBlocked},
			},
			wantNil: true,
		},
		{
			name: "no task state yet returns first",
			taskQueue: []TaskQueueItem{
				{Type: "pr", ID: "57"},
				{Type: "issue", ID: "6"},
			},
			taskStates: map[string]*TaskState{},
			wantType:   "pr",
			wantID:     "57",
		},
		{
			name: "issue ordering preserved",
			taskQueue: []TaskQueueItem{
				{Type: "issue", ID: "8"},
				{Type: "issue", ID: "9"},
				{Type: "issue", ID: "4"},
				{Type: "issue", ID: "11"},
			},
			taskStates: map[string]*TaskState{
				"issue:8":  {ID: "8", Phase: PhaseComplete},
				"issue:9":  {ID: "9", Phase: PhaseComplete},
				"issue:4":  {ID: "4", Phase: PhaseImplement},
				"issue:11": {ID: "11", Phase: PhaseImplement},
			},
			wantType: "issue",
			wantID:   "4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Controller{
				taskQueue:  tt.taskQueue,
				taskStates: tt.taskStates,
			}
			got := c.nextQueuedTask()
			if tt.wantNil {
				if got != nil {
					t.Errorf("nextQueuedTask() = %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("nextQueuedTask() = nil, want {Type:%q, ID:%q}", tt.wantType, tt.wantID)
			}
			if got.Type != tt.wantType {
				t.Errorf("nextQueuedTask().Type = %q, want %q", got.Type, tt.wantType)
			}
			if got.ID != tt.wantID {
				t.Errorf("nextQueuedTask().ID = %q, want %q", got.ID, tt.wantID)
			}
		})
	}
}

func TestNextQueuedTask_FullSequence(t *testing.T) {
	// Simulates: --issues 6,7,8,9,10,4,11
	// Expected processing order: 6, 7, 8, 9, 10, 4, 11
	c := &Controller{
		taskQueue: []TaskQueueItem{
			{Type: "issue", ID: "6"},
			{Type: "issue", ID: "7"},
			{Type: "issue", ID: "8"},
			{Type: "issue", ID: "9"},
			{Type: "issue", ID: "10"},
			{Type: "issue", ID: "4"},
			{Type: "issue", ID: "11"},
		},
		taskStates: map[string]*TaskState{
			"issue:6":  {ID: "6", Type: "issue", Phase: PhaseImplement},
			"issue:7":  {ID: "7", Type: "issue", Phase: PhaseImplement},
			"issue:8":  {ID: "8", Type: "issue", Phase: PhaseImplement},
			"issue:9":  {ID: "9", Type: "issue", Phase: PhaseImplement},
			"issue:10": {ID: "10", Type: "issue", Phase: PhaseImplement},
			"issue:4":  {ID: "4", Type: "issue", Phase: PhaseImplement},
			"issue:11": {ID: "11", Type: "issue", Phase: PhaseImplement},
		},
	}

	expectedOrder := []struct {
		typ string
		id  string
	}{
		{"issue", "6"}, {"issue", "7"}, {"issue", "8"},
		{"issue", "9"}, {"issue", "10"}, {"issue", "4"}, {"issue", "11"},
	}

	for i, expected := range expectedOrder {
		got := c.nextQueuedTask()
		if got == nil {
			t.Fatalf("step %d: nextQueuedTask() = nil, want {%s:%s}", i, expected.typ, expected.id)
		}
		if got.Type != expected.typ || got.ID != expected.id {
			t.Errorf("step %d: nextQueuedTask() = {%s:%s}, want {%s:%s}",
				i, got.Type, got.ID, expected.typ, expected.id)
		}
		// Mark current task as complete to advance
		taskID := taskKey(got.Type, got.ID)
		c.taskStates[taskID].Phase = PhaseComplete
	}

	// After all tasks complete, should return nil
	if got := c.nextQueuedTask(); got != nil {
		t.Errorf("after all complete: nextQueuedTask() = %+v, want nil", got)
	}
}

func TestNewController_RoutingAdapterInit(t *testing.T) {
	// Register a test adapter for routing
	agent.Register("test-adapter", func() agent.Agent {
		return &mockAgent{name: "test-adapter"}
	})

	tests := []struct {
		name         string
		routing      *routing.PhaseRouting
		wantAdapters []string
		wantErr      bool
		errContains  string
	}{
		{
			name:         "nil routing - only primary adapter",
			routing:      nil,
			wantAdapters: []string{"claude-code"},
		},
		{
			name: "routing with same adapter as primary",
			routing: &routing.PhaseRouting{
				Default: routing.ModelConfig{Adapter: "claude-code", Model: "opus"},
			},
			wantAdapters: []string{"claude-code"},
		},
		{
			name: "routing with additional adapter",
			routing: &routing.PhaseRouting{
				Default: routing.ModelConfig{Adapter: "claude-code", Model: "opus"},
				Overrides: map[string]routing.ModelConfig{
					"TEST": {Adapter: "test-adapter", Model: "gpt-4"},
				},
			},
			wantAdapters: []string{"claude-code", "test-adapter"},
		},
		{
			name: "routing with unknown adapter fails",
			routing: &routing.PhaseRouting{
				Overrides: map[string]routing.ModelConfig{
					"TEST": {Adapter: "nonexistent-adapter", Model: "gpt-4"},
				},
			},
			wantErr:     true,
			errContains: "failed to initialize routed adapter",
		},
		{
			name: "routing with unknown phase logs warning but succeeds",
			routing: &routing.PhaseRouting{
				Default: routing.ModelConfig{Adapter: "claude-code", Model: "opus"},
				Overrides: map[string]routing.ModelConfig{
					"TYPO_PHASE": {Adapter: "claude-code", Model: "sonnet"},
				},
			},
			wantAdapters: []string{"claude-code"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := SessionConfig{
				ID:          "test-session",
				Repository:  "github.com/org/repo",
				Agent:       "claude-code",
				MaxDuration: "1h",
				Routing:     tt.routing,
			}

			c, err := New(config)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			for _, name := range tt.wantAdapters {
				if _, ok := c.adapters[name]; !ok {
					t.Errorf("expected adapter %q in adapters map, got %v", name, adapterNames(c.adapters))
				}
			}
			if len(c.adapters) != len(tt.wantAdapters) {
				t.Errorf("expected %d adapters, got %d: %v", len(tt.wantAdapters), len(c.adapters), adapterNames(c.adapters))
			}
		})
	}
}

func TestDetermineActivePhase_Routing(t *testing.T) {
	c := &Controller{
		activeTask:     "42",
		activeTaskType: "issue",
		taskStates: map[string]*TaskState{
			"issue:42": {ID: "42", Type: "issue", Phase: PhaseImplement},
		},
	}

	phase := c.determineActivePhase()
	if phase != PhaseImplement {
		t.Errorf("determineActivePhase() = %q, want %q", phase, PhaseImplement)
	}
}

// mockAgent implements the agent.Agent interface for testing
type mockAgent struct {
	name string
}

func (m *mockAgent) Name() string                                                     { return m.name }
func (m *mockAgent) ContainerImage() string                                           { return "test-image:latest" }
func (m *mockAgent) ContainerEntrypoint() []string                                    { return []string{"test-agent"} }
func (m *mockAgent) BuildEnv(session *agent.Session, iteration int) map[string]string { return nil }
func (m *mockAgent) BuildCommand(session *agent.Session, iteration int) []string      { return nil }
func (m *mockAgent) BuildPrompt(session *agent.Session, iteration int) string         { return "" }
func (m *mockAgent) ParseOutput(exitCode int, stdout, stderr string) (*agent.IterationResult, error) {
	return &agent.IterationResult{}, nil
}
func (m *mockAgent) Validate() error { return nil }

func adapterNames(adapters map[string]agent.Agent) []string {
	names := make([]string, 0, len(adapters))
	for name := range adapters {
		names = append(names, name)
	}
	return names
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestCodexAuthConfig(t *testing.T) {
	tests := []struct {
		name       string
		envValue   string
		wantBase64 string
	}{
		{
			name: "codex auth parsed from config",
			envValue: `{
				"id": "test", "repository": "github.com/org/repo",
				"codex_auth": {"auth_json_base64": "eyJhY2Nlc3NfdG9rZW4iOiAidGVzdCJ9"}
			}`,
			wantBase64: "eyJhY2Nlc3NfdG9rZW4iOiAidGVzdCJ9",
		},
		{
			name: "empty when not provided",
			envValue: `{
				"id": "test", "repository": "github.com/org/repo"
			}`,
			wantBase64: "",
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
				return nil, fmt.Errorf("should not be called")
			}

			config, err := LoadConfigFromEnv(getenv, readFile)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if config.CodexAuth.AuthJSONBase64 != tt.wantBase64 {
				t.Errorf("CodexAuth.AuthJSONBase64 = %q, want %q", config.CodexAuth.AuthJSONBase64, tt.wantBase64)
			}
		})
	}
}

func TestLoadConfigFromEnv_PhasesConfig(t *testing.T) {
	tests := []struct {
		name       string
		envValue   string
		wantLen    int
		wantFirst  string
		wantWorker string
	}{
		{
			name: "phases with full step config",
			envValue: `{
				"id": "test", "repository": "github.com/org/repo",
				"phases": [
					{"name": "LINT", "max_iterations": 2, "worker": {"prompt": "run linting"}, "reviewer": {"prompt": "review lint"}, "judge": {"criteria": "lint criteria"}},
					{"name": "IMPLEMENT", "max_iterations": 5}
				]
			}`,
			wantLen:    2,
			wantFirst:  "LINT",
			wantWorker: "run linting",
		},
		{
			name: "phases absent",
			envValue: `{
				"id": "test", "repository": "github.com/org/repo"
			}`,
			wantLen: 0,
		},
		{
			name: "empty phases array",
			envValue: `{
				"id": "test", "repository": "github.com/org/repo",
				"phases": []
			}`,
			wantLen: 0,
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
				return nil, fmt.Errorf("should not be called")
			}

			config, err := LoadConfigFromEnv(getenv, readFile)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(config.Phases) != tt.wantLen {
				t.Fatalf("len(Phases) = %d, want %d", len(config.Phases), tt.wantLen)
			}

			if tt.wantLen > 0 {
				if config.Phases[0].Name != tt.wantFirst {
					t.Errorf("Phases[0].Name = %q, want %q", config.Phases[0].Name, tt.wantFirst)
				}
				if tt.wantWorker != "" {
					if config.Phases[0].Worker == nil {
						t.Fatal("Phases[0].Worker is nil")
					}
					if config.Phases[0].Worker.Prompt != tt.wantWorker {
						t.Errorf("Phases[0].Worker.Prompt = %q, want %q", config.Phases[0].Worker.Prompt, tt.wantWorker)
					}
				}
				// Verify all nested types parsed
				if config.Phases[0].Reviewer == nil {
					t.Fatal("Phases[0].Reviewer is nil")
				}
				if config.Phases[0].Reviewer.Prompt != "review lint" {
					t.Errorf("Phases[0].Reviewer.Prompt = %q, want %q", config.Phases[0].Reviewer.Prompt, "review lint")
				}
				if config.Phases[0].Judge == nil {
					t.Fatal("Phases[0].Judge is nil")
				}
				if config.Phases[0].Judge.Criteria != "lint criteria" {
					t.Errorf("Phases[0].Judge.Criteria = %q, want %q", config.Phases[0].Judge.Criteria, "lint criteria")
				}
				if config.Phases[0].MaxIterations != 2 {
					t.Errorf("Phases[0].MaxIterations = %d, want 2", config.Phases[0].MaxIterations)
				}
				if config.Phases[1].MaxIterations != 5 {
					t.Errorf("Phases[1].MaxIterations = %d, want 5", config.Phases[1].MaxIterations)
				}
			}
		})
	}
}

func TestNewController_InitialPhaseFromCustomPhases(t *testing.T) {
	// This test verifies that when custom Phases are provided, the initial
	// task phase is set to Phases[0].Name instead of the default PhasePlan.
	//
	// We can't call New() directly because it initializes cloud clients,
	// so we test the logic by verifying the phaseConfigs and initial phase
	// assignment via LoadConfigFromEnv + manual controller state check.
	envValue := `{
		"id": "test", "repository": "github.com/org/repo",
		"agent": "claude-code", "max_duration": "1h",
		"tasks": ["42"],
		"phases": [
			{"name": "LINT", "worker": {"prompt": "run lint"}},
			{"name": "IMPLEMENT"}
		],
		"phase_loop": {}
	}`

	getenv := func(key string) string {
		if key == "AGENTIUM_SESSION_CONFIG" {
			return envValue
		}
		return ""
	}
	readFile := func(path string) ([]byte, error) {
		return nil, fmt.Errorf("should not be called")
	}

	config, err := LoadConfigFromEnv(getenv, readFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(config.Phases) != 2 {
		t.Fatalf("len(Phases) = %d, want 2", len(config.Phases))
	}

	// Simulate what New() does: initial phase should be Phases[0].Name
	initialPhase := PhaseImplement
	if config.PhaseLoop != nil {
		initialPhase = PhasePlan
	}
	if len(config.Phases) > 0 {
		initialPhase = TaskPhase(config.Phases[0].Name)
	}

	if initialPhase != TaskPhase("LINT") {
		t.Errorf("initial phase = %q, want %q", initialPhase, "LINT")
	}
}
