package controller

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// CommentAttachmentThreshold is the rune count above which comment content
// is uploaded as a gist attachment instead of being posted directly.
const CommentAttachmentThreshold = 1000

// PlanAttachmentThreshold is the rune count above which plan content
// is uploaded as a gist attachment instead of being posted directly.
const PlanAttachmentThreshold = 2000

// createGistAttachment uploads content as a gist using `gh gist create`.
// For public repos, creates a public gist. For private repos, creates a secret gist.
// Returns the gist URL on success, or an empty string on failure (graceful fallback).
// This is best-effort: failures are logged but never cause the controller to crash.
func (c *Controller) createGistAttachment(ctx context.Context, filename, content string) string {
	// Write content to temp file
	tmpFile, err := os.CreateTemp("", "agentium-*.md")
	if err != nil {
		c.logWarning("failed to create temp file for gist: %v", err)
		return ""
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	if _, writeErr := tmpFile.WriteString(content); writeErr != nil {
		c.logWarning("failed to write content to temp file for gist: %v", writeErr)
		_ = tmpFile.Close()
		return ""
	}
	_ = tmpFile.Close()

	// Build gist create command - use --public only for public repos
	args := []string{"gist", "create", "--filename", filename}
	if c.isRepoPublic(ctx) {
		args = append(args, "--public")
	}
	args = append(args, tmpFile.Name())

	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Env = c.envWithGitHubToken()
	cmd.Dir = c.workDir

	output, err := cmd.Output()
	if err != nil {
		c.logWarning("failed to create gist: %v", err)
		return "" // Graceful fallback
	}

	gistURL := strings.TrimSpace(string(output))
	if gistURL != "" {
		c.logInfo("Created gist attachment: %s", gistURL)
	}
	return gistURL
}

// isRepoPublic checks if the configured repository is public.
// The result is cached after the first check since repo visibility doesn't change during a session.
// Returns false if the check fails (fail-safe: default to secret gists).
func (c *Controller) isRepoPublic(ctx context.Context) bool {
	// Return cached result if already checked
	if c.repoVisibilityChecked {
		return c.repoIsPublic
	}

	cmd := exec.CommandContext(ctx, "gh", "repo", "view", c.config.Repository,
		"--json", "visibility",
		"--jq", ".visibility",
	)
	cmd.Env = c.envWithGitHubToken()
	cmd.Dir = c.workDir

	output, err := cmd.Output()
	if err != nil {
		c.logWarning("failed to check repo visibility: %v (defaulting to secret gist)", err)
		// Cache the failure result to avoid repeated failed API calls
		c.repoVisibilityChecked = true
		c.repoIsPublic = false
		return false // Fail-safe: default to secret gist
	}

	visibility := strings.TrimSpace(strings.ToUpper(string(output)))
	c.repoIsPublic = visibility == "PUBLIC"
	c.repoVisibilityChecked = true

	if c.repoIsPublic {
		c.logInfo("Repository %s is public, gists will be public", c.config.Repository)
	} else {
		c.logInfo("Repository %s is private, gists will be secret", c.config.Repository)
	}

	return c.repoIsPublic
}

// contentNeedsAttachment returns true if the content length (in runes)
// exceeds the given threshold and should be uploaded as a gist.
func contentNeedsAttachment(content string, threshold int) bool {
	return len([]rune(content)) > threshold
}

// extractSummary returns the first ~maxRunes of content, breaking at a newline
// boundary when possible to avoid mid-sentence truncation.
func extractSummary(content string, maxRunes int) string {
	runes := []rune(content)
	if len(runes) <= maxRunes {
		return content
	}

	// Take first maxRunes
	excerpt := string(runes[:maxRunes])

	// Try to break at a newline for cleaner summary
	if idx := strings.LastIndex(excerpt, "\n"); idx > maxRunes/2 {
		excerpt = excerpt[:idx]
	}

	return excerpt + "..."
}

// gistFilename generates a standardized filename for gist attachments.
// The format varies by content type:
//   - Phase output: phase_{PHASE}_iter_{N}_issue_{ID}.md
//   - Reviewer feedback: review_{PHASE}_iter_{N}_issue_{ID}.md
//   - Judge feedback: judge_{PHASE}_issue_{ID}.md
//   - Plan: plan_issue_{ID}.md
func gistFilename(contentType string, phase TaskPhase, iteration int, taskID string) string {
	switch contentType {
	case "phase":
		return fmt.Sprintf("phase_%s_iter_%d_issue_%s.md", phase, iteration, taskID)
	case "review":
		return fmt.Sprintf("review_%s_iter_%d_issue_%s.md", phase, iteration, taskID)
	case "judge":
		return fmt.Sprintf("judge_%s_issue_%s.md", phase, taskID)
	case "plan":
		return fmt.Sprintf("plan_issue_%s.md", taskID)
	default:
		return fmt.Sprintf("agentium_%s_issue_%s.md", contentType, taskID)
	}
}
