package controller

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/andywolf/agentium/internal/agent"
	"github.com/andywolf/agentium/internal/handoff"
	"github.com/andywolf/agentium/internal/memory"
	"github.com/andywolf/agentium/internal/observability"
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
	defaultVerifyMaxIter    = 3
)

// SIMPLE path max iterations - fewer iterations for straightforward changes.
const (
	simplePlanMaxIter      = 1
	simpleImplementMaxIter = 2
	simpleDocsMaxIter      = 1
	simpleVerifyMaxIter    = 2
)

// defaultJudgeContextBudget is the default max characters of phase output sent to the judge.
const defaultJudgeContextBudget = 8000

// Skip condition constants for reviewer/judge conditional skipping.
const (
	// SkipConditionEmptyOutput skips if the worker produced no meaningful output.
	SkipConditionEmptyOutput = "empty_output"
	// SkipConditionSimpleOutput skips if the worker output is short/trivial (< N lines).
	SkipConditionSimpleOutput = "simple_output"
	// SkipConditionNoCodeChanges skips if git diff shows no file changes.
	SkipConditionNoCodeChanges = "no_code_changes"
)

// simpleOutputLineThreshold is the maximum number of non-empty lines for output
// to be considered "simple" (trivial).
const simpleOutputLineThreshold = 10

// phaseMaxIterations returns the configured max iterations for a phase,
// considering the workflow path and falling back to defaults when not specified.
// Priority: SIMPLE path → custom phase config → PhaseLoopConfig → defaults.
func (c *Controller) phaseMaxIterations(phase TaskPhase, workflowPath WorkflowPath) int {
	// For SIMPLE path, use reduced iterations (highest priority)
	if workflowPath == WorkflowPathSimple {
		return simpleMaxIter(phase)
	}

	// Check custom phase step config (API-provided per-phase max_iterations)
	if stepCfg, ok := c.phaseConfigs[phase]; ok && stepCfg.MaxIterations > 0 {
		return stepCfg.MaxIterations
	}

	cfg := c.config.PhaseLoop

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
	case PhaseVerify:
		if cfg.VerifyMaxIterations > 0 {
			return cfg.VerifyMaxIterations
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
	case PhaseVerify:
		return defaultVerifyMaxIter
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
	case PhaseVerify:
		return simpleVerifyMaxIter
	default:
		return 1
	}
}

// evaluateSkipCondition evaluates whether a skip_on condition is met.
// Returns true if the condition is met and the phase should be skipped.
// Unrecognized conditions return false (safe default: don't skip).
func (c *Controller) evaluateSkipCondition(condition, phaseOutput, taskID string) bool {
	if condition == "" {
		return false
	}

	switch condition {
	case SkipConditionEmptyOutput:
		return c.isOutputEmpty(phaseOutput)
	case SkipConditionSimpleOutput:
		return c.isOutputSimple(phaseOutput)
	case SkipConditionNoCodeChanges:
		return c.implementOutputHasNoCodeChanges(taskID)
	default:
		// Unrecognized condition: don't skip (safe default)
		c.logWarning("Unrecognized skip_on condition: %q (ignoring)", condition)
		return false
	}
}

// isOutputEmpty returns true if the phase output is empty or contains only whitespace.
func (c *Controller) isOutputEmpty(output string) bool {
	return strings.TrimSpace(output) == ""
}

// isOutputSimple returns true if the phase output is short/trivial (fewer than simpleOutputLineThreshold non-empty lines).
func (c *Controller) isOutputSimple(output string) bool {
	if output == "" {
		return true
	}

	lines := strings.Split(output, "\n")
	nonEmptyCount := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			nonEmptyCount++
			if nonEmptyCount >= simpleOutputLineThreshold {
				return false
			}
		}
	}
	return true
}

// implementOutputHasNoCodeChanges returns true if the handoff store indicates
// no file changes were made during the IMPLEMENT phase.
func (c *Controller) implementOutputHasNoCodeChanges(taskID string) bool {
	if !c.isHandoffEnabled() || c.handoffStore == nil {
		return false
	}

	hd := c.handoffStore.GetPhaseOutput(taskID, handoff.PhaseImplement)
	if hd == nil || hd.ImplementOutput == nil {
		return false
	}

	return len(hd.ImplementOutput.FilesChanged) == 0
}

// shouldSkipReviewer returns true if the reviewer should be skipped.
// Boolean skip field takes precedence over skip_on condition.
func (c *Controller) shouldSkipReviewer(phaseOutput, taskID string) (skip bool, reason string) {
	if c.config.PhaseLoop == nil {
		return false, ""
	}

	// Boolean skip takes precedence
	if c.config.PhaseLoop.ReviewerSkip {
		return true, "reviewer_skip=true"
	}

	// Then evaluate skip_on condition if configured
	skipOnCondition := c.config.PhaseLoop.ReviewerSkipOn
	if skipOnCondition == "" {
		return false, ""
	}

	if c.evaluateSkipCondition(skipOnCondition, phaseOutput, taskID) {
		return true, skipOnCondition
	}
	return false, ""
}

// shouldSkipJudge returns true if the judge should be skipped.
// Boolean skip field takes precedence over skip_on condition.
func (c *Controller) shouldSkipJudge(phaseOutput, taskID string) (skip bool, reason string) {
	if c.config.PhaseLoop == nil {
		return false, ""
	}

	// Boolean skip takes precedence
	if c.config.PhaseLoop.JudgeSkip {
		return true, "judge_skip=true"
	}

	// Then evaluate skip_on condition if configured
	skipOnCondition := c.config.PhaseLoop.JudgeSkipOn
	if skipOnCondition == "" {
		return false, ""
	}

	if c.evaluateSkipCondition(skipOnCondition, phaseOutput, taskID) {
		return true, skipOnCondition
	}
	return false, ""
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
	return c.config.PhaseLoop.SkipPlanIfExists
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

// docsOutputIndicatesNoChanges returns true if the DOCS phase handoff output
// indicates no documentation changes were made.
func (c *Controller) docsOutputIndicatesNoChanges(taskID string) bool {
	if !c.isHandoffEnabled() || c.handoffStore == nil {
		return false
	}

	hd := c.handoffStore.GetPhaseOutput(taskID, handoff.PhaseDocs)
	if hd == nil || hd.DocsOutput == nil {
		return false
	}

	return len(hd.DocsOutput.DocsUpdated) == 0 && !hd.DocsOutput.ReadmeChanged
}

// tryVerifyMerge checks if the PR was merged by the worker (via handoff) or
// attempts a controller-side merge if CI checks passed. Returns true if merged,
// and any remaining CI failures reported by the worker (for retry feedback).
func (c *Controller) tryVerifyMerge(ctx context.Context, taskID string, state *TaskState) (bool, []string) {
	// Check handoff for worker-reported merge
	if c.isHandoffEnabled() {
		vo := c.handoffStore.GetVerifyOutput(taskID)
		if vo != nil {
			if vo.MergeSuccessful && state.PRNumber != "" {
				c.logInfo("VERIFY: agent reported successful merge via handoff")
				return true, nil
			}
			if vo.ChecksPassed && state.PRNumber != "" {
				// CI passed but agent didn't merge — controller tries
				if err := c.attemptPRMerge(ctx, state.PRNumber); err == nil {
					c.logInfo("VERIFY: controller merge fallback succeeded (CI passed)")
					return true, nil
				}
				c.logWarning("VERIFY: controller merge fallback failed despite CI passing")
			}
			// CI not passed — don't try merge
			return false, vo.RemainingFailures
		}
	}

	// No handoff data — try merge directly (GitHub branch protection will gate it)
	if state.PRNumber != "" {
		if err := c.attemptPRMerge(ctx, state.PRNumber); err == nil {
			c.logInfo("VERIFY: controller merge succeeded (no handoff data)")
			return true, nil
		}
	}
	return false, nil
}

// phaseOrder returns the active phase sequence based on config.
// When custom Phases are provided, derives order from them.
// When auto-merge is enabled, VERIFY is appended after DOCS if not already present.
func (c *Controller) phaseOrder() []TaskPhase {
	if len(c.config.Phases) > 0 {
		order := make([]TaskPhase, len(c.config.Phases))
		for i, p := range c.config.Phases {
			order[i] = TaskPhase(p.Name)
		}
		// Append VERIFY if auto-merge is enabled and not already in the list
		if c.config.AutoMerge && !containsPhase(order, PhaseVerify) {
			order = append(order, PhaseVerify)
		}
		return order
	}
	if c.config.AutoMerge {
		return []TaskPhase{PhasePlan, PhaseImplement, PhaseDocs, PhaseVerify}
	}
	return issuePhaseOrder
}

// containsPhase returns true if the phase slice contains the given phase.
func containsPhase(phases []TaskPhase, target TaskPhase) bool {
	for _, p := range phases {
		if p == target {
			return true
		}
	}
	return false
}

// advancePhase returns the next phase in the issue phase order.
// If the current phase is the last one (or not found), returns PhaseComplete.
func (c *Controller) advancePhase(current TaskPhase) TaskPhase {
	order := c.phaseOrder()
	for i, p := range order {
		if p == current {
			if i+1 < len(order) {
				return order[i+1]
			}
			return PhaseComplete
		}
	}
	return PhaseComplete
}

// runPhaseLoop executes the controller-as-judge phase loop for the active issue task.
// It iterates through phases, running the agent and judge at each step.
func (c *Controller) runPhaseLoop(ctx context.Context) error {
	taskID := taskKey("issue", c.activeTask)
	state := c.taskStates[taskID]
	if state == nil {
		return fmt.Errorf("no task state for %s", taskID)
	}

	c.logInfo("Starting phase loop for issue #%s (initial phase: %s)", c.activeTask, state.Phase)

	// Start Langfuse trace for this task
	traceCtx := c.tracer.StartTrace(taskID, observability.TraceOptions{
		Workflow:   "phase_loop",
		Repository: c.config.Repository,
		SessionID:  c.config.ID,
	})
	var totalInputTokens, totalOutputTokens int
	traceStatus := "error" // default status if function exits unexpectedly

	// Track active phase span for deferred cleanup on early returns
	var activeSpanCtx observability.SpanContext
	var activePhaseStart time.Time
	var hasActiveSpan bool

	endActiveSpan := func(status string) {
		if hasActiveSpan {
			c.tracer.EndPhase(activeSpanCtx, status, time.Since(activePhaseStart).Milliseconds())
			hasActiveSpan = false
		}
	}

	defer func() {
		endActiveSpan("interrupted")
		c.tracer.CompleteTrace(traceCtx, observability.CompleteOptions{
			Status:            traceStatus,
			TotalInputTokens:  totalInputTokens,
			TotalOutputTokens: totalOutputTokens,
		})
	}()

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
			traceStatus = "cancelled"
			return ctx.Err()
		default:
		}

		currentPhase := state.Phase

		// Terminal phases end the loop - check BEFORE shouldTerminate() to ensure
		// finalizeDraftPR() is called when PhaseComplete is reached. shouldTerminate()
		// also returns true for terminal phases, so if we checked it first, we'd exit
		// the loop without finalizing the PR. See issue #284.
		if currentPhase == PhaseComplete || currentPhase == PhaseBlocked || currentPhase == PhaseNothingToDo {
			// Finalize draft PR when completing successfully
			if currentPhase == PhaseComplete && state.PRNumber != "" {
				if err := c.finalizeDraftPR(ctx, taskID); err != nil {
					c.logWarning("Failed to finalize draft PR: %v", err)
				}
			}
			c.logInfo("Phase loop: reached terminal phase %s", currentPhase)
			traceStatus = string(currentPhase)
			return nil
		}

		// Check global termination conditions (iteration limits, time limits)
		// Note: This is checked AFTER terminal phase handling to avoid the race
		// condition where shouldTerminate() sees PhaseComplete and exits before
		// finalizeDraftPR() can run.
		if c.shouldTerminate() {
			c.logInfo("Phase loop: global termination condition met")
			traceStatus = "terminated"
			return nil
		}

		// VERIFY phase pre-checks: skip if no PR or if NOMERGE flag is set
		if currentPhase == PhaseVerify {
			if state.PRNumber == "" {
				c.logWarning("VERIFY phase: no PR number available, skipping to COMPLETE")
				state.Phase = PhaseComplete
				continue
			}
			if state.ControllerOverrode || state.JudgeOverrodeReviewer {
				c.logWarning("VERIFY phase: NOMERGE flag set, skipping auto-merge")
				state.Phase = PhaseComplete
				continue
			}
			// Mark PR as ready to trigger CI checks
			if err := c.markPRReady(ctx, state.PRNumber); err != nil {
				c.logWarning("VERIFY phase: failed to mark PR as ready: %v", err)
			}
		}

		maxIter := c.phaseMaxIterations(currentPhase, state.WorkflowPath)
		state.MaxPhaseIterations = maxIter

		c.logInfo("Phase loop: entering phase %s (max %d iterations)", currentPhase, maxIter)

		// Start long-lived containers for this phase if container reuse is enabled
		if c.config.ContainerReuse {
			c.startPhaseContainerPool(ctx, currentPhase)
		}

		// Start Langfuse span for this phase
		activePhaseStart = time.Now()
		activeSpanCtx = c.tracer.StartPhase(traceCtx, string(currentPhase), observability.SpanOptions{
			MaxIterations: maxIter,
		})
		hasActiveSpan = true

		// Inner loop: iterate within the current phase
		advanced := false
		noSignalCount := 0
		for iter := 1; iter <= maxIter; iter++ {
			select {
			case <-ctx.Done():
				traceStatus = "cancelled"
				return ctx.Err()
			default:
			}

			if c.shouldTerminate() {
				traceStatus = "terminated"
				return nil
			}

			// Refresh GitHub token if needed before each phase iteration
			// This handles long-running phases that might exceed token lifetime
			if err := c.refreshGitHubTokenIfNeeded(); err != nil {
				c.logError("Phase %s: failed to refresh GitHub token: %v", currentPhase, err)
				state.Phase = PhaseBlocked
				traceStatus = "blocked"
				return fmt.Errorf("failed to refresh GitHub token: %w", err)
			}

			state.PhaseIteration = iter
			c.logInfo("Phase %s: iteration %d/%d", currentPhase, iter, maxIter)

			// Update the phase in state so skills/routing pick it up
			state.Phase = currentPhase

			// Check for pre-existing plan (PLAN phase, iteration 1 only)
			var phaseOutput string    // Full output for internal processing (handoff, judge)
			var commentContent string // Filtered output for GitHub comments
			var skipIteration bool
			if c.shouldSkipPlanIteration(currentPhase, iter) {
				planContent := c.extractExistingPlan()
				c.logInfo("Phase %s: detected pre-existing plan in issue body, skipping agent iteration", currentPhase)
				phaseOutput = planContent
				skipIteration = true
				c.postPhaseComment(ctx, currentPhase, iter, RoleController, "Pre-existing plan detected in issue body (skipped planning agent)")
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

				// Record Worker generation in Langfuse
				c.tracer.RecordGeneration(activeSpanCtx, observability.GenerationInput{
					Name:         "Worker",
					Model:        c.config.Agent,
					Input:        result.PromptInput,
					Output:       result.RawTextContent,
					InputTokens:  result.InputTokens,
					OutputTokens: result.OutputTokens,
					Status:       "completed",
					StartTime:    result.StartTime,
					EndTime:      result.EndTime,
				})
				totalInputTokens += result.InputTokens
				totalOutputTokens += result.OutputTokens

				// Full output for internal processing (handoff parsing, judge context)
				phaseOutput = result.RawTextContent
				if phaseOutput == "" {
					phaseOutput = result.Summary
				}

				// Filtered output for GitHub comments (assistant text only, no tool results)
				commentContent = result.AssistantText
				if commentContent == "" {
					commentContent = phaseOutput
				}
				commentContent = StripAgentiumSignals(commentContent)
				commentContent = StripPreamble(commentContent)
				commentContent = SummarizeForComment(commentContent, 250)
			}

			// Parse and store handoff output if enabled
			if c.isHandoffEnabled() && phaseOutput != "" {
				if handoffErr := c.processHandoffOutput(taskID, currentPhase, iter, phaseOutput); handoffErr != nil {
					c.logWarning("Failed to process handoff output for phase %s: %v", currentPhase, handoffErr)
				}
			}

			// When plan skip triggers and processHandoffOutput didn't store a PlanOutput
			// (because the issue body doesn't contain AGENTIUM_HANDOFF), create a minimal
			// PlanOutput from the issue body's structured plan sections.
			if skipIteration && c.isHandoffEnabled() {
				hd := c.handoffStore.GetPhaseOutput(taskID, handoff.PhasePlan)
				if hd == nil || hd.PlanOutput == nil {
					planOutput := extractPlanFromIssueBody(phaseOutput)
					if planOutput != nil {
						if storeErr := c.handoffStore.StorePhaseOutput(taskID, handoff.PhasePlan, iter, planOutput); storeErr != nil {
							c.logWarning("Failed to store extracted plan from issue body: %v", storeErr)
						} else {
							c.logInfo("Stored plan extracted from issue body in handoff store")
							if saveErr := c.handoffStore.Save(); saveErr != nil {
								c.logWarning("Failed to persist handoff store: %v", saveErr)
							}
						}
					}
				}
			}

			// For DOCS phase: skip reviewer/judge if no documentation changes were made
			if currentPhase == PhaseDocs && c.docsOutputIndicatesNoChanges(taskID) {
				c.logInfo("Phase %s: no documentation changes detected, skipping review/judge", currentPhase)
				c.postPhaseComment(ctx, currentPhase, iter, RoleController,
					"No documentation changes detected — skipping review (auto-advance)")

				// Clear feedback and record phase result
				if c.memoryStore != nil {
					c.memoryStore.ClearByType(memory.EvalFeedback)
					c.memoryStore.Update([]memory.Signal{
						{Type: memory.PhaseResult, Content: fmt.Sprintf("%s completed (no changes, auto-advanced)", currentPhase)},
					}, c.iteration, taskID)
				}
				advanced = true
				break
			}

			// For VERIFY phase: skip reviewer/judge (CI check/merge is mechanical, not creative)
			if currentPhase == PhaseVerify {
				merged, remainingFailures := c.tryVerifyMerge(ctx, taskID, state)
				if merged {
					state.PRMerged = true
					c.postPhaseComment(ctx, currentPhase, iter, RoleController,
						"Merge successful — skipping review (auto-advance)")
					if c.memoryStore != nil {
						c.memoryStore.ClearByType(memory.EvalFeedback)
						c.memoryStore.Update([]memory.Signal{
							{Type: memory.PhaseResult, Content: fmt.Sprintf("%s completed (merge successful, iteration %d)", currentPhase, iter)},
						}, c.iteration, taskID)
					}
					advanced = true
					break
				}
				// Not merged — surface remaining failures so worker knows what to fix
				retryMsg := "Merge not yet successful — iterating"
				if len(remainingFailures) > 0 {
					retryMsg = fmt.Sprintf("Merge not yet successful — remaining failures: %s", strings.Join(remainingFailures, ", "))
				}
				c.logInfo("VERIFY: not yet merged, continuing to iteration %d/%d", iter+1, maxIter)
				c.postPhaseComment(ctx, currentPhase, iter, RoleController, retryMsg)
				continue
			}

			// Post phase comment with filtered content (no tool results, max 250 lines)
			c.postPhaseComment(ctx, currentPhase, iter, RoleWorker, commentContent)

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
					state.WorkflowPath = complexityResult.Verdict
					c.logInfo("Workflow path set to %s: %s", state.WorkflowPath, complexityResult.Feedback)
				}

				// Post complexity verdict comment
				c.postPhaseComment(ctx, currentPhase, iter, RoleComplexityAssessor,
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
					// Post implementation plan as a comment (follows "append only" principle)
					if phaseOutput != "" {
						c.postImplementationPlan(ctx, c.formatPlanForComment(taskID, phaseOutput))
					}
					advanced = true
					break
				}

				// For COMPLEX tasks, recalculate max iterations now that we know the path
				maxIter = c.phaseMaxIterations(currentPhase, state.WorkflowPath)
				state.MaxPhaseIterations = maxIter
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

			// Extract worker's feedback responses from phase output (iteration > 1 only)
			var workerFeedbackResponses string
			if iter > 1 && phaseOutput != "" {
				responses := extractFeedbackResponses(phaseOutput)
				if len(responses) > 0 {
					workerFeedbackResponses = strings.Join(responses, "\n")
				}
			}

			// Check if reviewer should be skipped (skip=true takes precedence over skip_on)
			if skipReviewer, skipReason := c.shouldSkipReviewer(phaseOutput, taskID); skipReviewer {
				c.logInfo("Phase %s: reviewer skip condition met (%s), skipping review/judge (auto-advance)", currentPhase, skipReason)
				c.tracer.RecordSkipped(activeSpanCtx, "Reviewer", skipReason)
				c.tracer.RecordSkipped(activeSpanCtx, "Judge", "reviewer_skipped")
				c.postPhaseComment(ctx, currentPhase, iter, RoleController,
					fmt.Sprintf("Reviewer skip condition (%s) met — skipping review (auto-advance)", skipReason))

				// Clear feedback and record phase result
				if c.memoryStore != nil {
					c.memoryStore.ClearByType(memory.EvalFeedback)
					c.memoryStore.Update([]memory.Signal{
						{Type: memory.PhaseResult, Content: fmt.Sprintf("%s completed (skip condition met, auto-advanced)", currentPhase)},
					}, c.iteration, taskID)
				}
				advanced = true
				break
			}

			// Run reviewer + judge
			reviewResult, reviewErr := c.runReviewer(ctx, reviewRunParams{
				CompletedPhase:          currentPhase,
				PhaseOutput:             phaseOutput,
				Iteration:               iter,
				MaxIterations:           maxIter,
				PreviousFeedback:        previousFeedback,
				WorkerHandoffSummary:    workerHandoffSummary,
				WorkerFeedbackResponses: workerFeedbackResponses,
				ParentBranch:            state.ParentBranch,
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

			// Record Reviewer generation in Langfuse
			c.tracer.RecordGeneration(activeSpanCtx, observability.GenerationInput{
				Name:         "Reviewer",
				Model:        c.config.Agent,
				Input:        reviewResult.Prompt,
				Output:       reviewResult.Feedback,
				InputTokens:  reviewResult.InputTokens,
				OutputTokens: reviewResult.OutputTokens,
				Status:       "completed",
				StartTime:    reviewResult.StartTime,
				EndTime:      reviewResult.EndTime,
			})
			totalInputTokens += reviewResult.InputTokens
			totalOutputTokens += reviewResult.OutputTokens

			// Post reviewer feedback to appropriate location (filtered for readability)
			reviewFeedbackComment := StripAgentiumSignals(reviewResult.Feedback)
			reviewFeedbackComment = StripPreamble(reviewFeedbackComment)
			reviewFeedbackComment = SummarizeForComment(reviewFeedbackComment, 250)
			c.postReviewFeedbackForPhase(ctx, currentPhase, iter, reviewFeedbackComment)

			priorDirectives := ""
			if c.memoryStore != nil && iter > 1 {
				priorDirectives = c.memoryStore.BuildJudgeHistoryContext(taskID, iter)
			}

			// Check if judge should be skipped (skip=true takes precedence over skip_on)
			if skipJudge, skipReason := c.shouldSkipJudge(phaseOutput, taskID); skipJudge {
				c.logInfo("Phase %s: judge skip condition met (%s), skipping judge (auto-advance)", currentPhase, skipReason)
				c.tracer.RecordSkipped(activeSpanCtx, "Judge", skipReason)
				c.postPhaseComment(ctx, currentPhase, iter, RoleController,
					fmt.Sprintf("Judge skip condition (%s) met — skipping judge (auto-advance)", skipReason))

				// Clear feedback and record phase result
				if c.memoryStore != nil {
					c.memoryStore.ClearByType(memory.EvalFeedback)
					c.memoryStore.Update([]memory.Signal{
						{Type: memory.PhaseResult, Content: fmt.Sprintf("%s completed (judge skip condition met, auto-advanced)", currentPhase)},
					}, c.iteration, taskID)
				}
				advanced = true
				break
			}

			judgeResult, err := c.runJudge(ctx, judgeRunParams{
				CompletedPhase:  currentPhase,
				PhaseOutput:     phaseOutput,
				ReviewFeedback:  reviewResult.Feedback,
				Iteration:       iter,
				MaxIterations:   maxIter,
				PhaseIteration:  iter,
				PriorDirectives: priorDirectives,
			})
			if err != nil {
				c.logWarning("Judge error for phase %s: %v (defaulting to ADVANCE)", currentPhase, err)
				judgeResult = JudgeResult{Verdict: VerdictAdvance}
			}

			// Record Judge generation in Langfuse
			c.tracer.RecordGeneration(activeSpanCtx, observability.GenerationInput{
				Name:         "Judge",
				Model:        c.config.Agent,
				Input:        judgeResult.Prompt,
				Output:       judgeResult.Output,
				InputTokens:  judgeResult.InputTokens,
				OutputTokens: judgeResult.OutputTokens,
				Status:       "completed",
				StartTime:    judgeResult.StartTime,
				EndTime:      judgeResult.EndTime,
			})
			totalInputTokens += judgeResult.InputTokens
			totalOutputTokens += judgeResult.OutputTokens

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

			// Hard-gate: PLAN phase cannot advance without a valid PlanOutput in the handoff store
			if currentPhase == PhasePlan && judgeResult.Verdict == VerdictAdvance && c.isHandoffEnabled() {
				hd := c.handoffStore.GetPhaseOutput(taskID, handoff.PhasePlan)
				if hd == nil || hd.PlanOutput == nil {
					c.logWarning("PLAN: judge ADVANCE but no AGENTIUM_HANDOFF signal — forcing ITERATE")
					judgeResult = JudgeResult{
						Verdict:  VerdictIterate,
						Feedback: "No AGENTIUM_HANDOFF signal detected. You must emit the structured handoff signal with your plan before the PLAN phase can advance.",
						// SignalFound: true so noSignalCount resets — we want the hard-gate
						// to persist across iterations without triggering the no-signal fail-safe.
						SignalFound: true,
					}
					state.LastJudgeVerdict = string(judgeResult.Verdict)
					state.LastJudgeFeedback = judgeResult.Feedback
				}
			}

			// Detect when judge overrides reviewer's recommendation (NOMERGE trigger)
			if judgeResult.Verdict == VerdictAdvance {
				reviewerVerdict := extractReviewerVerdict(reviewResult.Feedback)
				if reviewerVerdict == VerdictIterate || reviewerVerdict == VerdictBlocked {
					state.JudgeOverrodeReviewer = true
					c.logWarning("Phase %s: judge ADVANCE overrode reviewer %s", currentPhase, reviewerVerdict)
				}
			}

			// Post judge comment
			c.postJudgeComment(ctx, currentPhase, iter, judgeResult)

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

				// Post implementation plan as a comment after PLAN phase advances
				// (postImplementationPlan follows "append only" principle)
				if currentPhase == PhasePlan && phaseOutput != "" {
					c.postImplementationPlan(ctx, c.formatPlanForComment(taskID, phaseOutput))
				}

				advanced = true

			case VerdictIterate:
				// Feedback is already stored in memory by runJudge
				c.logInfo("Phase %s: judge requested iteration (feedback: %s)", currentPhase, judgeResult.Feedback)
				continue

			case VerdictBlocked:
				state.Phase = PhaseBlocked
				c.logInfo("Phase %s: judge returned BLOCKED: %s", currentPhase, judgeResult.Feedback)
				endActiveSpan("blocked")
				traceStatus = "blocked"
				return nil
			}

			if advanced {
				break
			}
		}

		if !advanced {
			// Exhausted max iterations without ADVANCE
			switch currentPhase {
			case PhaseDocs:
				// DOCS phase auto-succeeds - documentation should not block PR finalization
				c.logInfo("Phase %s: exhausted %d iterations, auto-advancing (non-blocking)", currentPhase, maxIter)
				c.postPhaseComment(ctx, currentPhase, maxIter, RoleController,
					fmt.Sprintf("Auto-advanced: DOCS phase exhausted %d iterations (non-blocking)", maxIter))
			case PhaseVerify:
				// VERIFY phase exhaustion: PR is already ready for review, note that auto-merge failed
				c.logWarning("Phase %s: exhausted %d iterations, auto-merge failed (PR remains ready for human review)", currentPhase, maxIter)
				c.postPhaseComment(ctx, currentPhase, maxIter, RoleController,
					fmt.Sprintf("Auto-merge failed: exhausted %d iterations. PR is ready for human review.", maxIter))
			default:
				// Set ControllerOverrode flag for NOMERGE handling during PR finalization
				state.ControllerOverrode = true
				c.logWarning("Phase %s: exhausted %d iterations without ADVANCE, forcing advance (NOMERGE flag set)", currentPhase, maxIter)
				c.postPhaseComment(ctx, currentPhase, maxIter, RoleController,
					fmt.Sprintf("Forced advance: exhausted %d iterations without judge ADVANCE (PR will require human review)", maxIter))
			}
			if c.memoryStore != nil {
				c.memoryStore.ClearByType(memory.EvalFeedback)
			}
		}

		// Stop long-lived containers for this phase
		c.stopPhaseContainerPool(ctx)

		// End phase span in Langfuse
		phaseStatus := "completed"
		if !advanced {
			phaseStatus = "exhausted"
		}
		endActiveSpan(phaseStatus)

		// Move to next phase
		nextPhase := c.advancePhase(currentPhase)
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
	case PhaseVerify:
		if hd.VerifyOutput != nil {
			if hd.VerifyOutput.ChecksPassed {
				parts = append(parts, "CI checks: Passed")
			} else if len(hd.VerifyOutput.RemainingFailures) > 0 {
				parts = append(parts, fmt.Sprintf("Remaining failures: %v", hd.VerifyOutput.RemainingFailures))
			}
			if hd.VerifyOutput.MergeSuccessful {
				parts = append(parts, fmt.Sprintf("Merge: Successful (SHA: %s)", hd.VerifyOutput.MergeSHA))
			}
		}
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, "\n")
}

// formatPlanForComment formats the implementation plan for posting as a GitHub
// comment. When structured handoff data is available, it renders a clean
// markdown summary. Otherwise it falls back to stripping signals from the raw output.
func (c *Controller) formatPlanForComment(taskID, rawOutput string) string {
	if c.isHandoffEnabled() && c.handoffStore != nil {
		hd := c.handoffStore.GetPhaseOutput(taskID, handoff.PhasePlan)
		if hd != nil && hd.PlanOutput != nil {
			return formatPlanOutput(hd.PlanOutput)
		}
	}
	// Fallback: strip signals and cap length for GitHub comment limit
	stripped := StripAgentiumSignals(rawOutput)
	return SummarizeForComment(stripped, 200)
}

// formatPlanOutput renders a PlanOutput struct as clean markdown.
func formatPlanOutput(plan *handoff.PlanOutput) string {
	var sb strings.Builder

	if plan.Summary != "" {
		sb.WriteString(plan.Summary)
		sb.WriteString("\n\n")
	}

	if len(plan.FilesToModify) > 0 || len(plan.FilesToCreate) > 0 {
		sb.WriteString("### Files\n\n")
		for _, f := range plan.FilesToModify {
			sb.WriteString(fmt.Sprintf("- `%s` (modify)\n", f))
		}
		for _, f := range plan.FilesToCreate {
			sb.WriteString(fmt.Sprintf("- `%s` (create)\n", f))
		}
		sb.WriteString("\n")
	}

	if len(plan.ImplementationSteps) > 0 {
		sb.WriteString("### Steps\n\n")
		for _, step := range plan.ImplementationSteps {
			sb.WriteString(fmt.Sprintf("%d. %s", step.Order, step.Description))
			if step.File != "" {
				sb.WriteString(fmt.Sprintf(" (`%s`)", step.File))
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	if plan.TestingApproach != "" {
		sb.WriteString("### Testing\n\n")
		sb.WriteString(plan.TestingApproach)
		sb.WriteString("\n")
	}

	return strings.TrimSpace(sb.String())
}

// issuePlanFilePattern matches file paths in list items (e.g., "- `path/to/file.go`" or "- path/to/file.go").
var issuePlanFilePattern = regexp.MustCompile(`(?m)^[\s]*[-*]\s+` + "`?" + `([\w./\-]+\.\w+)` + "`?")

// issuePlanStepPattern matches numbered list items (e.g., "1. Do something").
var issuePlanStepPattern = regexp.MustCompile(`(?m)^[\s]*(\d+)\.\s+(.+)$`)

// extractPlanFromIssueBody parses an issue body's structured plan sections into a
// handoff.PlanOutput. This is deterministic regex/string parsing — no LLM call needed.
// Returns nil if the body does not contain enough plan structure.
func extractPlanFromIssueBody(body string) *handoff.PlanOutput {
	if body == "" {
		return nil
	}

	plan := &handoff.PlanOutput{}

	// Extract sections by splitting on markdown headings
	sections := splitMarkdownSections(body)

	for heading, content := range sections {
		normalizedHeading := strings.ToLower(strings.TrimSpace(heading))
		switch {
		case strings.Contains(normalizedHeading, "summary"):
			plan.Summary = strings.TrimSpace(content)
		case strings.Contains(normalizedHeading, "files to create/modify"):
			// Combined section: treat all as files to modify
			plan.FilesToModify = extractFilePaths(content)
		case strings.Contains(normalizedHeading, "files to modify"):
			plan.FilesToModify = extractFilePaths(content)
		case strings.Contains(normalizedHeading, "files to create"):
			plan.FilesToCreate = extractFilePaths(content)
		case strings.Contains(normalizedHeading, "implementation"):
			plan.ImplementationSteps = extractSteps(content)
		case strings.Contains(normalizedHeading, "test"):
			plan.TestingApproach = strings.TrimSpace(content)
		}
	}

	// Only return a plan if we extracted something meaningful
	if plan.Summary == "" && len(plan.FilesToModify) == 0 && len(plan.FilesToCreate) == 0 && len(plan.ImplementationSteps) == 0 {
		return nil
	}

	return plan
}

// splitMarkdownSections splits markdown content by ## or # headings, returning
// a map of heading text to section content.
func splitMarkdownSections(body string) map[string]string {
	sections := make(map[string]string)
	lines := strings.Split(body, "\n")

	currentHeading := ""
	var currentContent strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") || strings.HasPrefix(trimmed, "# ") {
			// Save previous section
			if currentHeading != "" {
				sections[currentHeading] = currentContent.String()
			}
			currentHeading = strings.TrimLeft(trimmed, "# ")
			currentContent.Reset()
		} else if currentHeading != "" {
			currentContent.WriteString(line)
			currentContent.WriteString("\n")
		}
	}

	// Save last section
	if currentHeading != "" {
		sections[currentHeading] = currentContent.String()
	}

	return sections
}

// extractFilePaths extracts file paths from markdown list items.
func extractFilePaths(content string) []string {
	matches := issuePlanFilePattern.FindAllStringSubmatch(content, -1)
	paths := make([]string, 0, len(matches))
	seen := make(map[string]bool)
	for _, m := range matches {
		path := m[1]
		if !seen[path] {
			paths = append(paths, path)
			seen[path] = true
		}
	}
	return paths
}

// extractSteps extracts numbered steps from markdown content.
func extractSteps(content string) []handoff.ImplementationStep {
	matches := issuePlanStepPattern.FindAllStringSubmatch(content, -1)
	steps := make([]handoff.ImplementationStep, 0, len(matches))
	for i, m := range matches {
		order := i + 1
		if len(m) >= 3 {
			// Try to parse the step number; fallback to sequential
			if n := parseStepNumber(m[1]); n > 0 {
				order = n
			}
			steps = append(steps, handoff.ImplementationStep{
				Order:       order,
				Description: strings.TrimSpace(m[2]),
			})
		}
	}
	return steps
}

// parseStepNumber converts a string like "1" to an int, returning 0 on failure.
func parseStepNumber(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// knownPhases is the set of built-in phases that have default skill prompts.
var knownPhases = map[TaskPhase]bool{
	PhasePlan:      true,
	PhaseImplement: true,
	PhaseDocs:      true,
	PhaseVerify:    true,
}

// validatePhases validates the Phases configuration.
// Known phases (PLAN, IMPLEMENT, DOCS, VERIFY) don't require prompts.
// Unknown phases require a worker.prompt since no built-in skills exist for them.
func validatePhases(phases []PhaseStepConfig) error {
	seen := make(map[string]bool, len(phases))
	for _, p := range phases {
		if p.Name == "" {
			return fmt.Errorf("phase name must not be empty")
		}
		if seen[p.Name] {
			return fmt.Errorf("duplicate phase name: %s", p.Name)
		}
		seen[p.Name] = true

		// Unknown phases must have a worker prompt
		if !knownPhases[TaskPhase(p.Name)] {
			if p.Worker == nil || p.Worker.Prompt == "" {
				return fmt.Errorf("unknown phase %q requires worker.prompt", p.Name)
			}
		}
	}
	return nil
}

// phaseWorkerPrompt returns the API-provided worker prompt for a phase, or empty string.
func (c *Controller) phaseWorkerPrompt(phase TaskPhase) string {
	if stepCfg, ok := c.phaseConfigs[phase]; ok && stepCfg.Worker != nil {
		return stepCfg.Worker.Prompt
	}
	return ""
}

// phaseReviewerPrompt returns the API-provided reviewer prompt for a phase, or empty string.
func (c *Controller) phaseReviewerPrompt(phase TaskPhase) string {
	if stepCfg, ok := c.phaseConfigs[phase]; ok && stepCfg.Reviewer != nil {
		return stepCfg.Reviewer.Prompt
	}
	return ""
}

// phaseJudgeCriteria returns the API-provided judge criteria for a phase, or empty string.
func (c *Controller) phaseJudgeCriteria(phase TaskPhase) string {
	if stepCfg, ok := c.phaseConfigs[phase]; ok && stepCfg.Judge != nil {
		return stepCfg.Judge.Criteria
	}
	return ""
}

// startPhaseContainerPool creates and starts long-lived containers for the
// given phase. Each role (worker, reviewer, judge) gets its own container
// with the correct adapter image, environment, and auth mounts based on
// model routing configuration.
func (c *Controller) startPhaseContainerPool(ctx context.Context, phase TaskPhase) {
	pool := NewContainerPool(c.workDir, c.containerMemLimit, c.config.ID, string(phase), c.execCommand, c.logger, c.logWarning)

	// Base session for building env
	session := &agent.Session{
		ID:             c.config.ID,
		Repository:     c.config.Repository,
		GitHubToken:    c.gitHubToken,
		ClaudeAuthMode: c.config.ClaudeAuth.AuthMode,
	}

	// Resolve per-role adapters using the same compound key fallback chains
	// as reviewer.go and judge.go
	roles := []ContainerRole{RoleWorkerContainer, RoleReviewerContainer, RoleJudgeContainer}
	for _, role := range roles {
		roleAgent := c.resolveAgentForRole(phase, role)

		c.ensureGHCRAuth(ctx, roleAgent.ContainerImage())

		env := roleAgent.BuildEnv(session, 0)
		authMounts := c.buildAuthMounts(roleAgent)

		if _, err := pool.Start(ctx, role, roleAgent.ContainerImage(), roleAgent.ContainerEntrypoint(), env, authMounts); err != nil {
			c.logWarning("Failed to start pooled container for role %s: %v (falling back to one-shot)", role, err)
			pool.StopAll(ctx)
			return
		}
	}

	c.containerPool = pool
	c.logInfo("Container pool started for phase %s (3 containers)", phase)
}

// stopPhaseContainerPool stops and removes all containers in the current pool.
func (c *Controller) stopPhaseContainerPool(ctx context.Context) {
	if c.containerPool == nil {
		return
	}
	c.containerPool.StopAll(ctx)
	c.containerPool = nil
	c.logInfo("Container pool stopped")
}

// resolveAgentForRole returns the agent adapter to use for a given phase and
// container role, using the same compound key fallback chains as reviewer.go
// and judge.go:
//   - Worker:   {PHASE} → default
//   - Reviewer: {PHASE}_REVIEW → REVIEW → default
//   - Judge:    {PHASE}_JUDGE  → JUDGE  → default
func (c *Controller) resolveAgentForRole(phase TaskPhase, role ContainerRole) agent.Agent {
	if c.modelRouter == nil || !c.modelRouter.IsConfigured() {
		return c.agent
	}

	phaseStr := string(phase)
	var modelCfg = c.modelRouter.ModelForPhase(phaseStr) // Worker default

	switch role {
	case RoleReviewerContainer:
		modelCfg = c.modelRouter.ModelForPhase(fmt.Sprintf("%s_REVIEW", phaseStr))
		if modelCfg.Adapter == "" && modelCfg.Model == "" {
			modelCfg = c.modelRouter.ModelForPhase("REVIEW")
		}
	case RoleJudgeContainer:
		modelCfg = c.modelRouter.ModelForPhase(fmt.Sprintf("%s_JUDGE", phaseStr))
		if modelCfg.Adapter == "" && modelCfg.Model == "" {
			modelCfg = c.modelRouter.ModelForPhase("JUDGE")
		}
	}

	if modelCfg.Adapter != "" {
		if a, ok := c.adapters[modelCfg.Adapter]; ok {
			return a
		}
	}
	return c.agent
}
