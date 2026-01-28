package scanner

import (
	"os"
	"path/filepath"
	"strings"
)

// Common source directory names
var sourceDirNames = map[string]bool{
	"src":        true,
	"lib":        true,
	"pkg":        true,
	"internal":   true,
	"app":        true,
	"cmd":        true,
	"core":       true,
	"components": true,
	"pages":      true,
}

// Common test directory names
var testDirNames = map[string]bool{
	"test":        true,
	"tests":       true,
	"spec":        true,
	"specs":       true,
	"__tests__":   true,
	"test_data":   true,
	"testdata":    true,
	"e2e":         true,
	"integration": true,
}

// Config file patterns
var configPatterns = []string{
	".agentium.yaml",
	".agentium.yml",
	".env",
	".env.example",
	"*.config.js",
	"*.config.ts",
	"tsconfig.json",
	"jest.config.*",
	"vitest.config.*",
	".eslintrc*",
	".prettierrc*",
	"docker-compose.yml",
	"docker-compose.yaml",
	"Dockerfile",
}

// detectStructure analyzes the project directory layout.
func detectStructure(rootDir string) ProjectStructure {
	structure := ProjectStructure{}

	// Get immediate subdirectories
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		return structure
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}

		if sourceDirNames[name] {
			structure.SourceDirs = append(structure.SourceDirs, name)
		}
		if testDirNames[name] {
			structure.TestDirs = append(structure.TestDirs, name)
		}
	}

	// Detect config files
	structure.ConfigFiles = detectConfigFiles(rootDir)

	// Detect entry points
	structure.EntryPoints = detectEntryPoints(rootDir)

	// Check for Docker
	structure.HasDocker = fileExists(filepath.Join(rootDir, "Dockerfile")) ||
		fileExists(filepath.Join(rootDir, "docker-compose.yml")) ||
		fileExists(filepath.Join(rootDir, "docker-compose.yaml"))

	// Check for CI
	structure.HasCI, structure.CISystem = detectCI(rootDir)

	return structure
}

func detectConfigFiles(rootDir string) []string {
	var configs []string

	entries, err := os.ReadDir(rootDir)
	if err != nil {
		return configs
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()

		// Check direct matches
		for _, pattern := range configPatterns {
			if matched, _ := filepath.Match(pattern, name); matched {
				configs = append(configs, name)
				break
			}
		}
	}

	return configs
}

func detectEntryPoints(rootDir string) []string {
	var entryPoints []string

	// Go entry points
	cmdDir := filepath.Join(rootDir, "cmd")
	if entries, err := os.ReadDir(cmdDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				mainFile := filepath.Join(cmdDir, entry.Name(), "main.go")
				if fileExists(mainFile) {
					entryPoints = append(entryPoints, "cmd/"+entry.Name()+"/main.go")
				}
			}
		}
	}

	// Root main.go
	if fileExists(filepath.Join(rootDir, "main.go")) {
		entryPoints = append(entryPoints, "main.go")
	}

	// Python entry points
	if fileExists(filepath.Join(rootDir, "main.py")) {
		entryPoints = append(entryPoints, "main.py")
	}
	if fileExists(filepath.Join(rootDir, "app.py")) {
		entryPoints = append(entryPoints, "app.py")
	}
	if fileExists(filepath.Join(rootDir, "__main__.py")) {
		entryPoints = append(entryPoints, "__main__.py")
	}

	// Node.js entry points
	if fileExists(filepath.Join(rootDir, "index.js")) {
		entryPoints = append(entryPoints, "index.js")
	}
	if fileExists(filepath.Join(rootDir, "index.ts")) {
		entryPoints = append(entryPoints, "index.ts")
	}
	if fileExists(filepath.Join(rootDir, "src", "index.ts")) {
		entryPoints = append(entryPoints, "src/index.ts")
	}
	if fileExists(filepath.Join(rootDir, "src", "index.js")) {
		entryPoints = append(entryPoints, "src/index.js")
	}
	if fileExists(filepath.Join(rootDir, "src", "main.ts")) {
		entryPoints = append(entryPoints, "src/main.ts")
	}
	if fileExists(filepath.Join(rootDir, "src", "main.js")) {
		entryPoints = append(entryPoints, "src/main.js")
	}

	return entryPoints
}

func detectCI(rootDir string) (hasCI bool, ciSystem string) {
	// GitHub Actions
	if dirExists(filepath.Join(rootDir, ".github", "workflows")) {
		return true, "github-actions"
	}

	// GitLab CI
	if fileExists(filepath.Join(rootDir, ".gitlab-ci.yml")) {
		return true, "gitlab-ci"
	}

	// CircleCI
	if fileExists(filepath.Join(rootDir, ".circleci", "config.yml")) {
		return true, "circleci"
	}

	// Travis CI
	if fileExists(filepath.Join(rootDir, ".travis.yml")) {
		return true, "travis-ci"
	}

	// Jenkins
	if fileExists(filepath.Join(rootDir, "Jenkinsfile")) {
		return true, "jenkins"
	}

	return false, ""
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
