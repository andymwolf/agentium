package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/andywolf/agentium/internal/agent"
	"github.com/andywolf/agentium/prompts/phases"
)

// SynthesisResult holds the output of the synthesis step in multi-reviewer mode.
type SynthesisResult struct {
	Feedback     string // Unified, deduplicated feedback for the judge
	Prompt       string // Prompt sent to the synthesizer (for Langfuse)
	SystemPrompt string // System/skills prompt (for Langfuse)
	InputTokens  int
	OutputTokens int
	StartTime    time.Time
	EndTime      time.Time
}

// runSynthesis runs the synthesis agent that merges findings from multiple reviewers
// into a single deduplicated, classified report for the judge.
func (c *Controller) runSynthesis(ctx context.Context, phase TaskPhase, results []NamedReviewResult, params reviewRunParams) (SynthesisResult, error) {
	c.logInfo("Starting synthesis for phase %s (%d reviewer results)...", phase, len(results))

	synthesisPrompt := c.buildSynthesisPrompt(phase, results, params)

	session := &agent.Session{
		ID:             c.config.ID,
		Repository:     c.config.Repository,
		Tasks:          c.config.Tasks,
		WorkDir:        c.workDir,
		GitHubToken:    c.gitHubToken,
		MaxDuration:    c.config.MaxDuration,
		Prompt:         synthesisPrompt,
		Metadata:       make(map[string]string),
		ClaudeAuthMode: c.config.ClaudeAuth.AuthMode,
		SystemPrompt:   c.systemPrompt,
		ActiveTask:     c.activeTask,
	}

	// Resolve skills prompt: API-provided → built-in → empty
	synthesisPhase := fmt.Sprintf("%s_SYNTHESIS", phase)
	var skillsPrompt string
	if stepCfg, ok := c.phaseConfigs[phase]; ok && stepCfg.Synthesis != nil && stepCfg.Synthesis.Prompt != "" {
		skillsPrompt = stepCfg.Synthesis.Prompt
		c.logInfo("Using API-provided synthesis prompt for phase %s", phase)
	} else {
		skillsPrompt = phases.Get(string(phase), "SYNTHESIS")
	}

	skillsPrompt = c.renderWithParameters(skillsPrompt)
	session.IterationContext = &agent.IterationContext{
		Phase:        synthesisPhase,
		SkillsPrompt: skillsPrompt,
	}

	// Select adapter via compound key fallback chain: {PHASE}_SYNTHESIS → SYNTHESIS → default
	activeAgent := c.agent
	if c.modelRouter != nil && c.modelRouter.IsConfigured() {
		modelCfg := c.modelRouter.ModelForPhase(synthesisPhase)
		if modelCfg.Adapter == "" && modelCfg.Model == "" {
			modelCfg = c.modelRouter.ModelForPhase("SYNTHESIS")
		}
		if modelCfg.Adapter != "" {
			if a, ok := c.adapters[modelCfg.Adapter]; ok {
				activeAgent = a
			} else {
				c.logWarning("Synthesis: configured adapter %q not found, using default %q",
					modelCfg.Adapter, c.agent.Name())
			}
		}
		if modelCfg.Model != "" {
			session.IterationContext.ModelOverride = modelCfg.Model
		}
		if modelCfg.Reasoning != "" {
			session.IterationContext.ReasoningOverride = modelCfg.Reasoning
		}
	}

	env := activeAgent.BuildEnv(session, 0)
	command := activeAgent.BuildCommand(session, 0)

	stdinPrompt := ""
	if provider, ok := activeAgent.(agent.StdinPromptProvider); ok {
		stdinPrompt = provider.GetStdinPrompt(session, 0)
	}

	modelName := ""
	if session.IterationContext.ModelOverride != "" {
		modelName = session.IterationContext.ModelOverride
	}
	c.logInfo("Running synthesis for phase %s: adapter=%s model=%s", phase, activeAgent.Name(), modelName)

	synthParams := containerRunParams{
		Agent:       activeAgent,
		Session:     session,
		Env:         env,
		Command:     command,
		LogTag:      "Synthesis",
		StdinPrompt: stdinPrompt,
	}

	// Synthesis always uses one-shot execution
	synthStart := time.Now()
	result, err := c.runAgentContainer(ctx, synthParams)
	synthEnd := time.Now()
	if err != nil {
		c.logError("Synthesis container failed for phase %s: %v", phase, err)
		return SynthesisResult{}, fmt.Errorf("synthesis failed: %w", err)
	}

	feedback := result.AssistantText
	if feedback == "" {
		feedback = result.RawTextContent
	}
	if feedback == "" {
		c.logWarning("Synthesis for phase %s produced no text content", phase)
		return SynthesisResult{}, fmt.Errorf("synthesis produced no output")
	}

	c.logInfo("Synthesis completed for phase %s", phase)

	return SynthesisResult{
		Feedback:     feedback,
		Prompt:       stdinPrompt,
		SystemPrompt: skillsPrompt,
		InputTokens:  result.InputTokens,
		OutputTokens: result.OutputTokens,
		StartTime:    synthStart,
		EndTime:      synthEnd,
	}, nil
}

// buildSynthesisPrompt composes the prompt for the synthesis agent with all reviewer findings.
func (c *Controller) buildSynthesisPrompt(phase TaskPhase, results []NamedReviewResult, params reviewRunParams) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("You are synthesizing review findings for the **%s** phase (iteration %d/%d).\n\n",
		phase, params.Iteration, params.MaxIterations))
	sb.WriteString(fmt.Sprintf("Repository: %s\n", c.config.Repository))
	sb.WriteString(fmt.Sprintf("Issue: #%s\n\n", c.activeTask))

	sb.WriteString("## Reviewer Findings\n\n")
	sb.WriteString(fmt.Sprintf("%d specialized reviewers examined the work. Their individual findings are below.\n\n", len(results)))

	for _, r := range results {
		sb.WriteString(fmt.Sprintf("### Reviewer: %s\n\n", r.Name))
		sb.WriteString(r.Feedback)
		sb.WriteString("\n\n---\n\n")
	}

	sb.WriteString("## Your Task\n\n")
	sb.WriteString("Synthesize the reviewer findings into a single, deduplicated, classified report.\n")
	sb.WriteString("Follow the instructions in your skills prompt for deduplication, classification, numbering, and sorting.\n")

	return sb.String()
}
