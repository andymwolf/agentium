package skills

//go:generate cp ../../prompts/skills/manifest.yaml manifest.yaml
//go:generate cp ../../prompts/skills/safety.md safety.md
//go:generate cp ../../prompts/skills/environment.md environment.md
//go:generate cp ../../prompts/skills/status_signals.md status_signals.md
//go:generate cp ../../prompts/skills/planning.md planning.md
//go:generate cp ../../prompts/skills/implement.md implement.md
//go:generate cp ../../prompts/skills/test.md test.md
//go:generate cp ../../prompts/skills/pr_creation.md pr_creation.md
//go:generate cp ../../prompts/skills/pr_review.md pr_review.md

import (
	_ "embed"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

//go:embed manifest.yaml
var embeddedManifest string

//go:embed safety.md
var embeddedSafety string

//go:embed environment.md
var embeddedEnvironment string

//go:embed status_signals.md
var embeddedStatusSignals string

//go:embed planning.md
var embeddedPlanning string

//go:embed implement.md
var embeddedImplement string

//go:embed test.md
var embeddedTest string

//go:embed pr_creation.md
var embeddedPRCreation string

//go:embed pr_review.md
var embeddedPRReview string

// skillFiles maps filenames to their embedded content.
var skillFiles = map[string]string{
	"safety.md":         embeddedSafety,
	"environment.md":    embeddedEnvironment,
	"status_signals.md": embeddedStatusSignals,
	"planning.md":       embeddedPlanning,
	"implement.md":      embeddedImplement,
	"test.md":           embeddedTest,
	"pr_creation.md":    embeddedPRCreation,
	"pr_review.md":      embeddedPRReview,
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
