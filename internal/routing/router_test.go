package routing

import (
	"sort"
	"testing"
)

func TestNilRouter(t *testing.T) {
	r := NewRouter(nil)

	if r.IsConfigured() {
		t.Error("nil router should not be configured")
	}

	cfg := r.ModelForPhase("IMPLEMENT")
	if cfg.Adapter != "" || cfg.Model != "" {
		t.Errorf("nil router ModelForPhase should return empty, got %+v", cfg)
	}

	if adapters := r.Adapters(); adapters != nil {
		t.Errorf("nil router Adapters should return nil, got %v", adapters)
	}
}

func TestDefaultOnly(t *testing.T) {
	r := NewRouter(&PhaseRouting{
		Default: ModelConfig{Adapter: "claude-code", Model: "opus"},
	})

	if !r.IsConfigured() {
		t.Error("router with default should be configured")
	}

	for _, phase := range []string{"IMPLEMENT", "TEST", "REVIEW", "PR_CREATION"} {
		cfg := r.ModelForPhase(phase)
		if cfg.Adapter != "claude-code" || cfg.Model != "opus" {
			t.Errorf("phase %s: expected default, got %+v", phase, cfg)
		}
	}
}

func TestOverrideExists(t *testing.T) {
	r := NewRouter(&PhaseRouting{
		Default: ModelConfig{Adapter: "claude-code", Model: "opus"},
		Overrides: map[string]ModelConfig{
			"TEST": {Adapter: "claude-code", Model: "sonnet"},
		},
	})

	cfg := r.ModelForPhase("TEST")
	if cfg.Adapter != "claude-code" || cfg.Model != "sonnet" {
		t.Errorf("TEST phase should use override, got %+v", cfg)
	}
}

func TestOverrideMissing(t *testing.T) {
	r := NewRouter(&PhaseRouting{
		Default: ModelConfig{Adapter: "claude-code", Model: "opus"},
		Overrides: map[string]ModelConfig{
			"TEST": {Adapter: "claude-code", Model: "sonnet"},
		},
	})

	cfg := r.ModelForPhase("IMPLEMENT")
	if cfg.Adapter != "claude-code" || cfg.Model != "opus" {
		t.Errorf("IMPLEMENT phase should fallback to default, got %+v", cfg)
	}
}

func TestAdaptersUnique(t *testing.T) {
	r := NewRouter(&PhaseRouting{
		Default: ModelConfig{Adapter: "claude-code", Model: "opus"},
		Overrides: map[string]ModelConfig{
			"TEST":   {Adapter: "claude-code", Model: "sonnet"},
			"REVIEW": {Adapter: "aider", Model: "gpt-4"},
		},
	})

	adapters := r.Adapters()
	sort.Strings(adapters)

	if len(adapters) != 2 {
		t.Fatalf("expected 2 unique adapters, got %d: %v", len(adapters), adapters)
	}
	if adapters[0] != "aider" || adapters[1] != "claude-code" {
		t.Errorf("unexpected adapters: %v", adapters)
	}
}

func TestAdaptersEmptyAdapter(t *testing.T) {
	r := NewRouter(&PhaseRouting{
		Default: ModelConfig{Model: "opus"},
		Overrides: map[string]ModelConfig{
			"TEST": {Model: "sonnet"},
		},
	})

	adapters := r.Adapters()
	if len(adapters) != 0 {
		t.Errorf("expected no adapters when all have empty adapter field, got %v", adapters)
	}
}

func TestIsConfiguredOverridesOnly(t *testing.T) {
	r := NewRouter(&PhaseRouting{
		Overrides: map[string]ModelConfig{
			"TEST": {Adapter: "claude-code", Model: "sonnet"},
		},
	})

	if !r.IsConfigured() {
		t.Error("router with overrides should be configured")
	}
}

func TestIsConfiguredEmpty(t *testing.T) {
	r := NewRouter(&PhaseRouting{})

	if r.IsConfigured() {
		t.Error("router with empty config should not be configured")
	}
}

func TestParseModelSpecWithColon(t *testing.T) {
	cfg := ParseModelSpec("claude-code:opus")
	if cfg.Adapter != "claude-code" || cfg.Model != "opus" {
		t.Errorf("expected {claude-code, opus}, got %+v", cfg)
	}
}

func TestParseModelSpecWithoutColon(t *testing.T) {
	cfg := ParseModelSpec("opus")
	if cfg.Adapter != "" || cfg.Model != "opus" {
		t.Errorf("expected {'', opus}, got %+v", cfg)
	}
}

func TestParseModelSpecMultipleColons(t *testing.T) {
	cfg := ParseModelSpec("claude-code:claude-opus-4-20250514")
	if cfg.Adapter != "claude-code" || cfg.Model != "claude-opus-4-20250514" {
		t.Errorf("expected {claude-code, claude-opus-4-20250514}, got %+v", cfg)
	}
}

func TestParseModelSpecEmpty(t *testing.T) {
	cfg := ParseModelSpec("")
	if cfg.Adapter != "" || cfg.Model != "" {
		t.Errorf("expected empty, got %+v", cfg)
	}
}
