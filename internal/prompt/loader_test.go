package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSystemPrompt_FileExists(t *testing.T) {
	// Create a temp directory with prompts/SYSTEM.md
	tmpDir := t.TempDir()
	promptsDir := filepath.Join(tmpDir, "prompts")
	if err := os.MkdirAll(promptsDir, 0755); err != nil {
		t.Fatal(err)
	}

	expected := "# System Prompt\nThis is the system prompt content."
	if err := os.WriteFile(filepath.Join(promptsDir, "SYSTEM.md"), []byte(expected), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := LoadSystemPrompt(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestLoadSystemPrompt_FileNotExists(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := LoadSystemPrompt(tmpDir)
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoadProjectPrompt_FileExists(t *testing.T) {
	// Create a temp directory with AGENT.md at root
	tmpDir := t.TempDir()

	expected := "# Project Instructions\nRun tests with: go test ./..."
	if err := os.WriteFile(filepath.Join(tmpDir, "AGENT.md"), []byte(expected), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := LoadProjectPrompt(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestLoadProjectPrompt_FileNotExists(t *testing.T) {
	tmpDir := t.TempDir()

	result, err := LoadProjectPrompt(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty string for missing file, got %q", result)
	}
}

func TestLoadProjectPrompt_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(tmpDir, "AGENT.md"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := LoadProjectPrompt(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty string for empty file, got %q", result)
	}
}

func TestLoadProjectPromptWithPackage_NoFiles(t *testing.T) {
	tmpDir := t.TempDir()

	result, err := LoadProjectPromptWithPackage(tmpDir, "packages/core")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty string when no AGENT.md files exist, got %q", result)
	}
}

func TestLoadProjectPromptWithPackage_RootOnly(t *testing.T) {
	tmpDir := t.TempDir()

	// Create root AGENT.md
	rootContent := "# Root Instructions\nRun tests with go test"
	if err := os.WriteFile(filepath.Join(tmpDir, "AGENT.md"), []byte(rootContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := LoadProjectPromptWithPackage(tmpDir, "packages/core")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "Repository Instructions") {
		t.Errorf("result should contain 'Repository Instructions' header")
	}
	if !strings.Contains(result, rootContent) {
		t.Errorf("result should contain root content")
	}
}

func TestLoadProjectPromptWithPackage_PackageOnly(t *testing.T) {
	tmpDir := t.TempDir()

	// Create package AGENT.md at packages/core/AGENT.md
	pkgDir := filepath.Join(tmpDir, "packages", "core")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatal(err)
	}

	pkgContent := "# Package Instructions\nThis is the core package"
	if err := os.WriteFile(filepath.Join(pkgDir, "AGENT.md"), []byte(pkgContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := LoadProjectPromptWithPackage(tmpDir, "packages/core")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "Package Instructions (packages/core)") {
		t.Errorf("result should contain package header with path")
	}
	if !strings.Contains(result, pkgContent) {
		t.Errorf("result should contain package content")
	}
}

func TestLoadProjectPromptWithPackage_Both(t *testing.T) {
	tmpDir := t.TempDir()

	// Create root AGENT.md
	rootContent := "Root instructions"
	if err := os.WriteFile(filepath.Join(tmpDir, "AGENT.md"), []byte(rootContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create package AGENT.md at packages/core/AGENT.md
	pkgDir := filepath.Join(tmpDir, "packages", "core")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatal(err)
	}
	pkgContent := "Package instructions"
	if err := os.WriteFile(filepath.Join(pkgDir, "AGENT.md"), []byte(pkgContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := LoadProjectPromptWithPackage(tmpDir, "packages/core")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify both sections are present
	if !strings.Contains(result, "Repository Instructions") {
		t.Errorf("result should contain repository header")
	}
	if !strings.Contains(result, rootContent) {
		t.Errorf("result should contain root content")
	}
	if !strings.Contains(result, "Package Instructions") {
		t.Errorf("result should contain package header")
	}
	if !strings.Contains(result, pkgContent) {
		t.Errorf("result should contain package content")
	}
	if !strings.Contains(result, "---") {
		t.Errorf("result should contain separator between sections")
	}
}

func TestLoadProjectPromptWithPackage_EmptyPackagePath(t *testing.T) {
	tmpDir := t.TempDir()

	// Create root AGENT.md
	rootContent := "Root instructions only"
	if err := os.WriteFile(filepath.Join(tmpDir, "AGENT.md"), []byte(rootContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Empty package path - should only load root
	result, err := LoadProjectPromptWithPackage(tmpDir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, rootContent) {
		t.Errorf("result should contain root content")
	}
	// Should not have package section header
	if strings.Contains(result, "Package Instructions") {
		t.Errorf("result should not contain package header when package path is empty")
	}
}
