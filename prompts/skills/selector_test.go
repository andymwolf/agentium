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
			Entry:   SkillEntry{Name: "status_signals", File: "status_signals.md", Priority: 30, Phases: nil},
			Content: "STATUS_SIGNALS CONTENT",
		},
		{
			Entry:   SkillEntry{Name: "planning", File: "planning.md", Priority: 40, Phases: []string{"ANALYZE"}},
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
		{
			Entry:   SkillEntry{Name: "code_reviewer", File: "code_reviewer.md", Priority: 91, Phases: []string{"IMPLEMENT_REVIEW"}},
			Content: "CODE REVIEWER CONTENT",
		},
		{
			Entry:   SkillEntry{Name: "docs_reviewer", File: "docs_reviewer.md", Priority: 92, Phases: []string{"DOCS_REVIEW"}},
			Content: "DOCS REVIEWER CONTENT",
		},
	}
}

func TestSelector_SelectForPhase_Implement(t *testing.T) {
	s := NewSelector(newTestSkills())
	result := s.SelectForPhase("IMPLEMENT")

	// Should include universal skills + IMPLEMENT-phase skills
	expected := []string{"SAFETY CONTENT", "ENVIRONMENT CONTENT", "STATUS_SIGNALS CONTENT", "IMPLEMENT CONTENT", "TEST CONTENT"}
	for _, exp := range expected {
		if !strings.Contains(result, exp) {
			t.Errorf("SelectForPhase(IMPLEMENT) missing %q", exp)
		}
	}

	// Should NOT include planning or PR-specific skills
	excluded := []string{"PLANNING CONTENT", "PR CREATION CONTENT", "PR REVIEW CONTENT"}
	for _, exc := range excluded {
		if strings.Contains(result, exc) {
			t.Errorf("SelectForPhase(IMPLEMENT) should not contain %q", exc)
		}
	}
}

func TestSelector_SelectForPhase_Test(t *testing.T) {
	s := NewSelector(newTestSkills())
	result := s.SelectForPhase("TEST")

	expected := []string{"SAFETY CONTENT", "ENVIRONMENT CONTENT", "STATUS_SIGNALS CONTENT", "TEST CONTENT"}
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

	expected := []string{"SAFETY CONTENT", "ENVIRONMENT CONTENT", "STATUS_SIGNALS CONTENT", "PLANNING CONTENT", "PR REVIEW CONTENT"}
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

	expected := []string{"SAFETY CONTENT", "ENVIRONMENT CONTENT", "STATUS_SIGNALS CONTENT", "PR CREATION CONTENT"}
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

	expected := []string{"SAFETY CONTENT", "ENVIRONMENT CONTENT", "STATUS_SIGNALS CONTENT", "PR REVIEW CONTENT"}
	for _, exp := range expected {
		if !strings.Contains(result, exp) {
			t.Errorf("SelectForPhase(PUSH) missing %q", exp)
		}
	}
}

func TestSelector_SelectForPhase_DocsReview(t *testing.T) {
	s := NewSelector(newTestSkills())
	result := s.SelectForPhase("DOCS_REVIEW")

	// Should include universal skills + docs_reviewer
	expected := []string{"SAFETY CONTENT", "ENVIRONMENT CONTENT", "STATUS_SIGNALS CONTENT", "DOCS REVIEWER CONTENT"}
	for _, exp := range expected {
		if !strings.Contains(result, exp) {
			t.Errorf("SelectForPhase(DOCS_REVIEW) missing %q", exp)
		}
	}

	// Should NOT include code_reviewer - this is the key assertion for issue #285
	excluded := []string{"CODE REVIEWER CONTENT", "PLANNING CONTENT", "IMPLEMENT CONTENT"}
	for _, exc := range excluded {
		if strings.Contains(result, exc) {
			t.Errorf("SelectForPhase(DOCS_REVIEW) should not contain %q", exc)
		}
	}
}

func TestSelector_SelectForPhase_ImplementReview(t *testing.T) {
	s := NewSelector(newTestSkills())
	result := s.SelectForPhase("IMPLEMENT_REVIEW")

	// Should include universal skills + code_reviewer
	expected := []string{"SAFETY CONTENT", "ENVIRONMENT CONTENT", "STATUS_SIGNALS CONTENT", "CODE REVIEWER CONTENT"}
	for _, exp := range expected {
		if !strings.Contains(result, exp) {
			t.Errorf("SelectForPhase(IMPLEMENT_REVIEW) missing %q", exp)
		}
	}

	// Should NOT include docs_reviewer
	excluded := []string{"DOCS REVIEWER CONTENT", "PLANNING CONTENT", "IMPLEMENT CONTENT"}
	for _, exc := range excluded {
		if strings.Contains(result, exc) {
			t.Errorf("SelectForPhase(IMPLEMENT_REVIEW) should not contain %q", exc)
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
	if !strings.Contains(result, "STATUS_SIGNALS CONTENT") {
		t.Error("SelectForPhase(UNKNOWN) missing universal skill: status_signals")
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

	// Verify priority ordering: safety comes before environment, which comes before status_signals, etc.
	safetyIdx := strings.Index(result, "SAFETY CONTENT")
	envIdx := strings.Index(result, "ENVIRONMENT CONTENT")
	statusIdx := strings.Index(result, "STATUS_SIGNALS CONTENT")
	implementIdx := strings.Index(result, "IMPLEMENT CONTENT")
	testIdx := strings.Index(result, "TEST CONTENT")

	if safetyIdx > envIdx {
		t.Error("safety should come before environment")
	}
	if envIdx > statusIdx {
		t.Error("environment should come before status_signals")
	}
	if statusIdx > implementIdx {
		t.Error("status_signals should come before implement")
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
		{"IMPLEMENT", []string{"safety", "environment", "status_signals", "implement", "test"}},
		{"TEST", []string{"safety", "environment", "status_signals", "test"}},
		{"ANALYZE", []string{"safety", "environment", "status_signals", "planning", "pr_review"}},
		{"PR_CREATION", []string{"safety", "environment", "status_signals", "pr_creation"}},
		{"PUSH", []string{"safety", "environment", "status_signals", "pr_review"}},
		{"IMPLEMENT_REVIEW", []string{"safety", "environment", "status_signals", "code_reviewer"}},
		{"DOCS_REVIEW", []string{"safety", "environment", "status_signals", "docs_reviewer"}},
		{"UNKNOWN", []string{"safety", "environment", "status_signals"}},
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

func TestSelector_SelectByNames(t *testing.T) {
	s := NewSelector(newTestSkills())
	result := s.SelectByNames([]string{"safety", "implement", "test"})

	expected := []string{"SAFETY CONTENT", "IMPLEMENT CONTENT", "TEST CONTENT"}
	for _, exp := range expected {
		if !strings.Contains(result, exp) {
			t.Errorf("SelectByNames missing %q", exp)
		}
	}

	excluded := []string{"ENVIRONMENT CONTENT", "PLANNING CONTENT", "PR CREATION CONTENT"}
	for _, exc := range excluded {
		if strings.Contains(result, exc) {
			t.Errorf("SelectByNames should not contain %q", exc)
		}
	}
}

func TestSelector_SelectByNames_PriorityOrder(t *testing.T) {
	s := NewSelector(newTestSkills())
	result := s.SelectByNames([]string{"test", "safety", "implement"})

	// Results should be in priority order (safety=10, implement=50, test=60)
	safetyIdx := strings.Index(result, "SAFETY CONTENT")
	implementIdx := strings.Index(result, "IMPLEMENT CONTENT")
	testIdx := strings.Index(result, "TEST CONTENT")

	if safetyIdx > implementIdx {
		t.Error("safety (priority 10) should come before implement (priority 50)")
	}
	if implementIdx > testIdx {
		t.Error("implement (priority 50) should come before test (priority 60)")
	}
}

func TestSelector_SelectByNames_Unknown(t *testing.T) {
	s := NewSelector(newTestSkills())
	result := s.SelectByNames([]string{"safety", "nonexistent", "also_missing"})

	if !strings.Contains(result, "SAFETY CONTENT") {
		t.Error("SelectByNames should include known skill 'safety'")
	}
	// Unknown names are silently skipped — result should only contain safety
	parts := strings.Split(result, "\n\n")
	if len(parts) != 1 {
		t.Errorf("Expected 1 part (only safety), got %d parts", len(parts))
	}
}

func TestSelector_SelectByNames_Empty(t *testing.T) {
	s := NewSelector(newTestSkills())
	result := s.SelectByNames([]string{})

	if result != "" {
		t.Errorf("SelectByNames with empty names should return empty string, got %q", result)
	}
}

func TestSelector_SelectByNames_Nil(t *testing.T) {
	s := NewSelector(newTestSkills())
	result := s.SelectByNames(nil)

	if result != "" {
		t.Errorf("SelectByNames with nil names should return empty string, got %q", result)
	}
}
