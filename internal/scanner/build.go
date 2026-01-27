package scanner

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// buildSystemInfo holds detection info for a build system.
type buildSystemInfo struct {
	name          string
	buildCommands []string
	testCommands  []string
	lintCommands  []string
}

// detectBuildSystem detects the build system and associated commands.
func detectBuildSystem(rootDir string) (buildSystem string, buildCmds, testCmds, lintCmds []string) {
	// Check for Go
	if fileExists(filepath.Join(rootDir, "go.mod")) {
		info := detectGoBuild(rootDir)
		return info.name, info.buildCommands, info.testCommands, info.lintCommands
	}

	// Check for Node.js
	if fileExists(filepath.Join(rootDir, "package.json")) {
		info := detectNodeBuild(rootDir)
		return info.name, info.buildCommands, info.testCommands, info.lintCommands
	}

	// Check for Rust
	if fileExists(filepath.Join(rootDir, "Cargo.toml")) {
		return "cargo", []string{"cargo build"}, []string{"cargo test"}, []string{"cargo clippy"}
	}

	// Check for Python
	if fileExists(filepath.Join(rootDir, "pyproject.toml")) {
		return "poetry/pip", []string{"poetry install", "pip install -e ."}, []string{"pytest"}, []string{"ruff check .", "black --check ."}
	}
	if fileExists(filepath.Join(rootDir, "setup.py")) || fileExists(filepath.Join(rootDir, "requirements.txt")) {
		return "pip", []string{"pip install -e .", "pip install -r requirements.txt"}, []string{"pytest"}, []string{"ruff check .", "black --check ."}
	}

	// Check for Java/Maven
	if fileExists(filepath.Join(rootDir, "pom.xml")) {
		return "maven", []string{"mvn compile"}, []string{"mvn test"}, []string{"mvn checkstyle:check"}
	}

	// Check for Java/Gradle
	if fileExists(filepath.Join(rootDir, "build.gradle")) || fileExists(filepath.Join(rootDir, "build.gradle.kts")) {
		return "gradle", []string{"./gradlew build"}, []string{"./gradlew test"}, []string{"./gradlew check"}
	}

	// Check for Ruby/Bundler
	if fileExists(filepath.Join(rootDir, "Gemfile")) {
		return "bundler", []string{"bundle install"}, []string{"bundle exec rspec"}, []string{"bundle exec rubocop"}
	}

	// Check for Makefile (generic)
	if fileExists(filepath.Join(rootDir, "Makefile")) {
		info := detectMakeTargets(rootDir)
		return info.name, info.buildCommands, info.testCommands, info.lintCommands
	}

	return "", nil, nil, nil
}

func detectGoBuild(rootDir string) buildSystemInfo {
	info := buildSystemInfo{
		name:          "go",
		buildCommands: []string{"go build ./..."},
		testCommands:  []string{"go test ./..."},
		lintCommands:  []string{"go vet ./..."},
	}

	// Check for golangci-lint config
	if fileExists(filepath.Join(rootDir, ".golangci.yml")) || fileExists(filepath.Join(rootDir, ".golangci.yaml")) {
		info.lintCommands = []string{"golangci-lint run"}
	}

	// Check for Makefile with common targets
	if fileExists(filepath.Join(rootDir, "Makefile")) {
		targets := parseMakefileTargets(rootDir)
		if contains(targets, "build") {
			info.buildCommands = []string{"make build"}
		}
		if contains(targets, "test") {
			info.testCommands = []string{"make test"}
		}
		if contains(targets, "lint") {
			info.lintCommands = []string{"make lint"}
		}
	}

	return info
}

func detectNodeBuild(rootDir string) buildSystemInfo {
	info := buildSystemInfo{
		name: "npm",
	}

	// Parse package.json for scripts
	pkgPath := filepath.Join(rootDir, "package.json")
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		return info
	}

	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return info
	}

	// Check for package manager
	if fileExists(filepath.Join(rootDir, "pnpm-lock.yaml")) {
		info.name = "pnpm"
	} else if fileExists(filepath.Join(rootDir, "yarn.lock")) {
		info.name = "yarn"
	} else if fileExists(filepath.Join(rootDir, "bun.lockb")) {
		info.name = "bun"
	}

	runner := info.name
	if runner == "npm" {
		runner = "npm run"
	}

	// Detect build commands
	for _, script := range []string{"build", "compile"} {
		if _, ok := pkg.Scripts[script]; ok {
			info.buildCommands = append(info.buildCommands, runner+" "+script)
			break
		}
	}

	// Detect test commands
	for _, script := range []string{"test", "test:unit", "jest", "vitest"} {
		if _, ok := pkg.Scripts[script]; ok {
			info.testCommands = append(info.testCommands, runner+" "+script)
			break
		}
	}

	// Detect lint commands
	for _, script := range []string{"lint", "eslint", "check"} {
		if _, ok := pkg.Scripts[script]; ok {
			info.lintCommands = append(info.lintCommands, runner+" "+script)
			break
		}
	}

	return info
}

func detectMakeTargets(rootDir string) buildSystemInfo {
	info := buildSystemInfo{name: "make"}
	targets := parseMakefileTargets(rootDir)

	for _, t := range []string{"build", "all"} {
		if contains(targets, t) {
			info.buildCommands = []string{"make " + t}
			break
		}
	}

	for _, t := range []string{"test", "check"} {
		if contains(targets, t) {
			info.testCommands = []string{"make " + t}
			break
		}
	}

	for _, t := range []string{"lint", "vet"} {
		if contains(targets, t) {
			info.lintCommands = []string{"make " + t}
			break
		}
	}

	return info
}

func parseMakefileTargets(rootDir string) []string {
	makefile := filepath.Join(rootDir, "Makefile")
	f, err := os.Open(makefile)
	if err != nil {
		return nil
	}
	defer f.Close()

	var targets []string
	targetRegex := regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_-]*):`)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if matches := targetRegex.FindStringSubmatch(line); len(matches) > 1 {
			target := matches[1]
			// Skip internal targets (starting with .)
			if !strings.HasPrefix(target, ".") {
				targets = append(targets, target)
			}
		}
	}

	return targets
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
