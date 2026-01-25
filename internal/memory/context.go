package memory

import (
	"fmt"
	"strings"
)

// sectionOrder defines the priority of sections in the context output.
var sectionOrder = []SignalType{
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
	EvalFeedback: "Evaluator Feedback",
	PhaseResult:  "Phase Results",
	StepPending:  "Pending Steps",
	KeyFact:      "Key Facts",
	Decision:     "Decisions",
	Error:        "Errors",
	StepDone:     "Completed Steps",
	FileModified: "Files Modified",
}

// evalSectionOrder defines the priority of sections in the evaluator context output.
// Only includes eval-relevant types, excluding agent-internal signals.
var evalSectionOrder = []SignalType{
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
		if e.Type == EvalFeedback || e.Type == PhaseResult {
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
