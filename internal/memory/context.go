package memory

import (
	"fmt"
	"strings"
)

// sectionOrder defines the priority of sections in the context output.
var sectionOrder = []SignalType{
	JudgeDirective,
	EvalFeedback,
	PhaseResult,
	StepPending,
	KeyFact,
	Decision,
	Error,
	StepDone,
	FileModified,
}

var sectionHeaders = map[SignalType]string{
	JudgeDirective: "Judge Directives",
	EvalFeedback:   "Evaluator Feedback",
	PhaseResult:    "Phase Results",
	StepPending:    "Pending Steps",
	KeyFact:        "Key Facts",
	Decision:       "Decisions",
	Error:          "Errors",
	StepDone:       "Completed Steps",
	FileModified:   "Files Modified",
}

// evalSectionOrder defines the priority of sections in the evaluator context output.
// Only includes eval-relevant types, excluding agent-internal signals.
var evalSectionOrder = []SignalType{
	JudgeDirective,
	EvalFeedback,
	PhaseResult,
}

// BuildEvalContext generates a budget-aware Markdown summary containing only
// evaluator-relevant entries (EvalFeedback and PhaseResult). This provides the
// judge with iteration history without agent-internal signals like StepPending
// or FileModified.
// If taskID is provided (non-empty), only entries for that task are included.
func (s *Store) BuildEvalContext(taskID string) string {
	if len(s.data.Entries) == 0 {
		return ""
	}

	// Group only eval-relevant entries by type, filtering by taskID if provided
	groups := make(map[SignalType][]string)
	for _, e := range s.data.Entries {
		// Skip entries from other tasks if taskID is specified
		if taskID != "" && e.TaskID != taskID {
			continue
		}
		if e.Type == EvalFeedback || e.Type == JudgeDirective || e.Type == PhaseResult {
			groups[e.Type] = append(groups[e.Type], fmt.Sprintf("[iter %d] %s", e.Iteration, e.Content))
		}
	}

	if len(groups) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Iteration History\n\n")
	used := sb.Len()

	for _, st := range evalSectionOrder {
		items, ok := groups[st]
		if !ok || len(items) == 0 {
			continue
		}

		header := fmt.Sprintf("### %s\n", sectionHeaders[st])
		section := header
		for _, item := range items {
			section += fmt.Sprintf("- %s\n", item)
		}
		section += "\n"

		if used+len(section) > s.contextBudget {
			break
		}
		sb.WriteString(section)
		used += len(section)
	}

	result := sb.String()
	if result == "## Iteration History\n\n" {
		return ""
	}
	return result
}

// BuildCurrentIterationEvalContext generates a budget-aware Markdown summary containing only
// the current phase iteration's EvalFeedback entries. This prevents the judge from seeing
// feedback from prior iterations that may have already been addressed.
// If taskID is provided (non-empty), only entries for that task are included.
// Respects the configured context budget to prevent overflowing model context.
func (s *Store) BuildCurrentIterationEvalContext(taskID string, phaseIteration int) string {
	if len(s.data.Entries) == 0 {
		return ""
	}

	// Collect only EvalFeedback from the current phase iteration
	var items []string
	for _, e := range s.data.Entries {
		// Skip entries from other tasks if taskID is specified
		if taskID != "" && e.TaskID != taskID {
			continue
		}
		if e.Type == EvalFeedback && e.PhaseIteration == phaseIteration {
			items = append(items, e.Content)
		}
	}

	if len(items) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Current Iteration Feedback\n\n")
	sb.WriteString("### Evaluator Feedback\n")
	used := sb.Len()

	for _, item := range items {
		line := fmt.Sprintf("- %s\n", item)
		// Check budget before adding item
		if used+len(line) > s.contextBudget {
			break
		}
		sb.WriteString(line)
		used += len(line)
	}
	sb.WriteString("\n")

	result := sb.String()
	// If only headers were written (no items fit), return empty
	if result == "## Current Iteration Feedback\n\n### Evaluator Feedback\n\n" {
		return ""
	}
	return result
}

// BuildContext generates a budget-aware Markdown summary of the memory entries.
// It groups entries by type and renders sections in priority order, stopping
// when approaching the context budget limit.
// If taskID is provided (non-empty), only entries for that task are included.
func (s *Store) BuildContext(taskID string) string {
	if len(s.data.Entries) == 0 {
		return ""
	}

	// Group entries by type, filtering by taskID if provided
	groups := make(map[SignalType][]string)
	for _, e := range s.data.Entries {
		// Skip entries from other tasks if taskID is specified
		if taskID != "" && e.TaskID != taskID {
			continue
		}
		groups[e.Type] = append(groups[e.Type], e.Content)
	}

	var sb strings.Builder
	sb.WriteString("## Memory from Previous Iterations\n\n")
	used := sb.Len()

	for _, st := range sectionOrder {
		items, ok := groups[st]
		if !ok || len(items) == 0 {
			continue
		}

		header := fmt.Sprintf("### %s\n", sectionHeaders[st])
		section := header
		for _, item := range items {
			section += fmt.Sprintf("- %s\n", item)
		}
		section += "\n"

		// Check budget before adding section
		if used+len(section) > s.contextBudget {
			break
		}
		sb.WriteString(section)
		used += len(section)
	}

	result := sb.String()
	if result == "## Memory from Previous Iterations\n\n" {
		// Only header was written — budget too small for any section
		return ""
	}
	return result
}
