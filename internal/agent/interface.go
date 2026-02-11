package agent

// ExistingWork represents prior work detected on GitHub for a given issue
type ExistingWork struct {
	Branch   string // Existing remote branch name (e.g. "agentium/issue-6-cloud-logging")
	PRNumber string // Existing open PR number (e.g. "87")
	PRTitle  string // Title of the existing PR
}

// IterationContext provides phase-aware context for a single iteration.
// When non-nil, SkillsPrompt should be preferred over Session.SystemPrompt.
type IterationContext struct {
	Phase             string // e.g., "IMPLEMENT", "TEST"
	SkillsPrompt      string // Composed from phase-relevant skills
	MemoryContext     string // Summarized memory from previous iterations (legacy mode)
	PhaseInput        string // Structured handoff input for this phase (handoff mode)
	ModelOverride     string // Model ID to pass as --model flag to the agent CLI
	ReasoningOverride string // Reasoning level for agents that support it (codex: model_reasoning_effort)
	Iteration         int    // Current iteration number
	SubTaskID         string // Unique ID for delegation tracking
}

// InjectedCredentials contains OAuth tokens injected from the task request.
// These are used instead of system-level API keys when present.
type InjectedCredentials struct {
	AnthropicAccessToken string // Anthropic OAuth access token
	OpenAIAccessToken    string // OpenAI OAuth access token
}

// Session represents an agent session with all necessary context
type Session struct {
	ID               string
	Repository       string
	Tasks            []string
	PRs              []string // PR numbers for review sessions
	WorkDir          string
	GitHubToken      string
	MaxDuration      string
	Prompt           string
	Metadata         map[string]string
	ClaudeAuthMode   string               // "api" or "oauth"
	SystemPrompt     string               // Content of SYSTEM.md (safety constraints, workflow, status signals)
	ProjectPrompt    string               // Content of .agentium/AGENTS.md from target repo (may be empty)
	ActiveTask       string               // The single issue number currently being worked on
	ExistingWork     *ExistingWork        // Prior work detected on GitHub for the active task
	IterationContext *IterationContext    // Phase-aware skill context (nil = legacy mode)
	Interactive      bool                 // When true, omit auto-accept permission flags
	PackagePath      string               // Monorepo: relative path from repo root (e.g., "packages/core")
	Credentials      *InjectedCredentials // Injected OAuth credentials (takes precedence over system API keys)
}

// IterationResult represents the outcome of a single agent iteration
type IterationResult struct {
	ExitCode       int
	Success        bool
	TasksCompleted []string
	PRsCreated     []string
	PushedChanges  bool   // True if git push was detected (for PR review sessions)
	AgentStatus    string // Status signal: TESTS_PASSED, PR_CREATED, PUSHED, COMPLETE, etc.
	StatusMessage  string // Optional message accompanying the status signal
	Error          string
	Summary        string
	TokensUsed     int           // Total tokens (sum of InputTokens + OutputTokens, kept for backward compatibility)
	InputTokens    int           // Input tokens consumed this iteration
	OutputTokens   int           // Output tokens consumed this iteration
	Events         []interface{} `json:"-"` // Structured events (type-assert per adapter)
	RawTextContent string        `json:"-"` // Aggregated text from structured output (includes tool results)
	AssistantText  string        `json:"-"` // Only assistant text blocks (for readable GitHub comments)
	HandoffOutput  string        `json:"-"` // Raw AGENTIUM_HANDOFF JSON if present
}

// Agent defines the interface that all agent adapters must implement
type Agent interface {
	// Name returns the agent identifier
	Name() string

	// ContainerImage returns the Docker image for this agent
	ContainerImage() string

	// BuildEnv constructs environment variables for the agent container
	BuildEnv(session *Session, iteration int) map[string]string

	// BuildCommand constructs the command to run in the agent container
	BuildCommand(session *Session, iteration int) []string

	// BuildPrompt constructs the prompt for the agent
	BuildPrompt(session *Session, iteration int) string

	// ParseOutput parses the agent's output to determine results
	ParseOutput(exitCode int, stdout, stderr string) (*IterationResult, error)

	// Validate checks if the agent configuration is valid
	Validate() error
}

// StdinPromptProvider is an optional interface for agents that need stdin-based prompt delivery.
// Agents can implement this to provide the prompt via stdin rather than command-line args.
// This is useful for non-interactive mode with --print flag where TTY issues may prevent
// prompt delivery via positional arguments.
type StdinPromptProvider interface {
	// GetStdinPrompt returns the prompt to pipe via stdin.
	// Returns empty string if prompt should be passed via command-line args.
	GetStdinPrompt(session *Session, iteration int) string
}

// ContinuationCapable is an optional interface for agents that support conversation
// continuation within a long-lived container. When supported, the controller can
// use --continue (or equivalent) to resume the conversation context from a previous
// invocation, avoiding full prompt reconstruction on iterations 2+.
type ContinuationCapable interface {
	// SupportsContinuation returns true if the adapter supports --continue mode.
	SupportsContinuation() bool

	// BuildContinueCommand constructs the command for continuation mode.
	// This typically omits --system-prompt and --append-system-prompt (they carry
	// over from the first invocation) and adds the --continue flag.
	BuildContinueCommand(session *Session, iteration int) []string
}

// PlanModeCapable is an optional interface for agents that support plan-only mode.
// This can be used to signal read-only planning capabilities.
type PlanModeCapable interface {
	// SupportsPlanMode returns true if the adapter can enforce plan-only mode.
	SupportsPlanMode() bool
}
