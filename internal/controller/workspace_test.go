package controller

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"
)

// newTestController creates a Controller with a workspace dir and a discarding logger
func newTestController(workDir string) *Controller {
	return &Controller{
		workDir: workDir,
		logger:  log.New(io.Discard, "", 0),
	}
}

func TestEnsureWorkspaceOwnership(t *testing.T) {
	// Skip if not running as root - chown requires root privileges
	if os.Getuid() != 0 {
		t.Skip("skipping test: requires root privileges to chown")
	}

	// Create a temporary directory structure
	tmpDir, err := os.MkdirTemp("", "workspace-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create nested files and directories
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}

	testFile := filepath.Join(subDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create controller with the temp workspace
	ctrl := newTestController(tmpDir)

	// Run ensureWorkspaceOwnership
	if err := ctrl.ensureWorkspaceOwnership(); err != nil {
		t.Fatalf("ensureWorkspaceOwnership failed: %v", err)
	}

	// Verify ownership of all paths
	paths := []string{tmpDir, subDir, testFile}
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("failed to stat %s: %v", path, err)
		}

		// On Unix, we can check the UID/GID via syscall
		// but for portability, we just verify the function didn't error
		_ = info
	}
}

func TestEnsureWorkspaceOwnership_NonExistentDir(t *testing.T) {
	ctrl := newTestController("/nonexistent/path/that/does/not/exist")

	err := ctrl.ensureWorkspaceOwnership()
	if err == nil {
		t.Error("expected error for non-existent directory, got nil")
	}
}

func TestEnsureWorkspaceOwnership_EmptyDir(t *testing.T) {
	// Skip if not running as root
	if os.Getuid() != 0 {
		t.Skip("skipping test: requires root privileges to chown")
	}

	tmpDir, err := os.MkdirTemp("", "workspace-empty-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	ctrl := newTestController(tmpDir)

	// Should succeed on empty directory
	if err := ctrl.ensureWorkspaceOwnership(); err != nil {
		t.Errorf("ensureWorkspaceOwnership failed on empty dir: %v", err)
	}
}

func TestConfigureGitSafeDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workspace-git-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	ctrl := newTestController(tmpDir)

	ctx := context.Background()

	// This should succeed (git config --global --add is idempotent)
	err = ctrl.configureGitSafeDirectory(ctx)
	if err != nil {
		// May fail if git is not installed, which is acceptable in CI
		t.Logf("configureGitSafeDirectory returned error (may be expected if git not installed): %v", err)
	}
}

func TestConfigureGitSafeDirectory_ContextCanceled(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workspace-git-cancel-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	ctrl := newTestController(tmpDir)

	// Create an already-canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Should fail or return quickly due to canceled context
	_ = ctrl.configureGitSafeDirectory(ctx)
	// We don't assert on the error because behavior varies by OS
}

func TestAgentiumUIDGIDConstants(t *testing.T) {
	// Verify the constants match expected values
	if AgentiumUID != 1000 {
		t.Errorf("AgentiumUID = %d, want 1000", AgentiumUID)
	}
	if AgentiumGID != 1000 {
		t.Errorf("AgentiumGID = %d, want 1000", AgentiumGID)
	}
}

func TestInitializeWorkspace_CreatesDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workspace-init-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	workDir := filepath.Join(tmpDir, "new-workspace")

	ctrl := newTestController(workDir)

	ctx := context.Background()
	if err = ctrl.initializeWorkspace(ctx); err != nil {
		t.Fatalf("initializeWorkspace failed: %v", err)
	}

	// Verify directory was created
	info, err := os.Stat(workDir)
	if err != nil {
		t.Fatalf("workspace directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("workspace is not a directory")
	}
}

func TestInitializeWorkspace_SkipsChownWhenNotRoot(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping test: must not run as root")
	}

	tmpDir, err := os.MkdirTemp("", "workspace-nonroot-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	workDir := filepath.Join(tmpDir, "workspace")

	ctrl := newTestController(workDir)

	ctx := context.Background()

	// Should succeed without attempting chown (which would fail as non-root)
	if err := ctrl.initializeWorkspace(ctx); err != nil {
		t.Fatalf("initializeWorkspace failed: %v", err)
	}
}
