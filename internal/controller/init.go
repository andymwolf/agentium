package controller

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/andywolf/agentium/internal/agent/event"
	"github.com/andywolf/agentium/internal/github"
	"github.com/andywolf/agentium/internal/handoff"
	"github.com/andywolf/agentium/internal/memory"
	"github.com/andywolf/agentium/internal/prompt"
)

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
	secretName := parseSecretName(secretPath)
	cmd := c.execCommand(ctx, "gcloud", "secrets", "versions", "access", "latest",
		"--secret", secretName,
	)

	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return string(output), nil
}

// parseSecretName extracts the secret name from a GCP Secret Manager path.
// Supported formats:
//   - "projects/PROJECT/secrets/SECRET_NAME/versions/VERSION" → "SECRET_NAME"
//   - "projects/PROJECT/secrets/SECRET_NAME" → "SECRET_NAME"
//   - "SECRET_NAME" → "SECRET_NAME" (plain name, returned as-is)
func parseSecretName(secretPath string) string {
	parts := strings.Split(secretPath, "/")
	// Full path: projects/P/secrets/NAME[/versions/V] — secret name is at index 3
	if len(parts) >= 4 && parts[0] == "projects" && parts[2] == "secrets" {
		return parts[3]
	}
	// Plain secret name (no slashes or unrecognized format)
	return secretPath
}

func (c *Controller) loadPrompts() {
	c.logInfo("Phase prompts loaded (static per-phase-role files)")

	// Load project prompt from workspace (.agentium/AGENTS.md) - optional
	projectPrompt, err := prompt.LoadProjectPrompt(c.workDir)
	if err != nil {
		c.logWarning("failed to load project prompt: %v", err)
	} else if projectPrompt != "" {
		c.projectPrompt = projectPrompt
		c.logInfo("Project prompt loaded from .agentium/AGENTS.md")
	}

	// Always initialize memory store — required for iterate feedback delivery,
	// phase result recording, and context building across all phases.
	c.memoryStore = memory.NewStore(c.workDir, memory.Config{
		MaxEntries:    c.config.Memory.MaxEntries,
		ContextBudget: c.config.Memory.ContextBudget,
	})
	if loadErr := c.memoryStore.Load(); loadErr != nil {
		c.logWarning("failed to load memory store: %v", loadErr)
	} else {
		c.logInfo("Memory store initialized (%d entries)", len(c.memoryStore.Entries()))
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
