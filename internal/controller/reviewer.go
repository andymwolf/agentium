package controller

import (
	"context"
	"fmt"
	"regexp"
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
	CompletedPhase          TaskPhase
	PhaseOutput             string
	Iteration               int
	MaxIterations           int
	PreviousFeedback        string // Feedback from iteration N-1 (for comparison)
	WorkerHandoffSummary    string // What worker claims to have done this iteration
	WorkerFeedbackResponses string // Worker's FEEDBACK_RESPONSE signals from current iteration
	ParentBranch            string // Parent branch for dependency chains (diff base instead of main)
}

// runReviewer runs a reviewer agent against the completed phase output.
// The reviewer gets fresh context (no memory injection) and provides
// constructive feedback without deciding the verdict.
func (c *Controller) runReviewer(ctx context.Context, params reviewRunParams) (ReviewResult, error) {
	c.logInfo("Starting reviewer for phase %s (iteration %d/%d)...", params.CompletedPhase, params.Iteration, params.MaxIterations)

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
			} else {
				c.logWarning("Reviewer: configured adapter %q not found, using default %q",
					modelCfg.Adapter, c.agent.Name())
			}
		}
		if modelCfg.Model != "" {
			if session.IterationContext == nil {
				session.IterationContext = &agent.IterationContext{}
			}
			session.IterationContext.ModelOverride = modelCfg.Model
		}
		if modelCfg.Reasoning != "" {
			if session.IterationContext == nil {
				session.IterationContext = &agent.IterationContext{}
			}
			session.IterationContext.ReasoningOverride = modelCfg.Reasoning
		}
	}

	env := activeAgent.BuildEnv(session, 0)
	command := activeAgent.BuildCommand(session, 0)

	// Check if agent supports stdin-based prompt delivery
	stdinPrompt := ""
	if provider, ok := activeAgent.(agent.StdinPromptProvider); ok {
		stdinPrompt = provider.GetStdinPrompt(session, 0)
	}

	modelName := ""
	if session.IterationContext != nil && session.IterationContext.ModelOverride != "" {
		modelName = session.IterationContext.ModelOverride
	}
	c.logInfo("Running reviewer for phase %s (iteration %d/%d): adapter=%s model=%s",
		params.CompletedPhase, params.Iteration, params.MaxIterations, activeAgent.Name(), modelName)

	result, err := c.runAgentContainer(ctx, containerRunParams{
		Agent:       activeAgent,
		Session:     session,
		Env:         env,
		Command:     command,
		LogTag:      "Reviewer",
		StdinPrompt: stdinPrompt,
	})
	if err != nil {
		c.logError("Reviewer container failed for phase %s: %v", params.CompletedPhase, err)
		return ReviewResult{}, fmt.Errorf("reviewer failed: %w", err)
	}

	// Prefer AssistantText (excludes tool results like diffs/file contents),
	// falling back to RawTextContent for adapters that don't populate it.
	feedback := result.AssistantText
	if feedback == "" {
		feedback = result.RawTextContent
	}
	if feedback == "" {
		// Log warning - this indicates a parsing issue that should be investigated
		c.logWarning("Reviewer for phase %s produced no text content (parsed summary: %s)",
			params.CompletedPhase, truncateString(result.Summary, 200))
		// Use a descriptive fallback instead of misleading PR/summary content
		if result.Success {
			feedback = "Review completed but no feedback text was captured. Check agent logs for details."
		} else {
			feedback = fmt.Sprintf("Review failed: %s", result.Error)
		}
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

	// Include worker's feedback responses or fall back to raw previous feedback
	if params.Iteration > 1 && params.WorkerFeedbackResponses != "" {
		sb.WriteString("## Worker's Response to Previous Feedback\n\n")
		sb.WriteString("The worker provided the following responses to previous feedback points:\n\n")
		sb.WriteString("```\n")
		sb.WriteString(params.WorkerFeedbackResponses)
		sb.WriteString("\n```\n\n")
	} else if params.Iteration > 1 && params.PreviousFeedback != "" {
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
		output = "... (earlier output truncated)\n\n" + output[len(output)-budget:]
	}
	sb.WriteString("```\n")
	sb.WriteString(output)
	sb.WriteString("\n```\n\n")

	sb.WriteString("## Your Task\n\n")
	sb.WriteString("Review the code changes produced in this phase.\n\n")
	sb.WriteString("**IMPORTANT:** Do not rely solely on the phase output log above. The log shows agent activity, not a clean view of the code. You MUST:\n")

	// Use parent branch as diff base when this issue depends on another issue's branch
	diffBase := "main"
	if params.ParentBranch != "" {
		diffBase = params.ParentBranch
	}
	sb.WriteString(fmt.Sprintf("1. Run `git diff %s..HEAD` to see the code changes on this branch\n", diffBase))
	sb.WriteString("2. Open and read key modified files to check surrounding context\n")
	sb.WriteString("3. Verify that the changes match what the worker claims to have done\n\n")

	if params.ParentBranch != "" {
		sb.WriteString(fmt.Sprintf("**DEPENDENCY CONTEXT:** This issue depends on work from branch `%s`. ", params.ParentBranch))
		sb.WriteString(fmt.Sprintf("The diff base is `%s` (not `main`) so you only see changes made for THIS issue. ", params.ParentBranch))
		sb.WriteString("Do NOT flag inherited parent branch changes as scope creep.\n\n")
	}
	sb.WriteString("Provide constructive, actionable review feedback.\n")
	sb.WriteString("Be specific about what to improve and indicate severity (critical/security, functional bug, minor style).\n")
	sb.WriteString("Security issues (data leakage, missing input validation, unguarded nil access) should always be flagged as critical.\n\n")

	sb.WriteString("## Output Format\n\n")
	sb.WriteString("**CRITICAL:** Output ONLY your feedback. Do NOT include:\n")
	sb.WriteString("- Preamble or planning statements (e.g., \"I need to review...\", \"Let me examine...\")\n")
	sb.WriteString("- Descriptions of your process or reasoning\n")
	sb.WriteString("- Issue or PR metadata\n\n")
	sb.WriteString("Start directly with your feedback content. Use concise bullet points or short paragraphs.\n")

	// Add comparison task if we have previous feedback or worker responses
	if params.Iteration > 1 && params.WorkerFeedbackResponses != "" {
		sb.WriteString("\n**Important:** Evaluate the worker's responses to previous feedback.\n")
		sb.WriteString("- For ADDRESSED items: verify the claims against the actual phase output.\n")
		sb.WriteString("- For DECLINED items: assess whether the justification is reasonable.\n")
		sb.WriteString("- For PARTIAL items: note what remains to be done.\n")
		sb.WriteString("- Are there any new issues introduced?\n")
	} else if params.Iteration > 1 && params.PreviousFeedback != "" {
		sb.WriteString("\n**Important:** Compare the current output against the previous iteration's feedback.\n")
		sb.WriteString("- Did the worker address the issues raised previously?\n")
		sb.WriteString("- What feedback items remain unresolved?\n")
		sb.WriteString("- Are there any new issues introduced?\n")
	}

	return sb.String()
}

// feedbackResponsePattern matches AGENTIUM_MEMORY: FEEDBACK_RESPONSE lines.
var feedbackResponsePattern = regexp.MustCompile(`(?m)^AGENTIUM_MEMORY:\s+FEEDBACK_RESPONSE\s+(.+)$`)

// extractFeedbackResponses parses FEEDBACK_RESPONSE signals from worker output.
func extractFeedbackResponses(output string) []string {
	matches := feedbackResponsePattern.FindAllStringSubmatch(output, -1)
	results := make([]string, 0, len(matches))
	for _, m := range matches {
		results = append(results, m[1])
	}
	return results
}

// extractReviewerVerdict parses the reviewer's recommended verdict from its feedback.
// The reviewer emits AGENTIUM_EVAL: ADVANCE|ITERATE|BLOCKED in its output, which is
// normally only consumed by the judge prompt. This function extracts it for the controller
// to detect when the judge overrides the reviewer's recommendation.
func extractReviewerVerdict(feedback string) JudgeVerdict {
	matches := judgePattern.FindStringSubmatch(feedback)
	if len(matches) < 2 {
		return ""
	}
	switch matches[1] {
	case "ITERATE":
		return VerdictIterate
	case "BLOCKED":
		return VerdictBlocked
	case "ADVANCE":
		return VerdictAdvance
	}
	return ""
}

// truncateString truncates a string to maxLen characters, adding "..." if truncated.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
