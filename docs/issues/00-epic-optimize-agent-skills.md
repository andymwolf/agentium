# Epic: Optimize Agent Skills and Sub-Agent Architecture

## Labels
`enhancement`, `architecture`, `epic`

---

## Overview

This epic tracks the implementation of four interconnected improvements to how agentium manages agent skills, execution context, and task orchestration. Together, they address:

- **Context window waste**: Monolithic `SYSTEM.md` injected every iteration regardless of phase
- **No task specialization**: Single agent handles entire lifecycle with all instructions
- **No cost/quality optimization**: Same LLM model used for all phases regardless of complexity
- **Context amnesia**: Between iterations, the agent starts from scratch with no memory of prior work

## Architecture

The four features layer on each other:

```
         Sub-Agent Delegation (Phase 4)
                    |
         +----------+----------+
         |                     |
   LLM Routing (3)    Persistent Memory (2)
         |                     |
         +----------+----------+
                    |
       Phase-Aware Skills (Phase 1)
                    |
          Existing Controller Loop
```

## Core Abstraction: IterationContext

All four features communicate through a new `IterationContext` struct on `agent.Session`:

```go
type IterationContext struct {
    Phase         string   // Current TaskPhase
    Iteration     int
    SkillsPrompt  string   // Composed from phase-relevant skills
    MemoryContext string   // Summarized memory for prompt injection
    ModelOverride string   // Model to use this iteration
    MaxTokens     int
    Temperature   float64
    SubTaskID     string   // If running as delegated sub-task
}
```

Adapters check `session.IterationContext != nil` and use the phase-aware fields. When nil, existing monolithic behavior is preserved (full backward compatibility).

## Configuration

All features are opt-in via `.agentium.yaml`:

```yaml
skills:
  source: "embedded"

routing:
  default:
    model: "claude-sonnet-4-20250514"
  overrides:
    IMPLEMENT:
      model: "claude-opus-4-20250514"

memory:
  enabled: true
  max_entries: 50
  context_budget: 3000

delegation:
  enabled: false
  strategy: "sequential"
  sub_agents:
    implement:
      model: { model: "claude-opus-4-20250514" }
```

## Implementation Order

| Phase | Feature | Dependencies |
|-------|---------|--------------|
| 1 | Phase-Aware Skill Selection | None |
| 2 | Persistent Memory | Benefits from Phase 1 |
| 3 | LLM Routing Per Phase | Requires Phase 1 |
| 4 | Sub-Agent Delegation | Requires 1+2+3 |

## Feature Interactions

| A -> B | Interaction |
|--------|-------------|
| Skills -> Memory | Phase determines which memory to prioritize |
| Skills -> Routing | Same `determineActivePhase()` drives both |
| Skills -> Delegation | Sub-agents can specify custom skill lists |
| Memory -> Delegation | Sub-agents share the memory store |
| Routing -> Delegation | Sub-agents can override the phase model |

## Key Design Decisions

1. **IterationContext as optional pointer** - nil means legacy mode, no adapter changes required
2. **Skills as `//go:embed` files** - Binary is self-contained; remote/local are opt-in alternatives
3. **Memory as JSON in workspace** - Simple, debuggable, ephemeral with VM lifecycle
4. **Character budget for memory** - Practical ~4:1 char-to-token approximation avoids tokenizer dependency
5. **Sub-agents reuse Docker execution** - No new execution model, just different agent/skill/model configs
6. **Model routing via adapter CLI flags** - Both claude-code and aider support `--model`

## New Package Structure

```
internal/
├── skills/               # Phase-aware skill decomposition
│   ├── types.go
│   ├── loader.go
│   ├── selector.go
│   └── loader_test.go
├── memory/               # Persistent iteration memory
│   ├── types.go
│   ├── store.go
│   ├── context.go
│   └── store_test.go
├── routing/              # LLM model routing per phase
│   ├── types.go
│   ├── router.go
│   └── router_test.go
├── controller/           # MODIFIED
│   ├── controller.go
│   └── subtask.go        # Sub-task orchestration
├── agent/                # MODIFIED (Session struct extended)
│   └── interface.go
└── config/               # MODIFIED
    └── config.go
prompts/
├── SYSTEM.md             # Retained for backward compatibility
└── skills/               # Decomposed skill files
    ├── manifest.yaml
    ├── safety.md
    ├── environment.md
    ├── status_signals.md
    ├── implement.md
    ├── test.md
    ├── pr_creation.md
    ├── pr_review.md
    └── planning.md
```

## Related Issues

- Phase 1: Phase-Aware Skill Selection
- Phase 2: Persistent Memory
- Phase 3: LLM Routing Per Phase
- Phase 4: Sub-Agent Delegation
