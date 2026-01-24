package controller

import "testing"

func TestAdvancePhase(t *testing.T) {
	tests := []struct {
		name    string
		current TaskPhase
		want    TaskPhase
	}{
		{"PLAN advances to IMPLEMENT", PhasePlan, PhaseImplement},
		{"IMPLEMENT advances to TEST", PhaseImplement, PhaseTest},
		{"TEST advances to REVIEW", PhaseTest, PhaseReview},
		{"REVIEW advances to PR_CREATION", PhaseReview, PhasePRCreation},
		{"PR_CREATION advances to COMPLETE", PhasePRCreation, PhaseComplete},
		{"unknown phase advances to COMPLETE", TaskPhase("UNKNOWN"), PhaseComplete},
		{"COMPLETE stays COMPLETE", PhaseComplete, PhaseComplete},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := advancePhase(tt.current)
			if got != tt.want {
				t.Errorf("advancePhase(%q) = %q, want %q", tt.current, got, tt.want)
			}
		})
	}
}

func TestPhaseMaxIterations_Defaults(t *testing.T) {
	c := &Controller{
		config: SessionConfig{
			PhaseLoop: &PhaseLoopConfig{Enabled: true},
		},
	}

	tests := []struct {
		phase TaskPhase
		want  int
	}{
		{PhasePlan, defaultPlanMaxIter},
		{PhaseImplement, defaultBuildMaxIter},
		{PhaseTest, defaultTestMaxIter},
		{PhaseReview, defaultReviewMaxIter},
		{PhasePRCreation, defaultPRMaxIter},
		{TaskPhase("UNKNOWN"), 1},
	}

	for _, tt := range tests {
		t.Run(string(tt.phase), func(t *testing.T) {
			got := c.phaseMaxIterations(tt.phase)
			if got != tt.want {
				t.Errorf("phaseMaxIterations(%q) = %d, want %d", tt.phase, got, tt.want)
			}
		})
	}
}

func TestPhaseMaxIterations_CustomConfig(t *testing.T) {
	c := &Controller{
		config: SessionConfig{
			PhaseLoop: &PhaseLoopConfig{
				Enabled:             true,
				PlanMaxIterations:   2,
				BuildMaxIterations:  10,
				TestMaxIterations:   8,
				ReviewMaxIterations: 4,
			},
		},
	}

	tests := []struct {
		phase TaskPhase
		want  int
	}{
		{PhasePlan, 2},
		{PhaseImplement, 10},
		{PhaseTest, 8},
		{PhaseReview, 4},
		{PhasePRCreation, defaultPRMaxIter}, // No custom config for PR_CREATION
	}

	for _, tt := range tests {
		t.Run(string(tt.phase), func(t *testing.T) {
			got := c.phaseMaxIterations(tt.phase)
			if got != tt.want {
				t.Errorf("phaseMaxIterations(%q) = %d, want %d", tt.phase, got, tt.want)
			}
		})
	}
}

func TestPhaseLoopConfig_Nil(t *testing.T) {
	c := &Controller{
		config: SessionConfig{
			PhaseLoop: nil,
		},
	}

	// Should use defaults when config is nil
	if got := c.phaseMaxIterations(PhasePlan); got != defaultPlanMaxIter {
		t.Errorf("phaseMaxIterations(PLAN) with nil config = %d, want %d", got, defaultPlanMaxIter)
	}
}

func TestIsPhaseLoopEnabled(t *testing.T) {
	tests := []struct {
		name   string
		config *PhaseLoopConfig
		want   bool
	}{
		{"nil config", nil, false},
		{"disabled", &PhaseLoopConfig{Enabled: false}, false},
		{"enabled", &PhaseLoopConfig{Enabled: true}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Controller{config: SessionConfig{PhaseLoop: tt.config}}
			if got := c.isPhaseLoopEnabled(); got != tt.want {
				t.Errorf("isPhaseLoopEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIssuePhaseOrder(t *testing.T) {
	// Verify the expected phase order
	expected := []TaskPhase{PhasePlan, PhaseImplement, PhaseTest, PhaseReview, PhasePRCreation}
	if len(issuePhaseOrder) != len(expected) {
		t.Fatalf("issuePhaseOrder length = %d, want %d", len(issuePhaseOrder), len(expected))
	}
	for i, phase := range expected {
		if issuePhaseOrder[i] != phase {
			t.Errorf("issuePhaseOrder[%d] = %q, want %q", i, issuePhaseOrder[i], phase)
		}
	}
}

func TestTruncateForComment(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int // expected length (0 means check equality)
	}{
		{"short string", "hello", 5},
		{"exactly 500", string(make([]byte, 500)), 500},
		{"over 500", string(make([]byte, 600)), 503}, // 500 + "..."
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateForComment(tt.input)
			if len(result) != tt.want {
				t.Errorf("truncateForComment() length = %d, want %d", len(result), tt.want)
			}
		})
	}
}
