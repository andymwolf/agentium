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
)

// Version is the controller version, set at build time via ldflags.
var Version = "dev"

// TaskPhase represents the current phase of a task in its lifecycle
type TaskPhase string

const (
	PhasePlan        TaskPhase = "PLAN"
	PhaseImplement   TaskPhase = "IMPLEMENT"
	PhaseReview      TaskPhase = "REVIEW"
	PhaseDocs        TaskPhase = "DOCS"
	PhasePRCreation  TaskPhase = "PR_CREATION"
	PhaseComplete    TaskPhase = "COMPLETE"
	PhaseBlocked     TaskPhase = "BLOCKED"
	PhaseNothingToDo TaskPhase = "NOTHING_TO_DO"
	// PR-specific phases
	PhaseAnalyze TaskPhase = "ANALYZE"
	PhasePush    TaskPhase = "PUSH"
)

// TaskState tracks the current state of a task being worked on
type TaskState struct {
	ID                string
	Type              string // "issue" or "pr"
	Phase             TaskPhase
	TestRetries       int
	LastStatus        string
	PRNumber          string // Linked PR number (for issues that create PRs)
	PhaseIteration    int    // Current iteration within the active phase (phase loop)
	MaxPhaseIter      int    // Max iterations for current phase (phase loop)
	LastJudgeVerdict  string // Last judge verdict (ADVANCE, ITERATE, BLOCKED, SIMPLE, COMPLEX, REGRESS)
	LastJudgeFeedback string // Last judge feedback text
	ReviewDecided     bool   // Whether complexity decision has been made
	ReviewActive      bool   // Whether review loop is active for this task (auto mode)
	IsSimple          bool   // Whether task was marked as SIMPLE (skip REVIEW phase)
	RegressionCount   int    // Number of times we've regressed from REVIEW to PLAN
}

// PhaseLoopConfig controls the controller-as-judge phase loop behavior.
type PhaseLoopConfig struct {
	Enabled                bool `json:"enabled"`
	PlanMaxIterations      int  `json:"plan_max_iterations,omitempty"`
	ImplementMaxIterations int  `json:"implement_max_iterations,omitempty"`
	ReviewMaxIterations    int  `json:"review_max_iterations,omitempty"`
	DocsMaxIterations      int  `json:"docs_max_iterations,omitempty"`
	JudgeContextBudget     int  `json:"judge_context_budget,omitempty"`
	JudgeNoSignalLimit     int  `json:"judge_no_signal_limit,omitempty"`
}

// SessionConfig is the configuration passed to the controller
type SessionConfig struct {
	ID            string   `json:"id"`
	Repository    string   `json:"repository"`
	Tasks         []string `json:"tasks"`
	PRs           []string `json:"prs,omitempty"`
	Agent         string   `json:"agent"`
	MaxIterations int      `json:"max_iterations"`
	MaxDuration   string   `json:"max_duration"`
	Prompt        string   `json:"prompt"`
	GitHub        struct {
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
	Routing    *routing.PhaseRouting `json:"routing,omitempty"`
	Delegation *DelegationConfig     `json:"delegation,omitempty"`
	PhaseLoop  *PhaseLoopConfig      `json:"phase_loop,omitempty"`
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
	completed              map[string]bool
	pushedChanges          bool // Tracks if changes were pushed (for PR review sessions)
	dockerAuthed           bool // Tracks if docker login to GHCR was done
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
	modelRouter            *routing.Router        // Phase-to-model routing (nil = no routing)
	adapters               map[string]agent.Agent // All initialized adapters (for multi-adapter routing)
	orchestrator           *SubTaskOrchestrator   // Sub-task delegation orchestrator (nil = disabled)
	metadataUpdater        gcp.MetadataUpdater    // Instance metadata updater (nil if unavailable)

	// Shutdown management
	shutdownHooks []ShutdownHook
	shutdownOnce  sync.Once
	shutdownCh    chan struct{} // Closed when shutdown is initiated
	logFlushFn    func() error  // Function to flush pending logs

	// cmdRunner executes external commands. Defaults to exec.CommandContext.
	// Override in tests to mock command execution.
	cmdRunner func(ctx context.Context, name string, args ...string) *exec.Cmd
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

	// Initialize Secret Manager client
	secretManager, err := gcp.NewSecretManagerClient(context.Background())
	if err != nil {
		// Log warning but don't fail - allow fallback to gcloud CLI
		log.Printf("[controller] Warning: failed to initialize Secret Manager client: %v", err)
	}

	workDir := os.Getenv("AGENTIUM_WORKDIR")
	if workDir == "" {
		workDir = "/workspace"
	}

	// Initialize Cloud Logging (non-fatal if unavailable)
	var cloudLogger *gcp.CloudLogger
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
	var metadataUpdater gcp.MetadataUpdater
	if gcp.IsRunningOnGCP() {
		metadataUpdaterInstance, err := gcp.NewComputeMetadataUpdater(context.Background())
		if err != nil {
			log.Printf("[controller] Warning: metadata updater unavailable, session status will not be reported: %v", err)
		} else {
			metadataUpdater = metadataUpdaterInstance
		}
	}

	c := &Controller{
		config:          config,
		agent:           agentAdapter,
		workDir:         workDir,
		iteration:       0,
		maxDuration:     maxDuration,
		completed:       make(map[string]bool),
		taskStates:      make(map[string]*TaskState),
		logger:          log.New(os.Stdout, "[controller] ", log.LstdFlags),
		cloudLogger:     cloudLogger,
		secretManager:   secretManager,
		metadataUpdater: metadataUpdater,
		shutdownCh:      make(chan struct{}),
	}

	// Initialize task states and build unified task queue (PRs first, then issues)
	for _, pr := range config.PRs {
		c.taskStates[fmt.Sprintf("pr:%s", pr)] = &TaskState{
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
		c.taskStates[fmt.Sprintf("issue:%s", task)] = &TaskState{
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

	// Clone repository
	if err := c.cloneRepository(ctx); err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
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

		// Build prompt based on task type
		if nextTask.Type == "pr" {
			c.logInfo("Focusing on PR #%s", nextTask.ID)
			if err := c.preparePRTask(ctx, nextTask.ID); err != nil {
				c.logError("Failed to prepare PR #%s: %v", nextTask.ID, err)
				// Mark as blocked and continue to next task
				taskID := fmt.Sprintf("pr:%s", nextTask.ID)
				if state, ok := c.taskStates[taskID]; ok {
					state.Phase = PhaseBlocked
				}
				continue
			}
		} else {
			c.logInfo("Focusing on issue #%s", nextTask.ID)
			existingWork := c.detectExistingWork(ctx, nextTask.ID)
			c.config.Prompt = c.buildPromptForTask(nextTask.ID, existingWork)
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
		taskID := fmt.Sprintf("%s:%s", c.activeTaskType, c.activeTask)
		c.updateTaskPhase(taskID, result)

		// Update instance metadata with current session status
		c.updateInstanceMetadata(ctx)

		// Update completion state (legacy)
		for _, task := range result.TasksCompleted {
			c.completed[task] = true
		}

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

	// Try to get token from environment first (for local testing)
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		c.gitHubToken = token
		return nil
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

	// Generate installation access token
	token, err := c.generateInstallationToken(privateKey)
	if err != nil {
		return fmt.Errorf("failed to generate installation token: %w", err)
	}

	c.gitHubToken = token
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
	cmd := exec.CommandContext(ctx, "gcloud", "secrets", "versions", "access", "latest",
		"--secret", filepath.Base(secretPath),
	)

	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return string(output), nil
}

func (c *Controller) generateInstallationToken(privateKey string) (string, error) {
	appID := strconv.FormatInt(c.config.GitHub.AppID, 10)
	installationID := c.config.GitHub.InstallationID

	// Generate JWT for GitHub App authentication
	jwtGen, err := github.NewJWTGenerator(appID, []byte(privateKey))
	if err != nil {
		return "", fmt.Errorf("failed to create JWT generator: %w", err)
	}

	jwt, err := jwtGen.GenerateToken()
	if err != nil {
		return "", fmt.Errorf("failed to generate JWT: %w", err)
	}

	// Exchange JWT for installation access token
	exchanger := github.NewTokenExchanger()
	token, err := exchanger.ExchangeToken(jwt, installationID)
	if err != nil {
		return "", fmt.Errorf("failed to exchange token: %w", err)
	}

	c.logger.Printf("Generated installation token (expires at %s)", token.ExpiresAt.Format(time.RFC3339))
	return token.Token, nil
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

	return nil
}

func (c *Controller) cloneRepository(ctx context.Context) error {
	c.logInfo("Cloning repository: %s", c.config.Repository)

	// Parse repository URL
	repo := c.config.Repository
	if !strings.HasPrefix(repo, "https://") && !strings.HasPrefix(repo, "git@") {
		repo = "https://" + repo
	}

	// Clone with token authentication
	// SECURITY: Avoid embedding tokens in URLs as they can leak in error messages and logs.
	// Instead, use git credential helper via http.extraHeader config option.
	var cmd *exec.Cmd
	if c.gitHubToken != "" && strings.HasPrefix(repo, "https://") {
		// Use http.extraHeader to pass token without embedding in URL
		authHeader := fmt.Sprintf("Authorization: Bearer %s", c.gitHubToken)
		cmd = exec.CommandContext(ctx, "git",
			"-c", fmt.Sprintf("http.extraHeader=%s", authHeader),
			"clone", repo, c.workDir)
	} else {
		cmd = exec.CommandContext(ctx, "git", "clone", repo, c.workDir)
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
	cmd := exec.CommandContext(ctx, "git", "config", "--global", "--add", "safe.directory", c.workDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		c.logWarning("failed to configure git safe.directory: %v (%s)", err, string(output))
		return err
	}
	c.logInfo("Configured git safe.directory for %s", c.workDir)
	return nil
}

type issueDetail struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
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
		cmd := exec.CommandContext(ctx, "gh", "issue", "view", taskID,
			"--repo", c.config.Repository,
			"--json", "number,title,body",
		)
		cmd.Env = append(os.Environ(), fmt.Sprintf("GITHUB_TOKEN=%s", c.gitHubToken))

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
		cmd := exec.CommandContext(ctx, "gh", "pr", "view", prNumber,
			"--repo", c.config.Repository,
			"--json", "number,title,body,headRefName",
		)
		cmd.Env = append(os.Environ(), fmt.Sprintf("GITHUB_TOKEN=%s", c.gitHubToken))

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
		reviewCmd := exec.CommandContext(ctx, "gh", "api",
			fmt.Sprintf("repos/%s/pulls/%s/reviews", repoPath, prNumber),
			"--jq", `[.[] | select(.state == "CHANGES_REQUESTED" or .state == "COMMENTED") | {state: .state, body: .body}]`,
		)
		reviewCmd.Env = append(os.Environ(), fmt.Sprintf("GITHUB_TOKEN=%s", c.gitHubToken))

		if reviewOutput, err := reviewCmd.Output(); err == nil {
			var reviews []prReview
			if err := json.Unmarshal(reviewOutput, &reviews); err == nil {
				prWithRev.Reviews = reviews
			}
		}

		// Fetch inline review comments
		commentCmd := exec.CommandContext(ctx, "gh", "api",
			fmt.Sprintf("repos/%s/pulls/%s/comments", repoPath, prNumber),
			"--jq", `[.[] | {path: .path, line: .line, body: .body, diffHunk: .diff_hunk}]`,
		)
		commentCmd.Env = append(os.Environ(), fmt.Sprintf("GITHUB_TOKEN=%s", c.gitHubToken))

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
		taskID := fmt.Sprintf("%s:%s", item.Type, item.ID)
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
	checkoutCmd := exec.CommandContext(ctx, "git", "checkout", prData.Detail.HeadRefName)
	checkoutCmd.Dir = c.workDir
	if err := checkoutCmd.Run(); err != nil {
		// Try fetching first (ignore error - subsequent checkout will fail if needed)
		fetchCmd := exec.CommandContext(ctx, "git", "fetch", "origin", prData.Detail.HeadRefName)
		fetchCmd.Dir = c.workDir
		_ = fetchCmd.Run()
		checkoutCmd = exec.CommandContext(ctx, "git", "checkout", prData.Detail.HeadRefName)
		checkoutCmd.Dir = c.workDir
		if err := checkoutCmd.Run(); err != nil {
			return fmt.Errorf("failed to checkout PR branch %s: %w", prData.Detail.HeadRefName, err)
		}
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
				if len(body) > 500 {
					body = body[:500] + "..."
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
			if len(body) > 300 {
				body = body[:300] + "..."
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
func (c *Controller) detectExistingWork(ctx context.Context, issueNumber string) *agent.ExistingWork {
	// Check for existing open PRs with branch matching agentium/issue-<N>
	// Use --search to find matching PRs regardless of age (avoids missing older PRs beyond default limit)
	branchPrefix := fmt.Sprintf("agentium/issue-%s-", issueNumber)
	cmd := exec.CommandContext(ctx, "gh", "pr", "list",
		"--repo", c.config.Repository,
		"--state", "open",
		"--search", fmt.Sprintf("head:%s", branchPrefix),
		"--json", "number,title,headRefName",
	)
	cmd.Dir = c.workDir
	cmd.Env = append(os.Environ(), fmt.Sprintf("GITHUB_TOKEN=%s", c.gitHubToken))

	if output, err := cmd.Output(); err == nil {
		var prs []struct {
			Number      int    `json:"number"`
			Title       string `json:"title"`
			HeadRefName string `json:"headRefName"`
		}
		if unmarshalErr := json.Unmarshal(output, &prs); unmarshalErr == nil {
			// The search should already filter for matching branches, but double-check to be safe
			for _, pr := range prs {
				if strings.HasPrefix(pr.HeadRefName, branchPrefix) {
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

	// No PR found; check for existing remote branches
	cmd = exec.CommandContext(ctx, "git", "branch", "-r", "--list",
		fmt.Sprintf("origin/agentium/issue-%s-*", issueNumber),
	)
	cmd.Dir = c.workDir

	if output, err := cmd.Output(); err == nil {
		branches := strings.TrimSpace(string(output))
		if branches != "" {
			// Take the first matching branch
			lines := strings.Split(branches, "\n")
			branch := strings.TrimSpace(lines[0])
			branch = strings.TrimPrefix(branch, "origin/")
			c.logInfo("Found existing branch for issue #%s: %s", issueNumber, branch)
			return &agent.ExistingWork{
				Branch: branch,
			}
		}
	} else {
		c.logWarning("failed to list remote branches for existing work detection on issue #%s: %v", issueNumber, err)
	}

	c.logInfo("No existing work found for issue #%s", issueNumber)
	return nil
}

// buildPromptForTask builds a focused prompt for a single issue, incorporating existing work context.
func (c *Controller) buildPromptForTask(issueNumber string, existingWork *agent.ExistingWork) string {
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
			if len(body) > 1000 {
				body = body[:1000] + "..."
			}
			sb.WriteString(fmt.Sprintf("**Description:**\n%s\n\n", body))
		}
	}

	if existingWork != nil {
		// Existing work found — instruct continuation
		sb.WriteString("## Existing Work Detected\n\n")
		if existingWork.PRNumber != "" {
			sb.WriteString(fmt.Sprintf("An open PR already exists for this issue: **PR #%s** (%s)\n",
				existingWork.PRNumber, existingWork.PRTitle))
			sb.WriteString(fmt.Sprintf("Branch: `%s`\n\n", existingWork.Branch))
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
			sb.WriteString(fmt.Sprintf("An existing branch was found for this issue: `%s`\n\n", existingWork.Branch))
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
		sb.WriteString("### Instructions\n\n")
		sb.WriteString(fmt.Sprintf("1. Create a new branch: `agentium/issue-%s-<short-description>`\n", issueNumber))
		sb.WriteString("2. Implement the fix or feature\n")
		sb.WriteString("3. Run tests to verify correctness\n")
		sb.WriteString("4. Commit your changes with a descriptive message\n")
		sb.WriteString("5. Push the branch\n")
		sb.WriteString("6. Create a pull request linking to the issue\n\n")
	}

	sb.WriteString("Use 'gh' CLI for GitHub operations and 'git' for version control.\n")
	sb.WriteString(fmt.Sprintf("The repository is already cloned at %s.\n", c.workDir))

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

	// Legacy fallback: issue sessions check if all tasks are complete
	allComplete := true
	for _, task := range c.config.Tasks {
		if !c.completed[task] {
			allComplete = false
			break
		}
	}
	if allComplete && len(c.config.Tasks) > 0 {
		c.logger.Println("All tasks completed (legacy detection)")
		return true
	}

	return false
}

// isPhaseLoopEnabled returns true if the phase loop is configured and enabled.
func (c *Controller) isPhaseLoopEnabled() bool {
	return c.config.PhaseLoop != nil && c.config.PhaseLoop.Enabled
}

// determineActivePhase returns the current phase for the active task.
// When no task state exists yet (first iteration), defaults are:
// - "ANALYZE" for PR tasks (agent starts by reading review comments)
// - "IMPLEMENT" for issue tasks (agent starts by writing code)
func (c *Controller) determineActivePhase() string {
	taskID := fmt.Sprintf("%s:%s", c.activeTaskType, c.activeTask)
	if state, ok := c.taskStates[taskID]; ok {
		return string(state.Phase)
	}
	if c.activeTaskType == "pr" {
		return "ANALYZE"
	}
	return "IMPLEMENT"
}

func (c *Controller) runIteration(ctx context.Context) (*agent.IterationResult, error) {
	// Check delegation before standard iteration
	if c.orchestrator != nil {
		phase := TaskPhase(c.determineActivePhase())
		if subCfg := c.orchestrator.ConfigForPhase(phase); subCfg != nil {
			c.logInfo("Phase %s: delegating to sub-agent config (agent=%s)", phase, subCfg.Agent)
			return c.runDelegatedIteration(ctx, phase, subCfg)
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
		Prompt:         c.config.Prompt,
		Metadata:       make(map[string]string),
		ClaudeAuthMode: c.config.ClaudeAuth.AuthMode,
		SystemPrompt:   c.systemPrompt,
		ProjectPrompt:  c.projectPrompt,
		ActiveTask:     c.activeTask,
		ExistingWork:   c.activeTaskExistingWork,
	}

	// Compose phase-aware skills if selector is available
	if c.skillSelector != nil {
		phase := c.determineActivePhase()
		session.IterationContext = &agent.IterationContext{
			Phase:        phase,
			SkillsPrompt: c.skillSelector.SelectForPhase(phase),
		}
		c.logInfo("Using skills for phase %s: %v", phase, c.skillSelector.SkillsForPhase(phase))
	}

	// Inject memory context if store is available
	if c.memoryStore != nil {
		// Build context scoped to the current task
		taskID := fmt.Sprintf("%s:%s", c.activeTaskType, c.activeTask)
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
		modelCfg := c.modelRouter.ModelForPhase(phase)
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

	c.logger.Printf("Running agent: %s %v", activeAgent.ContainerImage(), command)

	return c.runAgentContainer(ctx, containerRunParams{
		Agent:   activeAgent,
		Session: session,
		Env:     env,
		Command: command,
		LogTag:  "Agent",
	})
}

func (c *Controller) emitFinalLogs() {
	// Final metadata update so the provisioner sees the terminal state
	c.updateInstanceMetadata(context.Background())

	c.logInfo("=== Session Summary ===")
	c.logInfo("Session ID: %s", c.config.ID)
	c.logInfo("Duration: %s", time.Since(c.startTime).Round(time.Second))
	c.logInfo("Iterations: %d/%d", c.iteration, c.config.MaxIterations)

	completedCount := 0
	for _, task := range c.config.Tasks {
		if c.completed[task] {
			completedCount++
		}
	}
	c.logInfo("Tasks completed: %d/%d", completedCount, len(c.config.Tasks))

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
