package provisioner

import (
	"context"
	"fmt"
	"time"

	"github.com/andywolf/agentium/internal/routing"
)

// Provisioner is the interface for cloud provisioning
type Provisioner interface {
	// Provision creates a new VM for an agent session
	Provision(ctx context.Context, config VMConfig) (*ProvisionResult, error)

	// List returns all active Agentium sessions
	List(ctx context.Context) ([]SessionStatus, error)

	// Status gets the current status of a session
	Status(ctx context.Context, sessionID string) (*SessionStatus, error)

	// Logs retrieves logs from a session
	Logs(ctx context.Context, sessionID string, opts LogsOptions) (<-chan LogEntry, <-chan error)

	// Destroy terminates a session and cleans up resources
	Destroy(ctx context.Context, sessionID string) error
}

// VMConfig contains configuration for provisioning a VM
type VMConfig struct {
	Project         string
	Region          string
	MachineType     string
	UseSpot         bool
	DiskSizeGB      int
	Session         SessionConfig
	ControllerImage string
}

// ClaudeAuthConfig contains Claude authentication configuration for the VM
type ClaudeAuthConfig struct {
	AuthMode       string `json:"auth_mode"`
	AuthJSONBase64 string `json:"auth_json_base64,omitempty"`
}

// CodexAuthConfig contains Codex authentication configuration for the VM
type CodexAuthConfig struct {
	AuthJSONBase64 string `json:"auth_json_base64,omitempty"`
}

// SessionConfig contains the session configuration to pass to the VM
type SessionConfig struct {
	ID            string                `json:"id"`
	Repository    string                `json:"repository"`
	Tasks         []string              `json:"tasks"`
	PRs           []string              `json:"prs,omitempty"`
	Agent         string                `json:"agent"`
	MaxIterations int                   `json:"max_iterations"`
	MaxDuration   string                `json:"max_duration"`
	Prompt        string                `json:"prompt"`
	GitHub        GitHubConfig          `json:"github"`
	ClaudeAuth    ClaudeAuthConfig      `json:"claude_auth"`
	CodexAuth     CodexAuthConfig       `json:"codex_auth,omitempty"`
	Routing       *routing.PhaseRouting `json:"routing,omitempty"`
	Delegation    *ProvDelegationConfig `json:"delegation,omitempty"`
	PhaseLoop     *ProvPhaseLoopConfig  `json:"phase_loop,omitempty"`
}

// SubAgentConfig specifies agent overrides for a delegated sub-task type.
type SubAgentConfig struct {
	Agent  string              `json:"agent,omitempty"`
	Model  *routing.ModelConfig `json:"model,omitempty"`
	Skills []string            `json:"skills,omitempty"`
}

// ProvDelegationConfig controls sub-agent delegation for provisioned sessions.
type ProvDelegationConfig struct {
	Enabled   bool                       `json:"enabled"`
	Strategy  string                     `json:"strategy"`
	SubAgents map[string]SubAgentConfig  `json:"sub_agents,omitempty"`
}

// ProvPhaseLoopConfig contains phase loop configuration for provisioned sessions.
type ProvPhaseLoopConfig struct {
	Enabled                 bool   `json:"enabled"`
	ReviewEnabled           bool   `json:"review_enabled,omitempty"`
	ReviewMode              string `json:"review_mode,omitempty"` // "always", "auto", "never", ""
	PlanMaxIterations       int    `json:"plan_max_iterations,omitempty"`
	ImplementMaxIterations  int    `json:"implement_max_iterations,omitempty"`
	TestMaxIterations       int    `json:"test_max_iterations,omitempty"`
	ReviewMaxIterations     int    `json:"review_max_iterations,omitempty"`
	DocsMaxIterations       int    `json:"docs_max_iterations,omitempty"`
	EvalContextBudget       int    `json:"eval_context_budget,omitempty"`
	EvalNoSignalLimit       int    `json:"eval_no_signal_limit,omitempty"`
}

// GitHubConfig contains GitHub authentication configuration
type GitHubConfig struct {
	AppID            int64  `json:"app_id"`
	InstallationID   int64  `json:"installation_id"`
	PrivateKeySecret string `json:"private_key_secret"`
}

// ProvisionResult contains the result of provisioning
type ProvisionResult struct {
	InstanceID string
	PublicIP   string
	Zone       string
	SessionID  string
}

// SessionStatus represents the current state of a session
type SessionStatus struct {
	SessionID        string
	State            string // running, completed, failed, terminated
	InstanceID       string
	PublicIP         string
	Zone             string
	StartTime        time.Time
	EndTime          time.Time
	CurrentIteration int
	MaxIterations    int
	CompletedTasks   []string
	PendingTasks     []string
	LastError        string
}

// LogsOptions configures log retrieval
type LogsOptions struct {
	Follow     bool
	Tail       int
	Since      time.Time
	ShowEvents bool   // Include agent events (tool calls, decisions)
	MinLevel   string // Minimum log level: debug, info, warning, error
}

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp time.Time
	Message   string
	Level     string
	Source    string
	EventType string // From labels.event_type (e.g., "tool_use", "text")
	ToolName  string // From labels.tool_name (e.g., "Bash", "Read")
}

// New creates a new provisioner for the specified cloud provider
func New(provider string, verbose bool, project string) (Provisioner, error) {
	switch provider {
	case "gcp":
		return NewGCPProvisioner(verbose, project)
	case "aws":
		return nil, fmt.Errorf("AWS provisioner not yet implemented")
	case "azure":
		return nil, fmt.Errorf("Azure provisioner not yet implemented")
	default:
		return nil, fmt.Errorf("unknown cloud provider: %s", provider)
	}
}
