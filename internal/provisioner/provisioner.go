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

// ProviderCredential represents an OAuth token for an LLM provider
type ProviderCredential struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type,omitempty"`
}

// Credentials contains injected OAuth credentials for LLM providers
type Credentials struct {
	Anthropic *ProviderCredential `json:"anthropic,omitempty"`
	OpenAI    *ProviderCredential `json:"openai,omitempty"`
}

// PromptContext contains context for template variable substitution in prompts.
type PromptContext struct {
	IssueURL   string            `json:"issue_url,omitempty"`  // URL of the issue being worked on
	Parameters map[string]string `json:"parameters,omitempty"` // User-provided template parameters
}

// SessionConfig contains the session configuration to pass to the VM
type SessionConfig struct {
	ID             string                `json:"id"`
	CloudProvider  string                `json:"cloud_provider,omitempty"` // Cloud provider (gcp, aws, azure)
	Repository     string                `json:"repository"`
	Tasks          []string              `json:"tasks"`
	Agent          string                `json:"agent"`
	MaxDuration    string                `json:"max_duration"`
	Prompt         string                `json:"prompt"`
	PromptContext  *PromptContext        `json:"prompt_context,omitempty"` // Context for template variable substitution
	GitHub         GitHubConfig          `json:"github"`
	ClaudeAuth     ClaudeAuthConfig      `json:"claude_auth"`
	CodexAuth      CodexAuthConfig       `json:"codex_auth,omitempty"`
	Credentials    *Credentials          `json:"credentials,omitempty"` // Injected OAuth credentials for LLM providers
	Routing        *routing.PhaseRouting `json:"routing,omitempty"`
	Delegation     *ProvDelegationConfig `json:"delegation,omitempty"`
	PhaseLoop      *ProvPhaseLoopConfig  `json:"phase_loop,omitempty"`
	Fallback       *ProvFallbackConfig   `json:"fallback,omitempty"`
	Phases         []ProvPhaseStepConfig `json:"phases,omitempty"`
	AutoMerge      bool                  `json:"auto_merge,omitempty"`
	ContainerReuse bool                  `json:"container_reuse,omitempty"`
	Langfuse       *ProvLangfuseConfig   `json:"langfuse,omitempty"`
	Monorepo       *ProvMonorepoConfig   `json:"monorepo,omitempty"`
}

// SubAgentConfig specifies agent overrides for a delegated sub-task type.
type SubAgentConfig struct {
	Agent  string               `json:"agent,omitempty"`
	Model  *routing.ModelConfig `json:"model,omitempty"`
	Skills []string             `json:"skills,omitempty"`
}

// ProvDelegationConfig controls sub-agent delegation for provisioned sessions.
type ProvDelegationConfig struct {
	Enabled   bool                      `json:"enabled"`
	Strategy  string                    `json:"strategy"`
	SubAgents map[string]SubAgentConfig `json:"sub_agents,omitempty"`
}

// ProvPhaseLoopConfig contains phase loop configuration for provisioned sessions.
// Phase loop is enabled when this config is present (non-nil).
type ProvPhaseLoopConfig struct {
	PlanMaxIterations      int `json:"plan_max_iterations,omitempty"`
	ImplementMaxIterations int `json:"implement_max_iterations,omitempty"`
	ReviewMaxIterations    int `json:"review_max_iterations,omitempty"`
	DocsMaxIterations      int `json:"docs_max_iterations,omitempty"`
	VerifyMaxIterations    int `json:"verify_max_iterations,omitempty"`
	JudgeContextBudget     int `json:"judge_context_budget,omitempty"`
	JudgeNoSignalLimit     int `json:"judge_no_signal_limit,omitempty"`
}

// ProvMonorepoConfig contains monorepo settings for provisioned sessions.
type ProvMonorepoConfig struct {
	Enabled     bool                `json:"enabled"`
	LabelPrefix string              `json:"label_prefix"`
	Tiers       map[string][]string `json:"tiers,omitempty"`
}

// ProvPhaseStepConfig defines the configuration for a single phase step in provisioned sessions.
type ProvPhaseStepConfig struct {
	Name          string                 `json:"name"`
	MaxIterations int                    `json:"max_iterations,omitempty"`
	Worker        *ProvStepPromptConfig  `json:"worker,omitempty"`
	Reviewer      *ProvStepPromptConfig  `json:"reviewer,omitempty"`
	Judge         *ProvJudgePromptConfig `json:"judge,omitempty"`
}

// ProvStepPromptConfig contains an override prompt for a worker or reviewer step.
type ProvStepPromptConfig struct {
	Prompt string `json:"prompt"`
}

// ProvJudgePromptConfig contains override criteria for a judge step.
type ProvJudgePromptConfig struct {
	Criteria string `json:"criteria"`
}

// ProvLangfuseConfig contains Langfuse observability settings for provisioned sessions.
type ProvLangfuseConfig struct {
	PublicKeySecret string `json:"public_key_secret,omitempty"` // GCP Secret Manager path for public key
	SecretKeySecret string `json:"secret_key_secret,omitempty"` // GCP Secret Manager path for secret key
	BaseURL         string `json:"base_url,omitempty"`          // Langfuse API base URL
}

// ProvFallbackConfig controls adapter execution fallback for provisioned sessions.
type ProvFallbackConfig struct {
	Enabled bool `json:"enabled,omitempty"`
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
	EventType  string // Filter by event type (comma-separated: text,thinking,tool_use,tool_result,command,file_change,error,system)
	Iteration  int    // Filter by iteration number (0 = all iterations)
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

// New creates a new provisioner for the specified cloud provider.
// The serviceAccountKey parameter is optional; when set to a path to a GCP
// service account JSON key file, all terraform and gcloud commands will
// authenticate using that key instead of ambient credentials.
func New(provider string, verbose bool, project, serviceAccountKey string) (Provisioner, error) {
	switch provider {
	case "gcp":
		return NewGCPProvisioner(verbose, project, serviceAccountKey)
	case "aws":
		return nil, fmt.Errorf("AWS provisioner not yet implemented")
	case "azure":
		return nil, fmt.Errorf("azure provisioner not yet implemented")
	default:
		return nil, fmt.Errorf("unknown cloud provider: %s", provider)
	}
}
