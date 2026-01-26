package handoff

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Signal prefix for structured handoff output
const HandoffSignalPrefix = "AGENTIUM_HANDOFF:"

// handoffPattern matches AGENTIUM_HANDOFF: followed by JSON
// The JSON can span multiple lines, so we capture everything after the prefix
// until we find a complete JSON object.
var handoffPattern = regexp.MustCompile(`AGENTIUM_HANDOFF:\s*(\{[\s\S]*?\})(?:\s*$|\s*\n)`)

// ParsedHandoff contains the parsed handoff data and metadata about the parse.
type ParsedHandoff struct {
	Phase   string // Phase that produced this handoff (inferred from content)
	RawJSON string // The raw JSON string that was parsed
}

// ParseHandoffSignal extracts and parses an AGENTIUM_HANDOFF signal from agent output.
// Returns the raw JSON string and any error.
func ParseHandoffSignal(output string) (string, error) {
	// Look for the signal prefix
	idx := strings.Index(output, HandoffSignalPrefix)
	if idx == -1 {
		return "", fmt.Errorf("no AGENTIUM_HANDOFF signal found in output")
	}

	// Extract everything after the prefix
	remainder := output[idx+len(HandoffSignalPrefix):]
	remainder = strings.TrimSpace(remainder)

	// Find the JSON object - it should start with {
	if !strings.HasPrefix(remainder, "{") {
		return "", fmt.Errorf("AGENTIUM_HANDOFF signal not followed by JSON object")
	}

	// Parse the JSON to find where it ends
	jsonStr, err := extractJSONObject(remainder)
	if err != nil {
		return "", fmt.Errorf("failed to extract JSON from AGENTIUM_HANDOFF: %w", err)
	}

	return jsonStr, nil
}

// extractJSONObject extracts a complete JSON object from a string that starts with {.
// It handles nested objects and arrays properly.
func extractJSONObject(s string) (string, error) {
	if len(s) == 0 || s[0] != '{' {
		return "", fmt.Errorf("string does not start with {")
	}

	depth := 0
	inString := false
	escaped := false

	for i, c := range s {
		if escaped {
			escaped = false
			continue
		}

		if c == '\\' && inString {
			escaped = true
			continue
		}

		if c == '"' {
			inString = !inString
			continue
		}

		if inString {
			continue
		}

		if c == '{' {
			depth++
		} else if c == '}' {
			depth--
			if depth == 0 {
				return s[:i+1], nil
			}
		}
	}

	return "", fmt.Errorf("incomplete JSON object")
}

// ParsePlanOutput parses a PlanOutput from raw JSON.
func ParsePlanOutput(jsonStr string) (*PlanOutput, error) {
	var output PlanOutput
	if err := json.Unmarshal([]byte(jsonStr), &output); err != nil {
		return nil, fmt.Errorf("failed to parse PlanOutput: %w", err)
	}
	return &output, nil
}

// ParseImplementOutput parses an ImplementOutput from raw JSON.
func ParseImplementOutput(jsonStr string) (*ImplementOutput, error) {
	var output ImplementOutput
	if err := json.Unmarshal([]byte(jsonStr), &output); err != nil {
		return nil, fmt.Errorf("failed to parse ImplementOutput: %w", err)
	}
	return &output, nil
}

// ParseReviewOutput parses a ReviewOutput from raw JSON.
func ParseReviewOutput(jsonStr string) (*ReviewOutput, error) {
	var output ReviewOutput
	if err := json.Unmarshal([]byte(jsonStr), &output); err != nil {
		return nil, fmt.Errorf("failed to parse ReviewOutput: %w", err)
	}
	return &output, nil
}

// ParseDocsOutput parses a DocsOutput from raw JSON.
func ParseDocsOutput(jsonStr string) (*DocsOutput, error) {
	var output DocsOutput
	if err := json.Unmarshal([]byte(jsonStr), &output); err != nil {
		return nil, fmt.Errorf("failed to parse DocsOutput: %w", err)
	}
	return &output, nil
}

// ParsePRCreationOutput parses a PRCreationOutput from raw JSON.
func ParsePRCreationOutput(jsonStr string) (*PRCreationOutput, error) {
	var output PRCreationOutput
	if err := json.Unmarshal([]byte(jsonStr), &output); err != nil {
		return nil, fmt.Errorf("failed to parse PRCreationOutput: %w", err)
	}
	return &output, nil
}

// ParseAndStorePhaseOutput parses the AGENTIUM_HANDOFF signal from agent output
// and stores it in the handoff store for the given phase.
func ParseAndStorePhaseOutput(store *Store, taskID string, phase string, output string) error {
	jsonStr, err := ParseHandoffSignal(output)
	if err != nil {
		return err
	}

	switch phase {
	case "PLAN":
		parsed, err := ParsePlanOutput(jsonStr)
		if err != nil {
			return err
		}
		store.SetPlanOutput(taskID, parsed)

	case "IMPLEMENT":
		parsed, err := ParseImplementOutput(jsonStr)
		if err != nil {
			return err
		}
		store.SetImplementOutput(taskID, parsed)

	case "REVIEW":
		parsed, err := ParseReviewOutput(jsonStr)
		if err != nil {
			return err
		}
		store.SetReviewOutput(taskID, parsed)

	case "DOCS":
		parsed, err := ParseDocsOutput(jsonStr)
		if err != nil {
			return err
		}
		store.SetDocsOutput(taskID, parsed)

	case "PR_CREATION":
		parsed, err := ParsePRCreationOutput(jsonStr)
		if err != nil {
			return err
		}
		store.SetPRCreationOutput(taskID, parsed)

	default:
		return fmt.Errorf("unknown phase: %s", phase)
	}

	return nil
}

// HasHandoffSignal returns true if the output contains an AGENTIUM_HANDOFF signal.
func HasHandoffSignal(output string) bool {
	return strings.Contains(output, HandoffSignalPrefix)
}

// ExtractAllHandoffSignals finds all AGENTIUM_HANDOFF signals in the output.
// Useful when an agent emits multiple signals (though typically there should be one per phase).
func ExtractAllHandoffSignals(output string) []string {
	var signals []string
	remaining := output

	for {
		idx := strings.Index(remaining, HandoffSignalPrefix)
		if idx == -1 {
			break
		}

		signalStart := remaining[idx+len(HandoffSignalPrefix):]
		signalStart = strings.TrimSpace(signalStart)

		if strings.HasPrefix(signalStart, "{") {
			jsonStr, err := extractJSONObject(signalStart)
			if err == nil {
				signals = append(signals, jsonStr)
			}
		}

		// Move past this signal
		remaining = remaining[idx+len(HandoffSignalPrefix):]
	}

	return signals
}
