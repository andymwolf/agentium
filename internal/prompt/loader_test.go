package prompt

import (
	"os"
	"path/filepath"
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
	// Create a temp directory with .agentium/AGENT.md
	tmpDir := t.TempDir()
	agentiumDir := filepath.Join(tmpDir, ".agentium")
	if err := os.MkdirAll(agentiumDir, 0755); err != nil {
		t.Fatal(err)
	}

	expected := "# Project Instructions\nRun tests with: go test ./..."
	if err := os.WriteFile(filepath.Join(agentiumDir, "AGENT.md"), []byte(expected), 0644); err != nil {
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
	agentiumDir := filepath.Join(tmpDir, ".agentium")
	if err := os.MkdirAll(agentiumDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(agentiumDir, "AGENT.md"), []byte(""), 0644); err != nil {
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
