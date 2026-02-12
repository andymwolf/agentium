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

func newTestPoolLogger() *log.Logger {
	return log.New(os.Stderr, "[pool-test] ", log.LstdFlags)
}

func TestContainerPool_Start(t *testing.T) {
	responses := map[string]poolMockResponse{
		"run": {stdout: "abc123def456\n", exitCode: 0},
	}

	pool := NewContainerPool("/workspace", 0, "test-session-id", "IMPLEMENT",
		poolMockCmdRunner(responses), newTestPoolLogger())

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
		poolMockCmdRunner(responses), newTestPoolLogger())

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
		poolMockCmdRunner(responses), newTestPoolLogger())

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
		nil, newTestPoolLogger())

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
		nil, newTestPoolLogger())

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
		poolMockCmdRunner(responses), newTestPoolLogger())

	id, err := pool.Start(context.Background(), RoleWorkerContainer, "img",
		[]string{"agent"}, map[string]string{"KEY": "val"}, []string{"-v", "/tmp/claude/auth:/home/agentium/.claude/.credentials.json:ro"})
	if err != nil {
		t.Fatalf("Start error: %v", err)
	}
	if id == "" {
		t.Fatal("Start returned empty ID")
	}
}
