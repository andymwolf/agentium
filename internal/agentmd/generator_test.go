package agentmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andywolf/agentium/internal/scanner"
)

func TestGenerator_Generate(t *testing.T) {
	gen, err := NewGenerator()
	if err != nil {
		t.Fatal(err)
	}

	info := &scanner.ProjectInfo{
		Name:        "my-project",
		Framework:   "gin",
		BuildSystem: "go",
		Languages: []scanner.LanguageInfo{
			{Name: "Go", FileCount: 50, Percentage: 90},
			{Name: "Shell", FileCount: 5, Percentage: 10},
		},
		BuildCommands: []string{"go build ./..."},
		TestCommands:  []string{"go test ./..."},
		LintCommands:  []string{"golangci-lint run"},
		Structure: scanner.ProjectStructure{
			SourceDirs:  []string{"internal", "cmd"},
			TestDirs:    []string{"test"},
			EntryPoints: []string{"cmd/app/main.go"},
			ConfigFiles: []string{".agentium.yaml", "Makefile"},
			HasDocker:   true,
			HasCI:       true,
			CISystem:    "github-actions",
		},
		Dependencies: []string{"github.com/gin-gonic/gin", "github.com/spf13/cobra"},
	}

	content, err := gen.Generate(info)
	if err != nil {
		t.Fatal(err)
	}

	// Verify markers
	if !strings.Contains(content, GeneratedStartMarker) {
		t.Error("missing start marker")
	}
	if !strings.Contains(content, GeneratedEndMarker) {
		t.Error("missing end marker")
	}

	// Verify content
	if !strings.Contains(content, "my-project") {
		t.Error("missing project name")
	}
	if !strings.Contains(content, "gin") {
		t.Error("missing framework")
	}
	if !strings.Contains(content, "Go (90%)") {
		t.Error("missing language info")
	}
	if !strings.Contains(content, "go build ./...") {
		t.Error("missing build command")
	}
	if !strings.Contains(content, "github-actions") {
		t.Error("missing CI system")
	}
}

func TestGenerator_WriteToProject(t *testing.T) {
	gen, err := NewGenerator()
	if err != nil {
		t.Fatal(err)
	}

	tmpDir := t.TempDir()

	info := &scanner.ProjectInfo{
		Name:          "test-project",
		BuildSystem:   "go",
		Languages:     []scanner.LanguageInfo{{Name: "Go", FileCount: 10, Percentage: 100}},
		BuildCommands: []string{"go build ./..."},
		TestCommands:  []string{"go test ./..."},
	}

	// First write - should create file with custom section
	if err := gen.WriteToProject(tmpDir, info); err != nil {
		t.Fatal(err)
	}

	agentMDPath := filepath.Join(tmpDir, AgentiumDir, AgentMDFile)
	content, err := os.ReadFile(agentMDPath)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(content), "Custom Instructions") {
		t.Error("missing custom section in new file")
	}

	// Modify custom section
	modifiedContent := string(content) + "\n\n## My Custom Section\n\nThis should be preserved."
	if err := os.WriteFile(agentMDPath, []byte(modifiedContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Regenerate - should preserve custom content
	info.Name = "updated-project"
	if err := gen.WriteToProject(tmpDir, info); err != nil {
		t.Fatal(err)
	}

	updatedContent, err := os.ReadFile(agentMDPath)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(updatedContent), "updated-project") {
		t.Error("generated content not updated")
	}
	if !strings.Contains(string(updatedContent), "My Custom Section") {
		t.Error("custom section not preserved")
	}
}

func TestParser_Parse(t *testing.T) {
	tests := []struct {
		name           string
		content        string
		wantHasMarkers bool
		wantCustom     bool
	}{
		{
			name: "with markers and custom content",
			content: `<!-- agentium:generated:start -->
Generated content here
<!-- agentium:generated:end -->

## Custom Section
My custom content`,
			wantHasMarkers: true,
			wantCustom:     true,
		},
		{
			name:           "without markers",
			content:        "Just some plain content",
			wantHasMarkers: false,
			wantCustom:     true,
		},
		{
			name: "markers only",
			content: `<!-- agentium:generated:start -->
Generated content
<!-- agentium:generated:end -->`,
			wantHasMarkers: true,
			wantCustom:     false,
		},
	}

	parser := &Parser{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := parser.Parse(tt.content)
			if err != nil {
				t.Fatal(err)
			}

			if parsed.HasMarkers != tt.wantHasMarkers {
				t.Errorf("HasMarkers = %v, want %v", parsed.HasMarkers, tt.wantHasMarkers)
			}

			hasCustom := parsed.HasCustomContent()
			if hasCustom != tt.wantCustom {
				t.Errorf("HasCustomContent = %v, want %v", hasCustom, tt.wantCustom)
			}
		})
	}
}

func TestGenerateGreenfield(t *testing.T) {
	gen, err := NewGenerator()
	if err != nil {
		t.Fatal(err)
	}

	content := gen.GenerateGreenfield("new-project")

	if !strings.Contains(content, "new-project") {
		t.Error("missing project name")
	}
	if !strings.Contains(content, GeneratedStartMarker) {
		t.Error("missing start marker")
	}
	if !strings.Contains(content, GeneratedEndMarker) {
		t.Error("missing end marker")
	}
	if !strings.Contains(content, "agentium refresh") {
		t.Error("missing refresh instruction")
	}
}
