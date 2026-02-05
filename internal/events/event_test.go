package events

import (
	"testing"
)

func TestValidEventTypes(t *testing.T) {
	types := ValidEventTypes()
	if len(types) == 0 {
		t.Error("expected at least one valid event type")
	}

	// Verify expected types are present
	expectedTypes := []EventType{EventText, EventThinking, EventToolUse, EventToolResult, EventCommand, EventFileChange, EventError}
	for _, expected := range expectedTypes {
		found := false
		for _, actual := range types {
			if actual == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected type %q not found in ValidEventTypes()", expected)
		}
	}
}

func TestIsValidEventType(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"text", true},
		{"thinking", true},
		{"tool_use", true},
		{"tool_result", true},
		{"command", true},
		{"file_change", true},
		{"error", true},
		{"invalid", false},
		{"", false},
		{"TEXT", false}, // case sensitive
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := IsValidEventType(tc.input)
			if result != tc.expected {
				t.Errorf("IsValidEventType(%q) = %v, want %v", tc.input, result, tc.expected)
			}
		})
	}
}
