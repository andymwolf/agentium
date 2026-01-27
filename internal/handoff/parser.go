package handoff

import (
	"encoding/json"
	"fmt"
	"strings"
)

// SignalPrefix is the prefix for handoff signals in agent output.
const SignalPrefix = "AGENTIUM_HANDOFF:"

// Parser extracts and parses AGENTIUM_HANDOFF signals from agent output.
type Parser struct{}

// NewParser creates a new handoff parser.
func NewParser() *Parser {
	return &Parser{}
}

// ParseOutput extracts handoff data from agent stdout/stderr combined output.
// Returns the parsed output for the specified phase, or an error if not found/invalid.
func (p *Parser) ParseOutput(output string, phase Phase) (interface{}, error) {
	// Try to find the handoff signal
	jsonStr, err := p.extractJSON(output)
	if err != nil {
		return nil, err
	}

	// Parse based on expected phase
	switch phase {
	case PhasePlan:
		return p.parsePlanOutput(jsonStr)
	case PhaseImplement:
		return p.parseImplementOutput(jsonStr)
	case PhaseDocs:
		return p.parseDocsOutput(jsonStr)
	case PhasePRCreation:
		return p.parsePRCreationOutput(jsonStr)
	default:
		return nil, fmt.Errorf("unknown phase: %s", phase)
	}
}

// extractJSON finds and extracts the JSON payload from the handoff signal.
func (p *Parser) extractJSON(output string) (string, error) {
	// Use balanced brace extraction which handles multiline JSON correctly
	idx := strings.Index(output, SignalPrefix)
	if idx == -1 {
		return "", fmt.Errorf("no AGENTIUM_HANDOFF signal found in output")
	}

	// Find the start of JSON after the prefix
	jsonStart := idx + len(SignalPrefix)
	for jsonStart < len(output) && (output[jsonStart] == ' ' || output[jsonStart] == '\t' || output[jsonStart] == '\n' || output[jsonStart] == '\r') {
		jsonStart++
	}

	if jsonStart >= len(output) || output[jsonStart] != '{' {
		return "", fmt.Errorf("AGENTIUM_HANDOFF signal found but no JSON object follows")
	}

	// Extract balanced JSON object
	jsonStr, err := extractBalancedJSON(output[jsonStart:])
	if err != nil {
		return "", fmt.Errorf("failed to extract JSON from handoff: %w", err)
	}

	return jsonStr, nil
}

// extractBalancedJSON extracts a balanced JSON object from the start of a string.
func extractBalancedJSON(s string) (string, error) {
	if len(s) == 0 || s[0] != '{' {
		return "", fmt.Errorf("string does not start with '{'")
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

		switch c {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[:i+1], nil
			}
		}
	}

	return "", fmt.Errorf("unbalanced JSON object")
}

// parsePlanOutput parses PLAN phase output.
func (p *Parser) parsePlanOutput(jsonStr string) (*PlanOutput, error) {
	var output PlanOutput
	if err := json.Unmarshal([]byte(jsonStr), &output); err != nil {
		return nil, fmt.Errorf("failed to parse PlanOutput: %w", err)
	}
	return &output, nil
}

// parseImplementOutput parses IMPLEMENT phase output.
func (p *Parser) parseImplementOutput(jsonStr string) (*ImplementOutput, error) {
	var output ImplementOutput
	if err := json.Unmarshal([]byte(jsonStr), &output); err != nil {
		return nil, fmt.Errorf("failed to parse ImplementOutput: %w", err)
	}
	return &output, nil
}

// parseReviewOutput parses REVIEW phase output.
func (p *Parser) parseReviewOutput(jsonStr string) (*ReviewOutput, error) {
	var output ReviewOutput
	if err := json.Unmarshal([]byte(jsonStr), &output); err != nil {
		return nil, fmt.Errorf("failed to parse ReviewOutput: %w", err)
	}
	return &output, nil
}

// parseDocsOutput parses DOCS phase output.
func (p *Parser) parseDocsOutput(jsonStr string) (*DocsOutput, error) {
	var output DocsOutput
	if err := json.Unmarshal([]byte(jsonStr), &output); err != nil {
		return nil, fmt.Errorf("failed to parse DocsOutput: %w", err)
	}
	return &output, nil
}

// parsePRCreationOutput parses PR_CREATION phase output.
func (p *Parser) parsePRCreationOutput(jsonStr string) (*PRCreationOutput, error) {
	var output PRCreationOutput
	if err := json.Unmarshal([]byte(jsonStr), &output); err != nil {
		return nil, fmt.Errorf("failed to parse PRCreationOutput: %w", err)
	}
	return &output, nil
}

// HasHandoffSignal checks if output contains an AGENTIUM_HANDOFF signal.
func (p *Parser) HasHandoffSignal(output string) bool {
	return strings.Contains(output, SignalPrefix)
}

// ParseAny attempts to parse any handoff output, returning the phase and data.
// Useful when the phase is not known ahead of time.
func (p *Parser) ParseAny(output string) (Phase, interface{}, error) {
	jsonStr, err := p.extractJSON(output)
	if err != nil {
		return "", nil, err
	}

	// Try each phase type in order
	phases := []struct {
		phase  Phase
		parser func(string) (interface{}, error)
	}{
		{PhasePlan, func(s string) (interface{}, error) { return p.parsePlanOutput(s) }},
		{PhaseImplement, func(s string) (interface{}, error) { return p.parseImplementOutput(s) }},
		{PhaseDocs, func(s string) (interface{}, error) { return p.parseDocsOutput(s) }},
		{PhasePRCreation, func(s string) (interface{}, error) { return p.parsePRCreationOutput(s) }},
	}

	// We can't reliably distinguish between types just from JSON structure,
	// so we try to parse as a generic map and look for distinguishing fields
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return "", nil, fmt.Errorf("failed to parse handoff JSON: %w", err)
	}

	// Distinguish by unique fields - check most unique first
	// PR_CREATION: pr_number is unique
	if _, ok := raw["pr_number"]; ok {
		out, err := p.parsePRCreationOutput(jsonStr)
		return PhasePRCreation, out, err
	}
	// PLAN: implementation_steps is unique
	if _, ok := raw["implementation_steps"]; ok {
		out, err := p.parsePlanOutput(jsonStr)
		return PhasePlan, out, err
	}
	// IMPLEMENT: branch_name with commits/files_changed (not pr_number)
	if _, ok := raw["branch_name"]; ok {
		out, err := p.parseImplementOutput(jsonStr)
		return PhaseImplement, out, err
	}
	// DOCS: docs_updated is unique
	if _, ok := raw["docs_updated"]; ok {
		out, err := p.parseDocsOutput(jsonStr)
		return PhaseDocs, out, err
	}

	// Fallback: try each parser
	for _, ph := range phases {
		if out, err := ph.parser(jsonStr); err == nil {
			return ph.phase, out, nil
		}
	}

	return "", nil, fmt.Errorf("unable to determine phase from handoff data")
}
