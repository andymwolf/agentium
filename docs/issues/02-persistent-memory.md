# Phase 2: Persistent Memory Across Iterations

## Labels
`enhancement`, `architecture`, `memory`

---

## Problem

Between iterations, the agent's context resets completely. The only continuity is "This is iteration N. Continue from where you left off." This causes agents to:

- Redo work they already completed in prior iterations
- Lose track of which files they modified
- Forget decisions made earlier (e.g., why they chose a particular approach)
- Repeat the same errors without learning from prior failures
- Waste context window re-analyzing code they already understood

## Solution

Create a structured memory layer persisted as a JSON file in the workspace (`.agentium/memory.json`). Before each iteration, the controller builds a concise summary of relevant memory within a character budget and injects it into the agent's prompt. After each iteration, it records the results.

## Memory Data Structure

### Types (`internal/memory/types.go`)

```go
package memory

import "time"

// Data is the top-level structure persisted to disk between iterations
type Data struct {
    SessionID      string                `json:"session_id"`
    Repository     string                `json:"repository"`
    Iteration      int                   `json:"iteration"`
    Tasks          map[string]*TaskMemory `json:"tasks"`
    Decisions      []Decision            `json:"decisions"`
    Errors         []ErrorRecord         `json:"errors"`
    KeyFacts       []string              `json:"key_facts"`
    LastCheckpoint time.Time             `json:"last_checkpoint"`
}

// TaskMemory tracks per-task progress across iterations
type TaskMemory struct {
    ID             string   `json:"id"`
    Type           string   `json:"type"`              // "issue" or "pr"
    Phase          string   `json:"phase"`
    Branch         string   `json:"branch,omitempty"`
    PRNumber       string   `json:"pr_number,omitempty"`
    CompletedSteps []string `json:"completed_steps"`
    PendingSteps   []string `json:"pending_steps,omitempty"`
    BlockedReason  string   `json:"blocked_reason,omitempty"`
    FilesModified  []string `json:"files_modified,omitempty"`
}

// Decision records a significant decision made during execution
type Decision struct {
    Iteration   int    `json:"iteration"`
    TaskID      string `json:"task_id,omitempty"`
    Description string `json:"description"`
    Rationale   string `json:"rationale,omitempty"`
}

// ErrorRecord tracks errors encountered and their resolution
type ErrorRecord struct {
    Iteration  int    `json:"iteration"`
    TaskID     string `json:"task_id,omitempty"`
    Phase      string `json:"phase"`
    Error      string `json:"error"`
    Resolution string `json:"resolution,omitempty"`
    Resolved   bool   `json:"resolved"`
}
```

## Memory Store (`internal/memory/store.go`)

```go
package memory

type StoreConfig struct {
    Enabled       bool   `json:"enabled" yaml:"enabled"`
    FilePath      string `json:"file_path,omitempty" yaml:"file_path,omitempty"`
    MaxEntries    int    `json:"max_entries" yaml:"max_entries"`
    ContextBudget int    `json:"context_budget" yaml:"context_budget"` // max chars for prompt injection
}

type Store struct {
    config StoreConfig
    data   *Data
}

func NewStore(config StoreConfig) (*Store, error)
func (s *Store) Load() (*Data, error)
func (s *Store) Save() error
func (s *Store) Update(iteration int, taskID string, phase string, result IterationOutcome)

type IterationOutcome struct {
    Success       bool
    AgentStatus   string
    StatusMessage string
    FilesModified []string
    Error         string
    PRNumber      string
    Branch        string
}
```

## Context Builder (`internal/memory/context.go`)

The context builder generates a prompt-injectable summary within the character budget:

```go
type ContextBuilder struct {
    Budget int // max characters
}

// BuildContext creates a prompt summary prioritizing:
// 1. Current task state and pending steps (highest priority)
// 2. Unresolved errors from recent iterations
// 3. Recent decisions
// 4. Key facts
func (b *ContextBuilder) BuildContext(data *Data, currentPhase string) string
```

### Example Output (Injected into Prompt)

```
## Session Memory

### Task: Issue #42 (Phase: TEST)
Branch: agentium/issue-42-fix-login
Completed: Created branch, Modified auth/handler.go, Added test case
Pending: Fix failing test at handler_test.go:147, Run full suite, Create PR
Files modified: auth/handler.go, auth/handler_test.go

### Unresolved Errors
- [Iteration 2, TEST] TestTokenExpiry: expected ErrExpired, got nil

### Key Decisions
- Used time.Now() mock instead of real clock for token expiry test

### Key Facts
- Project uses Go 1.19 with standard testing package
- Auth module has no external dependencies
```

## Agent Memory Signals

Agents can write to memory via structured output signals:

```
AGENTIUM_MEMORY: KEY_FACT Project uses pytest for testing
AGENTIUM_MEMORY: DECISION Used factory pattern for the new service
AGENTIUM_MEMORY: STEP_DONE Implemented retry logic in client.go
AGENTIUM_MEMORY: STEP_PENDING Need to update integration tests
AGENTIUM_MEMORY: FILE_MODIFIED src/auth/handler.go
```

The controller's `ParseOutput` detects these and feeds them to the memory store.

## Memory File Lifecycle

1. **Creation**: Controller creates `.agentium/memory.json` after the first iteration (if `memory.enabled`)
2. **Loading**: Before each iteration, the store loads existing memory from disk
3. **Context Building**: `ContextBuilder` summarizes within character budget for prompt injection
4. **Updating**: After each iteration, controller writes results (phase transitions, errors, files)
5. **Agent Writes**: Agent output signals are parsed and recorded
6. **Pruning**: On `Save()`, entries beyond `max_entries` are pruned oldest-first (current task state always retained)
7. **Cleanup**: File is ephemeral - destroyed with the VM after session ends

## Controller Integration

```go
// In runIteration(), before creating session:
if c.memoryStore != nil {
    data, err := c.memoryStore.Load()
    if err == nil {
        builder := &memory.ContextBuilder{Budget: c.config.Memory.ContextBudget}
        iterCtx.MemoryContext = builder.BuildContext(data, string(activePhase))
    }
}

// After getting iteration result:
if c.memoryStore != nil {
    c.memoryStore.Update(c.iteration, activeTaskID, string(activePhase), memory.IterationOutcome{
        Success:       result.Success,
        AgentStatus:   result.AgentStatus,
        StatusMessage: result.StatusMessage,
        Error:         result.Error,
        PRNumber:      extractPR(result),
    })
    c.memoryStore.Save()
}
```

## Adapter Changes

Claude-code adapter injects memory into the user prompt:

```go
func (a *Adapter) BuildPrompt(session *agent.Session, iteration int) string {
    var sb strings.Builder
    // ... existing prompt ...

    if session.IterationContext != nil && session.IterationContext.MemoryContext != "" {
        sb.WriteString("\n## Session Memory\n\n")
        sb.WriteString(session.IterationContext.MemoryContext)
        sb.WriteString("\n\n")
    }

    if iteration > 1 {
        sb.WriteString(fmt.Sprintf("\nThis is iteration %d. Continue from where you left off.\n", iteration))
    }
    return sb.String()
}
```

## Configuration

```yaml
# .agentium.yaml
memory:
  enabled: true
  max_entries: 50           # Max decisions + errors to retain
  context_budget: 3000      # ~750 tokens injected per iteration
```

## Design Decisions

1. **JSON file in workspace** vs alternatives:
   - GitHub issue comments: Too slow (API round-trip each iteration), clutters issues
   - In-memory only: Lost on process crash
   - SQLite: Overkill for this data volume
   - Separate cloud storage: Adds complexity
   - JSON file: Simple, debuggable, automatically cleaned up with VM

2. **Character budget** vs token counting:
   - Token counting requires a tokenizer dependency
   - ~4:1 char-to-token ratio is a practical approximation
   - 3000 chars ~ 750 tokens is a reasonable default

3. **Memory in user prompt** vs system prompt:
   - User prompt keeps the system prompt stable (skills)
   - Memory is iteration-specific context, not persistent instructions
   - Avoids conflicts with phase-aware skill selection

## Implementation Steps

1. Define types in `internal/memory/types.go`
2. Implement `Store` (Load, Save, Update, pruning logic)
3. Implement `ContextBuilder` (budget-aware summarization)
4. Add `AGENTIUM_MEMORY:` signal parsing to claude-code adapter's `ParseOutput`
5. Add `AGENTIUM_MEMORY:` signal parsing to aider adapter's `ParseOutput`
6. Add `StoreConfig` to `SessionConfig`
7. Initialize `memoryStore` in controller's `New()`
8. Load memory and call `BuildContext()` before each iteration
9. Update memory after each iteration
10. Inject memory context in adapter `BuildPrompt()`
11. Ensure `.agentium/` directory is created in workspace
12. Add unit tests for store, context builder, and signal parsing
13. Run `go build ./...` and `go test ./...`

## Acceptance Criteria

- [ ] `internal/memory` package with Store, ContextBuilder types
- [ ] Memory file created at `.agentium/memory.json` after first iteration
- [ ] Memory context injected into agent prompt each iteration
- [ ] Agent `AGENTIUM_MEMORY:` signals parsed and recorded
- [ ] Old entries pruned when `max_entries` exceeded
- [ ] Context stays within `context_budget` character limit
- [ ] Memory disabled by default (opt-in via config)
- [ ] Unit tests pass for all new code
- [ ] `go build ./...` succeeds

## Dependencies

- Benefits from Phase 1 (phase awareness provides `currentPhase` to `BuildContext`)
- Can be built independently if needed (just uses a string phase name)
