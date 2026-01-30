package scope

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// ScopeValidator validates that file changes are within the allowed package scope.
type ScopeValidator struct {
	PackagePath string // Relative path from repo root (e.g., "packages/core")
	WorkDir     string // Repository root directory
}

// NewValidator creates a new ScopeValidator for the given package path and work directory.
func NewValidator(workDir, packagePath string) *ScopeValidator {
	return &ScopeValidator{
		PackagePath: packagePath,
		WorkDir:     workDir,
	}
}

// ValidationResult contains the result of scope validation.
type ValidationResult struct {
	Valid            bool
	OutOfScopeFiles  []string
	AllowedExempt    []string // Files that were out of scope but allowed by exemptions
	TotalFilesChanged int
}

// allowedExemptions lists files outside the package that are allowed to be modified.
// These are typically workspace-level files that need updates when changing package dependencies.
var allowedExemptions = []string{
	"package.json",           // Root package.json for workspace dependencies
	"pnpm-lock.yaml",         // Lock file updates
	"pnpm-workspace.yaml",    // Workspace config (rare, but valid in some cases)
	".github/workflows",      // CI workflow files
}

// ValidateChanges checks if all modified files are within the package scope.
// It returns a ValidationResult with details about any out-of-scope files.
// This includes both modified tracked files and untracked (new) files.
func (v *ScopeValidator) ValidateChanges() (*ValidationResult, error) {
	if v.PackagePath == "" {
		// No package scope set, all files are allowed
		return &ValidationResult{Valid: true}, nil
	}

	// Use git status --porcelain to get all changes including untracked files.
	// This is more comprehensive than git diff --name-only HEAD which misses
	// untracked files that the agent may have created outside the package scope.
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = v.WorkDir
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get modified files: %w", err)
	}

	return v.validateStatusOutput(string(output))
}

// validateStatusOutput validates files from git status --porcelain output.
func (v *ScopeValidator) validateStatusOutput(output string) (*ValidationResult, error) {
	var files []string
	for _, line := range strings.Split(output, "\n") {
		if len(line) < 3 {
			continue
		}
		// git status --porcelain format: "XY filename" or "XY original -> renamed"
		file := strings.TrimSpace(line[3:])
		// Handle renames (format: "old -> new")
		if idx := strings.Index(file, " -> "); idx != -1 {
			file = file[idx+4:]
		}
		if file != "" {
			files = append(files, file)
		}
	}
	return v.validateFiles(files)
}

// validateFiles checks if all files are within scope.
func (v *ScopeValidator) validateFiles(files []string) (*ValidationResult, error) {
	result := &ValidationResult{
		Valid:            true,
		TotalFilesChanged: len(files),
	}

	normalizedPackage := filepath.Clean(v.PackagePath)

	for _, file := range files {
		if file == "" {
			continue
		}

		// Check if file is within package scope
		if v.isInScope(file, normalizedPackage) {
			continue
		}

		// Check if file is in allowed exemptions
		if v.isExempt(file) {
			result.AllowedExempt = append(result.AllowedExempt, file)
			continue
		}

		// File is out of scope
		result.OutOfScopeFiles = append(result.OutOfScopeFiles, file)
		result.Valid = false
	}

	return result, nil
}

// isInScope checks if a file path is within the package scope.
func (v *ScopeValidator) isInScope(filePath, packagePath string) bool {
	filePath = filepath.Clean(filePath)

	// Check if file starts with package path
	if strings.HasPrefix(filePath, packagePath+string(filepath.Separator)) {
		return true
	}

	// Also allow exact match (unlikely but handle it)
	return filePath == packagePath
}

// isExempt checks if a file is in the allowed exemptions list.
func (v *ScopeValidator) isExempt(filePath string) bool {
	filePath = filepath.Clean(filePath)

	for _, exemption := range allowedExemptions {
		exemption = filepath.Clean(exemption)
		if filePath == exemption {
			return true
		}
		// Check if file is under an exempt directory
		// Treat paths without extensions as directories
		if !strings.Contains(filepath.Base(exemption), ".") {
			// Treat as directory prefix
			if strings.HasPrefix(filePath, exemption+string(filepath.Separator)) {
				return true
			}
		}
	}
	return false
}

// FormatViolationError creates a human-readable error message for scope violations.
func (v *ScopeValidator) FormatViolationError(result *ValidationResult) string {
	if result.Valid {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("SCOPE VIOLATION: %d file(s) modified outside package scope\n", len(result.OutOfScopeFiles)))
	sb.WriteString(fmt.Sprintf("Package scope: %s\n\n", v.PackagePath))
	sb.WriteString("Out-of-scope files:\n")
	for _, f := range result.OutOfScopeFiles {
		sb.WriteString(fmt.Sprintf("  - %s\n", f))
	}
	sb.WriteString("\nOnly files within the package directory may be modified.\n")
	sb.WriteString("Allowed exceptions: root package.json, pnpm-lock.yaml, .github/workflows/\n")

	return sb.String()
}

// ResetChanges reverts all uncommitted changes in the working directory.
// This is used to undo out-of-scope modifications.
func (v *ScopeValidator) ResetChanges() error {
	cmd := exec.Command("git", "checkout", ".")
	cmd.Dir = v.WorkDir
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to reset changes: %w", err)
	}

	// Also clean untracked files that were created
	cmd = exec.Command("git", "clean", "-fd")
	cmd.Dir = v.WorkDir
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to clean untracked files: %w", err)
	}

	return nil
}
