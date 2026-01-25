package controller

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDetectProjectLanguage tests the language detection logic
func TestDetectProjectLanguage(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		expected string
	}{
		{
			name:     "Go project",
			files:    []string{"go.mod", "main.go"},
			expected: "go",
		},
		{
			name:     "Rust project",
			files:    []string{"Cargo.toml", "src/main.rs"},
			expected: "rust",
		},
		{
			name:     "Java Maven project",
			files:    []string{"pom.xml", "src/main/java/App.java"},
			expected: "java",
		},
		{
			name:     "Java Gradle project",
			files:    []string{"build.gradle", "src/main/java/App.java"},
			expected: "java",
		},
		{
			name:     "Ruby project",
			files:    []string{"Gemfile", "app.rb"},
			expected: "ruby",
		},
		{
			name:     "DotNet project",
			files:    []string{"MyApp.csproj", "Program.cs"},
			expected: "dotnet",
		},
		{
			name:     "Node.js project",
			files:    []string{"package.json", "index.js"},
			expected: "nodejs",
		},
		{
			name:     "Python project with requirements.txt",
			files:    []string{"requirements.txt", "app.py"},
			expected: "python",
		},
		{
			name:     "Python project with pyproject.toml",
			files:    []string{"pyproject.toml", "src/main.py"},
			expected: "python",
		},
		{
			name:     "Unknown project",
			files:    []string{"README.md", "LICENSE"},
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory
			tmpDir := t.TempDir()

			// Create test files
			for _, file := range tt.files {
				path := filepath.Join(tmpDir, file)
				dir := filepath.Dir(path)
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatalf("Failed to create directory: %v", err)
				}
				if err := os.WriteFile(path, []byte("test content"), 0644); err != nil {
					t.Fatalf("Failed to create file %s: %v", file, err)
				}
			}

			// Test detection
			detected := detectProjectLanguage(tmpDir)
			if detected != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, detected)
			}
		})
	}
}

// detectProjectLanguage simulates the detection logic from the shell script
func detectProjectLanguage(dir string) string {
	// Check for Go
	if fileExists(filepath.Join(dir, "go.mod")) {
		return "go"
	}

	// Check for Rust
	if fileExists(filepath.Join(dir, "Cargo.toml")) {
		return "rust"
	}

	// Check for Java
	if fileExists(filepath.Join(dir, "pom.xml")) ||
		fileExists(filepath.Join(dir, "build.gradle")) ||
		fileExists(filepath.Join(dir, "build.gradle.kts")) {
		return "java"
	}

	// Check for Ruby
	if fileExists(filepath.Join(dir, "Gemfile")) {
		return "ruby"
	}

	// Check for .NET
	files, _ := filepath.Glob(filepath.Join(dir, "*.csproj"))
	if len(files) > 0 {
		return "dotnet"
	}
	files, _ = filepath.Glob(filepath.Join(dir, "*.sln"))
	if len(files) > 0 {
		return "dotnet"
	}

	// Check for Node.js
	if fileExists(filepath.Join(dir, "package.json")) {
		return "nodejs"
	}

	// Check for Python
	if fileExists(filepath.Join(dir, "requirements.txt")) ||
		fileExists(filepath.Join(dir, "pyproject.toml")) ||
		fileExists(filepath.Join(dir, "setup.py")) {
		return "python"
	}

	return "unknown"
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}