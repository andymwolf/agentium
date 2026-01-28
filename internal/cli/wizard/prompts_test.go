// Package wizard provides interactive prompts for CLI commands.
package wizard

import (
	"testing"

	"github.com/andywolf/agentium/internal/scanner"
)

func TestParseCommands(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "whitespace only",
			input:    "   ",
			expected: nil,
		},
		{
			name:     "single command",
			input:    "go build",
			expected: []string{"go build"},
		},
		{
			name:     "multiple commands",
			input:    "go build, go test, go vet",
			expected: []string{"go build", "go test", "go vet"},
		},
		{
			name:     "commands with extra whitespace",
			input:    "  go build  ,  go test  ,  go vet  ",
			expected: []string{"go build", "go test", "go vet"},
		},
		{
			name:     "empty items between commas",
			input:    "go build,, go test",
			expected: []string{"go build", "go test"},
		},
		{
			name:     "trailing comma",
			input:    "go build, go test,",
			expected: []string{"go build", "go test"},
		},
		{
			name:     "leading comma",
			input:    ",go build, go test",
			expected: []string{"go build", "go test"},
		},
		{
			name:     "command with args containing spaces",
			input:    "npm run build, npm run test:unit, make -j4",
			expected: []string{"npm run build", "npm run test:unit", "make -j4"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseCommands(tt.input)

			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}

			if len(result) != len(tt.expected) {
				t.Errorf("expected %d items, got %d: %v", len(tt.expected), len(result), result)
				return
			}

			for i, expected := range tt.expected {
				if result[i] != expected {
					t.Errorf("item %d: expected %q, got %q", i, expected, result[i])
				}
			}
		})
	}
}

func TestFormatLanguages(t *testing.T) {
	tests := []struct {
		name      string
		languages []scanner.LanguageInfo
		expected  string
	}{
		{
			name:      "empty languages",
			languages: []scanner.LanguageInfo{},
			expected:  "Unknown",
		},
		{
			name:      "nil languages",
			languages: nil,
			expected:  "Unknown",
		},
		{
			name: "single language",
			languages: []scanner.LanguageInfo{
				{Name: "Go", Percentage: 100},
			},
			expected: "Go (100%)",
		},
		{
			name: "multiple languages",
			languages: []scanner.LanguageInfo{
				{Name: "Go", Percentage: 75},
				{Name: "JavaScript", Percentage: 20},
				{Name: "Shell", Percentage: 5},
			},
			expected: "Go (75%), JavaScript (20%), Shell (5%)",
		},
		{
			name: "fractional percentages",
			languages: []scanner.LanguageInfo{
				{Name: "Python", Percentage: 85.7},
				{Name: "JavaScript", Percentage: 14.3},
			},
			expected: "Python (86%), JavaScript (14%)",
		},
		{
			name: "zero percentage",
			languages: []scanner.LanguageInfo{
				{Name: "Go", Percentage: 99},
				{Name: "Other", Percentage: 0},
			},
			expected: "Go (99%), Other (0%)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatLanguages(tt.languages)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}
