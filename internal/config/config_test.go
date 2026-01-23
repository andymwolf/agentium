package config

import (
	"testing"
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
			errMsg:  "at least one issue or PR is required",
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
					MachineType: "e2-medium",
					DiskSizeGB:  50,
				},
				Defaults: DefaultsConfig{
					Agent:         "claude-code",
					MaxIterations: 30,
					MaxDuration:   "2h",
				},
				Session: SessionConfig{
					Agent:         "claude-code",
					MaxIterations: 30,
					MaxDuration:   "2h",
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
					Agent:         "claude-code",
					MaxIterations: 30,
					MaxDuration:   "2h",
				},
				Session: SessionConfig{
					Agent:         "claude-code",
					MaxIterations: 30,
					MaxDuration:   "2h",
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
					Agent:         "claude-code",
					MaxIterations: 30,
					MaxDuration:   "2h",
				},
				Session: SessionConfig{
					Agent:         "claude-code",
					MaxIterations: 30,
					MaxDuration:   "2h",
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
					Agent:         "aider",
					MaxIterations: 50,
					MaxDuration:   "4h",
				},
			},
			expected: Config{
				Cloud: CloudConfig{
					Provider:    "gcp",
					MachineType: "n1-standard-4",
					DiskSizeGB:  100,
				},
				Defaults: DefaultsConfig{
					Agent:         "aider",
					MaxIterations: 50,
					MaxDuration:   "4h",
				},
				Session: SessionConfig{
					Agent:         "aider",
					MaxIterations: 50,
					MaxDuration:   "4h",
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
					MachineType: "e2-medium",
					DiskSizeGB:  50,
				},
				Defaults: DefaultsConfig{
					Agent:         "claude-code",
					MaxIterations: 30,
					MaxDuration:   "2h",
				},
				Session: SessionConfig{
					Repository:    "github.com/org/repo",
					Agent:         "claude-code",
					MaxIterations: 30,
					MaxDuration:   "2h",
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
			if tt.config.Defaults.Agent != tt.expected.Defaults.Agent {
				t.Errorf("Defaults.Agent = %q, want %q", tt.config.Defaults.Agent, tt.expected.Defaults.Agent)
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
