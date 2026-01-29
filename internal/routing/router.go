package routing

import "sort"

// Router resolves the adapter and model to use for a given phase.
type Router struct {
	routing *PhaseRouting
}

// NewRouter creates a router. Nil-safe: nil routing returns a no-op router.
func NewRouter(routing *PhaseRouting) *Router {
	return &Router{routing: routing}
}

// ModelForPhase returns the ModelConfig for the given phase.
// Returns override if one exists, otherwise returns Default.
func (r *Router) ModelForPhase(phase string) ModelConfig {
	if r.routing == nil {
		return ModelConfig{}
	}
	if r.routing.Overrides != nil {
		if cfg, ok := r.routing.Overrides[phase]; ok {
			return cfg
		}
	}
	return r.routing.Default
}

// IsConfigured returns true if the router has usable routing config
// (non-nil with a non-empty default adapter or model).
func (r *Router) IsConfigured() bool {
	if r.routing == nil {
		return false
	}
	return r.routing.Default.Adapter != "" || r.routing.Default.Model != "" || len(r.routing.Overrides) > 0
}

// Adapters returns the set of unique adapter names referenced in the config,
// sorted for deterministic ordering.
func (r *Router) Adapters() []string {
	if r.routing == nil {
		return nil
	}

	seen := make(map[string]bool)
	if r.routing.Default.Adapter != "" {
		seen[r.routing.Default.Adapter] = true
	}
	for _, cfg := range r.routing.Overrides {
		if cfg.Adapter != "" {
			seen[cfg.Adapter] = true
		}
	}

	adapters := make([]string, 0, len(seen))
	for name := range seen {
		adapters = append(adapters, name)
	}
	sort.Strings(adapters)
	return adapters
}

// UsesAdapter returns true if the given adapter name is used in routing config.
func (r *Router) UsesAdapter(adapter string) bool {
	if r.routing == nil {
		return false
	}
	if r.routing.Default.Adapter == adapter {
		return true
	}
	for _, cfg := range r.routing.Overrides {
		if cfg.Adapter == adapter {
			return true
		}
	}
	return false
}

// UnknownPhases returns phase names used in Overrides that are not in ValidPhases.
// Returns nil if all phases are recognized or if routing is nil.
func (r *Router) UnknownPhases() []string {
	if r.routing == nil {
		return nil
	}
	var unknown []string
	for phase := range r.routing.Overrides {
		if !ValidPhases[phase] {
			unknown = append(unknown, phase)
		}
	}
	sort.Strings(unknown)
	return unknown
}
