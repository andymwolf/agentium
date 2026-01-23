package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/andywolf/agentium/internal/agent"
	_ "github.com/andywolf/agentium/internal/agent/aider"
	_ "github.com/andywolf/agentium/internal/agent/claudecode"
	"github.com/andywolf/agentium/internal/cloud/gcp"
	"github.com/andywolf/agentium/internal/github"
	"github.com/andywolf/agentium/internal/prompt"
)

// TaskPhase represents the current phase of a task in its lifecycle
type TaskPhase string

const (
	PhaseImplement   TaskPhase = "IMPLEMENT"
	PhaseTest        TaskPhase = "TEST"
	PhasePRCreation  TaskPhase = "PR_CREATION"
	PhaseReview      TaskPhase = "REVIEW"
	PhaseComplete    TaskPhase = "COMPLETE"
	PhaseBlocked     TaskPhase = "BLOCKED"
	PhaseNothingToDo TaskPhase = "NOTHING_TO_DO"
	// PR-specific phases
	PhaseAnalyze TaskPhase = "ANALYZE"
	PhasePush    TaskPhase = "PUSH"
)

// TaskState tracks the current state of a task being worked on
type TaskState struct {
	ID           string
	Type         string // "issue" or "pr"
	Phase        TaskPhase
	TestRetries  int
	LastStatus   string
	PRNumber     string // Linked PR number (for issues that create PRs)
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
	Prompts struct {
		SystemMDURL  string `json:"system_md_url,omitempty"`
		FetchTimeout string `json:"fetch_timeout,omitempty"` // Duration string (e.g. "5s", "10s")
	} `json:"prompts,omitempty"`
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

// Controller manages the agent execution lifecycle
type Controller struct {
	config        SessionConfig
	agent         agent.Agent
	workDir       string
	iteration     int
	startTime     time.Time
	maxDuration   time.Duration
	gitHubToken   string
	completed     map[string]bool
	pushedChanges bool // Tracks if changes were pushed (for PR review sessions)
	dockerAuthed  bool // Tracks if docker login to GHCR was done
	taskStates    map[string]*TaskState
	logger        *log.Logger
	cloudLogger   *gcp.CloudLogger // Structured cloud logging (may be nil if unavailable)
	secretManager gcp.SecretFetcher
	systemPrompt  string // Loaded SYSTEM.md content
	projectPrompt string // Loaded .agentium/AGENT.md content (may be empty)
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

	c := &Controller{
		config:        config,
		agent:         agentAdapter,
		workDir:       workDir,
		iteration:     0,
		maxDuration:   maxDuration,
		completed:     make(map[string]bool),
		taskStates:    make(map[string]*TaskState),
		logger:        log.New(os.Stdout, "[controller] ", log.LstdFlags),
		cloudLogger:   cloudLogger,
		secretManager: secretManager,
	}

	// Initialize task states for all tasks and PRs
	for _, task := range config.Tasks {
		c.taskStates[fmt.Sprintf("issue:%s", task)] = &TaskState{
			ID:    task,
			Type:  "issue",
			Phase: PhaseImplement,
		}
	}
	for _, pr := range config.PRs {
		c.taskStates[fmt.Sprintf("pr:%s", pr)] = &TaskState{
			ID:    pr,
			Type:  "pr",
			Phase: PhaseAnalyze,
		}
	}

	return c, nil
}

// logInfo logs at INFO level to both local logger and cloud logger
func (c *Controller) logInfo(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	c.logger.Println(msg)
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

// Run executes the main control loop
func (c *Controller) Run(ctx context.Context) error {
	c.startTime = time.Now()
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

	// Handle PR mode vs issue mode
	if len(c.config.PRs) > 0 {
		// PR review session: fetch PR details and checkout PR branch
		prDetails, err := c.fetchPRDetails(ctx)
		if err != nil {
			c.logger.Printf("Warning: failed to fetch PR details: %v", err)
		}

		// Build PR-specific prompt
		if len(prDetails) > 0 {
			c.config.Prompt = c.buildPromptWithPRs(prDetails)
		}
	} else {
		// Issue session: fetch issue details
		issues, err := c.fetchIssueDetails(ctx)
		if err != nil {
			c.logger.Printf("Warning: failed to fetch issue details: %v", err)
		}

		// Build initial prompt with issue context
		if len(issues) > 0 {
			c.config.Prompt = c.buildPromptWithIssues(issues)
		}
	}

	// Main execution loop
	for {
		select {
		case <-ctx.Done():
			c.logInfo("Context cancelled, shutting down")
			return ctx.Err()
		default:
		}

		// Check termination conditions
		if c.shouldTerminate() {
			c.logInfo("Termination condition met")
			break
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

		// Update task phases based on agent status
		// For simplicity, update all active tasks (in practice, we'd track which task is being worked on)
		for taskID := range c.taskStates {
			c.updateTaskPhase(taskID, result)
		}

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
	c.logger.Println("Initializing workspace")

	if err := os.MkdirAll(c.workDir, 0755); err != nil {
		return err
	}

	return nil
}

func (c *Controller) fetchGitHubToken(ctx context.Context) error {
	c.logger.Println("Fetching GitHub token")

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
	// Parse configured fetch timeout
	var fetchTimeout time.Duration
	if c.config.Prompts.FetchTimeout != "" {
		parsed, err := time.ParseDuration(c.config.Prompts.FetchTimeout)
		if err != nil {
			c.logger.Printf("Warning: invalid fetch_timeout %q, using default", c.config.Prompts.FetchTimeout)
		} else {
			fetchTimeout = parsed
		}
	}

	// Load system prompt (hybrid fetch + embedded fallback)
	systemPrompt, err := prompt.LoadSystemPrompt(c.config.Prompts.SystemMDURL, fetchTimeout)
	if err != nil {
		return fmt.Errorf("failed to load system prompt: %w", err)
	}
	c.systemPrompt = systemPrompt
	c.logger.Println("System prompt loaded successfully")

	// Load project prompt from workspace (.agentium/AGENT.md)
	projectPrompt, err := prompt.LoadProjectPrompt(c.workDir)
	if err != nil {
		c.logger.Printf("Warning: failed to load project prompt: %v", err)
	} else if projectPrompt != "" {
		c.projectPrompt = projectPrompt
		c.logger.Println("Project prompt loaded from .agentium/AGENT.md")
	}

	return nil
}

func (c *Controller) cloneRepository(ctx context.Context) error {
	c.logger.Printf("Cloning repository: %s", c.config.Repository)

	// Parse repository URL
	repo := c.config.Repository
	if !strings.HasPrefix(repo, "https://") && !strings.HasPrefix(repo, "git@") {
		repo = "https://" + repo
	}

	// Clone with token authentication
	cloneURL := repo
	if c.gitHubToken != "" && strings.HasPrefix(repo, "https://") {
		// Insert token for authentication
		cloneURL = strings.Replace(repo, "https://", fmt.Sprintf("https://x-access-token:%s@", c.gitHubToken), 1)
	}

	cmd := exec.CommandContext(ctx, "git", "clone", cloneURL, c.workDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		// Check if directory already exists with content
		if entries, _ := os.ReadDir(c.workDir); len(entries) > 0 {
			c.logger.Println("Workspace already contains files, skipping clone")
			return nil
		}
		return err
	}

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

func (c *Controller) fetchIssueDetails(ctx context.Context) ([]issueDetail, error) {
	c.logger.Println("Fetching issue details")

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

	return issues, nil
}

type prWithReviews struct {
	Detail   prDetail
	Reviews  []prReview
	Comments []prComment
}

func (c *Controller) fetchPRDetails(ctx context.Context) ([]prWithReviews, error) {
	c.logger.Println("Fetching PR details")

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

		// Checkout PR branch
		c.logger.Printf("Checking out PR branch: %s", pr.HeadRefName)
		checkoutCmd := exec.CommandContext(ctx, "git", "checkout", pr.HeadRefName)
		checkoutCmd.Dir = c.workDir
		if err := checkoutCmd.Run(); err != nil {
			// Try fetching and checking out
			fetchCmd := exec.CommandContext(ctx, "git", "fetch", "origin", pr.HeadRefName)
			fetchCmd.Dir = c.workDir
			fetchCmd.Run()
			checkoutCmd = exec.CommandContext(ctx, "git", "checkout", pr.HeadRefName)
			checkoutCmd.Dir = c.workDir
			if err := checkoutCmd.Run(); err != nil {
				c.logger.Printf("Warning: failed to checkout PR branch %s: %v", pr.HeadRefName, err)
			}
		}

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

	return prs, nil
}

func (c *Controller) buildPromptWithPRs(prs []prWithReviews) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("You are working on repository: %s\n\n", c.config.Repository))
	sb.WriteString("## PR REVIEW SESSION\n\n")
	sb.WriteString("You are addressing code review feedback on existing pull request(s).\n\n")

	for _, pr := range prs {
		sb.WriteString(fmt.Sprintf("### PR #%d: %s\n", pr.Detail.Number, pr.Detail.Title))
		sb.WriteString(fmt.Sprintf("Branch: %s\n\n", pr.Detail.HeadRefName))

		// Add review comments
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

		// Add inline comments
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

func (c *Controller) buildPromptWithIssues(issues []issueDetail) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("You are working on repository: %s\n\n", c.config.Repository))
	sb.WriteString("Complete these GitHub issues:\n\n")

	for _, issue := range issues {
		sb.WriteString(fmt.Sprintf("Issue #%d: %s\n", issue.Number, issue.Title))
		if issue.Body != "" {
			// Truncate long bodies
			body := issue.Body
			if len(body) > 1000 {
				body = body[:1000] + "..."
			}
			sb.WriteString(fmt.Sprintf("Description: %s\n", body))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("For each issue:\n")
	sb.WriteString("1. Create branch: agentium/issue-<number>-<short-description>\n")
	sb.WriteString("2. Implement the fix\n")
	sb.WriteString("3. Run tests\n")
	sb.WriteString("4. Create a PR linking to the issue\n")

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
		state.Phase = PhaseTest
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

	state.LastStatus = result.AgentStatus
	c.logger.Printf("Task %s phase: %s (status: %s)", taskID, state.Phase, result.AgentStatus)
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

func (c *Controller) runIteration(ctx context.Context) (*agent.IterationResult, error) {
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
	}

	// Build environment and command
	env := c.agent.BuildEnv(session, c.iteration)
	command := c.agent.BuildCommand(session, c.iteration)

	c.logger.Printf("Running agent: %s %v", c.agent.ContainerImage(), command)

	// Authenticate with GHCR if needed (once per session)
	if !c.dockerAuthed && strings.Contains(c.agent.ContainerImage(), "ghcr.io") && c.gitHubToken != "" {
		loginCmd := exec.CommandContext(ctx, "docker", "login", "ghcr.io",
			"-u", "x-access-token", "--password-stdin")
		loginCmd.Stdin = strings.NewReader(c.gitHubToken)
		if out, err := loginCmd.CombinedOutput(); err != nil {
			c.logger.Printf("Warning: docker login to ghcr.io failed: %v (%s)", err, string(out))
		} else {
			c.dockerAuthed = true
		}
	}

	// Run agent container
	args := []string{
		"run", "--rm",
		"-v", fmt.Sprintf("%s:/workspace", c.workDir),
		"-w", "/workspace",
	}

	// Add environment variables
	for k, v := range env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	// Mount Claude credentials for OAuth mode
	if c.config.ClaudeAuth.AuthMode == "oauth" {
		args = append(args, "-v", "/etc/agentium/claude-auth.json:/home/agentium/.claude/.credentials.json:ro")
	}

	// Add image and command
	args = append(args, c.agent.ContainerImage())
	args = append(args, command...)

	cmd := exec.CommandContext(ctx, "docker", args...)

	// Capture output
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	// Read output
	stdoutBytes, _ := io.ReadAll(stdout)
	stderrBytes, _ := io.ReadAll(stderr)

	err = cmd.Wait()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}

	// Log agent output for debugging
	if exitCode != 0 {
		stderrStr := string(stderrBytes)
		stdoutStr := string(stdoutBytes)
		if len(stderrStr) > 500 {
			stderrStr = stderrStr[:500]
		}
		if len(stdoutStr) > 500 {
			stdoutStr = stdoutStr[:500]
		}
		c.logger.Printf("Agent exited with code %d", exitCode)
		if stderrStr != "" {
			c.logger.Printf("Agent stderr: %s", stderrStr)
		}
		if stdoutStr != "" {
			c.logger.Printf("Agent stdout: %s", stdoutStr)
		}
	}

	// Parse output
	result, parseErr := c.agent.ParseOutput(exitCode, string(stdoutBytes), string(stderrBytes))
	if parseErr != nil {
		return nil, parseErr
	}

	return result, nil
}

func (c *Controller) emitFinalLogs() {
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
	c.logInfo("Cleaning up")

	// Flush cloud logs before cleanup to ensure they survive VM termination
	if c.cloudLogger != nil {
		if err := c.cloudLogger.Flush(); err != nil {
			c.logger.Printf("Warning: failed to flush cloud logs: %v", err)
		}
	}

	// Clear sensitive data
	c.gitHubToken = ""

	// Close Cloud Logger
	if c.cloudLogger != nil {
		if err := c.cloudLogger.Close(); err != nil {
			c.logger.Printf("Warning: failed to close cloud logger: %v", err)
		}
	}

	// Close Secret Manager client
	if c.secretManager != nil {
		if err := c.secretManager.Close(); err != nil {
			c.logger.Printf("Warning: failed to close Secret Manager client: %v", err)
		}
	}

	// Request VM termination
	c.terminateVM()
}

func (c *Controller) terminateVM() {
	c.logger.Println("Initiating VM termination")

	// Get instance name from metadata
	cmd := exec.Command("curl", "-s", "-H", "Metadata-Flavor: Google",
		"http://metadata.google.internal/computeMetadata/v1/instance/name")
	instanceName, err := cmd.Output()
	if err != nil {
		c.logger.Printf("Warning: failed to get instance name: %v", err)
		return
	}

	// Get zone from metadata
	cmd = exec.Command("curl", "-s", "-H", "Metadata-Flavor: Google",
		"http://metadata.google.internal/computeMetadata/v1/instance/zone")
	zone, err := cmd.Output()
	if err != nil {
		c.logger.Printf("Warning: failed to get zone: %v", err)
		return
	}

	// Delete instance
	zoneName := filepath.Base(string(zone))
	cmd = exec.Command("gcloud", "compute", "instances", "delete",
		string(instanceName),
		"--zone", zoneName,
		"--quiet",
	)

	if err := cmd.Start(); err != nil {
		c.logger.Printf("Warning: failed to initiate VM deletion: %v", err)
	}
}
