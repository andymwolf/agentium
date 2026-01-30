package config

import (
	"fmt"
	"time"

	"github.com/andywolf/agentium/internal/routing"
	"github.com/spf13/viper"
)

// SubAgentConfigYAML specifies agent overrides for a delegated sub-task type in YAML config.
type SubAgentConfigYAML struct {
	Agent  string               `mapstructure:"agent"`
	Model  *routing.ModelConfig `mapstructure:"model"`
	Skills []string             `mapstructure:"skills"`
}

// DelegationConfigYAML controls sub-agent delegation in YAML config.
type DelegationConfigYAML struct {
	Enabled   bool                          `mapstructure:"enabled"`
	Strategy  string                        `mapstructure:"strategy"`
	SubAgents map[string]SubAgentConfigYAML `mapstructure:"sub_agents"`
}

// PhaseLoopConfig contains phase loop configuration in YAML config.
type PhaseLoopConfig struct {
	Enabled                bool `mapstructure:"enabled"`
	SkipPlanIfExists       bool `mapstructure:"skip_plan_if_exists"`
	PlanMaxIterations      int  `mapstructure:"plan_max_iterations"`
	ImplementMaxIterations int  `mapstructure:"implement_max_iterations"`
	ReviewMaxIterations    int  `mapstructure:"review_max_iterations"`
	DocsMaxIterations      int  `mapstructure:"docs_max_iterations"`
	JudgeContextBudget     int  `mapstructure:"judge_context_budget"`
	JudgeNoSignalLimit     int  `mapstructure:"judge_no_signal_limit"`
}

// CodexConfig contains Codex agent authentication settings
type CodexConfig struct {
	AuthJSONPath string `mapstructure:"auth_json_path"` // Path to auth.json (default: ~/.codex/auth.json)
}

// MonorepoConfig contains monorepo-specific settings for pnpm workspaces
type MonorepoConfig struct {
	Enabled     bool   `mapstructure:"enabled"`      // Set by agentium init when pnpm-workspace.yaml is detected
	LabelPrefix string `mapstructure:"label_prefix"` // Prefix for package labels (default: "pkg")
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
	Codex      CodexConfig          `mapstructure:"codex"`
	Routing    routing.PhaseRouting `mapstructure:"routing"`
	Delegation DelegationConfigYAML `mapstructure:"delegation"`
	PhaseLoop  PhaseLoopConfig      `mapstructure:"phase_loop"`
	Monorepo   MonorepoConfig       `mapstructure:"monorepo"`
}

// ClaudeConfig contains Claude AI authentication settings
type ClaudeConfig struct {
	AuthMode     string `mapstructure:"auth_mode"`      // "api" (default) or "oauth"
	AuthJSONPath string `mapstructure:"auth_json_path"` // Path to auth.json
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

	if cfg.Codex.AuthJSONPath == "" {
		cfg.Codex.AuthJSONPath = "~/.codex/auth.json"
	}

	// Enable phase loop by default for structured PLAN → IMPLEMENT → REVIEW → PR workflow
	// This can be disabled in config with phase_loop.enabled: false
	if !cfg.PhaseLoop.Enabled {
		// Only set default if not explicitly configured
		// Check if any phase loop fields are set to detect explicit configuration
		if cfg.PhaseLoop.PlanMaxIterations == 0 &&
			cfg.PhaseLoop.ImplementMaxIterations == 0 &&
			cfg.PhaseLoop.ReviewMaxIterations == 0 &&
			cfg.PhaseLoop.DocsMaxIterations == 0 {
			cfg.PhaseLoop.Enabled = true
		}
	}

	// Default monorepo label prefix
	if cfg.Monorepo.LabelPrefix == "" {
		cfg.Monorepo.LabelPrefix = "pkg"
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
		validAgents := map[string]bool{"claude-code": true, "aider": true, "codex": true}
		if !validAgents[c.Session.Agent] {
			return fmt.Errorf("invalid agent: %s (must be claude-code, aider, or codex)", c.Session.Agent)
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

	if len(c.Session.Tasks) == 0 {
		return fmt.Errorf("at least one issue is required")
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

// ValidateForLocalRun performs relaxed validation for local interactive mode.
// It skips GitHub App requirements since authentication uses GITHUB_TOKEN env var.
func (c *Config) ValidateForLocalRun() error {
	if c.Session.Repository == "" {
		return fmt.Errorf("repository is required")
	}

	if len(c.Session.Tasks) == 0 {
		return fmt.Errorf("at least one issue is required")
	}

	// Validate agent if specified
	if c.Session.Agent != "" {
		validAgents := map[string]bool{"claude-code": true, "aider": true, "codex": true}
		if !validAgents[c.Session.Agent] {
			return fmt.Errorf("invalid agent: %s (must be claude-code, aider, or codex)", c.Session.Agent)
		}
	}

	// Validate max_duration format if specified
	if c.Session.MaxDuration != "" {
		if _, err := time.ParseDuration(c.Session.MaxDuration); err != nil {
			return fmt.Errorf("invalid max_duration: %w", err)
		}
	}

	if c.Claude.AuthMode == "oauth" && c.Session.Agent != "claude-code" {
		return fmt.Errorf("oauth auth_mode is only supported with the claude-code agent")
	}

	return nil
}
