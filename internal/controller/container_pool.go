package controller

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
)

// ContainerRole identifies the role of a managed container within a phase.
type ContainerRole string

const (
	RoleWorkerContainer   ContainerRole = "worker"
	RoleReviewerContainer ContainerRole = "reviewer"
	RoleJudgeContainer    ContainerRole = "judge"
)

// ManagedContainer tracks a long-lived Docker container started for a phase.
type ManagedContainer struct {
	ID         string        // Docker container ID
	Role       ContainerRole // worker, reviewer, or judge
	Phase      string        // Phase this container belongs to
	Image      string        // Docker image used
	Entrypoint []string      // Original container entrypoint for docker exec
	ExecCount  int           // Number of exec calls made
	Healthy    bool          // Whether the container is considered healthy
}

// ContainerPool manages long-lived containers for a single phase.
// Containers are started once at the beginning of a phase and reused
// via docker exec for each iteration, avoiding repeated container startup costs.
type ContainerPool struct {
	mu         sync.Mutex
	containers map[ContainerRole]*ManagedContainer
	workDir    string
	memLimit   uint64
	sessionID  string
	phase      string
	cmdRunner  func(ctx context.Context, name string, args ...string) *exec.Cmd
	logger     *log.Logger
}

// NewContainerPool creates a new ContainerPool for managing phase containers.
func NewContainerPool(workDir string, memLimit uint64, sessionID, phase string, cmdRunner func(ctx context.Context, name string, args ...string) *exec.Cmd, logger *log.Logger) *ContainerPool {
	return &ContainerPool{
		containers: make(map[ContainerRole]*ManagedContainer),
		workDir:    workDir,
		memLimit:   memLimit,
		sessionID:  sessionID,
		phase:      phase,
		cmdRunner:  cmdRunner,
		logger:     logger,
	}
}

// containerName generates a deterministic container name for debuggability.
// Format: agentium-<session-suffix>-<phase>-<role>
func (p *ContainerPool) containerName(role ContainerRole) string {
	suffix := p.sessionID
	if len(suffix) > 8 {
		suffix = suffix[len(suffix)-8:]
	}
	return fmt.Sprintf("agentium-%s-%s-%s", suffix, strings.ToLower(p.phase), string(role))
}

// Start creates a long-lived container for the given role using docker run -d
// with an entrypoint of "sleep infinity". The container stays running until
// StopAll is called or it is marked unhealthy. The entrypoint parameter is the
// original container entrypoint (e.g., ["/runtime-scripts/agent-wrapper.sh", "claude"])
// which will be prepended to commands in Exec since the container's own entrypoint
// is overridden to "sleep".
func (p *ContainerPool) Start(ctx context.Context, role ContainerRole, image string, entrypoint []string, env map[string]string, authMounts []string) (string, error) {
	if len(entrypoint) == 0 {
		return "", fmt.Errorf("entrypoint must not be empty for pooled containers (role=%s)", role)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	name := p.containerName(role)

	args := []string{
		"run", "-d",
		"--name", name,
		"-v", fmt.Sprintf("%s:/workspace", p.workDir),
		"-w", "/workspace",
		"--entrypoint", "sleep",
	}

	if p.memLimit > 0 {
		limit := fmt.Sprintf("%d", p.memLimit)
		args = append(args, "--memory", limit, "--memory-swap", limit)
	}

	for k, v := range env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	// Add auth mounts (e.g., OAuth credential files)
	args = append(args, authMounts...)

	args = append(args, image, "infinity")

	cmd := p.cmdRunner(ctx, "docker", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to start container %s: %w (stderr: %s)", name, err, stderr.String())
	}

	containerID := strings.TrimSpace(stdout.String())
	if containerID == "" {
		return "", fmt.Errorf("docker run returned empty container ID for %s", name)
	}

	p.containers[role] = &ManagedContainer{
		ID:         containerID,
		Role:       role,
		Phase:      p.phase,
		Image:      image,
		Entrypoint: entrypoint,
		Healthy:    true,
	}

	shortID := containerID
	if len(shortID) > 12 {
		shortID = shortID[:12]
	}
	p.logger.Printf("[pool] Started container %s (role=%s, phase=%s, id=%s)", name, role, p.phase, shortID)
	return containerID, nil
}

// Exec runs a command inside an existing container via docker exec.
// Returns stdout, stderr, exit code, and any error.
func (p *ContainerPool) Exec(ctx context.Context, role ContainerRole, command []string, stdinPrompt string, extraEnv map[string]string) ([]byte, []byte, int, error) {
	p.mu.Lock()
	mc := p.containers[role]
	p.mu.Unlock()

	if mc == nil {
		return nil, nil, -1, fmt.Errorf("no container for role %s", role)
	}
	if !mc.Healthy {
		return nil, nil, -1, fmt.Errorf("container for role %s is unhealthy", role)
	}

	args := []string{"exec"}

	// Add -i flag when piping stdin
	if stdinPrompt != "" {
		args = append(args, "-i")
	}

	// Pass extra environment variables via docker exec -e
	for k, v := range extraEnv {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	args = append(args, mc.ID)
	args = append(args, mc.Entrypoint...)
	args = append(args, command...)

	cmd := p.cmdRunner(ctx, "docker", args...)

	if stdinPrompt != "" {
		cmd.Stdin = strings.NewReader(stdinPrompt)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	exitCode := 0
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			// Mark unhealthy on non-exit errors (container gone, etc.)
			p.mu.Lock()
			mc.Healthy = false
			p.mu.Unlock()
			return nil, nil, -1, fmt.Errorf("exec failed for role %s: %w", role, err)
		}
	}

	p.mu.Lock()
	mc.ExecCount++
	p.mu.Unlock()

	return stdout.Bytes(), stderr.Bytes(), exitCode, nil
}

// StopAll removes all managed containers via docker rm -f.
// Called at the end of each phase.
func (p *ContainerPool) StopAll(ctx context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for role, mc := range p.containers {
		name := p.containerName(role)
		cmd := p.cmdRunner(ctx, "docker", "rm", "-f", mc.ID)
		if out, err := cmd.CombinedOutput(); err != nil {
			p.logger.Printf("[pool] Warning: failed to remove container %s: %v (%s)", name, err, strings.TrimSpace(string(out)))
		} else {
			p.logger.Printf("[pool] Removed container %s (execs=%d)", name, mc.ExecCount)
		}
	}

	p.containers = make(map[ContainerRole]*ManagedContainer)
}

// IsHealthy returns true if a container for the given role exists and is healthy.
func (p *ContainerPool) IsHealthy(role ContainerRole) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	mc, ok := p.containers[role]
	return ok && mc.Healthy
}

// Get returns the ManagedContainer for the given role, or nil if not found.
func (p *ContainerPool) Get(role ContainerRole) *ManagedContainer {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.containers[role]
}

// MarkUnhealthy marks the container for the given role as unhealthy,
// triggering fallback to one-shot execution.
func (p *ContainerPool) MarkUnhealthy(role ContainerRole) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if mc, ok := p.containers[role]; ok {
		mc.Healthy = false
		p.logger.Printf("[pool] Marked container for role %s as unhealthy", role)
	}
}
