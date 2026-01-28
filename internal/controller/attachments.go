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

// createGistAttachment uploads content as a public gist using `gh gist create`.
// Returns the gist URL on success, or an empty string on failure (graceful fallback).
// This is best-effort: failures are logged but never cause the controller to crash.
func (c *Controller) createGistAttachment(ctx context.Context, filename, content string) string {
	// Write content to temp file
	tmpFile, err := os.CreateTemp("", "agentium-*.md")
	if err != nil {
		c.logWarning("failed to create temp file for gist: %v", err)
		return ""
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		c.logWarning("failed to write content to temp file for gist: %v", err)
		tmpFile.Close()
		return ""
	}
	tmpFile.Close()

	// Create gist via gh CLI
	cmd := exec.CommandContext(ctx, "gh", "gist", "create",
		"--public",
		"--filename", filename,
		tmpFile.Name(),
	)
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
