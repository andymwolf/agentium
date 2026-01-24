package routing

import "strings"

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

// ParseModelSpec parses "adapter:model" colon-separated string into ModelConfig.
// If no colon, treats the whole string as the model with empty adapter (use default).
func ParseModelSpec(spec string) ModelConfig {
	parts := strings.SplitN(spec, ":", 2)
	if len(parts) == 2 {
		return ModelConfig{Adapter: parts[0], Model: parts[1]}
	}
	return ModelConfig{Model: spec}
}
