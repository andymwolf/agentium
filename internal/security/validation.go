// Package security provides utilities for securing sensitive information in logs and output.
package security

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// CommandValidator provides validation for shell commands and arguments
type CommandValidator struct {
	// Allowed commands that can be executed
	allowedCommands map[string]bool
	// Pattern for valid identifiers (alphanumeric + dash/underscore)
	identifierPattern *regexp.Regexp
	// Pattern for valid file paths
	pathPattern *regexp.Regexp
}

// NewCommandValidator creates a new command validator with safe defaults
func NewCommandValidator() *CommandValidator {
	return &CommandValidator{
		allowedCommands: map[string]bool{
			"git":     true,
			"gh":      true,
			"docker":  true,
			"gcloud":  true,
			"python":  true,
			"python3": true,
			"node":    true,
			"npm":     true,
			"go":      true,
			"make":    true,
			"bash":    true,
			"sh":      true,
		},
		identifierPattern: regexp.MustCompile(`^[a-zA-Z0-9_-]+$`),
		pathPattern:       regexp.MustCompile(`^[a-zA-Z0-9._/\- ]+$`),
	}
}

// ValidateCommand checks if a command is safe to execute
func (v *CommandValidator) ValidateCommand(cmd string, args []string) error {
	// Check if command is in allowed list
	cmdBase := filepath.Base(cmd)
	if !v.allowedCommands[cmdBase] {
		return fmt.Errorf("command not in allowed list: %s", cmdBase)
	}

	// Validate arguments don't contain shell injection attempts
	for _, arg := range args {
		if err := v.validateArgument(arg); err != nil {
			return fmt.Errorf("invalid argument: %w", err)
		}
	}

	return nil
}

// validateArgument checks a single argument for injection attempts
func (v *CommandValidator) validateArgument(arg string) error {
	// Check for shell metacharacters that could lead to injection
	dangerous := []string{
		"$(",  // Command substitution
		"${",  // Variable expansion
		"`",   // Command substitution
		"&&",  // Command chaining
		"||",  // Command chaining
		";",   // Command separator
		"|",   // Pipe (sometimes ok, but flag for review)
		">",   // Redirect
		"<",   // Redirect
		">>",  // Append redirect
		"&",   // Background execution
		"\n",  // Newline
		"\r",  // Carriage return
	}

	for _, pattern := range dangerous {
		if strings.Contains(arg, pattern) {
			// Special cases where these are allowed
			if pattern == "|" && strings.HasPrefix(arg, "grep ") {
				continue // Allow piping to grep
			}
			return fmt.Errorf("argument contains dangerous pattern: %s", pattern)
		}
	}

	return nil
}

// ValidateGitRef validates a git reference (branch, tag, commit)
func (v *CommandValidator) ValidateGitRef(ref string) error {
	// Git refs have specific rules
	gitRefPattern := regexp.MustCompile(`^[a-zA-Z0-9/_.-]+$`)
	if !gitRefPattern.MatchString(ref) {
		return fmt.Errorf("invalid git ref format: %s", ref)
	}
	return nil
}

// ValidatePath validates a file system path
func (v *CommandValidator) ValidatePath(path string) error {
	// Prevent path traversal
	clean := filepath.Clean(path)
	if strings.Contains(clean, "..") {
		return fmt.Errorf("path traversal detected: %s", path)
	}

	// Ensure path doesn't try to escape workspace
	if filepath.IsAbs(path) && !strings.HasPrefix(path, "/workspace") {
		return fmt.Errorf("absolute path outside workspace: %s", path)
	}

	return nil
}

// ValidateSessionID validates a session ID format
func (v *CommandValidator) ValidateSessionID(id string) error {
	// Session IDs should be UUID-like
	sessionPattern := regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
	if !sessionPattern.MatchString(id) {
		return fmt.Errorf("invalid session ID format: %s", id)
	}
	return nil
}

// SanitizeForShell escapes a string for safe use in shell commands
// This should be used as a last resort - prefer validation
func SanitizeForShell(s string) string {
	// Use single quotes and escape any single quotes in the string
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}