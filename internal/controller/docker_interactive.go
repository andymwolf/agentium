package controller

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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

	// Pass clone-inside-container environment variables
	if c.config.CloneInsideContainer {
		args = append(args, "-e", "AGENTIUM_CLONE_INSIDE=true")
		args = append(args, "-e", fmt.Sprintf("AGENTIUM_REPOSITORY=%s", c.config.Repository))
		// Explicitly pass GITHUB_TOKEN if available (for automated auth inside container)
		if c.gitHubToken != "" {
			args = append(args, "-e", fmt.Sprintf("GITHUB_TOKEN=%s", c.gitHubToken))
		}
	}

	// Mount Claude OAuth credentials if configured
	if c.config.ClaudeAuth.AuthMode == "oauth" {
		authPath, err := c.writeInteractiveAuthFile("claude-auth.json", c.config.ClaudeAuth.AuthJSONBase64)
		if err != nil {
			c.logger.Printf("Warning: failed to write Claude auth file: %v", err)
		} else if authPath != "" {
			args = append(args, "-v", authPath+":/home/agentium/.claude/.credentials.json:ro")
		}
	}

	// Mount Codex OAuth credentials if configured
	if c.config.CodexAuth.AuthJSONBase64 != "" {
		authPath, err := c.writeInteractiveAuthFile("codex-auth.json", c.config.CodexAuth.AuthJSONBase64)
		if err != nil {
			c.logger.Printf("Warning: failed to write Codex auth file: %v", err)
		} else if authPath != "" {
			args = append(args, "-v", authPath+":/home/agentium/.codex/auth.json:ro")
		}
	}

	args = append(args, params.Agent.ContainerImage())
	args = append(args, params.Command...)

	cmd := exec.CommandContext(ctx, "docker", args...)

	// Create buffers to capture output while still displaying to terminal
	// This allows us to parse signals (AGENTIUM_STATUS, AGENTIUM_HANDOFF) for phase loop
	var stdoutBuf, stderrBuf bytes.Buffer

	cmd.Stdin = os.Stdin
	cmd.Stdout = io.MultiWriter(os.Stdout, &stdoutBuf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)

	if c.config.Verbose {
		c.logger.Printf("Running interactive agent: docker %s", strings.Join(args, " "))
	}

	// Run the container and wait for completion
	err := cmd.Run()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("%s failed to run: %w", params.LogTag, err)
		}
	}

	// Parse the captured output using the adapter (same as non-interactive)
	// This extracts AGENTIUM_STATUS, AGENTIUM_HANDOFF, and other signals for phase loop
	result, parseErr := params.Agent.ParseOutput(exitCode, stdoutBuf.String(), stderrBuf.String())
	if parseErr != nil {
		c.logWarning("Failed to parse interactive output: %v", parseErr)
		// Fall back to basic result
		result = &agent.IterationResult{
			ExitCode: exitCode,
			Success:  exitCode == 0,
			Summary:  "Interactive session completed",
		}
	}

	return result, nil
}

// writeInteractiveAuthFile writes base64-encoded auth credentials to a temp file
// in the workspace for mounting into containers. Returns the path to the written file,
// or empty string if no credentials were provided.
func (c *Controller) writeInteractiveAuthFile(filename, base64Data string) (string, error) {
	if base64Data == "" {
		return "", nil
	}

	// Decode base64 credentials
	authData, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return "", fmt.Errorf("failed to decode auth data: %w", err)
	}

	// Create auth directory in workspace
	authDir := filepath.Join(c.workDir, ".agentium-auth")
	if err := os.MkdirAll(authDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create auth directory: %w", err)
	}

	// Write auth file
	authPath := filepath.Join(authDir, filename)
	if err := os.WriteFile(authPath, authData, 0600); err != nil {
		return "", fmt.Errorf("failed to write auth file: %w", err)
	}

	return authPath, nil
}
