package controller

import (
	"context"
	"time"

	"github.com/andywolf/agentium/internal/agent"
	"github.com/andywolf/agentium/internal/cloud/gcp"
)

// updateTaskPhase updates the task state based on the agent's iteration result.
func (c *Controller) updateTaskPhase(taskID string, result *agent.IterationResult) {
	state, exists := c.taskStates[taskID]
	if !exists {
		return
	}

	// Update based on agent status signal
	switch result.AgentStatus {
	case "TESTS_RUNNING":
		// Tests are now part of IMPLEMENT phase, keep phase unchanged
	case "TESTS_PASSED":
		state.Phase = PhaseDocs
		state.TestRetries = 0 // Reset retries on success
	case "TESTS_FAILED":
		state.TestRetries++
		if state.TestRetries >= 3 {
			state.Phase = PhaseBlocked
			c.logWarning("Task %s blocked after %d test failures", taskID, state.TestRetries)
			// Propagate blocked state to dependent issues
			if state.Type == "issue" {
				c.propagateBlocked(state.ID)
			}
		}
	case "PR_CREATED":
		state.Phase = PhaseComplete
		state.PRNumber = result.StatusMessage
	case "PUSHED":
		state.Phase = PhaseComplete
	case "COMPLETE":
		state.Phase = PhaseComplete
	case "NOTHING_TO_DO":
		state.Phase = PhaseNothingToDo
	case "BLOCKED", "FAILED":
		state.Phase = PhaseBlocked
		// Propagate blocked state to dependent issues
		if state.Type == "issue" {
			c.propagateBlocked(state.ID)
		}
	}

	// Fallback: if no explicit status signal but PRs were detected for an issue task.
	// Only complete if already in DOCS phase to avoid skipping documentation updates.
	// If still in IMPLEMENT, advance to DOCS instead of completing.
	if result.AgentStatus == "" && state.Type == "issue" && len(result.PRsCreated) > 0 {
		state.PRNumber = result.PRsCreated[0]
		switch state.Phase {
		case PhaseDocs:
			state.Phase = PhaseComplete
			c.logInfo("Task %s completed via PR detection fallback (PR #%s)", taskID, state.PRNumber)
		case PhaseImplement:
			state.Phase = PhaseDocs
			c.logInfo("Task %s advancing to DOCS via PR detection (PR #%s)", taskID, state.PRNumber)
		}
	}

	state.LastStatus = result.AgentStatus
	c.logInfo("Task %s phase: %s (status: %s)", taskID, state.Phase, result.AgentStatus)
}

// updateInstanceMetadata writes the current session status to GCP instance metadata.
// This is best-effort: errors are logged but never cause the controller to crash.
func (c *Controller) updateInstanceMetadata(ctx context.Context) {
	if c.metadataUpdater == nil {
		return
	}

	var completed, pending []string
	for taskID, state := range c.taskStates {
		switch state.Phase {
		case PhaseComplete, PhaseNothingToDo:
			completed = append(completed, taskID)
		default:
			pending = append(pending, taskID)
		}
	}

	status := gcp.SessionStatusMetadata{
		Iteration:      c.iteration,
		CompletedTasks: completed,
		PendingTasks:   pending,
	}

	if err := c.metadataUpdater.UpdateStatus(ctx, status); err != nil {
		c.logWarning("failed to update instance metadata: %v", err)
	}
}

// shouldTerminate checks whether the session should end based on time limit
// or all tasks reaching a terminal phase.
func (c *Controller) shouldTerminate() bool {
	// Check time limit
	if time.Since(c.startTime) >= c.maxDuration {
		c.logInfo("Max duration reached")
		return true
	}

	// Check if all tasks have reached a terminal phase
	if len(c.taskStates) > 0 {
		allTerminal := true
		for taskID, state := range c.taskStates {
			switch state.Phase {
			case PhaseComplete, PhaseNothingToDo, PhaseBlocked:
				c.logInfo("Task %s in terminal phase: %s", taskID, state.Phase)
				continue
			default:
				allTerminal = false
			}
		}
		if allTerminal {
			c.logInfo("All tasks in terminal phase")
			return true
		}
	}

	return false
}

// emitFinalLogs outputs the session summary at the end of a run.
func (c *Controller) emitFinalLogs() {
	// Final metadata update so the provisioner sees the terminal state
	c.updateInstanceMetadata(context.Background())

	c.logInfo("=== Session Summary ===")
	c.logInfo("Session ID: %s", c.config.ID)
	c.logInfo("Duration: %s", time.Since(c.startTime).Round(time.Second))
	c.logInfo("Iterations: %d", c.iteration)

	// Count completed tasks using taskStates
	completedCount := 0
	for _, state := range c.taskStates {
		if state.Phase == PhaseComplete || state.Phase == PhaseNothingToDo {
			completedCount++
		}
	}
	c.logInfo("Tasks completed: %d/%d", completedCount, len(c.taskStates))

	// Log task state summary
	for taskID, state := range c.taskStates {
		c.logInfo("Task %s: phase=%s, retries=%d", taskID, state.Phase, state.TestRetries)
	}

	c.logInfo("======================")
}
