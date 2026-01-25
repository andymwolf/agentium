package controller

import (
	"context"
	"fmt"

	"github.com/andywolf/agentium/internal/memory"
)

// issuePhaseOrder defines the sequence of phases for issue tasks in the phase loop.
// TEST is merged into IMPLEMENT; REVIEW is skipped for SIMPLE tasks.
var issuePhaseOrder = []TaskPhase{
	PhasePlan,
	PhaseImplement,
	PhaseReview,
	PhaseDocs,
	PhasePRCreation,
}

// Default max iterations per phase when not configured.
const (
	defaultPlanMaxIter      = 3
	defaultImplementMaxIter = 5
	defaultReviewMaxIter    = 3
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
// For SIMPLE tasks, REVIEW is skipped.
func advancePhase(current TaskPhase, isSimple bool) TaskPhase {
	for i, p := range issuePhaseOrder {
		if p == current {
			if i+1 < len(issuePhaseOrder) {
				next := issuePhaseOrder[i+1]
				// Skip REVIEW for SIMPLE tasks
				if next == PhaseReview && isSimple {
					if i+2 < len(issuePhaseOrder) {
						return issuePhaseOrder[i+2]
					}
					return PhaseComplete
				}
				return next
			}
			return PhaseComplete
		}
	}
	return PhaseComplete
}

// maxRegressionCount is the maximum number of REGRESS events allowed before
// forcing the task to BLOCKED to prevent infinite regression loops.
const maxRegressionCount = 3

// runPhaseLoop executes the controller-as-judge phase loop for the active issue task.
// It iterates through phases, running the agent and judge at each step.
func (c *Controller) runPhaseLoop(ctx context.Context) error {
	taskID := fmt.Sprintf("issue:%s", c.activeTask)
	state := c.taskStates[taskID]
	if state == nil {
		return fmt.Errorf("no task state for %s", taskID)
	}

	c.logInfo("Starting phase loop for issue #%s (initial phase: %s)", c.activeTask, state.Phase)

phaseLoop:
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

			// Run reviewer + judge
			reviewResult, reviewErr := c.runReviewer(ctx, reviewRunParams{
				CompletedPhase: currentPhase,
				PhaseOutput:    phaseOutput,
				Iteration:      iter,
				MaxIterations:  maxIter,
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

			// Assess complexity during PLAN phase
			assessComplexity := currentPhase == PhasePlan && !state.ReviewDecided

			judgeResult, err := c.runJudge(ctx, judgeRunParams{
				CompletedPhase:   currentPhase,
				PhaseOutput:      phaseOutput,
				ReviewFeedback:   reviewResult.Feedback,
				Iteration:        iter,
				MaxIterations:    maxIter,
				AssessComplexity: assessComplexity,
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

			case VerdictSimple:
				// Mark task as simple (skip REVIEW phase)
				state.IsSimple = true
				state.ReviewDecided = true
				c.logInfo("Phase %s: judge marked task as SIMPLE (REVIEW will be skipped)", currentPhase)
				// Clear feedback and advance
				if c.memoryStore != nil {
					c.memoryStore.ClearByType(memory.EvalFeedback)
				}
				advanced = true

			case VerdictComplex:
				// Mark task as complex (include REVIEW phase)
				state.IsSimple = false
				state.ReviewDecided = true
				c.logInfo("Phase %s: judge marked task as COMPLEX (REVIEW will be used)", currentPhase)
				// Clear feedback and advance
				if c.memoryStore != nil {
					c.memoryStore.ClearByType(memory.EvalFeedback)
				}
				advanced = true

			case VerdictRegress:
				// Return to PLAN phase (only valid during REVIEW)
				if currentPhase != PhaseReview {
					c.logWarning("Phase %s: REGRESS verdict only valid during REVIEW, treating as ITERATE", currentPhase)
					continue
				}
				state.RegressionCount++

				// Guard against infinite regression loops
				if state.RegressionCount > maxRegressionCount {
					c.logWarning("Phase %s: max regression count (%d) exceeded, marking as BLOCKED",
						currentPhase, maxRegressionCount)
					state.Phase = PhaseBlocked
					state.LastJudgeFeedback = fmt.Sprintf("Exceeded max regression count (%d)", maxRegressionCount)
					return nil
				}

				c.logInfo("Phase %s: judge requested regression to PLAN (count: %d/%d, feedback: %s)",
					currentPhase, state.RegressionCount, maxRegressionCount, judgeResult.Feedback)

				// Keep review feedback in memory for context
				if c.memoryStore != nil && judgeResult.Feedback != "" {
					c.memoryStore.Update([]memory.Signal{
						{Type: memory.EvalFeedback, Content: fmt.Sprintf("REVIEW regression: %s", judgeResult.Feedback)},
					}, c.iteration, taskID)
				}

				// Reset to PLAN phase with fresh iterations
				state.Phase = PhasePlan
				state.PhaseIteration = 0
				// Don't clear IsSimple - let the new PLAN judge reassess
				state.ReviewDecided = false
				continue phaseLoop // Restart the outer loop without recursion
			}

			// Apply ReviewMode if set (complexity assessment during PLAN phase)
			// This happens in addition to the verdict, e.g., ADVANCE + REVIEW_MODE: SIMPLE
			if judgeResult.ReviewMode != "" && !state.ReviewDecided {
				switch judgeResult.ReviewMode {
				case "SIMPLE":
					state.IsSimple = true
					state.ReviewDecided = true
					c.logInfo("Phase %s: complexity assessment = SIMPLE (REVIEW will be skipped)", currentPhase)
				case "FULL":
					state.IsSimple = false
					state.ReviewDecided = true
					c.logInfo("Phase %s: complexity assessment = FULL (REVIEW will be used)", currentPhase)
				}
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

		// Move to next phase (respecting SIMPLE/COMPLEX path)
		nextPhase := advancePhase(currentPhase, state.IsSimple)
		c.logInfo("Phase loop: advancing from %s to %s (simple=%v)", currentPhase, nextPhase, state.IsSimple)
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
