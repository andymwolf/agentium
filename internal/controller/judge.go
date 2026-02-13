package controller

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/andywolf/agentium/internal/agent"
	"github.com/andywolf/agentium/internal/memory"
)

// JudgeVerdict represents the outcome of a judge decision.
type JudgeVerdict string

const (
	VerdictAdvance JudgeVerdict = "ADVANCE"
	VerdictIterate JudgeVerdict = "ITERATE"
	VerdictBlocked JudgeVerdict = "BLOCKED"
)

// JudgeResult holds the parsed judge verdict and feedback.
type JudgeResult struct {
	Verdict      JudgeVerdict
	Feedback     string
	SignalFound  bool      // Whether the AGENTIUM_EVAL signal was found in output
	Prompt       string    // Prompt text sent to the judge (for Langfuse generation input)
	Output       string    // Raw agent output (for Langfuse generation output)
	InputTokens  int       // Input tokens consumed by the judge
	OutputTokens int       // Output tokens consumed by the judge
	StartTime    time.Time // When the judge invocation started
	EndTime      time.Time // When the judge invocation finished
}

// judgeRunParams holds parameters for running a judge agent.
type judgeRunParams struct {
	CompletedPhase  TaskPhase
	PhaseOutput     string
	ReviewFeedback  string
	Iteration       int
	MaxIterations   int
	PhaseIteration  int    // Within-phase iteration (1-indexed) for scoped feedback
	PriorDirectives string // Judge's own prior ITERATE directives for loop detection
}

// judgePattern matches lines of the form: AGENTIUM_EVAL: VERDICT [optional feedback]
var judgePattern = regexp.MustCompile(`(?m)^AGENTIUM_EVAL:[ \t]+(ADVANCE|ITERATE|BLOCKED)[ \t]*(.*)$`)

// markdownFencePattern matches markdown code fences (```...```) with optional language tag
var markdownFencePattern = regexp.MustCompile("(?s)```[a-z]*\\n?(.*?)```")

// stripMarkdownFences removes markdown code fences that may wrap the signal,
// keeping their inner content. This handles cases where the agent wraps the
// verdict in a code block.
func stripMarkdownFences(s string) string {
	return markdownFencePattern.ReplaceAllString(s, "$1")
}

// parseJudgeVerdict extracts the judge verdict from agent output.
// If no verdict line is found, defaults to BLOCKED (fail-closed).
func parseJudgeVerdict(output string) JudgeResult {
	// First try matching the raw output
	matches := judgePattern.FindStringSubmatch(output)
	if matches == nil {
		// Fallback: strip markdown fences and try again
		cleaned := stripMarkdownFences(output)
		matches = judgePattern.FindStringSubmatch(cleaned)
	}
	if matches == nil {
		return JudgeResult{Verdict: VerdictBlocked, SignalFound: false}
	}
	return JudgeResult{
		Verdict:     JudgeVerdict(matches[1]),
		Feedback:    strings.TrimSpace(matches[2]),
		SignalFound: true,
	}
}

// runJudge runs a judge agent that interprets reviewer feedback and decides
// whether to ADVANCE, ITERATE, or BLOCKED.
func (c *Controller) runJudge(ctx context.Context, params judgeRunParams) (JudgeResult, error) {
	c.logInfo("Starting judge for phase %s (iteration %d/%d)...", params.CompletedPhase, params.Iteration, params.MaxIterations)

	judgePrompt := c.buildJudgePrompt(params)

	session := &agent.Session{
		ID:             c.config.ID,
		Repository:     c.config.Repository,
		Tasks:          c.config.Tasks,
		WorkDir:        c.workDir,
		GitHubToken:    c.gitHubToken,
		MaxDuration:    c.config.MaxDuration,
		Prompt:         judgePrompt,
		Metadata:       make(map[string]string),
		ClaudeAuthMode: c.config.ClaudeAuth.AuthMode,
		SystemPrompt:   c.systemPrompt,
		ActiveTask:     c.activeTask,
	}

	// Resolve phase key: <PHASE>_JUDGE → JUDGE → default
	judgePhase := fmt.Sprintf("%s_JUDGE", params.CompletedPhase)
	skillPhase := judgePhase

	// API-provided judge criteria takes precedence over built-in skills
	if judgeCriteria := c.phaseJudgeCriteria(params.CompletedPhase); judgeCriteria != "" {
		session.IterationContext = &agent.IterationContext{
			Phase:        skillPhase,
			SkillsPrompt: judgeCriteria,
		}
		c.logInfo("Using API-provided judge criteria for phase %s", params.CompletedPhase)
	} else if c.skillSelector != nil {
		session.IterationContext = &agent.IterationContext{
			Phase:        skillPhase,
			SkillsPrompt: c.skillSelector.SelectForPhase(skillPhase),
		}
	}

	// Select adapter via compound key fallback chain: <PHASE>_JUDGE → JUDGE → default
	activeAgent := c.agent
	if c.modelRouter != nil && c.modelRouter.IsConfigured() {
		modelCfg := c.modelRouter.ModelForPhase(judgePhase)
		// Fallback: JUDGE → default
		if modelCfg.Adapter == "" && modelCfg.Model == "" {
			modelCfg = c.modelRouter.ModelForPhase("JUDGE")
		}
		if modelCfg.Adapter != "" {
			if a, ok := c.adapters[modelCfg.Adapter]; ok {
				activeAgent = a
			} else {
				c.logWarning("Judge: configured adapter %q not found, using default %q",
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
	c.logInfo("Running judge for phase %s (iteration %d/%d): adapter=%s model=%s",
		params.CompletedPhase, params.Iteration, params.MaxIterations, activeAgent.Name(), modelName)

	judgeParams := containerRunParams{
		Agent:       activeAgent,
		Session:     session,
		Env:         env,
		Command:     command,
		LogTag:      "Judge",
		StdinPrompt: stdinPrompt,
	}

	// Use pooled execution if container pool is active
	var result *agent.IterationResult
	var err error
	judgeStart := time.Now()
	if c.containerPool != nil && c.containerPool.IsHealthy(RoleJudgeContainer) {
		c.logInfo("Using pooled execution for Judge")
		result, err = c.runAgentContainerPooled(ctx, RoleJudgeContainer, judgeParams)
	} else {
		result, err = c.runAgentContainer(ctx, judgeParams)
	}
	judgeEnd := time.Now()
	if err != nil {
		c.logError("Judge container failed for phase %s: %v", params.CompletedPhase, err)
		return JudgeResult{Verdict: VerdictAdvance}, fmt.Errorf("judge failed: %w", err)
	}

	parseSource := result.RawTextContent
	if parseSource == "" {
		parseSource = result.Summary
	}
	judgeResult := parseJudgeVerdict(parseSource)
	judgeResult.Prompt = stdinPrompt
	judgeResult.Output = parseSource
	judgeResult.InputTokens = result.InputTokens
	judgeResult.OutputTokens = result.OutputTokens
	judgeResult.StartTime = judgeStart
	judgeResult.EndTime = judgeEnd
	c.logInfo("Judge verdict for phase %s: %s (signal_found=%v)", params.CompletedPhase, judgeResult.Verdict, judgeResult.SignalFound)

	// On ITERATE, store both reviewer feedback and judge directive in memory for the worker.
	// The reviewer feedback provides detailed analysis, while the judge directive contains
	// the required action items. Use phase iteration to scope feedback so judge only sees
	// current iteration's feedback.
	if judgeResult.Verdict == VerdictIterate && c.memoryStore != nil {
		var signals []memory.Signal

		// Store reviewer feedback (detailed analysis context)
		if params.ReviewFeedback != "" {
			signals = append(signals, memory.Signal{
				Type:    memory.EvalFeedback,
				Content: params.ReviewFeedback,
			})
		}

		// Store judge directive (required action items)
		if judgeResult.Feedback != "" {
			signals = append(signals, memory.Signal{
				Type:    memory.JudgeDirective,
				Content: judgeResult.Feedback,
			})
		}

		if len(signals) > 0 {
			c.memoryStore.UpdateWithPhaseIteration(signals, c.iteration, params.PhaseIteration, taskKey("issue", c.activeTask))
		}
	}

	return judgeResult, nil
}

// judgeContextBudget returns the configured max characters for judge context,
// falling back to the default when not specified.
func (c *Controller) judgeContextBudget() int {
	if c.config.PhaseLoop != nil && c.config.PhaseLoop.JudgeContextBudget > 0 {
		return c.config.PhaseLoop.JudgeContextBudget
	}
	return defaultJudgeContextBudget
}

// buildJudgePrompt composes the judge prompt with reviewer feedback and iteration context.
func (c *Controller) buildJudgePrompt(params judgeRunParams) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("You are the **judge** for the **%s** phase.\n\n", params.CompletedPhase))
	sb.WriteString(fmt.Sprintf("Repository: %s\n", c.config.Repository))
	sb.WriteString(fmt.Sprintf("Issue: #%s\n", c.activeTask))
	sb.WriteString(fmt.Sprintf("Iteration: %d/%d\n\n", params.Iteration, params.MaxIterations))

	if params.PriorDirectives != "" {
		sb.WriteString("## Your Prior Directives\n\n")
		sb.WriteString(params.PriorDirectives)
		sb.WriteString("\n")
	}

	sb.WriteString("## Reviewer's Feedback\n\n")
	if params.ReviewFeedback != "" {
		sb.WriteString(params.ReviewFeedback)
	} else {
		sb.WriteString("(No feedback provided by reviewer)")
	}
	sb.WriteString("\n\n")

	sb.WriteString("## Phase Output Summary\n\n")
	budget := c.judgeContextBudget()
	output := params.PhaseOutput
	if len(output) > budget {
		output = "... (earlier output truncated)\n\n" + output[len(output)-budget:]
	}
	sb.WriteString("```\n")
	sb.WriteString(output)
	sb.WriteString("\n```\n\n")

	sb.WriteString("## Your Task\n\n")
	sb.WriteString("Based on the reviewer's feedback, decide if the work should advance or iterate.\n")
	sb.WriteString("You MUST emit exactly one line starting with `AGENTIUM_EVAL:` followed by your verdict.\n\n")

	sb.WriteString("### Available Verdicts\n\n")
	sb.WriteString("- `AGENTIUM_EVAL: ADVANCE` - Phase complete, move to next phase\n")
	sb.WriteString("- `AGENTIUM_EVAL: ITERATE <feedback>` - More work needed in current phase\n")
	sb.WriteString("- `AGENTIUM_EVAL: BLOCKED <reason>` - Unresolvable issue, needs human intervention\n")
	sb.WriteString("\n")

	if params.Iteration >= params.MaxIterations {
		sb.WriteString("**NOTE:** This is the FINAL iteration. Prefer ADVANCE unless there are critical issues that would prevent the work from being usable. However, security issues (data leakage to external services, missing input sanitization) are ALWAYS critical regardless of iteration count.\n\n")
	}

	// Apply template variable substitution
	return c.renderWithParameters(sb.String())
}
