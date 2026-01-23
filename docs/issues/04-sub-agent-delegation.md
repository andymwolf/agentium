# Phase 4: Sub-Agent Delegation

## Labels
`enhancement`, `architecture`, `delegation`

---

## Problem

Currently, a single agent handles the entire task lifecycle (plan -> implement -> test -> create PR) with the same configuration. This creates several issues:

- **No specialization**: The same agent with the same full skill set handles planning, coding, testing, and PR creation
- **No parallelism**: Independent sub-tasks (e.g., fixing frontend + backend) run sequentially within a single agent's context
- **Monolithic failure**: If the agent gets stuck on one aspect, it can't delegate to a fresh agent with different skills/context
- **One-size-fits-all**: Can't use different agent types for different parts of a task (e.g., aider for simple edits, claude-code for complex logic)

## Solution

Add a `SubTaskOrchestrator` to the controller that can run different agent/skill/model configurations per phase. When delegation is enabled, the controller matches the current phase to a sub-agent configuration and runs that specialized agent instead of the default.

## Sub-Task Types

```go
package controller

type SubTaskType string

const (
    SubTaskPlan      SubTaskType = "plan"
    SubTaskImplement SubTaskType = "implement"
    SubTaskTest      SubTaskType = "test"
    SubTaskReview    SubTaskType = "review"
)
```

## Configuration

```yaml
# .agentium.yaml
delegation:
  enabled: true
  strategy: "sequential"       # "sequential" or "parallel"
  sub_agents:
    plan:
      agent: "claude-code"
      model:
        model: "claude-sonnet-4-20250514"
      skills: ["safety", "environment", "planning", "status_signals"]
    implement:
      agent: "claude-code"
      model:
        model: "claude-opus-4-20250514"
      skills: ["safety", "environment", "implement", "test", "status_signals"]
    test:
      agent: "claude-code"
      model:
        model: "claude-sonnet-4-20250514"
      skills: ["safety", "environment", "test", "status_signals"]
    review:
      agent: "claude-code"
      model:
        model: "claude-sonnet-4-20250514"
      skills: ["safety", "environment", "pr_creation", "pr_review", "status_signals"]
```

## Types (`internal/controller/subtask.go`)

```go
package controller

import "github.com/andywolf/agentium/internal/routing"

// SubTaskConfig defines the configuration for a sub-task type
type SubTaskConfig struct {
    Agent  string              `json:"agent,omitempty" yaml:"agent,omitempty"`
    Model  *routing.ModelConfig `json:"model,omitempty" yaml:"model,omitempty"`
    Skills []string            `json:"skills,omitempty" yaml:"skills,omitempty"`
}

// DelegationConfig controls sub-agent delegation behavior
type DelegationConfig struct {
    Enabled   bool                          `json:"enabled" yaml:"enabled"`
    Strategy  string                        `json:"strategy" yaml:"strategy"`
    SubAgents map[SubTaskType]SubTaskConfig `json:"sub_agents,omitempty" yaml:"sub_agents,omitempty"`
}

// SubTask represents a decomposed unit of work at runtime
type SubTask struct {
    ID          string        `json:"id"`
    ParentTask  string        `json:"parent_task"`
    Type        SubTaskType   `json:"type"`
    Description string        `json:"description"`
    Config      SubTaskConfig `json:"config"`
    Status      string        `json:"status"` // "pending", "running", "completed", "failed"
    Result      *SubTaskResult `json:"result,omitempty"`
}

// SubTaskResult captures the outcome of a sub-task
type SubTaskResult struct {
    Success       bool     `json:"success"`
    Summary       string   `json:"summary"`
    FilesModified []string `json:"files_modified,omitempty"`
    NextSteps     []string `json:"next_steps,omitempty"`
    AgentStatus   string   `json:"agent_status,omitempty"`
}
```

## Orchestrator

```go
// SubTaskOrchestrator manages sub-task execution
type SubTaskOrchestrator struct {
    config     DelegationConfig
    controller *Controller
}

// ConfigForPhase returns the sub-agent config for a given phase,
// or nil if no delegation config exists for that phase.
func (o *SubTaskOrchestrator) ConfigForPhase(phase TaskPhase) *SubTaskConfig

// RunSubTask executes a single sub-task with its specific agent/skills/model.
func (o *SubTaskOrchestrator) RunSubTask(ctx context.Context, subtask *SubTask) error
```

## Phase-to-SubTask Mapping

| TaskPhase | SubTaskType | Description |
|-----------|-------------|-------------|
| IMPLEMENT | `implement` | Code generation and modification |
| TEST | `test` | Running tests, debugging failures |
| PR_CREATION | `review` | Creating PR with proper description |
| ANALYZE | `plan` | Understanding issue/PR, planning approach |
| PUSH | `review` | Pushing changes, final review |

## Controller Integration

The controller loop checks delegation config before running an iteration:

```go
func (c *Controller) runIteration(ctx context.Context) (*agent.IterationResult, error) {
    activePhase := c.determineActivePhase()

    // Check if delegation is active for this phase
    if c.orchestrator != nil {
        subConfig := c.orchestrator.ConfigForPhase(activePhase)
        if subConfig != nil {
            return c.runDelegatedIteration(ctx, activePhase, subConfig)
        }
    }

    // ... existing iteration logic (fallback) ...
}

func (c *Controller) runDelegatedIteration(ctx context.Context, phase TaskPhase, config *SubTaskConfig) (*agent.IterationResult, error) {
    // 1. Get the specified agent (or use default)
    agentName := c.config.Agent
    if config.Agent != "" {
        agentName = config.Agent
    }
    delegateAgent, err := agent.Get(agentName)
    if err != nil {
        return nil, fmt.Errorf("failed to get delegate agent %s: %w", agentName, err)
    }

    // 2. Build skills prompt from specified skills list
    var skillsPrompt string
    if c.skillSelector != nil && len(config.Skills) > 0 {
        skillsPrompt = c.skillSelector.SelectByNames(config.Skills)
    } else if c.skillSelector != nil {
        skillsPrompt = c.skillSelector.SelectForPhase(phase)
    }

    // 3. Build iteration context with overrides
    iterCtx := &agent.IterationContext{
        Phase:        string(phase),
        Iteration:    c.iteration,
        SkillsPrompt: skillsPrompt,
        SubTaskID:    fmt.Sprintf("%s-%s-%d", c.config.ID, phase, c.iteration),
    }

    // Apply model override from sub-agent config
    if config.Model != nil {
        iterCtx.ModelOverride = config.Model.Model
        iterCtx.MaxTokens = config.Model.MaxTokens
        iterCtx.Temperature = config.Model.Temperature
    }

    // 4. Build memory context (shared memory)
    if c.memoryStore != nil {
        data, _ := c.memoryStore.Load()
        builder := &memory.ContextBuilder{Budget: c.config.Memory.ContextBudget}
        iterCtx.MemoryContext = builder.BuildContext(data, string(phase))
    }

    // 5. Create session and run with delegate agent
    session := &agent.Session{
        // ... standard fields ...
        IterationContext: iterCtx,
    }

    // 6. Run the delegate agent container
    env := delegateAgent.BuildEnv(session, c.iteration)
    command := delegateAgent.BuildCommand(session, c.iteration)
    // ... docker run with delegateAgent.ContainerImage() ...

    // 7. Parse and return result
    result, err := delegateAgent.ParseOutput(exitCode, stdout, stderr)

    // 8. Update shared memory with sub-task results
    if c.memoryStore != nil {
        c.memoryStore.Update(c.iteration, activeTaskID, string(phase), memory.IterationOutcome{...})
        c.memoryStore.Save()
    }

    return result, err
}
```

## Shared Resources

Sub-agents share these with the main agent:
- **Workspace** (`/workspace`) - Same mounted directory, same git checkout
- **Memory store** (`.agentium/memory.json`) - Reads prior context, writes its results
- **GitHub token** - Same authentication
- **Git state** - Same branch, same commits

This means sub-agents can pick up exactly where the previous phase left off.

## Skill Selector Extension

Add a `SelectByNames` method to support custom skill lists:

```go
// SelectByNames returns composed prompt from specific skill names.
// Used by delegation to override phase-based selection.
func (s *Selector) SelectByNames(names []string) string {
    var skills []*Skill
    for _, name := range names {
        if skill, ok := s.loaded[name]; ok {
            skills = append(skills, skill)
        }
    }
    sort.Slice(skills, func(i, j int) bool {
        return skills[i].Priority < skills[j].Priority
    })
    // Compose and return
}
```

## Strategies

### Sequential (Default)

Sub-agents run one at a time. Each phase completes before the next begins. The controller's existing phase state machine determines which sub-agent runs next.

### Parallel (Future)

For tasks that can be decomposed into independent sub-problems:
- Multiple implement sub-agents run concurrently on different files/features
- Requires conflict detection and merge logic
- Not in initial implementation scope; flag is reserved for future use

## Design Decisions

1. **Delegation as phase override** vs task decomposition:
   - Phase override is simpler: just swap the agent/config for a known phase
   - Task decomposition (splitting one issue into multiple parallel sub-issues) is more complex and deferred to the parallel strategy

2. **Sub-agents reuse Docker execution**:
   - No new execution model required
   - Just a different container image, different CLI flags, different skills
   - Controller manages the lifecycle identically

3. **Shared workspace and memory**:
   - Sub-agents operate on the same git checkout
   - Memory provides continuity between phases
   - No need for result passing mechanisms beyond memory + git state

4. **Fallback when delegation not configured for a phase**:
   - The controller falls back to default iteration behavior
   - This means partial delegation works (e.g., only delegate IMPLEMENT, let default handle the rest)

## Implementation Steps

1. Define `SubTask`, `SubTaskConfig`, `DelegationConfig` in `internal/controller/subtask.go`
2. Implement `SubTaskOrchestrator` with `ConfigForPhase()` and `RunSubTask()`
3. Add `SelectByNames()` to `internal/skills/selector.go`
4. Add `DelegationConfig` to controller `SessionConfig`
5. Add `DelegationConfig` to CLI config
6. Modify controller `runIteration()` to check delegation before running
7. Implement `runDelegatedIteration()` with agent selection, skill override, model override
8. Ensure delegated agents write to shared memory
9. Add `SubTaskID` logging for debugging
10. Add unit tests for orchestrator and delegation logic
11. Add integration test: configure delegation for IMPLEMENT phase, verify correct agent/model used
12. Run `go build ./...` and `go test ./...`

## Acceptance Criteria

- [ ] `SubTaskOrchestrator` maps phases to sub-agent configs
- [ ] Controller runs delegated iterations when config exists for phase
- [ ] Sub-agents use specified agent adapter, skills, and model
- [ ] `SelectByNames()` works on skill selector
- [ ] Sub-agents share workspace and memory
- [ ] Fallback to default iteration when no delegation for phase
- [ ] Delegation disabled by default (opt-in)
- [ ] Partial delegation works (only some phases delegated)
- [ ] Unit tests pass
- [ ] `go build ./...` succeeds

## Dependencies

- Requires Phase 1: `IterationContext`, `determineActivePhase()`, skill selector
- Requires Phase 2: Shared memory store for sub-agent continuity
- Requires Phase 3: Model routing types (reuses `ModelConfig` in `SubTaskConfig`)

## Future Considerations

- **Parallel strategy**: Allow multiple implement sub-agents on independent file sets
- **Agent-specific containers**: Different Docker images per sub-task type
- **Sub-task retry**: If a sub-agent fails, retry with different config (e.g., upgrade model)
- **Cost reporting**: Track per-phase model usage and cost estimates
