# Phase 3: LLM Routing Per Phase

## Labels
`enhancement`, `architecture`, `routing`

---

## Problem

Currently, a single LLM model is used for all phases of task execution. This is suboptimal because:

- **Implementation** benefits from the most capable model (e.g., Opus for complex coding)
- **Testing/debugging** needs good reasoning but not top-tier generation
- **PR creation** is largely boilerplate - a faster/cheaper model suffices
- **Planning/analysis** benefits from strong reasoning but not necessarily the most expensive model
- No way to optimize cost vs quality per phase

## Solution

Add a model routing layer that maps `TaskPhase` to `ModelConfig`. The controller selects the appropriate model each iteration and passes it to the adapter via `IterationContext.ModelOverride`. Adapters pass this to their underlying CLI as a `--model` flag.

## New Package: `internal/routing`

### Types (`internal/routing/types.go`)

```go
package routing

// ModelConfig specifies an LLM model and its parameters
type ModelConfig struct {
    Model       string  `json:"model" yaml:"model"`
    MaxTokens   int     `json:"max_tokens,omitempty" yaml:"max_tokens,omitempty"`
    Temperature float64 `json:"temperature,omitempty" yaml:"temperature,omitempty"`
}

// PhaseRouting maps task phases to model configurations
type PhaseRouting struct {
    Default   ModelConfig            `json:"default" yaml:"default"`
    Overrides map[string]ModelConfig `json:"overrides,omitempty" yaml:"overrides,omitempty"`
}
```

### Router (`internal/routing/router.go`)

```go
package routing

// Router selects the model configuration for a given phase
type Router struct {
    routing *PhaseRouting
}

// NewRouter creates a router from the given configuration.
// If routing is nil, returns a no-op router that always returns empty ModelConfig.
func NewRouter(routing *PhaseRouting) *Router

// ModelForPhase returns the model config for the given phase.
// Falls back to Default if no override exists.
func (r *Router) ModelForPhase(phase string) ModelConfig

// IsConfigured returns true if the router has non-empty routing config.
func (r *Router) IsConfigured() bool
```

## Configuration

```yaml
# .agentium.yaml
routing:
  default:
    model: "claude-sonnet-4-20250514"
    max_tokens: 8192
  overrides:
    IMPLEMENT:
      model: "claude-opus-4-20250514"
      max_tokens: 16384
    TEST:
      model: "claude-sonnet-4-20250514"
      max_tokens: 8192
    PR_CREATION:
      model: "claude-haiku-4-20250514"
      max_tokens: 4096
    ANALYZE:
      model: "claude-sonnet-4-20250514"
      max_tokens: 8192
```

### Example Cost/Quality Strategy

| Phase | Model | Rationale |
|-------|-------|-----------|
| IMPLEMENT | Opus | Best code generation quality |
| TEST | Sonnet | Good reasoning for debugging, lower cost |
| PR_CREATION | Haiku | Boilerplate PR description, minimal cost |
| ANALYZE (review) | Sonnet | Good at understanding review comments |
| PUSH | Haiku | Simple git operations |

## Controller Integration

```go
// In New(), after initialization:
if config.Routing != nil {
    c.modelRouter = routing.NewRouter(config.Routing)
}

// In runIteration(), when building IterationContext:
if c.modelRouter != nil && c.modelRouter.IsConfigured() {
    modelCfg := c.modelRouter.ModelForPhase(string(activePhase))
    iterCtx.ModelOverride = modelCfg.Model
    iterCtx.MaxTokens = modelCfg.MaxTokens
    iterCtx.Temperature = modelCfg.Temperature
    c.logger.Printf("Model for phase %s: %s", activePhase, modelCfg.Model)
}
```

## Adapter Changes

### Claude Code Adapter

```go
func (a *Adapter) BuildCommand(session *agent.Session, iteration int) []string {
    // ... existing code ...

    // Apply model override if set
    if session.IterationContext != nil && session.IterationContext.ModelOverride != "" {
        args = append(args, "--model", session.IterationContext.ModelOverride)
    }

    // Apply max tokens if set
    if session.IterationContext != nil && session.IterationContext.MaxTokens > 0 {
        args = append(args, "--max-tokens", strconv.Itoa(session.IterationContext.MaxTokens))
    }

    // ...
}
```

### Aider Adapter

```go
func (a *Adapter) BuildCommand(session *agent.Session, iteration int) []string {
    // ... existing code ...

    // Apply model override
    model := "claude-3-5-sonnet-20241022" // default
    if session.IterationContext != nil && session.IterationContext.ModelOverride != "" {
        model = session.IterationContext.ModelOverride
    }
    args = append(args, "--model", model)

    // ...
}
```

## CLI Override

Add a `--model` flag to the `run` command for one-off overrides that bypass routing:

```go
// internal/cli/run.go
runCmd.Flags().StringVar(&modelOverride, "model", "", "Override model for all phases")
```

When set, this takes precedence over routing configuration (sets `Default` and clears `Overrides`).

## Config Types

Add to `internal/config/config.go`:

```go
type RoutingConfig struct {
    Default   ModelSpec            `mapstructure:"default"`
    Overrides map[string]ModelSpec `mapstructure:"overrides"`
}

type ModelSpec struct {
    Model       string  `mapstructure:"model"`
    MaxTokens   int     `mapstructure:"max_tokens"`
    Temperature float64 `mapstructure:"temperature"`
}
```

## Design Decisions

1. **Phase-level routing** vs task-level: Phase-level is simpler and covers the primary use case. Sub-agent delegation (Phase 4) provides task-level model control.

2. **Adapter CLI flags** vs API calls: Both claude-code and aider support `--model` flags. This avoids requiring adapters to manage API credentials directly.

3. **Temperature in routing**: Optional but useful - e.g., lower temperature for PR creation (deterministic), higher for implementation (creative solutions).

4. **No per-iteration cost tracking**: Out of scope for this issue. Could be added later by parsing adapter output for token usage.

## Implementation Steps

1. Define `ModelConfig` and `PhaseRouting` in `internal/routing/types.go`
2. Implement `Router` in `internal/routing/router.go`
3. Add `RoutingConfig` to `internal/config/config.go`
4. Add `*PhaseRouting` to controller `SessionConfig`
5. Add `ModelOverride`, `MaxTokens`, `Temperature` to `IterationContext` (if not already done in Phase 1)
6. Initialize router in controller `New()`
7. Call `ModelForPhase()` in `runIteration()`
8. Modify claude-code adapter `BuildCommand` to pass `--model` flag
9. Modify aider adapter `BuildCommand` to use model override
10. Add `--model` CLI flag for one-off override
11. Add unit tests for router
12. Run `go build ./...` and `go test ./...`

## Acceptance Criteria

- [ ] `internal/routing` package with Router type
- [ ] Configuration maps phases to model configs
- [ ] Controller selects model per iteration based on phase
- [ ] Claude-code adapter passes `--model` flag when override set
- [ ] Aider adapter uses model override when set
- [ ] CLI `--model` flag overrides all routing
- [ ] Default fallback when no routing configured
- [ ] Unit tests pass
- [ ] `go build ./...` succeeds

## Dependencies

- Requires Phase 1 (`IterationContext` struct and `determineActivePhase()`)
