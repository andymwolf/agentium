package controller

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/andywolf/agentium/internal/agent"
	"github.com/andywolf/agentium/internal/cloud/gcp"
	"github.com/andywolf/agentium/internal/observability"
)

// logInfo logs at INFO level to both local logger and cloud logger
func (c *Controller) logInfo(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	c.logger.Printf("%s", msg)
	if c.cloudLogger != nil {
		c.cloudLogger.Info(msg)
	}
}

// logWarning logs at WARNING level to both local logger and cloud logger
func (c *Controller) logWarning(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	c.logger.Printf("Warning: %s", msg)
	if c.cloudLogger != nil {
		c.cloudLogger.Warning(msg)
	}
}

// logError logs at ERROR level to both local logger and cloud logger
func (c *Controller) logError(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	c.logger.Printf("Error: %s", msg)
	if c.cloudLogger != nil {
		c.cloudLogger.Error(msg)
	}
}

// logTokenConsumption logs token usage for a completed iteration to Cloud Logging.
func (c *Controller) logTokenConsumption(result *agent.IterationResult, agentName string, session *agent.Session) {
	if c.cloudLogger == nil {
		return
	}
	if result.InputTokens == 0 && result.OutputTokens == 0 {
		return
	}

	taskID := taskKey(c.activeTaskType, c.activeTask)
	phase := ""
	if state, ok := c.taskStates[taskID]; ok && state != nil {
		phase = string(state.Phase)
	}

	labels := map[string]string{
		"log_type":      "token_usage",
		"task_id":       taskID,
		"phase":         phase,
		"agent":         agentName,
		"input_tokens":  strconv.Itoa(result.InputTokens),
		"output_tokens": strconv.Itoa(result.OutputTokens),
		"total_tokens":  strconv.Itoa(result.InputTokens + result.OutputTokens),
	}

	if session.IterationContext != nil && session.IterationContext.ModelOverride != "" {
		labels["model"] = session.IterationContext.ModelOverride
	}

	msg := fmt.Sprintf("Token usage: input=%d output=%d total=%d",
		result.InputTokens, result.OutputTokens, result.InputTokens+result.OutputTokens)

	c.cloudLogger.LogWithLabels(gcp.SeverityInfo, msg, labels)
}

// initTracer initializes the Langfuse observability tracer.
// It checks environment variables first (backward compat for local dev),
// then falls back to fetching keys from GCP Secret Manager using config paths.
// If neither source provides keys, the default NoOpTracer is kept.
func (c *Controller) initTracer(ctx context.Context, logger *log.Logger) {
	if os.Getenv("LANGFUSE_ENABLED") == "false" {
		logger.Printf("Langfuse: disabled via LANGFUSE_ENABLED=false")
		return
	}

	// 1. Try environment variables first (backward compat / local dev)
	publicKey := os.Getenv("LANGFUSE_PUBLIC_KEY")
	secretKey := os.Getenv("LANGFUSE_SECRET_KEY")

	// 2. If env vars are empty, try fetching from Secret Manager
	if publicKey == "" || secretKey == "" {
		pubPath := c.config.Langfuse.PublicKeySecret
		secPath := c.config.Langfuse.SecretKeySecret
		if pubPath == "" || secPath == "" {
			if pubPath != secPath {
				logger.Printf("Langfuse: incomplete config — both public_key_secret and secret_key_secret are required")
			}
			return // No env vars and no secret paths configured
		}

		var err error
		publicKey, err = c.fetchSecret(ctx, pubPath)
		if err != nil {
			logger.Printf("Langfuse: failed to fetch public key from %s: %v", pubPath, err)
			return
		}
		secretKey, err = c.fetchSecret(ctx, secPath)
		if err != nil {
			logger.Printf("Langfuse: failed to fetch secret key from %s: %v", secPath, err)
			return
		}
		publicKey = strings.TrimSpace(publicKey)
		secretKey = strings.TrimSpace(secretKey)
	}

	if publicKey == "" || secretKey == "" {
		return
	}

	// Determine base URL: env var > config > default
	baseURL := os.Getenv("LANGFUSE_BASE_URL")
	if baseURL == "" {
		baseURL = c.config.Langfuse.BaseURL
	}

	lt := observability.NewLangfuseTracer(observability.LangfuseConfig{
		PublicKey: publicKey,
		SecretKey: secretKey,
		BaseURL:   baseURL,
	}, logger)

	c.tracer = lt
	c.AddShutdownHook(func(ctx context.Context) error {
		return c.tracer.Stop(ctx)
	})
	logger.Printf("Langfuse: tracer initialized (base_url=%s)", lt.BaseURL())
}
