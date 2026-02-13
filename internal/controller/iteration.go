package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/andywolf/agentium/internal/agent"
	"github.com/andywolf/agentium/internal/handoff"
)

// phaseIteration returns the 1-indexed, phase-scoped iteration counter for
// the active task. Agent containers see this value as AGENTIUM_ITERATION
// (resets at each phase transition), while c.iteration remains the session-global
// counter used for memory, logging, and event tracking.
func (c *Controller) phaseIteration() int {
	taskID := taskKey(c.activeTaskType, c.activeTask)
	if state, ok := c.taskStates[taskID]; ok {
		return state.PhaseIteration
	}
	return 1
}

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

	// Compose phase-aware skills: API-provided worker prompt takes precedence over built-in skills
	phase := c.determineActivePhase()
	if workerPrompt := c.phaseWorkerPrompt(phase); workerPrompt != "" {
		session.IterationContext.Phase = string(phase)
		session.IterationContext.SkillsPrompt = workerPrompt
		c.logInfo("Using API-provided worker prompt for phase %s", phase)
	} else if c.skillSelector != nil {
		phaseStr := string(phase)
		session.IterationContext.Phase = phaseStr
		session.IterationContext.SkillsPrompt = c.skillSelector.SelectForPhase(phaseStr)
		c.logInfo("Using skills for phase %s: %v", phase, c.skillSelector.SkillsForPhase(phaseStr))
	}
	skillsPrompt := session.IterationContext.SkillsPrompt

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

	// Inject feedback from previous iteration for ITERATE cycles.
	// This ensures workers receive both reviewer analysis and judge directives.
	// buildIterateFeedbackSection checks memory store first, then falls back to
	// TaskState fields, so no outer nil guard is needed.
	feedbackTaskID := taskKey(c.activeTaskType, c.activeTask)
	if state := c.taskStates[feedbackTaskID]; state != nil && state.PhaseIteration > 1 {
		feedbackSection := c.buildIterateFeedbackSection(feedbackTaskID, state.PhaseIteration, state.ParentBranch, state.Phase)
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

	// Build environment and command using phase-scoped iteration
	phaseIter := c.phaseIteration()
	env := activeAgent.BuildEnv(session, phaseIter)
	command := activeAgent.BuildCommand(session, phaseIter)

	// Check if agent supports stdin-based prompt delivery (for non-interactive mode)
	stdinPrompt := ""
	if !c.config.Interactive {
		if provider, ok := activeAgent.(agent.StdinPromptProvider); ok {
			stdinPrompt = provider.GetStdinPrompt(session, phaseIter)
		}
	}

	// Determine prompt input for Langfuse generation tracking
	promptInput := stdinPrompt
	if promptInput == "" {
		promptInput = prompt
	}

	params := containerRunParams{
		Agent:       activeAgent,
		Session:     session,
		Env:         env,
		Command:     command,
		LogTag:      "Agent",
		StdinPrompt: stdinPrompt,
	}

	execStart := time.Now()

	// Use interactive Docker execution in local mode
	if c.config.Interactive {
		result, err := c.runAgentContainerInteractive(ctx, params)
		if result != nil {
			result.PromptInput = promptInput
			result.SystemPrompt = skillsPrompt
			result.StartTime = execStart
			result.EndTime = time.Now()
		}
		return result, err
	}

	// Use pooled execution if container pool is active
	if c.containerPool != nil && c.containerPool.IsHealthy(RoleWorkerContainer) {
		result, err := c.runIterationPooled(ctx, activeAgent, session, params)
		if result != nil {
			if result.PromptInput == "" {
				result.PromptInput = promptInput
			}
			result.SystemPrompt = skillsPrompt
			result.StartTime = execStart
			result.EndTime = time.Now()
		}
		return result, err
	}

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
			fallbackParams := c.buildFallbackParams(fallbackAdapter, session, activeAgent.Name(), phaseIter)
			fbResult, fbErr := c.runAgentContainer(ctx, fallbackParams)
			if fbResult != nil {
				fbResult.PromptInput = promptInput
				fbResult.SystemPrompt = skillsPrompt
				fbResult.StartTime = execStart
				fbResult.EndTime = time.Now()
			}
			return fbResult, fbErr
		}
	}

	if result != nil {
		result.PromptInput = promptInput
		result.SystemPrompt = skillsPrompt
		result.StartTime = execStart
		result.EndTime = time.Now()
	}
	return result, err
}

// runIterationPooled executes a worker iteration using the container pool.
// On iteration 1, uses the full prompt. On iteration 2+, uses continuation mode
// if the agent supports it (only incremental feedback, --continue flag).
//
// Note: adapter fallback (retrying with a different adapter on execution failure)
// is not replicated in the pooled path. The pooled container is started with a
// specific adapter image at phase start, so mid-iteration adapter switching is
// not possible. On pooled exec failure, the pool is marked unhealthy and the
// next iteration falls back to one-shot execution, which does support fallback.
func (c *Controller) runIterationPooled(ctx context.Context, activeAgent agent.Agent, session *agent.Session, params containerRunParams) (*agent.IterationResult, error) {
	taskID := taskKey(c.activeTaskType, c.activeTask)
	state := c.taskStates[taskID]

	// Check if this is a continuation iteration (2+) with a capable agent
	if state != nil && state.PhaseIteration > 1 {
		if cc, ok := activeAgent.(agent.ContinuationCapable); ok && cc.SupportsContinuation() {
			return c.runIterationContinue(ctx, activeAgent, session, state)
		}
	}

	// Iteration 1 or non-continuation-capable: use full prompt via pooled exec
	c.logInfo("Using pooled execution for Worker (phase iteration %d)", c.phaseIteration())
	return c.runAgentContainerPooled(ctx, RoleWorkerContainer, params)
}

// runIterationContinue executes a worker continuation iteration.
// It builds only the incremental feedback (reviewer analysis + judge directives)
// and pipes it via stdin with the --continue flag, preserving the conversation
// context from the first invocation.
func (c *Controller) runIterationContinue(ctx context.Context, activeAgent agent.Agent, session *agent.Session, state *TaskState) (*agent.IterationResult, error) {
	c.logInfo("Using continuation mode for Worker (phase iteration %d)", state.PhaseIteration)

	cc, ok := activeAgent.(agent.ContinuationCapable)
	if !ok {
		return nil, fmt.Errorf("agent %s does not implement ContinuationCapable", activeAgent.Name())
	}
	command := cc.BuildContinueCommand(session, state.PhaseIteration)

	// Build incremental feedback as the stdin prompt
	taskID := taskKey(c.activeTaskType, c.activeTask)
	feedbackSection := c.buildIterateFeedbackSection(taskID, state.PhaseIteration, state.ParentBranch, state.Phase)
	if feedbackSection == "" {
		feedbackSection = fmt.Sprintf("Continue working on the current phase. This is iteration %d.", state.PhaseIteration)
	}

	params := containerRunParams{
		Agent:       activeAgent,
		Session:     session,
		Env:         activeAgent.BuildEnv(session, state.PhaseIteration),
		Command:     command,
		LogTag:      "Agent (continue)",
		StdinPrompt: feedbackSection,
	}

	contStart := time.Now()
	result, err := c.runAgentContainerPooled(ctx, RoleWorkerContainer, params)
	if result != nil {
		result.PromptInput = feedbackSection
		result.StartTime = contStart
		result.EndTime = time.Now()
	}
	return result, err
}

// buildFallbackParams constructs container run parameters for the fallback adapter.
// It clones the session without model override so the fallback adapter uses its defaults.
func (c *Controller) buildFallbackParams(adapter agent.Agent, session *agent.Session, originalAdapter string, phaseIter int) containerRunParams {
	// Clone session without model override (use fallback adapter's default)
	fallbackSession := *session
	if fallbackSession.IterationContext != nil {
		ctx := *fallbackSession.IterationContext
		ctx.ModelOverride = ""
		fallbackSession.IterationContext = &ctx
	}

	env := adapter.BuildEnv(&fallbackSession, phaseIter)
	cmd := adapter.BuildCommand(&fallbackSession, phaseIter)

	var stdinPrompt string
	if provider, ok := adapter.(agent.StdinPromptProvider); ok {
		stdinPrompt = provider.GetStdinPrompt(&fallbackSession, phaseIter)
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
