package controller

import (
	"context"
	"fmt"
	"strings"

	"github.com/andywolf/agentium/internal/handoff"
	"github.com/andywolf/agentium/internal/memory"
	"github.com/andywolf/agentium/internal/observability"
)

// gatherFeedbackContext collects previous iteration feedback, handoff summary,
// and worker feedback responses for the reviewer/judge pipeline.
func (c *Controller) gatherFeedbackContext(plc *phaseLoopContext, iter int) (prevFeedback, handoffSummary, feedbackResponses string) {
	// Gather previous iteration feedback for reviewer context
	if iter > 1 && c.memoryStore != nil {
		prevEntries := c.memoryStore.GetPreviousIterationFeedback(plc.taskID, iter)
		if len(prevEntries) > 0 {
			var feedbackParts []string
			for _, e := range prevEntries {
				feedbackParts = append(feedbackParts, e.Content)
			}
			prevFeedback = strings.Join(feedbackParts, "\n")
		}
	}

	// Get worker handoff summary if available
	handoffSummary = c.buildWorkerHandoffSummary(plc.taskID, plc.currentPhase, iter)

	// Extract worker's feedback responses from phase output (iteration > 1 only)
	if iter > 1 && plc.phaseOutput != "" {
		responses := extractFeedbackResponses(plc.phaseOutput)
		if len(responses) > 0 {
			feedbackResponses = strings.Join(responses, "\n")
		}
	}
	return
}

// handleReviewerSkip checks if the reviewer should be skipped and auto-advances if so.
// Returns true if skipped (and plc.advanced is set).
func (c *Controller) handleReviewerSkip(ctx context.Context, plc *phaseLoopContext, iter int) bool {
	skipReviewer, skipReason := c.shouldSkipReviewer(plc.phaseOutput, plc.taskID)
	if !skipReviewer {
		return false
	}
	c.logInfo("Phase %s: reviewer skip condition met (%s), skipping review/judge (auto-advance)", plc.currentPhase, skipReason)
	c.tracer.RecordSkipped(plc.activeSpanCtx, "Reviewer", skipReason)
	c.tracer.RecordSkipped(plc.activeSpanCtx, "Judge", "reviewer_skipped")
	c.postPhaseComment(ctx, plc.currentPhase, iter, RoleController,
		fmt.Sprintf("Reviewer skip condition (%s) met — skipping review (auto-advance)", skipReason))
	c.recordPhaseAdvance(plc, fmt.Sprintf("%s completed (skip condition met, auto-advanced)", plc.currentPhase))
	plc.advanced = true
	return true
}

// handleJudgeSkip checks if the judge should be skipped and auto-advances if so.
// Returns true if skipped (and plc.advanced is set).
func (c *Controller) handleJudgeSkip(ctx context.Context, plc *phaseLoopContext, iter int) bool {
	skipJudge, skipReason := c.shouldSkipJudge(plc.phaseOutput, plc.taskID)
	if !skipJudge {
		return false
	}
	c.logInfo("Phase %s: judge skip condition met (%s), skipping judge (auto-advance)", plc.currentPhase, skipReason)
	c.tracer.RecordSkipped(plc.activeSpanCtx, "Judge", skipReason)
	c.postPhaseComment(ctx, plc.currentPhase, iter, RoleController,
		fmt.Sprintf("Judge skip condition (%s) met — skipping judge (auto-advance)", skipReason))
	c.recordPhaseAdvance(plc, fmt.Sprintf("%s completed (judge skip condition met, auto-advanced)", plc.currentPhase))
	plc.advanced = true
	return true
}

// applyJudgePostProcessing handles no-signal tracking, the PLAN hard-gate,
// and judge-overrides-reviewer detection. It may mutate judgeResult.
func (c *Controller) applyJudgePostProcessing(plc *phaseLoopContext, judgeResult *JudgeResult, reviewResult ReviewResult) {
	// Track consecutive no-signal count for fail-closed behavior
	if !judgeResult.SignalFound {
		plc.noSignalCount++
		c.logWarning("Judge did not emit signal for phase %s (no-signal count: %d/%d)", plc.currentPhase, plc.noSignalCount, c.judgeNoSignalLimit())
		if plc.noSignalCount >= c.judgeNoSignalLimit() {
			c.logWarning("Phase %s: no-signal limit reached, force-advancing", plc.currentPhase)
			*judgeResult = JudgeResult{Verdict: VerdictAdvance, SignalFound: false}
		}
	} else {
		plc.noSignalCount = 0
	}

	plc.state.LastJudgeVerdict = string(judgeResult.Verdict)
	plc.state.LastJudgeFeedback = judgeResult.Feedback

	// Hard-gate: PLAN phase cannot advance without a valid PlanOutput in the handoff store
	if plc.currentPhase == PhasePlan && judgeResult.Verdict == VerdictAdvance && c.isHandoffEnabled() {
		hd := c.handoffStore.GetPhaseOutput(plc.taskID, handoff.PhasePlan)
		if hd == nil || hd.PlanOutput == nil {
			c.logWarning("PLAN: judge ADVANCE but no AGENTIUM_HANDOFF signal — forcing ITERATE")
			*judgeResult = JudgeResult{
				Verdict:  VerdictIterate,
				Feedback: "No AGENTIUM_HANDOFF signal detected. You must emit the structured handoff signal with your plan before the PLAN phase can advance.",
				// SignalFound: true so noSignalCount resets — we want the hard-gate
				// to persist across iterations without triggering the no-signal fail-safe.
				SignalFound: true,
			}
			plc.state.LastJudgeVerdict = string(judgeResult.Verdict)
			plc.state.LastJudgeFeedback = judgeResult.Feedback
		}
	}

	// Detect when judge overrides reviewer's recommendation (NOMERGE trigger)
	if judgeResult.Verdict == VerdictAdvance {
		reviewerVerdict := extractReviewerVerdict(reviewResult.Feedback)
		if reviewerVerdict == VerdictIterate || reviewerVerdict == VerdictBlocked {
			plc.state.JudgeOverrodeReviewer = true
			c.logWarning("Phase %s: judge ADVANCE overrode reviewer %s", plc.currentPhase, reviewerVerdict)
		}
	}
}

// handleVerdict processes the judge verdict and returns flow control signals.
func (c *Controller) handleVerdict(ctx context.Context, plc *phaseLoopContext, judgeResult JudgeResult, reviewResult ReviewResult, iter int) (advanced, blocked, shouldContinue bool) {
	switch judgeResult.Verdict {
	case VerdictAdvance:
		c.recordPhaseAdvance(plc, fmt.Sprintf("%s completed (iteration %d)", plc.currentPhase, iter))

		// Post implementation plan as a comment after PLAN phase advances
		if plc.currentPhase == PhasePlan && plc.phaseOutput != "" {
			c.postImplementationPlan(ctx, c.formatPlanForComment(plc.taskID, plc.phaseOutput))
		}

		plc.advanced = true
		return true, false, false

	case VerdictIterate:
		// Store feedback in memory for the next iteration's worker prompt.
		// This is done HERE (not in runJudge) so that hard-gate overrides
		// (e.g., PLAN phase missing AGENTIUM_HANDOFF) also get their
		// feedback stored correctly.
		if c.memoryStore != nil {
			var signals []memory.Signal
			if reviewResult.Feedback != "" {
				signals = append(signals, memory.Signal{
					Type:    memory.EvalFeedback,
					Content: reviewResult.Feedback,
				})
			}
			if judgeResult.Feedback != "" {
				signals = append(signals, memory.Signal{
					Type:    memory.JudgeDirective,
					Content: judgeResult.Feedback,
				})
			}
			if len(signals) > 0 {
				c.memoryStore.UpdateWithPhaseIteration(signals, c.iteration, iter, plc.taskID)
			}
		}
		c.logInfo("Phase %s: judge requested iteration (feedback: %s)", plc.currentPhase, judgeResult.Feedback)
		return false, false, true

	case VerdictBlocked:
		plc.state.Phase = PhaseBlocked
		c.logInfo("Phase %s: judge returned BLOCKED: %s", plc.currentPhase, judgeResult.Feedback)
		c.endPhaseSpan(plc, "blocked")
		plc.traceStatus = "blocked"
		return false, true, false
	}

	return false, false, false
}

// runReviewJudgePipeline runs the reviewer and judge for the current iteration.
func (c *Controller) runReviewJudgePipeline(ctx context.Context, plc *phaseLoopContext, iter int) (advanced, blocked, shouldContinue bool) {
	previousFeedback, workerHandoffSummary, workerFeedbackResponses := c.gatherFeedbackContext(plc, iter)

	// Check if reviewer should be skipped
	if c.handleReviewerSkip(ctx, plc, iter) {
		return true, false, false
	}

	// Run reviewer
	reviewResult, reviewErr := c.runReviewer(ctx, reviewRunParams{
		CompletedPhase:          plc.currentPhase,
		PhaseOutput:             plc.phaseOutput,
		Iteration:               iter,
		MaxIterations:           plc.maxIter,
		PreviousFeedback:        previousFeedback,
		WorkerHandoffSummary:    workerHandoffSummary,
		WorkerFeedbackResponses: workerFeedbackResponses,
		ParentBranch:            plc.state.ParentBranch,
	})
	if reviewErr != nil {
		c.logWarning("Reviewer error for phase %s: %v (defaulting to ADVANCE)", plc.currentPhase, reviewErr)
		plc.state.LastJudgeVerdict = string(VerdictAdvance)
		c.recordPhaseAdvance(plc, fmt.Sprintf("%s completed (reviewer error, forced advance)", plc.currentPhase))
		plc.advanced = true
		return true, false, false
	}

	// Record Reviewer generation in Langfuse
	c.recordGenerationTokens(plc, observability.GenerationInput{
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

	// Store reviewer feedback on TaskState for defense-in-depth fallback
	plc.state.LastReviewerFeedback = reviewResult.Feedback

	// Post reviewer feedback to appropriate location (filtered for readability)
	reviewFeedbackComment := StripAgentiumSignals(reviewResult.Feedback)
	reviewFeedbackComment = StripPreamble(reviewFeedbackComment)
	reviewFeedbackComment = SummarizeForComment(reviewFeedbackComment, 250)
	c.postReviewFeedbackForPhase(ctx, plc.currentPhase, iter, reviewFeedbackComment)

	priorDirectives := ""
	if c.memoryStore != nil && iter > 1 {
		priorDirectives = c.memoryStore.BuildJudgeHistoryContext(plc.taskID, iter)
	}

	// Check if judge should be skipped
	if c.handleJudgeSkip(ctx, plc, iter) {
		return true, false, false
	}

	// Run judge
	judgeResult, err := c.runJudge(ctx, judgeRunParams{
		CompletedPhase:  plc.currentPhase,
		PhaseOutput:     plc.phaseOutput,
		ReviewFeedback:  reviewResult.Feedback,
		Iteration:       iter,
		MaxIterations:   plc.maxIter,
		PhaseIteration:  iter,
		PriorDirectives: priorDirectives,
	})
	if err != nil {
		c.logWarning("Judge error for phase %s: %v (defaulting to ADVANCE)", plc.currentPhase, err)
		judgeResult = JudgeResult{Verdict: VerdictAdvance}
	}

	// Record Judge generation in Langfuse
	c.recordGenerationTokens(plc, observability.GenerationInput{
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

	// Apply post-processing (no-signal tracking, hard-gate, override detection)
	c.applyJudgePostProcessing(plc, &judgeResult, reviewResult)

	// Post judge comment
	c.postJudgeComment(ctx, plc.currentPhase, iter, judgeResult)

	// Handle verdict
	return c.handleVerdict(ctx, plc, judgeResult, reviewResult, iter)
}
