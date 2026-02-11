package controller

import (
	"errors"
	"fmt"
	"io"
	"log"
	"testing"

	"github.com/andywolf/agentium/internal/agent"
	"github.com/andywolf/agentium/internal/memory"
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

func TestUpdateTaskPhase_PRDetectionFallback(t *testing.T) {
	tests := []struct {
		name         string
		taskType     string
		agentStatus  string
		prsCreated   []string
		initialPhase TaskPhase
		wantPhase    TaskPhase
		wantPR       string
	}{
		{
			name:         "no status signal but PR detected in IMPLEMENT - advances to DOCS",
			taskType:     "issue",
			agentStatus:  "",
			prsCreated:   []string{"110"},
			initialPhase: PhaseImplement,
			wantPhase:    PhaseDocs,
			wantPR:       "110",
		},
		{
			name:         "no status signal but PR detected in DOCS - completes",
			taskType:     "issue",
			agentStatus:  "",
			prsCreated:   []string{"110"},
			initialPhase: PhaseDocs,
			wantPhase:    PhaseComplete,
			wantPR:       "110",
		},
		{
			name:         "no status signal and no PRs - stays in current phase",
			taskType:     "issue",
			agentStatus:  "",
			prsCreated:   nil,
			initialPhase: PhaseImplement,
			wantPhase:    PhaseImplement,
			wantPR:       "",
		},
		{
			name:         "explicit PR_CREATED status takes precedence",
			taskType:     "issue",
			agentStatus:  "PR_CREATED",
			prsCreated:   []string{"110"},
			initialPhase: PhaseDocs,
			wantPhase:    PhaseComplete,
			wantPR:       "", // StatusMessage is used, not PRsCreated
		},
		{
			name:         "explicit COMPLETE status - no fallback needed",
			taskType:     "issue",
			agentStatus:  "COMPLETE",
			prsCreated:   nil,
			initialPhase: PhaseDocs,
			wantPhase:    PhaseComplete,
			wantPR:       "",
		},
		{
			name:         "fallback in IMPLEMENT uses first PR number and advances to DOCS",
			taskType:     "issue",
			agentStatus:  "",
			prsCreated:   []string{"110", "111"},
			initialPhase: PhaseImplement,
			wantPhase:    PhaseDocs,
			wantPR:       "110",
		},
		{
			name:         "PR detected in PLAN phase - no state change",
			taskType:     "issue",
			agentStatus:  "",
			prsCreated:   []string{"110"},
			initialPhase: PhasePlan,
			wantPhase:    PhasePlan,
			wantPR:       "110",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taskID := taskKey(tt.taskType, "24")
			c := &Controller{
				taskStates: map[string]*TaskState{
					taskID: {ID: "24", Type: tt.taskType, Phase: tt.initialPhase},
				},
				logger: log.New(io.Discard, "", 0),
			}

			result := &agent.IterationResult{
				AgentStatus: tt.agentStatus,
				PRsCreated:  tt.prsCreated,
			}

			c.updateTaskPhase(taskID, result)

			state := c.taskStates[taskID]
			if state.Phase != tt.wantPhase {
				t.Errorf("Phase = %q, want %q", state.Phase, tt.wantPhase)
			}
			if state.PRNumber != tt.wantPR {
				t.Errorf("PRNumber = %q, want %q", state.PRNumber, tt.wantPR)
			}
		})
	}
}

func TestLogTokenConsumption(t *testing.T) {
	t.Run("skips when cloudLogger is nil", func(t *testing.T) {
		c := &Controller{
			logger:         log.New(io.Discard, "", 0),
			cloudLogger:    nil,
			activeTaskType: "issue",
			activeTask:     "42",
		}

		result := &agent.IterationResult{
			InputTokens:  1000,
			OutputTokens: 500,
		}
		session := &agent.Session{}

		// Should not panic when cloudLogger is nil
		c.logTokenConsumption(result, "claude-code", session)
	})

	t.Run("skips when tokens are zero", func(t *testing.T) {
		c := &Controller{
			logger:         log.New(io.Discard, "", 0),
			cloudLogger:    nil,
			activeTaskType: "issue",
			activeTask:     "42",
		}

		result := &agent.IterationResult{
			InputTokens:  0,
			OutputTokens: 0,
		}
		session := &agent.Session{}

		// Should not panic when tokens are zero
		c.logTokenConsumption(result, "claude-code", session)
	})

	t.Run("builds correct task ID and phase", func(t *testing.T) {
		c := &Controller{
			logger:         log.New(io.Discard, "", 0),
			cloudLogger:    nil, // We can't test actual logging without mock
			activeTaskType: "issue",
			activeTask:     "42",
			taskStates: map[string]*TaskState{
				"issue:42": {ID: "42", Type: "issue", Phase: PhaseImplement},
			},
		}

		result := &agent.IterationResult{
			InputTokens:  1500,
			OutputTokens: 300,
		}
		session := &agent.Session{}

		// Should not panic - can't verify labels without mock logger
		c.logTokenConsumption(result, "claude-code", session)
	})
}

func TestSanitizeGitError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		token    string
		wantMsg  string
		wantNil  bool
		wantSame bool // error should be unchanged
	}{
		{
			name:    "nil error returns nil",
			err:     nil,
			token:   "ghp_secret123",
			wantNil: true,
		},
		{
			name:     "empty token returns original error",
			err:      errors.New("some error"),
			token:    "",
			wantSame: true,
		},
		{
			name:    "error without token returns original",
			err:     errors.New("failed to connect to server"),
			token:   "ghp_secret123",
			wantMsg: "failed to connect to server",
		},
		{
			name:    "error with token gets redacted",
			err:     errors.New("fatal: Authentication failed for 'https://x-access-token:ghp_secret123@github.com/org/repo.git/'"),
			token:   "ghp_secret123",
			wantMsg: "fatal: Authentication failed for 'https://x-access-token:[REDACTED]@github.com/org/repo.git/'",
		},
		{
			name:    "multiple occurrences all redacted",
			err:     errors.New("token ghp_abc used, retry with ghp_abc failed"),
			token:   "ghp_abc",
			wantMsg: "token [REDACTED] used, retry with [REDACTED] failed",
		},
		{
			name:    "token at start of message",
			err:     errors.New("ghp_secret: invalid token"),
			token:   "ghp_secret",
			wantMsg: "[REDACTED]: invalid token",
		},
		{
			name:    "token at end of message",
			err:     errors.New("invalid token: ghp_secret"),
			token:   "ghp_secret",
			wantMsg: "invalid token: [REDACTED]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeGitError(tt.err, tt.token)

			if tt.wantNil {
				if got != nil {
					t.Errorf("sanitizeGitError() = %v, want nil", got)
				}
				return
			}

			if tt.wantSame {
				if got != tt.err {
					t.Errorf("sanitizeGitError() should return same error, got different: %v vs %v", got, tt.err)
				}
				return
			}

			if got == nil {
				t.Fatal("sanitizeGitError() = nil, want non-nil error")
			}

			if got.Error() != tt.wantMsg {
				t.Errorf("sanitizeGitError().Error() = %q, want %q", got.Error(), tt.wantMsg)
			}

			// Ensure token is not present in output
			if tt.token != "" && containsString(got.Error(), tt.token) {
				t.Errorf("sanitizeGitError() output still contains token %q", tt.token)
			}
		})
	}
}

func TestBranchPrefixForLabels(t *testing.T) {
	tests := []struct {
		name   string
		labels []issueLabel
		want   string
	}{
		{
			name:   "no labels - default to feature",
			labels: nil,
			want:   "feature",
		},
		{
			name:   "empty labels - default to feature",
			labels: []issueLabel{},
			want:   "feature",
		},
		{
			name:   "bug label",
			labels: []issueLabel{{Name: "bug"}},
			want:   "bug",
		},
		{
			name:   "enhancement label",
			labels: []issueLabel{{Name: "enhancement"}},
			want:   "enhancement",
		},
		{
			name:   "multiple labels - use first",
			labels: []issueLabel{{Name: "bug"}, {Name: "urgent"}},
			want:   "bug",
		},
		{
			name:   "label with space - sanitized",
			labels: []issueLabel{{Name: "good first issue"}},
			want:   "good-first-issue",
		},
		{
			name:   "uppercase label - lowercased",
			labels: []issueLabel{{Name: "Feature"}},
			want:   "feature",
		},
		{
			name:   "mixed case with space",
			labels: []issueLabel{{Name: "Help Wanted"}},
			want:   "help-wanted",
		},
		{
			name:   "label with colon - sanitized",
			labels: []issueLabel{{Name: "type: bug"}},
			want:   "type-bug",
		},
		{
			name:   "label with question mark - sanitized",
			labels: []issueLabel{{Name: "priority?high"}},
			want:   "priority-high",
		},
		{
			name:   "label with slash - sanitized",
			labels: []issueLabel{{Name: "ui/ux"}},
			want:   "ui-ux",
		},
		{
			name:   "label with multiple special chars - sanitized",
			labels: []issueLabel{{Name: "type: bug [critical]"}},
			want:   "type-bug-critical",
		},
		{
			name:   "label with consecutive special chars - collapsed",
			labels: []issueLabel{{Name: "type::bug"}},
			want:   "type-bug",
		},
		{
			name:   "label starting with special char - trimmed",
			labels: []issueLabel{{Name: ":bug"}},
			want:   "bug",
		},
		{
			name:   "label ending with special char - trimmed",
			labels: []issueLabel{{Name: "bug:"}},
			want:   "bug",
		},
		{
			name:   "label that becomes empty after sanitization - default to feature",
			labels: []issueLabel{{Name: ":::"}},
			want:   "feature",
		},
		{
			name:   "label with numbers",
			labels: []issueLabel{{Name: "priority-1"}},
			want:   "priority-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := branchPrefixForLabels(tt.labels)
			if got != tt.want {
				t.Errorf("branchPrefixForLabels() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSanitizeBranchPrefix(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"bug", "bug"},
		{"Bug", "bug"},
		{"type: bug", "type-bug"},
		{"priority?high", "priority-high"},
		{"ui/ux", "ui-ux"},
		{"good first issue", "good-first-issue"},
		{"type::bug", "type-bug"},
		{":bug", "bug"},
		{"bug:", "bug"},
		{":::", ""},
		{"a~b^c", "a-b-c"},
		{"test*case", "test-case"},
		{"feature[1]", "feature-1"},
		{"path\\name", "path-name"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeBranchPrefix(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeBranchPrefix(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
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
		wantEmpty      bool
		wantContains   []string
		wantNotContain []string
	}{
		{
			name:        "nil memory store returns empty",
			memoryStore: false,
			wantEmpty:   true,
		},
		{
			name:           "first iteration returns empty",
			memoryStore:    true,
			phaseIteration: 1,
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
			wantContains: []string{
				"## Feedback from Previous Iteration",
				"### Reviewer Analysis (Context)",
				"Address the test coverage gap",
				"**How to use this feedback:**",
				"targeted, surgical fixes",
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
			wantContains: []string{
				"## Feedback from Previous Iteration",
				"### Judge Directives (REQUIRED)",
				"Add unit tests for edge cases",
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
			wantContains: []string{
				"### Reviewer Analysis (Context)",
				"Detailed analysis of the implementation",
				"### Judge Directives (REQUIRED)",
				"Fix the validation logic",
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
			wantContains:   []string{"Feedback for issue 42"},
			wantNotContain: []string{"Feedback for issue 99"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Controller{
				logger: log.New(io.Discard, "", 0),
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

			got := c.buildIterateFeedbackSection(tt.taskID, tt.phaseIteration, "")

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

func TestFormatExternalComments(t *testing.T) {
	tests := []struct {
		name     string
		comments []issueComment
		want     string
	}{
		{
			name:     "no comments",
			comments: nil,
			want:     "",
		},
		{
			name: "single external comment",
			comments: []issueComment{
				{
					Author:    issueCommentAuthor{Login: "alice"},
					Body:      "This approach looks wrong.",
					CreatedAt: "2025-01-15T10:30:00Z",
				},
			},
			want: "**@alice** (2025-01-15):\n> This approach looks wrong.\n\n",
		},
		{
			name: "filters agentium comments",
			comments: []issueComment{
				{
					Author:    issueCommentAuthor{Login: "alice"},
					Body:      "Please fix the tests.",
					CreatedAt: "2025-01-15T10:30:00Z",
				},
				{
					Author:    issueCommentAuthor{Login: "agentium-bot"},
					Body:      "Phase complete.\n\n<!-- agentium:gcp:agentium-abc123 -->",
					CreatedAt: "2025-01-15T11:00:00Z",
				},
			},
			want: "**@alice** (2025-01-15):\n> Please fix the tests.\n\n",
		},
		{
			name: "all agentium comments returns empty",
			comments: []issueComment{
				{
					Author:    issueCommentAuthor{Login: "agentium-bot"},
					Body:      "Status update.\n\n<!-- agentium:gcp:agentium-abc123 -->",
					CreatedAt: "2025-01-15T11:00:00Z",
				},
			},
			want: "",
		},
		{
			name: "multiline comment body",
			comments: []issueComment{
				{
					Author:    issueCommentAuthor{Login: "bob"},
					Body:      "Line one.\nLine two.\nLine three.",
					CreatedAt: "2025-02-01T08:00:00Z",
				},
			},
			want: "**@bob** (2025-02-01):\n> Line one.\n> Line two.\n> Line three.\n\n",
		},
		{
			name: "short createdAt preserved as-is",
			comments: []issueComment{
				{
					Author:    issueCommentAuthor{Login: "carol"},
					Body:      "Short date.",
					CreatedAt: "2025-03",
				},
			},
			want: "**@carol** (2025-03):\n> Short date.\n\n",
		},
		{
			name: "multiple external comments in order",
			comments: []issueComment{
				{
					Author:    issueCommentAuthor{Login: "alice"},
					Body:      "First comment.",
					CreatedAt: "2025-01-10T09:00:00Z",
				},
				{
					Author:    issueCommentAuthor{Login: "bob"},
					Body:      "Second comment.",
					CreatedAt: "2025-01-11T10:00:00Z",
				},
			},
			want: "**@alice** (2025-01-10):\n> First comment.\n\n**@bob** (2025-01-11):\n> Second comment.\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatExternalComments(tt.comments)
			if got != tt.want {
				t.Errorf("formatExternalComments() =\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}
