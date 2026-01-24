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
	Verdict  EvalVerdict
	Feedback string
}

// evalPattern matches lines of the form: AGENTIUM_EVAL: VERDICT [optional feedback]
var evalPattern = regexp.MustCompile(`(?m)^AGENTIUM_EVAL:[ \t]+(ADVANCE|ITERATE|BLOCKED)[ \t]*(.*)$`)

// parseEvalVerdict extracts the evaluator verdict from agent output.
// If no verdict line is found, defaults to ADVANCE (fail-open).
func parseEvalVerdict(output string) EvalResult {
	matches := evalPattern.FindStringSubmatch(output)
	if matches == nil {
		return EvalResult{Verdict: VerdictAdvance}
	}
	return EvalResult{
		Verdict:  EvalVerdict(matches[1]),
		Feedback: strings.TrimSpace(matches[2]),
	}
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

// buildEvalPrompt composes the evaluator prompt with phase context.
func (c *Controller) buildEvalPrompt(phase TaskPhase, phaseOutput string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("You are evaluating the output of the **%s** phase.\n\n", phase))
	sb.WriteString(fmt.Sprintf("Repository: %s\n", c.config.Repository))
	sb.WriteString(fmt.Sprintf("Issue: #%s\n\n", c.activeTask))

	sb.WriteString("## Phase Output\n\n")
	// Truncate very long outputs to avoid exceeding context
	output := phaseOutput
	if len(output) > 8000 {
		output = output[:8000] + "\n\n... (output truncated)"
	}
	sb.WriteString("```\n")
	sb.WriteString(output)
	sb.WriteString("\n```\n\n")

	sb.WriteString("## Your Task\n\n")
	sb.WriteString("Evaluate the phase output above and emit your verdict.\n")
	sb.WriteString("You MUST emit exactly one line starting with `AGENTIUM_EVAL:` followed by your verdict.\n")

	return sb.String()
}
