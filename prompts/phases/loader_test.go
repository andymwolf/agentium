package phases

import (
	"strings"
	"testing"
)

func TestGet_AllCombosNonEmpty(t *testing.T) {
	phases := []string{"PLAN", "IMPLEMENT", "DOCS", "VERIFY"}
	roles := []string{"WORKER", "REVIEWER", "JUDGE"}

	for _, phase := range phases {
		for _, role := range roles {
			t.Run(phase+"_"+role, func(t *testing.T) {
				content := Get(phase, role)
				if content == "" {
					t.Errorf("Get(%q, %q) returned empty string", phase, role)
				}
			})
		}
	}
}

func TestGet_UnknownCombo(t *testing.T) {
	tests := []struct {
		phase string
		role  string
	}{
		{"UNKNOWN", "WORKER"},
		{"PLAN", "UNKNOWN"},
		{"", ""},
		{"PLAN", ""},
		{"", "WORKER"},
	}

	for _, tt := range tests {
		t.Run(tt.phase+"_"+tt.role, func(t *testing.T) {
			content := Get(tt.phase, tt.role)
			if content != "" {
				t.Errorf("Get(%q, %q) should return empty string for unknown combo, got %d chars", tt.phase, tt.role, len(content))
			}
		})
	}
}

func TestGet_CaseInsensitive(t *testing.T) {
	upper := Get("PLAN", "WORKER")
	lower := Get("plan", "worker")
	mixed := Get("Plan", "Worker")

	if upper == "" {
		t.Fatal("Get(PLAN, WORKER) returned empty string")
	}
	if upper != lower {
		t.Error("Get should be case-insensitive: PLAN/WORKER != plan/worker")
	}
	if upper != mixed {
		t.Error("Get should be case-insensitive: PLAN/WORKER != Plan/Worker")
	}
}

func TestGet_ContentSpotChecks(t *testing.T) {
	tests := []struct {
		phase    string
		role     string
		expected string
	}{
		{"PLAN", "WORKER", "PLAN PHASE"},
		{"IMPLEMENT", "WORKER", "IMPLEMENT PHASE"},
		{"DOCS", "WORKER", "DOCS PHASE"},
		{"VERIFY", "WORKER", "VERIFY PHASE"},
		{"PLAN", "REVIEWER", "PLAN REVIEWER"},
		{"IMPLEMENT", "REVIEWER", "CODE REVIEWER"},
		{"DOCS", "REVIEWER", "DOCS REVIEWER"},
		{"VERIFY", "REVIEWER", "VERIFY REVIEWER"},
		{"PLAN", "JUDGE", "JUDGE"},
		{"IMPLEMENT", "JUDGE", "JUDGE"},
		{"DOCS", "JUDGE", "JUDGE"},
		{"VERIFY", "JUDGE", "JUDGE"},
	}

	for _, tt := range tests {
		t.Run(tt.phase+"_"+tt.role, func(t *testing.T) {
			content := Get(tt.phase, tt.role)
			if !strings.Contains(content, tt.expected) {
				t.Errorf("Get(%q, %q) does not contain %q", tt.phase, tt.role, tt.expected)
			}
		})
	}
}

func TestGet_WorkersSafetyConstraints(t *testing.T) {
	phases := []string{"PLAN", "IMPLEMENT", "DOCS", "VERIFY"}
	for _, phase := range phases {
		t.Run(phase, func(t *testing.T) {
			content := Get(phase, "WORKER")
			if !strings.Contains(content, "CRITICAL SAFETY CONSTRAINTS") {
				t.Errorf("Worker prompt for %s missing safety constraints", phase)
			}
			if !strings.Contains(content, "AGENTIUM_STATUS") {
				t.Errorf("Worker prompt for %s missing status signaling", phase)
			}
			if !strings.Contains(content, "AGENTIUM_MEMORY") {
				t.Errorf("Worker prompt for %s missing memory signaling", phase)
			}
		})
	}
}

func TestGet_ReviewersEvalSignal(t *testing.T) {
	phases := []string{"PLAN", "IMPLEMENT", "DOCS", "VERIFY"}
	for _, phase := range phases {
		t.Run(phase, func(t *testing.T) {
			content := Get(phase, "REVIEWER")
			if !strings.Contains(content, "AGENTIUM_EVAL") {
				t.Errorf("Reviewer prompt for %s missing AGENTIUM_EVAL signal", phase)
			}
		})
	}
}

func TestGet_JudgesEvalSignal(t *testing.T) {
	phases := []string{"PLAN", "IMPLEMENT", "DOCS", "VERIFY"}
	for _, phase := range phases {
		t.Run(phase, func(t *testing.T) {
			content := Get(phase, "JUDGE")
			if !strings.Contains(content, "AGENTIUM_EVAL") {
				t.Errorf("Judge prompt for %s missing AGENTIUM_EVAL signal", phase)
			}
		})
	}
}

func TestPhases(t *testing.T) {
	phases := Phases()
	expected := []string{"PLAN", "IMPLEMENT", "DOCS", "VERIFY"}

	if len(phases) != len(expected) {
		t.Fatalf("Phases() returned %d items, want %d", len(phases), len(expected))
	}

	for i, p := range expected {
		if phases[i] != p {
			t.Errorf("Phases()[%d] = %q, want %q", i, phases[i], p)
		}
	}
}
