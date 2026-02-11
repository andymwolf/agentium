package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
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
	"github.com/andywolf/agentium/internal/routing"
	"github.com/andywolf/agentium/internal/scope"
	"github.com/andywolf/agentium/internal/version"
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
	Handoff    struct{}              `json:"handoff,omitempty"` // Kept for config compatibility; handoff is always enabled
	Routing    *routing.PhaseRouting `json:"routing,omitempty"`
	Delegation *DelegationConfig     `json:"delegation,omitempty"`
	PhaseLoop  *PhaseLoopConfig      `json:"phase_loop,omitempty"`
	Fallback   *FallbackConfig       `json:"fallback,omitempty"`
	Phases     []PhaseStepConfig     `json:"phases,omitempty"`
	Verbose    bool                  `json:"verbose,omitempty"`
	AutoMerge  bool                  `json:"auto_merge,omitempty"`
	Langfuse LangfuseSessionConfig `json:"langfuse,omitempty"`
	Monorepo *MonorepoSessionConfig `json:"monorepo,omitempty"`
}

// PhaseStepConfig defines the configuration for a single phase step.
// When Phases is provided in SessionConfig, the phase order is derived from it
// and API-provided prompts replace the built-in skills.
type PhaseStepConfig struct {
	Name          string             `json:"name"`
	MaxIterations int                `json:"max_iterations,omitempty"`
	Worker        *StepPromptConfig  `json:"worker,omitempty"`
	Reviewer      *StepPromptConfig  `json:"reviewer,omitempty"`
	Judge         *JudgePromptConfig `json:"judge,omitempty"`
}

// StepPromptConfig contains an override prompt for a worker or reviewer step.
type StepPromptConfig struct {
	Prompt string `json:"prompt"`
}

// JudgePromptConfig contains override criteria for a judge step.
type JudgePromptConfig struct {
	Criteria string `json:"criteria"`
}

// LangfuseSessionConfig contains Langfuse observability settings for the session.
type LangfuseSessionConfig struct {
	PublicKeySecret string `json:"public_key_secret,omitempty"`
	SecretKeySecret string `json:"secret_key_secret,omitempty"`
	BaseURL         string `json:"base_url,omitempty"`
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

	// Custom phase step configs (indexed by phase name for O(1) lookup)
	phaseConfigs map[TaskPhase]*PhaseStepConfig

	// Docker container resource limits
	containerMemLimit uint64 // Docker --memory limit in bytes (0 = no limit)

	// Monorepo support
	packagePath    string                // Current package path for monorepo scope (empty if not monorepo)
	scopeValidator *scope.ScopeValidator // Validates file changes are within package scope (nil if not monorepo)

	// Parent issue -> sub-issue expansion
	parentSubIssues map[string][]string // parent issue ID -> sub-issue IDs
	subIssueCache   map[string][]string // issueID → cached open sub-issue IDs

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
		config:          config,
		agent:           agentAdapter,
		workDir:         workDir,
		iteration:       0,
		maxDuration:     maxDuration,
		taskStates:      make(map[string]*TaskState),
		logger:          logger,
		cloudLogger:     cloudLogger,
		secretManager:   secretManager,
		metadataUpdater: metadataUpdater,
		shutdownCh:      make(chan struct{}),
		parentSubIssues: make(map[string][]string),
		subIssueCache:   make(map[string][]string),
		tracer:          &observability.NoOpTracer{},
	}

	// Build phaseConfigs map from Phases slice for O(1) lookup
	if len(config.Phases) > 0 {
		if err := validatePhases(config.Phases); err != nil {
			return nil, fmt.Errorf("invalid phases config: %w", err)
		}
		c.phaseConfigs = make(map[TaskPhase]*PhaseStepConfig, len(config.Phases))
		for i := range config.Phases {
			c.phaseConfigs[TaskPhase(config.Phases[i].Name)] = &config.Phases[i]
		}
	}

	// Initialize task states and build task queue
	initialIssuePhase := PhaseImplement
	if c.isPhaseLoopEnabled() {
		initialIssuePhase = PhasePlan
	}
	if len(config.Phases) > 0 {
		initialIssuePhase = TaskPhase(config.Phases[0].Name)
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
	c.logInfo("Max duration: %s", c.maxDuration)

	// Initialize workspace
	if err := c.initializeWorkspace(ctx); err != nil {
		return fmt.Errorf("failed to initialize workspace: %w", err)
	}

	// Compute Docker memory limit from VM total memory
	total, _, memErr := readMemInfo()
	if memErr != nil {
		c.logInfo("Container memory limit: not set (/proc/meminfo unavailable)")
	} else {
		const reserveBytes = 1 << 30 // 1 GB for OS + Docker daemon + controller
		if total > reserveBytes {
			c.containerMemLimit = total - reserveBytes
			c.logInfo("Agent container memory limit: %d MB (VM total: %d MB, reserved: 1024 MB)",
				c.containerMemLimit/(1024*1024), total/(1024*1024))
		}
	}

	// Fetch GitHub token
	if err := c.fetchGitHubToken(ctx); err != nil {
		return fmt.Errorf("failed to fetch GitHub token: %w", err)
	}

	// Initialize Langfuse tracer (env vars or Secret Manager)
	c.initTracer(ctx, c.logger)

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

		// First check: does this issue have open sub-issues?
		subIssueIDs, detectErr := c.detectSubIssues(ctx, nextTask.ID)
		if detectErr != nil {
			// Hard-fail: if we cannot determine sub-issue structure, we must not
			// guess — processing a parent issue as a leaf task produces wrong work.
			return fmt.Errorf("cannot continue without GitHub API: %w", detectErr)
		}
		if len(subIssueIDs) > 0 {
			taskID := taskKey("issue", nextTask.ID)
			if expandErr := c.expandParentIssue(ctx, nextTask.ID, subIssueIDs); expandErr != nil {
				c.logError("Sub-issue expansion failed for #%s: %v", nextTask.ID, expandErr)
				if state, ok := c.taskStates[taskID]; ok {
					state.Phase = PhaseBlocked
				}
			} else {
				c.logInfo("Issue #%s: expanded %d sub-issues %v — parent complete", nextTask.ID, len(subIssueIDs), subIssueIDs)
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

		// Reset workspace to main branch to prevent branch state from leaking
		// between tasks (e.g., task N+1 inheriting task N's feature branch).
		c.resetWorkspaceToMain(ctx)
	}

	// Post final parent status comments
	for parentID, subIDs := range c.parentSubIssues {
		c.postParentStatusComment(ctx, parentID, subIDs, "completed")
	}

	return nil
}

// resetWorkspaceToMain checks out the main branch in the workspace directory.
// This prevents branch state from leaking between tasks — without it, task N+1
// would start on task N's feature branch, causing maybeCreateDraftPR to associate
// the wrong PR and VERIFY to merge the wrong PR.
func (c *Controller) resetWorkspaceToMain(ctx context.Context) {
	cmd := exec.CommandContext(ctx, "git", "checkout", "main")
	cmd.Dir = c.workDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		c.logWarning("Failed to reset workspace to main: %v (output: %s)", err, strings.TrimSpace(string(output)))
		return
	}
	c.logInfo("Workspace reset to main branch")
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

// isPhaseLoopEnabled returns true if the phase loop is configured.
func (c *Controller) isPhaseLoopEnabled() bool {
	return c.config.PhaseLoop != nil
}

// isHandoffEnabled returns true if the handoff store is initialized.
func (c *Controller) isHandoffEnabled() bool {
	return c.handoffStore != nil
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

// execCommand returns the command runner, defaulting to exec.CommandContext.
func (c *Controller) execCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	if c.cmdRunner != nil {
		return c.cmdRunner(ctx, name, args...)
	}
	return exec.CommandContext(ctx, name, args...)
}
