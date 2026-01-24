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
	now := time.Now()
	for _, sig := range signals {
		s.data.Entries = append(s.data.Entries, Entry{
			Type:      sig.Type,
			Content:   sig.Content,
			Iteration: iteration,
			TaskID:    taskID,
			Timestamp: now,
		})
	}
	s.resolvePending(signals)
	return s.prune()
}

// resolvePending removes STEP_PENDING entries whose content matches any
// incoming STEP_DONE signal, so completed steps don't linger as pending.
func (s *Store) resolvePending(signals []Signal) {
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
		if e.Type == StepPending && done[e.Content] {
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
