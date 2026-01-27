package controller

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/andywolf/agentium/internal/agent"
)

// ComplexityResult holds the parsed complexity verdict and feedback.
type ComplexityResult struct {
	Verdict     ComplexityVerdict
	Feedback    string
	SignalFound bool // Whether the AGENTIUM_EVAL signal was found in output
}

// complexityRunParams holds parameters for running a complexity assessor.
type complexityRunParams struct {
	PlanOutput    string
	Iteration     int
	MaxIterations int
}

// complexityPattern matches lines of the form: AGENTIUM_EVAL: SIMPLE|COMPLEX [optional feedback]
var complexityPattern = regexp.MustCompile(`(?m)^AGENTIUM_EVAL:[ \t]+(SIMPLE|COMPLEX)[ \t]*(.*)$`)

// parseComplexityVerdict extracts the complexity verdict from agent output.
// If no verdict line is found, defaults to COMPLEX (conservative fail-closed).
func parseComplexityVerdict(output string) ComplexityResult {
	matches := complexityPattern.FindStringSubmatch(output)
	if matches == nil {
		return ComplexityResult{Verdict: ComplexityComplex, SignalFound: false}
	}
	return ComplexityResult{
		Verdict:     ComplexityVerdict(matches[1]),
		Feedback:    strings.TrimSpace(matches[2]),
		SignalFound: true,
	}
}

// runComplexityAssessor runs a complexity assessor agent that determines
// whether the task is SIMPLE or COMPLEX based on the plan produced by the worker.
func (c *Controller) runComplexityAssessor(ctx context.Context, params complexityRunParams) (ComplexityResult, error) {
	assessorPrompt := c.buildComplexityPrompt(params)

	session := &agent.Session{
		ID:             c.config.ID,
		Repository:     c.config.Repository,
		Tasks:          c.config.Tasks,
		WorkDir:        c.workDir,
		GitHubToken:    c.gitHubToken,
		MaxIterations:  1,
		MaxDuration:    c.config.MaxDuration,
		Prompt:         assessorPrompt,
		Metadata:       make(map[string]string),
		ClaudeAuthMode: c.config.ClaudeAuth.AuthMode,
		SystemPrompt:   c.systemPrompt,
		ActiveTask:     c.activeTask,
	}

	// Resolve phase key: PLAN_COMPLEXITY → COMPLEXITY → default
	complexityPhase := "PLAN_COMPLEXITY"
	skillPhase := complexityPhase

	if c.skillSelector != nil {
		session.IterationContext = &agent.IterationContext{
			Phase:        skillPhase,
			SkillsPrompt: c.skillSelector.SelectForPhase(skillPhase),
		}
	}

	// Select adapter via compound key fallback chain
	activeAgent := c.agent
	if c.modelRouter != nil && c.modelRouter.IsConfigured() {
		modelCfg := c.modelRouter.ModelForPhase(complexityPhase)
		// Fallback to COMPLEXITY if no specific override
		if modelCfg.Adapter == "" && modelCfg.Model == "" {
			modelCfg = c.modelRouter.ModelForPhase("COMPLEXITY")
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

	c.logInfo("Running complexity assessor for PLAN (iteration %d/%d)", params.Iteration, params.MaxIterations)

	result, err := c.runAgentContainer(ctx, containerRunParams{
		Agent:       activeAgent,
		Session:     session,
		Env:         env,
		Command:     command,
		LogTag:      "ComplexityAssessor",
		StdinPrompt: stdinPrompt,
	})
	if err != nil {
		return ComplexityResult{Verdict: ComplexityComplex}, fmt.Errorf("complexity assessor failed: %w", err)
	}

	parseSource := result.RawTextContent
	if parseSource == "" {
		parseSource = result.Summary
	}
	complexityResult := parseComplexityVerdict(parseSource)
	c.logInfo("Complexity verdict: %s (signal_found=%v)", complexityResult.Verdict, complexityResult.SignalFound)

	return complexityResult, nil
}

// buildComplexityPrompt composes the complexity assessor prompt with plan context.
func (c *Controller) buildComplexityPrompt(params complexityRunParams) string {
	var sb strings.Builder

	sb.WriteString("You are the **complexity assessor** for the PLAN phase.\n\n")
	sb.WriteString(fmt.Sprintf("Repository: %s\n", c.config.Repository))
	sb.WriteString(fmt.Sprintf("Issue: #%s\n\n", c.activeTask))

	sb.WriteString("## Plan Output\n\n")
	budget := c.judgeContextBudget()
	output := params.PlanOutput
	if len(output) > budget {
		output = output[:budget] + "\n\n... (output truncated)"
	}
	sb.WriteString("```\n")
	sb.WriteString(output)
	sb.WriteString("\n```\n\n")

	sb.WriteString("## Your Task\n\n")
	sb.WriteString("Assess whether this task is SIMPLE or COMPLEX based on the plan above.\n")
	sb.WriteString("You MUST emit exactly one line starting with `AGENTIUM_EVAL:` followed by your verdict.\n\n")

	sb.WriteString("### Verdicts\n\n")
	sb.WriteString("- `AGENTIUM_EVAL: SIMPLE <reason>` - Straightforward change:\n")
	sb.WriteString("  - Single file or few closely-related files\n")
	sb.WriteString("  - Clear, well-defined scope\n")
	sb.WriteString("  - No architectural decisions needed\n")
	sb.WriteString("  - Standard patterns, no edge cases\n\n")
	sb.WriteString("- `AGENTIUM_EVAL: COMPLEX <reason>` - Complex change:\n")
	sb.WriteString("  - Multiple files or components\n")
	sb.WriteString("  - Architectural decisions required\n")
	sb.WriteString("  - Cross-cutting concerns\n")
	sb.WriteString("  - Edge cases or error handling complexity\n")
	sb.WriteString("  - Changes to public APIs or interfaces\n\n")

	sb.WriteString("**When in doubt, choose COMPLEX.** It's better to review thoroughly than to skip review on a complex change.\n")

	return sb.String()
}
