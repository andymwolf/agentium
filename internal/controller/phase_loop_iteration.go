package controller

import (
	"context"
	"fmt"

	"github.com/andywolf/agentium/internal/handoff"
	"github.com/andywolf/agentium/internal/observability"
)

// handlePlanSkip checks for a pre-existing plan and populates plc with the plan
// content if found, setting plc.skipIteration = true to skip the worker iteration.
func (c *Controller) handlePlanSkip(ctx context.Context, plc *phaseLoopContext, iter int) {
	if !c.shouldSkipPlanIteration(plc.currentPhase, iter) {
		return
	}
	planContent := c.extractExistingPlan()
	c.logInfo("Phase %s: detected pre-existing plan in issue body, skipping agent iteration", plc.currentPhase)
	plc.phaseOutput = planContent
	plc.skipIteration = true
	c.postPhaseComment(ctx, plc.currentPhase, iter, RoleController, "Pre-existing plan detected in issue body (skipped planning agent)")
}

// runWorkerIteration executes one worker agent iteration, recording the generation
// in Langfuse and populating plc with the output. Returns an error if the iteration fails.
func (c *Controller) runWorkerIteration(ctx context.Context, plc *phaseLoopContext, iter int) error {
	c.iteration++
	if c.cloudLogger != nil {
		c.cloudLogger.SetIteration(c.iteration)
	}

	result, err := c.runIteration(ctx)
	if err != nil {
		return fmt.Errorf("phase %s iteration %d failed: %w", plc.currentPhase, iter, err)
	}

	// Record Worker generation in Langfuse
	c.recordGenerationTokens(plc, observability.GenerationInput{
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

	// Full output for internal processing (handoff parsing, judge context)
	plc.phaseOutput = result.RawTextContent
	if plc.phaseOutput == "" {
		plc.phaseOutput = result.Summary
	}

	// Filtered output for GitHub comments (assistant text only, no tool results)
	plc.commentContent = result.AssistantText
	if plc.commentContent == "" {
		plc.commentContent = plc.phaseOutput
	}
	plc.commentContent = StripAgentiumSignals(plc.commentContent)
	plc.commentContent = StripPreamble(plc.commentContent)
	plc.commentContent = SummarizeForComment(plc.commentContent, 250)

	return nil
}

// processWorkerHandoff parses and stores handoff output from the worker, handling
// both regular handoff signals and the plan-skip fallback for issue body plans.
func (c *Controller) processWorkerHandoff(plc *phaseLoopContext, iter int) {
	// Warn if phase output exceeds judge context budget
	if plc.phaseOutput != "" && len(plc.phaseOutput) > c.judgeContextBudget() {
		c.logWarning("Phase %s output (%d chars) exceeds judge context budget (%d chars) — judge/reviewer will see truncated output",
			plc.currentPhase, len(plc.phaseOutput), c.judgeContextBudget())
	}

	// Parse and store handoff output if enabled
	if c.isHandoffEnabled() && plc.phaseOutput != "" {
		if handoffErr := c.processHandoffOutput(plc.taskID, plc.currentPhase, iter, plc.phaseOutput); handoffErr != nil {
			c.logWarning("Failed to process handoff output for phase %s: %v", plc.currentPhase, handoffErr)
		}
	}

	// When plan skip triggers and processHandoffOutput didn't store a PlanOutput
	// (because the issue body doesn't contain AGENTIUM_HANDOFF), create a minimal
	// PlanOutput from the issue body's structured plan sections.
	if plc.skipIteration && c.isHandoffEnabled() {
		hd := c.handoffStore.GetPhaseOutput(plc.taskID, handoff.PhasePlan)
		if hd == nil || hd.PlanOutput == nil {
			planOutput := extractPlanFromIssueBody(plc.phaseOutput)
			if planOutput != nil {
				if storeErr := c.handoffStore.StorePhaseOutput(plc.taskID, handoff.PhasePlan, iter, planOutput); storeErr != nil {
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
}
