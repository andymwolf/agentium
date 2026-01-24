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
func (s *Store) Update(signals []Signal, iteration int, taskID string) {
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
	s.prune()
}

// Entries returns the current list of memory entries.
func (s *Store) Entries() []Entry {
	return s.data.Entries
}

// prune drops the oldest entries when the store exceeds maxEntries.
func (s *Store) prune() {
	if len(s.data.Entries) <= s.maxEntries {
		return
	}
	excess := len(s.data.Entries) - s.maxEntries
	s.data.Entries = s.data.Entries[excess:]
}
