package controller

import (
	"context"
	"fmt"
	"strings"

	"github.com/andywolf/agentium/internal/handoff"
	"github.com/andywolf/agentium/internal/memory"
)

// issuePhaseOrder defines the sequence of phases for issue tasks in the phase loop.
// TEST is merged into IMPLEMENT. REVIEW phase has been removed as IMPLEMENT
// already runs Worker → Reviewer → Judge per iteration.
var issuePhaseOrder = []TaskPhase{
	PhasePlan,
	PhaseImplement,
	PhaseDocs,
	PhasePRCreation,
}

// Default max iterations per phase when not configured.
const (
	defaultPlanMaxIter      = 3
	defaultImplementMaxIter = 5
	defaultDocsMaxIter      = 2
	defaultPRMaxIter        = 1
)

// defaultJudgeContextBudget is the default max characters of phase output sent to the judge.
const defaultJudgeContextBudget = 8000

// phaseMaxIterations returns the configured max iterations for a phase,
// falling back to defaults when not specified.
func (c *Controller) phaseMaxIterations(phase TaskPhase) int {
	cfg := c.config.PhaseLoop
	if cfg == nil {
		return defaultMaxIter(phase)
	}
	switch phase {
	case PhasePlan:
		if cfg.PlanMaxIterations > 0 {
			return cfg.PlanMaxIterations
		}
	case PhaseImplement:
		if cfg.ImplementMaxIterations > 0 {
			return cfg.ImplementMaxIterations
		}
	case PhaseDocs:
		if cfg.DocsMaxIterations > 0 {
			return cfg.DocsMaxIterations
		}
	}
	return defaultMaxIter(phase)
}

func defaultMaxIter(phase TaskPhase) int {
	switch phase {
	case PhasePlan:
		return defaultPlanMaxIter
	case PhaseImplement:
		return defaultImplementMaxIter
	case PhaseDocs:
		return defaultDocsMaxIter
	case PhasePRCreation:
		return defaultPRMaxIter
	default:
		return 1
	}
}

// advancePhase returns the next phase in the issue phase order.
// If the current phase is the last one (or not found), returns PhaseComplete.
func advancePhase(current TaskPhase) TaskPhase {
	for i, p := range issuePhaseOrder {
		if p == current {
			if i+1 < len(issuePhaseOrder) {
				return issuePhaseOrder[i+1]
			}
			return PhaseComplete
		}
	}
	return PhaseComplete
}

// runPhaseLoop executes the controller-as-judge phase loop for the active issue task.
// It iterates through phases, running the agent and judge at each step.
func (c *Controller) runPhaseLoop(ctx context.Context) error {
	taskID := fmt.Sprintf("issue:%s", c.activeTask)
	state := c.taskStates[taskID]
	if state == nil {
		return fmt.Errorf("no task state for %s", taskID)
	}

	c.logInfo("Starting phase loop for issue #%s (initial phase: %s)", c.activeTask, state.Phase)

	// Initialize handoff store with issue context if enabled
	if c.isHandoffEnabled() {
		issueCtx := c.buildIssueContext()
		if issueCtx != nil {
			c.handoffStore.SetIssueContext(taskID, issueCtx)
			c.logInfo("Handoff store initialized with issue context for task %s", taskID)
		}
	}

	for {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Check global termination conditions
		if c.shouldTerminate() {
			c.logInfo("Phase loop: global termination condition met")
			return nil
		}

		currentPhase := state.Phase

		// Terminal phases end the loop
		if currentPhase == PhaseComplete || currentPhase == PhaseBlocked || currentPhase == PhaseNothingToDo {
			c.logInfo("Phase loop: reached terminal phase %s", currentPhase)
			return nil
		}

		maxIter := c.phaseMaxIterations(currentPhase)
		state.MaxPhaseIter = maxIter

		c.logInfo("Phase loop: entering phase %s (max %d iterations)", currentPhase, maxIter)

		// Inner loop: iterate within the current phase
		advanced := false
		noSignalCount := 0
		for iter := 1; iter <= maxIter; iter++ {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			if c.shouldTerminate() {
				return nil
			}

			state.PhaseIteration = iter
			c.logInfo("Phase %s: iteration %d/%d", currentPhase, iter, maxIter)

			// Update the phase in state so skills/routing pick it up
			state.Phase = currentPhase

			// Run an agent iteration for this phase
			c.iteration++
			if c.cloudLogger != nil {
				c.cloudLogger.SetIteration(c.iteration)
			}

			result, err := c.runIteration(ctx)
			if err != nil {
				c.logError("Phase %s iteration %d failed: %v", currentPhase, iter, err)
				continue
			}

			phaseOutput := result.RawTextContent
			if phaseOutput == "" {
				phaseOutput = result.Summary
			}

			// Parse and store handoff output if enabled
			if c.isHandoffEnabled() && phaseOutput != "" {
				if handoffErr := c.processHandoffOutput(taskID, currentPhase, iter, phaseOutput); handoffErr != nil {
					c.logWarning("Failed to process handoff output for phase %s: %v", currentPhase, handoffErr)
				}
			}

			// Post phase comment
			c.postPhaseComment(ctx, currentPhase, iter, truncateForComment(phaseOutput))

			// Skip judge for PR_CREATION (terminal action)
			if currentPhase == PhasePRCreation {
				// Check if PR was actually created
				if result.AgentStatus == "PR_CREATED" || len(result.PRsCreated) > 0 {
					state.Phase = PhaseComplete
					if len(result.PRsCreated) > 0 {
						state.PRNumber = result.PRsCreated[0]
					}
				} else {
					state.Phase = PhaseBlocked
					state.LastJudgeFeedback = "PR creation did not produce a PR"
				}
				return nil
			}

			// Gather previous iteration feedback for reviewer context
			var previousFeedback string
			if iter > 1 && c.memoryStore != nil {
				prevEntries := c.memoryStore.GetPreviousIterationFeedback(taskID, iter)
				if len(prevEntries) > 0 {
					var feedbackParts []string
					for _, e := range prevEntries {
						feedbackParts = append(feedbackParts, e.Content)
					}
					previousFeedback = strings.Join(feedbackParts, "\n")
				}
			}

			// Get worker handoff summary if available
			workerHandoffSummary := c.buildWorkerHandoffSummary(taskID, currentPhase, iter)

			// Run reviewer + judge
			reviewResult, reviewErr := c.runReviewer(ctx, reviewRunParams{
				CompletedPhase:       currentPhase,
				PhaseOutput:          phaseOutput,
				Iteration:            iter,
				MaxIterations:        maxIter,
				PreviousFeedback:     previousFeedback,
				WorkerHandoffSummary: workerHandoffSummary,
			})
			if reviewErr != nil {
				c.logWarning("Reviewer error for phase %s: %v (defaulting to ADVANCE)", currentPhase, reviewErr)
				state.LastJudgeVerdict = string(VerdictAdvance)
				// Clear stale feedback to prevent leaking into later phases
				if c.memoryStore != nil {
					c.memoryStore.ClearByType(memory.EvalFeedback)
				}
				// Record phase result
				if c.memoryStore != nil {
					c.memoryStore.Update([]memory.Signal{
						{Type: memory.PhaseResult, Content: fmt.Sprintf("%s completed (reviewer error, forced advance)", currentPhase)},
					}, c.iteration, taskID)
				}
				advanced = true
				break
			}

			judgeResult, err := c.runJudge(ctx, judgeRunParams{
				CompletedPhase: currentPhase,
				PhaseOutput:    phaseOutput,
				ReviewFeedback: reviewResult.Feedback,
				Iteration:      iter,
				MaxIterations:  maxIter,
				PhaseIteration: iter,
			})
			if err != nil {
				c.logWarning("Judge error for phase %s: %v (defaulting to ADVANCE)", currentPhase, err)
				judgeResult = JudgeResult{Verdict: VerdictAdvance}
			}

			// Track consecutive no-signal count for fail-closed behavior
			if !judgeResult.SignalFound {
				noSignalCount++
				c.logWarning("Judge did not emit signal for phase %s (no-signal count: %d/%d)", currentPhase, noSignalCount, c.judgeNoSignalLimit())
				if noSignalCount >= c.judgeNoSignalLimit() {
					c.logWarning("Phase %s: no-signal limit reached, force-advancing", currentPhase)
					judgeResult = JudgeResult{Verdict: VerdictAdvance, SignalFound: false}
				}
			} else {
				noSignalCount = 0
			}

			state.LastJudgeVerdict = string(judgeResult.Verdict)
			state.LastJudgeFeedback = judgeResult.Feedback

			// Post judge comment
			c.postJudgeComment(ctx, currentPhase, judgeResult)

			switch judgeResult.Verdict {
			case VerdictAdvance:
				// Clear feedback from memory and move to next phase
				if c.memoryStore != nil {
					c.memoryStore.ClearByType(memory.EvalFeedback)
				}
				// Store phase result in memory
				if c.memoryStore != nil {
					c.memoryStore.Update([]memory.Signal{
						{Type: memory.PhaseResult, Content: fmt.Sprintf("%s completed (iteration %d)", currentPhase, iter)},
					}, c.iteration, taskID)
				}
				advanced = true

			case VerdictIterate:
				// Feedback is already stored in memory by runJudge
				c.logInfo("Phase %s: judge requested iteration (feedback: %s)", currentPhase, judgeResult.Feedback)
				continue

			case VerdictBlocked:
				state.Phase = PhaseBlocked
				c.logInfo("Phase %s: judge returned BLOCKED: %s", currentPhase, judgeResult.Feedback)
				return nil
			}

			if advanced {
				break
			}
		}

		if !advanced {
			// Exhausted max iterations without ADVANCE — force advance
			c.logWarning("Phase %s: exhausted %d iterations without ADVANCE, forcing advance", currentPhase, maxIter)
			c.postPhaseComment(ctx, currentPhase, maxIter,
				fmt.Sprintf("Forced advance: exhausted %d iterations without judge ADVANCE", maxIter))
			if c.memoryStore != nil {
				c.memoryStore.ClearByType(memory.EvalFeedback)
			}
		}

		// Move to next phase
		nextPhase := advancePhase(currentPhase)
		c.logInfo("Phase loop: advancing from %s to %s", currentPhase, nextPhase)
		state.Phase = nextPhase
	}
}

// defaultJudgeNoSignalLimit is the default max consecutive no-signal judgments
// before force-advancing.
const defaultJudgeNoSignalLimit = 2

// judgeNoSignalLimit returns the configured max consecutive no-signal judgments,
// falling back to the default when not specified.
func (c *Controller) judgeNoSignalLimit() int {
	if c.config.PhaseLoop != nil && c.config.PhaseLoop.JudgeNoSignalLimit > 0 {
		return c.config.PhaseLoop.JudgeNoSignalLimit
	}
	return defaultJudgeNoSignalLimit
}

// truncateForComment truncates text for use in GitHub comments.
// It operates on runes to avoid splitting multi-byte UTF-8 characters.
func truncateForComment(s string) string {
	const maxRunes = 500
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "..."
}

// processHandoffOutput parses and stores structured handoff output from phase iteration.
func (c *Controller) processHandoffOutput(taskID string, phase TaskPhase, iteration int, output string) error {
	if c.handoffParser == nil || c.handoffStore == nil {
		return nil
	}

	// Check if output contains a handoff signal
	if !c.handoffParser.HasHandoffSignal(output) {
		c.logInfo("Phase %s iteration %d: no handoff signal found", phase, iteration)
		return nil
	}

	// Convert TaskPhase to handoff.Phase
	handoffPhase := handoff.Phase(phase)

	// Parse the handoff output
	parsedOutput, err := c.handoffParser.ParseOutput(output, handoffPhase)
	if err != nil {
		return fmt.Errorf("failed to parse handoff output: %w", err)
	}

	// Validate the parsed output
	if c.handoffValidator != nil {
		errs := c.handoffValidator.ValidatePhaseOutput(handoffPhase, parsedOutput)
		if errs.HasErrors() {
			c.logWarning("Phase %s handoff validation warnings: %v", phase, errs)
			// Continue despite validation warnings - don't fail the phase
		}
	}

	// Store the handoff output
	if err := c.handoffStore.StorePhaseOutput(taskID, handoffPhase, iteration, parsedOutput); err != nil {
		return fmt.Errorf("failed to store handoff output: %w", err)
	}

	c.logInfo("Phase %s iteration %d: handoff output stored", phase, iteration)

	// Save to disk
	if err := c.handoffStore.Save(); err != nil {
		c.logWarning("Failed to persist handoff store: %v", err)
	}

	return nil
}

// buildWorkerHandoffSummary extracts a summary of what the worker claims to have done
// from the handoff store. This helps the reviewer verify claims against actual output.
// Only returns data if the handoff was produced in the current iteration to avoid
// showing stale claims from previous iterations.
func (c *Controller) buildWorkerHandoffSummary(taskID string, phase TaskPhase, currentIteration int) string {
	if !c.isHandoffEnabled() || c.handoffStore == nil {
		return ""
	}

	handoffPhase := handoff.Phase(phase)
	hd := c.handoffStore.GetPhaseOutput(taskID, handoffPhase)
	if hd == nil {
		return ""
	}

	// Skip stale handoff data from previous iterations to avoid validating wrong work
	if hd.Iteration != currentIteration {
		return ""
	}

	var parts []string

	// Extract relevant info based on phase type
	switch phase {
	case PhasePlan:
		if hd.PlanOutput != nil {
			if hd.PlanOutput.Summary != "" {
				parts = append(parts, fmt.Sprintf("Summary: %s", hd.PlanOutput.Summary))
			}
			if len(hd.PlanOutput.FilesToModify) > 0 {
				parts = append(parts, fmt.Sprintf("Files to modify: %v", hd.PlanOutput.FilesToModify))
			}
			if len(hd.PlanOutput.FilesToCreate) > 0 {
				parts = append(parts, fmt.Sprintf("Files to create: %v", hd.PlanOutput.FilesToCreate))
			}
		}
	case PhaseImplement:
		if hd.ImplementOutput != nil {
			if hd.ImplementOutput.BranchName != "" {
				parts = append(parts, fmt.Sprintf("Branch: %s", hd.ImplementOutput.BranchName))
			}
			if len(hd.ImplementOutput.FilesChanged) > 0 {
				parts = append(parts, fmt.Sprintf("Files changed: %v", hd.ImplementOutput.FilesChanged))
			}
			if hd.ImplementOutput.TestsPassed {
				parts = append(parts, "Tests: Passed")
			} else if hd.ImplementOutput.TestOutput != "" {
				parts = append(parts, fmt.Sprintf("Tests: %s", hd.ImplementOutput.TestOutput))
			}
		}
	case PhaseDocs:
		if hd.DocsOutput != nil {
			if len(hd.DocsOutput.DocsUpdated) > 0 {
				parts = append(parts, fmt.Sprintf("Docs updated: %v", hd.DocsOutput.DocsUpdated))
			}
			if hd.DocsOutput.ReadmeChanged {
				parts = append(parts, "README: Updated")
			}
		}
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, "\n")
}
