package skills

//go:generate cp ../../prompts/skills/manifest.yaml manifest.yaml
//go:generate cp ../../prompts/skills/safety.md safety.md
//go:generate cp ../../prompts/skills/environment.md environment.md
//go:generate cp ../../prompts/skills/status_signals.md status_signals.md
//go:generate cp ../../prompts/skills/planning.md planning.md
//go:generate cp ../../prompts/skills/plan.md plan.md
//go:generate cp ../../prompts/skills/implement.md implement.md
//go:generate cp ../../prompts/skills/test.md test.md
//go:generate cp ../../prompts/skills/pr_creation.md pr_creation.md
//go:generate cp ../../prompts/skills/review.md review.md
//go:generate cp ../../prompts/skills/docs.md docs.md
//go:generate cp ../../prompts/skills/pr_review.md pr_review.md
//go:generate cp ../../prompts/skills/plan_reviewer.md plan_reviewer.md
//go:generate cp ../../prompts/skills/code_reviewer.md code_reviewer.md
//go:generate cp ../../prompts/skills/judge.md judge.md

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

//go:embed plan.md
var embeddedPlan string

//go:embed implement.md
var embeddedImplement string

//go:embed test.md
var embeddedTest string

//go:embed pr_creation.md
var embeddedPRCreation string

//go:embed review.md
var embeddedReview string

//go:embed docs.md
var embeddedDocs string

//go:embed pr_review.md
var embeddedPRReview string

//go:embed plan_reviewer.md
var embeddedPlanReviewer string

//go:embed code_reviewer.md
var embeddedCodeReviewer string

//go:embed judge.md
var embeddedJudge string

// skillFiles maps filenames to their embedded content.
var skillFiles = map[string]string{
	"safety.md":         embeddedSafety,
	"environment.md":    embeddedEnvironment,
	"status_signals.md": embeddedStatusSignals,
	"planning.md":       embeddedPlanning,
	"plan.md":           embeddedPlan,
	"implement.md":      embeddedImplement,
	"test.md":           embeddedTest,
	"pr_creation.md":    embeddedPRCreation,
	"review.md":         embeddedReview,
	"docs.md":           embeddedDocs,
	"pr_review.md":      embeddedPRReview,
	"plan_reviewer.md":  embeddedPlanReviewer,
	"code_reviewer.md":  embeddedCodeReviewer,
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
