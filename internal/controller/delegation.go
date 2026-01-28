package controller

import (
	"context"
	"fmt"

	"github.com/andywolf/agentium/internal/agent"
)

// runDelegatedIteration executes a single iteration using the delegated sub-task config.
// It resolves the agent adapter, builds skills and session context, and runs the
// agent container with the specified overrides.
// The prompt parameter contains the phase-aware prompt built by the caller.
func (c *Controller) runDelegatedIteration(ctx context.Context, phase TaskPhase, config *SubTaskConfig, prompt string) (*agent.IterationResult, error) {
	// Resolve agent adapter
	activeAgent := c.agent
	if config.Agent != "" {
		if a, ok := c.adapters[config.Agent]; ok {
			activeAgent = a
		} else {
			c.logWarning("Delegation phase %s: configured adapter %q not found, using default %q", phase, config.Agent, c.agent.Name())
		}
	}

	// Build skills prompt
	var skillsPrompt string
	if c.skillSelector != nil {
		if len(config.Skills) > 0 {
			skillsPrompt = c.skillSelector.SelectByNames(config.Skills)
		} else {
			skillsPrompt = c.skillSelector.SelectForPhase(string(phase))
		}
	}

	// Build model override
	var modelOverride string
	if config.Model != nil && config.Model.Model != "" {
		modelOverride = config.Model.Model
	}

	// Build session
	subTaskID := fmt.Sprintf("delegation-%s-%d", phase, c.iteration)
	session := &agent.Session{
		ID:             c.config.ID,
		Repository:     c.config.Repository,
		Tasks:          c.config.Tasks,
		WorkDir:        c.workDir,
		GitHubToken:    c.gitHubToken,
		MaxIterations:  c.config.MaxIterations,
		MaxDuration:    c.config.MaxDuration,
		Prompt:         prompt, // Use phase-aware prompt passed by caller
		Metadata:       make(map[string]string),
		ClaudeAuthMode: c.config.ClaudeAuth.AuthMode,
		SystemPrompt:   c.systemPrompt,
		ProjectPrompt:  c.projectPrompt,
		ActiveTask:     c.activeTask,
		ExistingWork:   c.activeTaskExistingWork,
		IterationContext: &agent.IterationContext{
			Phase:         string(phase),
			SkillsPrompt:  skillsPrompt,
			Iteration:     c.iteration,
			SubTaskID:     subTaskID,
			ModelOverride: modelOverride,
		},
	}

	// Inject memory context if store is available
	if c.memoryStore != nil {
		// Build context scoped to the current task
		taskID := taskKey(c.activeTaskType, c.activeTask)
		memCtx := c.memoryStore.BuildContext(taskID)
		if memCtx != "" {
			session.IterationContext.MemoryContext = memCtx
		}
	}

	c.logInfo("Delegating phase %s: adapter=%s subtask=%s", phase, activeAgent.Name(), subTaskID)

	// Build environment and command
	env := activeAgent.BuildEnv(session, c.iteration)
	command := activeAgent.BuildCommand(session, c.iteration)

	// Check if agent supports stdin-based prompt delivery
	stdinPrompt := ""
	if provider, ok := activeAgent.(agent.StdinPromptProvider); ok {
		stdinPrompt = provider.GetStdinPrompt(session, c.iteration)
	}

	return c.runAgentContainer(ctx, containerRunParams{
		Agent:       activeAgent,
		Session:     session,
		Env:         env,
		Command:     command,
		LogTag:      "Delegated agent",
		StdinPrompt: stdinPrompt,
	})
}
