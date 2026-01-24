package memory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewStore_Defaults(t *testing.T) {
	s := NewStore("/tmp/test", Config{})
	if s.maxEntries != DefaultMaxEntries {
		t.Errorf("expected maxEntries %d, got %d", DefaultMaxEntries, s.maxEntries)
	}
	if s.contextBudget != DefaultContextBudget {
		t.Errorf("expected contextBudget %d, got %d", DefaultContextBudget, s.contextBudget)
	}
	if s.filePath != "/tmp/test/.agentium/memory.json" {
		t.Errorf("unexpected filePath: %s", s.filePath)
	}
}

func TestNewStore_CustomConfig(t *testing.T) {
	s := NewStore("/work", Config{MaxEntries: 50, ContextBudget: 2000})
	if s.maxEntries != 50 {
		t.Errorf("expected maxEntries 50, got %d", s.maxEntries)
	}
	if s.contextBudget != 2000 {
		t.Errorf("expected contextBudget 2000, got %d", s.contextBudget)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	s := NewStore(t.TempDir(), Config{})
	if err := s.Load(); err != nil {
		t.Fatalf("Load on missing file should not error: %v", err)
	}
	if len(s.Entries()) != 0 {
		t.Errorf("expected empty entries, got %d", len(s.Entries()))
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	agentiumDir := filepath.Join(dir, ".agentium")
	os.MkdirAll(agentiumDir, 0755)
	os.WriteFile(filepath.Join(agentiumDir, "memory.json"), []byte("not json"), 0644)

	s := NewStore(dir, Config{})
	if err := s.Load(); err != nil {
		t.Fatalf("Load on invalid JSON should not error: %v", err)
	}
	if len(s.Entries()) != 0 {
		t.Errorf("expected empty entries after invalid JSON, got %d", len(s.Entries()))
	}
}

func TestSaveAndLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir, Config{})
	s.Update([]Signal{
		{Type: KeyFact, Content: "test fact"},
		{Type: Decision, Content: "test decision"},
	}, 1, "issue:42")

	if err := s.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	s2 := NewStore(dir, Config{})
	if err := s2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	entries := s2.Entries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Type != KeyFact || entries[0].Content != "test fact" {
		t.Errorf("unexpected entry[0]: %+v", entries[0])
	}
	if entries[1].Type != Decision || entries[1].Content != "test decision" {
		t.Errorf("unexpected entry[1]: %+v", entries[1])
	}
	if entries[0].Iteration != 1 || entries[0].TaskID != "issue:42" {
		t.Errorf("unexpected metadata in entry[0]: iter=%d, task=%s", entries[0].Iteration, entries[0].TaskID)
	}
}

func TestUpdate_AppendsEntries(t *testing.T) {
	s := NewStore(t.TempDir(), Config{})
	s.Update([]Signal{{Type: KeyFact, Content: "fact1"}}, 1, "issue:1")
	s.Update([]Signal{{Type: StepDone, Content: "step1"}}, 2, "issue:1")

	if len(s.Entries()) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(s.Entries()))
	}
}

func TestPrune(t *testing.T) {
	s := NewStore(t.TempDir(), Config{MaxEntries: 3})

	// Add 5 entries
	for i := 0; i < 5; i++ {
		s.Update([]Signal{{Type: KeyFact, Content: "fact"}}, i, "issue:1")
	}

	entries := s.Entries()
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries after prune, got %d", len(entries))
	}
	// Should keep the last 3 (iterations 2, 3, 4)
	if entries[0].Iteration != 2 {
		t.Errorf("expected oldest remaining entry to have iteration 2, got %d", entries[0].Iteration)
	}
}

func TestResolvePending_MatchingStepDone(t *testing.T) {
	s := NewStore(t.TempDir(), Config{})
	s.Update([]Signal{{Type: StepPending, Content: "write tests"}}, 1, "issue:1")
	s.Update([]Signal{{Type: StepPending, Content: "add logging"}}, 1, "issue:1")

	if len(s.Entries()) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(s.Entries()))
	}

	// Complete one of the pending steps
	s.Update([]Signal{{Type: StepDone, Content: "write tests"}}, 2, "issue:1")

	entries := s.Entries()
	// Should have: "add logging" (STEP_PENDING) + "write tests" (STEP_DONE)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries after resolve, got %d", len(entries))
	}
	for _, e := range entries {
		if e.Type == StepPending && e.Content == "write tests" {
			t.Error("STEP_PENDING 'write tests' should have been resolved")
		}
	}
}

func TestResolvePending_NoMatchLeavesPending(t *testing.T) {
	s := NewStore(t.TempDir(), Config{})
	s.Update([]Signal{{Type: StepPending, Content: "write tests"}}, 1, "issue:1")

	// STEP_DONE with different content should not resolve the pending
	s.Update([]Signal{{Type: StepDone, Content: "something else"}}, 2, "issue:1")

	entries := s.Entries()
	hasPending := false
	for _, e := range entries {
		if e.Type == StepPending && e.Content == "write tests" {
			hasPending = true
		}
	}
	if !hasPending {
		t.Error("STEP_PENDING 'write tests' should still exist")
	}
}

func TestResolvePending_SameBatchResolution(t *testing.T) {
	s := NewStore(t.TempDir(), Config{})
	s.Update([]Signal{{Type: StepPending, Content: "deploy"}}, 1, "issue:1")

	// Both a new pending and its done in the same signal batch
	s.Update([]Signal{
		{Type: StepPending, Content: "run migrations"},
		{Type: StepDone, Content: "deploy"},
	}, 2, "issue:1")

	entries := s.Entries()
	for _, e := range entries {
		if e.Type == StepPending && e.Content == "deploy" {
			t.Error("STEP_PENDING 'deploy' should have been resolved")
		}
	}
	// "run migrations" pending should remain (no matching STEP_DONE)
	hasMigrations := false
	for _, e := range entries {
		if e.Type == StepPending && e.Content == "run migrations" {
			hasMigrations = true
		}
	}
	if !hasMigrations {
		t.Error("STEP_PENDING 'run migrations' should still exist")
	}
}

func TestLoad_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	agentiumDir := filepath.Join(dir, ".agentium")
	os.MkdirAll(agentiumDir, 0755)

	data := `{"version":"1","entries":[{"type":"KEY_FACT","content":"loaded","iteration":5,"task_id":"issue:10","timestamp":"2024-01-01T00:00:00Z"}]}`
	os.WriteFile(filepath.Join(agentiumDir, "memory.json"), []byte(data), 0644)

	s := NewStore(dir, Config{})
	if err := s.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	entries := s.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Content != "loaded" {
		t.Errorf("expected content 'loaded', got %q", entries[0].Content)
	}
}
