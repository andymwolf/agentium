package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/andywolf/agentium/internal/agent"
	_ "github.com/andywolf/agentium/internal/agent/aider"
	_ "github.com/andywolf/agentium/internal/agent/claudecode"
	_ "github.com/andywolf/agentium/internal/agent/codex"
	"github.com/andywolf/agentium/internal/agent/event"
	"github.com/andywolf/agentium/internal/cloud/gcp"
	"github.com/andywolf/agentium/internal/github"
	"github.com/andywolf/agentium/internal/handoff"
	"github.com/andywolf/agentium/internal/memory"
	"github.com/andywolf/agentium/internal/observability"
	"github.com/andywolf/agentium/internal/prompt"
	"github.com/andywolf/agentium/internal/routing"
	"github.com/andywolf/agentium/internal/scope"
	"github.com/andywolf/agentium/internal/template"
	"github.com/andywolf/agentium/internal/version"
	"github.com/andywolf/agentium/internal/workspace"
	"github.com/andywolf/agentium/prompts/skills"
)

const (
	// ShutdownTimeout is the maximum time to wait for graceful shutdown
	ShutdownTimeout = 30 * time.Second

	// LogFlushTimeout is the maximum time to wait for log flush operations
	LogFlushTimeout = 10 * time.Second

	// VMTerminationTimeout is the maximum time to wait for the VM deletion command
	VMTerminationTimeout = 30 * time.Second

	// AgentiumUID is the user ID for the agentium user in agent containers.
	// This must match the UID in docker/claudecode/Dockerfile, docker/aider/Dockerfile,
	// and docker/codex/Dockerfile where the agentium user is created with useradd -u 1000.
	AgentiumUID = 1000

	// AgentiumGID is the group ID for the agentium group in agent containers.
	// This must match the GID in the agent Dockerfiles (defaults to same as UID).
	AgentiumGID = 1000

	// MaxReviewBodyLen is the maximum length for review body text before truncation.
	MaxReviewBodyLen = 500

	// MaxCommentBodyLen is the maximum length for inline comment text before truncation.
	MaxCommentBodyLen = 300

	// MaxIssueBodyLen is the maximum length for issue body text before truncation.
	MaxIssueBodyLen = 1000
)

// TaskPhase represents the current phase of a task in its lifecycle
type TaskPhase string

const (
	PhasePlan        TaskPhase = "PLAN"
	PhaseImplement   TaskPhase = "IMPLEMENT"
	PhaseDocs        TaskPhase = "DOCS"
	PhaseVerify      TaskPhase = "VERIFY"
	PhaseComplete    TaskPhase = "COMPLETE"
	PhaseBlocked     TaskPhase = "BLOCKED"
	PhaseNothingToDo TaskPhase = "NOTHING_TO_DO"
)

// WorkflowPath represents the complexity path determined after PLAN iteration 1.
type WorkflowPath string

const (
	WorkflowPathUnset   WorkflowPath = ""        // Not yet determined
	WorkflowPathSimple  WorkflowPath = "SIMPLE"  // Straightforward change, fewer iterations
	WorkflowPathComplex WorkflowPath = "COMPLEX" // Multiple components, full review
)

// TaskState tracks the current state of a task being worked on.
//
// Phase vs LastStatus:
//   - Phase represents the derived workflow state (e.g., PhasePlan, PhaseImplement, PhaseComplete).
//     It is computed by the controller based on agent signals and phase transitions.
//   - LastStatus stores the raw agent signal string (e.g., "TESTS_PASSED", "PR_CREATED", "BLOCKED").
//     This preserves the exact signal emitted by the agent for debugging and audit purposes.
//
// Example: When an agent emits "TESTS_PASSED", LastStatus is set to "TESTS_PASSED" while
// Phase transitions to PhaseDocs.
type TaskState struct {
	ID                    string
	Type                  string    // "issue" or "pr"
	Phase                 TaskPhase // Derived workflow state (computed from signals and phase transitions)
	TestRetries           int
	LastStatus            string       // Raw agent signal string for debugging/audit (e.g., "TESTS_PASSED")
	PRNumber              string       // Linked PR number (for issues that create PRs)
	PhaseIteration        int          // Current iteration within the active phase (phase loop)
	MaxPhaseIterations    int          // Max iterations for current phase (phase loop)
	LastJudgeVerdict      string       // Last judge verdict (ADVANCE, ITERATE, BLOCKED)
	LastJudgeFeedback     string       // Last judge feedback text
	DraftPRCreated        bool         // Whether draft PR has been created for this task
	WorkflowPath          WorkflowPath // Set after PLAN iteration 1 (SIMPLE or COMPLEX)
	ControllerOverrode    bool         // True if controller forced ADVANCE at max iterations (triggers NOMERGE)
	JudgeOverrodeReviewer bool         // True if judge ADVANCE overrode reviewer ITERATE/BLOCKED (triggers NOMERGE)
	PRMerged              bool         // True if auto-merge successfully merged the PR
	ParentBranch          string       // Parent issue's branch to base this task on (for dependency chains)
}

// PhaseLoopConfig controls the controller-as-judge phase loop behavior.
type PhaseLoopConfig struct {
	SkipPlanIfExists       bool   `json:"skip_plan_if_exists,omitempty"`
	PlanMaxIterations      int    `json:"plan_max_iterations,omitempty"`
	ImplementMaxIterations int    `json:"implement_max_iterations,omitempty"`
	DocsMaxIterations      int    `json:"docs_max_iterations,omitempty"`
	VerifyMaxIterations    int    `json:"verify_max_iterations,omitempty"`
	JudgeContextBudget     int    `json:"judge_context_budget,omitempty"`
	JudgeNoSignalLimit     int    `json:"judge_no_signal_limit,omitempty"`
	ReviewerSkip           bool   `json:"reviewer_skip,omitempty"`
	JudgeSkip              bool   `json:"judge_skip,omitempty"`
	ReviewerSkipOn         string `json:"reviewer_skip_on,omitempty"`
	JudgeSkipOn            string `json:"judge_skip_on,omitempty"`
}

// FallbackConfig controls adapter execution fallback behavior.
type FallbackConfig struct {
	Enabled bool `json:"enabled,omitempty"` // Enable fallback on adapter failure
}

// DefaultFallbackAdapter is the default adapter used for fallback when none is specified.
const DefaultFallbackAdapter = "claude-code"

// PromptContext contains context for template variable substitution in prompts.
type PromptContext struct {
	IssueURL   string            `json:"issue_url,omitempty"`  // URL of the issue being worked on
	Parameters map[string]string `json:"parameters,omitempty"` // User-provided template parameters
}

// SessionConfig is the configuration passed to the controller
type SessionConfig struct {
	ID                   string         `json:"id"`
	CloudProvider        string         `json:"cloud_provider,omitempty"` // Cloud provider (gcp, aws, azure, local)
	Repository           string         `json:"repository"`
	Tasks                []string       `json:"tasks"`
	Agent                string         `json:"agent"`
	MaxIterations        int            `json:"max_iterations"`
	MaxDuration          string         `json:"max_duration"`
	Prompt               string         `json:"prompt"`
	PromptContext        *PromptContext `json:"prompt_context,omitempty"`         // Context for template variable substitution
	Interactive          bool           `json:"interactive,omitempty"`            // Local interactive mode (no cloud clients)
	CloneInsideContainer bool           `json:"clone_inside_container,omitempty"` // Clone repository inside Docker container
	GitHub               struct {
		AppID            int64  `json:"app_id"`
		InstallationID   int64  `json:"installation_id"`
		PrivateKeySecret string `json:"private_key_secret"`
	} `json:"github"`
	ClaudeAuth struct {
		AuthMode       string `json:"auth_mode"`
		AuthJSONBase64 string `json:"auth_json_base64,omitempty"`
	} `json:"claude_auth"`
	CodexAuth struct {
		AuthJSONBase64 string `json:"auth_json_base64,omitempty"`
	} `json:"codex_auth"`
	Skills struct {
		Enabled bool `json:"enabled,omitempty"`
	} `json:"skills,omitempty"`
	Memory struct {
		Enabled       bool `json:"enabled,omitempty"`
		MaxEntries    int  `json:"max_entries,omitempty"`
		ContextBudget int  `json:"context_budget,omitempty"`
	} `json:"memory,omitempty"`
	Handoff    struct{}               `json:"handoff,omitempty"` // Kept for config compatibility; handoff is always enabled
	Routing    *routing.PhaseRouting  `json:"routing,omitempty"`
	Delegation *DelegationConfig      `json:"delegation,omitempty"`
	PhaseLoop  *PhaseLoopConfig       `json:"phase_loop,omitempty"`
	Fallback   *FallbackConfig        `json:"fallback,omitempty"`
	Verbose    bool                   `json:"verbose,omitempty"`
	AutoMerge  bool                   `json:"auto_merge,omitempty"`
	Monorepo   *MonorepoSessionConfig `json:"monorepo,omitempty"`
}

// MonorepoSessionConfig contains monorepo-specific settings for the session.
type MonorepoSessionConfig struct {
	Enabled     bool                `json:"enabled"`
	LabelPrefix string              `json:"label_prefix"` // Default: "pkg"
	Tiers       map[string][]string `json:"tiers,omitempty"`
}

// DefaultConfigPath is the default path for the session config file
const DefaultConfigPath = "/etc/agentium/session.json"

// LoadConfig loads the session configuration from environment or file.
// It first checks for AGENTIUM_SESSION_CONFIG env var (JSON string),
// then falls back to reading from a file path specified by AGENTIUM_CONFIG_PATH
// or the default path /etc/agentium/session.json.
func LoadConfig() (SessionConfig, error) {
	return LoadConfigFromEnv(os.Getenv, os.ReadFile)
}

// LoadConfigFromEnv loads config using the provided getenv and readFile functions.
// This allows for easier testing by injecting mock implementations.
func LoadConfigFromEnv(getenv func(string) string, readFile func(string) ([]byte, error)) (SessionConfig, error) {
	var config SessionConfig

	// Try environment variable first
	if configJSON := getenv("AGENTIUM_SESSION_CONFIG"); configJSON != "" {
		if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
			return config, fmt.Errorf("failed to parse AGENTIUM_SESSION_CONFIG: %w", err)
		}
		return config, nil
	}

	// Try config file
	configPath := getenv("AGENTIUM_CONFIG_PATH")
	if configPath == "" {
		configPath = DefaultConfigPath
	}

	data, err := readFile(configPath)
	if err != nil {
		return config, fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	if err := json.Unmarshal(data, &config); err != nil {
		return config, fmt.Errorf("failed to parse config file: %w", err)
	}

	return config, nil
}

// TaskQueueItem represents a single item in the unified task queue
type TaskQueueItem struct {
	Type string // "pr" or "issue"
	ID   string // PR number or issue number
}

// ShutdownHook is a function that will be called during graceful shutdown.
// It receives a context with timeout and should respect cancellation.
type ShutdownHook func(ctx context.Context) error

// Controller manages the agent execution lifecycle
type Controller struct {
	config                 SessionConfig
	agent                  agent.Agent
	workDir                string
	iteration              int
	startTime              time.Time
	maxDuration            time.Duration
	gitHubToken            string
	tokenManager           *github.TokenManager // Manages token refresh for long-running sessions (nil for static tokens)
	dockerAuthed           bool                 // Tracks if docker login to GHCR was done
	taskStates             map[string]*TaskState
	logger                 *log.Logger
	cloudLogger            *gcp.CloudLogger // Structured cloud logging (may be nil if unavailable)
	secretManager          gcp.SecretFetcher
	systemPrompt           string                  // Loaded SYSTEM.md content
	projectPrompt          string                  // Loaded .agentium/AGENTS.md content (may be empty)
	taskQueue              []TaskQueueItem         // Task queue for issues
	issueDetails           []issueDetail           // Fetched issue details for prompt building
	issueDetailsByNumber   map[string]*issueDetail // O(1) lookup by issue number string
	activeTask             string                  // Current task ID being focused on
	activeTaskType         string                  // "issue"
	activeTaskExistingWork *agent.ExistingWork     // Existing work detected for active task (issues only)
	skillSelector          *skills.Selector        // Phase-aware skill selector (nil = legacy mode)
	memoryStore            *memory.Store           // Persistent memory store (nil = disabled)
	handoffStore           *handoff.Store          // Structured handoff store (nil = disabled)
	handoffBuilder         *handoff.Builder        // Phase input builder (nil = disabled)
	handoffParser          *handoff.Parser         // Handoff signal parser (nil = disabled)
	handoffValidator       *handoff.Validator      // Handoff validation (nil = disabled)
	modelRouter            *routing.Router         // Phase-to-model routing (nil = no routing)
	depGraph               *DependencyGraph        // Inter-issue dependency graph (nil = no dependencies)
	adapters               map[string]agent.Agent  // All initialized adapters (for multi-adapter routing)
	orchestrator           *SubTaskOrchestrator    // Sub-task delegation orchestrator (nil = disabled)
	metadataUpdater        gcp.MetadataUpdater     // Instance metadata updater (nil if unavailable)
	eventSink              *event.FileSink         // Local JSONL event sink (nil = disabled)
	tracer                 observability.Tracer    // Langfuse observability tracer (never nil; NoOpTracer if disabled)

	// Monorepo support
	packagePath    string                // Current package path for monorepo scope (empty if not monorepo)
	scopeValidator *scope.ScopeValidator // Validates file changes are within package scope (nil if not monorepo)

	// Tracker support
	trackerSubIssues map[string][]string // tracker issue ID -> sub-issue IDs

	// Shutdown management
	shutdownHooks []ShutdownHook
	shutdownOnce  sync.Once
	shutdownCh    chan struct{} // Closed when shutdown is initiated
	logFlushFn    func() error  // Function to flush pending logs

	// cmdRunner executes external commands. Defaults to exec.CommandContext.
	// Override in tests to mock command execution.
	cmdRunner func(ctx context.Context, name string, args ...string) *exec.Cmd
}

// taskKey creates a composite key from task type and ID (e.g., "issue:123" or "pr:456").
func taskKey(typ, id string) string {
	return typ + ":" + id
}

// envWithGitHubToken returns os.Environ() with the GITHUB_TOKEN appended.
func (c *Controller) envWithGitHubToken() []string {
	return append(os.Environ(), "GITHUB_TOKEN="+c.gitHubToken)
}

// New creates a new session controller
func New(config SessionConfig) (*Controller, error) {
	// Get the agent adapter
	agentAdapter, err := agent.Get(config.Agent)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}

	// Parse max duration
	maxDuration, err := time.ParseDuration(config.MaxDuration)
	if err != nil {
		maxDuration = 2 * time.Hour
	}

	workDir := os.Getenv("AGENTIUM_WORKDIR")
	if workDir == "" {
		workDir = "/workspace"
	}

	// Create logger early so we can use it during initialization
	logger := log.New(os.Stdout, "[controller] ", log.LstdFlags)

	// In interactive mode, skip cloud client initialization
	var secretManager gcp.SecretFetcher
	var cloudLogger *gcp.CloudLogger
	var metadataUpdater gcp.MetadataUpdater

	if !config.Interactive {
		// Initialize Secret Manager client
		secretManager, err = gcp.NewSecretManagerClient(context.Background())
		if err != nil {
			// Log warning but don't fail - allow fallback to gcloud CLI
			logger.Printf("Warning: failed to initialize Secret Manager client: %v", err)
		}

		// Initialize Cloud Logging (non-fatal if unavailable)
		cloudLoggerInstance, err := gcp.NewCloudLogger(context.Background(), gcp.CloudLoggerConfig{
			SessionID:  config.ID,
			Repository: config.Repository,
		})
		if err != nil {
			logger.Printf("Warning: Cloud Logging unavailable, using local logs only: %v", err)
		} else {
			cloudLogger = cloudLoggerInstance
		}

		// Initialize metadata updater (only on GCP instances)
		if gcp.IsRunningOnGCP() {
			metadataUpdaterInstance, err := gcp.NewComputeMetadataUpdater(context.Background())
			if err != nil {
				logger.Printf("Warning: metadata updater unavailable, session status will not be reported: %v", err)
			} else {
				metadataUpdater = metadataUpdaterInstance
			}
		}
	}

	c := &Controller{
		config:           config,
		agent:            agentAdapter,
		workDir:          workDir,
		iteration:        0,
		maxDuration:      maxDuration,
		taskStates:       make(map[string]*TaskState),
		logger:           logger,
		cloudLogger:      cloudLogger,
		secretManager:    secretManager,
		metadataUpdater:  metadataUpdater,
		shutdownCh:       make(chan struct{}),
		trackerSubIssues: make(map[string][]string),
		tracer:           &observability.NoOpTracer{},
	}

	// Initialize Langfuse tracer if configured via environment variables
	c.initTracer(logger)

	// Initialize task states and build task queue
	initialIssuePhase := PhaseImplement
	if c.isPhaseLoopEnabled() {
		initialIssuePhase = PhasePlan
	}
	for _, task := range config.Tasks {
		c.taskStates[taskKey("issue", task)] = &TaskState{
			ID:    task,
			Type:  "issue",
			Phase: initialIssuePhase,
		}
		c.taskQueue = append(c.taskQueue, TaskQueueItem{Type: "issue", ID: task})
	}

	// Initialize model routing
	c.modelRouter = routing.NewRouter(config.Routing)
	c.adapters = map[string]agent.Agent{
		config.Agent: agentAdapter,
	}
	if c.modelRouter.IsConfigured() {
		if unknowns := c.modelRouter.UnknownPhases(); len(unknowns) > 0 {
			c.logWarning("routing config references unknown phases: %v (valid: %v)", unknowns, routing.ValidPhaseNames())
		}
		for _, name := range c.modelRouter.Adapters() {
			if _, exists := c.adapters[name]; !exists {
				a, err := agent.Get(name)
				if err != nil {
					return nil, fmt.Errorf("failed to initialize routed adapter %q: %w", name, err)
				}
				c.adapters[name] = a
			}
		}
	}

	// Initialize delegation orchestrator
	if config.Delegation != nil && config.Delegation.Enabled {
		if config.Delegation.Strategy != "" && config.Delegation.Strategy != "sequential" {
			c.logWarning("delegation strategy %q not supported, falling back to sequential", config.Delegation.Strategy)
		}
		c.orchestrator = NewSubTaskOrchestrator(*config.Delegation, c)
		// Pre-initialize adapters referenced in delegation config
		for _, subCfg := range config.Delegation.SubAgents {
			if subCfg.Agent != "" {
				if _, exists := c.adapters[subCfg.Agent]; !exists {
					a, err := agent.Get(subCfg.Agent)
					if err != nil {
						return nil, fmt.Errorf("failed to initialize delegated adapter %q: %w", subCfg.Agent, err)
					}
					c.adapters[subCfg.Agent] = a
				}
			}
		}
	}

	// Initialize fallback adapter if configured
	if config.Fallback != nil && config.Fallback.Enabled {
		if _, exists := c.adapters[DefaultFallbackAdapter]; !exists {
			a, err := agent.Get(DefaultFallbackAdapter)
			if err != nil {
				return nil, fmt.Errorf("failed to initialize fallback adapter %q: %w", DefaultFallbackAdapter, err)
			}
			c.adapters[DefaultFallbackAdapter] = a
			c.logInfo("Initialized fallback adapter: %s", DefaultFallbackAdapter)
		}
	}

	return c, nil
}

// logInfo logs at INFO level to both local logger and cloud logger
func (c *Controller) logInfo(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	c.logger.Printf("%s", msg)
	if c.cloudLogger != nil {
		c.cloudLogger.Info(msg)
	}
}

// logWarning logs at WARNING level to both local logger and cloud logger
func (c *Controller) logWarning(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	c.logger.Printf("Warning: %s", msg)
	if c.cloudLogger != nil {
		c.cloudLogger.Warning(msg)
	}
}

// logError logs at ERROR level to both local logger and cloud logger
func (c *Controller) logError(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	c.logger.Printf("Error: %s", msg)
	if c.cloudLogger != nil {
		c.cloudLogger.Error(msg)
	}
}

// logTokenConsumption logs token usage for a completed iteration to Cloud Logging.
func (c *Controller) logTokenConsumption(result *agent.IterationResult, agentName string, session *agent.Session) {
	if c.cloudLogger == nil {
		return
	}
	if result.InputTokens == 0 && result.OutputTokens == 0 {
		return
	}

	taskID := taskKey(c.activeTaskType, c.activeTask)
	phase := ""
	if state, ok := c.taskStates[taskID]; ok && state != nil {
		phase = string(state.Phase)
	}

	labels := map[string]string{
		"log_type":      "token_usage",
		"task_id":       taskID,
		"phase":         phase,
		"agent":         agentName,
		"input_tokens":  strconv.Itoa(result.InputTokens),
		"output_tokens": strconv.Itoa(result.OutputTokens),
		"total_tokens":  strconv.Itoa(result.InputTokens + result.OutputTokens),
	}

	if session.IterationContext != nil && session.IterationContext.ModelOverride != "" {
		labels["model"] = session.IterationContext.ModelOverride
	}

	msg := fmt.Sprintf("Token usage: input=%d output=%d total=%d",
		result.InputTokens, result.OutputTokens, result.InputTokens+result.OutputTokens)

	c.cloudLogger.LogWithLabels(gcp.SeverityInfo, msg, labels)
}

// initTracer initializes the Langfuse observability tracer from environment variables.
// If LANGFUSE_PUBLIC_KEY and LANGFUSE_SECRET_KEY are set (and LANGFUSE_ENABLED is not "false"),
// a LangfuseTracer is created. Otherwise the default NoOpTracer is kept.
func (c *Controller) initTracer(logger *log.Logger) {
	publicKey := os.Getenv("LANGFUSE_PUBLIC_KEY")
	secretKey := os.Getenv("LANGFUSE_SECRET_KEY")

	if publicKey == "" || secretKey == "" {
		return
	}

	if os.Getenv("LANGFUSE_ENABLED") == "false" {
		logger.Printf("Langfuse: disabled via LANGFUSE_ENABLED=false")
		return
	}

	baseURL := os.Getenv("LANGFUSE_BASE_URL")

	lt := observability.NewLangfuseTracer(observability.LangfuseConfig{
		PublicKey: publicKey,
		SecretKey: secretKey,
		BaseURL:   baseURL,
	}, logger)

	c.tracer = lt
	c.AddShutdownHook(func(ctx context.Context) error {
		return c.tracer.Stop(ctx)
	})
	logger.Printf("Langfuse: tracer initialized (base_url=%s)", lt.BaseURL())
}

// AddShutdownHook registers a function to be called during graceful shutdown.
// Hooks are executed in the order they were added.
func (c *Controller) AddShutdownHook(hook ShutdownHook) {
	c.shutdownHooks = append(c.shutdownHooks, hook)
}

// SetLogFlushFunc sets the function used to flush pending log writes.
// This is called with a timeout during shutdown to ensure logs are persisted.
func (c *Controller) SetLogFlushFunc(fn func() error) {
	c.logFlushFn = fn
}

// Run executes the main control loop
func (c *Controller) Run(ctx context.Context) error {
	c.startTime = time.Now()

	// Set up signal handling for graceful shutdown
	ctx, cancel := c.setupSignalHandler(ctx)
	defer cancel()

	// Initialize session (workspace, credentials, repository, prompts, task details)
	if err := c.initSession(ctx); err != nil {
		return err
	}

	// Start background resource monitor (logs memory pressure warnings)
	go c.startResourceMonitor(ctx)

	// Run main task processing loop
	if err := c.runMainLoop(ctx); err != nil {
		return err
	}

	// Emit final logs
	c.emitFinalLogs()

	// Cleanup
	c.cleanup()

	return nil
}

// initSession performs all initialization steps before the main loop:
// - Logs session configuration
// - Initializes workspace directory
// - Fetches GitHub credentials
// - Clones repository
// - Loads prompts and skills
// - Fetches task details (issues/PRs)
// - Builds dependency graph
func (c *Controller) initSession(ctx context.Context) error {
	c.logInfo("Controller started (%s)", version.Info())
	c.logInfo("Starting session %s", c.config.ID)
	c.logInfo("Repository: %s", c.config.Repository)
	if len(c.config.Tasks) > 0 {
		c.logInfo("Tasks: %v", c.config.Tasks)
	}
	c.logInfo("Agent: %s", c.config.Agent)
	c.logInfo("Max iterations: %d", c.config.MaxIterations)
	c.logInfo("Max duration: %s", c.maxDuration)

	// Initialize workspace
	if err := c.initializeWorkspace(ctx); err != nil {
		return fmt.Errorf("failed to initialize workspace: %w", err)
	}

	// Fetch GitHub token
	if err := c.fetchGitHubToken(ctx); err != nil {
		return fmt.Errorf("failed to fetch GitHub token: %w", err)
	}

	// Pre-pull agent container images to avoid first-iteration latency
	c.prePullAgentImages(ctx)

	// Clone repository (skip if cloning inside container)
	if !c.config.CloneInsideContainer {
		if err := c.cloneRepository(ctx); err != nil {
			return fmt.Errorf("failed to clone repository: %w", err)
		}
	} else {
		c.logInfo("Skipping host-side clone (will clone inside container)")
	}

	// Load system and project prompts
	if err := c.loadPrompts(); err != nil {
		return fmt.Errorf("failed to load prompts: %w", err)
	}

	// Fetch all task details upfront
	if len(c.config.Tasks) > 0 {
		c.issueDetails = c.fetchIssueDetails(ctx)
	}

	// Build inter-issue dependency graph (only for multi-issue batches)
	if len(c.issueDetails) > 1 {
		c.buildDependencyGraph()
	}

	c.logInfo("Task queue: %d issue(s) [%s]", len(c.config.Tasks), strings.Join(c.config.Tasks, ", "))

	return nil
}

// runMainLoop processes tasks sequentially from the unified queue (PRs first, then issues).
// For each task, it prepares the context, runs agent iterations, and updates task state.
// Returns nil on normal completion, or ctx.Err() if the context is cancelled.
func (c *Controller) runMainLoop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			c.logInfo("Context cancelled, initiating graceful shutdown")
			c.emitFinalLogs()
			c.cleanup()
			return ctx.Err()
		default:
		}

		// Check termination conditions
		if c.shouldTerminate() {
			c.logInfo("Termination condition met")
			break
		}

		// Get next task from unified queue
		nextTask := c.nextQueuedTask()
		if nextTask == nil {
			c.logInfo("No active tasks remaining")
			break
		}

		c.activeTask = nextTask.ID
		c.activeTaskType = nextTask.Type

		// Refresh GitHub token if needed before starting work on this task
		// This ensures a fresh token (~1 hour validity) at the start of each task
		if err := c.refreshGitHubTokenIfNeeded(); err != nil {
			c.logError("Failed to refresh GitHub token: %v", err)
			// Mark task as blocked and continue to next task
			taskID := fmt.Sprintf("%s:%s", nextTask.Type, nextTask.ID)
			if state, ok := c.taskStates[taskID]; ok {
				state.Phase = PhaseBlocked
			}
			continue
		}

		// Build prompt for issue task
		c.logInfo("Focusing on issue #%s", nextTask.ID)

		// Check if this is a tracker issue — expand instead of running phase loop
		issue := c.issueDetailsByNumber[nextTask.ID]
		if issue != nil && isTrackerIssue(issue) {
			taskID := taskKey("issue", nextTask.ID)
			expanded, err := c.expandTrackerIssue(ctx, nextTask.ID, issue)
			if err != nil {
				c.logError("Tracker #%s expansion failed: %v", nextTask.ID, err)
				if state, ok := c.taskStates[taskID]; ok {
					state.Phase = PhaseBlocked
				}
			} else {
				if !expanded {
					c.logInfo("Tracker #%s has no sub-issues, marking NOTHING_TO_DO", nextTask.ID)
				}
				if state, ok := c.taskStates[taskID]; ok {
					state.Phase = PhaseNothingToDo
				}
			}
			continue
		}

		// Initialize monorepo package scope for this issue
		if err := c.initPackageScope(nextTask.ID); err != nil {
			c.logError("Issue #%s blocked: %v", nextTask.ID, err)
			taskID := taskKey("issue", nextTask.ID)
			if state, ok := c.taskStates[taskID]; ok {
				state.Phase = PhaseBlocked
			}
			continue
		}

		// Resolve parent branch for dependency chains
		issueTaskID := taskKey("issue", nextTask.ID)
		state := c.taskStates[issueTaskID]
		parentBranch, err := c.resolveParentBranch(ctx, nextTask.ID)
		if err != nil {
			c.logWarning("Issue #%s blocked: %v", nextTask.ID, err)
			if state != nil {
				state.Phase = PhaseBlocked
			}
			c.propagateBlocked(nextTask.ID)
			continue
		}
		if state != nil {
			state.ParentBranch = parentBranch
		}

		existingWork := c.detectExistingWork(ctx, nextTask.ID)
		c.config.Prompt = c.buildPromptForTask(nextTask.ID, existingWork, "")
		c.activeTaskExistingWork = existingWork

		// Run phase loop for issue tasks
		if err := c.runPhaseLoop(ctx); err != nil {
			c.logError("Phase loop failed for issue #%s: %v", nextTask.ID, err)
		}
	}

	// Post final tracker status comments
	for trackerID, subIDs := range c.trackerSubIssues {
		c.postTrackerStatusComment(ctx, trackerID, subIDs, "completed")
	}

	return nil
}

func (c *Controller) initializeWorkspace(ctx context.Context) error {
	c.logInfo("Initializing workspace")

	if err := os.MkdirAll(c.workDir, 0755); err != nil {
		return err
	}

	// Only set ownership and configure git safe.directory when running as root.
	// When running as non-root (e.g., local development), the workspace will
	// already be owned by the current user.
	if os.Getuid() == 0 {
		// Set ownership to agentium user so agent containers can access
		if err := os.Chown(c.workDir, AgentiumUID, AgentiumGID); err != nil {
			c.logWarning("failed to set workspace ownership: %v", err)
		}

		// Configure git safe.directory as a fallback
		_ = c.configureGitSafeDirectory(ctx)
	}

	return nil
}

func (c *Controller) fetchGitHubToken(ctx context.Context) error {
	c.logInfo("Fetching GitHub token")

	// Try to get token from environment first (for local testing or interactive mode)
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		c.gitHubToken = token
		return nil
	}

	// In interactive mode with clone-inside-container, token is optional (auth happens in container)
	if c.config.Interactive && c.config.CloneInsideContainer {
		c.logInfo("No GITHUB_TOKEN found; authentication will happen inside container")
		return nil
	}

	// In interactive mode without clone-inside-container, token is required
	if c.config.Interactive {
		return fmt.Errorf("GITHUB_TOKEN environment variable is required for local interactive mode")
	}

	// Fetch from cloud secret manager
	secretPath := c.config.GitHub.PrivateKeySecret
	if secretPath == "" {
		return fmt.Errorf("GitHub private key secret path not configured")
	}

	// Get private key from secret manager
	privateKey, err := c.fetchSecret(ctx, secretPath)
	if err != nil {
		return fmt.Errorf("failed to fetch private key: %w", err)
	}

	// Initialize TokenManager for automatic refresh
	appID := strconv.FormatInt(c.config.GitHub.AppID, 10)
	tm, err := github.NewTokenManager(appID, c.config.GitHub.InstallationID, []byte(privateKey))
	if err != nil {
		return fmt.Errorf("failed to create token manager: %w", err)
	}
	c.tokenManager = tm

	// Get initial token
	token, err := tm.Token()
	if err != nil {
		return fmt.Errorf("failed to get initial token: %w", err)
	}

	c.gitHubToken = token
	c.logInfo("GitHub token obtained (expires at %s)", tm.ExpiresAt().Format(time.RFC3339))
	return nil
}

// refreshGitHubTokenIfNeeded checks if the GitHub token needs to be refreshed and refreshes it if so.
// This should be called before starting work on each task to ensure a fresh token (~1 hour validity).
// For static tokens (from GITHUB_TOKEN env var), this is a no-op.
func (c *Controller) refreshGitHubTokenIfNeeded() error {
	// Skip if no token manager (static token from env var)
	if c.tokenManager == nil {
		return nil
	}

	// Check if refresh is needed
	if !c.tokenManager.NeedsRefresh() {
		return nil
	}

	c.logInfo("GitHub token expiring soon, refreshing...")
	token, err := c.tokenManager.Refresh()
	if err != nil {
		return fmt.Errorf("failed to refresh GitHub token: %w", err)
	}

	c.gitHubToken = token
	c.logInfo("GitHub token refreshed (expires at %s)", c.tokenManager.ExpiresAt().Format(time.RFC3339))
	return nil
}

func (c *Controller) fetchSecret(ctx context.Context, secretPath string) (string, error) {
	// Try to use Secret Manager client first
	if c.secretManager != nil {
		secret, err := c.secretManager.FetchSecret(ctx, secretPath)
		if err == nil {
			return secret, nil
		}
		c.logWarning("Secret Manager client failed: %v, falling back to gcloud CLI", err)
	}

	// Fallback to gcloud CLI
	cmd := c.execCommand(ctx, "gcloud", "secrets", "versions", "access", "latest",
		"--secret", filepath.Base(secretPath),
	)

	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return string(output), nil
}

func (c *Controller) loadPrompts() error {
	// Load skills manifest (required)
	manifest, err := skills.LoadManifest()
	if err != nil {
		return fmt.Errorf("failed to load skills manifest: %w", err)
	}

	loaded, err := skills.LoadSkills(manifest)
	if err != nil {
		return fmt.Errorf("failed to load skills: %w", err)
	}

	c.skillSelector = skills.NewSelector(loaded)
	names := make([]string, len(loaded))
	for i, s := range loaded {
		names[i] = s.Entry.Name
	}
	c.logInfo("Skills loaded: %v", names)

	// Load project prompt from workspace (.agentium/AGENTS.md) - optional
	projectPrompt, err := prompt.LoadProjectPrompt(c.workDir)
	if err != nil {
		c.logWarning("failed to load project prompt: %v", err)
	} else if projectPrompt != "" {
		c.projectPrompt = projectPrompt
		c.logInfo("Project prompt loaded from .agentium/AGENTS.md")
	}

	// Initialize persistent memory store if enabled
	if c.config.Memory.Enabled {
		c.memoryStore = memory.NewStore(c.workDir, memory.Config{
			Enabled:       true,
			MaxEntries:    c.config.Memory.MaxEntries,
			ContextBudget: c.config.Memory.ContextBudget,
		})
		if loadErr := c.memoryStore.Load(); loadErr != nil {
			c.logWarning("failed to load memory store: %v", loadErr)
		} else {
			c.logInfo("Memory store initialized (%d entries)", len(c.memoryStore.Entries()))
		}
	}

	// Initialize structured handoff store (always enabled for reviewer context)
	store, err := handoff.NewStore(c.workDir)
	if err != nil {
		c.logWarning("failed to initialize handoff store: %v", err)
	} else {
		c.handoffStore = store
		c.handoffBuilder = handoff.NewBuilder(store)
		c.handoffParser = handoff.NewParser()
		c.handoffValidator = handoff.NewValidator()
		c.logInfo("Handoff store initialized")
	}

	// Initialize local event sink if AGENTIUM_EVENT_FILE is set
	if eventFile := os.Getenv("AGENTIUM_EVENT_FILE"); eventFile != "" {
		sink, err := event.NewFileSink(eventFile)
		if err != nil {
			c.logWarning("failed to initialize event sink: %v", err)
		} else {
			c.eventSink = sink
			c.logInfo("Event sink initialized: %s", eventFile)
		}
	}

	return nil
}

func (c *Controller) cloneRepository(ctx context.Context) error {
	c.logInfo("Cloning repository: %s", c.config.Repository)

	// Parse repository URL
	repo := c.config.Repository
	if !strings.HasPrefix(repo, "https://") && !strings.HasPrefix(repo, "git@") {
		// Handle various shorthand formats:
		// - "owner/repo" -> "https://github.com/owner/repo"
		// - "github.com/owner/repo" -> "https://github.com/owner/repo"
		if strings.HasPrefix(repo, "github.com/") {
			repo = "https://" + repo
		} else {
			repo = "https://github.com/" + repo
		}
	}

	// Clone with token authentication
	// SECURITY: Avoid embedding tokens in URLs as they can leak in error messages and logs.
	// Use a credential helper that reads from environment variable for safety.
	var cmd *exec.Cmd
	if c.gitHubToken != "" && strings.HasPrefix(repo, "https://") {
		// Use credential helper to pass token securely via environment variable
		// GitHub App installation tokens require x-access-token username format
		credentialHelper := "!f() { echo username=x-access-token; echo \"password=$GIT_TOKEN\"; }; f"
		cmd = c.execCommand(ctx, "git",
			"-c", fmt.Sprintf("credential.helper=%s", credentialHelper),
			"clone", repo, c.workDir)
		cmd.Env = append(os.Environ(), "GIT_TOKEN="+c.gitHubToken)
	} else {
		cmd = c.execCommand(ctx, "git", "clone", repo, c.workDir)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		// Check if directory already exists with content
		if entries, _ := os.ReadDir(c.workDir); len(entries) > 0 {
			c.logInfo("Workspace already contains files, skipping clone")
			// Fix ownership for existing workspaces (only when running as root)
			if os.Getuid() == 0 {
				if ownerErr := c.ensureWorkspaceOwnership(); ownerErr != nil {
					c.logWarning("failed to set workspace ownership: %v", ownerErr)
				}
			}
			return nil
		}
		// Sanitize error to ensure no tokens leak in error messages
		return sanitizeGitError(err, c.gitHubToken)
	}

	// Fix ownership after clone so agent containers can access (only when running as root)
	if os.Getuid() == 0 {
		if err := c.ensureWorkspaceOwnership(); err != nil {
			c.logWarning("failed to set workspace ownership after clone: %v", err)
		}
	}

	return nil
}

// sanitizeGitError removes sensitive tokens from error messages to prevent credential leaks.
// This is a defense-in-depth measure for cases where tokens might appear in git error output.
func sanitizeGitError(err error, token string) error {
	if err == nil || token == "" {
		return err
	}
	msg := err.Error()
	if strings.Contains(msg, token) {
		msg = strings.ReplaceAll(msg, token, "[REDACTED]")
		return fmt.Errorf("%s", msg)
	}
	return err
}

// ensureWorkspaceOwnership recursively changes ownership of the workspace
// to agentium (uid=1000, gid=1000) so agent containers can access it.
func (c *Controller) ensureWorkspaceOwnership() error {
	c.logInfo("Setting workspace ownership to agentium (uid=%d, gid=%d)", AgentiumUID, AgentiumGID)

	return filepath.WalkDir(c.workDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := os.Chown(path, AgentiumUID, AgentiumGID); err != nil {
			return fmt.Errorf("failed to chown %s: %w", path, err)
		}
		return nil
	})
}

// configureGitSafeDirectory adds the workspace to git's safe.directory config
// as a fallback for ownership issues.
func (c *Controller) configureGitSafeDirectory(ctx context.Context) error {
	cmd := c.execCommand(ctx, "git", "config", "--global", "--add", "safe.directory", c.workDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		c.logWarning("failed to configure git safe.directory: %v (%s)", err, string(output))
		return err
	}
	c.logInfo("Configured git safe.directory for %s", c.workDir)
	return nil
}

// issueLabel represents a GitHub issue label.
type issueLabel struct {
	Name string `json:"name"`
}

type issueDetail struct {
	Number    int          `json:"number"`
	Title     string       `json:"title"`
	Body      string       `json:"body"`
	Labels    []issueLabel `json:"labels"`
	DependsOn []string     // Parsed dependency issue IDs (populated by buildDependencyGraph)
}

// extractPackageLabelFromIssue extracts the package path from issue labels using the configured label prefix.
// Returns the package name and true if found, empty string and false otherwise.
func (c *Controller) extractPackageLabelFromIssue(issue *issueDetail) (string, bool) {
	if c.config.Monorepo == nil || !c.config.Monorepo.Enabled {
		return "", false
	}

	prefix := c.config.Monorepo.LabelPrefix
	if prefix == "" {
		prefix = "pkg" // Default
	}

	for _, label := range issue.Labels {
		if pkg, ok := workspace.ExtractPackageFromLabel(label.Name, prefix); ok {
			return pkg, true
		}
	}
	return "", false
}

// resolveAndValidatePackage resolves and validates the package for the current issue.
// It returns the resolved package path, or an error if:
// - Monorepo is enabled but no package label is found
// - The package label doesn't match a valid workspace package
//
// When tiers are configured, multiple pkg:* labels are allowed. Infrastructure/integration
// packages are implicitly permitted alongside a single domain/app package.
func (c *Controller) resolveAndValidatePackage(issueNumber string) (string, error) {
	if c.config.Monorepo == nil || !c.config.Monorepo.Enabled {
		return "", nil
	}

	// Check if workspace has pnpm-workspace.yaml
	if !workspace.HasPnpmWorkspace(c.workDir) {
		return "", fmt.Errorf("monorepo enabled but pnpm-workspace.yaml not found in %s", c.workDir)
	}

	// Get issue details
	issue := c.issueDetailsByNumber[issueNumber]
	if issue == nil {
		return "", fmt.Errorf("issue #%s not found in fetched details", issueNumber)
	}

	// Check for tracker issues — skip package scope
	if isTrackerIssue(issue) {
		return "", nil
	}

	prefix := c.config.Monorepo.LabelPrefix
	if prefix == "" {
		prefix = "pkg"
	}

	// When tiers are configured, use tier-aware validation
	if len(c.config.Monorepo.Tiers) > 0 {
		// Collect label name strings
		var labelNames []string
		for _, l := range issue.Labels {
			labelNames = append(labelNames, l.Name)
		}

		classifications, err := workspace.ClassifyPackageLabels(labelNames, prefix, c.config.Monorepo.Tiers, c.workDir)
		if err != nil {
			return "", fmt.Errorf("issue #%s: %w", issueNumber, err)
		}

		if len(classifications) == 0 {
			return "", fmt.Errorf("monorepo requires %s:<package> label on issue #%s", prefix, issueNumber)
		}

		pkgPath, err := workspace.ValidatePackageLabels(classifications)
		if err != nil {
			return "", fmt.Errorf("issue #%s: %w", issueNumber, err)
		}
		return pkgPath, nil
	}

	// No tiers configured — preserve existing single-label behavior
	pkgName, found := c.extractPackageLabelFromIssue(issue)
	if !found {
		return "", fmt.Errorf("monorepo requires %s:<package> label on issue #%s", prefix, issueNumber)
	}

	// Resolve package name to full path (e.g., "core" -> "packages/core")
	pkgPath, err := workspace.ResolvePackagePath(c.workDir, pkgName)
	if err != nil {
		return "", fmt.Errorf("invalid package label on issue #%s: %w", issueNumber, err)
	}

	return pkgPath, nil
}

// initPackageScope sets up the package scope for a monorepo issue.
// It detects the package from issue labels, validates it, and initializes the scope validator.
// It also reloads the project prompt to include package-specific AGENTS.md.
func (c *Controller) initPackageScope(issueNumber string) error {
	pkgPath, err := c.resolveAndValidatePackage(issueNumber)
	if err != nil {
		return err
	}

	if pkgPath == "" {
		// Not a monorepo or package not required
		c.packagePath = ""
		c.scopeValidator = nil
		return nil
	}

	c.packagePath = pkgPath
	c.scopeValidator = scope.NewValidator(c.workDir, pkgPath)
	c.logInfo("Monorepo package scope: %s", pkgPath)

	// Reload project prompt with package-specific AGENTS.md merged in
	// Always update projectPrompt (even if empty) to avoid stale prompts from previous packages
	projectPrompt, err := prompt.LoadProjectPromptWithPackage(c.workDir, pkgPath)
	if err != nil {
		c.logWarning("failed to load hierarchical project prompt: %v", err)
	} else {
		c.projectPrompt = projectPrompt
		if projectPrompt != "" {
			c.logInfo("Project prompt loaded with package context (%s)", pkgPath)
		} else {
			c.logInfo("No project prompt found for package %s", pkgPath)
		}
	}

	return nil
}

// buildPackageScopeInstructions returns package scope constraint instructions for the agent prompt.
// Returns empty string if no package scope is active.
func (c *Controller) buildPackageScopeInstructions() string {
	if c.packagePath == "" {
		return ""
	}

	return fmt.Sprintf(`## PACKAGE SCOPE CONSTRAINT

You are working within monorepo package: %s

STRICT CONSTRAINTS:
- Only modify files within: %s/
- Exception: You may update root package.json for workspace dependencies
- Run commands from package directory: cd %s && pnpm test
- Do NOT modify files in other packages or repository root (except root package.json)

Violations will cause your changes to be rejected and reverted.
`, c.packagePath, c.packagePath, c.packagePath)
}

// branchPrefixForLabels returns the branch prefix based on the first issue label.
// Returns "feature" as default when no labels are present or if sanitization yields empty string.
func branchPrefixForLabels(labels []issueLabel) string {
	if len(labels) > 0 {
		prefix := sanitizeBranchPrefix(labels[0].Name)
		if prefix != "" {
			return prefix
		}
	}
	return "feature" // Default when no labels or invalid label
}

// sanitizeBranchPrefix converts a label name to a valid git branch prefix.
// It handles characters that are invalid in git refs: ~ ^ : ? * [ \ space and more.
func sanitizeBranchPrefix(label string) string {
	// Lowercase first
	result := strings.ToLower(label)

	// Replace any character that's not alphanumeric or hyphen with hyphen
	var sanitized strings.Builder
	for _, r := range result {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			sanitized.WriteRune(r)
		} else {
			sanitized.WriteRune('-')
		}
	}
	result = sanitized.String()

	// Collapse consecutive hyphens
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}

	// Trim leading and trailing hyphens
	result = strings.Trim(result, "-")

	return result
}

func (c *Controller) fetchIssueDetails(ctx context.Context) []issueDetail {
	c.logInfo("Fetching issue details")

	issues := make([]issueDetail, 0, len(c.config.Tasks))
	c.issueDetailsByNumber = make(map[string]*issueDetail, len(c.config.Tasks))

	for _, taskID := range c.config.Tasks {
		// Use gh CLI to fetch issue
		cmd := c.execCommand(ctx, "gh", "issue", "view", taskID,
			"--repo", c.config.Repository,
			"--json", "number,title,body,labels",
		)
		cmd.Env = c.envWithGitHubToken()

		output, err := cmd.Output()
		if err != nil {
			c.logWarning("failed to fetch issue #%s: %v", taskID, err)
			continue
		}

		var issue issueDetail
		if err := json.Unmarshal(output, &issue); err != nil {
			c.logWarning("failed to parse issue #%s: %v", taskID, err)
			continue
		}

		issues = append(issues, issue)
	}

	// Build O(1) lookup map after collecting all issues
	for i := range issues {
		issueNumStr := fmt.Sprintf("%d", issues[i].Number)
		c.issueDetailsByNumber[issueNumStr] = &issues[i]
	}

	return issues
}

// nextQueuedTask returns the first task in the queue that hasn't reached a terminal phase.
func (c *Controller) nextQueuedTask() *TaskQueueItem {
	for i := range c.taskQueue {
		item := &c.taskQueue[i]
		taskID := taskKey(item.Type, item.ID)
		state := c.taskStates[taskID]
		if state == nil {
			return item
		}
		switch state.Phase {
		case PhaseComplete, PhaseNothingToDo, PhaseBlocked:
			continue
		default:
			return item
		}
	}
	return nil
}

// detectExistingWork checks GitHub for existing branches and PRs related to an issue.
// It searches for branches matching the pattern */issue-<N>-* (any prefix).
func (c *Controller) detectExistingWork(ctx context.Context, issueNumber string) *agent.ExistingWork {
	// Check for existing open PRs with branch matching */issue-<N>-*
	// Use --limit to ensure we scan enough PRs in repos with many open PRs
	// Search pattern matches any prefix (feature, bug, enhancement, agentium, etc.)
	branchPattern := fmt.Sprintf("/issue-%s-", issueNumber)
	cmd := c.execCommand(ctx, "gh", "pr", "list",
		"--repo", c.config.Repository,
		"--state", "open",
		"--limit", "200",
		"--json", "number,title,headRefName",
	)
	cmd.Dir = c.workDir
	cmd.Env = c.envWithGitHubToken()

	if output, err := cmd.Output(); err == nil {
		var prs []struct {
			Number      int    `json:"number"`
			Title       string `json:"title"`
			HeadRefName string `json:"headRefName"`
		}
		if unmarshalErr := json.Unmarshal(output, &prs); unmarshalErr == nil {
			// Filter for branches matching */issue-<N>-*
			for _, pr := range prs {
				if strings.Contains(pr.HeadRefName, branchPattern) {
					c.logInfo("Found existing PR #%d for issue #%s on branch %s",
						pr.Number, issueNumber, pr.HeadRefName)
					return &agent.ExistingWork{
						PRNumber: fmt.Sprintf("%d", pr.Number),
						PRTitle:  pr.Title,
						Branch:   pr.HeadRefName,
					}
				}
			}
		}
	} else {
		c.logWarning("failed to list PRs for existing work detection on issue #%s: %v", issueNumber, err)
	}

	// No PR found; check for existing remote branches matching */issue-<N>-*
	// First, list all remote branches
	cmd = c.execCommand(ctx, "git", "branch", "-r")
	cmd.Dir = c.workDir

	if output, err := cmd.Output(); err == nil {
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, line := range lines {
			branch := strings.TrimSpace(line)
			branch = strings.TrimPrefix(branch, "origin/")
			// Match pattern: */issue-<N>-*
			if strings.Contains(branch, branchPattern) {
				c.logInfo("Found existing branch for issue #%s: %s", issueNumber, branch)
				return &agent.ExistingWork{
					Branch: branch,
				}
			}
		}
	} else {
		c.logWarning("failed to list remote branches for existing work detection on issue #%s: %v", issueNumber, err)
	}

	c.logInfo("No existing work found for issue #%s", issueNumber)
	return nil
}

// renderWithParameters applies template variable substitution to a prompt string.
// It merges built-in variables (repository, issue_url, etc.) with user-provided parameters,
// where user parameters take precedence on name collision.
func (c *Controller) renderWithParameters(prompt string) string {
	// Build built-in variables from session config
	builtins := map[string]string{
		"repository": c.config.Repository,
	}

	// Add issue_url: prefer explicit PromptContext value, fall back to derived URL
	if c.config.PromptContext != nil && c.config.PromptContext.IssueURL != "" {
		builtins["issue_url"] = c.config.PromptContext.IssueURL
	} else if c.activeTask != "" && c.activeTaskType == "issue" && c.config.Repository != "" {
		builtins["issue_url"] = fmt.Sprintf("https://github.com/%s/issues/%s", c.config.Repository, c.activeTask)
	}

	// Add issue_number only for issue tasks (not PR tasks)
	if c.activeTask != "" && c.activeTaskType == "issue" {
		builtins["issue_number"] = c.activeTask
	}

	// Merge with user parameters (user params override builtins)
	var userParams map[string]string
	if c.config.PromptContext != nil {
		userParams = c.config.PromptContext.Parameters
	}

	merged := template.MergeVariables(builtins, userParams)
	return template.RenderPrompt(prompt, merged)
}

// buildPromptForTask builds a focused prompt for a single issue, incorporating existing work context.
// The phase parameter controls whether implementation instructions are included:
// - For IMPLEMENT phase (or empty phase): include full implementation instructions
// - For other phases (PLAN, DOCS, etc.): defer to the phase-specific system prompt
func (c *Controller) buildPromptForTask(issueNumber string, existingWork *agent.ExistingWork, phase TaskPhase) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("You are working on repository: %s\n\n", c.config.Repository))

	// O(1) lookup for issue detail
	issue := c.issueDetailsByNumber[issueNumber]

	sb.WriteString(fmt.Sprintf("## Your Task: Issue #%s\n\n", issueNumber))
	if issue != nil {
		sb.WriteString(fmt.Sprintf("**Title:** %s\n\n", issue.Title))
		if issue.Body != "" {
			body := issue.Body
			if len(body) > MaxIssueBodyLen {
				body = body[:MaxIssueBodyLen] + "..."
			}
			sb.WriteString(fmt.Sprintf("**Description:**\n%s\n\n", body))
		}
	}

	// Always include existing work context (branch/PR info) regardless of phase
	if existingWork != nil {
		sb.WriteString("## Existing Work Detected\n\n")
		if existingWork.PRNumber != "" {
			sb.WriteString(fmt.Sprintf("An open PR already exists for this issue: **PR #%s** (%s)\n",
				existingWork.PRNumber, existingWork.PRTitle))
			sb.WriteString(fmt.Sprintf("Branch: `%s`\n\n", existingWork.Branch))
		} else {
			sb.WriteString(fmt.Sprintf("An existing branch was found for this issue: `%s`\n\n", existingWork.Branch))
		}
	}

	// Only include detailed implementation instructions for IMPLEMENT phase (or when phase is empty/unspecified)
	// For PLAN, DOCS, and other phases, defer to the phase-specific system prompt
	switch phase {
	case PhaseImplement, "":
		if existingWork != nil {
			if existingWork.PRNumber != "" {
				sb.WriteString("### Instructions\n\n")
				sb.WriteString(fmt.Sprintf("1. Check out the existing branch: `git checkout %s`\n", existingWork.Branch))
				sb.WriteString("2. Review the current state of the code on this branch\n")
				sb.WriteString("3. Continue implementation or fix any issues found\n")
				sb.WriteString("4. Run tests to verify correctness\n")
				sb.WriteString(fmt.Sprintf("5. Push updates to the existing branch: `git push origin %s`\n", existingWork.Branch))
				sb.WriteString("6. The existing PR will update automatically\n\n")
				sb.WriteString("### DO NOT\n\n")
				sb.WriteString("- Do NOT create a new branch\n")
				sb.WriteString("- Do NOT create a new PR\n")
				sb.WriteString("- Do NOT close or delete the existing PR\n")
			} else {
				sb.WriteString("### Instructions\n\n")
				sb.WriteString(fmt.Sprintf("1. Check out the existing branch: `git checkout %s`\n", existingWork.Branch))
				sb.WriteString("2. Review what's already been done on this branch\n")
				sb.WriteString("3. Continue implementation or fix issues\n")
				sb.WriteString("4. Run tests to verify correctness\n")
				sb.WriteString("5. Commit and push your changes\n")
				sb.WriteString("6. Create a PR linking to the issue (if one doesn't exist yet)\n\n")
				sb.WriteString("### DO NOT\n\n")
				sb.WriteString("- Do NOT create a new branch (use the existing one)\n")
			}
		} else {
			// No existing work — fresh start
			// Check if this issue depends on a parent issue's branch
			taskID := taskKey("issue", issueNumber)
			parentBranch := ""
			if state, ok := c.taskStates[taskID]; ok && state.ParentBranch != "" {
				parentBranch = state.ParentBranch
			}

			// Determine branch prefix from issue labels
			branchPrefix := "feature" // Default
			if issue != nil {
				branchPrefix = branchPrefixForLabels(issue.Labels)
			}

			sb.WriteString("### Instructions\n\n")
			if parentBranch != "" {
				sb.WriteString(fmt.Sprintf("**NOTE:** This issue depends on work from another issue. You must branch from: `%s`\n\n", parentBranch))
				sb.WriteString(fmt.Sprintf("1. First, check out the parent branch: `git checkout %s`\n", parentBranch))
				sb.WriteString(fmt.Sprintf("2. Create your new branch from it: `git checkout -b %s/issue-%s-<short-description>`\n", branchPrefix, issueNumber))
				sb.WriteString("3. Implement the fix or feature\n")
				sb.WriteString("4. Run tests to verify correctness\n")
				sb.WriteString("5. Commit your changes with a descriptive message\n")
				sb.WriteString("6. Push the branch\n")
				sb.WriteString("7. Create a pull request targeting `main` (NOT the parent branch)\n\n")
				sb.WriteString("### IMPORTANT\n\n")
				sb.WriteString("- Your PR must target `main`, not the parent branch\n")
				sb.WriteString("- The PR diff will include parent changes until the parent PR is merged\n")
				sb.WriteString("- After the parent PR merges, GitHub will auto-resolve the diff\n")
			} else {
				sb.WriteString(fmt.Sprintf("1. Create a new branch: `%s/issue-%s-<short-description>`\n", branchPrefix, issueNumber))
				sb.WriteString("2. Implement the fix or feature\n")
				sb.WriteString("3. Run tests to verify correctness\n")
				sb.WriteString("4. Commit your changes with a descriptive message\n")
				sb.WriteString("5. Push the branch\n")
				sb.WriteString("6. Create a pull request linking to the issue\n\n")
			}
		}
		sb.WriteString("Use 'gh' CLI for GitHub operations and 'git' for version control.\n")
		sb.WriteString(fmt.Sprintf("The repository is already cloned at %s.\n", c.workDir))
	case PhaseVerify:
		// VERIFY phase: provide PR number and repo context for CI checking and merging
		taskID := taskKey("issue", issueNumber)
		state := c.taskStates[taskID]
		sb.WriteString("### Instructions\n\n")
		sb.WriteString("Follow the instructions in your system prompt to verify CI checks and merge the PR.\n\n")
		if state != nil && state.PRNumber != "" {
			sb.WriteString(fmt.Sprintf("**PR Number:** %s\n", state.PRNumber))
		}
		sb.WriteString(fmt.Sprintf("**Repository:** %s\n", c.config.Repository))
		sb.WriteString(fmt.Sprintf("The repository is cloned at %s.\n", c.workDir))
	default:
		// For PLAN, DOCS, and other phases: defer to the phase-specific system prompt
		sb.WriteString("### Instructions\n\n")
		sb.WriteString("Follow the instructions in your system prompt to complete this phase.\n")
		sb.WriteString(fmt.Sprintf("The repository is cloned at %s.\n", c.workDir))
	}

	// Apply template variable substitution
	return c.renderWithParameters(sb.String())
}

// updateTaskPhase transitions task phase based on agent status signals
func (c *Controller) updateTaskPhase(taskID string, result *agent.IterationResult) {
	state, exists := c.taskStates[taskID]
	if !exists {
		return
	}

	// Update based on agent status signal
	switch result.AgentStatus {
	case "TESTS_RUNNING":
		// Tests are now part of IMPLEMENT phase, keep phase unchanged
	case "TESTS_PASSED":
		state.Phase = PhaseDocs
		state.TestRetries = 0 // Reset retries on success
	case "TESTS_FAILED":
		state.TestRetries++
		if state.TestRetries >= 3 {
			state.Phase = PhaseBlocked
			c.logWarning("Task %s blocked after %d test failures", taskID, state.TestRetries)
			// Propagate blocked state to dependent issues
			if state.Type == "issue" {
				c.propagateBlocked(state.ID)
			}
		}
	case "PR_CREATED":
		state.Phase = PhaseComplete
		state.PRNumber = result.StatusMessage
	case "PUSHED":
		state.Phase = PhaseComplete
	case "COMPLETE":
		state.Phase = PhaseComplete
	case "NOTHING_TO_DO":
		state.Phase = PhaseNothingToDo
	case "BLOCKED", "FAILED":
		state.Phase = PhaseBlocked
		// Propagate blocked state to dependent issues
		if state.Type == "issue" {
			c.propagateBlocked(state.ID)
		}
	}

	// Fallback: if no explicit status signal but PRs were detected for an issue task.
	// Only complete if already in DOCS phase to avoid skipping documentation updates.
	// If still in IMPLEMENT, advance to DOCS instead of completing.
	if result.AgentStatus == "" && state.Type == "issue" && len(result.PRsCreated) > 0 {
		state.PRNumber = result.PRsCreated[0]
		switch state.Phase {
		case PhaseDocs:
			state.Phase = PhaseComplete
			c.logInfo("Task %s completed via PR detection fallback (PR #%s)", taskID, state.PRNumber)
		case PhaseImplement:
			state.Phase = PhaseDocs
			c.logInfo("Task %s advancing to DOCS via PR detection (PR #%s)", taskID, state.PRNumber)
		}
	}

	state.LastStatus = result.AgentStatus
	c.logInfo("Task %s phase: %s (status: %s)", taskID, state.Phase, result.AgentStatus)
}

// updateInstanceMetadata writes the current session status to GCP instance metadata.
// This is best-effort: errors are logged but never cause the controller to crash.
func (c *Controller) updateInstanceMetadata(ctx context.Context) {
	if c.metadataUpdater == nil {
		return
	}

	var completed, pending []string
	for taskID, state := range c.taskStates {
		switch state.Phase {
		case PhaseComplete, PhaseNothingToDo:
			completed = append(completed, taskID)
		default:
			pending = append(pending, taskID)
		}
	}

	status := gcp.SessionStatusMetadata{
		Iteration:      c.iteration,
		MaxIterations:  c.config.MaxIterations,
		CompletedTasks: completed,
		PendingTasks:   pending,
	}

	if err := c.metadataUpdater.UpdateStatus(ctx, status); err != nil {
		c.logWarning("failed to update instance metadata: %v", err)
	}
}

func (c *Controller) shouldTerminate() bool {
	// Check iteration limit
	if c.iteration >= c.config.MaxIterations {
		c.logInfo("Max iterations reached")
		return true
	}

	// Check time limit
	if time.Since(c.startTime) >= c.maxDuration {
		c.logInfo("Max duration reached")
		return true
	}

	// Check if all tasks have reached a terminal phase
	if len(c.taskStates) > 0 {
		allTerminal := true
		for taskID, state := range c.taskStates {
			switch state.Phase {
			case PhaseComplete, PhaseNothingToDo, PhaseBlocked:
				c.logInfo("Task %s in terminal phase: %s", taskID, state.Phase)
				continue
			default:
				allTerminal = false
			}
		}
		if allTerminal {
			c.logInfo("All tasks in terminal phase")
			return true
		}
	}

	return false
}

// buildDependencyGraph constructs an inter-issue dependency graph from issue bodies.
// It parses dependencies, detects and breaks cycles (with warnings), and reorders the task queue.
// This method is only called when there are multiple issues in the batch.
func (c *Controller) buildDependencyGraph() {
	if len(c.issueDetails) <= 1 {
		return
	}

	// Build set of batch issue IDs
	batchIDs := make(map[string]bool)
	for _, issue := range c.issueDetails {
		batchIDs[strconv.Itoa(issue.Number)] = true
	}

	// Parse dependencies and populate DependsOn field
	for i := range c.issueDetails {
		deps := parseDependencies(c.issueDetails[i].Body)
		c.issueDetails[i].DependsOn = deps
	}

	// Build the dependency graph
	c.depGraph = NewDependencyGraph(c.issueDetails, batchIDs)

	// Log any cycles that were broken
	if brokenEdges := c.depGraph.BrokenEdges(); len(brokenEdges) > 0 {
		for _, edge := range brokenEdges {
			c.logWarning("Cycle detected: edge from #%s to #%s was removed", edge.ParentID, edge.ChildID)
		}
	}

	// Reorder task queue if graph has dependencies
	if c.depGraph.HasDependencies() {
		c.reorderTaskQueue(c.depGraph.SortedIssueIDs())
		c.logInfo("Task queue reordered based on dependencies: %v", c.depGraph.SortedIssueIDs())
	}
}

// reorderTaskQueue reorders the task queue to match the topologically sorted issue order.
func (c *Controller) reorderTaskQueue(sortedIDs []string) {
	issueMap := make(map[string]TaskQueueItem)

	for _, item := range c.taskQueue {
		issueMap[item.ID] = item
	}

	// Rebuild queue in topological order
	newQueue := make([]TaskQueueItem, 0, len(c.taskQueue))

	for _, id := range sortedIDs {
		if item, ok := issueMap[id]; ok {
			newQueue = append(newQueue, item)
			delete(issueMap, id)
		}
	}

	// Append any remaining issues not in the sorted list (shouldn't happen, but be safe)
	for _, item := range issueMap {
		newQueue = append(newQueue, item)
	}

	c.taskQueue = newQueue
}

// resolveParentBranch determines the branch a child issue should be based on.
// Returns the parent's branch name if the child depends on a parent issue, or "" to use main.
// Returns an error if the child should be marked BLOCKED (e.g., parent failed or has no branch).
func (c *Controller) resolveParentBranch(ctx context.Context, childID string) (string, error) {
	if c.depGraph == nil {
		return "", nil
	}

	parents := c.depGraph.ParentsOf(childID)
	if len(parents) == 0 {
		return "", nil
	}

	// For chained dependencies, only the immediate parent matters
	// (multi-parent chaining already linearized the graph)
	parentID := parents[0]

	// Check if parent is in our batch
	parentTaskID := taskKey("issue", parentID)
	parentState, inBatch := c.taskStates[parentTaskID]

	if inBatch {
		// In-batch parent: check its completion state
		switch parentState.Phase {
		case PhaseComplete:
			// Parent completed successfully, find its branch
			existingWork := c.detectExistingWork(ctx, parentID)
			if existingWork != nil && existingWork.Branch != "" {
				c.logInfo("Issue #%s will branch from parent #%s's branch: %s", childID, parentID, existingWork.Branch)
				return existingWork.Branch, nil
			}
			// Parent complete but no branch found (maybe it was merged?)
			c.logInfo("Issue #%s: parent #%s complete but no branch found, using main", childID, parentID)
			return "", nil

		case PhaseNothingToDo:
			// Parent had nothing to do, no dependency effect
			c.logInfo("Issue #%s: parent #%s had nothing to do, using main", childID, parentID)
			return "", nil

		case PhaseBlocked:
			// Parent is blocked, child should also be blocked
			return "", fmt.Errorf("parent issue #%s is blocked", parentID)

		default:
			// Parent not yet complete, child should wait (block for now, controller will re-check)
			return "", fmt.Errorf("parent issue #%s not yet complete (phase: %s)", parentID, parentState.Phase)
		}
	}

	// External parent (not in batch): check for existing PR
	return c.resolveExternalParentBranch(ctx, parentID, childID)
}

// resolveExternalParentBranch resolves the branch for an external (not in batch) parent issue.
func (c *Controller) resolveExternalParentBranch(ctx context.Context, parentID, childID string) (string, error) {
	// Look for an existing PR for the external parent
	existingWork := c.detectExistingWork(ctx, parentID)
	if existingWork == nil {
		// No work found for external parent - child is blocked
		return "", fmt.Errorf("external parent issue #%s has no branch or PR", parentID)
	}

	if existingWork.PRNumber != "" {
		// External parent has a PR - check if it's merged
		merged, err := c.isPRMerged(ctx, existingWork.PRNumber)
		if err != nil {
			c.logWarning("Failed to check merge status of PR #%s: %v", existingWork.PRNumber, err)
			// Assume not merged, use the branch
			c.logInfo("Issue #%s will branch from external parent #%s's PR branch: %s", childID, parentID, existingWork.Branch)
			return existingWork.Branch, nil
		}

		if merged {
			// PR is merged, code is in main
			c.logInfo("Issue #%s: external parent #%s's PR merged, using main", childID, parentID)
			return "", nil
		}

		// PR is open, use its branch
		c.logInfo("Issue #%s will branch from external parent #%s's open PR branch: %s", childID, parentID, existingWork.Branch)
		return existingWork.Branch, nil
	}

	// External parent has a branch but no PR
	c.logInfo("Issue #%s will branch from external parent #%s's branch: %s", childID, parentID, existingWork.Branch)
	return existingWork.Branch, nil
}

// isPRMerged checks if a PR has been merged.
func (c *Controller) isPRMerged(ctx context.Context, prNumber string) (bool, error) {
	cmd := exec.CommandContext(ctx, "gh", "pr", "view", prNumber,
		"--repo", c.config.Repository,
		"--json", "state",
	)
	cmd.Dir = c.workDir
	cmd.Env = c.envWithGitHubToken()

	output, err := cmd.Output()
	if err != nil {
		return false, err
	}

	var result struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return false, err
	}

	return result.State == "MERGED", nil
}

// propagateBlocked marks all children of a blocked issue as BLOCKED via BFS.
func (c *Controller) propagateBlocked(issueID string) {
	if c.depGraph == nil {
		return
	}

	// BFS through children
	queue := []string{issueID}
	visited := make(map[string]bool)

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if visited[current] {
			continue
		}
		visited[current] = true

		children := c.depGraph.ChildrenOf(current)
		for _, childID := range children {
			taskID := taskKey("issue", childID)
			if state, ok := c.taskStates[taskID]; ok {
				if state.Phase != PhaseBlocked && state.Phase != PhaseComplete && state.Phase != PhaseNothingToDo {
					state.Phase = PhaseBlocked
					c.logInfo("Issue #%s marked BLOCKED (parent #%s blocked)", childID, current)
					queue = append(queue, childID)
				}
			}
		}
	}
}

// isPhaseLoopEnabled returns true if the phase loop is configured.
func (c *Controller) isPhaseLoopEnabled() bool {
	return c.config.PhaseLoop != nil
}

// isHandoffEnabled returns true if the handoff store is initialized.
func (c *Controller) isHandoffEnabled() bool {
	return c.handoffStore != nil
}

// buildIssueContext creates a handoff.IssueContext from the active issue details.
func (c *Controller) buildIssueContext() *handoff.IssueContext {
	if c.activeTask == "" || c.activeTaskType != "issue" {
		return nil
	}

	// O(1) lookup for issue in issueDetails
	issue := c.issueDetailsByNumber[c.activeTask]
	if issue == nil {
		return nil
	}
	return &handoff.IssueContext{
		Number:     issue.Number,
		Title:      issue.Title,
		Body:       issue.Body,
		Repository: c.config.Repository,
	}
}

// determineActivePhase returns the current phase for the active task.
// When no task state exists yet (first iteration), defaults to PhaseImplement.
func (c *Controller) determineActivePhase() TaskPhase {
	taskID := taskKey(c.activeTaskType, c.activeTask)
	if state, ok := c.taskStates[taskID]; ok {
		return state.Phase
	}
	return PhaseImplement
}

// buildIterateFeedbackSection constructs the feedback section for ITERATE prompts.
// It retrieves the previous iteration's reviewer feedback (EvalFeedback) and judge
// directives (JudgeDirective) from memory, formatting them into a structured section
// that guides the worker on what must be addressed.
//
// The returned section contains:
// - Guidance on how to interpret feedback types
// - Judge directives (REQUIRED action items)
// - Reviewer analysis (detailed context)
//
// Returns empty string if no feedback is available for the previous iteration.
func (c *Controller) buildIterateFeedbackSection(taskID string, phaseIteration int, parentBranch string) string {
	if c.memoryStore == nil {
		return ""
	}

	entries := c.memoryStore.GetPreviousIterationFeedback(taskID, phaseIteration)
	if len(entries) == 0 {
		return ""
	}

	var sb strings.Builder

	sb.WriteString("## Feedback from Previous Iteration\n\n")

	// Guidance on how to interpret feedback
	sb.WriteString("**How to use this feedback:**\n")
	sb.WriteString("- **Reviewer feedback**: Detailed analysis - consider all points as context\n")
	sb.WriteString("- **Judge directives**: Required action items - you MUST address these\n\n")
	sb.WriteString("**Approach:**\n")
	diffBase := "main"
	if parentBranch != "" {
		diffBase = parentBranch
	}
	sb.WriteString(fmt.Sprintf("- Your implementation already exists on this branch. Run `git log --oneline %s..HEAD` and `git diff %s..HEAD` to review your existing work before making changes.\n", diffBase, diffBase))
	sb.WriteString("- Make **targeted, surgical fixes** to address the feedback. Do not rewrite or start over unless the judge directive explicitly says to take a different approach.\n")
	sb.WriteString("- If a directive asks for a specific fix, make that fix and nothing else. If it asks to reconsider the approach, then a broader change is warranted.\n\n")

	// Separate reviewer feedback and judge directives
	var reviewerFeedback, judgeDirectives []string
	for _, e := range entries {
		switch e.Type {
		case memory.EvalFeedback:
			reviewerFeedback = append(reviewerFeedback, e.Content)
		case memory.JudgeDirective:
			judgeDirectives = append(judgeDirectives, e.Content)
		}
	}

	// Judge directives first (required actions)
	if len(judgeDirectives) > 0 {
		sb.WriteString("### Judge Directives (REQUIRED)\n\n")
		for _, d := range judgeDirectives {
			sb.WriteString(d)
			sb.WriteString("\n\n")
		}
	}

	// Reviewer feedback second (context)
	if len(reviewerFeedback) > 0 {
		sb.WriteString("### Reviewer Analysis (Context)\n\n")
		for _, f := range reviewerFeedback {
			sb.WriteString(f)
			sb.WriteString("\n\n")
		}
	}

	// Instructions for responding to feedback
	sb.WriteString("### Responding to Feedback\n\n")
	sb.WriteString("For each point in the reviewer analysis above, emit a `FEEDBACK_RESPONSE` memory signal indicating how you handled it.\n\n")
	sb.WriteString("Format: `AGENTIUM_MEMORY: FEEDBACK_RESPONSE [STATUS] <feedback summary> - <your response>`\n\n")
	sb.WriteString("STATUS values:\n")
	sb.WriteString("- `[ADDRESSED]` — You fixed or implemented the feedback point\n")
	sb.WriteString("- `[DECLINED]` — You chose not to act on it (explain why)\n")
	sb.WriteString("- `[PARTIAL]` — You partially addressed it (explain what remains)\n\n")
	sb.WriteString("Emit one signal per feedback point. This is expected for every reviewer feedback item.\n\n")

	return sb.String()
}

func (c *Controller) runIteration(ctx context.Context) (*agent.IterationResult, error) {
	// Build phase-aware prompt FIRST (before delegation check)
	// This ensures both delegated and non-delegated paths use the same phase-appropriate prompt
	prompt := c.config.Prompt
	if c.activeTaskType == "issue" && c.activeTask != "" {
		phase := c.determineActivePhase()
		prompt = c.buildPromptForTask(c.activeTask, c.activeTaskExistingWork, phase)
	}

	// Check delegation AFTER prompt is built
	if c.orchestrator != nil {
		phase := c.determineActivePhase()
		if subCfg := c.orchestrator.ConfigForPhase(phase); subCfg != nil {
			c.logInfo("Phase %s: delegating to sub-agent config (agent=%s)", phase, subCfg.Agent)
			return c.runDelegatedIteration(ctx, phase, subCfg, prompt)
		}
	}

	// Build project prompt with package scope instructions if applicable
	projectPrompt := c.projectPrompt
	if scopeInstructions := c.buildPackageScopeInstructions(); scopeInstructions != "" {
		if projectPrompt != "" {
			projectPrompt = projectPrompt + "\n\n" + scopeInstructions
		} else {
			projectPrompt = scopeInstructions
		}
	}

	// Initialize IterationContext once at session creation to avoid repeated nil checks
	session := &agent.Session{
		ID:               c.config.ID,
		Repository:       c.config.Repository,
		Tasks:            c.config.Tasks,
		WorkDir:          c.workDir,
		GitHubToken:      c.gitHubToken,
		MaxIterations:    c.config.MaxIterations,
		MaxDuration:      c.config.MaxDuration,
		Prompt:           prompt,
		Metadata:         make(map[string]string),
		ClaudeAuthMode:   c.config.ClaudeAuth.AuthMode,
		SystemPrompt:     c.systemPrompt,
		ProjectPrompt:    projectPrompt,
		ActiveTask:       c.activeTask,
		ExistingWork:     c.activeTaskExistingWork,
		Interactive:      c.config.Interactive,
		IterationContext: &agent.IterationContext{},
		PackagePath:      c.packagePath,
	}

	// Compose phase-aware skills if selector is available
	if c.skillSelector != nil {
		phase := c.determineActivePhase()
		phaseStr := string(phase)
		session.IterationContext.Phase = phaseStr
		session.IterationContext.SkillsPrompt = c.skillSelector.SelectForPhase(phaseStr)
		c.logInfo("Using skills for phase %s: %v", phase, c.skillSelector.SkillsForPhase(phaseStr))
	}

	// Inject structured handoff context if enabled
	handoffInjected := false
	if c.isHandoffEnabled() {
		taskID := taskKey(c.activeTaskType, c.activeTask)
		phase := handoff.Phase(c.determineActivePhase())
		phaseInput, err := c.handoffBuilder.BuildMarkdownContext(taskID, phase)
		if err != nil {
			c.logWarning("Failed to build handoff context for phase %s: %v (falling back to memory)", phase, err)
		} else if phaseInput != "" {
			session.IterationContext.PhaseInput = phaseInput
			handoffInjected = true
			c.logInfo("Injected handoff context for phase %s (%d chars)", phase, len(phaseInput))
		}
	}

	// Inject feedback from previous iteration for ITERATE cycles
	// This ensures workers receive both reviewer analysis and judge directives
	if c.memoryStore != nil {
		taskID := taskKey(c.activeTaskType, c.activeTask)
		state := c.taskStates[taskID]
		if state != nil && state.PhaseIteration > 1 {
			feedbackSection := c.buildIterateFeedbackSection(taskID, state.PhaseIteration, state.ParentBranch)
			if feedbackSection != "" {
				// Prepend to PhaseInput for maximum visibility
				if session.IterationContext.PhaseInput != "" {
					session.IterationContext.PhaseInput = feedbackSection + "\n\n" + session.IterationContext.PhaseInput
				} else {
					session.IterationContext.PhaseInput = feedbackSection
				}
				c.logInfo("Injected ITERATE feedback section (%d chars)", len(feedbackSection))
			}
		}
	}

	// Inject memory context as fallback if handoff wasn't injected
	// This ensures PR tasks and unsupported phases still get context
	if c.memoryStore != nil && !handoffInjected {
		taskID := taskKey(c.activeTaskType, c.activeTask)
		memCtx := c.memoryStore.BuildContext(taskID)
		if memCtx != "" {
			session.IterationContext.MemoryContext = memCtx
		}
	}

	// Select adapter and model based on routing config
	activeAgent := c.agent
	if c.modelRouter != nil && c.modelRouter.IsConfigured() {
		phase := c.determineActivePhase()
		phaseStr := string(phase)
		modelCfg := c.modelRouter.ModelForPhase(phaseStr)
		if modelCfg.Adapter != "" {
			if a, ok := c.adapters[modelCfg.Adapter]; ok {
				activeAgent = a
			} else {
				c.logWarning("Phase %s: configured adapter %q not found in initialized adapters, using default %q", phase, modelCfg.Adapter, c.agent.Name())
			}
		}
		if modelCfg.Model != "" {
			session.IterationContext.ModelOverride = modelCfg.Model
		}
		if modelCfg.Reasoning != "" {
			session.IterationContext.ReasoningOverride = modelCfg.Reasoning
		}
		c.logInfo("Routing phase %s: adapter=%s model=%s", phase, activeAgent.Name(), modelCfg.Model)
	}

	// Build environment and command
	env := activeAgent.BuildEnv(session, c.iteration)
	command := activeAgent.BuildCommand(session, c.iteration)

	// Check if agent supports stdin-based prompt delivery (for non-interactive mode)
	stdinPrompt := ""
	if !c.config.Interactive {
		if provider, ok := activeAgent.(agent.StdinPromptProvider); ok {
			stdinPrompt = provider.GetStdinPrompt(session, c.iteration)
		}
	}

	params := containerRunParams{
		Agent:       activeAgent,
		Session:     session,
		Env:         env,
		Command:     command,
		LogTag:      "Agent",
		StdinPrompt: stdinPrompt,
	}

	// Use interactive Docker execution in local mode
	if c.config.Interactive {
		return c.runAgentContainerInteractive(ctx, params)
	}

	execStart := time.Now()
	result, err := c.runAgentContainer(ctx, params)
	execDuration := time.Since(execStart)

	// Attempt fallback on adapter execution failure
	if err != nil && c.canFallback(activeAgent.Name(), session) {
		stderr := ""
		if result != nil {
			stderr = result.Error
		}

		if isAdapterExecutionFailure(err, stderr, execDuration) {
			fallbackName := c.getFallbackAdapter()
			if fallbackName == activeAgent.Name() {
				c.logWarning("Adapter %s failed (%v), retrying without model override",
					activeAgent.Name(), err)
			} else {
				c.logWarning("Adapter %s failed (%v), falling back to %s",
					activeAgent.Name(), err, fallbackName)
			}

			fallbackAdapter := c.adapters[fallbackName]
			fallbackParams := c.buildFallbackParams(fallbackAdapter, session, activeAgent.Name())
			return c.runAgentContainer(ctx, fallbackParams)
		}
	}

	return result, err
}

// buildFallbackParams constructs container run parameters for the fallback adapter.
// It clones the session without model override so the fallback adapter uses its defaults.
func (c *Controller) buildFallbackParams(adapter agent.Agent, session *agent.Session, originalAdapter string) containerRunParams {
	// Clone session without model override (use fallback adapter's default)
	fallbackSession := *session
	if fallbackSession.IterationContext != nil {
		ctx := *fallbackSession.IterationContext
		ctx.ModelOverride = ""
		fallbackSession.IterationContext = &ctx
	}

	env := adapter.BuildEnv(&fallbackSession, c.iteration)
	cmd := adapter.BuildCommand(&fallbackSession, c.iteration)

	var stdinPrompt string
	if provider, ok := adapter.(agent.StdinPromptProvider); ok {
		stdinPrompt = provider.GetStdinPrompt(&fallbackSession, c.iteration)
	}

	return containerRunParams{
		Agent:       adapter,
		Session:     &fallbackSession,
		Env:         env,
		Command:     cmd,
		LogTag:      fmt.Sprintf("Agent (fallback from %s)", originalAdapter),
		StdinPrompt: stdinPrompt,
	}
}

func (c *Controller) emitFinalLogs() {
	// Final metadata update so the provisioner sees the terminal state
	c.updateInstanceMetadata(context.Background())

	c.logInfo("=== Session Summary ===")
	c.logInfo("Session ID: %s", c.config.ID)
	c.logInfo("Duration: %s", time.Since(c.startTime).Round(time.Second))
	c.logInfo("Iterations: %d/%d", c.iteration, c.config.MaxIterations)

	// Count completed tasks using taskStates
	completedCount := 0
	for _, state := range c.taskStates {
		if state.Phase == PhaseComplete || state.Phase == PhaseNothingToDo {
			completedCount++
		}
	}
	c.logInfo("Tasks completed: %d/%d", completedCount, len(c.taskStates))

	// Log task state summary
	for taskID, state := range c.taskStates {
		c.logInfo("Task %s: phase=%s, retries=%d", taskID, state.Phase, state.TestRetries)
	}

	c.logInfo("======================")
}

func (c *Controller) cleanup() {
	c.logInfo("Initiating graceful shutdown")

	// Execute shutdown with timeout
	c.gracefulShutdown()
}

// gracefulShutdown performs a controlled shutdown sequence:
// 1. Flush pending log writes (with timeout)
// 2. Run registered shutdown hooks
// 3. Clear sensitive data from memory
// 4. Close clients
// 5. Terminate VM
func (c *Controller) gracefulShutdown() {
	c.shutdownOnce.Do(func() {
		close(c.shutdownCh)

		// Create a timeout context for the entire shutdown sequence
		ctx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
		defer cancel()

		// Step 1: Flush pending log writes with timeout
		c.flushLogs(ctx)

		// Step 2: Run registered shutdown hooks
		c.runShutdownHooks(ctx)

		// Step 3: Clear sensitive data from memory
		c.clearSensitiveData()

		// Step 4: Close clients
		if c.eventSink != nil {
			if err := c.eventSink.Close(); err != nil {
				c.logWarning("failed to close event sink: %v", err)
			}
		}
		if c.metadataUpdater != nil {
			if err := c.metadataUpdater.Close(); err != nil {
				c.logWarning("failed to close metadata updater: %v", err)
			}
		}
		if c.cloudLogger != nil {
			if err := c.cloudLogger.Close(); err != nil {
				c.logWarning("failed to close cloud logger: %v", err)
			}
		}
		if c.secretManager != nil {
			if err := c.secretManager.Close(); err != nil {
				c.logWarning("failed to close Secret Manager client: %v", err)
			}
		}

		c.logInfo("Graceful shutdown complete")

		// Step 5: Request VM termination (last action)
		c.terminateVM()
	})
}

// flushLogs ensures all pending log writes are sent before shutdown.
// It uses a timeout to prevent blocking indefinitely on log flush.
func (c *Controller) flushLogs(ctx context.Context) {
	if c.logFlushFn == nil {
		return
	}

	c.logInfo("Flushing pending log writes...")

	// Create a sub-context with log flush timeout
	flushCtx, cancel := context.WithTimeout(ctx, LogFlushTimeout)
	defer cancel()

	// Run flush in a goroutine so we can respect the timeout
	done := make(chan error, 1)
	go func() {
		done <- c.logFlushFn()
	}()

	select {
	case err := <-done:
		if err != nil {
			c.logWarning("log flush completed with error: %v", err)
		} else {
			c.logInfo("Log flush completed successfully")
		}
	case <-flushCtx.Done():
		c.logWarning("log flush timed out, some logs may be lost")
	}
}

// runShutdownHooks executes all registered shutdown hooks in order.
// Each hook receives the shutdown context and should respect cancellation.
func (c *Controller) runShutdownHooks(ctx context.Context) {
	if len(c.shutdownHooks) == 0 {
		return
	}

	c.logInfo("Running %d shutdown hooks", len(c.shutdownHooks))

	for i, hook := range c.shutdownHooks {
		select {
		case <-ctx.Done():
			c.logWarning("shutdown timeout reached, skipping remaining %d hooks", len(c.shutdownHooks)-i)
			return
		default:
		}

		if err := hook(ctx); err != nil {
			c.logWarning("shutdown hook %d failed: %v", i+1, err)
		}
	}
}

// clearSensitiveData removes sensitive information from memory
func (c *Controller) clearSensitiveData() {
	c.logInfo("Clearing sensitive data from memory")

	// Clear GitHub token
	c.gitHubToken = ""

	// Clear prompt content (may contain sensitive context)
	c.config.Prompt = ""

	// Clear Claude auth data
	c.config.ClaudeAuth.AuthJSONBase64 = ""

	// Clear Codex auth data
	c.config.CodexAuth.AuthJSONBase64 = ""

	// Clear GitHub app credentials
	c.config.GitHub.PrivateKeySecret = ""
}

// setupSignalHandler sets up OS signal handling for graceful shutdown.
// It returns a new context that will be cancelled when a shutdown signal is received.
func (c *Controller) setupSignalHandler(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		select {
		case sig := <-sigCh:
			c.logInfo("Received signal %v, initiating graceful shutdown", sig)
			cancel()
		case <-ctx.Done():
			// Context was cancelled by other means
		}
		signal.Stop(sigCh)
	}()

	return ctx, cancel
}

// execCommand returns the command runner, defaulting to exec.CommandContext.
func (c *Controller) execCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	if c.cmdRunner != nil {
		return c.cmdRunner(ctx, name, args...)
	}
	return exec.CommandContext(ctx, name, args...)
}

func (c *Controller) terminateVM() {
	// Skip VM termination in interactive mode (no VM to terminate)
	if c.config.Interactive {
		c.logInfo("Skipping VM termination (interactive mode)")
		return
	}

	c.logInfo("Initiating VM termination")

	ctx, cancel := context.WithTimeout(context.Background(), VMTerminationTimeout)
	defer cancel()

	// Get instance name from metadata
	cmd := c.execCommand(ctx, "curl", "-s", "-H", "Metadata-Flavor: Google",
		"http://metadata.google.internal/computeMetadata/v1/instance/name")
	instanceName, err := cmd.Output()
	if err != nil {
		c.logError("failed to get instance name from metadata: %v — VM will not be deleted", err)
		return
	}

	// Get zone from metadata
	cmd = c.execCommand(ctx, "curl", "-s", "-H", "Metadata-Flavor: Google",
		"http://metadata.google.internal/computeMetadata/v1/instance/zone")
	zone, err := cmd.Output()
	if err != nil {
		c.logError("failed to get zone from metadata: %v — VM will not be deleted", err)
		return
	}

	// Delete instance (blocks until completion or timeout)
	name := strings.TrimSpace(string(instanceName))
	zoneName := filepath.Base(strings.TrimSpace(string(zone)))
	c.logInfo("Deleting VM instance %s in zone %s", name, zoneName)

	cmd = c.execCommand(ctx, "gcloud", "compute", "instances", "delete",
		name,
		"--zone", zoneName,
		"--quiet",
	)

	if err := cmd.Run(); err != nil {
		c.logError("VM deletion command failed: %v — VM may remain running until max_run_duration", err)
		return
	}

	c.logInfo("VM deletion command completed successfully")
}
