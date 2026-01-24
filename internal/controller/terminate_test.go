package controller

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// mockCmdRunner creates a command runner that records invocations and returns
// configurable output/errors via a helper test binary pattern.
type mockCmdRunner struct {
	calls   []mockCall
	handler func(name string, args []string) ([]byte, error)
}

type mockCall struct {
	name string
	args []string
}

// newSuccessRunner returns a cmdRunner that simulates successful metadata fetches
// and a successful gcloud delete command.
func newSuccessRunner(instanceName, zone string) *mockCmdRunner {
	m := &mockCmdRunner{}
	m.handler = func(name string, args []string) ([]byte, error) {
		if name == "curl" {
			for _, arg := range args {
				if strings.Contains(arg, "/instance/name") {
					return []byte(instanceName), nil
				}
				if strings.Contains(arg, "/instance/zone") {
					return []byte(zone), nil
				}
			}
		}
		if name == "gcloud" {
			return nil, nil // Successful deletion
		}
		return nil, fmt.Errorf("unexpected command: %s", name)
	}
	return m
}

// run creates a cmdRunner function that can be assigned to Controller.cmdRunner.
// It uses "echo" as the underlying command but captures and tracks calls.
func (m *mockCmdRunner) run(ctx context.Context, name string, args ...string) *exec.Cmd {
	m.calls = append(m.calls, mockCall{name: name, args: args})
	output, err := m.handler(name, args)

	if err != nil {
		// Return a command that will fail
		return exec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("echo %q >&2; exit 1", err.Error()))
	}

	// Return a command that outputs the expected data
	if output != nil {
		return exec.CommandContext(ctx, "echo", "-n", string(output))
	}
	return exec.CommandContext(ctx, "true")
}

func TestTerminateVM_Success(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "[test] ", 0)

	c := &Controller{
		logger:     logger,
		shutdownCh: make(chan struct{}),
	}

	runner := newSuccessRunner("test-instance", "projects/my-project/zones/us-central1-a")
	c.cmdRunner = runner.run

	c.terminateVM()

	logOutput := buf.String()

	// Verify it logged initiation
	if !strings.Contains(logOutput, "Initiating VM termination") {
		t.Errorf("expected 'Initiating VM termination' in log, got: %s", logOutput)
	}

	// Verify it logged the instance details
	if !strings.Contains(logOutput, "Deleting VM instance test-instance in zone us-central1-a") {
		t.Errorf("expected instance details in log, got: %s", logOutput)
	}

	// Verify it logged success
	if !strings.Contains(logOutput, "VM deletion command completed successfully") {
		t.Errorf("expected success message in log, got: %s", logOutput)
	}

	// Verify all three commands were called: two curl + one gcloud
	if len(runner.calls) != 3 {
		t.Fatalf("expected 3 commands, got %d: %+v", len(runner.calls), runner.calls)
	}

	// Verify the gcloud command arguments
	gcloudCall := runner.calls[2]
	if gcloudCall.name != "gcloud" {
		t.Errorf("expected third call to be 'gcloud', got %q", gcloudCall.name)
	}
	expectedArgs := []string{"compute", "instances", "delete", "test-instance", "--zone", "us-central1-a", "--quiet"}
	if len(gcloudCall.args) != len(expectedArgs) {
		t.Fatalf("expected %d gcloud args, got %d: %v", len(expectedArgs), len(gcloudCall.args), gcloudCall.args)
	}
	for i, expected := range expectedArgs {
		if gcloudCall.args[i] != expected {
			t.Errorf("gcloud arg[%d]: expected %q, got %q", i, expected, gcloudCall.args[i])
		}
	}
}

func TestTerminateVM_InstanceNameFetchError(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "[test] ", 0)

	c := &Controller{
		logger:     logger,
		shutdownCh: make(chan struct{}),
	}

	runner := &mockCmdRunner{}
	runner.handler = func(name string, args []string) ([]byte, error) {
		if name == "curl" {
			return nil, fmt.Errorf("connection refused")
		}
		return nil, nil
	}
	c.cmdRunner = runner.run

	c.terminateVM()

	logOutput := buf.String()

	// Should log an error about metadata
	if !strings.Contains(logOutput, "Error: failed to get instance name from metadata") {
		t.Errorf("expected metadata error in log, got: %s", logOutput)
	}
	if !strings.Contains(logOutput, "VM will not be deleted") {
		t.Errorf("expected 'VM will not be deleted' warning, got: %s", logOutput)
	}

	// Should only have called curl once (failed on first attempt)
	if len(runner.calls) != 1 {
		t.Errorf("expected 1 command call (failed early), got %d", len(runner.calls))
	}
}

func TestTerminateVM_ZoneFetchError(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "[test] ", 0)

	c := &Controller{
		logger:     logger,
		shutdownCh: make(chan struct{}),
	}

	callCount := 0
	runner := &mockCmdRunner{}
	runner.handler = func(name string, args []string) ([]byte, error) {
		if name == "curl" {
			callCount++
			if callCount == 1 {
				// First curl (instance name) succeeds
				return []byte("test-instance"), nil
			}
			// Second curl (zone) fails
			return nil, fmt.Errorf("metadata unavailable")
		}
		return nil, nil
	}
	c.cmdRunner = runner.run

	c.terminateVM()

	logOutput := buf.String()

	if !strings.Contains(logOutput, "Error: failed to get zone from metadata") {
		t.Errorf("expected zone error in log, got: %s", logOutput)
	}
	if !strings.Contains(logOutput, "VM will not be deleted") {
		t.Errorf("expected 'VM will not be deleted' warning, got: %s", logOutput)
	}

	// Should have called curl twice (name succeeded, zone failed)
	if len(runner.calls) != 2 {
		t.Errorf("expected 2 command calls, got %d", len(runner.calls))
	}
}

func TestTerminateVM_DeletionFailure(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "[test] ", 0)

	c := &Controller{
		logger:     logger,
		shutdownCh: make(chan struct{}),
	}

	runner := &mockCmdRunner{}
	runner.handler = func(name string, args []string) ([]byte, error) {
		if name == "curl" {
			for _, arg := range args {
				if strings.Contains(arg, "/instance/name") {
					return []byte("test-instance"), nil
				}
				if strings.Contains(arg, "/instance/zone") {
					return []byte("projects/p/zones/us-east1-b"), nil
				}
			}
		}
		if name == "gcloud" {
			return nil, fmt.Errorf("permission denied")
		}
		return nil, nil
	}
	c.cmdRunner = runner.run

	c.terminateVM()

	logOutput := buf.String()

	if !strings.Contains(logOutput, "Error: VM deletion command failed") {
		t.Errorf("expected deletion error in log, got: %s", logOutput)
	}
	if !strings.Contains(logOutput, "VM may remain running until max_run_duration") {
		t.Errorf("expected max_run_duration warning, got: %s", logOutput)
	}
}

func TestTerminateVM_Timeout(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "[test] ", 0)

	c := &Controller{
		logger:     logger,
		shutdownCh: make(chan struct{}),
	}

	// Use a real command that will hang, but the context timeout should kill it
	c.cmdRunner = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if name == "gcloud" {
			// Simulate a command that hangs
			return exec.CommandContext(ctx, "sleep", "60")
		}
		// For curl calls, return the expected metadata
		for _, arg := range args {
			if strings.Contains(arg, "/instance/name") {
				return exec.CommandContext(ctx, "echo", "-n", "test-instance")
			}
			if strings.Contains(arg, "/instance/zone") {
				return exec.CommandContext(ctx, "echo", "-n", "projects/p/zones/us-west1-a")
			}
		}
		return exec.CommandContext(ctx, "true")
	}

	start := time.Now()
	c.terminateVM()
	elapsed := time.Since(start)

	// Should complete within VMTerminationTimeout (30s) + a buffer, not wait for 60s sleep
	if elapsed > VMTerminationTimeout+2*time.Second {
		t.Errorf("terminateVM took %v, should have timed out within %v", elapsed, VMTerminationTimeout+2*time.Second)
	}

	logOutput := buf.String()
	if !strings.Contains(logOutput, "Error: VM deletion command failed") {
		t.Errorf("expected timeout error in log, got: %s", logOutput)
	}
}

func TestTerminateVM_TrimsWhitespace(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "[test] ", 0)

	c := &Controller{
		logger:     logger,
		shutdownCh: make(chan struct{}),
	}

	// Return metadata with whitespace/newlines (as curl often does)
	runner := newSuccessRunner("  my-instance\n", "projects/p/zones/europe-west1-b\n")
	c.cmdRunner = runner.run

	c.terminateVM()

	// Verify the gcloud command received trimmed values
	if len(runner.calls) < 3 {
		t.Fatalf("expected at least 3 calls, got %d", len(runner.calls))
	}
	gcloudCall := runner.calls[2]
	// Instance name should be trimmed
	if gcloudCall.args[3] != "my-instance" {
		t.Errorf("expected trimmed instance name 'my-instance', got %q", gcloudCall.args[3])
	}
	// Zone should be the basename and trimmed
	if gcloudCall.args[5] != "europe-west1-b" {
		t.Errorf("expected zone 'europe-west1-b', got %q", gcloudCall.args[5])
	}
}

func TestTerminateVM_ZonePathExtraction(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "[test] ", 0)

	c := &Controller{
		logger:     logger,
		shutdownCh: make(chan struct{}),
	}

	// The zone metadata returns a full path like "projects/123/zones/us-central1-a"
	// terminateVM should extract just the zone name
	runner := newSuccessRunner("instance-1", "projects/my-project/zones/asia-east1-c")
	c.cmdRunner = runner.run

	c.terminateVM()

	if len(runner.calls) < 3 {
		t.Fatalf("expected at least 3 calls, got %d", len(runner.calls))
	}
	gcloudCall := runner.calls[2]
	if gcloudCall.args[5] != "asia-east1-c" {
		t.Errorf("expected zone 'asia-east1-c', got %q", gcloudCall.args[5])
	}
}

func TestExecCommand_DefaultsToExecCommandContext(t *testing.T) {
	c := &Controller{
		logger:     newTestLogger(),
		shutdownCh: make(chan struct{}),
	}

	// Without cmdRunner set, execCommand should return a real exec.Cmd
	ctx := context.Background()
	cmd := c.execCommand(ctx, "echo", "hello")
	if cmd == nil {
		t.Fatal("expected non-nil command from execCommand")
	}

	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("expected echo to succeed: %v", err)
	}
	if strings.TrimSpace(string(output)) != "hello" {
		t.Errorf("expected 'hello', got %q", string(output))
	}
}

func TestExecCommand_UsesCustomRunner(t *testing.T) {
	c := &Controller{
		logger:     newTestLogger(),
		shutdownCh: make(chan struct{}),
	}

	called := false
	c.cmdRunner = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		called = true
		return exec.CommandContext(ctx, "echo", "custom")
	}

	ctx := context.Background()
	cmd := c.execCommand(ctx, "anything")
	output, _ := cmd.Output()

	if !called {
		t.Error("expected custom cmdRunner to be called")
	}
	if strings.TrimSpace(string(output)) != "custom" {
		t.Errorf("expected 'custom', got %q", string(output))
	}
}
