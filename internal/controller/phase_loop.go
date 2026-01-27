package controller

import (
	"context"
	"fmt"
	"strings"

	"github.com/andywolf/agentium/internal/handoff"
	"github.com/andywolf/agentium/internal/memory"
)

// issuePhaseOrder defines the sequence of phases for issue tasks in the phase loop.
// TEST is merged into IMPLEMENT. REVIEW and PR_CREATION phases have been removed.
// Draft PRs are created during IMPLEMENT phase and finalized at PhaseComplete.
var issuePhaseOrder = []TaskPhase{
	PhasePlan,
	PhaseImplement,
	PhaseDocs,
}

// Default max iterations per phase when not configured.
const (
	defaultPlanMaxIter      = 3
	defaultImplementMaxIter = 5
	defaultDocsMaxIter      = 2
)

// SIMPLE path max iterations - fewer iterations for straightforward changes.
const (
	simplePlanMaxIter      = 1
	simpleImplementMaxIter = 2
	simpleDocsMaxIter      = 1
)

// defaultJudgeContextBudget is the default max characters of phase output sent to the judge.
const defaultJudgeContextBudget = 8000

// phaseMaxIterations returns the configured max iterations for a phase,
// considering the workflow path and falling back to defaults when not specified.
func (c *Controller) phaseMaxIterations(phase TaskPhase, workflowPath WorkflowPath) int {
	cfg := c.config.PhaseLoop

	// For SIMPLE path, use reduced iterations
	if workflowPath == WorkflowPathSimple {
		return simpleMaxIter(phase)
	}

	// For COMPLEX or UNSET, use configured or default iterations
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
	default:
		return 1
	}
}

func simpleMaxIter(phase TaskPhase) int {
	switch phase {
	case PhasePlan:
		return simplePlanMaxIter
	case PhaseImplement:
		return simpleImplementMaxIter
	case PhaseDocs:
		return simpleDocsMaxIter
	default:
		return 1
	}
}

// existingPlanIndicators are strings that indicate an issue already contains
// a complete implementation plan. When any of these are found in the issue body,
// the PLAN phase agent iteration can be skipped.
var existingPlanIndicators = []string{
	"Files to Create/Modify",
	"Files to Modify",
	"Implementation Steps",
	"## Implementation Plan",
}

// hasExistingPlan checks if the active issue body contains indicators
// of a pre-existing implementation plan.
func (c *Controller) hasExistingPlan() bool {
	issueBody := c.getActiveIssueBody()
	if issueBody == "" {
		return false
	}
	for _, indicator := range existingPlanIndicators {
		if strings.Contains(issueBody, indicator) {
			return true
		}
	}
	return false
}

// extractExistingPlan returns the issue body as the plan content if
// an existing plan is detected, otherwise returns an empty string.
func (c *Controller) extractExistingPlan() string {
	if !c.hasExistingPlan() {
		return ""
	}
	return c.getActiveIssueBody()
}

// getActiveIssueBody returns the body of the currently active issue.
func (c *Controller) getActiveIssueBody() string {
	for _, issue := range c.issueDetails {
		if fmt.Sprintf("%d", issue.Number) == c.activeTask {
			return issue.Body
		}
	}
	return ""
}

// isPlanSkipEnabled returns true if plan skipping is configured and enabled.
func (c *Controller) isPlanSkipEnabled() bool {
	if c.config.PhaseLoop == nil {
		return false
	}
	return c.config.PhaseLoop.Enabled && c.config.PhaseLoop.SkipPlanIfExists
}

// shouldSkipPlanIteration returns true if the planning agent iteration should
// be skipped because a pre-existing plan was detected in the issue body.
// This ONLY returns true when:
// 1. The current phase is PLAN
// 2. This is iteration 1 (first iteration of the phase)
// 3. The skip_plan_if_exists config option is enabled
// 4. The issue body contains plan indicators
//
// Subsequent iterations (2, 3, etc.) will NEVER be skipped, even if the issue
// contains a plan. This ensures that if the reviewer requests iteration (ITERATE
// verdict), the agent will run normally on iteration 2+.
func (c *Controller) shouldSkipPlanIteration(phase TaskPhase, iter int) bool {
	// Only skip on iteration 1 of PLAN phase
	if phase != PhasePlan || iter != 1 {
		return false
	}
	// Check if skip is enabled and plan exists
	if !c.isPlanSkipEnabled() {
		return false
	}
	return c.hasExistingPlan()
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
			// Finalize draft PR when completing successfully
			if currentPhase == PhaseComplete && state.PRNumber != "" {
				if err := c.finalizeDraftPR(ctx, taskID); err != nil {
					c.logWarning("Failed to finalize draft PR: %v", err)
				}
			}
			c.logInfo("Phase loop: reached terminal phase %s", currentPhase)
			return nil
		}

		maxIter := c.phaseMaxIterations(currentPhase, state.WorkflowPath)
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

			// Check for pre-existing plan (PLAN phase, iteration 1 only)
			var phaseOutput string
			var skipIteration bool
			if c.shouldSkipPlanIteration(currentPhase, iter) {
				planContent := c.extractExistingPlan()
				c.logInfo("Phase %s: detected pre-existing plan in issue body, skipping agent iteration", currentPhase)
				phaseOutput = planContent
				skipIteration = true
				c.postPhaseComment(ctx, currentPhase, iter, "Pre-existing plan detected in issue body (skipped planning agent)")
			}

			if !skipIteration {
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

				phaseOutput = result.RawTextContent
				if phaseOutput == "" {
					phaseOutput = result.Summary
				}
			}

			// Parse and store handoff output if enabled
			if c.isHandoffEnabled() && phaseOutput != "" {
				if handoffErr := c.processHandoffOutput(taskID, currentPhase, iter, phaseOutput); handoffErr != nil {
					c.logWarning("Failed to process handoff output for phase %s: %v", currentPhase, handoffErr)
				}
			}

			// Post phase comment
			c.postPhaseComment(ctx, currentPhase, iter, truncateForComment(phaseOutput))

			// Create draft PR after first IMPLEMENT iteration with commits
			if currentPhase == PhaseImplement && !state.DraftPRCreated {
				if prErr := c.maybeCreateDraftPR(ctx, taskID); prErr != nil {
					c.logWarning("Failed to create draft PR: %v", prErr)
				}
			}

			// Run complexity assessor after PLAN iteration 1 (only if workflow path is unset)
			if currentPhase == PhasePlan && iter == 1 && state.WorkflowPath == WorkflowPathUnset {
				complexityResult, complexityErr := c.runComplexityAssessor(ctx, complexityRunParams{
					PlanOutput:    phaseOutput,
					Iteration:     iter,
					MaxIterations: maxIter,
				})
				if complexityErr != nil {
					c.logWarning("Complexity assessor error: %v (defaulting to COMPLEX)", complexityErr)
					state.WorkflowPath = WorkflowPathComplex
				} else {
					state.WorkflowPath = WorkflowPath(complexityResult.Verdict)
					c.logInfo("Workflow path set to %s: %s", state.WorkflowPath, complexityResult.Feedback)
				}

				// Post complexity verdict comment
				c.postPhaseComment(ctx, currentPhase, iter,
					fmt.Sprintf("Complexity assessment: **%s**\n\n%s", state.WorkflowPath, complexityResult.Feedback))

				// For SIMPLE tasks, auto-advance from PLAN (skip reviewer/judge)
				if state.WorkflowPath == WorkflowPathSimple {
					c.logInfo("SIMPLE workflow: auto-advancing from PLAN phase")
					// Clear any feedback and store phase result
					if c.memoryStore != nil {
						c.memoryStore.ClearByType(memory.EvalFeedback)
						c.memoryStore.Update([]memory.Signal{
							{Type: memory.PhaseResult, Content: fmt.Sprintf("%s completed (SIMPLE path, iteration %d)", currentPhase, iter)},
						}, c.iteration, taskID)
					}
					// Update issue with plan
					if phaseOutput != "" {
						c.updateIssuePlan(ctx, truncateForPlan(phaseOutput))
					}
					advanced = true
					break
				}

				// For COMPLEX tasks, recalculate max iterations now that we know the path
				maxIter = c.phaseMaxIterations(currentPhase, state.WorkflowPath)
				state.MaxPhaseIter = maxIter
				c.logInfo("COMPLEX workflow: continuing with reviewer/judge (max iterations: %d)", maxIter)
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

			// Post reviewer feedback to appropriate location
			c.postReviewFeedbackForPhase(ctx, currentPhase, iter, reviewResult.Feedback)

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

				// Update issue with plan after PLAN phase advances
				if currentPhase == PhasePlan && phaseOutput != "" {
					c.updateIssuePlan(ctx, truncateForPlan(phaseOutput))
				}

				advanced = true

			case VerdictIterate:
				// Feedback is already stored in memory by runJudge
				c.logInfo("Phase %s: judge requested iteration (feedback: %s)", currentPhase, judgeResult.Feedback)
				// Post judge verdict to PR if available (makes ITERATE visible)
				if prNumber := c.getPRNumberForTask(); prNumber != "" {
					c.postPRJudgeVerdict(ctx, prNumber, currentPhase, judgeResult)
				}
				continue

			case VerdictBlocked:
				state.Phase = PhaseBlocked
				c.logInfo("Phase %s: judge returned BLOCKED: %s", currentPhase, judgeResult.Feedback)
				// Post judge verdict to PR if available (makes BLOCKED visible)
				if prNumber := c.getPRNumberForTask(); prNumber != "" {
					c.postPRJudgeVerdict(ctx, prNumber, currentPhase, judgeResult)
				}
				return nil
			}

			if advanced {
				break
			}
		}

		if !advanced {
			// Exhausted max iterations without ADVANCE
			if currentPhase == PhaseDocs {
				// DOCS phase auto-succeeds - documentation should not block PR finalization
				c.logInfo("Phase %s: exhausted %d iterations, auto-advancing (non-blocking)", currentPhase, maxIter)
				c.postPhaseComment(ctx, currentPhase, maxIter,
					fmt.Sprintf("Auto-advanced: DOCS phase exhausted %d iterations (non-blocking)", maxIter))
			} else {
				// Set ControllerOverrode flag for NOMERGE handling during PR finalization
				state.ControllerOverrode = true
				c.logWarning("Phase %s: exhausted %d iterations without ADVANCE, forcing advance (NOMERGE flag set)", currentPhase, maxIter)
				c.postPhaseComment(ctx, currentPhase, maxIter,
					fmt.Sprintf("Forced advance: exhausted %d iterations without judge ADVANCE (PR will require human review)", maxIter))
			}
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

// truncateForPlan truncates plan text for use in GitHub issue bodies.
// Plans can be longer than comments but still need a reasonable limit.
func truncateForPlan(s string) string {
	const maxRunes = 4000
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "\n\n... (plan truncated)"
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
