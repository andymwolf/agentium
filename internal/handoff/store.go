package handoff

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Store manages structured phase outputs for a task.
// Unlike the memory store which accumulates signals, the handoff store
// maintains only the current state of each phase's output.
type Store struct {
	mu       sync.RWMutex
	filePath string

	// Per-task state, keyed by taskID (e.g., "issue:42")
	tasks map[string]*TaskHandoff
}

// TaskHandoff holds all phase outputs for a single task.
type TaskHandoff struct {
	IssueContext *IssueContext     `json:"issue_context,omitempty"`
	Plan         *PlanOutput       `json:"plan,omitempty"`
	Implement    *ImplementOutput  `json:"implement,omitempty"`
	Review       *ReviewOutput     `json:"review,omitempty"`
	Docs         *DocsOutput       `json:"docs,omitempty"`
	PRCreation   *PRCreationOutput `json:"pr_creation,omitempty"`
}

// StoreData is the on-disk representation of the handoff store.
type StoreData struct {
	Version string                   `json:"version"`
	Tasks   map[string]*TaskHandoff  `json:"tasks"`
}

// NewStore creates a new handoff store for the given work directory.
func NewStore(workDir string) *Store {
	return &Store{
		filePath: filepath.Join(workDir, ".agentium", "handoff.json"),
		tasks:    make(map[string]*TaskHandoff),
	}
}

// Load reads the handoff file from disk. If the file does not exist, the store
// starts empty without error.
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var data StoreData
	if err := json.Unmarshal(raw, &data); err != nil {
		// Invalid JSON - start fresh
		return nil
	}

	if data.Tasks != nil {
		s.tasks = data.Tasks
	}
	return nil
}

// Save writes the current handoff data to disk, creating the directory if needed.
func (s *Store) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data := StoreData{
		Version: "1",
		Tasks:   s.tasks,
	}

	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, raw, 0644)
}

// getOrCreateTask returns the TaskHandoff for the given taskID, creating it if needed.
func (s *Store) getOrCreateTask(taskID string) *TaskHandoff {
	if s.tasks[taskID] == nil {
		s.tasks[taskID] = &TaskHandoff{}
	}
	return s.tasks[taskID]
}

// SetIssueContext stores the issue context for a task.
func (s *Store) SetIssueContext(taskID string, ctx *IssueContext) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getOrCreateTask(taskID).IssueContext = ctx
}

// GetIssueContext retrieves the issue context for a task.
func (s *Store) GetIssueContext(taskID string) *IssueContext {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if task := s.tasks[taskID]; task != nil {
		return task.IssueContext
	}
	return nil
}

// SetPlanOutput stores the PLAN phase output for a task.
func (s *Store) SetPlanOutput(taskID string, output *PlanOutput) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getOrCreateTask(taskID).Plan = output
}

// GetPlanOutput retrieves the PLAN phase output for a task.
func (s *Store) GetPlanOutput(taskID string) *PlanOutput {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if task := s.tasks[taskID]; task != nil {
		return task.Plan
	}
	return nil
}

// SetImplementOutput stores the IMPLEMENT phase output for a task.
func (s *Store) SetImplementOutput(taskID string, output *ImplementOutput) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getOrCreateTask(taskID).Implement = output
}

// GetImplementOutput retrieves the IMPLEMENT phase output for a task.
func (s *Store) GetImplementOutput(taskID string) *ImplementOutput {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if task := s.tasks[taskID]; task != nil {
		return task.Implement
	}
	return nil
}

// SetReviewOutput stores the REVIEW phase output for a task.
func (s *Store) SetReviewOutput(taskID string, output *ReviewOutput) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getOrCreateTask(taskID).Review = output
}

// GetReviewOutput retrieves the REVIEW phase output for a task.
func (s *Store) GetReviewOutput(taskID string) *ReviewOutput {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if task := s.tasks[taskID]; task != nil {
		return task.Review
	}
	return nil
}

// SetDocsOutput stores the DOCS phase output for a task.
func (s *Store) SetDocsOutput(taskID string, output *DocsOutput) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getOrCreateTask(taskID).Docs = output
}

// GetDocsOutput retrieves the DOCS phase output for a task.
func (s *Store) GetDocsOutput(taskID string) *DocsOutput {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if task := s.tasks[taskID]; task != nil {
		return task.Docs
	}
	return nil
}

// SetPRCreationOutput stores the PR_CREATION phase output for a task.
func (s *Store) SetPRCreationOutput(taskID string, output *PRCreationOutput) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getOrCreateTask(taskID).PRCreation = output
}

// GetPRCreationOutput retrieves the PR_CREATION phase output for a task.
func (s *Store) GetPRCreationOutput(taskID string) *PRCreationOutput {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if task := s.tasks[taskID]; task != nil {
		return task.PRCreation
	}
	return nil
}

// ClearFromPhase clears all phase outputs from the specified phase onwards.
// Used when regression is needed (e.g., REVIEW finds fundamental issues requiring re-planning).
func (s *Store) ClearFromPhase(taskID string, phase string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task := s.tasks[taskID]
	if task == nil {
		return
	}

	// Clear phases in order from the specified phase onwards
	switch phase {
	case "PLAN":
		task.Plan = nil
		fallthrough
	case "IMPLEMENT":
		task.Implement = nil
		fallthrough
	case "REVIEW":
		task.Review = nil
		fallthrough
	case "DOCS":
		task.Docs = nil
		fallthrough
	case "PR_CREATION":
		task.PRCreation = nil
	}
}

// HasPlanOutput returns true if the task has a PLAN output stored.
func (s *Store) HasPlanOutput(taskID string) bool {
	return s.GetPlanOutput(taskID) != nil
}

// HasImplementOutput returns true if the task has an IMPLEMENT output stored.
func (s *Store) HasImplementOutput(taskID string) bool {
	return s.GetImplementOutput(taskID) != nil
}

// Summary returns a human-readable summary of what's stored for a task.
func (s *Store) Summary(taskID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	task := s.tasks[taskID]
	if task == nil {
		return fmt.Sprintf("Task %s: no handoff data", taskID)
	}

	phases := []string{}
	if task.IssueContext != nil {
		phases = append(phases, "IssueContext")
	}
	if task.Plan != nil {
		phases = append(phases, "Plan")
	}
	if task.Implement != nil {
		phases = append(phases, "Implement")
	}
	if task.Review != nil {
		phases = append(phases, "Review")
	}
	if task.Docs != nil {
		phases = append(phases, "Docs")
	}
	if task.PRCreation != nil {
		phases = append(phases, "PRCreation")
	}

	if len(phases) == 0 {
		return fmt.Sprintf("Task %s: empty", taskID)
	}
	return fmt.Sprintf("Task %s: %v", taskID, phases)
}
