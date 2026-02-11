package controller

import (
	"fmt"

	"github.com/andywolf/agentium/internal/prompt"
	"github.com/andywolf/agentium/internal/scope"
	"github.com/andywolf/agentium/internal/workspace"
)

// extractPackageLabelFromIssue extracts the package path from issue labels using the configured label prefix.
// Returns the package name and true if found, empty string and false otherwise.
func (c *Controller) extractPackageLabelFromIssue(issue *issueDetail) (string, bool) {
	if c.config.Monorepo == nil || !c.config.Monorepo.Enabled {
		return "", false
	}

	prefix := c.config.Monorepo.LabelPrefix
	if prefix == "" {
		prefix = "pkg" // Default
	}

	for _, label := range issue.Labels {
		if pkg, ok := workspace.ExtractPackageFromLabel(label.Name, prefix); ok {
			return pkg, true
		}
	}
	return "", false
}

// resolveAndValidatePackage resolves and validates the package for the current issue.
// It returns the resolved package path, or an error if:
// - Monorepo is enabled but no package label is found
// - The package label doesn't match a valid workspace package
//
// When tiers are configured, multiple pkg:* labels are allowed. Infrastructure/integration
// packages are implicitly permitted alongside a single domain/app package.
func (c *Controller) resolveAndValidatePackage(issueNumber string) (string, error) {
	if c.config.Monorepo == nil || !c.config.Monorepo.Enabled {
		return "", nil
	}

	// Check if workspace has pnpm-workspace.yaml
	if !workspace.HasPnpmWorkspace(c.workDir) {
		return "", fmt.Errorf("monorepo enabled but pnpm-workspace.yaml not found in %s", c.workDir)
	}

	// Get issue details
	issue := c.issueDetailsByNumber[issueNumber]
	if issue == nil {
		return "", fmt.Errorf("issue #%s not found in fetched details", issueNumber)
	}

	// Parent issues with sub-issues are expanded before reaching this point,
	// so no special check is needed here.

	prefix := c.config.Monorepo.LabelPrefix
	if prefix == "" {
		prefix = "pkg"
	}

	// When tiers are configured, use tier-aware validation
	if len(c.config.Monorepo.Tiers) > 0 {
		// Collect label name strings
		var labelNames []string
		for _, l := range issue.Labels {
			labelNames = append(labelNames, l.Name)
		}

		classifications, err := workspace.ClassifyPackageLabels(labelNames, prefix, c.config.Monorepo.Tiers, c.workDir)
		if err != nil {
			return "", fmt.Errorf("issue #%s: %w", issueNumber, err)
		}

		if len(classifications) == 0 {
			return "", fmt.Errorf("monorepo requires %s:<package> label on issue #%s", prefix, issueNumber)
		}

		pkgPath, err := workspace.ValidatePackageLabels(classifications)
		if err != nil {
			return "", fmt.Errorf("issue #%s: %w", issueNumber, err)
		}
		return pkgPath, nil
	}

	// No tiers configured — preserve existing single-label behavior
	pkgName, found := c.extractPackageLabelFromIssue(issue)
	if !found {
		return "", fmt.Errorf("monorepo requires %s:<package> label on issue #%s", prefix, issueNumber)
	}

	// Resolve package name to full path (e.g., "core" -> "packages/core")
	pkgPath, err := workspace.ResolvePackagePath(c.workDir, pkgName)
	if err != nil {
		return "", fmt.Errorf("invalid package label on issue #%s: %w", issueNumber, err)
	}

	return pkgPath, nil
}

// initPackageScope sets up the package scope for a monorepo issue.
// It detects the package from issue labels, validates it, and initializes the scope validator.
// It also reloads the project prompt to include package-specific AGENTS.md.
func (c *Controller) initPackageScope(issueNumber string) error {
	pkgPath, err := c.resolveAndValidatePackage(issueNumber)
	if err != nil {
		return err
	}

	if pkgPath == "" {
		// Not a monorepo or package not required
		c.packagePath = ""
		c.scopeValidator = nil
		return nil
	}

	c.packagePath = pkgPath
	c.scopeValidator = scope.NewValidator(c.workDir, pkgPath)
	c.logInfo("Monorepo package scope: %s", pkgPath)

	// Reload project prompt with package-specific AGENTS.md merged in
	// Always update projectPrompt (even if empty) to avoid stale prompts from previous packages
	projectPrompt, err := prompt.LoadProjectPromptWithPackage(c.workDir, pkgPath)
	if err != nil {
		c.logWarning("failed to load hierarchical project prompt: %v", err)
	} else {
		c.projectPrompt = projectPrompt
		if projectPrompt != "" {
			c.logInfo("Project prompt loaded with package context (%s)", pkgPath)
		} else {
			c.logInfo("No project prompt found for package %s", pkgPath)
		}
	}

	return nil
}

// buildPackageScopeInstructions returns package scope constraint instructions for the agent prompt.
// Returns empty string if no package scope is active.
func (c *Controller) buildPackageScopeInstructions() string {
	if c.packagePath == "" {
		return ""
	}

	return fmt.Sprintf(`## PACKAGE SCOPE CONSTRAINT

You are working within monorepo package: %s

STRICT CONSTRAINTS:
- Only modify files within: %s/
- Exception: You may update root package.json for workspace dependencies
- Run commands from package directory: cd %s && pnpm test
- Do NOT modify files in other packages or repository root (except root package.json)

Violations will cause your changes to be rejected and reverted.
`, c.packagePath, c.packagePath, c.packagePath)
}
