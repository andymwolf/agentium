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
	_ = os.MkdirAll(agentiumDir, 0755)
	_ = os.WriteFile(filepath.Join(agentiumDir, "memory.json"), []byte("not json"), 0644)

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

func TestClearByType(t *testing.T) {
	s := NewStore(t.TempDir(), Config{})
	s.Update([]Signal{
		{Type: KeyFact, Content: "fact1"},
		{Type: EvalFeedback, Content: "fix the nil pointer"},
		{Type: KeyFact, Content: "fact2"},
		{Type: EvalFeedback, Content: "add error handling"},
		{Type: Decision, Content: "use JWT"},
	}, 1, "issue:42")

	if len(s.Entries()) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(s.Entries()))
	}

	s.ClearByType(EvalFeedback)

	entries := s.Entries()
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries after ClearByType, got %d", len(entries))
	}

	for _, e := range entries {
		if e.Type == EvalFeedback {
			t.Error("found EvalFeedback entry after ClearByType")
		}
	}
}

func TestClearByType_NoMatch(t *testing.T) {
	s := NewStore(t.TempDir(), Config{})
	s.Update([]Signal{
		{Type: KeyFact, Content: "fact1"},
		{Type: Decision, Content: "decision1"},
	}, 1, "issue:1")

	s.ClearByType(EvalFeedback) // No EvalFeedback entries exist

	if len(s.Entries()) != 2 {
		t.Errorf("expected 2 entries (unchanged), got %d", len(s.Entries()))
	}
}

func TestClearByType_AllMatch(t *testing.T) {
	s := NewStore(t.TempDir(), Config{})
	s.Update([]Signal{
		{Type: EvalFeedback, Content: "feedback1"},
		{Type: EvalFeedback, Content: "feedback2"},
	}, 1, "issue:1")

	s.ClearByType(EvalFeedback)

	if len(s.Entries()) != 0 {
		t.Errorf("expected 0 entries after clearing all, got %d", len(s.Entries()))
	}
}

func TestLoad_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	agentiumDir := filepath.Join(dir, ".agentium")
	_ = os.MkdirAll(agentiumDir, 0755)

	data := `{"version":"1","entries":[{"type":"KEY_FACT","content":"loaded","iteration":5,"task_id":"issue:10","timestamp":"2024-01-01T00:00:00Z"}]}`
	_ = os.WriteFile(filepath.Join(agentiumDir, "memory.json"), []byte(data), 0644)

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

func TestResolvePending_TaskScoped(t *testing.T) {
	s := NewStore(t.TempDir(), Config{})

	// Add pending steps for different tasks
	s.Update([]Signal{{Type: StepPending, Content: "write tests"}}, 1, "issue:123")
	s.Update([]Signal{{Type: StepPending, Content: "write tests"}}, 1, "issue:456")
	s.Update([]Signal{{Type: StepPending, Content: "add docs"}}, 1, "issue:123")

	if len(s.Entries()) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(s.Entries()))
	}

	// Complete "write tests" for task issue:123 only
	s.Update([]Signal{{Type: StepDone, Content: "write tests"}}, 2, "issue:123")

	entries := s.Entries()
	// Should have 3 entries:
	// 1. "write tests" STEP_PENDING for issue:456 (not resolved - different task)
	// 2. "add docs" STEP_PENDING for issue:123 (not resolved - different content)
	// 3. "write tests" STEP_DONE for issue:123 (the new entry)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries after resolve, got %d", len(entries))
	}

	// Verify issue:456's "write tests" is still pending
	foundTask456Pending := false
	for _, e := range entries {
		if e.Type == StepPending && e.Content == "write tests" && e.TaskID == "issue:456" {
			foundTask456Pending = true
			break
		}
	}
	if !foundTask456Pending {
		t.Error("STEP_PENDING 'write tests' for issue:456 should not have been resolved")
	}

	// Verify issue:123's "write tests" is no longer pending
	for _, e := range entries {
		if e.Type == StepPending && e.Content == "write tests" && e.TaskID == "issue:123" {
			t.Error("STEP_PENDING 'write tests' for issue:123 should have been resolved")
		}
	}
}

func TestUpdateWithPhaseIteration_SetsPhaseIteration(t *testing.T) {
	s := NewStore(t.TempDir(), Config{})
	s.UpdateWithPhaseIteration([]Signal{
		{Type: EvalFeedback, Content: "feedback1"},
	}, 5, 2, "issue:42")

	entries := s.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Iteration != 5 {
		t.Errorf("expected Iteration 5, got %d", entries[0].Iteration)
	}
	if entries[0].PhaseIteration != 2 {
		t.Errorf("expected PhaseIteration 2, got %d", entries[0].PhaseIteration)
	}
}

func TestUpdateWithPhaseIteration_DefaultsToZero(t *testing.T) {
	s := NewStore(t.TempDir(), Config{})
	// Using Update (which calls UpdateWithPhaseIteration with 0)
	s.Update([]Signal{
		{Type: EvalFeedback, Content: "feedback"},
	}, 1, "issue:42")

	entries := s.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].PhaseIteration != 0 {
		t.Errorf("expected PhaseIteration 0, got %d", entries[0].PhaseIteration)
	}
}

func TestGetPreviousIterationFeedback_ReturnsEmpty_WhenFirstIteration(t *testing.T) {
	s := NewStore(t.TempDir(), Config{})
	s.UpdateWithPhaseIteration([]Signal{
		{Type: EvalFeedback, Content: "feedback"},
	}, 1, 1, "issue:42")

	result := s.GetPreviousIterationFeedback("issue:42", 1)
	if len(result) != 0 {
		t.Errorf("expected empty result for first iteration, got %d entries", len(result))
	}
}

func TestGetPreviousIterationFeedback_ReturnsPreviousIteration(t *testing.T) {
	s := NewStore(t.TempDir(), Config{})
	s.UpdateWithPhaseIteration([]Signal{
		{Type: EvalFeedback, Content: "iter1 feedback"},
	}, 1, 1, "issue:42")
	s.UpdateWithPhaseIteration([]Signal{
		{Type: EvalFeedback, Content: "iter2 feedback"},
	}, 2, 2, "issue:42")
	s.UpdateWithPhaseIteration([]Signal{
		{Type: EvalFeedback, Content: "iter3 feedback"},
	}, 3, 3, "issue:42")

	// Request previous iteration for iteration 2 (should return iteration 1)
	result := s.GetPreviousIterationFeedback("issue:42", 2)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
	if result[0].Content != "iter1 feedback" {
		t.Errorf("expected 'iter1 feedback', got %q", result[0].Content)
	}

	// Request previous iteration for iteration 3 (should return iteration 2)
	result = s.GetPreviousIterationFeedback("issue:42", 3)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
	if result[0].Content != "iter2 feedback" {
		t.Errorf("expected 'iter2 feedback', got %q", result[0].Content)
	}
}

func TestGetPreviousIterationFeedback_FiltersByTaskID(t *testing.T) {
	s := NewStore(t.TempDir(), Config{})
	s.UpdateWithPhaseIteration([]Signal{
		{Type: EvalFeedback, Content: "task1 feedback"},
	}, 1, 1, "issue:123")
	s.UpdateWithPhaseIteration([]Signal{
		{Type: EvalFeedback, Content: "task2 feedback"},
	}, 1, 1, "issue:456")

	// Request previous iteration for task1, iteration 2
	result := s.GetPreviousIterationFeedback("issue:123", 2)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
	if result[0].Content != "task1 feedback" {
		t.Errorf("expected 'task1 feedback', got %q", result[0].Content)
	}
}

func TestGetPreviousIterationFeedback_OnlyReturnsFeedbackTypes(t *testing.T) {
	s := NewStore(t.TempDir(), Config{})
	s.UpdateWithPhaseIteration([]Signal{
		{Type: EvalFeedback, Content: "eval feedback"},
		{Type: JudgeDirective, Content: "judge directive"},
		{Type: PhaseResult, Content: "phase result"},
		{Type: KeyFact, Content: "key fact"},
	}, 1, 1, "issue:42")

	result := s.GetPreviousIterationFeedback("issue:42", 2)
	if len(result) != 2 {
		t.Fatalf("expected 2 entries (EvalFeedback + JudgeDirective), got %d", len(result))
	}
	// Check that both feedback types are returned
	hasEval := false
	hasJudge := false
	for _, e := range result {
		if e.Type == EvalFeedback {
			hasEval = true
		}
		if e.Type == JudgeDirective {
			hasJudge = true
		}
	}
	if !hasEval {
		t.Error("expected EvalFeedback in result")
	}
	if !hasJudge {
		t.Error("expected JudgeDirective in result")
	}
}

func TestGetPreviousIterationFeedback_MultipleFeedbackEntries(t *testing.T) {
	s := NewStore(t.TempDir(), Config{})
	s.UpdateWithPhaseIteration([]Signal{
		{Type: EvalFeedback, Content: "feedback A"},
		{Type: EvalFeedback, Content: "feedback B"},
	}, 1, 1, "issue:42")

	result := s.GetPreviousIterationFeedback("issue:42", 2)
	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}
}
