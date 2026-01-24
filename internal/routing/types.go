package routing

import (
	"sort"
	"strings"
)

// ModelConfig specifies an adapter and model for a phase
type ModelConfig struct {
	Adapter string `json:"adapter" yaml:"adapter" mapstructure:"adapter"`
	Model   string `json:"model" yaml:"model" mapstructure:"model"`
}

// PhaseRouting maps phases to adapter+model configurations
type PhaseRouting struct {
	Default   ModelConfig            `json:"default" yaml:"default" mapstructure:"default"`
	Overrides map[string]ModelConfig `json:"overrides,omitempty" yaml:"overrides,omitempty" mapstructure:"overrides"`
}

// ValidPhases is the set of recognized task phase names.
var ValidPhases = map[string]bool{
	"PLAN":         true,
	"IMPLEMENT":    true,
	"TEST":         true,
	"PR_CREATION":  true,
	"REVIEW":       true,
	"DOCS":         true,
	"EVALUATE":     true,
	"COMPLETE":     true,
	"BLOCKED":      true,
	"NOTHING_TO_DO": true,
	"ANALYZE":      true,
	"PUSH":         true,
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
