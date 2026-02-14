package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/andywolf/agentium/internal/agent"
	"github.com/andywolf/agentium/internal/handoff"
	"github.com/andywolf/agentium/internal/observability"
)

// phaseLoopContext bundles the mutable state threaded through runPhaseLoop,
// eliminating the need to pass many local variables between extracted methods.
//
// Field ownership by file:
//
//	phase_loop.go          — initializes/resets all fields in the outer and inner loops
//	phase_loop_tracing.go  — manages tracing fields (traceCtx … traceStatus)
//	phase_loop_iteration.go — writes per-iteration output (phaseOutput, commentContent)
//	phase_loop_phases.go   — writes advanced, maxIter (complexity assessment), and state fields
//	phase_loop_eval.go     — writes advanced, noSignalCount, traceStatus, and state fields
type phaseLoopContext struct {
	taskID string
	state  *TaskState

	// Langfuse tracing — owned by phase_loop_tracing.go
	traceCtx          observability.TraceContext
	activeSpanCtx     observability.SpanContext
	activePhaseStart  time.Time
	hasActiveSpan     bool
	totalInputTokens  int
	totalOutputTokens int
	traceStatus       string // also set by phase_loop.go (cancelled/terminated) and phase_loop_eval.go (blocked)

	// Per-phase state (reset each phase in runPhaseLoop)
	currentPhase  TaskPhase
	maxIter       int  // also updated by handleComplexityAssessment (phase_loop_phases.go)
	advanced      bool // set by phase_loop_phases.go and phase_loop_eval.go
	noSignalCount int  // updated by applyJudgePostProcessing (phase_loop_eval.go)

	// Per-iteration output (reset each iteration in runPhaseLoop)
	phaseOutput    string // written by runWorkerIteration (phase_loop_iteration.go)
	commentContent string // written by runWorkerIteration (phase_loop_iteration.go)
}

// issuePhaseOrder defines the sequence of phases for issue tasks in the phase loop.
// TEST is merged into IMPLEMENT. REVIEW and PR_CREATION phases have been removed.
// Draft PRs are created during IMPLEMENT phase and finalized at PhaseComplete.
var issuePhaseOrder = []TaskPhase{
	PhasePlan,
	PhaseImplement,
}

// Default max iterations per phase when not configured.
const (
	defaultPlanMaxIter      = 3
	defaultImplementMaxIter = 5
	defaultVerifyMaxIter    = 3
)

// SIMPLE path max iterations - fewer iterations for straightforward changes.
const (
	simplePlanMaxIter      = 1
	simpleImplementMaxIter = 2
	simpleVerifyMaxIter    = 2
)

// defaultJudgeContextBudget is the default max characters of phase output sent to the judge.
// Increased from 8000 to 16000 to avoid truncating PLAN phase output that the judge/reviewer
// need to see in full for correct ADVANCE/ITERATE decisions.
const defaultJudgeContextBudget = 16000

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
// When auto-merge is enabled, VERIFY is appended after IMPLEMENT if not already present.
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
		return []TaskPhase{PhasePlan, PhaseImplement, PhaseVerify}
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

	plc := &phaseLoopContext{
		taskID: taskID,
		state:  state,
	}

	c.initPhaseLoopTrace(plc)
	defer c.completePhaseLoopTrace(plc)

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
			plc.traceStatus = "cancelled"
			return ctx.Err()
		default:
		}

		plc.currentPhase = state.Phase

		// Terminal phases end the loop - check BEFORE shouldTerminate() to ensure
		// finalizeDraftPR() is called when PhaseComplete is reached. shouldTerminate()
		// also returns true for terminal phases, so if we checked it first, we'd exit
		// the loop without finalizing the PR. See issue #284.
		if plc.currentPhase == PhaseComplete || plc.currentPhase == PhaseBlocked || plc.currentPhase == PhaseNothingToDo {
			// Finalize draft PR when completing successfully
			if plc.currentPhase == PhaseComplete && state.PRNumber != "" {
				if err := c.finalizeDraftPR(ctx, taskID); err != nil {
					c.logWarning("Failed to finalize draft PR: %v", err)
				}
			}
			c.logInfo("Phase loop: reached terminal phase %s", plc.currentPhase)
			plc.traceStatus = string(plc.currentPhase)
			return nil
		}

		// Check global termination conditions (iteration limits, time limits)
		// Note: This is checked AFTER terminal phase handling to avoid the race
		// condition where shouldTerminate() sees PhaseComplete and exits before
		// finalizeDraftPR() can run.
		if c.shouldTerminate() {
			c.logInfo("Phase loop: global termination condition met")
			plc.traceStatus = "terminated"
			return nil
		}

		// VERIFY phase pre-checks: skip if no PR or if NOMERGE flag is set
		if c.handleVerifyPreChecks(plc) {
			continue
		}

		// Mark PR as ready to trigger CI checks (only for VERIFY, after pre-checks pass)
		if plc.currentPhase == PhaseVerify {
			if err := c.markPRReady(ctx, state.PRNumber); err != nil {
				c.logWarning("VERIFY phase: failed to mark PR as ready: %v", err)
			}
		}

		plc.maxIter = c.phaseMaxIterations(plc.currentPhase, state.WorkflowPath)
		state.MaxPhaseIterations = plc.maxIter

		c.logInfo("Phase loop: entering phase %s (max %d iterations)", plc.currentPhase, plc.maxIter)

		// Start long-lived containers for this phase if container reuse is enabled
		if c.config.ContainerReuse {
			c.startPhaseContainerPool(ctx, plc.currentPhase)
		}

		c.startPhaseSpan(plc)

		// Reset per-phase state
		plc.advanced = false
		plc.noSignalCount = 0

		// Inner loop: iterate within the current phase
		for iter := 1; iter <= plc.maxIter; iter++ {
			select {
			case <-ctx.Done():
				plc.traceStatus = "cancelled"
				return ctx.Err()
			default:
			}

			if c.shouldTerminate() {
				plc.traceStatus = "terminated"
				return nil
			}

			// Refresh GitHub token if needed before each phase iteration
			if err := c.refreshGitHubTokenIfNeeded(); err != nil {
				c.logError("Phase %s: failed to refresh GitHub token: %v", plc.currentPhase, err)
				state.Phase = PhaseBlocked
				plc.traceStatus = "blocked"
				return fmt.Errorf("failed to refresh GitHub token: %w", err)
			}

			state.PhaseIteration = iter
			c.logInfo("Phase %s: iteration %d/%d", plc.currentPhase, iter, plc.maxIter)

			// Update the phase in state so skills/routing pick it up
			state.Phase = plc.currentPhase

			// Reset per-iteration state
			plc.phaseOutput = ""
			plc.commentContent = ""

			if err := c.runWorkerIteration(ctx, plc, iter); err != nil {
				c.logError("%v", err)
				continue
			}

			if handoffErr := c.processWorkerHandoff(plc, iter); handoffErr != nil {
				c.logError("Phase %s: fatal handoff error: %v", plc.currentPhase, handoffErr)
				state.Phase = PhaseBlocked
				state.ControllerOverrode = true
				c.postPhaseComment(ctx, plc.currentPhase, iter, RoleController,
					fmt.Sprintf("BLOCKED: %v — task requires human intervention.", handoffErr))
				if state.PRNumber != "" {
					c.postNOMERGEComment(ctx, state.PRNumber,
						fmt.Sprintf("Plan file write failed: %v", handoffErr))
				}
				return nil
			}

			if advanced, _, shouldContinue := c.handleVerifyPhase(ctx, plc, iter); advanced {
				break
			} else if shouldContinue {
				continue
			}

			// Post phase comment with filtered content (no tool results, max 250 lines)
			c.postPhaseComment(ctx, plc.currentPhase, iter, RoleWorker, plc.commentContent)

			// Create draft PR after first IMPLEMENT iteration with commits
			if plc.currentPhase == PhaseImplement && !state.DraftPRCreated {
				if prErr := c.maybeCreateDraftPR(ctx, taskID); prErr != nil {
					c.logWarning("Failed to create draft PR: %v", prErr)
				}
			}

			// Complexity assessment after PLAN iteration 1
			if c.handleComplexityAssessment(ctx, plc, iter) {
				break
			}

			// Review/judge pipeline
			advanced, blocked, shouldContinue := c.runReviewJudgePipeline(ctx, plc, iter)
			if blocked {
				return nil
			}
			if shouldContinue {
				continue
			}
			if advanced {
				break
			}
		}

		if !plc.advanced {
			c.handleExhaustedIterations(ctx, plc)
		}

		// Stop long-lived containers for this phase
		c.stopPhaseContainerPool(ctx)

		// End phase span in Langfuse
		phaseStatus := "completed"
		if !plc.advanced {
			phaseStatus = "exhausted"
		}
		c.endPhaseSpan(plc, phaseStatus)

		// Move to next phase
		nextPhase := c.advancePhase(plc.currentPhase)
		c.logInfo("Phase loop: advancing from %s to %s", plc.currentPhase, nextPhase)
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
