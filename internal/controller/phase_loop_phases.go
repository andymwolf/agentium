package controller

import (
	"context"
	"fmt"
	"strings"

	"github.com/andywolf/agentium/internal/memory"
)

// recordPhaseAdvance clears feedback memory and records a phase result.
// This pattern is used in multiple places where a phase auto-advances.
func (c *Controller) recordPhaseAdvance(plc *phaseLoopContext, reason string) {
	if c.memoryStore != nil {
		c.memoryStore.ClearByType(memory.EvalFeedback)
		c.memoryStore.Update([]memory.Signal{
			{Type: memory.PhaseResult, Content: reason},
		}, c.iteration, plc.taskID)
	}
}

// handleVerifyPreChecks handles pre-checks for the VERIFY phase.
// Returns true if the phase should be skipped (sets state.Phase to PhaseComplete).
func (c *Controller) handleVerifyPreChecks(plc *phaseLoopContext) bool {
	if plc.currentPhase != PhaseVerify {
		return false
	}
	if plc.state.PRNumber == "" {
		c.logWarning("VERIFY phase: no PR number available, skipping to COMPLETE")
		plc.state.Phase = PhaseComplete
		return true
	}
	if plc.state.ControllerOverrode || plc.state.JudgeOverrodeReviewer {
		c.logWarning("VERIFY phase: NOMERGE flag set, skipping auto-merge")
		plc.state.Phase = PhaseComplete
		return true
	}
	return false
}

// handleVerifyPhase handles VERIFY phase logic (merge attempt or retry).
// Returns the same (advanced, blocked, shouldContinue) tuple as runReviewJudgePipeline
// for consistent flow control at the call site. blocked is always false here since
// VERIFY never blocks.
func (c *Controller) handleVerifyPhase(ctx context.Context, plc *phaseLoopContext, iter int) (advanced, blocked, shouldContinue bool) { //nolint:unparam // blocked is always false but kept for API consistency with runReviewJudgePipeline
	if plc.currentPhase != PhaseVerify {
		return false, false, false
	}
	merged, remainingFailures := c.tryVerifyMerge(ctx, plc.taskID, plc.state)
	if merged {
		plc.state.PRMerged = true
		c.postPhaseComment(ctx, plc.currentPhase, iter, RoleController,
			"Merge successful — skipping review (auto-advance)")
		c.recordPhaseAdvance(plc, fmt.Sprintf("%s completed (merge successful, iteration %d)", plc.currentPhase, iter))
		plc.advanced = true
		return true, false, false
	}
	// Not merged — surface remaining failures so worker knows what to fix
	retryMsg := "Merge not yet successful — iterating"
	if len(remainingFailures) > 0 {
		retryMsg = fmt.Sprintf("Merge not yet successful — remaining failures: %s", strings.Join(remainingFailures, ", "))
	}
	c.logInfo("VERIFY: not yet merged, continuing to iteration %d/%d", iter+1, plc.maxIter)
	c.postPhaseComment(ctx, plc.currentPhase, iter, RoleController, retryMsg)
	return false, false, true
}

// handleComplexityAssessment runs the complexity assessor after PLAN iteration 1
// and handles SIMPLE path auto-advance. Returns true if advanced (break inner loop).
func (c *Controller) handleComplexityAssessment(ctx context.Context, plc *phaseLoopContext, iter int) bool {
	if plc.currentPhase != PhasePlan || iter != 1 || plc.state.WorkflowPath != WorkflowPathUnset {
		return false
	}
	complexityResult, complexityErr := c.runComplexityAssessor(ctx, complexityRunParams{
		PlanOutput:    plc.phaseOutput,
		Iteration:     iter,
		MaxIterations: plc.maxIter,
	})
	if complexityErr != nil {
		c.logWarning("Complexity assessor error: %v (defaulting to COMPLEX)", complexityErr)
		plc.state.WorkflowPath = WorkflowPathComplex
		c.postPhaseComment(ctx, plc.currentPhase, iter, RoleComplexityAssessor,
			fmt.Sprintf("Complexity assessment: **%s** (assessor error: %v)", plc.state.WorkflowPath, complexityErr))
	} else {
		plc.state.WorkflowPath = complexityResult.Verdict
		c.logInfo("Workflow path set to %s: %s", plc.state.WorkflowPath, complexityResult.Feedback)
		c.postPhaseComment(ctx, plc.currentPhase, iter, RoleComplexityAssessor,
			fmt.Sprintf("Complexity assessment: **%s**\n\n%s", plc.state.WorkflowPath, complexityResult.Feedback))
	}

	// For SIMPLE tasks, auto-advance from PLAN (skip reviewer/judge)
	if plc.state.WorkflowPath == WorkflowPathSimple {
		c.logInfo("SIMPLE workflow: auto-advancing from PLAN phase")
		c.recordPhaseAdvance(plc, fmt.Sprintf("%s completed (SIMPLE path, iteration %d)", plc.currentPhase, iter))
		// Post implementation plan as a comment (follows "append only" principle)
		if plc.phaseOutput != "" {
			c.postImplementationPlan(ctx, c.formatPlanForComment(plc.taskID, plc.phaseOutput))
		}
		plc.advanced = true
		return true
	}

	// For COMPLEX tasks, recalculate max iterations now that we know the path
	plc.maxIter = c.phaseMaxIterations(plc.currentPhase, plc.state.WorkflowPath)
	plc.state.MaxPhaseIterations = plc.maxIter
	c.logInfo("COMPLEX workflow: continuing with reviewer/judge (max iterations: %d)", plc.maxIter)
	return false
}

// handleExhaustedIterations handles the case where a phase exhausted all iterations
// without receiving an ADVANCE verdict.
func (c *Controller) handleExhaustedIterations(ctx context.Context, plc *phaseLoopContext) {
	switch plc.currentPhase {
	case PhaseVerify:
		// VERIFY phase exhaustion: PR is already ready for review, note that auto-merge failed
		c.logWarning("Phase %s: exhausted %d iterations, auto-merge failed (PR remains ready for human review)", plc.currentPhase, plc.maxIter)
		c.postPhaseComment(ctx, plc.currentPhase, plc.maxIter, RoleController,
			fmt.Sprintf("Auto-merge failed: exhausted %d iterations. PR is ready for human review.", plc.maxIter))
	default:
		// Set ControllerOverrode flag for NOMERGE handling during PR finalization
		plc.state.ControllerOverrode = true
		c.logWarning("Phase %s: exhausted %d iterations without ADVANCE, forcing advance (NOMERGE flag set)", plc.currentPhase, plc.maxIter)
		c.postPhaseComment(ctx, plc.currentPhase, plc.maxIter, RoleController,
			fmt.Sprintf("Forced advance: exhausted %d iterations without judge ADVANCE (PR will require human review)", plc.maxIter))
	}
	if c.memoryStore != nil {
		c.memoryStore.ClearByType(memory.EvalFeedback)
	}
}
