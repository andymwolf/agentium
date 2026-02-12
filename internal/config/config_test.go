package config

import (
	"testing"

	"github.com/andywolf/agentium/internal/routing"
)

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: Config{
				Cloud: CloudConfig{
					Provider: "gcp",
					Region:   "us-central1",
				},
				Session: SessionConfig{
					Agent: "claude-code",
				},
			},
			wantErr: false,
		},
		{
			name: "missing provider",
			config: Config{
				Cloud: CloudConfig{
					Region: "us-central1",
				},
			},
			wantErr: true,
			errMsg:  "cloud provider is required",
		},
		{
			name: "invalid provider",
			config: Config{
				Cloud: CloudConfig{
					Provider: "invalid",
					Region:   "us-central1",
				},
			},
			wantErr: true,
			errMsg:  "invalid cloud provider",
		},
		{
			name: "missing region",
			config: Config{
				Cloud: CloudConfig{
					Provider: "gcp",
				},
			},
			wantErr: true,
			errMsg:  "cloud region is required",
		},
		{
			name: "invalid agent",
			config: Config{
				Cloud: CloudConfig{
					Provider: "gcp",
					Region:   "us-central1",
				},
				Session: SessionConfig{
					Agent: "invalid-agent",
				},
			},
			wantErr: true,
			errMsg:  "invalid agent",
		},
		{
			name: "invalid max_duration",
			config: Config{
				Cloud: CloudConfig{
					Provider: "gcp",
					Region:   "us-central1",
				},
				Session: SessionConfig{
					MaxDuration: "invalid",
				},
			},
			wantErr: true,
			errMsg:  "invalid max_duration",
		},
		{
			name: "valid aws provider",
			config: Config{
				Cloud: CloudConfig{
					Provider: "aws",
					Region:   "us-east-1",
				},
			},
			wantErr: false,
		},
		{
			name: "valid azure provider",
			config: Config{
				Cloud: CloudConfig{
					Provider: "azure",
					Region:   "eastus",
				},
			},
			wantErr: false,
		},
		{
			name: "valid aider agent",
			config: Config{
				Cloud: CloudConfig{
					Provider: "gcp",
					Region:   "us-central1",
				},
				Session: SessionConfig{
					Agent: "aider",
				},
			},
			wantErr: false,
		},
		{
			name: "valid codex agent",
			config: Config{
				Cloud: CloudConfig{
					Provider: "gcp",
					Region:   "us-central1",
				},
				Session: SessionConfig{
					Agent: "codex",
				},
			},
			wantErr: false,
		},
		{
			name: "valid duration format",
			config: Config{
				Cloud: CloudConfig{
					Provider: "gcp",
					Region:   "us-central1",
				},
				Session: SessionConfig{
					MaxDuration: "2h30m",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() expected error containing %q, got nil", tt.errMsg)
					return
				}
				if tt.errMsg != "" && !containsString(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %q, want error containing %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestConfig_ValidateForRun(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid run config",
			config: Config{
				Cloud: CloudConfig{
					Provider: "gcp",
					Region:   "us-central1",
				},
				Session: SessionConfig{
					Repository: "github.com/org/repo",
					Tasks:      []string{"1", "2"},
				},
				GitHub: GitHubConfig{
					AppID:            123456,
					InstallationID:   789012,
					PrivateKeySecret: "projects/test/secrets/key",
				},
			},
			wantErr: false,
		},
		{
			name: "missing repository",
			config: Config{
				Cloud: CloudConfig{
					Provider: "gcp",
					Region:   "us-central1",
				},
				Session: SessionConfig{
					Tasks: []string{"1"},
				},
				GitHub: GitHubConfig{
					AppID:            123456,
					InstallationID:   789012,
					PrivateKeySecret: "projects/test/secrets/key",
				},
			},
			wantErr: true,
			errMsg:  "repository is required",
		},
		{
			name: "missing tasks",
			config: Config{
				Cloud: CloudConfig{
					Provider: "gcp",
					Region:   "us-central1",
				},
				Session: SessionConfig{
					Repository: "github.com/org/repo",
				},
				GitHub: GitHubConfig{
					AppID:            123456,
					InstallationID:   789012,
					PrivateKeySecret: "projects/test/secrets/key",
				},
			},
			wantErr: true,
			errMsg:  "at least one issue is required",
		},
		{
			name: "missing GitHub App ID",
			config: Config{
				Cloud: CloudConfig{
					Provider: "gcp",
					Region:   "us-central1",
				},
				Session: SessionConfig{
					Repository: "github.com/org/repo",
					Tasks:      []string{"1"},
				},
				GitHub: GitHubConfig{
					InstallationID:   789012,
					PrivateKeySecret: "projects/test/secrets/key",
				},
			},
			wantErr: true,
			errMsg:  "GitHub App ID is required",
		},
		{
			name: "missing Installation ID",
			config: Config{
				Cloud: CloudConfig{
					Provider: "gcp",
					Region:   "us-central1",
				},
				Session: SessionConfig{
					Repository: "github.com/org/repo",
					Tasks:      []string{"1"},
				},
				GitHub: GitHubConfig{
					AppID:            123456,
					PrivateKeySecret: "projects/test/secrets/key",
				},
			},
			wantErr: true,
			errMsg:  "GitHub App Installation ID is required",
		},
		{
			name: "missing private key secret",
			config: Config{
				Cloud: CloudConfig{
					Provider: "gcp",
					Region:   "us-central1",
				},
				Session: SessionConfig{
					Repository: "github.com/org/repo",
					Tasks:      []string{"1"},
				},
				GitHub: GitHubConfig{
					AppID:          123456,
					InstallationID: 789012,
				},
			},
			wantErr: true,
			errMsg:  "GitHub App private key secret path is required",
		},
		{
			name: "oauth auth with aider agent",
			config: Config{
				Cloud: CloudConfig{
					Provider: "gcp",
					Region:   "us-central1",
				},
				Session: SessionConfig{
					Repository: "github.com/org/repo",
					Tasks:      []string{"1"},
					Agent:      "aider",
				},
				GitHub: GitHubConfig{
					AppID:            123456,
					InstallationID:   789012,
					PrivateKeySecret: "projects/test/secrets/key",
				},
				Claude: ClaudeConfig{
					AuthMode: "oauth",
				},
			},
			wantErr: true,
			errMsg:  "oauth auth_mode is only supported with the claude-code agent",
		},
		{
			name: "oauth auth with claude-code agent",
			config: Config{
				Cloud: CloudConfig{
					Provider: "gcp",
					Region:   "us-central1",
				},
				Session: SessionConfig{
					Repository: "github.com/org/repo",
					Tasks:      []string{"1"},
					Agent:      "claude-code",
				},
				GitHub: GitHubConfig{
					AppID:            123456,
					InstallationID:   789012,
					PrivateKeySecret: "projects/test/secrets/key",
				},
				Claude: ClaudeConfig{
					AuthMode: "oauth",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.ValidateForRun()
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateForRun() expected error containing %q, got nil", tt.errMsg)
					return
				}
				if tt.errMsg != "" && !containsString(err.Error(), tt.errMsg) {
					t.Errorf("ValidateForRun() error = %q, want error containing %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateForRun() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestApplyDefaults(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		expected Config
	}{
		{
			name: "applies GCP machine type default",
			config: Config{
				Cloud: CloudConfig{
					Provider: "gcp",
				},
			},
			expected: Config{
				Cloud: CloudConfig{
					Provider:    "gcp",
					MachineType: "e2-standard-2",
					DiskSizeGB:  50,
				},
				Defaults: DefaultsConfig{
					MaxDuration: "2h",
				},
				Session: SessionConfig{
					Agent:       "claude-code",
					MaxDuration: "2h",
				},
				Controller: ControllerConfig{
					Image: "ghcr.io/andymwolf/agentium-controller:latest",
				},
			},
		},
		{
			name: "applies AWS machine type default",
			config: Config{
				Cloud: CloudConfig{
					Provider: "aws",
				},
			},
			expected: Config{
				Cloud: CloudConfig{
					Provider:    "aws",
					MachineType: "t3.medium",
					DiskSizeGB:  50,
				},
				Defaults: DefaultsConfig{
					MaxDuration: "2h",
				},
				Session: SessionConfig{
					Agent:       "claude-code",
					MaxDuration: "2h",
				},
				Controller: ControllerConfig{
					Image: "ghcr.io/andymwolf/agentium-controller:latest",
				},
			},
		},
		{
			name: "applies Azure machine type default",
			config: Config{
				Cloud: CloudConfig{
					Provider: "azure",
				},
			},
			expected: Config{
				Cloud: CloudConfig{
					Provider:    "azure",
					MachineType: "Standard_B2s",
					DiskSizeGB:  50,
				},
				Defaults: DefaultsConfig{
					MaxDuration: "2h",
				},
				Session: SessionConfig{
					Agent:       "claude-code",
					MaxDuration: "2h",
				},
				Controller: ControllerConfig{
					Image: "ghcr.io/andymwolf/agentium-controller:latest",
				},
			},
		},
		{
			name: "does not override existing values",
			config: Config{
				Cloud: CloudConfig{
					Provider:    "gcp",
					MachineType: "n1-standard-4",
					DiskSizeGB:  100,
				},
				Defaults: DefaultsConfig{
					MaxDuration: "4h",
				},
				Routing: routing.PhaseRouting{
					Default: routing.ModelConfig{
						Adapter: "aider",
					},
				},
			},
			expected: Config{
				Cloud: CloudConfig{
					Provider:    "gcp",
					MachineType: "n1-standard-4",
					DiskSizeGB:  100,
				},
				Defaults: DefaultsConfig{
					MaxDuration: "4h",
				},
				Routing: routing.PhaseRouting{
					Default: routing.ModelConfig{
						Adapter: "aider",
					},
				},
				Session: SessionConfig{
					Agent:       "aider",
					MaxDuration: "4h",
				},
				Controller: ControllerConfig{
					Image: "ghcr.io/andymwolf/agentium-controller:latest",
				},
			},
		},
		{
			name: "session inherits from project repository",
			config: Config{
				Project: ProjectConfig{
					Repository: "github.com/org/repo",
				},
				Cloud: CloudConfig{
					Provider: "gcp",
				},
			},
			expected: Config{
				Project: ProjectConfig{
					Repository: "github.com/org/repo",
				},
				Cloud: CloudConfig{
					Provider:    "gcp",
					MachineType: "e2-standard-2",
					DiskSizeGB:  50,
				},
				Defaults: DefaultsConfig{
					MaxDuration: "2h",
				},
				Session: SessionConfig{
					Repository:  "github.com/org/repo",
					Agent:       "claude-code",
					MaxDuration: "2h",
				},
				Controller: ControllerConfig{
					Image: "ghcr.io/andymwolf/agentium-controller:latest",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applyDefaults(&tt.config)

			if tt.config.Cloud.MachineType != tt.expected.Cloud.MachineType {
				t.Errorf("MachineType = %q, want %q", tt.config.Cloud.MachineType, tt.expected.Cloud.MachineType)
			}
			if tt.config.Cloud.DiskSizeGB != tt.expected.Cloud.DiskSizeGB {
				t.Errorf("DiskSizeGB = %d, want %d", tt.config.Cloud.DiskSizeGB, tt.expected.Cloud.DiskSizeGB)
			}
			if tt.config.Session.Agent != tt.expected.Session.Agent {
				t.Errorf("Session.Agent = %q, want %q", tt.config.Session.Agent, tt.expected.Session.Agent)
			}
			if tt.config.Session.Repository != tt.expected.Session.Repository {
				t.Errorf("Session.Repository = %q, want %q", tt.config.Session.Repository, tt.expected.Session.Repository)
			}
		})
	}
}

func boolPtr(b bool) *bool { return &b }

func TestApplyDefaults_ContainerReuse(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		wantNil  bool
		wantBool bool
	}{
		{
			name: "default propagates when session not set",
			config: Config{
				Cloud:    CloudConfig{Provider: "gcp"},
				Defaults: DefaultsConfig{ContainerReuse: true},
			},
			wantNil:  false,
			wantBool: true,
		},
		{
			name: "session explicit false not overridden by default true",
			config: Config{
				Cloud:    CloudConfig{Provider: "gcp"},
				Defaults: DefaultsConfig{ContainerReuse: true},
				Session:  SessionConfig{ContainerReuse: boolPtr(false)},
			},
			wantNil:  false,
			wantBool: false,
		},
		{
			name: "session explicit true preserved",
			config: Config{
				Cloud:   CloudConfig{Provider: "gcp"},
				Session: SessionConfig{ContainerReuse: boolPtr(true)},
			},
			wantNil:  false,
			wantBool: true,
		},
		{
			name: "no default and no session leaves nil",
			config: Config{
				Cloud: CloudConfig{Provider: "gcp"},
			},
			wantNil: true,
		},
		{
			name: "default false and session not set leaves nil",
			config: Config{
				Cloud:    CloudConfig{Provider: "gcp"},
				Defaults: DefaultsConfig{ContainerReuse: false},
			},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applyDefaults(&tt.config)

			if tt.wantNil {
				if tt.config.Session.ContainerReuse != nil {
					t.Errorf("Session.ContainerReuse = %v, want nil", *tt.config.Session.ContainerReuse)
				}
				return
			}
			if tt.config.Session.ContainerReuse == nil {
				t.Fatal("Session.ContainerReuse is nil, want non-nil")
			}
			if *tt.config.Session.ContainerReuse != tt.wantBool {
				t.Errorf("Session.ContainerReuse = %v, want %v", *tt.config.Session.ContainerReuse, tt.wantBool)
			}
		})
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestNormalizeRoutingKeys(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]routing.ModelConfig
		expected map[string]routing.ModelConfig
	}{
		{
			name:     "nil overrides",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty overrides",
			input:    map[string]routing.ModelConfig{},
			expected: map[string]routing.ModelConfig{},
		},
		{
			name: "lowercase keys normalized to uppercase",
			input: map[string]routing.ModelConfig{
				"plan_review":      {Adapter: "codex", Model: "gpt-5"},
				"implement_review": {Adapter: "codex", Model: "gpt-5"},
			},
			expected: map[string]routing.ModelConfig{
				"PLAN_REVIEW":      {Adapter: "codex", Model: "gpt-5"},
				"IMPLEMENT_REVIEW": {Adapter: "codex", Model: "gpt-5"},
			},
		},
		{
			name: "mixed case normalized to uppercase",
			input: map[string]routing.ModelConfig{
				"Plan_Review": {Adapter: "codex", Model: "gpt-5"},
				"IMPLEMENT":   {Adapter: "claude-code", Model: "opus"},
			},
			expected: map[string]routing.ModelConfig{
				"PLAN_REVIEW": {Adapter: "codex", Model: "gpt-5"},
				"IMPLEMENT":   {Adapter: "claude-code", Model: "opus"},
			},
		},
		{
			name: "already uppercase unchanged",
			input: map[string]routing.ModelConfig{
				"PLAN_REVIEW": {Adapter: "codex", Model: "gpt-5"},
			},
			expected: map[string]routing.ModelConfig{
				"PLAN_REVIEW": {Adapter: "codex", Model: "gpt-5"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Routing: routing.PhaseRouting{
					Overrides: tt.input,
				},
			}
			normalizeRoutingKeys(cfg)

			if tt.expected == nil {
				if len(cfg.Routing.Overrides) > 0 {
					t.Errorf("expected nil/empty overrides, got %v", cfg.Routing.Overrides)
				}
				return
			}

			if len(cfg.Routing.Overrides) != len(tt.expected) {
				t.Errorf("expected %d overrides, got %d", len(tt.expected), len(cfg.Routing.Overrides))
				return
			}

			for key, expectedVal := range tt.expected {
				actualVal, ok := cfg.Routing.Overrides[key]
				if !ok {
					t.Errorf("missing key %q in normalized overrides", key)
					continue
				}
				if actualVal.Adapter != expectedVal.Adapter || actualVal.Model != expectedVal.Model {
					t.Errorf("key %q: expected %+v, got %+v", key, expectedVal, actualVal)
				}
			}
		})
	}
}
