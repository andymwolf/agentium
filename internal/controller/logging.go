package controller

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"

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

// initTracer initializes the Langfuse observability tracer from environment variables.
// If LANGFUSE_PUBLIC_KEY and LANGFUSE_SECRET_KEY are set (and LANGFUSE_ENABLED is not "false"),
// a LangfuseTracer is created. Otherwise the default NoOpTracer is kept.
func (c *Controller) initTracer(logger *log.Logger) {
	publicKey := os.Getenv("LANGFUSE_PUBLIC_KEY")
	secretKey := os.Getenv("LANGFUSE_SECRET_KEY")

	if publicKey == "" || secretKey == "" {
		return
	}

	if os.Getenv("LANGFUSE_ENABLED") == "false" {
		logger.Printf("Langfuse: disabled via LANGFUSE_ENABLED=false")
		return
	}

	baseURL := os.Getenv("LANGFUSE_BASE_URL")

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
