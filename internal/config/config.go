package config

import (
	"fmt"
	"time"

	"github.com/andywolf/agentium/internal/routing"
	"github.com/spf13/viper"
)

// SubAgentConfigYAML specifies agent overrides for a delegated sub-task type in YAML config.
type SubAgentConfigYAML struct {
	Agent  string              `mapstructure:"agent"`
	Model  *routing.ModelConfig `mapstructure:"model"`
	Skills []string            `mapstructure:"skills"`
}

// DelegationConfigYAML controls sub-agent delegation in YAML config.
type DelegationConfigYAML struct {
	Enabled   bool                          `mapstructure:"enabled"`
	Strategy  string                        `mapstructure:"strategy"`
	SubAgents map[string]SubAgentConfigYAML `mapstructure:"sub_agents"`
}

// Config represents the full Agentium configuration
type Config struct {
	Project    ProjectConfig        `mapstructure:"project"`
	GitHub     GitHubConfig         `mapstructure:"github"`
	Cloud      CloudConfig          `mapstructure:"cloud"`
	Defaults   DefaultsConfig       `mapstructure:"defaults"`
	Session    SessionConfig        `mapstructure:"session"`
	Controller ControllerConfig     `mapstructure:"controller"`
	Claude     ClaudeConfig         `mapstructure:"claude"`
	Prompts    PromptsConfig        `mapstructure:"prompts"`
	Routing    routing.PhaseRouting `mapstructure:"routing"`
	Delegation DelegationConfigYAML `mapstructure:"delegation"`
}

// ClaudeConfig contains Claude AI authentication settings
type ClaudeConfig struct {
	AuthMode     string `mapstructure:"auth_mode"`      // "api" (default) or "oauth"
	AuthJSONPath string `mapstructure:"auth_json_path"` // Path to auth.json
}

// PromptsConfig contains settings for system and project prompt loading
type PromptsConfig struct {
	SystemMDURL  string `mapstructure:"system_md_url"`  // URL to fetch SYSTEM.md (overrides default)
	FetchTimeout string `mapstructure:"fetch_timeout"`  // Timeout for fetching remote prompts (default: 5s)
}

// ProjectConfig contains project-level settings
type ProjectConfig struct {
	Name       string `mapstructure:"name"`
	Repository string `mapstructure:"repository"`
}

// GitHubConfig contains GitHub App authentication settings
type GitHubConfig struct {
	AppID            int64  `mapstructure:"app_id"`
	InstallationID   int64  `mapstructure:"installation_id"`
	PrivateKeySecret string `mapstructure:"private_key_secret"`
}

// CloudConfig contains cloud provider settings
type CloudConfig struct {
	Provider    string `mapstructure:"provider"`
	Region      string `mapstructure:"region"`
	Project     string `mapstructure:"project"`      // GCP project ID
	MachineType string `mapstructure:"machine_type"` // VM instance type
	UseSpot     bool   `mapstructure:"use_spot"`     // Use spot/preemptible instances
	DiskSizeGB  int    `mapstructure:"disk_size_gb"`
}

// DefaultsConfig contains default session settings
type DefaultsConfig struct {
	Agent         string `mapstructure:"agent"`
	MaxIterations int    `mapstructure:"max_iterations"`
	MaxDuration   string `mapstructure:"max_duration"`
}

// SessionConfig contains per-session settings
type SessionConfig struct {
	Repository    string   `mapstructure:"repository"`
	Tasks         []string `mapstructure:"tasks"`
	PRs           []string `mapstructure:"prs"`
	Agent         string   `mapstructure:"agent"`
	MaxIterations int      `mapstructure:"max_iterations"`
	MaxDuration   string   `mapstructure:"max_duration"`
	Prompt        string   `mapstructure:"prompt"`
}

// ControllerConfig contains session controller settings
type ControllerConfig struct {
	Image string `mapstructure:"image"`
}

// Load loads configuration from file and environment
func Load() (*Config, error) {
	cfg := &Config{}

	if err := viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Apply defaults
	applyDefaults(cfg)

	return cfg, nil
}

// applyDefaults sets default values for unset fields
func applyDefaults(cfg *Config) {
	if cfg.Cloud.MachineType == "" {
		switch cfg.Cloud.Provider {
		case "gcp":
			cfg.Cloud.MachineType = "e2-medium"
		case "aws":
			cfg.Cloud.MachineType = "t3.medium"
		case "azure":
			cfg.Cloud.MachineType = "Standard_B2s"
		}
	}

	if cfg.Cloud.DiskSizeGB == 0 {
		cfg.Cloud.DiskSizeGB = 50
	}

	if cfg.Defaults.Agent == "" {
		cfg.Defaults.Agent = "claude-code"
	}

	if cfg.Defaults.MaxIterations == 0 {
		cfg.Defaults.MaxIterations = 30
	}

	if cfg.Defaults.MaxDuration == "" {
		cfg.Defaults.MaxDuration = "2h"
	}

	// Apply defaults to session if not overridden
	if cfg.Session.Repository == "" {
		cfg.Session.Repository = cfg.Project.Repository
	}

	if cfg.Session.Agent == "" {
		cfg.Session.Agent = cfg.Defaults.Agent
	}

	if cfg.Session.MaxIterations == 0 {
		cfg.Session.MaxIterations = cfg.Defaults.MaxIterations
	}

	if cfg.Session.MaxDuration == "" {
		cfg.Session.MaxDuration = cfg.Defaults.MaxDuration
	}

	if cfg.Controller.Image == "" {
		cfg.Controller.Image = "ghcr.io/andymwolf/agentium-controller:latest"
	}

	if cfg.Claude.AuthMode == "" {
		cfg.Claude.AuthMode = "api"
	}

	if cfg.Claude.AuthJSONPath == "" {
		cfg.Claude.AuthJSONPath = "~/.config/claude-code/auth.json"
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Cloud.Provider == "" {
		return fmt.Errorf("cloud provider is required")
	}

	validProviders := map[string]bool{"gcp": true, "aws": true, "azure": true}
	if !validProviders[c.Cloud.Provider] {
		return fmt.Errorf("invalid cloud provider: %s (must be gcp, aws, or azure)", c.Cloud.Provider)
	}

	if c.Cloud.Region == "" {
		return fmt.Errorf("cloud region is required")
	}

	if c.Session.Agent != "" {
		validAgents := map[string]bool{"claude-code": true, "aider": true}
		if !validAgents[c.Session.Agent] {
			return fmt.Errorf("invalid agent: %s (must be claude-code or aider)", c.Session.Agent)
		}
	}

	if c.Session.MaxDuration != "" {
		if _, err := time.ParseDuration(c.Session.MaxDuration); err != nil {
			return fmt.Errorf("invalid max_duration: %w", err)
		}
	}

	if c.Claude.AuthMode != "" {
		validAuthModes := map[string]bool{"api": true, "oauth": true}
		if !validAuthModes[c.Claude.AuthMode] {
			return fmt.Errorf("invalid claude auth_mode: %s (must be api or oauth)", c.Claude.AuthMode)
		}
	}

	return nil
}

// ValidateForRun performs additional validation required before running a session
func (c *Config) ValidateForRun() error {
	if err := c.Validate(); err != nil {
		return err
	}

	if c.Session.Repository == "" {
		return fmt.Errorf("repository is required")
	}

	if len(c.Session.Tasks) == 0 && len(c.Session.PRs) == 0 {
		return fmt.Errorf("at least one issue or PR is required")
	}

	if c.GitHub.AppID == 0 {
		return fmt.Errorf("GitHub App ID is required")
	}

	if c.GitHub.InstallationID == 0 {
		return fmt.Errorf("GitHub App Installation ID is required")
	}

	if c.GitHub.PrivateKeySecret == "" {
		return fmt.Errorf("GitHub App private key secret path is required")
	}

	if c.Claude.AuthMode == "oauth" && c.Session.Agent != "claude-code" {
		return fmt.Errorf("oauth auth_mode is only supported with the claude-code agent")
	}

	return nil
}
