package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// PnpmWorkspace represents the structure of pnpm-workspace.yaml
type PnpmWorkspace struct {
	Packages []string `yaml:"packages"`
}

// ParsePnpmWorkspace parses pnpm-workspace.yaml and returns expanded package paths.
// It expands glob patterns like "packages/*" into actual directory paths.
func ParsePnpmWorkspace(workDir string) ([]string, error) {
	workspaceFile := filepath.Join(workDir, "pnpm-workspace.yaml")
	data, err := os.ReadFile(workspaceFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read pnpm-workspace.yaml: %w", err)
	}

	var ws PnpmWorkspace
	if err := yaml.Unmarshal(data, &ws); err != nil {
		return nil, fmt.Errorf("failed to parse pnpm-workspace.yaml: %w", err)
	}

	var packages []string
	for _, pattern := range ws.Packages {
		// Normalize the pattern (remove leading ./ and trailing /)
		pattern = NormalizePackagePath(pattern)

		// Expand globs
		expanded, err := expandGlob(workDir, pattern)
		if err != nil {
			return nil, fmt.Errorf("failed to expand pattern %q: %w", pattern, err)
		}
		packages = append(packages, expanded...)
	}

	return packages, nil
}

// expandGlob expands a glob pattern relative to workDir and returns matching directories.
func expandGlob(workDir, pattern string) ([]string, error) {
	fullPattern := filepath.Join(workDir, pattern)
	matches, err := filepath.Glob(fullPattern)
	if err != nil {
		return nil, err
	}

	var dirs []string
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil {
			continue
		}
		if info.IsDir() {
			// Return path relative to workDir
			rel, err := filepath.Rel(workDir, match)
			if err != nil {
				continue
			}
			dirs = append(dirs, rel)
		}
	}
	return dirs, nil
}

// ValidatePackage checks if a package path exists in the pnpm workspace.
func ValidatePackage(workDir, packagePath string) error {
	packages, err := ParsePnpmWorkspace(workDir)
	if err != nil {
		return err
	}

	normalized := NormalizePackagePath(packagePath)
	for _, pkg := range packages {
		if NormalizePackagePath(pkg) == normalized {
			return nil
		}
	}

	return fmt.Errorf("package %q not found in pnpm workspace (available: %v)", packagePath, packages)
}

// NormalizePackagePath cleans up a package path by removing leading "./" and trailing "/".
func NormalizePackagePath(path string) string {
	path = strings.TrimPrefix(path, "./")
	path = strings.TrimSuffix(path, "/")
	return path
}

// HasPnpmWorkspace checks if a pnpm-workspace.yaml file exists in the given directory.
func HasPnpmWorkspace(workDir string) bool {
	workspaceFile := filepath.Join(workDir, "pnpm-workspace.yaml")
	_, err := os.Stat(workspaceFile)
	return err == nil
}

// ExtractPackageFromLabel extracts the package path from a pkg:<name> label.
// It returns the package name and true if the label matches, or empty string and false otherwise.
func ExtractPackageFromLabel(label, prefix string) (string, bool) {
	fullPrefix := prefix + ":"
	if strings.HasPrefix(label, fullPrefix) {
		return strings.TrimPrefix(label, fullPrefix), true
	}
	return "", false
}

// FindPackageLabel searches labels for a package label with the given prefix.
// Returns the package name if found, empty string otherwise.
func FindPackageLabel(labels []string, prefix string) string {
	for _, label := range labels {
		if pkg, ok := ExtractPackageFromLabel(label, prefix); ok {
			return pkg
		}
	}
	return ""
}

// ResolvePackagePath converts a package name (from a label) to a full package path.
// It searches the workspace for a matching package directory.
// For example, "core" might resolve to "packages/core" or "apps/core".
func ResolvePackagePath(workDir, packageName string) (string, error) {
	packages, err := ParsePnpmWorkspace(workDir)
	if err != nil {
		return "", err
	}

	// First, try exact match
	for _, pkg := range packages {
		if pkg == packageName || NormalizePackagePath(pkg) == packageName {
			return pkg, nil
		}
	}

	// Then, try matching by base name (e.g., "core" matches "packages/core")
	for _, pkg := range packages {
		if filepath.Base(pkg) == packageName {
			return pkg, nil
		}
	}

	return "", fmt.Errorf("package %q not found in workspace (available: %v)", packageName, packages)
}
