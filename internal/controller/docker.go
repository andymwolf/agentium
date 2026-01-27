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
			c.logger.Printf("Warning: docker login to ghcr.io failed: %v (%s)", err, string(out))
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

	// Mount Claude OAuth credentials
	if c.config.ClaudeAuth.AuthMode == "oauth" {
		if c.config.Interactive {
			// In local mode, write credentials to temp file and mount from there
			authPath, err := c.writeInteractiveAuthFile("claude-auth.json", c.config.ClaudeAuth.AuthJSONBase64)
			if err != nil {
				c.logWarning("Failed to write Claude auth file: %v", err)
			} else if authPath != "" {
				args = append(args, "-v", authPath+":/home/agentium/.claude/.credentials.json:ro")
				c.logInfo("Mounting Claude OAuth credentials from %s", authPath)
			}
		} else {
			// In cloud mode, mount from VM path set up by provisioner
			args = append(args, "-v", "/etc/agentium/claude-auth.json:/home/agentium/.claude/.credentials.json:ro")
			c.logInfo("Mounting Claude OAuth credentials from /etc/agentium/claude-auth.json")
		}
	} else if c.config.ClaudeAuth.AuthMode != "" {
		c.logInfo("Claude auth mode is %q, not mounting OAuth credentials", c.config.ClaudeAuth.AuthMode)
	}

	// Mount Codex OAuth credentials
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
			taskID := fmt.Sprintf("%s:%s", c.activeTaskType, c.activeTask)
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
		c.logger.Printf("%s warning: reading stdout: %v", logTag, stdoutErr)
	}
	if stderrErr != nil {
		c.logger.Printf("%s warning: reading stderr: %v", logTag, stderrErr)
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
		c.logger.Printf("%s exited with code %d", logTag, exitCode)
		if stderrStr != "" {
			c.logger.Printf("%s stderr: %s", logTag, stderrStr)
		}
		if stdoutStr != "" {
			c.logger.Printf("%s stdout: %s", logTag, stdoutStr)
		}
	}

	return stdoutBytes, stderrBytes, exitCode, nil
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
