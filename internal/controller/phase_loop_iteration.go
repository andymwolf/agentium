package controller

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/andywolf/agentium/internal/handoff"
	"github.com/andywolf/agentium/internal/observability"
)

// Plan marker constants for extracting rich plan content from worker output.
const (
	planMarkerStart = "AGENTIUM_PLAN_START"
	planMarkerEnd   = "AGENTIUM_PLAN_END"
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

	// Prefer AssistantText (excludes tool results like diffs/file contents),
	// falling back to RawTextContent for adapters that don't populate it.
	workerOutput := result.AssistantText
	if workerOutput == "" {
		workerOutput = result.RawTextContent
	}

	// Record Worker generation in Langfuse
	c.recordGenerationTokens(plc, observability.GenerationInput{
		Name:         "Worker",
		Model:        c.config.Agent,
		Input:        result.PromptInput,
		Output:       workerOutput,
		SystemPrompt: result.SystemPrompt,
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

	// Write plan markdown file when PLAN phase produces marker-delimited plan content.
	// The controller acts as write proxy since PLAN-WORKER runs in read-only mode.
	if plc.currentPhase == PhasePlan {
		if planMD := extractPlanMarkdown(plc.phaseOutput); planMD != "" {
			planPath := filepath.Join(c.workDir, ".agentium", "plan.md")
			if mkdirErr := os.MkdirAll(filepath.Dir(planPath), 0755); mkdirErr != nil {
				c.logWarning("Failed to create .agentium directory for plan file: %v", mkdirErr)
			} else if writeErr := os.WriteFile(planPath, []byte(planMD), 0644); writeErr != nil {
				c.logWarning("Failed to write plan file: %v", writeErr)
			} else {
				c.logInfo("Wrote plan to %s (%d bytes)", planPath, len(planMD))
			}
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

// extractPlanMarkdown extracts content between AGENTIUM_PLAN_START and
// AGENTIUM_PLAN_END markers from worker output. Returns empty string if
// markers are not found or content is empty.
func extractPlanMarkdown(output string) string {
	startIdx := strings.Index(output, planMarkerStart)
	if startIdx == -1 {
		return ""
	}

	// Move past the marker and any trailing whitespace/newline
	contentStart := startIdx + len(planMarkerStart)
	if contentStart < len(output) && output[contentStart] == '\n' {
		contentStart++
	}

	endIdx := strings.Index(output[contentStart:], planMarkerEnd)
	if endIdx == -1 {
		return ""
	}

	content := strings.TrimSpace(output[contentStart : contentStart+endIdx])
	return content
}
