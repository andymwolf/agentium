package controller

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/andywolf/agentium/internal/agent"
	"github.com/andywolf/agentium/internal/agent/claudecode"
	"github.com/andywolf/agentium/internal/cloud/gcp"
	"github.com/andywolf/agentium/internal/memory"
)

// containerRunParams holds the parameters for running an agent container.
type containerRunParams struct {
	Agent   agent.Agent
	Session *agent.Session
	Env     map[string]string
	Command []string
	LogTag  string // Prefix for log messages (e.g. "Agent", "Delegated agent")
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

	for k, v := range params.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	if c.config.ClaudeAuth.AuthMode == "oauth" {
		args = append(args, "-v", "/etc/agentium/claude-auth.json:/home/agentium/.claude/.credentials.json:ro")
	}

	args = append(args, params.Agent.ContainerImage())
	args = append(args, params.Command...)

	cmd := exec.CommandContext(ctx, "docker", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("%s stdout pipe: %w", params.LogTag, err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("%s stderr pipe: %w", params.LogTag, err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("%s start: %w", params.LogTag, err)
	}

	stdoutBytes, _ := io.ReadAll(stdout)
	stderrBytes, _ := io.ReadAll(stderr)

	err = cmd.Wait()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
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
		c.logger.Printf("%s exited with code %d", params.LogTag, exitCode)
		if stderrStr != "" {
			c.logger.Printf("%s stderr: %s", params.LogTag, stderrStr)
		}
		if stdoutStr != "" {
			c.logger.Printf("%s stdout: %s", params.LogTag, stdoutStr)
		}
	}

	// Parse output
	result, parseErr := params.Agent.ParseOutput(exitCode, string(stdoutBytes), string(stderrBytes))
	if parseErr != nil {
		return nil, fmt.Errorf("%s parse output: %w", params.LogTag, parseErr)
	}

	// Log structured events at DEBUG level
	if c.cloudLogger != nil && len(result.Events) > 0 {
		c.logAgentEvents(result.Events)
	}

	// Process memory signals - prefer parsed text content when available
	signalSource := string(stdoutBytes) + string(stderrBytes)
	if result.RawTextContent != "" {
		signalSource = result.RawTextContent + "\n" + string(stderrBytes)
	}
	if c.memoryStore != nil {
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
