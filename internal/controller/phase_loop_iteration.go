package controller

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/andywolf/agentium/internal/observability"
)

// Plan marker constants for extracting rich plan content from worker output.
const (
	planMarkerStart = "AGENTIUM_PLAN_START"
	planMarkerEnd   = "AGENTIUM_PLAN_END"
)

// Plan file path constants.
const (
	planFileDir    = ".agentium"
	planFilePrefix = "plan-"
	planFileExt    = ".md"
)

// PlanFilePath returns the issue-scoped plan file path, e.g. ".agentium/plan-42.md".
func PlanFilePath(issueNumber string) string {
	return filepath.Join(planFileDir, planFilePrefix+issueNumber+planFileExt)
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

// processWorkerHandoff parses and stores handoff output from the worker and
// writes the plan file for the PLAN phase. Returns an error if the plan file
// write fails — callers should treat this as a fatal condition (BLOCKED).
func (c *Controller) processWorkerHandoff(plc *phaseLoopContext, iter int) error {
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
			planPath := filepath.Join(c.workDir, PlanFilePath(c.activeTask))
			if err := os.MkdirAll(filepath.Dir(planPath), 0755); err != nil {
				return fmt.Errorf("create plan directory: %w", err)
			}
			if err := os.WriteFile(planPath, []byte(planMD), 0644); err != nil {
				return fmt.Errorf("write plan file %s: %w", planPath, err)
			}
			c.logInfo("Wrote plan to %s (%d bytes)", planPath, len(planMD))
		}
	}

	return nil
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
