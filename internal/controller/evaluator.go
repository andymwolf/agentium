package controller

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/andywolf/agentium/internal/agent"
	"github.com/andywolf/agentium/internal/memory"
)

// EvalVerdict represents the outcome of an evaluator judgment.
type EvalVerdict string

const (
	VerdictAdvance EvalVerdict = "ADVANCE"
	VerdictIterate EvalVerdict = "ITERATE"
	VerdictBlocked EvalVerdict = "BLOCKED"
)

// EvalResult holds the parsed evaluator verdict and feedback.
type EvalResult struct {
	Verdict     EvalVerdict
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

// evalPattern matches lines of the form: AGENTIUM_EVAL: VERDICT [optional feedback]
var evalPattern = regexp.MustCompile(`(?m)^AGENTIUM_EVAL:[ \t]+(ADVANCE|ITERATE|BLOCKED)[ \t]*(.*)$`)

// parseEvalVerdict extracts the evaluator verdict from agent output.
// If no verdict line is found, defaults to ADVANCE (fail-open) for backward compatibility.
func parseEvalVerdict(output string) EvalResult {
	matches := evalPattern.FindStringSubmatch(output)
	if matches == nil {
		return EvalResult{Verdict: VerdictAdvance, SignalFound: false}
	}
	return EvalResult{
		Verdict:     EvalVerdict(matches[1]),
		Feedback:    strings.TrimSpace(matches[2]),
		SignalFound: true,
	}
}

// parseJudgeVerdict extracts the judge verdict from agent output.
// If no verdict line is found, defaults to ITERATE (fail-closed).
func parseJudgeVerdict(output string) EvalResult {
	matches := evalPattern.FindStringSubmatch(output)
	if matches == nil {
		return EvalResult{Verdict: VerdictIterate, SignalFound: false}
	}
	return EvalResult{
		Verdict:     EvalVerdict(matches[1]),
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

// runEvaluator runs an evaluator agent against the completed phase output,
// parses the verdict, and stores feedback in memory on ITERATE.
func (c *Controller) runEvaluator(ctx context.Context, completedPhase TaskPhase, phaseOutput string) (EvalResult, error) {
	evalPrompt := c.buildEvalPrompt(completedPhase, phaseOutput)

	session := &agent.Session{
		ID:             c.config.ID,
		Repository:     c.config.Repository,
		Tasks:          c.config.Tasks,
		WorkDir:        c.workDir,
		GitHubToken:    c.gitHubToken,
		MaxIterations:  1,
		MaxDuration:    c.config.MaxDuration,
		Prompt:         evalPrompt,
		Metadata:       make(map[string]string),
		ClaudeAuthMode: c.config.ClaudeAuth.AuthMode,
		SystemPrompt:   c.systemPrompt,
		ActiveTask:     c.activeTask,
	}

	// Compose EVALUATE phase skills
	if c.skillSelector != nil {
		session.IterationContext = &agent.IterationContext{
			Phase:        "EVALUATE",
			SkillsPrompt: c.skillSelector.SelectForPhase("EVALUATE"),
		}
	}

	// Select adapter for EVALUATE phase
	activeAgent := c.agent
	if c.modelRouter != nil && c.modelRouter.IsConfigured() {
		modelCfg := c.modelRouter.ModelForPhase("EVALUATE")
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

	c.logInfo("Running evaluator for phase %s", completedPhase)

	result, err := c.runAgentContainer(ctx, containerRunParams{
		Agent:   activeAgent,
		Session: session,
		Env:     env,
		Command: command,
		LogTag:  "Evaluator",
	})
	if err != nil {
		return EvalResult{Verdict: VerdictAdvance}, fmt.Errorf("evaluator failed: %w", err)
	}

	// Parse verdict from raw text content first, fall back to summary
	parseSource := result.RawTextContent
	if parseSource == "" {
		parseSource = result.Summary
	}
	evalResult := parseEvalVerdict(parseSource)
	c.logInfo("Evaluator verdict for phase %s: %s", completedPhase, evalResult.Verdict)

	// On ITERATE, store feedback in memory for the next iteration
	if evalResult.Verdict == VerdictIterate && evalResult.Feedback != "" && c.memoryStore != nil {
		c.memoryStore.Update([]memory.Signal{
			{Type: memory.EvalFeedback, Content: evalResult.Feedback},
		}, c.iteration, fmt.Sprintf("issue:%s", c.activeTask))
	}

	return evalResult, nil
}

// evalContextBudget returns the configured max characters for evaluator context,
// falling back to the default when not specified.
func (c *Controller) evalContextBudget() int {
	if c.config.PhaseLoop != nil && c.config.PhaseLoop.EvalContextBudget > 0 {
		return c.config.PhaseLoop.EvalContextBudget
	}
	return defaultEvalContextBudget
}

// runJudge runs a judge agent that interprets reviewer feedback and decides
// whether to ADVANCE, ITERATE, or BLOCKED.
func (c *Controller) runJudge(ctx context.Context, params judgeRunParams) (EvalResult, error) {
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

	// Resolve phase key: <PHASE>_JUDGE → JUDGE → EVALUATE → default
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
		evalCtx := c.memoryStore.BuildEvalContext()
		if evalCtx != "" {
			if session.IterationContext == nil {
				session.IterationContext = &agent.IterationContext{}
			}
			session.IterationContext.MemoryContext = evalCtx
		}
	}

	// Select adapter via compound key fallback chain
	activeAgent := c.agent
	if c.modelRouter != nil && c.modelRouter.IsConfigured() {
		modelCfg := c.modelRouter.ModelForPhase(judgePhase)
		// Fallback: JUDGE → EVALUATE → default
		if modelCfg.Adapter == "" && modelCfg.Model == "" {
			modelCfg = c.modelRouter.ModelForPhase("JUDGE")
		}
		if modelCfg.Adapter == "" && modelCfg.Model == "" {
			modelCfg = c.modelRouter.ModelForPhase("EVALUATE")
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

	c.logInfo("Running judge for phase %s (iteration %d/%d)", params.CompletedPhase, params.Iteration, params.MaxIterations)

	result, err := c.runAgentContainer(ctx, containerRunParams{
		Agent:   activeAgent,
		Session: session,
		Env:     env,
		Command: command,
		LogTag:  "Judge",
	})
	if err != nil {
		return EvalResult{Verdict: VerdictAdvance}, fmt.Errorf("judge failed: %w", err)
	}

	parseSource := result.RawTextContent
	if parseSource == "" {
		parseSource = result.Summary
	}
	evalResult := parseJudgeVerdict(parseSource)
	c.logInfo("Judge verdict for phase %s: %s (signal_found=%v)", params.CompletedPhase, evalResult.Verdict, evalResult.SignalFound)

	// Parse review mode signal when complexity assessment was requested
	if params.AssessComplexity {
		evalResult.ReviewMode = parseReviewModeSignal(parseSource)
	}

	// On ITERATE, store the reviewer's feedback (not the judge's) in memory for the worker
	if evalResult.Verdict == VerdictIterate && params.ReviewFeedback != "" && c.memoryStore != nil {
		c.memoryStore.Update([]memory.Signal{
			{Type: memory.EvalFeedback, Content: params.ReviewFeedback},
		}, c.iteration, fmt.Sprintf("issue:%s", c.activeTask))
	}

	return evalResult, nil
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
	budget := c.evalContextBudget()
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

// buildEvalPrompt composes the evaluator prompt with phase context.
func (c *Controller) buildEvalPrompt(phase TaskPhase, phaseOutput string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("You are evaluating the output of the **%s** phase.\n\n", phase))
	sb.WriteString(fmt.Sprintf("Repository: %s\n", c.config.Repository))
	sb.WriteString(fmt.Sprintf("Issue: #%s\n\n", c.activeTask))

	sb.WriteString("## Phase Output\n\n")
	// Truncate very long outputs to avoid exceeding context
	budget := c.evalContextBudget()
	output := phaseOutput
	if len(output) > budget {
		output = output[:budget] + "\n\n... (output truncated)"
	}
	sb.WriteString("```\n")
	sb.WriteString(output)
	sb.WriteString("\n```\n\n")

	sb.WriteString("## Your Task\n\n")
	sb.WriteString("Evaluate the phase output above and emit your verdict.\n")
	sb.WriteString("You MUST emit exactly one line starting with `AGENTIUM_EVAL:` followed by your verdict.\n")

	return sb.String()
}
