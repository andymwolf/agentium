package controller

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/andywolf/agentium/internal/agent"
	"github.com/andywolf/agentium/internal/agent/claudecode"
	"github.com/andywolf/agentium/internal/cloud/gcp"
	"github.com/andywolf/agentium/internal/memory"
)

// cleanupAuthPath removes any directory at an auth file path.
// Docker creates directories at mount points when source files don't exist.
func (c *Controller) cleanupAuthPath(path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		if c.logger != nil {
			c.logWarning("Removing stale directory at auth path: %s", path)
		}
		return os.Remove(path)
	}
	return nil
}

// validateAuthFile checks that an auth file exists and is not a directory.
// In cloud mode, Docker creates a directory at the mount point if the file doesn't exist,
// causing EISDIR errors when the agent tries to read it.
func (c *Controller) validateAuthFile(path, name string) error {
	// Clean up stale directories from previous Docker mount attempts
	if err := c.cleanupAuthPath(path); err != nil {
		return fmt.Errorf("%s auth path cleanup failed: %w", name, err)
	}

	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return fmt.Errorf("%s auth file not found at %s (cloud-init may have failed)", name, path)
	}
	if err != nil {
		return fmt.Errorf("%s auth file error: %w", name, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s auth path is a directory - Docker mount failed: %s", name, path)
	}
	return nil
}

// containerRunParams holds the parameters for running an agent container.
type containerRunParams struct {
	Agent       agent.Agent
	Session     *agent.Session
	Env         map[string]string
	Command     []string
	LogTag      string // Prefix for log messages (e.g. "Agent", "Delegated agent")
	StdinPrompt string // Prompt to pipe via stdin (if non-empty)
}

// runAgentContainer executes a Docker container for the given agent and returns the parsed result.
// It handles GHCR authentication, Docker argument construction, process execution,
// output parsing, and memory signal processing.
func (c *Controller) runAgentContainer(ctx context.Context, params containerRunParams) (*agent.IterationResult, error) {
	// Authenticate with GHCR if needed (once per session)
	if !c.dockerAuthed && strings.Contains(params.Agent.ContainerImage(), "ghcr.io") && c.gitHubToken != "" {
		loginCmd := exec.CommandContext(ctx, "docker", "login", "ghcr.io",
			"-u", "x-access-token", "--password-stdin")
		loginCmd.Stdin = strings.NewReader(c.gitHubToken)
		if out, err := loginCmd.CombinedOutput(); err != nil {
			c.logWarning("docker login to ghcr.io failed: %v (%s)", err, string(out))
		} else {
			c.dockerAuthed = true
		}
	}

	// Validate auth files exist in cloud mode before mounting
	// This prevents Docker from creating directories at mount points when files are missing
	// Only validate auth for the specific adapter being run to avoid false failures
	if !c.config.Interactive {
		switch params.Agent.Name() {
		case "claude-code":
			if c.config.ClaudeAuth.AuthMode == "oauth" {
				if err := c.validateAuthFile("/etc/agentium/claude-auth.json", "Claude"); err != nil {
					return nil, err
				}
			}
		case "codex":
			if c.config.CodexAuth.AuthJSONBase64 != "" {
				if err := c.validateAuthFile("/etc/agentium/codex-auth.json", "Codex"); err != nil {
					return nil, err
				}
			}
		}
	}

	// Build Docker arguments
	args := []string{
		"run", "--rm",
		"-v", fmt.Sprintf("%s:/workspace", c.workDir),
		"-w", "/workspace",
	}

	// Add -i flag when piping stdin (keeps stdin open for prompt delivery)
	if params.StdinPrompt != "" {
		args = append(args, "-i")
	}

	for k, v := range params.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	// Mount OAuth credentials only for the adapter being used
	switch params.Agent.Name() {
	case "claude-code":
		if c.config.ClaudeAuth.AuthMode == "oauth" {
			if c.config.Interactive {
				// In local mode, write credentials to temp file and mount from there
				authPath, err := c.writeInteractiveAuthFile("claude-auth.json", c.config.ClaudeAuth.AuthJSONBase64)
				if err != nil {
					c.logWarning("Failed to write Claude auth file: %v", err)
				} else if authPath != "" {
					args = append(args, "-v", authPath+":/home/agentium/.claude/.credentials.json:ro")
				}
			} else {
				// In cloud mode, mount from VM path set up by provisioner
				args = append(args, "-v", "/etc/agentium/claude-auth.json:/home/agentium/.claude/.credentials.json:ro")
			}
		} else if c.config.ClaudeAuth.AuthMode != "" {
			c.logInfo("Claude auth mode is %q, not mounting OAuth credentials", c.config.ClaudeAuth.AuthMode)
		}
	case "codex":
		if c.config.CodexAuth.AuthJSONBase64 != "" {
			if c.config.Interactive {
				// In local mode, write credentials to temp file and mount from there
				authPath, err := c.writeInteractiveAuthFile("codex-auth.json", c.config.CodexAuth.AuthJSONBase64)
				if err != nil {
					c.logWarning("Failed to write Codex auth file: %v", err)
				} else if authPath != "" {
					args = append(args, "-v", authPath+":/home/agentium/.codex/auth.json:ro")
				}
			} else {
				// In cloud mode, mount from VM path set up by provisioner
				args = append(args, "-v", "/etc/agentium/codex-auth.json:/home/agentium/.codex/auth.json:ro")
			}
		}
	}

	args = append(args, params.Agent.ContainerImage())
	args = append(args, params.Command...)

	cmd := exec.CommandContext(ctx, "docker", args...)

	// Pipe prompt via stdin if provided (for non-interactive mode)
	if params.StdinPrompt != "" {
		cmd.Stdin = strings.NewReader(params.StdinPrompt)
	}

	stdoutBytes, stderrBytes, exitCode, err := c.executeAndCollect(cmd, params.LogTag)
	if err != nil {
		return nil, err
	}

	// Parse output
	result, parseErr := params.Agent.ParseOutput(exitCode, string(stdoutBytes), string(stderrBytes))
	if parseErr != nil {
		return nil, fmt.Errorf("%s parse output: %w", params.LogTag, parseErr)
	}

	// Log token consumption to GCP Cloud Logging
	c.logTokenConsumption(result, params.Agent.Name(), params.Session)

	// Log structured events at DEBUG level
	if c.cloudLogger != nil && len(result.Events) > 0 {
		c.logAgentEvents(result.Events)
		// Emit security audit events at INFO level
		c.emitAuditEvents(result.Events, params.Agent.Name())
	}

	// Process memory signals using the adapter's parsed text content
	if c.memoryStore != nil {
		signalSource := result.RawTextContent + "\n" + string(stderrBytes)
		signals := memory.ParseSignals(signalSource)
		if len(signals) > 0 {
			taskID := taskKey(c.activeTaskType, c.activeTask)
			pruned := c.memoryStore.Update(signals, c.iteration, taskID)
			if pruned > 0 {
				c.logWarning("Memory store pruned %d oldest entries (max_entries=%d)", pruned, c.config.Memory.MaxEntries)
			}
			if err := c.memoryStore.Save(); err != nil {
				c.logWarning("failed to save memory store: %v", err)
			} else {
				c.logInfo("Memory updated: %d new signals, %d total entries", len(signals), len(c.memoryStore.Entries()))
			}
		}
	}

	return result, nil
}

// executeAndCollect starts the command, reads stdout and stderr concurrently,
// waits for the process to exit, and returns the collected output along with the exit code.
// Reading both streams concurrently prevents deadlocks that occur when one pipe's
// OS buffer fills while the other is being read sequentially.
func (c *Controller) executeAndCollect(cmd *exec.Cmd, logTag string) (stdoutBytes, stderrBytes []byte, exitCode int, err error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, 0, fmt.Errorf("%s stdout pipe: %w", logTag, err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, 0, fmt.Errorf("%s stderr pipe: %w", logTag, err)
	}

	if err := cmd.Start(); err != nil {
		return nil, nil, 0, fmt.Errorf("%s start: %w", logTag, err)
	}

	c.logInfo("%s: prompt delivered, awaiting response", logTag)

	// Read stdout and stderr concurrently to avoid deadlock.
	// If either pipe's OS buffer fills while the other is being read sequentially,
	// the process will block, causing a hang.
	var stdoutErr, stderrErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		stdoutBytes, stdoutErr = io.ReadAll(stdout)
	}()
	go func() {
		defer wg.Done()
		stderrBytes, stderrErr = io.ReadAll(stderr)
	}()
	wg.Wait()

	if stdoutErr != nil {
		c.logWarning("%s: reading stdout: %v", logTag, stdoutErr)
	}
	if stderrErr != nil {
		c.logWarning("%s: reading stderr: %v", logTag, stderrErr)
	}

	waitErr := cmd.Wait()
	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}

	if exitCode != 0 {
		stderrStr := string(stderrBytes)
		stdoutStr := string(stdoutBytes)
		if len(stderrStr) > 500 {
			stderrStr = stderrStr[:500]
		}
		if len(stdoutStr) > 500 {
			stdoutStr = stdoutStr[:500]
		}
		c.logWarning("%s exited with code %d", logTag, exitCode)
		if stderrStr != "" {
			c.logWarning("%s stderr: %s", logTag, stderrStr)
		}
		if stdoutStr != "" {
			c.logWarning("%s stdout: %s", logTag, stdoutStr)
		}
	}

	return stdoutBytes, stderrBytes, exitCode, nil
}

// prePullAgentImages pulls all agent container images that will be used in this session.
// This is called during initSession() to avoid pull latency on the first iteration.
// Failures are logged as warnings but not returned since pre-pulling is non-fatal.
func (c *Controller) prePullAgentImages(ctx context.Context) {
	// Collect unique images from all configured adapters
	images := make(map[string]bool)
	for _, adapter := range c.adapters {
		images[adapter.ContainerImage()] = true
	}

	if len(images) == 0 {
		return
	}

	c.logInfo("Pre-pulling %d agent container image(s)...", len(images))

	// Authenticate with GHCR if any images require it
	for image := range images {
		if strings.Contains(image, "ghcr.io") && c.gitHubToken != "" && !c.dockerAuthed {
			loginCmd := exec.CommandContext(ctx, "docker", "login", "ghcr.io",
				"-u", "x-access-token", "--password-stdin")
			loginCmd.Stdin = strings.NewReader(c.gitHubToken)
			if out, err := loginCmd.CombinedOutput(); err != nil {
				c.logWarning("docker login to ghcr.io failed: %v (%s)", err, string(out))
			} else {
				c.dockerAuthed = true
			}
			break // Only need to auth once
		}
	}

	// Pull each image
	for image := range images {
		c.logInfo("Pulling image: %s", image)
		pullCmd := exec.CommandContext(ctx, "docker", "pull", image)
		if out, err := pullCmd.CombinedOutput(); err != nil {
			c.logWarning("Failed to pre-pull image %s: %v (%s)", image, err, string(out))
			// Non-fatal: docker run will retry on first iteration
		} else {
			c.logInfo("Successfully pulled: %s", image)
		}
	}
}

// logAgentEvents logs structured agent events at DEBUG level to Cloud Logging.
func (c *Controller) logAgentEvents(events []interface{}) {
	for _, evt := range events {
		se, ok := evt.(claudecode.StreamEvent)
		if !ok {
			continue
		}
		labels := map[string]string{"event_type": string(se.Subtype)}
		if se.ToolName != "" {
			labels["tool_name"] = se.ToolName
		}
		if se.Subtype == claudecode.BlockThinking {
			labels["content_type"] = "thinking"
		}
		msg := se.Content
		if len(msg) > 2000 {
			msg = msg[:2000] + "...(truncated)"
		}
		c.cloudLogger.LogWithLabels(gcp.SeverityDebug, msg, labels)
	}
}
