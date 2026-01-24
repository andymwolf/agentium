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
		{PhaseImplement, defaultImplementMaxIter},
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
				Enabled:                true,
				PlanMaxIterations:      2,
				ImplementMaxIterations: 10,
				TestMaxIterations:      8,
				ReviewMaxIterations:    4,
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
		name      string
		input     string
		wantRunes int // expected rune count of result
	}{
		{"short string", "hello", 5},
		{"exactly 500 runes", string(make([]byte, 500)), 500},
		{"over 500 runes", string(make([]byte, 600)), 503}, // 500 + "..."
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateForComment(tt.input)
			if len([]rune(result)) != tt.wantRunes {
				t.Errorf("truncateForComment() rune count = %d, want %d", len([]rune(result)), tt.wantRunes)
			}
		})
	}
}

func TestTruncateForComment_UTF8Safety(t *testing.T) {
	// Build a string of 600 multi-byte runes (each is 3 bytes in UTF-8)
	runes := make([]rune, 600)
	for i := range runes {
		runes[i] = '\u4e16' // 世 (3 bytes per rune)
	}
	input := string(runes)

	result := truncateForComment(input)

	// Should have 500 runes + "..." (503 runes total), not split mid-character
	resultRunes := []rune(result)
	if len(resultRunes) != 503 {
		t.Errorf("rune count = %d, want 503", len(resultRunes))
	}
	// First 500 runes should all be 世
	for i := 0; i < 500; i++ {
		if resultRunes[i] != '\u4e16' {
			t.Errorf("rune[%d] = %U, want U+4E16", i, resultRunes[i])
			break
		}
	}
	// Verify valid UTF-8 (Go strings are always valid, but check no replacement chars)
	if resultRunes[500] != '.' {
		t.Errorf("expected '.' at position 500, got %U", resultRunes[500])
	}
}
