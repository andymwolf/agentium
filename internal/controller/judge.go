package controller

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/andywolf/agentium/internal/agent"
	"github.com/andywolf/agentium/internal/memory"
)

// JudgeVerdict represents the outcome of a judge decision.
type JudgeVerdict string

const (
	VerdictAdvance JudgeVerdict = "ADVANCE"
	VerdictIterate JudgeVerdict = "ITERATE"
	VerdictBlocked JudgeVerdict = "BLOCKED"
	VerdictSimple  JudgeVerdict = "SIMPLE"  // Task is simple, skip reviewers
	VerdictComplex JudgeVerdict = "COMPLEX" // Task is complex, use reviewers
	VerdictRegress JudgeVerdict = "REGRESS" // Return to PLAN phase
)

// JudgeResult holds the parsed judge verdict and feedback.
type JudgeResult struct {
	Verdict     JudgeVerdict
	Feedback    string
	SignalFound bool   // Whether the AGENTIUM_EVAL signal was found in output
	ReviewMode  string // "FULL", "SIMPLE", or "" (only set when AssessComplexity is true)
}

// judgeRunParams holds parameters for running a judge agent.
type judgeRunParams struct {
	CompletedPhase   TaskPhase
	PhaseOutput      string
	ReviewFeedback   string
	Iteration        int
	MaxIterations    int
	AssessComplexity bool // When true, judge also emits AGENTIUM_REVIEW_MODE signal
}

// judgePattern matches lines of the form: AGENTIUM_EVAL: VERDICT [optional feedback]
var judgePattern = regexp.MustCompile(`(?m)^AGENTIUM_EVAL:[ \t]+(ADVANCE|ITERATE|BLOCKED|SIMPLE|COMPLEX|REGRESS)[ \t]*(.*)$`)

// parseJudgeVerdict extracts the judge verdict from agent output.
// If no verdict line is found, defaults to ITERATE (fail-closed).
func parseJudgeVerdict(output string) JudgeResult {
	matches := judgePattern.FindStringSubmatch(output)
	if matches == nil {
		return JudgeResult{Verdict: VerdictIterate, SignalFound: false}
	}
	return JudgeResult{
		Verdict:     JudgeVerdict(matches[1]),
		Feedback:    strings.TrimSpace(matches[2]),
		SignalFound: true,
	}
}

// reviewModePattern matches lines of the form: AGENTIUM_REVIEW_MODE: FULL|SIMPLE
var reviewModePattern = regexp.MustCompile(`(?m)^AGENTIUM_REVIEW_MODE:[ \t]+(FULL|SIMPLE)[ \t]*$`)

// parseReviewModeSignal extracts the review mode decision from judge output.
// Returns "FULL", "SIMPLE", or "" if no signal found.
func parseReviewModeSignal(output string) string {
	matches := reviewModePattern.FindStringSubmatch(output)
	if matches == nil {
		return ""
	}
	return matches[1]
}

// runJudge runs a judge agent that interprets reviewer feedback and decides
// whether to ADVANCE, ITERATE, BLOCKED, SIMPLE, COMPLEX, or REGRESS.
func (c *Controller) runJudge(ctx context.Context, params judgeRunParams) (JudgeResult, error) {
	judgePrompt := c.buildJudgePrompt(params)

	session := &agent.Session{
		ID:             c.config.ID,
		Repository:     c.config.Repository,
		Tasks:          c.config.Tasks,
		WorkDir:        c.workDir,
		GitHubToken:    c.gitHubToken,
		MaxIterations:  1,
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

	if c.skillSelector != nil {
		session.IterationContext = &agent.IterationContext{
			Phase:        skillPhase,
			SkillsPrompt: c.skillSelector.SelectForPhase(skillPhase),
		}
	}

	// Inject eval memory context for iteration awareness
	if c.memoryStore != nil {
		// Build context scoped to the current task
		taskID := fmt.Sprintf("%s:%s", c.activeTaskType, c.activeTask)
		evalCtx := c.memoryStore.BuildEvalContext(taskID)
		if evalCtx != "" {
			if session.IterationContext == nil {
				session.IterationContext = &agent.IterationContext{}
			}
			session.IterationContext.MemoryContext = evalCtx
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

	c.logInfo("Running judge for phase %s (iteration %d/%d)", params.CompletedPhase, params.Iteration, params.MaxIterations)

	result, err := c.runAgentContainer(ctx, containerRunParams{
		Agent:       activeAgent,
		Session:     session,
		Env:         env,
		Command:     command,
		LogTag:      "Judge",
		StdinPrompt: stdinPrompt,
	})
	if err != nil {
		return JudgeResult{Verdict: VerdictAdvance}, fmt.Errorf("judge failed: %w", err)
	}

	parseSource := result.RawTextContent
	if parseSource == "" {
		parseSource = result.Summary
	}
	judgeResult := parseJudgeVerdict(parseSource)
	c.logInfo("Judge verdict for phase %s: %s (signal_found=%v)", params.CompletedPhase, judgeResult.Verdict, judgeResult.SignalFound)

	// Parse review mode signal when complexity assessment was requested
	if params.AssessComplexity {
		judgeResult.ReviewMode = parseReviewModeSignal(parseSource)
	}

	// On ITERATE or REGRESS, store the reviewer's feedback (not the judge's) in memory for the worker
	if (judgeResult.Verdict == VerdictIterate || judgeResult.Verdict == VerdictRegress) && params.ReviewFeedback != "" && c.memoryStore != nil {
		c.memoryStore.Update([]memory.Signal{
			{Type: memory.EvalFeedback, Content: params.ReviewFeedback},
		}, c.iteration, fmt.Sprintf("issue:%s", c.activeTask))
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
		output = output[:budget] + "\n\n... (output truncated)"
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

	// Only show REGRESS option during REVIEW phase
	if params.CompletedPhase == PhaseReview {
		sb.WriteString("- `AGENTIUM_EVAL: REGRESS <reason>` - Significant issues found, return to PLAN phase\n")
	}

	sb.WriteString("\n")

	if params.Iteration >= params.MaxIterations {
		sb.WriteString("**NOTE:** This is the FINAL iteration. Prefer ADVANCE unless there are critical issues that would prevent the work from being usable.\n\n")
	}

	if params.AssessComplexity {
		sb.WriteString("## Complexity Assessment\n\n")
		sb.WriteString("In addition to your verdict, assess whether this task is complex enough\n")
		sb.WriteString("to warrant detailed review in subsequent phases.\n\n")
		sb.WriteString("Emit exactly one line:\n")
		sb.WriteString("  AGENTIUM_REVIEW_MODE: FULL\n")
		sb.WriteString("or\n")
		sb.WriteString("  AGENTIUM_REVIEW_MODE: SIMPLE\n\n")
		sb.WriteString("Use FULL when: multiple files, architectural changes, complex logic,\n")
		sb.WriteString("significant new functionality, or non-trivial testing requirements.\n")
		sb.WriteString("Use SIMPLE when: single-file changes, straightforward fixes, config\n")
		sb.WriteString("updates, documentation-only, or well-scoped small features.\n\n")
	}

	return sb.String()
}
