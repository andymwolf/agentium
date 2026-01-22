package provisioner

import (
	"context"
	"fmt"
	"time"
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
	Region          string
	MachineType     string
	UseSpot         bool
	DiskSizeGB      int
	Session         SessionConfig
	ControllerImage string
}

// SessionConfig contains the session configuration to pass to the VM
type SessionConfig struct {
	ID            string
	Repository    string
	Tasks         []string
	PRs           []string
	Agent         string
	MaxIterations int
	MaxDuration   string
	Prompt        string
	GitHub        GitHubConfig
}

// GitHubConfig contains GitHub authentication configuration
type GitHubConfig struct {
	AppID            int64
	InstallationID   int64
	PrivateKeySecret string
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
