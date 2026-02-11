package controller

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

// AddShutdownHook registers a function to be called during graceful shutdown.
// Hooks are executed in the order they were added.
func (c *Controller) AddShutdownHook(hook ShutdownHook) {
	c.shutdownHooks = append(c.shutdownHooks, hook)
}

// SetLogFlushFunc sets the function used to flush pending log writes.
// This is called with a timeout during shutdown to ensure logs are persisted.
func (c *Controller) SetLogFlushFunc(fn func() error) {
	c.logFlushFn = fn
}

// setupSignalHandler sets up OS signal handling for graceful shutdown.
// It returns a new context that will be cancelled when a shutdown signal is received.
func (c *Controller) setupSignalHandler(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		select {
		case sig := <-sigCh:
			c.logInfo("Received signal %v, initiating graceful shutdown", sig)
			cancel()
		case <-ctx.Done():
			// Context was cancelled by other means
		}
		signal.Stop(sigCh)
	}()

	return ctx, cancel
}

func (c *Controller) cleanup() {
	c.logInfo("Initiating graceful shutdown")

	// Execute shutdown with timeout
	c.gracefulShutdown()
}

// gracefulShutdown performs a controlled shutdown sequence:
// 1. Flush pending log writes (with timeout)
// 2. Run registered shutdown hooks
// 3. Clear sensitive data from memory
// 4. Close clients
// 5. Terminate VM
func (c *Controller) gracefulShutdown() {
	c.shutdownOnce.Do(func() {
		close(c.shutdownCh)

		// Create a timeout context for the entire shutdown sequence
		ctx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
		defer cancel()

		// Step 1: Flush pending log writes with timeout
		c.flushLogs(ctx)

		// Step 2: Run registered shutdown hooks
		c.runShutdownHooks(ctx)

		// Step 3: Clear sensitive data from memory
		c.clearSensitiveData()

		// Step 4: Close clients
		if c.eventSink != nil {
			if err := c.eventSink.Close(); err != nil {
				c.logWarning("failed to close event sink: %v", err)
			}
		}
		if c.metadataUpdater != nil {
			if err := c.metadataUpdater.Close(); err != nil {
				c.logWarning("failed to close metadata updater: %v", err)
			}
		}
		if c.cloudLogger != nil {
			if err := c.cloudLogger.Close(); err != nil {
				c.logWarning("failed to close cloud logger: %v", err)
			}
		}
		if c.secretManager != nil {
			if err := c.secretManager.Close(); err != nil {
				c.logWarning("failed to close Secret Manager client: %v", err)
			}
		}

		c.logInfo("Graceful shutdown complete")

		// Step 5: Request VM termination (last action)
		c.terminateVM()
	})
}

// flushLogs ensures all pending log writes are sent before shutdown.
// It uses a timeout to prevent blocking indefinitely on log flush.
func (c *Controller) flushLogs(ctx context.Context) {
	if c.logFlushFn == nil {
		return
	}

	c.logInfo("Flushing pending log writes...")

	// Create a sub-context with log flush timeout
	flushCtx, cancel := context.WithTimeout(ctx, LogFlushTimeout)
	defer cancel()

	// Run flush in a goroutine so we can respect the timeout
	done := make(chan error, 1)
	go func() {
		done <- c.logFlushFn()
	}()

	select {
	case err := <-done:
		if err != nil {
			c.logWarning("log flush completed with error: %v", err)
		} else {
			c.logInfo("Log flush completed successfully")
		}
	case <-flushCtx.Done():
		c.logWarning("log flush timed out, some logs may be lost")
	}
}

// runShutdownHooks executes all registered shutdown hooks in order.
// Each hook receives the shutdown context and should respect cancellation.
func (c *Controller) runShutdownHooks(ctx context.Context) {
	if len(c.shutdownHooks) == 0 {
		return
	}

	c.logInfo("Running %d shutdown hooks", len(c.shutdownHooks))

	for i, hook := range c.shutdownHooks {
		select {
		case <-ctx.Done():
			c.logWarning("shutdown timeout reached, skipping remaining %d hooks", len(c.shutdownHooks)-i)
			return
		default:
		}

		if err := hook(ctx); err != nil {
			c.logWarning("shutdown hook %d failed: %v", i+1, err)
		}
	}
}

// clearSensitiveData removes sensitive information from memory
func (c *Controller) clearSensitiveData() {
	c.logInfo("Clearing sensitive data from memory")

	// Clear GitHub token
	c.gitHubToken = ""

	// Clear prompt content (may contain sensitive context)
	c.config.Prompt = ""

	// Clear Claude auth data
	c.config.ClaudeAuth.AuthJSONBase64 = ""

	// Clear Codex auth data
	c.config.CodexAuth.AuthJSONBase64 = ""

	// Clear GitHub app credentials
	c.config.GitHub.PrivateKeySecret = ""
}

func (c *Controller) terminateVM() {
	// Skip VM termination in interactive mode (no VM to terminate)
	if c.config.Interactive {
		c.logInfo("Skipping VM termination (interactive mode)")
		return
	}

	c.logInfo("Initiating VM termination")

	ctx, cancel := context.WithTimeout(context.Background(), VMTerminationTimeout)
	defer cancel()

	// Get instance name from metadata
	cmd := c.execCommand(ctx, "curl", "-s", "-H", "Metadata-Flavor: Google",
		"http://metadata.google.internal/computeMetadata/v1/instance/name")
	instanceName, err := cmd.Output()
	if err != nil {
		c.logError("failed to get instance name from metadata: %v — VM will not be deleted", err)
		return
	}

	// Get zone from metadata
	cmd = c.execCommand(ctx, "curl", "-s", "-H", "Metadata-Flavor: Google",
		"http://metadata.google.internal/computeMetadata/v1/instance/zone")
	zone, err := cmd.Output()
	if err != nil {
		c.logError("failed to get zone from metadata: %v — VM will not be deleted", err)
		return
	}

	// Delete instance (blocks until completion or timeout)
	name := strings.TrimSpace(string(instanceName))
	zoneName := filepath.Base(strings.TrimSpace(string(zone)))
	c.logInfo("Deleting VM instance %s in zone %s", name, zoneName)

	cmd = c.execCommand(ctx, "gcloud", "compute", "instances", "delete",
		name,
		"--zone", zoneName,
		"--quiet",
	)

	if err := cmd.Run(); err != nil {
		c.logError("VM deletion command failed: %v — VM may remain running until max_run_duration", err)
		return
	}

	c.logInfo("VM deletion command completed successfully")
}
