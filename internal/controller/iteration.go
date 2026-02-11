package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/andywolf/agentium/internal/agent"
	"github.com/andywolf/agentium/internal/handoff"
)

// runIteration executes a single agent iteration, building the session context
// and running the agent container. Handles phase-aware prompts, handoff injection,
// memory context, model routing, and adapter fallback.
func (c *Controller) runIteration(ctx context.Context) (*agent.IterationResult, error) {
	// Build phase-aware prompt FIRST (before delegation check)
	// This ensures both delegated and non-delegated paths use the same phase-appropriate prompt
	prompt := c.config.Prompt
	if c.activeTaskType == "issue" && c.activeTask != "" {
		phase := c.determineActivePhase()
		prompt = c.buildPromptForTask(c.activeTask, c.activeTaskExistingWork, phase)
	}

	// Check delegation AFTER prompt is built
	if c.orchestrator != nil {
		phase := c.determineActivePhase()
		if subCfg := c.orchestrator.ConfigForPhase(phase); subCfg != nil {
			c.logInfo("Phase %s: delegating to sub-agent config (agent=%s)", phase, subCfg.Agent)
			return c.runDelegatedIteration(ctx, phase, subCfg, prompt)
		}
	}

	// Build project prompt with package scope instructions if applicable
	projectPrompt := c.projectPrompt
	if scopeInstructions := c.buildPackageScopeInstructions(); scopeInstructions != "" {
		if projectPrompt != "" {
			projectPrompt = projectPrompt + "\n\n" + scopeInstructions
		} else {
			projectPrompt = scopeInstructions
		}
	}

	// Initialize IterationContext once at session creation to avoid repeated nil checks
	session := &agent.Session{
		ID:               c.config.ID,
		Repository:       c.config.Repository,
		Tasks:            c.config.Tasks,
		WorkDir:          c.workDir,
		GitHubToken:      c.gitHubToken,
		MaxDuration:      c.config.MaxDuration,
		Prompt:           prompt,
		Metadata:         make(map[string]string),
		ClaudeAuthMode:   c.config.ClaudeAuth.AuthMode,
		SystemPrompt:     c.systemPrompt,
		ProjectPrompt:    projectPrompt,
		ActiveTask:       c.activeTask,
		ExistingWork:     c.activeTaskExistingWork,
		Interactive:      c.config.Interactive,
		IterationContext: &agent.IterationContext{},
		PackagePath:      c.packagePath,
	}

	// Compose phase-aware skills if selector is available
	if c.skillSelector != nil {
		phase := c.determineActivePhase()
		phaseStr := string(phase)
		session.IterationContext.Phase = phaseStr
		session.IterationContext.SkillsPrompt = c.skillSelector.SelectForPhase(phaseStr)
		c.logInfo("Using skills for phase %s: %v", phase, c.skillSelector.SkillsForPhase(phaseStr))
	}

	// Inject structured handoff context if enabled
	handoffInjected := false
	if c.isHandoffEnabled() {
		taskID := taskKey(c.activeTaskType, c.activeTask)
		phase := handoff.Phase(c.determineActivePhase())
		phaseInput, err := c.handoffBuilder.BuildMarkdownContext(taskID, phase)
		if err != nil {
			c.logWarning("Failed to build handoff context for phase %s: %v (falling back to memory)", phase, err)
		} else if phaseInput != "" {
			session.IterationContext.PhaseInput = phaseInput
			handoffInjected = true
			c.logInfo("Injected handoff context for phase %s (%d chars)", phase, len(phaseInput))
		}
	}

	// Inject feedback from previous iteration for ITERATE cycles
	// This ensures workers receive both reviewer analysis and judge directives
	if c.memoryStore != nil {
		taskID := taskKey(c.activeTaskType, c.activeTask)
		state := c.taskStates[taskID]
		if state != nil && state.PhaseIteration > 1 {
			feedbackSection := c.buildIterateFeedbackSection(taskID, state.PhaseIteration, state.ParentBranch)
			if feedbackSection != "" {
				// Prepend to PhaseInput for maximum visibility
				if session.IterationContext.PhaseInput != "" {
					session.IterationContext.PhaseInput = feedbackSection + "\n\n" + session.IterationContext.PhaseInput
				} else {
					session.IterationContext.PhaseInput = feedbackSection
				}
				c.logInfo("Injected ITERATE feedback section (%d chars)", len(feedbackSection))
			}
		}
	}

	// Inject memory context as fallback if handoff wasn't injected
	// This ensures PR tasks and unsupported phases still get context
	if c.memoryStore != nil && !handoffInjected {
		taskID := taskKey(c.activeTaskType, c.activeTask)
		memCtx := c.memoryStore.BuildContext(taskID)
		if memCtx != "" {
			session.IterationContext.MemoryContext = memCtx
		}
	}

	// Select adapter and model based on routing config
	activeAgent := c.agent
	if c.modelRouter != nil && c.modelRouter.IsConfigured() {
		phase := c.determineActivePhase()
		phaseStr := string(phase)
		modelCfg := c.modelRouter.ModelForPhase(phaseStr)
		if modelCfg.Adapter != "" {
			if a, ok := c.adapters[modelCfg.Adapter]; ok {
				activeAgent = a
			} else {
				c.logWarning("Phase %s: configured adapter %q not found in initialized adapters, using default %q", phase, modelCfg.Adapter, c.agent.Name())
			}
		}
		if modelCfg.Model != "" {
			session.IterationContext.ModelOverride = modelCfg.Model
		}
		if modelCfg.Reasoning != "" {
			session.IterationContext.ReasoningOverride = modelCfg.Reasoning
		}
		c.logInfo("Routing phase %s: adapter=%s model=%s", phase, activeAgent.Name(), modelCfg.Model)
	}

	// Build environment and command
	env := activeAgent.BuildEnv(session, c.iteration)
	command := activeAgent.BuildCommand(session, c.iteration)

	// Check if agent supports stdin-based prompt delivery (for non-interactive mode)
	stdinPrompt := ""
	if !c.config.Interactive {
		if provider, ok := activeAgent.(agent.StdinPromptProvider); ok {
			stdinPrompt = provider.GetStdinPrompt(session, c.iteration)
		}
	}

	params := containerRunParams{
		Agent:       activeAgent,
		Session:     session,
		Env:         env,
		Command:     command,
		LogTag:      "Agent",
		StdinPrompt: stdinPrompt,
	}

	// Use interactive Docker execution in local mode
	if c.config.Interactive {
		return c.runAgentContainerInteractive(ctx, params)
	}

	execStart := time.Now()
	result, err := c.runAgentContainer(ctx, params)
	execDuration := time.Since(execStart)

	// Attempt fallback on adapter execution failure
	if err != nil && c.canFallback(activeAgent.Name(), session) {
		stderr := ""
		if result != nil {
			stderr = result.Error
		}

		if isAdapterExecutionFailure(err, stderr, execDuration) {
			fallbackName := c.getFallbackAdapter()
			if fallbackName == activeAgent.Name() {
				c.logWarning("Adapter %s failed (%v), retrying without model override",
					activeAgent.Name(), err)
			} else {
				c.logWarning("Adapter %s failed (%v), falling back to %s",
					activeAgent.Name(), err, fallbackName)
			}

			fallbackAdapter := c.adapters[fallbackName]
			fallbackParams := c.buildFallbackParams(fallbackAdapter, session, activeAgent.Name())
			return c.runAgentContainer(ctx, fallbackParams)
		}
	}

	return result, err
}

// buildFallbackParams constructs container run parameters for the fallback adapter.
// It clones the session without model override so the fallback adapter uses its defaults.
func (c *Controller) buildFallbackParams(adapter agent.Agent, session *agent.Session, originalAdapter string) containerRunParams {
	// Clone session without model override (use fallback adapter's default)
	fallbackSession := *session
	if fallbackSession.IterationContext != nil {
		ctx := *fallbackSession.IterationContext
		ctx.ModelOverride = ""
		fallbackSession.IterationContext = &ctx
	}

	env := adapter.BuildEnv(&fallbackSession, c.iteration)
	cmd := adapter.BuildCommand(&fallbackSession, c.iteration)

	var stdinPrompt string
	if provider, ok := adapter.(agent.StdinPromptProvider); ok {
		stdinPrompt = provider.GetStdinPrompt(&fallbackSession, c.iteration)
	}

	return containerRunParams{
		Agent:       adapter,
		Session:     &fallbackSession,
		Env:         env,
		Command:     cmd,
		LogTag:      fmt.Sprintf("Agent (fallback from %s)", originalAdapter),
		StdinPrompt: stdinPrompt,
	}
}
