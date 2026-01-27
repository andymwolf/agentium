package routing

import (
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

	for _, phase := range []string{"IMPLEMENT", "TEST", "PR_CREATION"} {
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
			"TEST": {Adapter: "claude-code", Model: "sonnet"},
			"DOCS": {Adapter: "aider", Model: "gpt-4"},
		},
	})

	adapters := r.Adapters()
	// Adapters() returns sorted results
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

func TestUnknownPhases_AllValid(t *testing.T) {
	r := NewRouter(&PhaseRouting{
		Default: ModelConfig{Adapter: "claude-code", Model: "opus"},
		Overrides: map[string]ModelConfig{
			"IMPLEMENT":   {Model: "sonnet"},
			"DOCS":        {Model: "haiku"},
			"PR_CREATION": {Model: "opus"},
		},
	})

	if unknowns := r.UnknownPhases(); len(unknowns) != 0 {
		t.Errorf("expected no unknown phases, got %v", unknowns)
	}
}

func TestUnknownPhases_HasUnknown(t *testing.T) {
	r := NewRouter(&PhaseRouting{
		Default: ModelConfig{Adapter: "claude-code", Model: "opus"},
		Overrides: map[string]ModelConfig{
			"IMPLMENT": {Model: "sonnet"}, // typo
			"DEPLOY":   {Model: "opus"},   // not a valid phase
		},
	})

	unknowns := r.UnknownPhases()
	if len(unknowns) != 2 {
		t.Fatalf("expected 2 unknown phases, got %d: %v", len(unknowns), unknowns)
	}
	// Sorted output: DEPLOY comes before IMPLMENT
	expected := map[string]bool{"DEPLOY": true, "IMPLMENT": true}
	for _, u := range unknowns {
		if !expected[u] {
			t.Errorf("unexpected unknown phase: %s", u)
		}
	}
}

func TestUnknownPhases_NilRouter(t *testing.T) {
	r := NewRouter(nil)
	if unknowns := r.UnknownPhases(); unknowns != nil {
		t.Errorf("nil router UnknownPhases should return nil, got %v", unknowns)
	}
}

func TestCompoundPhaseKeysAreValid(t *testing.T) {
	compoundPhases := []string{
		"PLAN_REVIEW", "IMPLEMENT_REVIEW", "DOCS_REVIEW",
		"JUDGE", "PLAN_JUDGE", "IMPLEMENT_JUDGE", "DOCS_JUDGE",
	}

	for _, phase := range compoundPhases {
		if !ValidPhases[phase] {
			t.Errorf("compound phase %q should be in ValidPhases", phase)
		}
	}
}

func TestCompoundPhaseOverrides(t *testing.T) {
	r := NewRouter(&PhaseRouting{
		Default: ModelConfig{Adapter: "claude-code", Model: "opus"},
		Overrides: map[string]ModelConfig{
			"PLAN_REVIEW":     {Adapter: "codex", Model: "o3"},
			"IMPLEMENT_JUDGE": {Adapter: "claude-code", Model: "sonnet"},
		},
	})

	// Compound phase should resolve to its specific override
	cfg := r.ModelForPhase("PLAN_REVIEW")
	if cfg.Adapter != "codex" || cfg.Model != "o3" {
		t.Errorf("PLAN_REVIEW should use override, got %+v", cfg)
	}

	cfg = r.ModelForPhase("IMPLEMENT_JUDGE")
	if cfg.Adapter != "claude-code" || cfg.Model != "sonnet" {
		t.Errorf("IMPLEMENT_JUDGE should use override, got %+v", cfg)
	}

	// Non-overridden compound phase falls back to default
	cfg = r.ModelForPhase("TEST_REVIEW")
	if cfg.Adapter != "claude-code" || cfg.Model != "opus" {
		t.Errorf("TEST_REVIEW should fall back to default, got %+v", cfg)
	}
}

func TestCompoundPhasesNotFlaggedAsUnknown(t *testing.T) {
	r := NewRouter(&PhaseRouting{
		Default: ModelConfig{Adapter: "claude-code"},
		Overrides: map[string]ModelConfig{
			"PLAN_REVIEW":      {Adapter: "codex"},
			"IMPLEMENT_REVIEW": {Adapter: "codex"},
			"JUDGE":            {Adapter: "claude-code", Model: "sonnet"},
			"PLAN_JUDGE":       {Model: "sonnet"},
		},
	})

	unknowns := r.UnknownPhases()
	if len(unknowns) != 0 {
		t.Errorf("compound phases should not be unknown, got %v", unknowns)
	}
}

func TestAdaptersSorted(t *testing.T) {
	r := NewRouter(&PhaseRouting{
		Default: ModelConfig{Adapter: "zeta", Model: "opus"},
		Overrides: map[string]ModelConfig{
			"TEST": {Adapter: "alpha", Model: "sonnet"},
			"DOCS": {Adapter: "middle", Model: "gpt-4"},
		},
	})

	adapters := r.Adapters()
	if len(adapters) != 3 {
		t.Fatalf("expected 3 adapters, got %d: %v", len(adapters), adapters)
	}
	if adapters[0] != "alpha" || adapters[1] != "middle" || adapters[2] != "zeta" {
		t.Errorf("adapters not sorted: %v", adapters)
	}
}
