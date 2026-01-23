package controller

import (
	"errors"
	"fmt"
	"testing"

	"github.com/andywolf/agentium/internal/agent"
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
			name: "PRs first then issues",
			taskQueue: []TaskQueueItem{
				{Type: "pr", ID: "57"},
				{Type: "pr", ID: "54"},
				{Type: "issue", ID: "6"},
				{Type: "issue", ID: "7"},
			},
			taskStates: map[string]*TaskState{
				"pr:57":    {ID: "57", Type: "pr", Phase: PhaseAnalyze},
				"pr:54":    {ID: "54", Type: "pr", Phase: PhaseAnalyze},
				"issue:6":  {ID: "6", Type: "issue", Phase: PhaseImplement},
				"issue:7":  {ID: "7", Type: "issue", Phase: PhaseImplement},
			},
			wantType: "pr",
			wantID:   "57",
		},
		{
			name: "first PR complete, second PR next",
			taskQueue: []TaskQueueItem{
				{Type: "pr", ID: "57"},
				{Type: "pr", ID: "54"},
				{Type: "issue", ID: "6"},
			},
			taskStates: map[string]*TaskState{
				"pr:57":   {ID: "57", Type: "pr", Phase: PhaseComplete},
				"pr:54":   {ID: "54", Type: "pr", Phase: PhaseAnalyze},
				"issue:6": {ID: "6", Type: "issue", Phase: PhaseImplement},
			},
			wantType: "pr",
			wantID:   "54",
		},
		{
			name: "all PRs done, issues next",
			taskQueue: []TaskQueueItem{
				{Type: "pr", ID: "57"},
				{Type: "pr", ID: "54"},
				{Type: "issue", ID: "6"},
				{Type: "issue", ID: "7"},
			},
			taskStates: map[string]*TaskState{
				"pr:57":   {ID: "57", Type: "pr", Phase: PhaseComplete},
				"pr:54":   {ID: "54", Type: "pr", Phase: PhaseNothingToDo},
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
			name: "all tasks complete",
			taskQueue: []TaskQueueItem{
				{Type: "pr", ID: "57"},
				{Type: "issue", ID: "6"},
			},
			taskStates: map[string]*TaskState{
				"pr:57":   {ID: "57", Type: "pr", Phase: PhaseComplete},
				"issue:6": {ID: "6", Type: "issue", Phase: PhaseBlocked},
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
	// Simulates: --prs 57,54 --issues 6,7,8,9,10,4,11
	// Expected processing order: 57, 54, 6, 7, 8, 9, 10, 4, 11
	c := &Controller{
		taskQueue: []TaskQueueItem{
			{Type: "pr", ID: "57"},
			{Type: "pr", ID: "54"},
			{Type: "issue", ID: "6"},
			{Type: "issue", ID: "7"},
			{Type: "issue", ID: "8"},
			{Type: "issue", ID: "9"},
			{Type: "issue", ID: "10"},
			{Type: "issue", ID: "4"},
			{Type: "issue", ID: "11"},
		},
		taskStates: map[string]*TaskState{
			"pr:57":    {ID: "57", Type: "pr", Phase: PhaseAnalyze},
			"pr:54":    {ID: "54", Type: "pr", Phase: PhaseAnalyze},
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
		{"pr", "57"}, {"pr", "54"},
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
		taskID := fmt.Sprintf("%s:%s", got.Type, got.ID)
		c.taskStates[taskID].Phase = PhaseComplete
	}

	// After all tasks complete, should return nil
	if got := c.nextQueuedTask(); got != nil {
		t.Errorf("after all complete: nextQueuedTask() = %+v, want nil", got)
	}
}

func TestBuildPromptForPR(t *testing.T) {
	c := &Controller{
		config: SessionConfig{Repository: "github.com/org/repo"},
	}

	pr := prWithReviews{
		Detail: prDetail{
			Number:      57,
			Title:       "Fix authentication flow",
			HeadRefName: "agentium/issue-5-fix-auth",
		},
		Reviews: []prReview{
			{State: "CHANGES_REQUESTED", Body: "Please add error handling"},
		},
		Comments: []prComment{
			{Path: "auth/handler.go", Line: 42, Body: "Missing nil check here"},
		},
	}

	prompt := c.buildPromptForPR(pr)

	contains := []string{
		"github.com/org/repo",
		"PR REVIEW SESSION",
		"PR #57",
		"Fix authentication flow",
		"agentium/issue-5-fix-auth",
		"Please add error handling",
		"auth/handler.go",
		"Missing nil check here",
		"ALREADY on the PR branch",
		"do NOT create a new branch",
	}
	for _, substr := range contains {
		if !containsString(prompt, substr) {
			t.Errorf("buildPromptForPR() missing %q", substr)
		}
	}
}

func TestBuildPromptForTask(t *testing.T) {
	tests := []struct {
		name         string
		issueNumber  string
		issueDetails []issueDetail
		existingWork *agent.ExistingWork
		contains     []string
		notContains  []string
	}{
		{
			name:        "fresh start - no existing work",
			issueNumber: "42",
			issueDetails: []issueDetail{
				{Number: 42, Title: "Fix login bug", Body: "The login page crashes"},
			},
			existingWork: nil,
			contains: []string{
				"Issue #42",
				"Fix login bug",
				"login page crashes",
				"Create a new branch",
				"Create a pull request",
			},
			notContains: []string{
				"Existing Work Detected",
				"Do NOT create a new branch",
			},
		},
		{
			name:        "existing PR found",
			issueNumber: "6",
			issueDetails: []issueDetail{
				{Number: 6, Title: "Add cloud logging", Body: "Integrate GCP logging"},
			},
			existingWork: &agent.ExistingWork{
				Branch:   "agentium/issue-6-cloud-logging",
				PRNumber: "87",
				PRTitle:  "Add Cloud Logging integration",
			},
			contains: []string{
				"Issue #6",
				"Existing Work Detected",
				"PR #87",
				"agentium/issue-6-cloud-logging",
				"Do NOT create a new branch",
				"Do NOT create a new PR",
			},
			notContains: []string{
				"Create a new branch",
			},
		},
		{
			name:        "existing branch only (no PR)",
			issueNumber: "7",
			issueDetails: []issueDetail{
				{Number: 7, Title: "Graceful shutdown", Body: "Implement shutdown"},
			},
			existingWork: &agent.ExistingWork{
				Branch: "agentium/issue-7-graceful-shutdown",
			},
			contains: []string{
				"Issue #7",
				"Existing Work Detected",
				"agentium/issue-7-graceful-shutdown",
				"Do NOT create a new branch",
				"Create a PR linking to the issue",
			},
			notContains: []string{
				"Create a new branch",
				"Do NOT create a new PR",
			},
		},
		{
			name:         "issue not in details",
			issueNumber:  "99",
			issueDetails: []issueDetail{},
			existingWork: nil,
			contains: []string{
				"Issue #99",
				"Create a new branch",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Controller{
				config:       SessionConfig{Repository: "github.com/org/repo"},
				issueDetails: tt.issueDetails,
				workDir:      "/workspace",
			}
			got := c.buildPromptForTask(tt.issueNumber, tt.existingWork)

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

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
