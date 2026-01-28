package scanner

import (
	"os"
	"path/filepath"
)

const maxFiles = 10000

// Scanner analyzes a project directory to detect its characteristics.
type Scanner struct {
	rootDir string
}

// New creates a new Scanner for the given root directory.
func New(rootDir string) *Scanner {
	return &Scanner{rootDir: rootDir}
}

// Scan analyzes the project and returns detected information.
func (s *Scanner) Scan() (*ProjectInfo, error) {
	info := &ProjectInfo{
		Name: filepath.Base(s.rootDir),
	}

	// Collect file extensions
	extCounts := make(map[string]int)
	fileCount := 0

	err := filepath.Walk(s.rootDir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip inaccessible files
		}

		// Skip common non-source directories
		if fi.IsDir() {
			name := fi.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" ||
				name == ".venv" || name == "__pycache__" || name == "dist" ||
				name == "build" || name == "target" || name == ".next" {
				return filepath.SkipDir
			}
			return nil
		}

		if fileCount >= maxFiles {
			return filepath.SkipAll
		}
		fileCount++

		ext := filepath.Ext(path)
		if ext != "" {
			extCounts[ext]++
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Detect languages from extensions
	info.Languages = detectLanguages(extCounts)

	// Detect build system and commands
	info.BuildSystem, info.BuildCommands, info.TestCommands, info.LintCommands = detectBuildSystem(s.rootDir)

	// Detect project structure
	info.Structure = detectStructure(s.rootDir)

	// Detect framework
	info.Framework, info.Dependencies = detectFramework(s.rootDir, info.Languages)

	return info, nil
}
