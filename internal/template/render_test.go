package template

import (
	"testing"
)

func TestRenderPrompt(t *testing.T) {
	tests := []struct {
		name      string
		prompt    string
		variables map[string]string
		want      string
	}{
		{
			name:      "empty prompt",
			prompt:    "",
			variables: map[string]string{"foo": "bar"},
			want:      "",
		},
		{
			name:      "no variables",
			prompt:    "Hello world",
			variables: nil,
			want:      "Hello world",
		},
		{
			name:      "empty variables map",
			prompt:    "Hello {{name}}",
			variables: map[string]string{},
			want:      "Hello {{name}}",
		},
		{
			name:      "single substitution",
			prompt:    "Hello {{name}}!",
			variables: map[string]string{"name": "Alice"},
			want:      "Hello Alice!",
		},
		{
			name:      "multiple substitutions",
			prompt:    "{{greeting}}, {{name}}! Welcome to {{place}}.",
			variables: map[string]string{"greeting": "Hello", "name": "Bob", "place": "Agentium"},
			want:      "Hello, Bob! Welcome to Agentium.",
		},
		{
			name:      "unknown variable preserved",
			prompt:    "Hello {{name}}, your ID is {{unknown}}",
			variables: map[string]string{"name": "Charlie"},
			want:      "Hello Charlie, your ID is {{unknown}}",
		},
		{
			name:      "same variable multiple times",
			prompt:    "{{topic}} is great. I love {{topic}}!",
			variables: map[string]string{"topic": "AI"},
			want:      "AI is great. I love AI!",
		},
		{
			name:      "variable at start and end",
			prompt:    "{{start}}middle{{end}}",
			variables: map[string]string{"start": "BEGIN_", "end": "_END"},
			want:      "BEGIN_middle_END",
		},
		{
			name:      "variable with underscores",
			prompt:    "Value: {{my_variable_name}}",
			variables: map[string]string{"my_variable_name": "test_value"},
			want:      "Value: test_value",
		},
		{
			name:      "variable with numbers",
			prompt:    "Value: {{var1}} and {{var2}}",
			variables: map[string]string{"var1": "one", "var2": "two"},
			want:      "Value: one and two",
		},
		{
			name:      "empty value substitution",
			prompt:    "Before{{empty}}After",
			variables: map[string]string{"empty": ""},
			want:      "BeforeAfter",
		},
		{
			name:      "multiline prompt",
			prompt:    "Line 1: {{topic}}\nLine 2: {{subtopic}}\nLine 3: {{topic}} again",
			variables: map[string]string{"topic": "AI", "subtopic": "ML"},
			want:      "Line 1: AI\nLine 2: ML\nLine 3: AI again",
		},
		{
			name:      "value with special characters",
			prompt:    "Query: {{query}}",
			variables: map[string]string{"query": "SELECT * FROM users WHERE name = 'test'"},
			want:      "Query: SELECT * FROM users WHERE name = 'test'",
		},
		{
			name:      "value with newlines",
			prompt:    "Content: {{content}}",
			variables: map[string]string{"content": "line1\nline2\nline3"},
			want:      "Content: line1\nline2\nline3",
		},
		{
			name:      "invalid variable name - starts with number",
			prompt:    "Invalid: {{1var}}",
			variables: map[string]string{"1var": "value"},
			want:      "Invalid: {{1var}}", // Not replaced - invalid variable name
		},
		{
			name:      "invalid variable name - contains dash",
			prompt:    "Invalid: {{my-var}}",
			variables: map[string]string{"my-var": "value"},
			want:      "Invalid: {{my-var}}", // Not replaced - invalid variable name
		},
		{
			name:      "triple braces ignored",
			prompt:    "{{{notvar}}}",
			variables: map[string]string{"notvar": "value"},
			want:      "{value}", // Inner {{notvar}} is replaced, outer braces remain
		},
		{
			name:      "nested braces not valid",
			prompt:    "{{outer{{inner}}}}",
			variables: map[string]string{"inner": "value", "outer": "test"},
			want:      "{{outervalue}}", // Only inner is a valid pattern
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RenderPrompt(tt.prompt, tt.variables)
			if got != tt.want {
				t.Errorf("RenderPrompt() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMergeVariables(t *testing.T) {
	tests := []struct {
		name       string
		builtins   map[string]string
		userParams map[string]string
		wantKeys   []string // Keys that should exist
		wantValues map[string]string
	}{
		{
			name:       "both nil",
			builtins:   nil,
			userParams: nil,
			wantKeys:   nil,
			wantValues: nil,
		},
		{
			name:       "both empty",
			builtins:   map[string]string{},
			userParams: map[string]string{},
			wantKeys:   nil,
			wantValues: nil,
		},
		{
			name:       "only builtins",
			builtins:   map[string]string{"issue_url": "https://github.com/test/repo/issues/1"},
			userParams: nil,
			wantKeys:   []string{"issue_url"},
			wantValues: map[string]string{"issue_url": "https://github.com/test/repo/issues/1"},
		},
		{
			name:       "only user params",
			builtins:   nil,
			userParams: map[string]string{"topic": "AI"},
			wantKeys:   []string{"topic"},
			wantValues: map[string]string{"topic": "AI"},
		},
		{
			name:       "no collision",
			builtins:   map[string]string{"issue_url": "https://github.com/test/repo/issues/1"},
			userParams: map[string]string{"topic": "AI"},
			wantKeys:   []string{"issue_url", "topic"},
			wantValues: map[string]string{
				"issue_url": "https://github.com/test/repo/issues/1",
				"topic":     "AI",
			},
		},
		{
			name:       "user params override builtins",
			builtins:   map[string]string{"issue_url": "builtin_url", "repo": "builtin_repo"},
			userParams: map[string]string{"issue_url": "user_url"},
			wantKeys:   []string{"issue_url", "repo"},
			wantValues: map[string]string{
				"issue_url": "user_url",     // User override
				"repo":      "builtin_repo", // Original builtin
			},
		},
		{
			name:       "multiple overrides",
			builtins:   map[string]string{"a": "1", "b": "2", "c": "3"},
			userParams: map[string]string{"a": "override_a", "c": "override_c"},
			wantKeys:   []string{"a", "b", "c"},
			wantValues: map[string]string{
				"a": "override_a",
				"b": "2",
				"c": "override_c",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergeVariables(tt.builtins, tt.userParams)

			// Check nil case
			if tt.wantKeys == nil {
				if got != nil {
					t.Errorf("MergeVariables() = %v, want nil", got)
				}
				return
			}

			// Check that all expected keys exist with correct values
			for _, key := range tt.wantKeys {
				gotVal, ok := got[key]
				if !ok {
					t.Errorf("MergeVariables() missing key %q", key)
					continue
				}
				wantVal := tt.wantValues[key]
				if gotVal != wantVal {
					t.Errorf("MergeVariables()[%q] = %q, want %q", key, gotVal, wantVal)
				}
			}

			// Check no extra keys
			if len(got) != len(tt.wantKeys) {
				t.Errorf("MergeVariables() has %d keys, want %d", len(got), len(tt.wantKeys))
			}
		})
	}
}

func TestRenderPromptWithMergedVariables(t *testing.T) {
	// Integration test: merge then render
	builtins := map[string]string{
		"issue_url":  "https://github.com/test/repo/issues/42",
		"repository": "test/repo",
	}
	userParams := map[string]string{
		"topic":       "AI-assisted healthcare",
		"competitors": "Jane App, Cliniko",
		"issue_url":   "custom_url", // Override
	}

	prompt := `Research competitors in the {{topic}} space.
Key competitors: {{competitors}}
Issue: {{issue_url}}
Repository: {{repository}}
Unknown: {{unknown_var}}`

	merged := MergeVariables(builtins, userParams)
	result := RenderPrompt(prompt, merged)

	expected := `Research competitors in the AI-assisted healthcare space.
Key competitors: Jane App, Cliniko
Issue: custom_url
Repository: test/repo
Unknown: {{unknown_var}}`

	if result != expected {
		t.Errorf("Integrated render failed:\ngot:\n%s\n\nwant:\n%s", result, expected)
	}
}
