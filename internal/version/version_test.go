package version

import (
	"runtime"
	"strings"
	"testing"
)

func TestShort(t *testing.T) {
	// Default value should be "dev"
	result := Short()
	if result != Version {
		t.Errorf("Short() = %q, want %q", result, Version)
	}
}

func TestInfo(t *testing.T) {
	result := Info()

	// Should contain key components
	if !strings.Contains(result, "agentium") {
		t.Errorf("Info() should contain 'agentium', got %q", result)
	}
	if !strings.Contains(result, Version) {
		t.Errorf("Info() should contain version %q, got %q", Version, result)
	}
	if !strings.Contains(result, "commit:") {
		t.Errorf("Info() should contain 'commit:', got %q", result)
	}
	if !strings.Contains(result, "built:") {
		t.Errorf("Info() should contain 'built:', got %q", result)
	}
	if !strings.Contains(result, runtime.Version()) {
		t.Errorf("Info() should contain Go version %q, got %q", runtime.Version(), result)
	}
}

func TestInfoCommitTruncation(t *testing.T) {
	// Save original and restore after test
	originalCommit := Commit
	defer func() { Commit = originalCommit }()

	// Test with a long commit SHA
	Commit = "abc123456789abcdef"
	result := Info()

	// Should contain truncated commit (7 chars)
	if !strings.Contains(result, "abc1234") {
		t.Errorf("Info() should contain truncated commit 'abc1234', got %q", result)
	}
	// Should NOT contain full commit
	if strings.Contains(result, "abc123456789abcdef") {
		t.Errorf("Info() should NOT contain full commit, got %q", result)
	}
}

func TestInfoShortCommit(t *testing.T) {
	// Save original and restore after test
	originalCommit := Commit
	defer func() { Commit = originalCommit }()

	// Test with a short commit (less than 7 chars)
	Commit = "abc"
	result := Info()

	// Should contain the short commit as-is
	if !strings.Contains(result, "abc") {
		t.Errorf("Info() should contain short commit 'abc', got %q", result)
	}
}

func TestFull(t *testing.T) {
	result := Full()

	// Should contain all components
	if !strings.Contains(result, "agentium") {
		t.Errorf("Full() should contain 'agentium', got %q", result)
	}
	if !strings.Contains(result, Version) {
		t.Errorf("Full() should contain version %q, got %q", Version, result)
	}
	if !strings.Contains(result, "Commit:") {
		t.Errorf("Full() should contain 'Commit:', got %q", result)
	}
	if !strings.Contains(result, "Built:") {
		t.Errorf("Full() should contain 'Built:', got %q", result)
	}
	if !strings.Contains(result, "Go version:") {
		t.Errorf("Full() should contain 'Go version:', got %q", result)
	}
	if !strings.Contains(result, "OS/Arch:") {
		t.Errorf("Full() should contain 'OS/Arch:', got %q", result)
	}
	if !strings.Contains(result, runtime.GOOS) {
		t.Errorf("Full() should contain OS %q, got %q", runtime.GOOS, result)
	}
	if !strings.Contains(result, runtime.GOARCH) {
		t.Errorf("Full() should contain arch %q, got %q", runtime.GOARCH, result)
	}
}

func TestFullMultiLine(t *testing.T) {
	result := Full()

	// Should be multi-line
	lines := strings.Split(result, "\n")
	if len(lines) < 5 {
		t.Errorf("Full() should have at least 5 lines, got %d: %q", len(lines), result)
	}
}
