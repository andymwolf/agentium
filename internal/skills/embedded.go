package skills

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed gh-issues/SKILL.md
var embeddedSkills embed.FS

// InstallProjectSkills installs Claude Code skills to .claude/skills/
func InstallProjectSkills(rootDir string, force bool) error {
	skills := []string{"gh-issues"}

	for _, skill := range skills {
		if err := installSkill(rootDir, skill, force); err != nil {
			return fmt.Errorf("failed to install skill %s: %w", skill, err)
		}
	}

	return nil
}

func installSkill(rootDir, skillName string, force bool) error {
	skillsDir := filepath.Join(rootDir, ".claude", "skills", skillName)
	skillPath := filepath.Join(skillsDir, "SKILL.md")

	// Check if already exists
	if _, err := os.Stat(skillPath); err == nil && !force {
		return nil // Already installed
	}

	// Read embedded content
	content, err := embeddedSkills.ReadFile(filepath.Join(skillName, "SKILL.md"))
	if err != nil {
		return fmt.Errorf("failed to read embedded skill: %w", err)
	}

	// Create directory
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		return fmt.Errorf("failed to create skills directory: %w", err)
	}

	// Write skill file
	if err := os.WriteFile(skillPath, content, 0644); err != nil {
		return fmt.Errorf("failed to write skill file: %w", err)
	}

	return nil
}
