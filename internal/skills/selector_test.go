package skills

import (
	"strings"
	"testing"
)

func newTestSkills() []Skill {
	return []Skill{
		{
			Entry:   SkillEntry{Name: "safety", File: "safety.md", Priority: 10, Phases: nil},
			Content: "SAFETY CONTENT",
		},
		{
			Entry:   SkillEntry{Name: "environment", File: "environment.md", Priority: 20, Phases: nil},
			Content: "ENVIRONMENT CONTENT",
		},
		{
			Entry:   SkillEntry{Name: "planning", File: "planning.md", Priority: 40, Phases: []string{"IMPLEMENT", "ANALYZE"}},
			Content: "PLANNING CONTENT",
		},
		{
			Entry:   SkillEntry{Name: "implement", File: "implement.md", Priority: 50, Phases: []string{"IMPLEMENT"}},
			Content: "IMPLEMENT CONTENT",
		},
		{
			Entry:   SkillEntry{Name: "test", File: "test.md", Priority: 60, Phases: []string{"TEST", "IMPLEMENT"}},
			Content: "TEST CONTENT",
		},
		{
			Entry:   SkillEntry{Name: "pr_creation", File: "pr_creation.md", Priority: 70, Phases: []string{"PR_CREATION"}},
			Content: "PR CREATION CONTENT",
		},
		{
			Entry:   SkillEntry{Name: "pr_review", File: "pr_review.md", Priority: 80, Phases: []string{"ANALYZE", "PUSH"}},
			Content: "PR REVIEW CONTENT",
		},
	}
}

func TestSelector_SelectForPhase_Implement(t *testing.T) {
	s := NewSelector(newTestSkills())
	result := s.SelectForPhase("IMPLEMENT")

	// Should include universal skills + IMPLEMENT-phase skills
	expected := []string{"SAFETY CONTENT", "ENVIRONMENT CONTENT", "PLANNING CONTENT", "IMPLEMENT CONTENT", "TEST CONTENT"}
	for _, exp := range expected {
		if !strings.Contains(result, exp) {
			t.Errorf("SelectForPhase(IMPLEMENT) missing %q", exp)
		}
	}

	// Should NOT include PR-specific skills
	excluded := []string{"PR CREATION CONTENT", "PR REVIEW CONTENT"}
	for _, exc := range excluded {
		if strings.Contains(result, exc) {
			t.Errorf("SelectForPhase(IMPLEMENT) should not contain %q", exc)
		}
	}
}

func TestSelector_SelectForPhase_Test(t *testing.T) {
	s := NewSelector(newTestSkills())
	result := s.SelectForPhase("TEST")

	expected := []string{"SAFETY CONTENT", "ENVIRONMENT CONTENT", "TEST CONTENT"}
	for _, exp := range expected {
		if !strings.Contains(result, exp) {
			t.Errorf("SelectForPhase(TEST) missing %q", exp)
		}
	}

	excluded := []string{"PLANNING CONTENT", "IMPLEMENT CONTENT", "PR CREATION CONTENT", "PR REVIEW CONTENT"}
	for _, exc := range excluded {
		if strings.Contains(result, exc) {
			t.Errorf("SelectForPhase(TEST) should not contain %q", exc)
		}
	}
}

func TestSelector_SelectForPhase_Analyze(t *testing.T) {
	s := NewSelector(newTestSkills())
	result := s.SelectForPhase("ANALYZE")

	expected := []string{"SAFETY CONTENT", "ENVIRONMENT CONTENT", "PLANNING CONTENT", "PR REVIEW CONTENT"}
	for _, exp := range expected {
		if !strings.Contains(result, exp) {
			t.Errorf("SelectForPhase(ANALYZE) missing %q", exp)
		}
	}

	excluded := []string{"IMPLEMENT CONTENT", "TEST CONTENT", "PR CREATION CONTENT"}
	for _, exc := range excluded {
		if strings.Contains(result, exc) {
			t.Errorf("SelectForPhase(ANALYZE) should not contain %q", exc)
		}
	}
}

func TestSelector_SelectForPhase_PRCreation(t *testing.T) {
	s := NewSelector(newTestSkills())
	result := s.SelectForPhase("PR_CREATION")

	expected := []string{"SAFETY CONTENT", "ENVIRONMENT CONTENT", "PR CREATION CONTENT"}
	for _, exp := range expected {
		if !strings.Contains(result, exp) {
			t.Errorf("SelectForPhase(PR_CREATION) missing %q", exp)
		}
	}

	excluded := []string{"PLANNING CONTENT", "IMPLEMENT CONTENT", "TEST CONTENT", "PR REVIEW CONTENT"}
	for _, exc := range excluded {
		if strings.Contains(result, exc) {
			t.Errorf("SelectForPhase(PR_CREATION) should not contain %q", exc)
		}
	}
}

func TestSelector_SelectForPhase_Push(t *testing.T) {
	s := NewSelector(newTestSkills())
	result := s.SelectForPhase("PUSH")

	expected := []string{"SAFETY CONTENT", "ENVIRONMENT CONTENT", "PR REVIEW CONTENT"}
	for _, exp := range expected {
		if !strings.Contains(result, exp) {
			t.Errorf("SelectForPhase(PUSH) missing %q", exp)
		}
	}
}

func TestSelector_SelectForPhase_UnknownPhase(t *testing.T) {
	s := NewSelector(newTestSkills())
	result := s.SelectForPhase("UNKNOWN")

	// Should only include universal skills
	if !strings.Contains(result, "SAFETY CONTENT") {
		t.Error("SelectForPhase(UNKNOWN) missing universal skill: safety")
	}
	if !strings.Contains(result, "ENVIRONMENT CONTENT") {
		t.Error("SelectForPhase(UNKNOWN) missing universal skill: environment")
	}

	// Should not include any phase-specific skills
	phaseSpecific := []string{"PLANNING CONTENT", "IMPLEMENT CONTENT", "TEST CONTENT", "PR CREATION CONTENT", "PR REVIEW CONTENT"}
	for _, exc := range phaseSpecific {
		if strings.Contains(result, exc) {
			t.Errorf("SelectForPhase(UNKNOWN) should not contain %q", exc)
		}
	}
}

func TestSelector_SelectForPhase_PriorityOrder(t *testing.T) {
	s := NewSelector(newTestSkills())
	result := s.SelectForPhase("IMPLEMENT")

	// Verify priority ordering: safety comes before environment, which comes before planning, etc.
	safetyIdx := strings.Index(result, "SAFETY CONTENT")
	envIdx := strings.Index(result, "ENVIRONMENT CONTENT")
	planningIdx := strings.Index(result, "PLANNING CONTENT")
	implementIdx := strings.Index(result, "IMPLEMENT CONTENT")
	testIdx := strings.Index(result, "TEST CONTENT")

	if safetyIdx > envIdx {
		t.Error("safety should come before environment")
	}
	if envIdx > planningIdx {
		t.Error("environment should come before planning")
	}
	if planningIdx > implementIdx {
		t.Error("planning should come before implement")
	}
	if implementIdx > testIdx {
		t.Error("implement should come before test")
	}
}

func TestSelector_SkillsForPhase(t *testing.T) {
	s := NewSelector(newTestSkills())

	tests := []struct {
		phase    string
		expected []string
	}{
		{"IMPLEMENT", []string{"safety", "environment", "planning", "implement", "test"}},
		{"TEST", []string{"safety", "environment", "test"}},
		{"ANALYZE", []string{"safety", "environment", "planning", "pr_review"}},
		{"PR_CREATION", []string{"safety", "environment", "pr_creation"}},
		{"PUSH", []string{"safety", "environment", "pr_review"}},
		{"UNKNOWN", []string{"safety", "environment"}},
	}

	for _, tt := range tests {
		t.Run(tt.phase, func(t *testing.T) {
			names := s.SkillsForPhase(tt.phase)
			if len(names) != len(tt.expected) {
				t.Errorf("SkillsForPhase(%s) = %v, want %v", tt.phase, names, tt.expected)
				return
			}
			for i, name := range tt.expected {
				if names[i] != name {
					t.Errorf("SkillsForPhase(%s)[%d] = %q, want %q", tt.phase, i, names[i], name)
				}
			}
		})
	}
}

func TestSelector_SelectForPhase_Separator(t *testing.T) {
	s := NewSelector(newTestSkills())
	result := s.SelectForPhase("IMPLEMENT")

	// Verify parts are separated by double newlines
	parts := strings.Split(result, "\n\n")
	if len(parts) < 5 {
		t.Errorf("Expected at least 5 parts separated by double newlines, got %d", len(parts))
	}
}

func TestSelector_EmptySkills(t *testing.T) {
	s := NewSelector([]Skill{})

	result := s.SelectForPhase("IMPLEMENT")
	if result != "" {
		t.Errorf("SelectForPhase with empty skills should return empty string, got %q", result)
	}

	names := s.SkillsForPhase("IMPLEMENT")
	if len(names) != 0 {
		t.Errorf("SkillsForPhase with empty skills should return nil, got %v", names)
	}
}
