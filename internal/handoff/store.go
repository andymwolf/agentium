package handoff

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Store manages structured handoff data between phases.
// Unlike the memory store which accumulates signals, this store
// maintains typed phase outputs that replace previous outputs.
type Store struct {
	mu       sync.RWMutex
	filePath string
	data     map[string]*TaskHandoffs // taskID -> handoffs
}

// TaskHandoffs holds all handoff data for a single task.
type TaskHandoffs struct {
	TaskID   string         `json:"task_id"`
	Issue    *IssueContext  `json:"issue,omitempty"`
	Handoffs []*HandoffData `json:"handoffs"`
}

// NewStore creates a new handoff store with persistence at the given path.
func NewStore(workDir string) (*Store, error) {
	filePath := filepath.Join(workDir, ".agentium", "handoffs.json")

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create handoff directory: %w", err)
	}

	s := &Store{
		filePath: filePath,
		data:     make(map[string]*TaskHandoffs),
	}

	// Load existing data if present
	if err := s.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load handoff store: %w", err)
	}

	return s, nil
}

// load reads handoff data from disk.
func (s *Store) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}

	var loaded map[string]*TaskHandoffs
	if err := json.Unmarshal(data, &loaded); err != nil {
		return fmt.Errorf("failed to parse handoff file: %w", err)
	}

	s.data = loaded
	return nil
}

// Save persists handoff data to disk.
func (s *Store) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal handoff data: %w", err)
	}

	if err := os.WriteFile(s.filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write handoff file: %w", err)
	}

	return nil
}

// SetIssueContext stores the issue context for a task.
// This should be called once at task start.
func (s *Store) SetIssueContext(taskID string, issue *IssueContext) {
	s.mu.Lock()
	defer s.mu.Unlock()

	th := s.getOrCreateTask(taskID)
	th.Issue = issue
}

// GetIssueContext retrieves the issue context for a task.
func (s *Store) GetIssueContext(taskID string) *IssueContext {
	s.mu.RLock()
	defer s.mu.RUnlock()

	th, ok := s.data[taskID]
	if !ok || th.Issue == nil {
		return nil
	}
	return th.Issue
}

// StorePhaseOutput stores the output from a completed phase.
// It replaces any previous output for the same phase.
func (s *Store) StorePhaseOutput(taskID string, phase Phase, iteration int, output interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	th := s.getOrCreateTask(taskID)

	// Create handoff data envelope
	hd := &HandoffData{
		TaskID:    taskID,
		Phase:     phase,
		Timestamp: time.Now(),
		Iteration: iteration,
	}

	// Assign the appropriate typed output
	switch v := output.(type) {
	case *PlanOutput:
		hd.PlanOutput = v
	case *ImplementOutput:
		hd.ImplementOutput = v
	case *ReviewOutput:
		hd.ReviewOutput = v
	case *DocsOutput:
		hd.DocsOutput = v
	default:
		return fmt.Errorf("unknown output type for phase %s: %T", phase, output)
	}

	// Remove any existing handoff for this phase (replace semantics)
	filtered := make([]*HandoffData, 0, len(th.Handoffs))
	for _, h := range th.Handoffs {
		if h.Phase != phase {
			filtered = append(filtered, h)
		}
	}
	th.Handoffs = append(filtered, hd)

	return nil
}

// GetPhaseOutput retrieves the most recent output for a phase.
func (s *Store) GetPhaseOutput(taskID string, phase Phase) *HandoffData {
	s.mu.RLock()
	defer s.mu.RUnlock()

	th, ok := s.data[taskID]
	if !ok {
		return nil
	}

	for _, h := range th.Handoffs {
		if h.Phase == phase {
			return h
		}
	}
	return nil
}

// GetPlanOutput is a convenience method to get typed plan output.
func (s *Store) GetPlanOutput(taskID string) *PlanOutput {
	hd := s.GetPhaseOutput(taskID, PhasePlan)
	if hd == nil {
		return nil
	}
	return hd.PlanOutput
}

// GetImplementOutput is a convenience method to get typed implement output.
func (s *Store) GetImplementOutput(taskID string) *ImplementOutput {
	hd := s.GetPhaseOutput(taskID, PhaseImplement)
	if hd == nil {
		return nil
	}
	return hd.ImplementOutput
}

// GetDocsOutput is a convenience method to get typed docs output.
func (s *Store) GetDocsOutput(taskID string) *DocsOutput {
	hd := s.GetPhaseOutput(taskID, PhaseDocs)
	if hd == nil {
		return nil
	}
	return hd.DocsOutput
}

// ClearFromPhase clears all handoff data from the specified phase onwards.
func (s *Store) ClearFromPhase(taskID string, phase Phase) {
	s.mu.Lock()
	defer s.mu.Unlock()

	th, ok := s.data[taskID]
	if !ok {
		return
	}

	phaseOrder := map[Phase]int{
		PhasePlan:      0,
		PhaseImplement: 1,
		PhaseDocs:      2,
	}

	targetOrder := phaseOrder[phase]

	// Keep only handoffs from phases before the target
	filtered := make([]*HandoffData, 0, len(th.Handoffs))
	for _, h := range th.Handoffs {
		if order, ok := phaseOrder[h.Phase]; ok && order < targetOrder {
			filtered = append(filtered, h)
		}
	}
	th.Handoffs = filtered
}

// ClearTask removes all handoff data for a task.
func (s *Store) ClearTask(taskID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.data, taskID)
}

// HasPhaseOutput checks if output exists for a phase.
func (s *Store) HasPhaseOutput(taskID string, phase Phase) bool {
	return s.GetPhaseOutput(taskID, phase) != nil
}

// getOrCreateTask returns the TaskHandoffs for a taskID, creating if needed.
// Must be called with lock held.
func (s *Store) getOrCreateTask(taskID string) *TaskHandoffs {
	th, ok := s.data[taskID]
	if !ok {
		th = &TaskHandoffs{
			TaskID:   taskID,
			Handoffs: make([]*HandoffData, 0),
		}
		s.data[taskID] = th
	}
	return th
}
