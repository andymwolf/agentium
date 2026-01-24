package controller

import (
	"context"
	"fmt"

	"github.com/andywolf/agentium/internal/memory"
)

// issuePhaseOrder defines the sequence of phases for issue tasks in the phase loop.
var issuePhaseOrder = []TaskPhase{
	PhasePlan,
	PhaseImplement,
	PhaseTest,
	PhaseReview,
	PhaseDocs,
	PhasePRCreation,
}

// Default max iterations per phase when not configured.
const (
	defaultPlanMaxIter      = 3
	defaultImplementMaxIter = 5
	defaultTestMaxIter      = 5
	defaultReviewMaxIter    = 3
	defaultDocsMaxIter      = 2
	defaultPRMaxIter        = 1
)

// defaultEvalContextBudget is the default max characters of phase output sent to the evaluator.
const defaultEvalContextBudget = 8000

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
	case PhaseTest:
		if cfg.TestMaxIterations > 0 {
			return cfg.TestMaxIterations
		}
	case PhaseReview:
		if cfg.ReviewMaxIterations > 0 {
			return cfg.ReviewMaxIterations
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
	case PhaseTest:
		return defaultTestMaxIter
	case PhaseReview:
		return defaultReviewMaxIter
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
// It iterates through phases, running the agent and evaluator at each step.
func (c *Controller) runPhaseLoop(ctx context.Context) error {
	taskID := fmt.Sprintf("issue:%s", c.activeTask)
	state := c.taskStates[taskID]
	if state == nil {
		return fmt.Errorf("no task state for %s", taskID)
	}

	c.logInfo("Starting phase loop for issue #%s (initial phase: %s)", c.activeTask, state.Phase)

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

			// Post phase comment
			c.postPhaseComment(ctx, currentPhase, iter, truncateForComment(phaseOutput))

			// Skip evaluator for PR_CREATION (terminal action)
			if currentPhase == PhasePRCreation {
				// Check if PR was actually created
				if result.AgentStatus == "PR_CREATED" || len(result.PRsCreated) > 0 {
					state.Phase = PhaseComplete
					if len(result.PRsCreated) > 0 {
						state.PRNumber = result.PRsCreated[0]
					}
				} else {
					state.Phase = PhaseBlocked
					state.LastEvalFeedback = "PR creation did not produce a PR"
				}
				return nil
			}

			// Run evaluator (reviewer+judge when review_enabled, or legacy single-evaluator)
			var evalResult EvalResult
			if c.reviewEnabled() {
				// Three-agent loop: reviewer then judge
				reviewResult, reviewErr := c.runReviewer(ctx, reviewRunParams{
					CompletedPhase: currentPhase,
					PhaseOutput:    phaseOutput,
					Iteration:      iter,
					MaxIterations:  maxIter,
				})
				if reviewErr != nil {
					c.logWarning("Reviewer error for phase %s: %v (falling back to legacy evaluator)", currentPhase, reviewErr)
					evalResult, err = c.runEvaluator(ctx, currentPhase, phaseOutput)
					if err != nil {
						c.logWarning("Evaluator error for phase %s: %v (defaulting to ADVANCE)", currentPhase, err)
						evalResult = EvalResult{Verdict: VerdictAdvance}
					}
				} else {
					evalResult, err = c.runJudge(ctx, judgeRunParams{
						CompletedPhase: currentPhase,
						PhaseOutput:    phaseOutput,
						ReviewFeedback: reviewResult.Feedback,
						Iteration:      iter,
						MaxIterations:  maxIter,
					})
					if err != nil {
						c.logWarning("Judge error for phase %s: %v (defaulting to ADVANCE)", currentPhase, err)
						evalResult = EvalResult{Verdict: VerdictAdvance}
					}

					// Track consecutive no-signal count for fail-closed behavior
					if !evalResult.SignalFound {
						noSignalCount++
						c.logWarning("Judge did not emit signal for phase %s (no-signal count: %d/%d)", currentPhase, noSignalCount, c.evalNoSignalLimit())
						if noSignalCount >= c.evalNoSignalLimit() {
							c.logWarning("Phase %s: no-signal limit reached, force-advancing", currentPhase)
							evalResult = EvalResult{Verdict: VerdictAdvance, SignalFound: false}
						}
					} else {
						noSignalCount = 0
					}
				}
			} else {
				// Legacy two-agent loop: direct evaluator
				evalResult, err = c.runEvaluator(ctx, currentPhase, phaseOutput)
				if err != nil {
					c.logWarning("Evaluator error for phase %s: %v (defaulting to ADVANCE)", currentPhase, err)
					evalResult = EvalResult{Verdict: VerdictAdvance}
				}
			}

			state.LastEvalVerdict = string(evalResult.Verdict)
			state.LastEvalFeedback = evalResult.Feedback

			// Post eval comment
			c.postEvalComment(ctx, currentPhase, evalResult)

			switch evalResult.Verdict {
			case VerdictAdvance:
				// Clear eval feedback from memory and move to next phase
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
				// Feedback is already stored in memory by runEvaluator
				c.logInfo("Phase %s: evaluator requested iteration (feedback: %s)", currentPhase, evalResult.Feedback)
				continue

			case VerdictBlocked:
				state.Phase = PhaseBlocked
				c.logInfo("Phase %s: evaluator returned BLOCKED: %s", currentPhase, evalResult.Feedback)
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
				fmt.Sprintf("Forced advance: exhausted %d iterations without evaluator ADVANCE", maxIter))
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

// defaultEvalNoSignalLimit is the default max consecutive no-signal evaluations
// before force-advancing.
const defaultEvalNoSignalLimit = 2

// reviewEnabled returns true if the review loop (reviewer+judge) is enabled.
func (c *Controller) reviewEnabled() bool {
	return c.config.PhaseLoop != nil && c.config.PhaseLoop.ReviewEnabled
}

// evalNoSignalLimit returns the configured max consecutive no-signal evaluations,
// falling back to the default when not specified.
func (c *Controller) evalNoSignalLimit() int {
	if c.config.PhaseLoop != nil && c.config.PhaseLoop.EvalNoSignalLimit > 0 {
		return c.config.PhaseLoop.EvalNoSignalLimit
	}
	return defaultEvalNoSignalLimit
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
