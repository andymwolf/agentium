# Agentium Workflow and Phasing System

This document describes the workflow and phasing system used by Agentium to process issues and pull requests.

## Overview

Agentium uses a **sequential phase-based workflow** where tasks progress through predefined phases. Each phase can iterate multiple times before advancing, with a unified Judge determining when to move forward. The workflow supports two paths: **SIMPLE** (streamlined) and **COMPLEX** (with code review).

## Phase Definitions

### Issue Workflow Phases

The primary phases for processing issues (in order):

| Phase | Constant | Purpose | Default Max Iterations |
|-------|----------|---------|----------------------|
| PLAN | `PhasePlan` | Create implementation plan | 3 |
| IMPLEMENT | `PhaseImplement` | Write code, run tests, create draft PR | 5 |
| DOCS | `PhaseDocs` | Update documentation (non-blocking) | 2 |

**Notes:**
- Testing is integrated into the IMPLEMENT phase. There is no separate TEST phase.
- Draft PRs are created during the IMPLEMENT phase (not a separate phase).
- The DOCS phase auto-succeeds after max iterations to avoid blocking PR finalization.
- PRs are finalized (marked as ready for review) when the workflow reaches PhaseComplete.

### Terminal Phases

Tasks end in one of these states:

| Phase | Constant | Meaning |
|-------|----------|---------|
| COMPLETE | `PhaseComplete` | Task finished successfully |
| BLOCKED | `PhaseBlocked` | Encountered unresolvable issue |
| NOTHING_TO_DO | `PhaseNothingToDo` | No work was needed |

### PR-Specific Phases

For pull request review sessions:

| Phase | Constant | Purpose |
|-------|----------|---------|
| ANALYZE | `PhaseAnalyze` | Initial PR analysis |
| PUSH | `PhasePush` | Push changes to PR branch |

## Workflow Paths

### Standard Path

All tasks follow the same streamlined workflow:

```
PLAN → IMPLEMENT (creates draft PR) → DOCS → COMPLETE (finalizes PR)
```

**Draft PR Creation:**
- A draft PR is created during the first IMPLEMENT iteration that has commits to push
- Subsequent IMPLEMENT iterations push to the same branch, automatically updating the PR
- Review feedback can be posted to the draft PR during the IMPLEMENT phase

**PR Finalization:**
- When the workflow reaches PhaseComplete, the draft PR is marked as ready for review
- This happens automatically via `gh pr ready`

**DOCS Phase Behavior:**
- The DOCS phase is non-blocking: it auto-succeeds after max iterations
- Documentation issues should not prevent PR finalization

## Phase Loop Execution

The phase loop is implemented in `internal/controller/phase_loop.go`:

```
┌─────────────────────────────────────────────────────────────┐
│                      Phase Loop                              │
│                                                              │
│  ┌──────────┐    ┌──────────┐    ┌─────────┐                │
│  │  Worker  │───▶│ Reviewer │───▶│  Judge  │                │
│  │  Agent   │    │  Agent   │    │  Agent  │                │
│  └──────────┘    └──────────┘    └─────────┘                │
│                                       │                      │
│                                       ▼                      │
│                              ┌─────────────────┐            │
│                              │    Verdict?     │            │
│                              └─────────────────┘            │
│                                       │                      │
│        ┌──────────────────────────────┼──────────────────┐  │
│        │           │          │       │         │        │  │
│        ▼           ▼          ▼       ▼         ▼        ▼  │
│   ┌─────────┐ ┌─────────┐ ┌───────┐ ┌───────┐ ┌────────┐ ┌─────────┐
│   │ ADVANCE │ │ ITERATE │ │BLOCKED│ │SIMPLE │ │COMPLEX │ │ REGRESS │
│   └─────────┘ └─────────┘ └───────┘ └───────┘ └────────┘ └─────────┘
│        │           │          │       │         │        │  │
│        │           │          │       └────┬────┘        │  │
│        │           │          │            │             │  │
│        ▼           ▼          ▼            ▼             ▼  │
│   Next phase   Same phase   Stop    Set workflow     Return │
│                                       path          to PLAN │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### Iteration Control

- Each phase has a configurable maximum iteration count
- When max iterations reached, phase auto-advances
- `PhaseIteration` tracks current iteration within phase
- Iterations reset when advancing to next phase or regressing

## Judge System

The Judge is the unified decision-maker for all phase transitions. It replaces the previous Evaluator system.

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

### Judge Verdicts

| Verdict | Constant | When Used | Effect |
|---------|----------|-----------|--------|
| ADVANCE | `VerdictAdvance` | All phases | Phase complete, move to next phase |
| ITERATE | `VerdictIterate` | All phases | More work needed, run another iteration |
| BLOCKED | `VerdictBlocked` | All phases | Unresolvable issue, stop task |
| SIMPLE | `VerdictSimple` | PLAN only | Task is simple, skip REVIEW phase |
| COMPLEX | `VerdictComplex` | PLAN only | Task is complex, include REVIEW phase |
| REGRESS | `VerdictRegress` | REVIEW only | Return to PLAN phase with feedback |

### Verdict Signal Format

Agents emit verdicts using this format:

```
AGENTIUM_EVAL: ADVANCE [optional feedback]
AGENTIUM_EVAL: ITERATE More work needed on error handling
AGENTIUM_EVAL: BLOCKED Cannot access required API
AGENTIUM_EVAL: SIMPLE straightforward config change
AGENTIUM_EVAL: COMPLEX multiple files and architectural changes
AGENTIUM_EVAL: REGRESS fundamental design issue needs re-planning
```

### Fail-Safe Behaviors

| Scenario | Behavior |
|----------|----------|
| Judge produces no signal | Defaults to ITERATE (fail-closed) |
| Judge no-signal for N consecutive iterations | Force ADVANCE (prevents infinite loops) |

The `evalNoSignalLimit` config (default: 2) controls consecutive no-signal iterations before force-advancing.

## Phase Regression

When the Judge issues a REGRESS verdict during REVIEW:

1. **Iteration count resets** - Fresh start for planning
2. **Review feedback preserved** - Context for what went wrong
3. **Return to PLAN phase** - Re-plan with new context
4. **Complexity re-assessed** - Fresh SIMPLE/COMPLEX decision

This allows the workflow to recover from fundamental design issues discovered during code review.

## Memory System

The memory system persists context across iterations and phases. Implemented in `internal/memory/`.

### Memory Behavior on Phase Transitions

| Event | Memory Action |
|-------|---------------|
| ITERATE within phase | Keep all memory |
| ADVANCE to next phase | Clear phase-specific, keep KEY_FACTs |
| REGRESS from REVIEW→PLAN | Keep review feedback for context |

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
    ID               string      // Issue/PR number
    Type             string      // "issue" or "pr"
    Phase            TaskPhase   // Current phase
    TestRetries      int         // Count of test failures
    LastStatus       string      // Last agent status signal
    PRNumber         string      // Linked PR for issues
    PhaseIteration   int         // Current iteration within phase
    MaxPhaseIter     int         // Max iterations for current phase
    LastJudgeVerdict string      // "ADVANCE", "ITERATE", "BLOCKED"
    LastJudgeFeedback string     // Judge's feedback text
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
- `PLAN`, `IMPLEMENT`, `REVIEW`, `DOCS`, `PR_CREATION`
- `ANALYZE`, `PUSH`
- `COMPLETE`, `BLOCKED`, `NOTHING_TO_DO`

Reviewer phases:
- `PLAN_REVIEW`, `IMPLEMENT_REVIEW`, `REVIEW_REVIEW`, `DOCS_REVIEW`

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

### Standard Workflow (Issue #42)

```
Task: issue:42
Initial Phase: PLAN

=== PLAN Phase ===
Iteration 1/3:
  → Worker agent creates plan
  → Plan Reviewer provides feedback
  → Judge verdict: ADVANCE

=== IMPLEMENT Phase ===
Iteration 1/5:
  → Worker implements changes and runs tests
  → Code Reviewer provides feedback
  → Controller creates draft PR #100
  → Judge verdict: ADVANCE

=== DOCS Phase ===
Iteration 1/2:
  → Worker updates documentation
  → Judge verdict: ADVANCE

=== PhaseComplete ===
  → Controller finalizes draft PR #100 (marks as ready for review)

Final Phase: COMPLETE
```

### Workflow with Multiple IMPLEMENT Iterations (Issue #99)

```
Task: issue:99
Initial Phase: PLAN

=== PLAN Phase ===
Iteration 1/3:
  → Worker creates comprehensive plan
  → Plan Reviewer provides feedback
  → Judge verdict: ADVANCE

=== IMPLEMENT Phase ===
Iteration 1/5:
  → Worker implements changes
  → Controller creates draft PR #150
  → Code Reviewer: needs error handling
  → Judge verdict: ITERATE

Iteration 2/5:
  → Worker adds error handling
  → Pushes to existing branch (PR #150 auto-updates)
  → Judge verdict: ITERATE (tests failing)

Iteration 3/5:
  → Worker fixes test failures
  → Pushes to existing branch
  → Judge verdict: ADVANCE

=== DOCS Phase ===
Iteration 1/2:
  → Worker updates documentation
  → Judge verdict: ADVANCE

=== PhaseComplete ===
  → Controller finalizes draft PR #150 (marks as ready for review)

Final Phase: COMPLETE
```

### Workflow with DOCS Auto-Advance (Issue #77)

```
Task: issue:77
Initial Phase: PLAN

=== PLAN Phase ===
Iteration 1/3:
  → Worker creates plan
  → Judge verdict: ADVANCE

=== IMPLEMENT Phase ===
Iteration 1/5:
  → Worker implements feature
  → Controller creates draft PR #200
  → Judge verdict: ADVANCE

=== DOCS Phase ===
Iteration 1/2:
  → Worker attempts to update docs
  → Judge verdict: ITERATE

Iteration 2/2:
  → Worker updates docs but reviewer has concerns
  → Max iterations reached, auto-advancing (non-blocking)

=== PhaseComplete ===
  → Controller finalizes draft PR #200

Final Phase: COMPLETE
```
