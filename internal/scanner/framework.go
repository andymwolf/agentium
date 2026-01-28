package scanner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// detectFramework detects the framework used in the project.
func detectFramework(rootDir string, languages []LanguageInfo) (framework string, dependencies []string) {
	if len(languages) == 0 {
		return "", nil
	}

	primary := languages[0].Name

	switch primary {
	case "Go":
		return detectGoFramework(rootDir)
	case "JavaScript", "TypeScript":
		return detectJSFramework(rootDir)
	case "Python":
		return detectPythonFramework(rootDir)
	case "Ruby":
		return detectRubyFramework(rootDir)
	case "Rust":
		return detectRustFramework(rootDir)
	case "Java", "Kotlin":
		return detectJavaFramework(rootDir)
	}

	return "", nil
}

// goFramework represents a framework with its detection patterns.
type goFramework struct {
	name     string
	patterns []string
}

// goFrameworks defines frameworks in priority order (first match wins).
// Web frameworks are prioritized over CLI frameworks since they're more
// specific to the project's purpose.
var goFrameworks = []goFramework{
	{name: "gin", patterns: []string{"github.com/gin-gonic/gin"}},
	{name: "echo", patterns: []string{"github.com/labstack/echo"}},
	{name: "fiber", patterns: []string{"github.com/gofiber/fiber"}},
	{name: "chi", patterns: []string{"github.com/go-chi/chi"}},
	{name: "gorilla", patterns: []string{"github.com/gorilla/mux"}},
	{name: "cobra", patterns: []string{"github.com/spf13/cobra"}},
}

func detectGoFramework(rootDir string) (string, []string) {
	goModPath := filepath.Join(rootDir, "go.mod")
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return "", nil
	}

	content := string(data)
	var deps []string

	// Detect framework using ordered list (first match wins for determinism)
	var detected string
	for _, fw := range goFrameworks {
		for _, pattern := range fw.patterns {
			if strings.Contains(content, pattern) {
				if detected == "" {
					detected = fw.name
				}
				deps = append(deps, pattern)
				break // Found this framework, check next
			}
		}
	}

	// Extract more dependencies
	requireRegex := regexp.MustCompile(`require\s+([^\s]+)\s+v`)
	matches := requireRegex.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) > 1 {
			deps = append(deps, match[1])
		}
	}

	return detected, deps
}

func detectJSFramework(rootDir string) (string, []string) {
	pkgPath := filepath.Join(rootDir, "package.json")
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		return "", nil
	}

	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return "", nil
	}

	allDeps := make(map[string]bool)
	for dep := range pkg.Dependencies {
		allDeps[dep] = true
	}
	for dep := range pkg.DevDependencies {
		allDeps[dep] = true
	}

	var deps []string
	for dep := range allDeps {
		deps = append(deps, dep)
	}

	// Framework detection
	if allDeps["next"] {
		return "next.js", deps
	}
	if allDeps["react"] {
		if allDeps["vite"] {
			return "react+vite", deps
		}
		return "react", deps
	}
	if allDeps["vue"] {
		if allDeps["nuxt"] {
			return "nuxt", deps
		}
		return "vue", deps
	}
	if allDeps["svelte"] {
		if allDeps["@sveltejs/kit"] {
			return "sveltekit", deps
		}
		return "svelte", deps
	}
	if allDeps["express"] {
		return "express", deps
	}
	if allDeps["fastify"] {
		return "fastify", deps
	}
	if allDeps["nest"] || allDeps["@nestjs/core"] {
		return "nestjs", deps
	}
	if allDeps["angular"] || allDeps["@angular/core"] {
		return "angular", deps
	}

	return "", deps
}

func detectPythonFramework(rootDir string) (string, []string) {
	var deps []string

	// Check pyproject.toml
	pyprojectPath := filepath.Join(rootDir, "pyproject.toml")
	if data, err := os.ReadFile(pyprojectPath); err == nil {
		content := string(data)
		if strings.Contains(content, "django") {
			return "django", deps
		}
		if strings.Contains(content, "fastapi") {
			return "fastapi", deps
		}
		if strings.Contains(content, "flask") {
			return "flask", deps
		}
	}

	// Check requirements.txt
	reqPath := filepath.Join(rootDir, "requirements.txt")
	if data, err := os.ReadFile(reqPath); err == nil {
		content := strings.ToLower(string(data))
		if strings.Contains(content, "django") {
			return "django", deps
		}
		if strings.Contains(content, "fastapi") {
			return "fastapi", deps
		}
		if strings.Contains(content, "flask") {
			return "flask", deps
		}
	}

	return "", deps
}

func detectRubyFramework(rootDir string) (string, []string) {
	gemfilePath := filepath.Join(rootDir, "Gemfile")
	data, err := os.ReadFile(gemfilePath)
	if err != nil {
		return "", nil
	}

	content := string(data)
	if strings.Contains(content, "rails") {
		return "rails", nil
	}
	if strings.Contains(content, "sinatra") {
		return "sinatra", nil
	}

	return "", nil
}

func detectRustFramework(rootDir string) (string, []string) {
	cargoPath := filepath.Join(rootDir, "Cargo.toml")
	data, err := os.ReadFile(cargoPath)
	if err != nil {
		return "", nil
	}

	content := string(data)
	if strings.Contains(content, "actix-web") {
		return "actix-web", nil
	}
	if strings.Contains(content, "axum") {
		return "axum", nil
	}
	if strings.Contains(content, "rocket") {
		return "rocket", nil
	}
	if strings.Contains(content, "warp") {
		return "warp", nil
	}

	return "", nil
}

func detectJavaFramework(rootDir string) (string, []string) {
	// Check pom.xml
	pomPath := filepath.Join(rootDir, "pom.xml")
	if data, err := os.ReadFile(pomPath); err == nil {
		content := string(data)
		if strings.Contains(content, "spring-boot") {
			return "spring-boot", nil
		}
		if strings.Contains(content, "quarkus") {
			return "quarkus", nil
		}
		if strings.Contains(content, "micronaut") {
			return "micronaut", nil
		}
	}

	// Check build.gradle
	gradlePath := filepath.Join(rootDir, "build.gradle")
	if data, err := os.ReadFile(gradlePath); err == nil {
		content := string(data)
		if strings.Contains(content, "spring-boot") {
			return "spring-boot", nil
		}
		if strings.Contains(content, "quarkus") {
			return "quarkus", nil
		}
	}

	return "", nil
}
