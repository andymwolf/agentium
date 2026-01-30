package skills

import (
	_ "embed"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

//go:embed manifest.yaml
var embeddedManifest string

//go:embed scope.md
var embeddedScope string

//go:embed safety.md
var embeddedSafety string

//go:embed environment.md
var embeddedEnvironment string

//go:embed status_signals.md
var embeddedStatusSignals string

//go:embed plan.md
var embeddedPlan string

//go:embed implement.md
var embeddedImplement string

//go:embed test.md
var embeddedTest string

//go:embed pr_update.md
var embeddedPRUpdate string

//go:embed docs.md
var embeddedDocs string

//go:embed plan_reviewer.md
var embeddedPlanReviewer string

//go:embed code_reviewer.md
var embeddedCodeReviewer string

//go:embed docs_reviewer.md
var embeddedDocsReviewer string

//go:embed judge.md
var embeddedJudge string

// skillFiles maps filenames to their embedded content.
var skillFiles = map[string]string{
	"scope.md":          embeddedScope,
	"safety.md":         embeddedSafety,
	"environment.md":    embeddedEnvironment,
	"status_signals.md": embeddedStatusSignals,
	"plan.md":           embeddedPlan,
	"implement.md":      embeddedImplement,
	"test.md":           embeddedTest,
	"pr_update.md":      embeddedPRUpdate,
	"docs.md":           embeddedDocs,
	"plan_reviewer.md":  embeddedPlanReviewer,
	"code_reviewer.md":  embeddedCodeReviewer,
	"docs_reviewer.md":  embeddedDocsReviewer,
	"judge.md":          embeddedJudge,
}

// LoadManifest parses the embedded manifest YAML.
func LoadManifest() (*Manifest, error) {
	var manifest Manifest
	if err := yaml.Unmarshal([]byte(embeddedManifest), &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse skills manifest: %w", err)
	}
	return &manifest, nil
}

// LoadSkills loads all skill content from embedded files, sorted by priority.
func LoadSkills(manifest *Manifest) ([]Skill, error) {
	skills := make([]Skill, 0, len(manifest.Skills))

	for _, entry := range manifest.Skills {
		content, ok := skillFiles[entry.File]
		if !ok {
			return nil, fmt.Errorf("skill file %q not found for skill %q", entry.File, entry.Name)
		}
		skills = append(skills, Skill{
			Entry:   entry,
			Content: content,
		})
	}

	sort.Slice(skills, func(i, j int) bool {
		return skills[i].Entry.Priority < skills[j].Entry.Priority
	})

	return skills, nil
}
