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
	"strings"
	"time"

	"github.com/andywolf/agentium/internal/agent"
	_ "github.com/andywolf/agentium/internal/agent/aider"
	_ "github.com/andywolf/agentium/internal/agent/claudecode"
)

// SessionConfig is the configuration passed to the controller
type SessionConfig struct {
	ID            string   `json:"id"`
	Repository    string   `json:"repository"`
	Tasks         []string `json:"tasks"`
	Agent         string   `json:"agent"`
	MaxIterations int      `json:"max_iterations"`
	MaxDuration   string   `json:"max_duration"`
	Prompt        string   `json:"prompt"`
	GitHub        struct {
		AppID            int64  `json:"app_id"`
		InstallationID   int64  `json:"installation_id"`
		PrivateKeySecret string `json:"private_key_secret"`
	} `json:"github"`
}

// Controller manages the agent execution lifecycle
type Controller struct {
	config       SessionConfig
	agent        agent.Agent
	workDir      string
	iteration    int
	startTime    time.Time
	maxDuration  time.Duration
	gitHubToken  string
	completed    map[string]bool
	logger       *log.Logger
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

	return &Controller{
		config:      config,
		agent:       agentAdapter,
		workDir:     "/workspace",
		iteration:   0,
		maxDuration: maxDuration,
		completed:   make(map[string]bool),
		logger:      log.New(os.Stdout, "[controller] ", log.LstdFlags),
	}, nil
}

// Run executes the main control loop
func (c *Controller) Run(ctx context.Context) error {
	c.startTime = time.Now()
	c.logger.Printf("Starting session %s", c.config.ID)
	c.logger.Printf("Repository: %s", c.config.Repository)
	c.logger.Printf("Tasks: %v", c.config.Tasks)
	c.logger.Printf("Agent: %s", c.config.Agent)
	c.logger.Printf("Max iterations: %d", c.config.MaxIterations)
	c.logger.Printf("Max duration: %s", c.maxDuration)

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

	// Fetch issue details
	issues, err := c.fetchIssueDetails(ctx)
	if err != nil {
		c.logger.Printf("Warning: failed to fetch issue details: %v", err)
	}

	// Build initial prompt with issue context
	if len(issues) > 0 {
		c.config.Prompt = c.buildPromptWithIssues(issues)
	}

	// Main execution loop
	for {
		select {
		case <-ctx.Done():
			c.logger.Println("Context cancelled, shutting down")
			return ctx.Err()
		default:
		}

		// Check termination conditions
		if c.shouldTerminate() {
			c.logger.Println("Termination condition met")
			break
		}

		c.iteration++
		c.logger.Printf("Starting iteration %d/%d", c.iteration, c.config.MaxIterations)

		// Run agent iteration
		result, err := c.runIteration(ctx)
		if err != nil {
			c.logger.Printf("Iteration %d failed: %v", c.iteration, err)
			continue
		}

		c.logger.Printf("Iteration %d completed: %s", c.iteration, result.Summary)

		// Update completion state
		for _, task := range result.TasksCompleted {
			c.completed[task] = true
		}

		// Check PRs created
		if len(result.PRsCreated) > 0 {
			c.logger.Printf("PRs created: %v", result.PRsCreated)
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
	// Use gcloud to fetch secret
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
	// Write private key to temp file
	keyFile, err := os.CreateTemp("", "github-app-key-*.pem")
	if err != nil {
		return "", err
	}
	defer os.Remove(keyFile.Name())

	if _, err := keyFile.WriteString(privateKey); err != nil {
		return "", err
	}
	keyFile.Close()

	// Use gh CLI to generate token (requires GitHub CLI with app authentication)
	// For MVP, we'll use a simpler approach with curl and JWT
	// In production, this should use proper JWT generation

	// For now, assume GITHUB_TOKEN is set or use a placeholder
	c.logger.Println("Warning: Using GITHUB_TOKEN from environment, proper App token generation not implemented")
	return os.Getenv("GITHUB_TOKEN"), nil
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

	// Check if all tasks are complete
	allComplete := true
	for _, task := range c.config.Tasks {
		if !c.completed[task] {
			allComplete = false
			break
		}
	}
	if allComplete && len(c.config.Tasks) > 0 {
		c.logger.Println("All tasks completed")
		return true
	}

	return false
}

func (c *Controller) runIteration(ctx context.Context) (*agent.IterationResult, error) {
	session := &agent.Session{
		ID:            c.config.ID,
		Repository:    c.config.Repository,
		Tasks:         c.config.Tasks,
		WorkDir:       c.workDir,
		GitHubToken:   c.gitHubToken,
		MaxIterations: c.config.MaxIterations,
		MaxDuration:   c.config.MaxDuration,
		Prompt:        c.config.Prompt,
		Metadata:      make(map[string]string),
	}

	// Build environment and command
	env := c.agent.BuildEnv(session, c.iteration)
	command := c.agent.BuildCommand(session, c.iteration)

	c.logger.Printf("Running agent: %s %v", c.agent.ContainerImage(), command)

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

	// Parse output
	result, parseErr := c.agent.ParseOutput(exitCode, string(stdoutBytes), string(stderrBytes))
	if parseErr != nil {
		return nil, parseErr
	}

	return result, nil
}

func (c *Controller) emitFinalLogs() {
	c.logger.Println("=== Session Summary ===")
	c.logger.Printf("Session ID: %s", c.config.ID)
	c.logger.Printf("Duration: %s", time.Since(c.startTime).Round(time.Second))
	c.logger.Printf("Iterations: %d/%d", c.iteration, c.config.MaxIterations)

	completedCount := 0
	for _, task := range c.config.Tasks {
		if c.completed[task] {
			completedCount++
		}
	}
	c.logger.Printf("Tasks completed: %d/%d", completedCount, len(c.config.Tasks))
	c.logger.Println("======================")
}

func (c *Controller) cleanup() {
	c.logger.Println("Cleaning up")

	// Clear sensitive data
	c.gitHubToken = ""

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
