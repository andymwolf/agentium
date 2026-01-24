package memory

import (
	"fmt"
	"strings"
)

// sectionOrder defines the priority of sections in the context output.
var sectionOrder = []SignalType{
	StepPending,
	KeyFact,
	Decision,
	Error,
	StepDone,
	FileModified,
}

var sectionHeaders = map[SignalType]string{
	StepPending:  "Pending Steps",
	KeyFact:      "Key Facts",
	Decision:     "Decisions",
	Error:        "Errors",
	StepDone:     "Completed Steps",
	FileModified: "Files Modified",
}

// BuildContext generates a budget-aware Markdown summary of the memory entries.
// It groups entries by type and renders sections in priority order, stopping
// when approaching the context budget limit.
func (s *Store) BuildContext() string {
	if len(s.data.Entries) == 0 {
		return ""
	}

	// Group entries by type
	groups := make(map[SignalType][]string)
	for _, e := range s.data.Entries {
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
