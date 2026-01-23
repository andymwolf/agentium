package prompt

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSystemPrompt_FetchSuccess(t *testing.T) {
	expected := "# Test System Prompt\nFetched from remote."

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(expected))
	}))
	defer server.Close()

	result, err := LoadSystemPrompt(server.URL)
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

	result, err := LoadSystemPrompt(server.URL)
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
	result, err := LoadSystemPrompt("http://127.0.0.1:1/nonexistent")
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
	result, err := LoadSystemPrompt("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// In CI/test environment the default URL may or may not be reachable,
	// but we should get a non-empty result either way
	if result == "" {
		t.Error("expected non-empty result")
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
	if !contains(embeddedSystemMD, "CRITICAL SAFETY CONSTRAINTS") {
		t.Error("embedded system.md missing CRITICAL SAFETY CONSTRAINTS section")
	}
	if !contains(embeddedSystemMD, "AGENTIUM_STATUS") {
		t.Error("embedded system.md missing AGENTIUM_STATUS section")
	}
	if !contains(embeddedSystemMD, "PROHIBITED ACTIONS") {
		t.Error("embedded system.md missing PROHIBITED ACTIONS section")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
