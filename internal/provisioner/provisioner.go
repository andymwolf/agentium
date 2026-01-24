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
	Routing       *routing.PhaseRouting `json:"routing,omitempty"`
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
	Follow bool
	Tail   int
	Since  time.Time
}

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp time.Time
	Message   string
	Level     string
	Source    string
}

// New creates a new provisioner for the specified cloud provider
func New(provider string, verbose bool) (Provisioner, error) {
	switch provider {
	case "gcp":
		return NewGCPProvisioner(verbose)
	case "aws":
		return nil, fmt.Errorf("AWS provisioner not yet implemented")
	case "azure":
		return nil, fmt.Errorf("Azure provisioner not yet implemented")
	default:
		return nil, fmt.Errorf("unknown cloud provider: %s", provider)
	}
}
