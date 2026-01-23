package prompt

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadSystemPrompt_FetchSuccess(t *testing.T) {
	expected := "# Test System Prompt\nFetched from remote."

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(expected))
	}))
	defer server.Close()

	result, err := LoadSystemPrompt(server.URL, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestLoadSystemPrompt_FetchFailsFallsBackToEmbedded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	result, err := LoadSystemPrompt(server.URL, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should fall back to embedded system.md
	if result == "" {
		t.Error("expected non-empty embedded fallback")
	}
	if result != embeddedSystemMD {
		t.Error("expected result to match embedded system.md content")
	}
}

func TestLoadSystemPrompt_UnreachableURLFallsBack(t *testing.T) {
	// Use a URL that will fail to connect
	result, err := LoadSystemPrompt("http://127.0.0.1:1/nonexistent", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == "" {
		t.Error("expected non-empty embedded fallback")
	}
	if result != embeddedSystemMD {
		t.Error("expected result to match embedded system.md content")
	}
}

func TestLoadSystemPrompt_EmptyURLUsesDefault(t *testing.T) {
	// With empty URL and the default URL likely unreachable in test,
	// it should fall back to embedded
	result, err := LoadSystemPrompt("", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// In CI/test environment the default URL may or may not be reachable,
	// but we should get a non-empty result either way
	if result == "" {
		t.Error("expected non-empty result")
	}
}

func TestLoadSystemPrompt_LargeResponseTruncated(t *testing.T) {
	// Create a response larger than maxPromptSize (1MB)
	largeContent := strings.Repeat("x", maxPromptSize+1000)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(largeContent))
	}))
	defer server.Close()

	result, err := LoadSystemPrompt(server.URL, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) > maxPromptSize {
		t.Errorf("expected result to be at most %d bytes, got %d", maxPromptSize, len(result))
	}
}

func TestLoadSystemPrompt_CustomTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("custom timeout test"))
	}))
	defer server.Close()

	result, err := LoadSystemPrompt(server.URL, 10*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "custom timeout test" {
		t.Errorf("got %q, want %q", result, "custom timeout test")
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

func TestEmbeddedSystemMD_NotEmpty(t *testing.T) {
	if embeddedSystemMD == "" {
		t.Error("embedded system.md should not be empty")
	}

	// Verify it contains expected content markers
	if !strings.Contains(embeddedSystemMD, "CRITICAL SAFETY CONSTRAINTS") {
		t.Error("embedded system.md missing CRITICAL SAFETY CONSTRAINTS section")
	}
	if !strings.Contains(embeddedSystemMD, "AGENTIUM_STATUS") {
		t.Error("embedded system.md missing AGENTIUM_STATUS section")
	}
	if !strings.Contains(embeddedSystemMD, "PROHIBITED ACTIONS") {
		t.Error("embedded system.md missing PROHIBITED ACTIONS section")
	}
}
