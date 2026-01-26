package skills

// SkillEntry represents a single skill definition from the manifest.
type SkillEntry struct {
	Name     string   `yaml:"name"`
	File     string   `yaml:"file"`
	Priority int      `yaml:"priority"`
	Phases   []string `yaml:"phases"`
}

// Manifest represents the complete skills manifest.
type Manifest struct {
	Skills []SkillEntry `yaml:"skills"`
}

// Skill represents a loaded skill with its content.
type Skill struct {
	Entry   SkillEntry
	Content string
}
