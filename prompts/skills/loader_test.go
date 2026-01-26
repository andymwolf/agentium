package skills

import (
	"strings"
	"testing"
)

func TestLoadManifest(t *testing.T) {
	manifest, err := LoadManifest()
	if err != nil {
		t.Fatalf("LoadManifest() error: %v", err)
	}

	if len(manifest.Skills) == 0 {
		t.Fatal("LoadManifest() returned empty skills list")
	}

	// Verify expected skill names
	expectedNames := []string{
		"safety", "environment", "status_signals",
		"planning", "plan", "implement", "test",
		"pr_creation", "review", "docs", "pr_review",
		"plan_reviewer", "code_reviewer", "judge",
	}

	names := make(map[string]bool)
	for _, s := range manifest.Skills {
		names[s.Name] = true
	}

	for _, name := range expectedNames {
		if !names[name] {
			t.Errorf("LoadManifest() missing expected skill %q", name)
		}
	}
}

func TestLoadManifest_Phases(t *testing.T) {
	manifest, err := LoadManifest()
	if err != nil {
		t.Fatalf("LoadManifest() error: %v", err)
	}

	tests := []struct {
		name       string
		wantPhases []string
	}{
		{"safety", []string{"PLAN", "IMPLEMENT", "REVIEW", "DOCS", "PR_CREATION", "ANALYZE", "PUSH", "PLAN_REVIEW", "IMPLEMENT_REVIEW", "REVIEW_REVIEW", "DOCS_REVIEW"}},
		{"environment", nil},
		{"status_signals", nil},
		{"planning", []string{"IMPLEMENT", "ANALYZE"}},
		{"plan", []string{"PLAN"}},
		{"implement", []string{"IMPLEMENT"}},
		{"test", []string{"IMPLEMENT"}}, // TEST merged into IMPLEMENT
		{"pr_creation", []string{"PR_CREATION"}},
		{"review", []string{"REVIEW"}},
		{"docs", []string{"DOCS"}},
		{"pr_review", []string{"ANALYZE", "PUSH"}},
		{"plan_reviewer", []string{"PLAN_REVIEW"}},
		{"code_reviewer", []string{"IMPLEMENT_REVIEW", "REVIEW_REVIEW", "DOCS_REVIEW"}},
		{"judge", []string{"JUDGE", "PLAN_JUDGE", "IMPLEMENT_JUDGE", "REVIEW_JUDGE", "DOCS_JUDGE"}},
	}

	skillMap := make(map[string]SkillEntry)
	for _, s := range manifest.Skills {
		skillMap[s.Name] = s
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, ok := skillMap[tt.name]
			if !ok {
				t.Fatalf("skill %q not found in manifest", tt.name)
			}

			if tt.wantPhases == nil {
				if len(entry.Phases) != 0 {
					t.Errorf("skill %q phases = %v, want empty (universal)", tt.name, entry.Phases)
				}
			} else {
				if len(entry.Phases) != len(tt.wantPhases) {
					t.Errorf("skill %q phases = %v, want %v", tt.name, entry.Phases, tt.wantPhases)
					return
				}
				for i, p := range tt.wantPhases {
					if entry.Phases[i] != p {
						t.Errorf("skill %q phases[%d] = %q, want %q", tt.name, i, entry.Phases[i], p)
					}
				}
			}
		})
	}
}

func TestLoadSkills(t *testing.T) {
	manifest, err := LoadManifest()
	if err != nil {
		t.Fatalf("LoadManifest() error: %v", err)
	}

	loaded, err := LoadSkills(manifest)
	if err != nil {
		t.Fatalf("LoadSkills() error: %v", err)
	}

	if len(loaded) != len(manifest.Skills) {
		t.Errorf("LoadSkills() returned %d skills, want %d", len(loaded), len(manifest.Skills))
	}

	// Verify all skills have non-empty content
	for _, skill := range loaded {
		if skill.Content == "" {
			t.Errorf("skill %q has empty content", skill.Entry.Name)
		}
	}
}

func TestLoadSkills_PriorityOrder(t *testing.T) {
	manifest, err := LoadManifest()
	if err != nil {
		t.Fatalf("LoadManifest() error: %v", err)
	}

	loaded, err := LoadSkills(manifest)
	if err != nil {
		t.Fatalf("LoadSkills() error: %v", err)
	}

	for i := 1; i < len(loaded); i++ {
		if loaded[i].Entry.Priority < loaded[i-1].Entry.Priority {
			t.Errorf("skills not sorted by priority: %q (priority %d) comes after %q (priority %d)",
				loaded[i].Entry.Name, loaded[i].Entry.Priority,
				loaded[i-1].Entry.Name, loaded[i-1].Entry.Priority)
		}
	}
}

func TestLoadSkills_ContentValidation(t *testing.T) {
	manifest, err := LoadManifest()
	if err != nil {
		t.Fatalf("LoadManifest() error: %v", err)
	}

	loaded, err := LoadSkills(manifest)
	if err != nil {
		t.Fatalf("LoadSkills() error: %v", err)
	}

	// Spot-check that key content exists in expected skills
	contentChecks := map[string]string{
		"safety":         "CRITICAL SAFETY CONSTRAINTS",
		"environment":    "ENVIRONMENT",
		"status_signals": "STATUS SIGNALING",
		"planning":       "Plan Your Approach",
		"plan":           "PLAN PHASE",
		"implement":      "Pre-Flight Check",
		"test":           "Development Loop",
		"pr_creation":    "Push and Create PR",
		"review":         "REVIEW PHASE",
		"docs":           "DOCS PHASE",
		"pr_review":      "PR REVIEW SESSIONS",
		"plan_reviewer":  "PLAN REVIEWER",
		"code_reviewer":  "CODE REVIEWER",
		"judge":          "JUDGE",
	}

	skillMap := make(map[string]Skill)
	for _, s := range loaded {
		skillMap[s.Entry.Name] = s
	}

	for name, expected := range contentChecks {
		t.Run(name, func(t *testing.T) {
			skill, ok := skillMap[name]
			if !ok {
				t.Fatalf("skill %q not found", name)
			}
			if !strings.Contains(skill.Content, expected) {
				t.Errorf("skill %q content does not contain %q", name, expected)
			}
		})
	}
}

func TestLoadSkills_MissingFile(t *testing.T) {
	manifest := &Manifest{
		Skills: []SkillEntry{
			{Name: "missing", File: "nonexistent.md", Priority: 1},
		},
	}

	_, err := LoadSkills(manifest)
	if err == nil {
		t.Fatal("LoadSkills() with missing file should return error")
	}
}
