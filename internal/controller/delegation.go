package controller

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/andywolf/agentium/internal/agent"
	"github.com/andywolf/agentium/internal/memory"
)

// runDelegatedIteration executes a single iteration using the delegated sub-task config.
// It resolves the agent adapter, builds skills and session context, runs the Docker
// container, parses output, and updates shared memory.
func (c *Controller) runDelegatedIteration(ctx context.Context, phase TaskPhase, config *SubTaskConfig) (*agent.IterationResult, error) {
	// Resolve agent adapter
	activeAgent := c.agent
	if config.Agent != "" {
		if a, ok := c.adapters[config.Agent]; ok {
			activeAgent = a
		} else {
			c.logWarning("Delegation phase %s: configured adapter %q not found, using default %q", phase, config.Agent, c.agent.Name())
		}
	}

	// Build skills prompt
	var skillsPrompt string
	if c.skillSelector != nil {
		if len(config.Skills) > 0 {
			skillsPrompt = c.skillSelector.SelectByNames(config.Skills)
		} else {
			skillsPrompt = c.skillSelector.SelectForPhase(string(phase))
		}
	}

	// Build model override
	var modelOverride string
	if config.Model != nil && config.Model.Model != "" {
		modelOverride = config.Model.Model
	}

	// Build session
	subTaskID := fmt.Sprintf("delegation-%s-%d", phase, c.iteration)
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
		IterationContext: &agent.IterationContext{
			Phase:         string(phase),
			SkillsPrompt:  skillsPrompt,
			Iteration:     c.iteration,
			SubTaskID:     subTaskID,
			ModelOverride: modelOverride,
		},
	}

	// Inject memory context if store is available
	if c.memoryStore != nil {
		memCtx := c.memoryStore.BuildContext()
		if memCtx != "" {
			session.IterationContext.MemoryContext = memCtx
		}
	}

	c.logInfo("Delegating phase %s: adapter=%s subtask=%s", phase, activeAgent.Name(), subTaskID)

	// Build environment and command
	env := activeAgent.BuildEnv(session, c.iteration)
	command := activeAgent.BuildCommand(session, c.iteration)

	// Authenticate with GHCR if needed
	if !c.dockerAuthed && strings.Contains(activeAgent.ContainerImage(), "ghcr.io") && c.gitHubToken != "" {
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

	for k, v := range env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	if c.config.ClaudeAuth.AuthMode == "oauth" {
		args = append(args, "-v", "/etc/agentium/claude-auth.json:/home/agentium/.claude/.credentials.json:ro")
	}

	args = append(args, activeAgent.ContainerImage())
	args = append(args, command...)

	cmd := exec.CommandContext(ctx, "docker", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("delegation stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("delegation stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("delegation start: %w", err)
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
		c.logger.Printf("Delegated agent exited with code %d", exitCode)
		if stderrStr != "" {
			c.logger.Printf("Delegated agent stderr: %s", stderrStr)
		}
		if stdoutStr != "" {
			c.logger.Printf("Delegated agent stdout: %s", stdoutStr)
		}
	}

	// Parse output
	result, parseErr := activeAgent.ParseOutput(exitCode, string(stdoutBytes), string(stderrBytes))
	if parseErr != nil {
		return nil, fmt.Errorf("delegation parse output: %w", parseErr)
	}

	// Update shared memory with signals from output
	if c.memoryStore != nil {
		signals := memory.ParseSignals(string(stdoutBytes) + string(stderrBytes))
		if len(signals) > 0 {
			taskID := fmt.Sprintf("%s:%s", c.activeTaskType, c.activeTask)
			pruned := c.memoryStore.Update(signals, c.iteration, taskID)
			if pruned > 0 {
				c.logWarning("Memory store pruned %d oldest entries (max_entries=%d)", pruned, c.config.Memory.MaxEntries)
			}
			if err := c.memoryStore.Save(); err != nil {
				c.logWarning("failed to save memory store: %v", err)
			} else {
				c.logInfo("Delegation memory updated: %d new signals, %d total entries", len(signals), len(c.memoryStore.Entries()))
			}
		}
	}

	return result, nil
}
