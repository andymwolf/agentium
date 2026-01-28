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
	"github.com/andywolf/agentium/internal/cloud/gcp"
	"github.com/andywolf/agentium/internal/github"
	"github.com/andywolf/agentium/internal/handoff"
	"github.com/andywolf/agentium/internal/memory"
	"github.com/andywolf/agentium/internal/prompt"
	"github.com/andywolf/agentium/internal/routing"
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

// Version is the controller version, set at build time via ldflags.
var Version = "dev"

// TaskPhase represents the current phase of a task in its lifecycle
type TaskPhase string

const (
	PhasePlan        TaskPhase = "PLAN"
	PhaseImplement   TaskPhase = "IMPLEMENT"
	PhaseDocs        TaskPhase = "DOCS"
	PhasePRCreation  TaskPhase = "PR_CREATION"
	PhaseComplete    TaskPhase = "COMPLETE"
	PhaseBlocked     TaskPhase = "BLOCKED"
	PhaseNothingToDo TaskPhase = "NOTHING_TO_DO"
	// PR-specific phases
	PhaseAnalyze TaskPhase = "ANALYZE"
	PhasePush    TaskPhase = "PUSH"
)

// WorkflowPath represents the complexity path determined after PLAN iteration 1.
type WorkflowPath string

const (
	WorkflowPathUnset   WorkflowPath = ""        // Not yet determined
	WorkflowPathSimple  WorkflowPath = "SIMPLE"  // Straightforward change, fewer iterations
	WorkflowPathComplex WorkflowPath = "COMPLEX" // Multiple components, full review
)

// ComplexityVerdict represents the outcome of complexity assessment.
type ComplexityVerdict string

const (
	ComplexitySimple  ComplexityVerdict = "SIMPLE"
	ComplexityComplex ComplexityVerdict = "COMPLEX"
)

// TaskState tracks the current state of a task being worked on
type TaskState struct {
	ID                 string
	Type               string // "issue" or "pr"
	Phase              TaskPhase
	TestRetries        int
	LastStatus         string
	PRNumber           string       // Linked PR number (for issues that create PRs)
	PhaseIteration     int          // Current iteration within the active phase (phase loop)
	MaxPhaseIterations int          // Max iterations for current phase (phase loop)
	LastJudgeVerdict   string       // Last judge verdict (ADVANCE, ITERATE, BLOCKED)
	LastJudgeFeedback  string       // Last judge feedback text
	DraftPRCreated     bool         // Whether draft PR has been created for this task
	WorkflowPath       WorkflowPath // Set after PLAN iteration 1 (SIMPLE or COMPLEX)
	ControllerOverrode bool         // True if controller forced ADVANCE at max iterations (triggers NOMERGE)
	ParentBranch       string       // Parent issue's branch to base this task on (for dependency chains)
}

// PhaseLoopConfig controls the controller-as-judge phase loop behavior.
type PhaseLoopConfig struct {
	Enabled                bool `json:"enabled"`
	SkipPlanIfExists       bool `json:"skip_plan_if_exists,omitempty"`
	PlanMaxIterations      int  `json:"plan_max_iterations,omitempty"`
	ImplementMaxIterations int  `json:"implement_max_iterations,omitempty"`
	DocsMaxIterations      int  `json:"docs_max_iterations,omitempty"`
	JudgeContextBudget     int  `json:"judge_context_budget,omitempty"`
	JudgeNoSignalLimit     int  `json:"judge_no_signal_limit,omitempty"`
}

// SessionConfig is the configuration passed to the controller
type SessionConfig struct {
	ID                   string   `json:"id"`
	CloudProvider        string   `json:"cloud_provider,omitempty"` // Cloud provider (gcp, aws, azure, local)
	Repository           string   `json:"repository"`
	Tasks                []string `json:"tasks"`
	PRs                  []string `json:"prs,omitempty"`
	Agent                string   `json:"agent"`
	MaxIterations        int      `json:"max_iterations"`
	MaxDuration          string   `json:"max_duration"`
	Prompt               string   `json:"prompt"`
	Interactive          bool     `json:"interactive,omitempty"`            // Local interactive mode (no cloud clients)
	CloneInsideContainer bool     `json:"clone_inside_container,omitempty"` // Clone repository inside Docker container
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
	Handoff struct{} `json:"handoff,omitempty"` // Kept for config compatibility; handoff is always enabled
	Routing    *routing.PhaseRouting `json:"routing,omitempty"`
	Delegation *DelegationConfig     `json:"delegation,omitempty"`
	PhaseLoop  *PhaseLoopConfig      `json:"phase_loop,omitempty"`
	Verbose    bool                  `json:"verbose,omitempty"`
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
	pushedChanges          bool                 // Tracks if changes were pushed (for PR review sessions)
	dockerAuthed           bool                 // Tracks if docker login to GHCR was done
	taskStates             map[string]*TaskState
	logger                 *log.Logger
	cloudLogger            *gcp.CloudLogger // Structured cloud logging (may be nil if unavailable)
	secretManager          gcp.SecretFetcher
	systemPrompt           string                 // Loaded SYSTEM.md content
	projectPrompt          string                 // Loaded .agentium/AGENT.md content (may be empty)
	taskQueue              []TaskQueueItem        // Ordered queue: PRs first, then issues
	issueDetails           []issueDetail          // Fetched issue details for prompt building
	prDetails              []prWithReviews        // Fetched PR details for prompt building
	activeTask             string                 // Current task ID being focused on
	activeTaskType         string                 // "pr" or "issue"
	activeTaskExistingWork *agent.ExistingWork    // Existing work detected for active task (issues only)
	skillSelector          *skills.Selector       // Phase-aware skill selector (nil = legacy mode)
	memoryStore            *memory.Store          // Persistent memory store (nil = disabled)
	handoffStore           *handoff.Store         // Structured handoff store (nil = disabled)
	handoffBuilder         *handoff.Builder       // Phase input builder (nil = disabled)
	handoffParser          *handoff.Parser        // Handoff signal parser (nil = disabled)
	handoffValidator       *handoff.Validator     // Handoff validation (nil = disabled)
	modelRouter            *routing.Router        // Phase-to-model routing (nil = no routing)
	depGraph               *DependencyGraph       // Inter-issue dependency graph (nil = no dependencies)
	adapters               map[string]agent.Agent // All initialized adapters (for multi-adapter routing)
	orchestrator           *SubTaskOrchestrator   // Sub-task delegation orchestrator (nil = disabled)
	metadataUpdater        gcp.MetadataUpdater    // Instance metadata updater (nil if unavailable)

	// Repo visibility cache (for gist creation)
	repoVisibilityChecked bool // True after first visibility check
	repoIsPublic          bool // Cached result of visibility check

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

// checkoutBranch checks out the given branch, fetching from origin first if needed.
func (c *Controller) checkoutBranch(ctx context.Context, branch string) error {
	checkoutCmd := c.execCommand(ctx, "git", "checkout", branch)
	checkoutCmd.Dir = c.workDir
	if err := checkoutCmd.Run(); err != nil {
		// Fetch and retry (ignore fetch error - checkout will fail if needed)
		fetchCmd := c.execCommand(ctx, "git", "fetch", "origin", branch)
		fetchCmd.Dir = c.workDir
		_ = fetchCmd.Run()

		checkoutCmd = c.execCommand(ctx, "git", "checkout", branch)
		checkoutCmd.Dir = c.workDir
		if err := checkoutCmd.Run(); err != nil {
			return fmt.Errorf("failed to checkout branch %s: %w", branch, err)
		}
	}
	return nil
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

	// In interactive mode, skip cloud client initialization
	var secretManager gcp.SecretFetcher
	var cloudLogger *gcp.CloudLogger
	var metadataUpdater gcp.MetadataUpdater

	if !config.Interactive {
		// Initialize Secret Manager client
		secretManager, err = gcp.NewSecretManagerClient(context.Background())
		if err != nil {
			// Log warning but don't fail - allow fallback to gcloud CLI
			log.Printf("[controller] Warning: failed to initialize Secret Manager client: %v", err)
		}

		// Initialize Cloud Logging (non-fatal if unavailable)
		cloudLoggerInstance, err := gcp.NewCloudLogger(context.Background(), gcp.CloudLoggerConfig{
			SessionID:  config.ID,
			Repository: config.Repository,
		})
		if err != nil {
			log.Printf("[controller] Warning: Cloud Logging unavailable, using local logs only: %v", err)
		} else {
			cloudLogger = cloudLoggerInstance
		}

		// Initialize metadata updater (only on GCP instances)
		if gcp.IsRunningOnGCP() {
			metadataUpdaterInstance, err := gcp.NewComputeMetadataUpdater(context.Background())
			if err != nil {
				log.Printf("[controller] Warning: metadata updater unavailable, session status will not be reported: %v", err)
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
		logger:          log.New(os.Stdout, "[controller] ", log.LstdFlags),
		cloudLogger:     cloudLogger,
		secretManager:   secretManager,
		metadataUpdater: metadataUpdater,
		shutdownCh:      make(chan struct{}),
	}

	// Initialize task states and build unified task queue (PRs first, then issues)
	for _, pr := range config.PRs {
		c.taskStates[taskKey("pr", pr)] = &TaskState{
			ID:    pr,
			Type:  "pr",
			Phase: PhaseAnalyze,
		}
		c.taskQueue = append(c.taskQueue, TaskQueueItem{Type: "pr", ID: pr})
	}
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

	c.logInfo("Controller started (version %s)", Version)
	c.logInfo("Starting session %s", c.config.ID)
	c.logInfo("Repository: %s", c.config.Repository)
	if len(c.config.Tasks) > 0 {
		c.logInfo("Tasks: %v", c.config.Tasks)
	}
	if len(c.config.PRs) > 0 {
		c.logInfo("PRs: %v", c.config.PRs)
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
	if len(c.config.PRs) > 0 {
		c.prDetails = c.fetchPRDetails(ctx)
	}
	if len(c.config.Tasks) > 0 {
		c.issueDetails = c.fetchIssueDetails(ctx)
	}

	// Build inter-issue dependency graph (only for multi-issue batches)
	if len(c.issueDetails) > 1 {
		c.buildDependencyGraph()
	}

	c.logInfo("Task queue: %d issue(s) [%s], %d PR(s)", len(c.config.Tasks), strings.Join(c.config.Tasks, ", "), len(c.config.PRs))

	// Main execution loop - processes tasks sequentially (PRs first, then issues)
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

		// Build prompt based on task type
		if nextTask.Type == "pr" {
			c.logInfo("Focusing on PR #%s", nextTask.ID)
			if err := c.preparePRTask(ctx, nextTask.ID); err != nil {
				c.logError("Failed to prepare PR #%s: %v", nextTask.ID, err)
				// Mark as blocked and continue to next task
				taskID := taskKey("pr", nextTask.ID)
				if state, ok := c.taskStates[taskID]; ok {
					state.Phase = PhaseBlocked
				}
				continue
			}
		} else {
			c.logInfo("Focusing on issue #%s", nextTask.ID)

			// Resolve parent branch for dependency chains
			taskID := taskKey("issue", nextTask.ID)
			state := c.taskStates[taskID]
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

			// Use phase loop if enabled for issue tasks
			if c.isPhaseLoopEnabled() {
				if err := c.runPhaseLoop(ctx); err != nil {
					c.logError("Phase loop failed for issue #%s: %v", nextTask.ID, err)
				}
				continue
			}
		}

		c.iteration++
		if c.cloudLogger != nil {
			c.cloudLogger.SetIteration(c.iteration)
		}
		c.logInfo("Starting iteration %d/%d", c.iteration, c.config.MaxIterations)

		// Run agent iteration
		result, err := c.runIteration(ctx)
		if err != nil {
			c.logError("Iteration %d failed: %v", c.iteration, err)
			continue
		}

		c.logInfo("Iteration %d completed: %s", c.iteration, result.Summary)

		// Log agent status if present
		if result.AgentStatus != "" {
			c.logInfo("Agent status: %s %s", result.AgentStatus, result.StatusMessage)
		}

		// Update task phase for the active task
		taskID := taskKey(c.activeTaskType, c.activeTask)
		c.updateTaskPhase(taskID, result)

		// Update instance metadata with current session status
		c.updateInstanceMetadata(ctx)

		// Check PRs created (for issue sessions)
		if len(result.PRsCreated) > 0 {
			c.logInfo("PRs created: %v", result.PRsCreated)
		}

		// Check if changes were pushed (for PR review sessions)
		if result.PushedChanges {
			c.pushedChanges = true
			c.logInfo("Changes pushed to PR branch")
		}
	}

	// Emit final logs
	c.emitFinalLogs()

	// Cleanup
	c.cleanup()

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
		c.logger.Printf("Warning: Secret Manager client failed: %v, falling back to gcloud CLI", err)
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

	// Load project prompt from workspace (.agentium/AGENT.md) - optional
	projectPrompt, err := prompt.LoadProjectPrompt(c.workDir)
	if err != nil {
		c.logWarning("failed to load project prompt: %v", err)
	} else if projectPrompt != "" {
		c.projectPrompt = projectPrompt
		c.logInfo("Project prompt loaded from .agentium/AGENT.md")
	}

	// Initialize persistent memory store if enabled
	if c.config.Memory.Enabled {
		c.memoryStore = memory.NewStore(c.workDir, memory.Config{
			Enabled:       true,
			MaxEntries:    c.config.Memory.MaxEntries,
			ContextBudget: c.config.Memory.ContextBudget,
		})
		if err := c.memoryStore.Load(); err != nil {
			c.logWarning("failed to load memory store: %v", err)
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

type prDetail struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	HeadRefName string `json:"headRefName"`
}

type prReview struct {
	State string `json:"state"`
	Body  string `json:"body"`
}

type prComment struct {
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Body     string `json:"body"`
	DiffHunk string `json:"diffHunk"`
}

func (c *Controller) fetchIssueDetails(ctx context.Context) []issueDetail {
	c.logInfo("Fetching issue details")

	issues := make([]issueDetail, 0, len(c.config.Tasks))

	for _, taskID := range c.config.Tasks {
		// Use gh CLI to fetch issue
		cmd := c.execCommand(ctx, "gh", "issue", "view", taskID,
			"--repo", c.config.Repository,
			"--json", "number,title,body,labels",
		)
		cmd.Env = c.envWithGitHubToken()

		output, err := cmd.Output()
		if err != nil {
			c.logger.Printf("Warning: failed to fetch issue #%s: %v", taskID, err)
			continue
		}

		var issue issueDetail
		if err := json.Unmarshal(output, &issue); err != nil {
			c.logger.Printf("Warning: failed to parse issue #%s: %v", taskID, err)
			continue
		}

		issues = append(issues, issue)
	}

	return issues
}

type prWithReviews struct {
	Detail   prDetail
	Reviews  []prReview
	Comments []prComment
}

func (c *Controller) fetchPRDetails(ctx context.Context) []prWithReviews {
	c.logInfo("Fetching PR details")

	prs := make([]prWithReviews, 0, len(c.config.PRs))

	for _, prNumber := range c.config.PRs {
		// Fetch basic PR info
		cmd := c.execCommand(ctx, "gh", "pr", "view", prNumber,
			"--repo", c.config.Repository,
			"--json", "number,title,body,headRefName",
		)
		cmd.Env = c.envWithGitHubToken()

		output, err := cmd.Output()
		if err != nil {
			c.logger.Printf("Warning: failed to fetch PR #%s: %v", prNumber, err)
			continue
		}

		var pr prDetail
		if err := json.Unmarshal(output, &pr); err != nil {
			c.logger.Printf("Warning: failed to parse PR #%s: %v", prNumber, err)
			continue
		}

		prWithRev := prWithReviews{Detail: pr}

		// Fetch reviews requesting changes
		repoPath := c.config.Repository
		reviewCmd := c.execCommand(ctx, "gh", "api",
			fmt.Sprintf("repos/%s/pulls/%s/reviews", repoPath, prNumber),
			"--jq", `[.[] | select(.state == "CHANGES_REQUESTED" or .state == "COMMENTED") | {state: .state, body: .body}]`,
		)
		reviewCmd.Env = c.envWithGitHubToken()

		if reviewOutput, err := reviewCmd.Output(); err == nil {
			var reviews []prReview
			if err := json.Unmarshal(reviewOutput, &reviews); err == nil {
				prWithRev.Reviews = reviews
			}
		}

		// Fetch inline review comments
		commentCmd := c.execCommand(ctx, "gh", "api",
			fmt.Sprintf("repos/%s/pulls/%s/comments", repoPath, prNumber),
			"--jq", `[.[] | {path: .path, line: .line, body: .body, diffHunk: .diff_hunk}]`,
		)
		commentCmd.Env = c.envWithGitHubToken()

		if commentOutput, err := commentCmd.Output(); err == nil {
			var comments []prComment
			if err := json.Unmarshal(commentOutput, &comments); err == nil {
				prWithRev.Comments = comments
			}
		}

		prs = append(prs, prWithRev)
	}

	return prs
}

// nextQueuedTask returns the first task in the queue that hasn't reached a terminal phase.
// The queue is ordered PRs first, then issues, ensuring sequential processing.
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

// preparePRTask checks out the PR branch and builds the prompt for a single PR review task.
func (c *Controller) preparePRTask(ctx context.Context, prNumber string) error {
	// Find the PR detail from pre-fetched data
	var prData *prWithReviews
	for i := range c.prDetails {
		if fmt.Sprintf("%d", c.prDetails[i].Detail.Number) == prNumber {
			prData = &c.prDetails[i]
			break
		}
	}

	if prData == nil {
		return fmt.Errorf("PR #%s not found in fetched details", prNumber)
	}

	// Checkout PR branch
	c.logInfo("Checking out PR branch: %s", prData.Detail.HeadRefName)
	if err := c.checkoutBranch(ctx, prData.Detail.HeadRefName); err != nil {
		return err
	}

	// Build prompt for this single PR
	c.config.Prompt = c.buildPromptForPR(*prData)
	c.activeTaskExistingWork = nil // Not applicable for PR reviews
	return nil
}

// buildPromptForPR builds a focused prompt for a single PR review task.
func (c *Controller) buildPromptForPR(pr prWithReviews) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("You are working on repository: %s\n\n", c.config.Repository))
	sb.WriteString("## PR REVIEW SESSION\n\n")
	sb.WriteString("You are addressing code review feedback on an existing pull request.\n\n")

	sb.WriteString(fmt.Sprintf("### PR #%d: %s\n", pr.Detail.Number, pr.Detail.Title))
	sb.WriteString(fmt.Sprintf("Branch: %s\n\n", pr.Detail.HeadRefName))

	if len(pr.Reviews) > 0 {
		sb.WriteString("**Review Feedback:**\n")
		for _, review := range pr.Reviews {
			if review.Body != "" {
				body := review.Body
				if len(body) > MaxReviewBodyLen {
					body = body[:MaxReviewBodyLen] + "..."
				}
				sb.WriteString(fmt.Sprintf("- [%s] %s\n", review.State, body))
			}
		}
		sb.WriteString("\n")
	}

	if len(pr.Comments) > 0 {
		sb.WriteString("**Inline Comments:**\n")
		for _, comment := range pr.Comments {
			body := comment.Body
			if len(body) > MaxCommentBodyLen {
				body = body[:MaxCommentBodyLen] + "..."
			}
			sb.WriteString(fmt.Sprintf("- File: %s (line %d)\n", comment.Path, comment.Line))
			sb.WriteString(fmt.Sprintf("  Comment: %s\n", body))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Instructions\n\n")
	sb.WriteString("1. You are ALREADY on the PR branch - do NOT create a new branch\n")
	sb.WriteString("2. Read and understand the review comments\n")
	sb.WriteString("3. Make targeted changes to address the feedback\n")
	sb.WriteString("4. Run tests to verify your changes\n")
	sb.WriteString("5. Commit with message: \"Address review feedback\"\n")
	sb.WriteString("6. Push your changes: `git push origin HEAD`\n\n")
	sb.WriteString("## DO NOT\n\n")
	sb.WriteString("- Create a new branch (you're already on the PR branch)\n")
	sb.WriteString("- Close or merge the PR\n")
	sb.WriteString("- Dismiss reviews\n")
	sb.WriteString("- Force push (unless absolutely necessary)\n")
	sb.WriteString("- Make unrelated changes\n")

	return sb.String()
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

// buildPromptForTask builds a focused prompt for a single issue, incorporating existing work context.
// The phase parameter controls whether implementation instructions are included:
// - For IMPLEMENT phase (or empty phase): include full implementation instructions
// - For other phases (PLAN, DOCS, etc.): defer to the phase-specific system prompt
func (c *Controller) buildPromptForTask(issueNumber string, existingWork *agent.ExistingWork, phase TaskPhase) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("You are working on repository: %s\n\n", c.config.Repository))

	// Find the issue detail
	var issue *issueDetail
	for i := range c.issueDetails {
		if fmt.Sprintf("%d", c.issueDetails[i].Number) == issueNumber {
			issue = &c.issueDetails[i]
			break
		}
	}

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
	if phase == PhaseImplement || phase == "" {
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
	} else {
		// For PLAN, DOCS, and other phases: defer to the phase-specific system prompt
		sb.WriteString("### Instructions\n\n")
		sb.WriteString("Follow the instructions in your system prompt to complete this phase.\n")
		sb.WriteString(fmt.Sprintf("The repository is cloned at %s.\n", c.workDir))
	}

	return sb.String()
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
		if state.Type == "issue" {
			state.Phase = PhasePRCreation
		} else {
			state.Phase = PhasePush
		}
		state.TestRetries = 0 // Reset retries on success
	case "TESTS_FAILED":
		state.TestRetries++
		if state.TestRetries >= 3 {
			state.Phase = PhaseBlocked
			c.logger.Printf("Task %s blocked after %d test failures", taskID, state.TestRetries)
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
	case "ANALYZING":
		state.Phase = PhaseAnalyze
	}

	// Fallback: if no explicit status signal but PRs were detected for an issue task,
	// treat the task as complete. This handles agents that create PRs without emitting
	// the AGENTIUM_STATUS: PR_CREATED signal.
	if result.AgentStatus == "" && state.Type == "issue" && len(result.PRsCreated) > 0 {
		state.Phase = PhaseComplete
		state.PRNumber = result.PRsCreated[0]
		c.logger.Printf("Task %s completed via PR detection fallback (PR #%s)", taskID, state.PRNumber)
	}

	state.LastStatus = result.AgentStatus
	c.logger.Printf("Task %s phase: %s (status: %s)", taskID, state.Phase, result.AgentStatus)
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
		c.logger.Println("Max iterations reached")
		return true
	}

	// Check time limit
	if time.Since(c.startTime) >= c.maxDuration {
		c.logger.Println("Max duration reached")
		return true
	}

	// Check if all tasks have reached a terminal phase
	if len(c.taskStates) > 0 {
		allTerminal := true
		for taskID, state := range c.taskStates {
			switch state.Phase {
			case PhaseComplete, PhaseNothingToDo, PhaseBlocked:
				c.logger.Printf("Task %s in terminal phase: %s", taskID, state.Phase)
				continue
			default:
				allTerminal = false
			}
		}
		if allTerminal {
			c.logger.Println("All tasks in terminal phase")
			return true
		}
	}

	// Legacy fallback: PR review sessions check if changes were pushed
	if len(c.config.PRs) > 0 && c.pushedChanges {
		c.logger.Println("PR review complete: changes pushed (legacy detection)")
		return true
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
// PR items remain first in the queue, then issues follow the sorted order.
func (c *Controller) reorderTaskQueue(sortedIDs []string) {
	// Separate PRs from issues
	var prItems []TaskQueueItem
	issueMap := make(map[string]TaskQueueItem)

	for _, item := range c.taskQueue {
		if item.Type == "pr" {
			prItems = append(prItems, item)
		} else {
			issueMap[item.ID] = item
		}
	}

	// Rebuild queue: PRs first, then issues in topological order
	newQueue := make([]TaskQueueItem, 0, len(c.taskQueue))
	newQueue = append(newQueue, prItems...)

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

// isPhaseLoopEnabled returns true if the phase loop is configured and enabled.
func (c *Controller) isPhaseLoopEnabled() bool {
	return c.config.PhaseLoop != nil && c.config.PhaseLoop.Enabled
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

	// Find the issue in issueDetails
	for _, issue := range c.issueDetails {
		if fmt.Sprintf("%d", issue.Number) == c.activeTask {
			return &handoff.IssueContext{
				Number:     issue.Number,
				Title:      issue.Title,
				Body:       issue.Body,
				Repository: c.config.Repository,
			}
		}
	}
	return nil
}

// determineActivePhase returns the current phase for the active task.
// When no task state exists yet (first iteration), defaults are:
// - PhaseAnalyze for PR tasks (agent starts by reading review comments)
// - PhaseImplement for issue tasks (agent starts by writing code)
func (c *Controller) determineActivePhase() TaskPhase {
	taskID := taskKey(c.activeTaskType, c.activeTask)
	if state, ok := c.taskStates[taskID]; ok {
		return state.Phase
	}
	if c.activeTaskType == "pr" {
		return PhaseAnalyze
	}
	return PhaseImplement
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

	session := &agent.Session{
		ID:             c.config.ID,
		Repository:     c.config.Repository,
		Tasks:          c.config.Tasks,
		WorkDir:        c.workDir,
		GitHubToken:    c.gitHubToken,
		MaxIterations:  c.config.MaxIterations,
		MaxDuration:    c.config.MaxDuration,
		Prompt:         prompt,
		Metadata:       make(map[string]string),
		ClaudeAuthMode: c.config.ClaudeAuth.AuthMode,
		SystemPrompt:   c.systemPrompt,
		ProjectPrompt:  c.projectPrompt,
		ActiveTask:     c.activeTask,
		ExistingWork:   c.activeTaskExistingWork,
		Interactive:    c.config.Interactive,
	}

	// Compose phase-aware skills if selector is available
	if c.skillSelector != nil {
		phase := c.determineActivePhase()
		phaseStr := string(phase)
		session.IterationContext = &agent.IterationContext{
			Phase:        phaseStr,
			SkillsPrompt: c.skillSelector.SelectForPhase(phaseStr),
		}
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
			if session.IterationContext == nil {
				session.IterationContext = &agent.IterationContext{}
			}
			session.IterationContext.PhaseInput = phaseInput
			handoffInjected = true
			c.logInfo("Injected handoff context for phase %s (%d chars)", phase, len(phaseInput))
		}
	}

	// Inject memory context as fallback if handoff wasn't injected
	// This ensures PR tasks and unsupported phases still get context
	if c.memoryStore != nil && !handoffInjected {
		taskID := taskKey(c.activeTaskType, c.activeTask)
		memCtx := c.memoryStore.BuildContext(taskID)
		if memCtx != "" {
			if session.IterationContext == nil {
				session.IterationContext = &agent.IterationContext{}
			}
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
			if session.IterationContext == nil {
				session.IterationContext = &agent.IterationContext{}
			}
			session.IterationContext.ModelOverride = modelCfg.Model
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

	return c.runAgentContainer(ctx, params)
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
		if c.metadataUpdater != nil {
			if err := c.metadataUpdater.Close(); err != nil {
				c.logger.Printf("Warning: failed to close metadata updater: %v", err)
			}
		}
		if c.cloudLogger != nil {
			if err := c.cloudLogger.Close(); err != nil {
				c.logger.Printf("Warning: failed to close cloud logger: %v", err)
			}
		}
		if c.secretManager != nil {
			if err := c.secretManager.Close(); err != nil {
				c.logger.Printf("Warning: failed to close Secret Manager client: %v", err)
			}
		}

		c.logger.Println("Graceful shutdown complete")

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

	c.logger.Println("Flushing pending log writes...")

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
			c.logger.Printf("Warning: log flush completed with error: %v", err)
		} else {
			c.logger.Println("Log flush completed successfully")
		}
	case <-flushCtx.Done():
		c.logger.Println("Warning: log flush timed out, some logs may be lost")
	}
}

// runShutdownHooks executes all registered shutdown hooks in order.
// Each hook receives the shutdown context and should respect cancellation.
func (c *Controller) runShutdownHooks(ctx context.Context) {
	if len(c.shutdownHooks) == 0 {
		return
	}

	c.logger.Printf("Running %d shutdown hooks", len(c.shutdownHooks))

	for i, hook := range c.shutdownHooks {
		select {
		case <-ctx.Done():
			c.logger.Printf("Warning: shutdown timeout reached, skipping remaining %d hooks", len(c.shutdownHooks)-i)
			return
		default:
		}

		if err := hook(ctx); err != nil {
			c.logger.Printf("Warning: shutdown hook %d failed: %v", i+1, err)
		}
	}
}

// clearSensitiveData removes sensitive information from memory
func (c *Controller) clearSensitiveData() {
	c.logger.Println("Clearing sensitive data from memory")

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
			c.logger.Printf("Received signal %v, initiating graceful shutdown", sig)
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
		c.logger.Println("Skipping VM termination (interactive mode)")
		return
	}

	c.logger.Println("Initiating VM termination")

	ctx, cancel := context.WithTimeout(context.Background(), VMTerminationTimeout)
	defer cancel()

	// Get instance name from metadata
	cmd := c.execCommand(ctx, "curl", "-s", "-H", "Metadata-Flavor: Google",
		"http://metadata.google.internal/computeMetadata/v1/instance/name")
	instanceName, err := cmd.Output()
	if err != nil {
		c.logger.Printf("Error: failed to get instance name from metadata: %v — VM will not be deleted", err)
		return
	}

	// Get zone from metadata
	cmd = c.execCommand(ctx, "curl", "-s", "-H", "Metadata-Flavor: Google",
		"http://metadata.google.internal/computeMetadata/v1/instance/zone")
	zone, err := cmd.Output()
	if err != nil {
		c.logger.Printf("Error: failed to get zone from metadata: %v — VM will not be deleted", err)
		return
	}

	// Delete instance (blocks until completion or timeout)
	name := strings.TrimSpace(string(instanceName))
	zoneName := filepath.Base(strings.TrimSpace(string(zone)))
	c.logger.Printf("Deleting VM instance %s in zone %s", name, zoneName)

	cmd = c.execCommand(ctx, "gcloud", "compute", "instances", "delete",
		name,
		"--zone", zoneName,
		"--quiet",
	)

	if err := cmd.Run(); err != nil {
		c.logger.Printf("Error: VM deletion command failed: %v — VM may remain running until max_run_duration", err)
		return
	}

	c.logger.Println("VM deletion command completed successfully")
}
