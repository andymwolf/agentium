package controller

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"

	"github.com/andywolf/agentium/internal/agent"
	"github.com/andywolf/agentium/internal/agent/claudecode"
	"github.com/andywolf/agentium/internal/agent/codex"
	"github.com/andywolf/agentium/internal/agent/event"
	"github.com/andywolf/agentium/internal/cloud/gcp"
	"github.com/andywolf/agentium/internal/memory"
)

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

	// Mount OAuth credentials only for the adapter being used.
	// We always use workspace-based auth (write credentials to workspace temp file)
	// for both local and cloud modes. This eliminates cloud-init timing issues and
	// Docker directory creation problems that occurred with /etc/agentium mounts.
	switch params.Agent.Name() {
	case "claude-code":
		if c.config.ClaudeAuth.AuthMode == "oauth" {
			authPath, err := c.writeInteractiveAuthFile("claude-auth.json", c.config.ClaudeAuth.AuthJSONBase64)
			if err != nil {
				c.logWarning("Failed to write Claude auth file: %v", err)
			} else if authPath != "" {
				args = append(args, "-v", authPath+":/home/agentium/.claude/.credentials.json:ro")
			}
		} else if c.config.ClaudeAuth.AuthMode != "" {
			c.logInfo("Claude auth mode is %q, not mounting OAuth credentials", c.config.ClaudeAuth.AuthMode)
		}
	case "codex":
		if c.config.CodexAuth.AuthJSONBase64 != "" {
			authPath, err := c.writeInteractiveAuthFile("codex-auth.json", c.config.CodexAuth.AuthJSONBase64)
			if err != nil {
				c.logWarning("Failed to write Codex auth file: %v", err)
			} else if authPath != "" {
				args = append(args, "-v", authPath+":/home/agentium/.codex/auth.json:ro")
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
	if len(result.Events) > 0 {
		c.logAgentEvents(result.Events)
		// Emit security audit events at INFO level
		if c.cloudLogger != nil {
			c.emitAuditEvents(result.Events, params.Agent.Name())
		}
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

// logAgentEvents logs structured agent events at DEBUG level to Cloud Logging
// and writes them to the file sink if configured.
func (c *Controller) logAgentEvents(events []interface{}) {
	var unifiedEvents []*event.AgentEvent

	for _, evt := range events {
		var agentEvent *event.AgentEvent

		// Convert adapter-specific events to unified AgentEvent
		switch e := evt.(type) {
		case claudecode.StreamEvent:
			agentEvent = event.FromClaudeCode(e, c.config.ID, c.iteration)
		case codex.CodexEvent:
			agentEvent = event.FromCodex(e, c.config.ID, c.iteration)
		default:
			// Skip unknown event types
			continue
		}

		unifiedEvents = append(unifiedEvents, agentEvent)

		// Log to Cloud Logging if available
		if c.cloudLogger != nil {
			// Restrict labels to a safe set to avoid exceeding label limits
			// and high-cardinality issues. Only include short, bounded values.
			labels := map[string]string{
				"event_type": string(agentEvent.Type),
				"iteration":  fmt.Sprintf("%d", c.iteration),
			}
			// Only copy safe metadata keys with bounded values to avoid
			// exceeding Cloud Logging label limits (63 byte max).
			safeKeys := []string{"tool_name", "action"}
			for _, k := range safeKeys {
				if v, ok := agentEvent.Metadata[k]; ok {
					if len(v) > 63 {
						v = v[:63]
					}
					labels[k] = v
				}
			}
			// Log Summary (not Content) to Cloud Logging to avoid leaking
			// sensitive data from tool results, command output, etc.
			// Full Content is only written to the local FileSink.
			msg := agentEvent.Summary
			if msg == "" {
				msg = string(agentEvent.Type)
			}
			c.cloudLogger.LogWithLabels(gcp.SeverityDebug, msg, labels)
		}
	}

	// Write unified events to file sink if configured
	if c.eventSink != nil && len(unifiedEvents) > 0 {
		if err := c.eventSink.WriteBatch(unifiedEvents); err != nil {
			c.logWarning("failed to write events to sink: %v", err)
		} else {
			if err := c.eventSink.Flush(); err != nil {
				c.logWarning("failed to flush event sink: %v", err)
			}
		}
	}
}
