package memory

import "time"

// SignalType represents the type of memory signal emitted by an agent.
type SignalType string

const (
	KeyFact        SignalType = "KEY_FACT"
	Decision       SignalType = "DECISION"
	StepDone       SignalType = "STEP_DONE"
	StepPending    SignalType = "STEP_PENDING"
	FileModified   SignalType = "FILE_MODIFIED"
	Error          SignalType = "ERROR"
	EvalFeedback   SignalType = "EVAL_FEEDBACK"
	JudgeDirective SignalType = "JUDGE_DIRECTIVE"
	PhaseResult    SignalType = "PHASE_RESULT"
)

// Signal is a parsed memory signal extracted from agent output.
type Signal struct {
	Type    SignalType
	Content string
}

// Entry is a single persisted memory entry.
type Entry struct {
	Type           SignalType `json:"type"`
	Content        string     `json:"content"`
	Iteration      int        `json:"iteration"`       // Global iteration across all phases
	PhaseIteration int        `json:"phase_iteration"` // Within-phase iteration (1-indexed)
	TaskID         string     `json:"task_id"`
	Timestamp      time.Time  `json:"timestamp"`
}

// Data is the on-disk representation of the memory store.
type Data struct {
	Version string  `json:"version"`
	Entries []Entry `json:"entries"`
}

// Config holds memory feature configuration.
type Config struct {
	Enabled       bool
	MaxEntries    int
	ContextBudget int
}

const (
	DefaultMaxEntries    = 100
	DefaultContextBudget = 3000
)
