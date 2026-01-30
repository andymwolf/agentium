package agentmd

import (
	"strings"
)

// ParsedContent represents the parsed sections of an AGENTS.md file.
type ParsedContent struct {
	PreContent       string // Content before the generated section
	GeneratedContent string
	CustomContent    string // Content after the generated section
	HasMarkers       bool
}

// Parser parses AGENTS.md files to extract generated and custom sections.
type Parser struct{}

// Parse splits the content into generated and custom sections.
func (p *Parser) Parse(content string) (*ParsedContent, error) {
	result := &ParsedContent{}

	startIdx := strings.Index(content, GeneratedStartMarker)
	endIdx := strings.Index(content, GeneratedEndMarker)

	if startIdx == -1 || endIdx == -1 {
		// No markers found, treat entire content as custom
		result.CustomContent = content
		result.HasMarkers = false
		return result, nil
	}

	result.HasMarkers = true

	// Extract content before the generated section
	if startIdx > 0 {
		result.PreContent = content[:startIdx]
	}

	// Extract generated content (including markers)
	result.GeneratedContent = content[startIdx : endIdx+len(GeneratedEndMarker)]

	// Extract custom content (everything after end marker)
	afterMarker := endIdx + len(GeneratedEndMarker)
	if afterMarker < len(content) {
		result.CustomContent = content[afterMarker:]
	}

	return result, nil
}

// HasCustomContent returns true if the parsed content has custom sections.
func (p *ParsedContent) HasCustomContent() bool {
	trimmed := strings.TrimSpace(p.CustomContent)
	return len(trimmed) > 0
}
