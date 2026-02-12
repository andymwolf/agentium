package controller

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// poolMockCmdRunner creates a cmdRunner that calls the test binary itself.
// The test binary dispatches to TestPoolHelperProcess which returns canned responses.
func poolMockCmdRunner(responses map[string]poolMockResponse) func(ctx context.Context, name string, args ...string) *exec.Cmd {
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		// Build a key from the first docker subcommand
		key := "unknown"
		if len(args) > 0 {
			key = args[0] // "run", "exec", "rm"
		}

		resp, ok := responses[key]
		if !ok {
			resp = poolMockResponse{stdout: "", exitCode: 0}
		}

		cs := []string{"-test.run=TestPoolHelperProcess", "--", key, resp.stdout, fmt.Sprintf("%d", resp.exitCode)}
		cmd := exec.CommandContext(ctx, os.Args[0], cs...)
		cmd.Env = append(os.Environ(),
			"GO_WANT_POOL_HELPER=1",
			fmt.Sprintf("POOL_MOCK_STDOUT=%s", resp.stdout),
			fmt.Sprintf("POOL_MOCK_EXIT=%d", resp.exitCode),
		)
		return cmd
	}
}

type poolMockResponse struct {
	stdout   string
	exitCode int
}

// TestPoolHelperProcess is a helper that simulates external commands.
// It is invoked by poolMockCmdRunner.
func TestPoolHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_POOL_HELPER") != "1" {
		return
	}
	stdout := os.Getenv("POOL_MOCK_STDOUT")
	exitCode := os.Getenv("POOL_MOCK_EXIT")

	_, _ = fmt.Fprint(os.Stdout, stdout)

	if exitCode != "0" {
		os.Exit(1)
	}
	os.Exit(0)
}

type capturedCall struct {
	name string
	args []string
}

func poolCapturingCmdRunner(responses map[string]poolMockResponse, calls *[]capturedCall) func(ctx context.Context, name string, args ...string) *exec.Cmd {
	inner := poolMockCmdRunner(responses)
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		*calls = append(*calls, capturedCall{name: name, args: append([]string(nil), args...)})
		return inner(ctx, name, args...)
	}
}

func newTestPoolLogger() *log.Logger {
	return log.New(os.Stderr, "[pool-test] ", log.LstdFlags)
}

func TestContainerPool_Start(t *testing.T) {
	responses := map[string]poolMockResponse{
		"run": {stdout: "abc123def456\n", exitCode: 0},
	}

	pool := NewContainerPool("/workspace", 0, "test-session-id", "IMPLEMENT",
		poolMockCmdRunner(responses), newTestPoolLogger(), nil)

	id, err := pool.Start(context.Background(), RoleWorkerContainer, "test-image:latest",
		[]string{"/runtime-scripts/agent-wrapper.sh", "claude"}, map[string]string{"FOO": "bar"}, nil)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if id == "" {
		t.Fatal("Start() returned empty container ID")
	}

	// Verify container is tracked
	mc := pool.Get(RoleWorkerContainer)
	if mc == nil {
		t.Fatal("Get() returned nil after Start")
	}
	if !mc.Healthy {
		t.Error("Container should be healthy after Start")
	}
	if mc.Role != RoleWorkerContainer {
		t.Errorf("Role = %v, want %v", mc.Role, RoleWorkerContainer)
	}
}

func TestContainerPool_IsHealthy(t *testing.T) {
	responses := map[string]poolMockResponse{
		"run": {stdout: "container-id\n", exitCode: 0},
	}

	pool := NewContainerPool("/workspace", 0, "sess", "PLAN",
		poolMockCmdRunner(responses), newTestPoolLogger(), nil)

	// Not started yet
	if pool.IsHealthy(RoleWorkerContainer) {
		t.Error("IsHealthy should be false before Start")
	}

	_, err := pool.Start(context.Background(), RoleWorkerContainer, "img", []string{"agent"}, nil, nil)
	if err != nil {
		t.Fatalf("Start error: %v", err)
	}

	if !pool.IsHealthy(RoleWorkerContainer) {
		t.Error("IsHealthy should be true after Start")
	}

	// Mark unhealthy
	pool.MarkUnhealthy(RoleWorkerContainer)
	if pool.IsHealthy(RoleWorkerContainer) {
		t.Error("IsHealthy should be false after MarkUnhealthy")
	}
}

func TestContainerPool_StopAll(t *testing.T) {
	responses := map[string]poolMockResponse{
		"run": {stdout: "container-id\n", exitCode: 0},
		"rm":  {stdout: "", exitCode: 0},
	}

	pool := NewContainerPool("/workspace", 0, "sess", "PLAN",
		poolMockCmdRunner(responses), newTestPoolLogger(), nil)

	_, err := pool.Start(context.Background(), RoleWorkerContainer, "img", []string{"agent"}, nil, nil)
	if err != nil {
		t.Fatalf("Start error: %v", err)
	}
	_, err = pool.Start(context.Background(), RoleReviewerContainer, "img", []string{"agent"}, nil, nil)
	if err != nil {
		t.Fatalf("Start error: %v", err)
	}

	pool.StopAll(context.Background())

	if pool.Get(RoleWorkerContainer) != nil {
		t.Error("Get(worker) should be nil after StopAll")
	}
	if pool.Get(RoleReviewerContainer) != nil {
		t.Error("Get(reviewer) should be nil after StopAll")
	}
}

func TestContainerPool_containerName(t *testing.T) {
	pool := NewContainerPool("/workspace", 0, "abcdefghijklmnop", "IMPLEMENT",
		nil, newTestPoolLogger(), nil)

	name := pool.containerName(RoleWorkerContainer)
	if !strings.HasPrefix(name, "agentium-") {
		t.Errorf("containerName should start with 'agentium-', got %q", name)
	}
	if !strings.Contains(name, "implement") {
		t.Errorf("containerName should contain phase, got %q", name)
	}
	if !strings.HasSuffix(name, "-worker") {
		t.Errorf("containerName should end with role, got %q", name)
	}
	// Session suffix should be last 8 chars
	if !strings.Contains(name, "ijklmnop") {
		t.Errorf("containerName should contain session suffix, got %q", name)
	}
}

func TestContainerPool_Exec_NoContainer(t *testing.T) {
	pool := NewContainerPool("/workspace", 0, "sess", "PLAN",
		nil, newTestPoolLogger(), nil)

	_, _, _, err := pool.Exec(context.Background(), RoleWorkerContainer, []string{"echo"}, "", nil)
	if err == nil {
		t.Fatal("Exec should fail when no container exists")
	}
}

func TestContainerPool_StartWithMemLimit(t *testing.T) {
	responses := map[string]poolMockResponse{
		"run": {stdout: "mem-container\n", exitCode: 0},
	}

	memLimit := uint64(4 * 1024 * 1024 * 1024) // 4GB
	pool := NewContainerPool("/workspace", memLimit, "sess", "PLAN",
		poolMockCmdRunner(responses), newTestPoolLogger(), nil)

	id, err := pool.Start(context.Background(), RoleWorkerContainer, "img",
		[]string{"agent"}, map[string]string{"KEY": "val"}, []string{"-v", "/tmp/claude/auth:/home/agentium/.claude/.credentials.json:ro"})
	if err != nil {
		t.Fatalf("Start error: %v", err)
	}
	if id == "" {
		t.Fatal("Start returned empty ID")
	}
}

func TestContainerPool_Start_EmptyEntrypoint(t *testing.T) {
	pool := NewContainerPool("/workspace", 0, "sess", "PLAN",
		poolMockCmdRunner(nil), newTestPoolLogger(), nil)

	_, err := pool.Start(context.Background(), RoleWorkerContainer, "img", nil, nil, nil)
	if err == nil {
		t.Fatal("Start should fail with nil entrypoint")
	}
	if !strings.Contains(err.Error(), "entrypoint") {
		t.Errorf("error should mention entrypoint, got: %v", err)
	}

	_, err = pool.Start(context.Background(), RoleWorkerContainer, "img", []string{}, nil, nil)
	if err == nil {
		t.Fatal("Start should fail with empty entrypoint")
	}
}

func TestContainerPool_Exec_PrependsEntrypoint(t *testing.T) {
	responses := map[string]poolMockResponse{
		"run":  {stdout: "container-id\n", exitCode: 0},
		"exec": {stdout: "output", exitCode: 0},
	}

	var calls []capturedCall
	pool := NewContainerPool("/workspace", 0, "sess", "PLAN",
		poolCapturingCmdRunner(responses, &calls), newTestPoolLogger(), nil)

	entrypoint := []string{"/runtime-scripts/agent-wrapper.sh", "claude"}
	_, err := pool.Start(context.Background(), RoleWorkerContainer, "img", entrypoint, nil, nil)
	if err != nil {
		t.Fatalf("Start error: %v", err)
	}

	command := []string{"--print", "--prompt", "hello"}
	_, _, exitCode, err := pool.Exec(context.Background(), RoleWorkerContainer, command, "", nil)
	if err != nil {
		t.Fatalf("Exec error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("Exec exit code = %d, want 0", exitCode)
	}

	// Find the exec call and verify entrypoint is prepended before command
	var execArgs []string
	for _, c := range calls {
		if len(c.args) > 0 && c.args[0] == "exec" {
			execArgs = c.args
			break
		}
	}
	if execArgs == nil {
		t.Fatal("no docker exec call captured")
	}

	// Expected args: ["exec", "<container-id>", "/runtime-scripts/agent-wrapper.sh", "claude", "--print", "--prompt", "hello"]
	joined := strings.Join(execArgs, " ")
	expectedSeq := strings.Join(append(entrypoint, command...), " ")
	if !strings.Contains(joined, expectedSeq) {
		t.Errorf("exec args should contain entrypoint+command sequence %q, got %v", expectedSeq, execArgs)
	}
}
