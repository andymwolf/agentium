// Package scanner provides project analysis functionality for detecting
// language, build systems, and project structure.
package scanner

// LanguageInfo contains information about a detected programming language.
type LanguageInfo struct {
	Name       string   `json:"name"`
	FileCount  int      `json:"file_count"`
	Percentage float64  `json:"percentage"`
	Extensions []string `json:"extensions"`
}

// ProjectStructure contains information about the project's directory layout.
type ProjectStructure struct {
	SourceDirs  []string `json:"source_dirs"`
	TestDirs    []string `json:"test_dirs"`
	ConfigFiles []string `json:"config_files"`
	EntryPoints []string `json:"entry_points"`
	HasDocker   bool     `json:"has_docker"`
	HasCI       bool     `json:"has_ci"`
	CISystem    string   `json:"ci_system,omitempty"`
}

// ProjectInfo contains all detected information about a project.
type ProjectInfo struct {
	Name          string           `json:"name"`
	Languages     []LanguageInfo   `json:"languages"`
	BuildSystem   string           `json:"build_system"`
	BuildCommands []string         `json:"build_commands"`
	TestCommands  []string         `json:"test_commands"`
	LintCommands  []string         `json:"lint_commands"`
	Structure     ProjectStructure `json:"structure"`
	Dependencies  []string         `json:"dependencies"`
	Framework     string           `json:"framework,omitempty"`
}
