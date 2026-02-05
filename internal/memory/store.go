package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Store manages the persistent memory entries.
type Store struct {
	filePath      string
	data          *Data
	maxEntries    int
	contextBudget int
}

// NewStore creates a new memory store for the given work directory.
func NewStore(workDir string, config Config) *Store {
	maxEntries := config.MaxEntries
	if maxEntries <= 0 {
		maxEntries = DefaultMaxEntries
	}
	contextBudget := config.ContextBudget
	if contextBudget <= 0 {
		contextBudget = DefaultContextBudget
	}
	return &Store{
		filePath:      filepath.Join(workDir, ".agentium", "memory.json"),
		data:          &Data{Version: "1", Entries: []Entry{}},
		maxEntries:    maxEntries,
		contextBudget: contextBudget,
	}
}

// Load reads the memory file from disk. If the file does not exist, the store
// starts empty without error.
func (s *Store) Load() error {
	raw, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var data Data
	if err := json.Unmarshal(raw, &data); err != nil {
		// Invalid JSON — start fresh
		return nil
	}
	s.data = &data
	return nil
}

// Save writes the current memory data to disk, creating the directory if needed.
func (s *Store) Save() error {
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, raw, 0644)
}

// Update appends new entries from the given signals and prunes if necessary.
// Returns the number of entries that were pruned (0 if no pruning occurred).
func (s *Store) Update(signals []Signal, iteration int, taskID string) int {
	return s.UpdateWithPhaseIteration(signals, iteration, 0, taskID)
}

// UpdateWithPhaseIteration appends new entries with both global and phase iteration tracking.
// phaseIteration is the within-phase iteration (1-indexed), used to scope feedback to specific iterations.
// Returns the number of entries that were pruned (0 if no pruning occurred).
func (s *Store) UpdateWithPhaseIteration(signals []Signal, iteration int, phaseIteration int, taskID string) int {
	now := time.Now()
	for _, sig := range signals {
		s.data.Entries = append(s.data.Entries, Entry{
			Type:           sig.Type,
			Content:        sig.Content,
			Iteration:      iteration,
			PhaseIteration: phaseIteration,
			TaskID:         taskID,
			Timestamp:      now,
		})
	}
	s.resolvePending(signals, taskID)
	return s.prune()
}

// resolvePending removes STEP_PENDING entries whose content matches any
// incoming STEP_DONE signal, so completed steps don't linger as pending.
// This is now task-scoped to prevent clearing unrelated pending steps.
func (s *Store) resolvePending(signals []Signal, taskID string) {
	done := make(map[string]bool)
	for _, sig := range signals {
		if sig.Type == StepDone {
			done[sig.Content] = true
		}
	}
	if len(done) == 0 {
		return
	}
	filtered := s.data.Entries[:0]
	for _, e := range s.data.Entries {
		// Only remove STEP_PENDING entries that match both:
		// 1. The content is marked as done
		// 2. The entry belongs to the same task
		if e.Type == StepPending && done[e.Content] && e.TaskID == taskID {
			continue
		}
		filtered = append(filtered, e)
	}
	s.data.Entries = filtered
}

// Entries returns the current list of memory entries.
func (s *Store) Entries() []Entry {
	return s.data.Entries
}

// ClearByType removes all entries matching the given signal type.
func (s *Store) ClearByType(signalType SignalType) {
	filtered := make([]Entry, 0, len(s.data.Entries))
	for _, e := range s.data.Entries {
		if e.Type != signalType {
			filtered = append(filtered, e)
		}
	}
	s.data.Entries = filtered
}

// prune drops the oldest entries when the store exceeds maxEntries.
// Returns the number of entries removed.
func (s *Store) prune() int {
	if len(s.data.Entries) <= s.maxEntries {
		return 0
	}
	excess := len(s.data.Entries) - s.maxEntries
	s.data.Entries = s.data.Entries[excess:]
	return excess
}

// GetPreviousIterationFeedback returns the EvalFeedback and JudgeDirective entries from the previous phase iteration.
// This allows the reviewer to see what feedback was given in iteration N-1 so it can verify
// whether the worker addressed that feedback, and allows the worker to see both the detailed
// reviewer analysis (EvalFeedback) and the required action items from the judge (JudgeDirective).
func (s *Store) GetPreviousIterationFeedback(taskID string, currentPhaseIteration int) []Entry {
	if currentPhaseIteration <= 1 {
		return nil
	}

	previousIteration := currentPhaseIteration - 1
	var result []Entry
	for _, e := range s.data.Entries {
		if taskID != "" && e.TaskID != taskID {
			continue
		}
		if (e.Type == EvalFeedback || e.Type == JudgeDirective) && e.PhaseIteration == previousIteration {
			result = append(result, e)
		}
	}
	return result
}
