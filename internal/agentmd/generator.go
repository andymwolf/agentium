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

	// File name for agent instructions (now at project root)
	AgentMDFile = "AGENT.md"

	// Deprecated: AgentiumDir is kept for backward compatibility but is no longer used.
	// AGENT.md is now written to the project root instead of .agentium/.
	AgentiumDir = ".agentium"
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

// WriteToProject writes the AGENT.md file to the project root directory.
// If the file already exists, it preserves content outside the generated markers.
func (g *Generator) WriteToProject(rootDir string, info *scanner.ProjectInfo) error {
	agentMDPath := filepath.Join(rootDir, AgentMDFile)

	// Generate new content
	newContent, err := g.Generate(info)
	if err != nil {
		return err
	}

	// Check if file exists
	existingContent, err := os.ReadFile(agentMDPath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, write with default workflow and custom sections
			fullContent := newContent + defaultWorkflowSection + defaultCustomSection
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

	// Combine preserved pre-content, new generated content, and preserved custom content
	finalContent := parsed.PreContent + newContent
	if parsed.CustomContent != "" {
		finalContent += parsed.CustomContent
	} else {
		finalContent += defaultWorkflowSection + defaultCustomSection
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
%s%s`, GeneratedStartMarker, projectName, "`", "`", GeneratedEndMarker, defaultWorkflowSection, defaultCustomSection)
}

const defaultWorkflowSection = `

## Workflow Requirements

### Critical Rules

**NEVER make code changes directly on the ` + "`main`" + ` branch.** All work must follow the branch workflow below.

**ALWAYS create a feature branch BEFORE writing any code.** If you find yourself on ` + "`main`" + `, switch to a feature branch immediately.

**When given a plan or implementation instructions:**
1. First update the relevant GitHub issue(s) with the plan details (use ` + "`gh issue edit`" + `)
2. Create a feature branch
3. Then implement the changes

### Branch and PR Workflow

When implementing any GitHub issue:

1. **Update the GitHub issue** with implementation details if needed:
   ` + "```bash" + `
   gh issue edit <number> --body "$(gh issue view <number> --json body -q .body)

   ## Implementation Plan
   <details added from plan>"
   ` + "```" + `

2. **Create a feature branch** from ` + "`main`" + `:
   ` + "```bash" + `
   git checkout main
   git pull origin main
   git checkout -b <label>/issue-<number>-<short-description>
   ` + "```" + `

3. **Make commits** with clear messages referencing the issue:
   ` + "```bash" + `
   git commit -m "Add feature X

   Implements #<issue-number>"
   ` + "```" + `

4. **Push and create a PR**:
   ` + "```bash" + `
   git push -u origin <branch-name>
   gh pr create --title "..." --body "Closes #<issue-number>"
   ` + "```" + `

5. **Never commit directly to ` + "`main`" + `** - This is strictly enforced.

### Branch Naming Convention

Branch prefixes are determined by the first label on the issue:
- ` + "`<label>/issue-<number>-<short-description>`" + ` - Based on first issue label
- Examples: ` + "`bug/issue-123-fix-auth`" + `, ` + "`enhancement/issue-456-add-cache`" + `, ` + "`feature/issue-789-new-api`" + `
- Default: ` + "`feature/issue-<number>-*`" + ` when no labels are present

### Before Starting Work

1. **Verify you are NOT on main:** ` + "`git branch --show-current`" + `
2. Read the issue description fully
3. Check for dependencies on other issues
4. Ensure you're on a clean working tree
5. Pull latest ` + "`main`" + ` and create feature branch

### Commit Messages

Format:
` + "```" + `
<short summary>

<detailed description if needed>

Closes #<issue-number>
Co-Authored-By: Claude <noreply@anthropic.com>
` + "```" + `
`

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
