// Package template provides Mustache-style template rendering for agent prompts.
package template

import (
	"regexp"
)

// variablePattern matches Mustache-style {{variable}} placeholders.
// It captures the variable name inside the double braces.
var variablePattern = regexp.MustCompile(`\{\{([a-zA-Z_][a-zA-Z0-9_]*)\}\}`)

// RenderPrompt substitutes Mustache-style {{variable}} placeholders in the prompt
// with values from the provided variables map. Unknown variables (those not in the map)
// are left as-is in the output.
func RenderPrompt(prompt string, variables map[string]string) string {
	if len(variables) == 0 {
		return prompt
	}

	return variablePattern.ReplaceAllStringFunc(prompt, func(match string) string {
		// Extract variable name from {{name}}
		submatches := variablePattern.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		varName := submatches[1]

		// Look up value in variables map
		if value, ok := variables[varName]; ok {
			return value
		}

		// Unknown variable: leave as-is
		return match
	})
}

// MergeVariables merges built-in variables with user-provided parameters.
// User-provided parameters take precedence over built-in variables on name collision.
func MergeVariables(builtins, userParams map[string]string) map[string]string {
	if len(builtins) == 0 && len(userParams) == 0 {
		return nil
	}

	result := make(map[string]string, len(builtins)+len(userParams))

	// Copy builtins first
	for k, v := range builtins {
		result[k] = v
	}

	// User params override builtins
	for k, v := range userParams {
		result[k] = v
	}

	return result
}
