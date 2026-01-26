package controller

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/andywolf/agentium/internal/agent"
)

// runAgentContainerInteractive executes a Docker container in interactive mode with
// stdin/stdout/stderr attached to the terminal. This is used for --local mode
// where the user wants to watch and interact with agent execution.
//
// Unlike runAgentContainer, this function:
// - Uses docker run -it for interactive terminal access
// - Attaches stdin/stdout/stderr directly to the process
// - Returns a basic result based on exit code (structured output cannot be parsed)
func (c *Controller) runAgentContainerInteractive(ctx context.Context, params containerRunParams) (*agent.IterationResult, error) {
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

	// Build Docker arguments for interactive mode
	args := []string{
		"run", "--rm",
		"-it", // Interactive with TTY
		"-v", fmt.Sprintf("%s:/workspace", c.workDir),
		"-w", "/workspace",
	}

	for k, v := range params.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	// Mount Claude OAuth credentials if configured
	if c.config.ClaudeAuth.AuthMode == "oauth" {
		// In local mode, mount from user's home directory
		home, err := os.UserHomeDir()
		if err == nil {
			claudeAuthPath := home + "/.config/claude-code/auth.json"
			if _, statErr := os.Stat(claudeAuthPath); statErr == nil {
				args = append(args, "-v", claudeAuthPath+":/home/agentium/.claude/.credentials.json:ro")
			}
		}
	}

	// Mount Codex OAuth credentials if configured
	if c.config.CodexAuth.AuthJSONBase64 != "" {
		home, err := os.UserHomeDir()
		if err == nil {
			codexAuthPath := home + "/.codex/auth.json"
			if _, statErr := os.Stat(codexAuthPath); statErr == nil {
				args = append(args, "-v", codexAuthPath+":/home/agentium/.codex/auth.json:ro")
			}
		}
	}

	args = append(args, params.Agent.ContainerImage())
	args = append(args, params.Command...)

	cmd := exec.CommandContext(ctx, "docker", args...)

	// Attach stdin/stdout/stderr to the terminal for interactive use
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	c.logger.Printf("Running interactive agent: docker %s", strings.Join(args, " "))

	// Run the container and wait for completion
	err := cmd.Run()

	// Build a basic result based on exit code
	result := &agent.IterationResult{
		ExitCode: 0,
		Success:  true,
		Summary:  "Interactive session completed",
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			result.Success = false
			result.Error = fmt.Sprintf("agent exited with code %d", result.ExitCode)
			result.Summary = fmt.Sprintf("Interactive session failed (exit code %d)", result.ExitCode)
		} else {
			return nil, fmt.Errorf("%s failed to run: %w", params.LogTag, err)
		}
	}

	return result, nil
}
