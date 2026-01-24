package controller

import (
	"context"
	"fmt"
	"io"
	"log"
	"os/exec"
	"testing"
	"time"

	"github.com/andywolf/agentium/internal/agent"
)

// runAgentContainerWithCommand is a test helper that executes an arbitrary command
// (instead of a Docker container) and runs the same IO handling and parsing logic.
// This allows testing the concurrent IO handling without Docker.
func runAgentContainerWithCommand(ctx context.Context, c *Controller, params containerRunParams, name string, args ...string) (*agent.IterationResult, error) {
	cmd := exec.CommandContext(ctx, name, args...)

	stdoutBytes, stderrBytes, exitCode, err := c.executeAndCollect(cmd, params.LogTag)
	if err != nil {
		return nil, err
	}

	result, parseErr := params.Agent.ParseOutput(exitCode, string(stdoutBytes), string(stderrBytes))
	if parseErr != nil {
		return nil, fmt.Errorf("%s parse output: %w", params.LogTag, parseErr)
	}

	return result, nil
}

// dockerTestAgent implements agent.Agent for testing runAgentContainer IO handling.
type dockerTestAgent struct{}

func (m *dockerTestAgent) Name() string                                       { return "docker-test" }
func (m *dockerTestAgent) ContainerImage() string                             { return "test-image:latest" }
func (m *dockerTestAgent) BuildEnv(_ *agent.Session, _ int) map[string]string { return nil }
func (m *dockerTestAgent) BuildCommand(_ *agent.Session, _ int) []string      { return nil }
func (m *dockerTestAgent) BuildPrompt(_ *agent.Session, _ int) string         { return "" }
func (m *dockerTestAgent) ParseOutput(exitCode int, stdout, stderr string) (*agent.IterationResult, error) {
	return &agent.IterationResult{
		ExitCode: exitCode,
		Success:  exitCode == 0,
		Summary:  stdout,
	}, nil
}
func (m *dockerTestAgent) Validate() error { return nil }

// TestRunAgentContainer_LargeOutput verifies that runAgentContainer does not deadlock
// when the subprocess produces large output on both stdout and stderr simultaneously.
// This test catches the bug where sequential io.ReadAll calls on stdout then stderr
// could cause a deadlock if either pipe's OS buffer fills up.
func TestRunAgentContainer_LargeOutput(t *testing.T) {
	// Create a controller with minimal configuration for testing
	c := &Controller{
		config:  SessionConfig{},
		workDir: "/tmp",
		logger:  log.New(io.Discard, "", 0),
	}

	// Use a timeout to detect deadlocks - if the function hangs, the test will fail
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Run a command that produces large output on both stdout and stderr simultaneously.
	// The output size (128KB per stream) exceeds the typical OS pipe buffer (64KB on Linux),
	// which would cause a deadlock with sequential reads.
	params := containerRunParams{
		Agent: &dockerTestAgent{},
		Env:   map[string]string{},
		// Use bash to write >64KB to both stdout and stderr concurrently.
		// dd writes 128KB (2048 blocks * 64 bytes each) to each stream.
		Command: []string{},
		LogTag:  "Test",
	}

	// Override the command to use a direct process (not docker) for testing.
	// First dd: 2>/dev/null suppresses dd's progress stats on stderr; data goes to stdout.
	// Second dd: >&2 redirects dd's data output to stderr; 2>/dev/null (evaluated first by
	// the shell) suppresses dd's progress stats.
	result, err := runAgentContainerWithCommand(ctx, c, params,
		"bash", "-c",
		"dd if=/dev/zero bs=64 count=2048 2>/dev/null && dd if=/dev/zero bs=64 count=2048 >&2 2>/dev/null",
	)

	if err != nil {
		t.Fatalf("runAgentContainerWithCommand failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
}

// TestRunAgentContainer_LargeStdoutOnly verifies that large stdout output alone
// does not cause issues.
func TestRunAgentContainer_LargeStdoutOnly(t *testing.T) {
	c := &Controller{
		config:  SessionConfig{},
		workDir: "/tmp",
		logger:  log.New(io.Discard, "", 0),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	params := containerRunParams{
		Agent:   &dockerTestAgent{},
		Env:     map[string]string{},
		Command: []string{},
		LogTag:  "Test",
	}

	result, err := runAgentContainerWithCommand(ctx, c, params,
		"bash", "-c",
		"dd if=/dev/zero bs=64 count=4096 2>/dev/null",
	)

	if err != nil {
		t.Fatalf("runAgentContainerWithCommand failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
}

// TestRunAgentContainer_LargeStderrOnly verifies that large stderr output alone
// does not cause issues.
func TestRunAgentContainer_LargeStderrOnly(t *testing.T) {
	c := &Controller{
		config:  SessionConfig{},
		workDir: "/tmp",
		logger:  log.New(io.Discard, "", 0),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	params := containerRunParams{
		Agent:   &dockerTestAgent{},
		Env:     map[string]string{},
		Command: []string{},
		LogTag:  "Test",
	}

	// >&2 redirects dd's data output (normally stdout) to stderr, while 2>/dev/null
	// (evaluated first by the shell) suppresses dd's progress stats.
	result, err := runAgentContainerWithCommand(ctx, c, params,
		"bash", "-c",
		"dd if=/dev/zero bs=64 count=4096 >&2 2>/dev/null",
	)

	if err != nil {
		t.Fatalf("runAgentContainerWithCommand failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
}

// TestRunAgentContainer_NonZeroExit verifies that non-zero exit codes are captured correctly.
func TestRunAgentContainer_NonZeroExit(t *testing.T) {
	c := &Controller{
		config:  SessionConfig{},
		workDir: "/tmp",
		logger:  log.New(io.Discard, "", 0),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	params := containerRunParams{
		Agent:   &dockerTestAgent{},
		Env:     map[string]string{},
		Command: []string{},
		LogTag:  "Test",
	}

	result, err := runAgentContainerWithCommand(ctx, c, params,
		"bash", "-c", "echo 'error output' >&2; exit 1",
	)

	if err != nil {
		t.Fatalf("runAgentContainerWithCommand failed: %v", err)
	}

	if result.ExitCode != 1 {
		t.Errorf("expected exit code 1, got %d", result.ExitCode)
	}
}

// TestRunAgentContainer_ContextTimeout verifies that context cancellation terminates the process.
func TestRunAgentContainer_ContextTimeout(t *testing.T) {
	c := &Controller{
		config:  SessionConfig{},
		workDir: "/tmp",
		logger:  log.New(io.Discard, "", 0),
	}

	// Use a short timeout to test cancellation (1s for CI resilience on slow machines)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	params := containerRunParams{
		Agent:   &dockerTestAgent{},
		Env:     map[string]string{},
		Command: []string{},
		LogTag:  "Test",
	}

	result, err := runAgentContainerWithCommand(ctx, c, params,
		"sleep", "30",
	)

	// The process should be killed by context, resulting in a non-zero exit code
	if err != nil {
		t.Fatalf("runAgentContainerWithCommand failed: %v", err)
	}

	if result.ExitCode == 0 {
		t.Error("expected non-zero exit code from cancelled context")
	}
}
