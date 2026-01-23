package skills

import "strings"

// Selector provides phase-aware skill composition.
type Selector struct {
	skills []Skill
}

// NewSelector creates a Selector from a sorted slice of loaded skills.
func NewSelector(skills []Skill) *Selector {
	return &Selector{skills: skills}
}

// SelectForPhase composes matching skills for the given phase into a single prompt string.
// Skills with empty Phases are always included (universal skills).
// Results are ordered by priority with "\n\n" separators.
func (s *Selector) SelectForPhase(phase string) string {
	var parts []string
	for _, skill := range s.skills {
		if s.matchesPhase(skill, phase) {
			parts = append(parts, skill.Content)
		}
	}
	return strings.Join(parts, "\n\n")
}

// SkillsForPhase returns the names of skills that match the given phase.
func (s *Selector) SkillsForPhase(phase string) []string {
	var names []string
	for _, skill := range s.skills {
		if s.matchesPhase(skill, phase) {
			names = append(names, skill.Entry.Name)
		}
	}
	return names
}

// matchesPhase returns true if the skill applies to the given phase.
// Skills with empty Phases are universal and always match.
func (s *Selector) matchesPhase(skill Skill, phase string) bool {
	if len(skill.Entry.Phases) == 0 {
		return true
	}
	for _, p := range skill.Entry.Phases {
		if p == phase {
			return true
		}
	}
	return false
}
