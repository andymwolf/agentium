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
	Phase         string // e.g., "IMPLEMENT", "TEST"
	SkillsPrompt  string // Composed from phase-relevant skills
	MemoryContext string // Summarized memory from previous iterations
	ModelOverride string // Model ID to pass as --model flag to the agent CLI
	Iteration     int    // Current iteration number
	SubTaskID     string // Unique ID for delegation tracking
}

// Session represents an agent session with all necessary context
type Session struct {
	ID             string
	Repository     string
	Tasks          []string
	PRs            []string // PR numbers for review sessions
	WorkDir        string
	GitHubToken    string
	MaxIterations  int
	MaxDuration    string
	Prompt         string
	Metadata       map[string]string
	ClaudeAuthMode   string            // "api" or "oauth"
	SystemPrompt     string            // Content of SYSTEM.md (safety constraints, workflow, status signals)
	ProjectPrompt    string            // Content of .agentium/AGENT.md from target repo (may be empty)
	ActiveTask       string            // The single issue number currently being worked on
	ExistingWork     *ExistingWork     // Prior work detected on GitHub for the active task
	IterationContext *IterationContext // Phase-aware skill context (nil = legacy mode)
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
	TokensUsed     int
	Events         []interface{} `json:"-"` // Structured events (type-assert per adapter)
	RawTextContent string        `json:"-"` // Aggregated text from structured output
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
