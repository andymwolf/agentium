# Agentium Workflow and Phasing System

This document describes the workflow and phasing system used by Agentium to process issues and pull requests.

## Overview

Agentium uses a **sequential phase-based workflow** where tasks progress through predefined phases. Each phase can iterate multiple times before advancing, with a unified Judge determining when to move forward. The workflow supports two paths: **SIMPLE** (streamlined) and **COMPLEX** (with code review).

## Phase Definitions

### Issue Workflow Phases

The primary phases for processing issues (in order):

| Phase | Constant | Purpose | 
|-------|----------|---------|
| PLAN | `PhasePlan` | Create implementation plan | 
| IMPLEMENT | `PhaseImplement` | Write code, run tests, create draft PR | 
| DOCS | `PhaseDocs` | Update documentation (non-blocking) | 

**Notes:**
- Testing is integrated into the IMPLEMENT phase.
- Draft PRs are created during the IMPLEMENT phase.
- All phases skip the Reviewer Agent and ADVANCE when they hit max iterations
- PRs are finalized (marked as ready for review) when the workflow reaches PhaseComplete.

### Terminal Phases

Tasks end in one of these states:

| Phase | Constant | Meaning |
|-------|----------|---------|
| COMPLETE | `PhaseComplete` | Task finished successfully |
| BLOCKED | `PhaseBlocked` | Encountered unresolvable issue |
| NOTHING_TO_DO | `PhaseNothingToDo` | No work was needed |

## Workflow Paths

### Universal Workflow

Both paths follow the basic workflow:
```
PLAN → IMPLEMENT (creates draft PR) → DOCS → COMPLETE (finalizes PR)
```

**Draft PR Creation:**
- A draft PR is created during the first IMPLEMENT iteration that has commits to push
- Subsequent IMPLEMENT iterations push to the same branch, automatically updating the PR
- Implemenation and Docs review feedback is posted to the draft PR

**PR Finalization:**
- When the workflow reaches PhaseComplete, the draft PR is marked as ready for review via `gh pr ready`


### Path Choice

Both paths follow the basic workflow:
```
PLAN → IMPLEMENT (creates draft PR) → DOCS → COMPLETE (finalizes PR)
```
After Plan Iteration 1, the Judge Agent determines if the plan is SIMPLE or COMPLEX.


### Simple Path

The SIMPLE path maintains context and minimizes review. 

| Phase | SIMPLE Max Iterations |
|-------|----------|---------|----------------------|
| PLAN | 1 |
| IMPLEMENT | 2 |
| DOCS | 1 |

**Notes:**
- SIMPLE plans always ADVANCE to Implementation without further plan review
- The same Implementation Worker Agent should be used for all Implement and Docs phases to save context


### Complex Path

The COMPLEX path refreshes context and conducts additional review. 

| Phase | SIMPLE Max Iterations |
|-------|----------|---------|----------------------|
| PLAN | 3 |
| IMPLEMENT | 5 |
| DOCS | 3 |

**Notes:**
- New Implementation Worker Agents are used for each iteration

## Phase Loop Execution

The phase loop is implemented in `internal/controller/phase_loop.go`:

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           Phase Loop                                     │
│                                                                          │
│  ┌──────────┐                                                           │
│  │  Worker  │                                                           │
│  │  Agent   │                                                           │
│  └────┬─────┘                                                           │
│       │                                                                  │
│       ▼                                                                  │
│  ┌─────────────────────────────────────────┐                            │
│  │  PLAN iteration 1 & path not set?       │                            │
│  └─────────────────────────────────────────┘                            │
│       │                                                                  │
│       ├── Yes ──▶ ┌─────────────┐    ┌───────────────────────────┐      │
│       │           │ Complexity  │───▶│ SIMPLE: auto-advance,     │      │
│       │           │ Assessor    │    │         skip reviewer     │      │
│       │           └─────────────┘    │ COMPLEX: continue below   │      │
│       │                              └───────────────────────────┘      │
│       │                                                                  │
│       ▼                                                                  │
│  ┌──────────┐    ┌─────────┐                                            │
│  │ Reviewer │───▶│  Judge  │                                            │
│  │  Agent   │    │  Agent  │                                            │
│  └──────────┘    └────┬────┘                                            │
│                       │                                                  │
│                       ▼                                                  │
│              ┌─────────────────┐                                        │
│              │    Verdict?     │                                        │
│              └─────────────────┘                                        │
│                       │                                                  │
│        ┌──────────────┼──────────────┐                                  │
│        │              │              │                                  │
│        ▼              ▼              ▼                                  │
│   ┌─────────┐   ┌─────────┐   ┌───────┐                                 │
│   │ ADVANCE │   │ ITERATE │   │BLOCKED│                                 │
│   └────┬────┘   └────┬────┘   └───┬───┘                                 │
│        │              │           │                                      │
│        ▼              ▼           ▼                                      │
│   Next phase    Same phase      Stop                                     │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

### Iteration Control

- Each phase has a configurable maximum iteration count
- When max iterations for a phase is reached Reviewer Agent is skipped and Judge always answers ADVANCE
- `PhaseIteration` tracks current iteration within phase
- Iterations reset when advancing to next phase or regressing

## Judge System

The Judge is the unified decision-maker for all phase transitions. 

### Three-Agent Loop

Each phase iteration follows this pattern:

1. **Worker Agent** - Executes the phase work
2. **Reviewer Agent** - Provides constructive feedback (no verdict)
3. **Judge Agent** - Interprets feedback and decides verdict

### Reviewer Skills

Different reviewers are used for different phases:

| Skill | Phases | Purpose |
|-------|--------|---------|
| `plan_reviewer` | PLAN_REVIEW | Reviews implementation plans |
| `code_reviewer` | IMPLEMENT_REVIEW, REVIEW_REVIEW, DOCS_REVIEW | Reviews code changes |

### Complexity Assessor

After PLAN iteration 1, a **Complexity Assessor** (not the Judge) determines the workflow path:

| Verdict | When Used | Effect |
|---------|-----------|--------|
| SIMPLE | PLAN iteration 1 only | Auto-advance, skip reviewer, use reduced iteration limits |
| COMPLEX | PLAN iteration 1 only | Continue to reviewer/judge, use standard iteration limits |

The Complexity Assessor emits verdicts using `AGENTIUM_EVAL: SIMPLE` or `AGENTIUM_EVAL: COMPLEX`.

### Judge Verdicts

| Verdict | Constant | When Used | Effect |
|---------|----------|-----------|--------|
| ADVANCE | `VerdictAdvance` | All phases | Phase complete, move to next phase |
| ITERATE | `VerdictIterate` | All phases | More work needed, run another iteration |
| BLOCKED | `VerdictBlocked` | All phases | Unresolvable issue, stop task |

### Verdict Signal Format

Agents emit verdicts using this format:

```
AGENTIUM_EVAL: ADVANCE [optional feedback]
AGENTIUM_EVAL: ITERATE More work needed on error handling
AGENTIUM_EVAL: BLOCKED Cannot access required API
```

### NOMERGE Behavior

NOMERGE is **not a verdict** but a controller behavior. When the controller forces ADVANCE at max iterations (because the Judge kept returning ITERATE), the `ControllerOverrode` flag is set. At PR finalization:

- If `ControllerOverrode` is true, the PR remains as a draft
- A NOMERGE comment is posted explaining human review is required
- The PR is NOT marked as ready for review

### Fail-Safe Behaviors

| Scenario | Behavior |
|----------|----------|
| Judge produces no signal | Mark as BLOCKED |

The `evalNoSignalLimit` config (default: 2) controls consecutive no-signal iterations before force-advancing.

## Memory System

The memory system persists context across iterations and phases. Implemented in `internal/memory/`.

### Memory Behavior on Phase Transitions

| Event | Memory Action |
|-------|---------------|
| ITERATE within phase | Keep all memory |
| ADVANCE to next phase | Clear phase-specific, keep KEY_FACTs |

### Signal Types

| Signal | Purpose |
|--------|---------|
| `KEY_FACT` | Important information to remember |
| `DECISION` | Architectural or implementation decisions |
| `STEP_DONE` | Step completed (resolves matching STEP_PENDING) |
| `STEP_PENDING` | Upcoming work to be done |
| `FILE_MODIFIED` | Tracks file changes |
| `ERROR` | Error log entries |
| `EVAL_FEEDBACK` | Judge feedback (stored on ITERATE/REGRESS) |
| `PHASE_RESULT` | Phase completion summary |

### Signal Format

```
AGENTIUM_MEMORY: KEY_FACT The API uses OAuth2 for authentication
AGENTIUM_MEMORY: DECISION Using PostgreSQL for the database
AGENTIUM_MEMORY: STEP_DONE Implemented user login endpoint
AGENTIUM_MEMORY: STEP_PENDING Add rate limiting to API
```

## Task State

The `TaskState` struct tracks per-task metadata:

```go
type TaskState struct {
    ID                 string       // Issue/PR number
    Type               string       // "issue" or "pr"
    Phase              TaskPhase    // Current phase
    TestRetries        int          // Count of test failures
    LastStatus         string       // Last agent status signal
    PRNumber           string       // Linked PR for issues
    PhaseIteration     int          // Current iteration within phase
    MaxPhaseIterations int          // Max iterations for current phase
    LastJudgeVerdict   string       // "ADVANCE", "ITERATE", "BLOCKED"
    LastJudgeFeedback  string       // Judge's feedback text
    DraftPRCreated     bool         // Whether draft PR has been created
    WorkflowPath       WorkflowPath // SIMPLE or COMPLEX (set after PLAN iteration 1)
    ControllerOverrode bool         // True if controller forced ADVANCE (triggers NOMERGE)
}
```

## Model and Adapter Routing

Implemented in `internal/routing/`, routing allows different phases to use different adapters and models.

### Phase Routing Keys

The routing system supports compound keys with fallback chains:

```
PLAN_JUDGE → JUDGE → default
IMPLEMENT_REVIEW → REVIEW → default
```

### Valid Phase Keys

Base phases:
- `PLAN`, `IMPLEMENT`, `REVIEW`, `DOCS`
- `COMPLETE`, `BLOCKED`, `NOTHING_TO_DO`

Reviewer phases:
- `PLAN_REVIEW`, `IMPLEMENT_REVIEW`, `DOCS_REVIEW`

Judge phases:
- `JUDGE`, `PLAN_JUDGE`, `IMPLEMENT_JUDGE`, `REVIEW_JUDGE`, `DOCS_JUDGE`

### Example Configuration

```yaml
routing:
  default:
    adapter: "claude-code"
    model: "claude-opus-4-20250514"
  overrides:
    PLAN:
      model: "claude-opus-4-20250514"
    IMPLEMENT:
      model: "claude-opus-4-20250514"
    REVIEW:
      adapter: "codex"
      model: "gpt-5.2"
    PLAN_REVIEW:
      adapter: "codex"
      model: "gpt-5.2"
    JUDGE:
      model: "claude-opus-4-20250514"
```

## Key Source Files

| File | Purpose |
|------|---------|
| `internal/controller/controller.go` | Main controller, session lifecycle, task queue |
| `internal/controller/phase_loop.go` | Phase loop execution, iteration control |
| `internal/controller/judge.go` | Judge agent, verdict parsing, feedback management |
| `internal/controller/reviewer.go` | Reviewer agent for three-agent loop |
| `internal/controller/delegation.go` | Delegated iteration execution |
| `internal/controller/orchestrator.go` | Sub-task orchestration mapping |
| `internal/controller/docker.go` | Container execution, memory signal processing |
| `internal/routing/` | Model and adapter routing configuration |
| `internal/memory/` | Memory persistence and signal management |

## Example Workflows

### Standard Simple Workflow (Issue #22)

```
Task: issue:22
Initial Phase: PLAN

=== PLAN Phase ===
Iteration 1:
  → Worker agent creates plan
  → Judge verdict: SIMPLE -> ADVANCE

=== IMPLEMENT Phase ===
Iteration 1/2:
  → Worker implements changes and runs tests
  → Worker creates draft PR #100
  → Code Reviewer provides feedback
  → Judge verdict: ITERATE

Iteration 2/2:
  → Worker implements changes and runs tests
  → Worker updates draft PR #100
  → Code Reviewer provides feedback
  → Judge verdict: ITERATE
  → Controller overrides: ADVANCE

=== DOCS Phase ===
Iteration 1/1:
  → Worker updates documentation
  → Docs Reviewer provides feedback
  → Judge verdict: ADVANCE

=== PhaseComplete ===
  → Controller finalizes draft PR #100 (marks as ready for review)

Final Phase: COMPLETE
```

### Standard Complex Workflow (Issue #42)

```
Task: issue:42
Initial Phase: PLAN

=== PLAN Phase ===
Iteration 1/3:
  → Worker agent creates plan
  → Judge verdict: COMPLEX
  → Plan Reviewer provides feedback
  → Judge verdict: ADVANCE

=== IMPLEMENT Phase ===
Iteration 1/5:
  → Worker implements changes and runs tests
  → Worker creates draft PR #101
  → Code Reviewer provides feedback
  → Judge verdict: ADVANCE

=== DOCS Phase ===
Iteration 1/3:
  → Worker updates documentation
  → Docs Reviewer provides feedback
  → Judge verdict: ADVANCE

=== PhaseComplete ===
  → Controller finalizes draft PR #101 (marks as ready for review)

Final Phase: COMPLETE
```

### Workflow with Multiple Iterations (Issue #99)

```
Task: issue:99
Initial Phase: PLAN

=== PLAN Phase ===
Iteration 1/3:
  → Worker creates comprehensive plan
  → Judge verdict: COMPLEX
  → Plan Reviewer provides feedback
  → Judge verdict: ITERATE

Iteration 2/3:
  → Worker updates plan
  → Plan Reviewer provides feedback
  → Judge verdict: ADVANCE

=== IMPLEMENT Phase ===
Iteration 1/5:
  → Worker implements Plan
  → Worker creates draft PR #150
  → Code Reviewer: needs error handling
  → Judge verdict: ITERATE

Iteration 2/5:
  → Worker adds error handling
  → Pushes to existing branch (PR #150 auto-updates)
  → Code Reviewer: error handling still insufficient
  → Judge verdict: ITERATE (tests failing)

Iteration 3/5:
  → Worker fixes test failures
  → Pushes to existing branch
  → Code Reviewer: provides insignificant feedback
  → Judge verdict: ADVANCE

=== DOCS Phase ===
Iteration 1/2:
  → Worker updates documentation
  → Docs Reviewer: provides insignificant feedback
  → Judge verdict: ADVANCE

=== PhaseComplete ===
  → Controller finalizes draft PR #150 (marks as ready for review)

Final Phase: COMPLETE
```

### Workflow with Auto-Advance (Issue #77)

```
Task: issue:77
Initial Phase: PLAN

=== PLAN Phase ===
Iteration 1/3:
  → Worker creates comprehensive plan
  → Judge verdict: COMPLEX
  → Plan Reviewer provides feedback
  → Judge verdict: ITERATE

Iteration 2/3:
  → Worker updates plan
  → Plan Reviewer provides feedback
  → Judge verdict: ITERATE

Iteration 3/3:
  → Worker updates plan
  → Plan Reviewer provides feedback
  → Judge verdict: ITERATE
  → Controller hits max iterations: ADVANCE

=== IMPLEMENT Phase ===
Iteration 1/5:
  → Worker implements Plan
  → Worker creates draft PR #175
  → Code Reviewer: provides insignificant feedback
  → Judge verdict: ADVANCE

Iteration 2/5:
  → Worker adds error handling
  → Pushes to existing branch (PR #175 auto-updates)
  → Code Reviewer: error handling still insufficient
  → Judge verdict: ITERATE (tests failing)

Iteration 3/5:
  → Worker fixes test failures
  → Pushes to existing branch
  → Code Reviewer: provides insignificant feedback
  → Judge verdict: ADVANCE

=== DOCS Phase ===
Iteration 1/2:
  → Worker updates documentation
  → Docs Reviewer: provides insignificant feedback
  → Judge verdict: ADVANCE

=== PhaseComplete ===
  → Controller overrode the Judge in at least one phase. Comments NOMERGE on draft PR #175. PR is not finalized.

Final Phase: COMPLETE
```
