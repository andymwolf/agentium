package routing

import (
	"sort"
	"strings"
)

// ModelConfig specifies an adapter and model for a phase
type ModelConfig struct {
	Adapter   string `json:"adapter" yaml:"adapter" mapstructure:"adapter"`
	Model     string `json:"model" yaml:"model" mapstructure:"model"`
	Reasoning string `json:"reasoning,omitempty" yaml:"reasoning,omitempty" mapstructure:"reasoning"`
}

// ValidReasoningLevels is the set of recognized reasoning level values.
// For codex: minimal, low, medium, high, xhigh (passed as model_reasoning_effort config)
var ValidReasoningLevels = map[string]bool{
	"minimal": true,
	"low":     true,
	"medium":  true,
	"high":    true,
	"xhigh":   true,
}

// ValidReasoningLevelNames returns the sorted list of recognized reasoning levels.
func ValidReasoningLevelNames() []string {
	names := make([]string, 0, len(ValidReasoningLevels))
	for level := range ValidReasoningLevels {
		names = append(names, level)
	}
	sort.Strings(names)
	return names
}

// PhaseRouting maps phases to adapter+model configurations
type PhaseRouting struct {
	Default   ModelConfig            `json:"default" yaml:"default" mapstructure:"default"`
	Overrides map[string]ModelConfig `json:"overrides,omitempty" yaml:"overrides,omitempty" mapstructure:"overrides"`
}

// ValidPhases is the set of recognized task phase names.
var ValidPhases = map[string]bool{
	"PLAN":          true,
	"IMPLEMENT":     true,
	"DOCS":          true,
	"COMPLETE":      true,
	"BLOCKED":       true,
	"NOTHING_TO_DO": true,
	// Compound phase keys for reviewer (per-iteration review)
	"PLAN_REVIEW":      true,
	"IMPLEMENT_REVIEW": true,
	"DOCS_REVIEW":      true,
	// Compound phase keys for judge
	"JUDGE":           true,
	"PLAN_JUDGE":      true,
	"IMPLEMENT_JUDGE": true,
	"DOCS_JUDGE":      true,
}

// ValidPhaseNames returns the sorted list of recognized phase names.
func ValidPhaseNames() []string {
	names := make([]string, 0, len(ValidPhases))
	for phase := range ValidPhases {
		names = append(names, phase)
	}
	sort.Strings(names)
	return names
}

// ParseModelSpec parses an "adapter:model" colon-separated string into ModelConfig.
// The first colon is used as the delimiter: everything before it is the adapter name,
// everything after is the model ID. If no colon is present, the entire string is
// treated as the model with an empty adapter (uses the session's default adapter).
//
// Note: Model IDs that contain colons cannot be represented with this format.
// Known model IDs (e.g., "claude-opus-4-20250514") do not contain colons.
func ParseModelSpec(spec string) ModelConfig {
	parts := strings.SplitN(spec, ":", 2)
	if len(parts) == 2 {
		return ModelConfig{Adapter: parts[0], Model: parts[1]}
	}
	return ModelConfig{Model: spec}
}
