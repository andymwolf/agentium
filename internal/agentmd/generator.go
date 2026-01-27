// Package agentmd generates and parses AGENT.md files for AI agents.
package agentmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/andywolf/agentium/internal/scanner"
)

const (
	// Markers for regeneration-safe sections
	GeneratedStartMarker = "<!-- agentium:generated:start -->"
	GeneratedEndMarker   = "<!-- agentium:generated:end -->"

	// Directory and file names
	AgentiumDir  = ".agentium"
	AgentMDFile  = "AGENT.md"
)

// Generator creates AGENT.md files from project info.
type Generator struct {
	tmpl *template.Template
}

// NewGenerator creates a new AGENT.md generator.
func NewGenerator() (*Generator, error) {
	tmpl, err := template.New("agentmd").Parse(agentMDTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}
	return &Generator{tmpl: tmpl}, nil
}

// Generate creates AGENT.md content from project info.
func (g *Generator) Generate(info *scanner.ProjectInfo) (string, error) {
	var buf bytes.Buffer
	if err := g.tmpl.Execute(&buf, info); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}
	return buf.String(), nil
}

// WriteToProject writes the AGENT.md file to the project directory.
// If the file already exists, it preserves content outside the generated markers.
func (g *Generator) WriteToProject(rootDir string, info *scanner.ProjectInfo) error {
	agentiumDir := filepath.Join(rootDir, AgentiumDir)
	if err := os.MkdirAll(agentiumDir, 0755); err != nil {
		return fmt.Errorf("failed to create .agentium directory: %w", err)
	}

	agentMDPath := filepath.Join(agentiumDir, AgentMDFile)

	// Generate new content
	newContent, err := g.Generate(info)
	if err != nil {
		return err
	}

	// Check if file exists
	existingContent, err := os.ReadFile(agentMDPath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, write with default custom section
			fullContent := newContent + defaultCustomSection
			return os.WriteFile(agentMDPath, []byte(fullContent), 0644)
		}
		return fmt.Errorf("failed to read existing AGENT.md: %w", err)
	}

	// Parse existing file and preserve custom sections
	parser := &Parser{}
	parsed, err := parser.Parse(string(existingContent))
	if err != nil {
		return fmt.Errorf("failed to parse existing AGENT.md: %w", err)
	}

	// Combine new generated content with preserved custom content
	finalContent := newContent
	if parsed.CustomContent != "" {
		finalContent += parsed.CustomContent
	} else {
		finalContent += defaultCustomSection
	}

	return os.WriteFile(agentMDPath, []byte(finalContent), 0644)
}

// GenerateGreenfield creates a minimal AGENT.md for a new project.
func (g *Generator) GenerateGreenfield(projectName string) string {
	return fmt.Sprintf(`%s
# %s

This is a new project. Run %sagentium refresh%s after adding code to generate project-specific instructions.

## Project Overview

*Project details will be auto-detected after code is added.*

## Build & Test Commands

*Commands will be detected after build files are added.*

%s

## Custom Instructions

Add any project-specific instructions for AI agents here.

`, GeneratedStartMarker, projectName, "`", "`", GeneratedEndMarker)
}

const defaultCustomSection = `

## Custom Instructions

Add project-specific guidelines for AI agents below. These will be preserved when regenerating.

### Code Style

<!-- Add style guidelines specific to your project -->

### Important Notes

<!-- Add any warnings or special considerations -->

### Off-Limits Areas

<!-- Specify files or directories agents should not modify -->
`
