# Phase 1: Phase-Aware Skill Selection

## Labels
`enhancement`, `architecture`, `skills`

---

## Problem

The current system injects the entire monolithic `prompts/SYSTEM.md` (~300+ lines) into every agent iteration regardless of which phase the task is in. During the TEST phase, the agent receives instructions for PR creation, planning, and review that are irrelevant. This wastes context window space and dilutes focus.

## Solution

Decompose `SYSTEM.md` into modular skill files, each tagged with the phases where it applies. A new `internal/skills` package loads and composes only relevant skills per iteration based on the current `TaskPhase`.

## Skill Decomposition

Current `SYSTEM.md` sections map to skill files:

| SYSTEM.md Section | Skill File | Phases |
|---|---|---|
| CRITICAL SAFETY CONSTRAINTS | `safety.md` | All |
| ENVIRONMENT | `environment.md` | All |
| STATUS SIGNALING | `status_signals.md` | All |
| Step 1-2 (Understand, Plan) | `planning.md` | IMPLEMENT, ANALYZE |
| Step 3-5 (Branch, Implement, Loop) | `implement.md` | IMPLEMENT |
| Step 5a-5d (Test loop) | `test.md` | TEST, IMPLEMENT |
| Step 6-7 (Push, Create PR) | `pr_creation.md` | PR_CREATION |
| PR REVIEW SESSIONS | `pr_review.md` | ANALYZE, PUSH |

During the TEST phase, the agent receives ~120 lines of focused prompt instead of ~300 lines.

## New File Structure

```
prompts/skills/
├── manifest.yaml         # Metadata: which skills apply to which phases
├── safety.md             # Always loaded (branch protection, no secrets, etc.)
├── environment.md        # Always loaded (workspace, tools, iteration info)
├── status_signals.md     # Always loaded (AGENTIUM_STATUS protocol)
├── planning.md           # IMPLEMENT, ANALYZE phases
├── implement.md          # IMPLEMENT phase
├── test.md               # TEST, IMPLEMENT phases
├── pr_creation.md        # PR_CREATION phase
└── pr_review.md          # ANALYZE, PUSH phases
```

## Manifest Format

```yaml
# prompts/skills/manifest.yaml
version: "1"
skills:
  - name: safety
    file: safety.md
    priority: 10
    phases: []  # Empty = all phases

  - name: environment
    file: environment.md
    priority: 20
    phases: []

  - name: status_signals
    file: status_signals.md
    priority: 90
    phases: []

  - name: planning
    file: planning.md
    priority: 30
    phases: ["IMPLEMENT", "ANALYZE"]

  - name: implement
    file: implement.md
    priority: 40
    phases: ["IMPLEMENT"]

  - name: test
    file: test.md
    priority: 50
    phases: ["TEST", "IMPLEMENT"]

  - name: pr_creation
    file: pr_creation.md
    priority: 60
    phases: ["PR_CREATION"]

  - name: pr_review
    file: pr_review.md
    priority: 70
    phases: ["ANALYZE", "PUSH"]
```

## New Package: `internal/skills`

### Types (`internal/skills/types.go`)

```go
package skills

// Skill represents a single, focused prompt module
type Skill struct {
    Name     string   `yaml:"name"`
    File     string   `yaml:"file"`
    Priority int      `yaml:"priority"`
    Phases   []string `yaml:"phases"`   // Empty = all phases
    Content  string   `yaml:"-"`        // Loaded content (not serialized)
}

// Manifest defines the available skills and their phase mappings
type Manifest struct {
    Version string  `yaml:"version"`
    Skills  []Skill `yaml:"skills"`
}
```

### Loader (`internal/skills/loader.go`)

```go
package skills

import "time"

type SourceType string

const (
    SourceEmbedded SourceType = "embedded"
    SourceRemote   SourceType = "remote"
    SourceLocal    SourceType = "local"
)

type LoaderConfig struct {
    Source       SourceType
    RemoteURL    string
    LocalPath    string
    FetchTimeout time.Duration
}

type Loader struct {
    config LoaderConfig
}

func NewLoader(config LoaderConfig) *Loader
func (l *Loader) Load() (*Selector, error)
```

### Selector (`internal/skills/selector.go`)

```go
package skills

// Selector composes system prompts from phase-relevant skills
type Selector struct {
    manifest *Manifest
    loaded   map[string]*Skill
}

// SelectForPhase returns composed prompt for a given TaskPhase.
// Skills with empty Phases list are always included.
// Skills are ordered by Priority (lower first).
func (s *Selector) SelectForPhase(phase string) string

// SelectAll returns all skills composed (backward compat with monolithic mode).
func (s *Selector) SelectAll() string

// SkillNames returns skill names that would be selected for a phase.
func (s *Selector) SkillNames(phase string) []string
```

## Agent Interface Changes

Add `IterationContext` to `internal/agent/interface.go`:

```go
type IterationContext struct {
    Phase         string
    Iteration     int
    SkillsPrompt  string   // Composed from phase-relevant skills
    MemoryContext string   // (Phase 2)
    ModelOverride string   // (Phase 3)
    MaxTokens     int      // (Phase 3)
    Temperature   float64  // (Phase 3)
    SubTaskID     string   // (Phase 4)
}

type Session struct {
    // ... existing fields unchanged ...
    SystemPrompt     string            // DEPRECATED: kept for backward compat
    IterationContext *IterationContext  // NEW: nil = legacy mode
}
```

## Controller Changes

In `runIteration()`, build `IterationContext` before creating the session:

```go
func (c *Controller) runIteration(ctx context.Context) (*agent.IterationResult, error) {
    activePhase := c.determineActivePhase()

    var iterCtx *agent.IterationContext
    if c.useSkills && c.skillSelector != nil {
        iterCtx = &agent.IterationContext{
            Phase:        string(activePhase),
            Iteration:    c.iteration,
            SkillsPrompt: c.skillSelector.SelectForPhase(activePhase),
        }
        c.logger.Printf("Skills for phase %s: %v", activePhase, c.skillSelector.SkillNames(activePhase))
    }

    session := &agent.Session{
        // ... existing fields ...
        SystemPrompt:     c.systemPrompt,  // Fallback
        IterationContext: iterCtx,
    }
    // ...
}

func (c *Controller) determineActivePhase() TaskPhase {
    for _, state := range c.taskStates {
        switch state.Phase {
        case PhaseComplete, PhaseNothingToDo, PhaseBlocked:
            continue
        default:
            return state.Phase
        }
    }
    return PhaseImplement
}
```

## Adapter Changes

Claude Code adapter (`internal/agent/claudecode/adapter.go`) prefers `SkillsPrompt`:

```go
func (a *Adapter) BuildCommand(session *agent.Session, iteration int) []string {
    // ...
    systemPrompt := session.SystemPrompt
    if session.IterationContext != nil && session.IterationContext.SkillsPrompt != "" {
        systemPrompt = session.IterationContext.SkillsPrompt
    }
    if systemPrompt != "" {
        args = append(args, "--system-prompt", systemPrompt)
    }
    // ...
}
```

## Configuration

```yaml
# .agentium.yaml
skills:
  source: "embedded"          # "embedded" | "remote" | "local"
  # url: "https://..."       # only if source=remote
  # path: "./custom-skills"  # only if source=local
  # fetch_timeout: "5s"      # only if source=remote
```

SessionConfig addition:

```go
type SkillsConfig struct {
    Source       string `json:"source,omitempty"`
    RemoteURL    string `json:"remote_url,omitempty"`
    LocalPath    string `json:"local_path,omitempty"`
    FetchTimeout string `json:"fetch_timeout,omitempty"`
}
```

## Backward Compatibility

- When `skills.source` is not configured, the existing monolithic `SYSTEM.md` behavior is preserved
- `Session.SystemPrompt` remains populated as fallback
- Adapters that don't check `IterationContext` continue to work unchanged

## Implementation Steps

1. Create `prompts/skills/manifest.yaml` and individual skill `.md` files by decomposing `prompts/SYSTEM.md`
2. Add `//go:embed` directives for skill files in `internal/skills/loader.go`
3. Implement `Manifest` parsing, `Skill` loading, and `Selector` composition logic
4. Add `SkillsConfig` to `SessionConfig`
5. Add `IterationContext` struct to `internal/agent/interface.go`
6. Add `determineActivePhase()` to controller
7. Call `skillSelector.SelectForPhase()` in the iteration loop
8. Modify claude-code adapter to prefer `IterationContext.SkillsPrompt`
9. Modify aider adapter similarly
10. Retain fallback to monolithic `SystemPrompt` when skills config is absent
11. Add unit tests for loader, selector, and phase mapping
12. Run `go build ./...` and `go test ./...`

## Acceptance Criteria

- [ ] Skills manifest and files exist in `prompts/skills/`
- [ ] `internal/skills` package loads and composes skills by phase
- [ ] Controller passes phase-aware skills via `IterationContext`
- [ ] Claude-code adapter uses `SkillsPrompt` when available
- [ ] Aider adapter uses `SkillsPrompt` when available
- [ ] Monolithic fallback works when skills config is absent
- [ ] Unit tests pass for all new code
- [ ] `go build ./...` succeeds
