package controller

import (
	"strings"
	"time"

	"github.com/andywolf/agentium/internal/agent"
)

// isAdapterExecutionFailure checks if an error indicates an adapter-level failure
// that warrants fallback (vs task failure that should not retry).
// It examines both the error message and stderr output for known failure patterns.
func isAdapterExecutionFailure(err error, stderr string, duration time.Duration) bool {
	if err == nil {
		return false
	}

	combined := strings.ToLower(err.Error() + " " + stderr)

	// Known adapter failure patterns
	patterns := []string{
		"eisdir", "is a directory",
		"enoent", "no such file",
		"permission denied",
		"docker: error",
		"no such image",
		"connection refused",
		"auth file",
		"oci runtime",
	}

	for _, p := range patterns {
		if strings.Contains(combined, p) {
			return true
		}
	}

	// Very short execution with error = startup failure
	// If the container ran for less than 30 seconds and failed, it's likely a startup issue
	return duration < 30*time.Second
}

// getFallbackAdapter returns the fallback adapter name if configured, empty string otherwise.
func (c *Controller) getFallbackAdapter() string {
	if c.config.Fallback == nil || !c.config.Fallback.Enabled {
		return ""
	}
	return DefaultFallbackAdapter
}

// canFallback returns true if a fallback can be attempted from the current adapter.
// Returns false if fallback is disabled or the fallback adapter is not available.
// When the current adapter matches the fallback adapter, fallback is still allowed
// if there's a model override that can be removed (retry with default model).
func (c *Controller) canFallback(currentAdapter string, session *agent.Session) bool {
	fallback := c.getFallbackAdapter()
	if fallback == "" {
		return false
	}
	if fallback == currentAdapter {
		// Same adapter - can only fallback if there's a model override to remove
		return session != nil &&
			session.IterationContext != nil &&
			session.IterationContext.ModelOverride != ""
	}
	_, exists := c.adapters[fallback]
	return exists
}
