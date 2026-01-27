package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanner_ScanGoProject(t *testing.T) {
	// Create a temporary Go project
	tmpDir := t.TempDir()

	// Create go.mod
	goMod := `module example.com/myapp

go 1.19

require (
	github.com/spf13/cobra v1.7.0
)
`
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatal(err)
	}

	// Create some Go files
	if err := os.MkdirAll(filepath.Join(tmpDir, "cmd", "app"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "cmd", "app", "main.go"), []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "internal", "pkg"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "internal", "pkg", "util.go"), []byte("package pkg"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create Makefile with targets
	makefile := `build:
	go build ./...

test:
	go test ./...

lint:
	golangci-lint run
`
	if err := os.WriteFile(filepath.Join(tmpDir, "Makefile"), []byte(makefile), 0644); err != nil {
		t.Fatal(err)
	}

	scanner := New(tmpDir)
	info, err := scanner.Scan()
	if err != nil {
		t.Fatal(err)
	}

	// Check project name
	if info.Name != filepath.Base(tmpDir) {
		t.Errorf("expected name %s, got %s", filepath.Base(tmpDir), info.Name)
	}

	// Check language detection
	if len(info.Languages) == 0 {
		t.Fatal("expected at least one language detected")
	}
	if info.Languages[0].Name != "Go" {
		t.Errorf("expected primary language Go, got %s", info.Languages[0].Name)
	}

	// Check build system
	if info.BuildSystem != "go" {
		t.Errorf("expected build system 'go', got %s", info.BuildSystem)
	}

	// Check that Makefile targets were detected
	if len(info.BuildCommands) == 0 || info.BuildCommands[0] != "make build" {
		t.Errorf("expected 'make build' command, got %v", info.BuildCommands)
	}
	if len(info.TestCommands) == 0 || info.TestCommands[0] != "make test" {
		t.Errorf("expected 'make test' command, got %v", info.TestCommands)
	}

	// Check structure
	if len(info.Structure.SourceDirs) == 0 {
		t.Error("expected source dirs to be detected")
	}

	// Check entry points
	found := false
	for _, ep := range info.Structure.EntryPoints {
		if ep == "cmd/app/main.go" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected cmd/app/main.go in entry points, got %v", info.Structure.EntryPoints)
	}

	// Check framework detection (cobra)
	if info.Framework != "cobra" {
		t.Errorf("expected framework 'cobra', got %s", info.Framework)
	}
}

func TestScanner_ScanNodeProject(t *testing.T) {
	tmpDir := t.TempDir()

	// Create package.json
	pkgJSON := `{
	"name": "my-react-app",
	"scripts": {
		"build": "vite build",
		"test": "vitest",
		"lint": "eslint ."
	},
	"dependencies": {
		"react": "^18.0.0",
		"vite": "^5.0.0"
	}
}`
	if err := os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(pkgJSON), 0644); err != nil {
		t.Fatal(err)
	}

	// Create some JS files
	if err := os.MkdirAll(filepath.Join(tmpDir, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "src", "index.tsx"), []byte("export {}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "src", "App.tsx"), []byte("export {}"), 0644); err != nil {
		t.Fatal(err)
	}

	scanner := New(tmpDir)
	info, err := scanner.Scan()
	if err != nil {
		t.Fatal(err)
	}

	// Check language detection
	if len(info.Languages) == 0 {
		t.Fatal("expected at least one language detected")
	}
	if info.Languages[0].Name != "TypeScript" {
		t.Errorf("expected primary language TypeScript, got %s", info.Languages[0].Name)
	}

	// Check build system
	if info.BuildSystem != "npm" {
		t.Errorf("expected build system 'npm', got %s", info.BuildSystem)
	}

	// Check commands from package.json scripts
	if len(info.BuildCommands) == 0 || info.BuildCommands[0] != "npm run build" {
		t.Errorf("expected 'npm run build' command, got %v", info.BuildCommands)
	}
	if len(info.TestCommands) == 0 || info.TestCommands[0] != "npm run test" {
		t.Errorf("expected 'npm run test' command, got %v", info.TestCommands)
	}

	// Check framework
	if info.Framework != "react+vite" {
		t.Errorf("expected framework 'react+vite', got %s", info.Framework)
	}
}

func TestDetectLanguages(t *testing.T) {
	tests := []struct {
		name      string
		extCounts map[string]int
		wantFirst string
	}{
		{
			name:      "Go project",
			extCounts: map[string]int{".go": 50, ".md": 5},
			wantFirst: "Go",
		},
		{
			name:      "TypeScript project",
			extCounts: map[string]int{".ts": 30, ".tsx": 20, ".css": 10},
			wantFirst: "TypeScript",
		},
		{
			name:      "Python project",
			extCounts: map[string]int{".py": 100, ".txt": 5},
			wantFirst: "Python",
		},
		{
			name:      "Mixed JS/TS",
			extCounts: map[string]int{".js": 10, ".ts": 20},
			wantFirst: "TypeScript",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			langs := detectLanguages(tt.extCounts)
			if len(langs) == 0 {
				t.Fatal("expected at least one language")
			}
			if langs[0].Name != tt.wantFirst {
				t.Errorf("expected first language %s, got %s", tt.wantFirst, langs[0].Name)
			}
		})
	}
}

func TestDetectCI(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(dir string)
		wantHasCI  bool
		wantSystem string
	}{
		{
			name: "GitHub Actions",
			setup: func(dir string) {
				os.MkdirAll(filepath.Join(dir, ".github", "workflows"), 0755)
			},
			wantHasCI:  true,
			wantSystem: "github-actions",
		},
		{
			name: "GitLab CI",
			setup: func(dir string) {
				os.WriteFile(filepath.Join(dir, ".gitlab-ci.yml"), []byte(""), 0644)
			},
			wantHasCI:  true,
			wantSystem: "gitlab-ci",
		},
		{
			name:       "No CI",
			setup:      func(dir string) {},
			wantHasCI:  false,
			wantSystem: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tt.setup(tmpDir)

			hasCI, system := detectCI(tmpDir)
			if hasCI != tt.wantHasCI {
				t.Errorf("hasCI = %v, want %v", hasCI, tt.wantHasCI)
			}
			if system != tt.wantSystem {
				t.Errorf("system = %s, want %s", system, tt.wantSystem)
			}
		})
	}
}

func TestPrimaryLanguage(t *testing.T) {
	info := &ProjectInfo{
		Languages: []LanguageInfo{
			{Name: "Go", FileCount: 50},
			{Name: "Shell", FileCount: 5},
		},
	}

	if got := info.PrimaryLanguage(); got != "Go" {
		t.Errorf("PrimaryLanguage() = %s, want Go", got)
	}

	emptyInfo := &ProjectInfo{}
	if got := emptyInfo.PrimaryLanguage(); got != "" {
		t.Errorf("PrimaryLanguage() for empty = %s, want empty", got)
	}
}
