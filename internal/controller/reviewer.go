package controller

import (
	"context"
	"fmt"
	"strings"

	"github.com/andywolf/agentium/internal/agent"
)

// ReviewResult holds the raw feedback from a reviewer agent.
type ReviewResult struct {
	Feedback string
	Error    error
}

// reviewRunParams holds parameters for running a reviewer agent.
type reviewRunParams struct {
	CompletedPhase       TaskPhase
	PhaseOutput          string
	Iteration            int
	MaxIterations        int
	PreviousFeedback     string // Feedback from iteration N-1 (for comparison)
	WorkerHandoffSummary string // What worker claims to have done this iteration
}

// runReviewer runs a reviewer agent against the completed phase output.
// The reviewer gets fresh context (no memory injection) and provides
// constructive feedback without deciding the verdict.
func (c *Controller) runReviewer(ctx context.Context, params reviewRunParams) (ReviewResult, error) {
	reviewPrompt := c.buildReviewPrompt(params)

	session := &agent.Session{
		ID:             c.config.ID,
		Repository:     c.config.Repository,
		Tasks:          c.config.Tasks,
		WorkDir:        c.workDir,
		GitHubToken:    c.gitHubToken,
		MaxIterations:  1,
		MaxDuration:    c.config.MaxDuration,
		Prompt:         reviewPrompt,
		Metadata:       make(map[string]string),
		ClaudeAuthMode: c.config.ClaudeAuth.AuthMode,
		SystemPrompt:   c.systemPrompt,
		ActiveTask:     c.activeTask,
	}

	// Resolve phase key: <PHASE>_REVIEW → REVIEW → default
	reviewPhase := fmt.Sprintf("%s_REVIEW", params.CompletedPhase)
	skillPhase := reviewPhase

	if c.skillSelector != nil {
		session.IterationContext = &agent.IterationContext{
			Phase:        skillPhase,
			SkillsPrompt: c.skillSelector.SelectForPhase(skillPhase),
		}
	}

	// Select adapter via compound key fallback chain
	activeAgent := c.agent
	if c.modelRouter != nil && c.modelRouter.IsConfigured() {
		modelCfg := c.modelRouter.ModelForPhase(reviewPhase)
		// Fallback to REVIEW if no specific override
		if modelCfg.Adapter == "" && modelCfg.Model == "" {
			modelCfg = c.modelRouter.ModelForPhase("REVIEW")
		}
		if modelCfg.Adapter != "" {
			if a, ok := c.adapters[modelCfg.Adapter]; ok {
				activeAgent = a
			}
		}
		if modelCfg.Model != "" {
			if session.IterationContext == nil {
				session.IterationContext = &agent.IterationContext{}
			}
			session.IterationContext.ModelOverride = modelCfg.Model
		}
	}

	env := activeAgent.BuildEnv(session, 0)
	command := activeAgent.BuildCommand(session, 0)

	// Check if agent supports stdin-based prompt delivery
	stdinPrompt := ""
	if provider, ok := activeAgent.(agent.StdinPromptProvider); ok {
		stdinPrompt = provider.GetStdinPrompt(session, 0)
	}

	c.logInfo("Running reviewer for phase %s (iteration %d/%d)", params.CompletedPhase, params.Iteration, params.MaxIterations)

	result, err := c.runAgentContainer(ctx, containerRunParams{
		Agent:       activeAgent,
		Session:     session,
		Env:         env,
		Command:     command,
		LogTag:      "Reviewer",
		StdinPrompt: stdinPrompt,
	})
	if err != nil {
		return ReviewResult{}, fmt.Errorf("reviewer failed: %w", err)
	}

	feedback := result.RawTextContent
	if feedback == "" {
		feedback = result.Summary
	}

	c.logInfo("Reviewer completed for phase %s", params.CompletedPhase)

	return ReviewResult{Feedback: feedback}, nil
}

// buildReviewPrompt composes the reviewer prompt with phase context.
// The reviewer only sees the phase output — no memory from previous iterations.
func (c *Controller) buildReviewPrompt(params reviewRunParams) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("You are reviewing the output of the **%s** phase (iteration %d/%d).\n\n",
		params.CompletedPhase, params.Iteration, params.MaxIterations))
	sb.WriteString(fmt.Sprintf("Repository: %s\n", c.config.Repository))
	sb.WriteString(fmt.Sprintf("Issue: #%s\n\n", c.activeTask))

	// Include previous iteration feedback for comparison (if iteration > 1)
	if params.Iteration > 1 && params.PreviousFeedback != "" {
		sb.WriteString("## Previous Iteration Feedback\n\n")
		sb.WriteString("The following feedback was given in the previous iteration:\n\n")
		sb.WriteString("```\n")
		sb.WriteString(params.PreviousFeedback)
		sb.WriteString("\n```\n\n")
	}

	// Include worker handoff summary if available
	if params.WorkerHandoffSummary != "" {
		sb.WriteString("## Worker's Claimed Actions\n\n")
		sb.WriteString("The worker claims to have done the following this iteration:\n\n")
		sb.WriteString("```\n")
		sb.WriteString(params.WorkerHandoffSummary)
		sb.WriteString("\n```\n\n")
	}

	sb.WriteString("## Phase Output\n\n")
	budget := c.judgeContextBudget()
	output := params.PhaseOutput
	if len(output) > budget {
		output = output[:budget] + "\n\n... (output truncated)"
	}
	sb.WriteString("```\n")
	sb.WriteString(output)
	sb.WriteString("\n```\n\n")

	sb.WriteString("## Your Task\n\n")
	sb.WriteString("Provide constructive, actionable review feedback on the phase output above.\n")
	sb.WriteString("Be specific about what to improve. Do NOT emit any verdict — your role is to provide feedback only.\n")

	// Add comparison task if we have previous feedback
	if params.Iteration > 1 && params.PreviousFeedback != "" {
		sb.WriteString("\n**Important:** Compare the current output against the previous iteration's feedback.\n")
		sb.WriteString("- Did the worker address the issues raised previously?\n")
		sb.WriteString("- What feedback items remain unresolved?\n")
		sb.WriteString("- Are there any new issues introduced?\n")
	}

	return sb.String()
}
