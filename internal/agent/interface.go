package agent

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
	ClaudeAuthMode string // "api" or "oauth"
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
