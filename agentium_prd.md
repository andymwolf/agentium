# PRD: Agentium — Ephemeral, Safe Execution of AI Coding Agents

## 1. Purpose & Goals

### Objective

Design and implement a **repeatable, low-touch cloud execution framework** for running AI coding agents safely and cheaply, where:

- Agents run in **ephemeral or session-scoped execution environments**
- Agent execution is **containerized for portability and determinism**
- Human time is spent **authoring prompts and reviewing PRs**, not managing infrastructure
- Agent environments have **minimal privileges** and **no production secrets**
- Agent execution lifetime is **prompt-configurable**
- The system is **largely automated and reusable across many repositories**

This PRD explicitly covers **agent execution infrastructure only**. Application hosting and runtime concerns are specified in a separate PRD.

---

## 2. Non-Goals

- Supporting high-traffic or latency-critical workloads
- Supporting always-on agent compute
- Allowing agents to deploy directly to production
- Operating long-lived development VMs or clusters
- Introducing container orchestration platforms (e.g., Kubernetes)

---

## 3. Target Users

- A single technical operator (human) comfortable with CLI-based agentic tools
- Autonomous or semi-autonomous AI coding agents
- GitHub Actions (as the trusted deployment authority)

---

## 4. High-Level Architecture

### Planes of Responsibility

**Agentium (This PRD)**

- Ephemeral VM launched per **agent session**
- VM provides security boundary, identity, and billing scope
- VM runs one or more **agent runtime containers**
- A **session controller** orchestrates a **phase loop** (plan, implement, test, review) with LLM evaluation between phases
- Containers execute tasks **sequentially** within the session
- VM terminates deterministically based on configured conditions

Agents never hold production credentials.

**Application Runtime (Out of Scope)**

- Hosting, deployment, and operation of applications produced by agents
- Defined in a separate Application Runtime Platform PRD.

### Bootstrap System (Phase 0)

For users who want to run agent sessions without building the Go CLI, a standalone bootstrap system exists:

- `bootstrap/run.sh` - Local launcher script
- `bootstrap/main.tf` - GCP Terraform configuration
- `bootstrap/cloud-init.yaml` - VM initialization
- `bootstrap/session.sh` - Agent session orchestration
- `bootstrap/SYSTEM.md` - Agent safety guardrails

This system is GCP-only and represents the MVP implementation.

---

## 5. Core Design Principles

1. **Container-first execution**
   - Agent logic always runs in containers
   - Containers are immutable and versioned
2. **Ephemerality with control**
   - VMs are temporary but may execute multiple tasks linearly
3. **Controller-as-judge lifecycle**
   - An LLM evaluator decides when phases are complete, not hardcoded heuristics
4. **PR-driven change**
   - All production changes flow through GitHub PRs
5. **Least privilege everywhere**
   - Agent VM has only GitHub write access (branches + PRs)
6. **Composable and repeatable**
   - New projects require minimal bespoke setup
7. **Agent-operable infrastructure**
   - Agents can reason about and operate the system without manual intervention

---

## 6. Functional Requirements

### FR-1: Project Bootstrap Automation

The system must support creating a new webapp with:

- A single command, script, or agent-driven workflow
- Outputs:
  - GitHub repository
  - Production hosting configured
  - CI/CD pipeline enabled
  - Agent execution pipeline ready

No manual cloud console steps should be required after initial platform setup.

---

### FR-3: Agent Session Execution Model (Containerized)

The system must support **agent sessions**, where:

- A single VM hosts one **agent session**
- The session runs one or more **agent runtime containers**
- Containers execute tasks **sequentially** within the session
- Containers may be reused across iterations within the same session
- Containers must not persist beyond the lifetime of the VM

The VM is the **security, identity, and billing boundary**.

---

### FR-4: Agent Runtime Container

An **agent runtime container** is a versioned, immutable image that contains:

- Language runtimes
- Agent tooling
- Repo interaction utilities
- Session client logic

Requirements:

- Containers must be runnable without network ingress
- Containers must not store long-lived secrets
- Containers must be provider-agnostic
- Containers must support deterministic startup

---

### FR-4.1: Supported Agent Adapters

The system supports pluggable agent adapters via a registry pattern:

**Claude Code** (Primary)
- Container: `ghcr.io/andywolf/agentium-claudecode:latest`
- Invocation: `claude --print --output-format stream-json --dangerously-skip-permissions`
- Output parsing: Structured stream-json events (tool calls, thinking, results, PR detection)
- Auth modes: API key or OAuth credential mount

**Aider** (Secondary)
- Container: `ghcr.io/andywolf/agentium-aider:latest`
- Invocation: `aider --model claude-3-5-sonnet --yes-always --no-git`
- Output parsing: File modification detection

Custom agents can be added by implementing the `Agent` interface and registering in the adapter registry.

---

### FR-5: Session Properties (Prompt-Configurable)

Each agent session must accept:

- One or more task identifiers (e.g., GitHub issue numbers, PR numbers)
- A maximum iteration count
- Optional wall-clock time limit
- Optional token budget
- Optional failure policy
- Phase loop configuration (iteration limits per phase)

#### Example Prompt Semantics

> "Complete issues 12, 17, and 24.\
> Max 30 iterations.\
> Shut down when all issues have PRs, iteration limit is reached, tokens are exhausted, or a fatal error occurs."

**Implementation Status:**
- Maximum iteration count: implemented
- Wall-clock time limit: implemented
- Phase loop iteration limits: implemented
- Token budget enforcement: planned (#18)

---

### FR-6: Phase Loop Agent Execution

Within a session, the agent operates via a **controller-as-judge phase loop**:

#### Phases (in order):
1. **PLAN** — Agent understands the issue and creates an implementation plan
2. **IMPLEMENT** — Agent writes code following the plan
3. **TEST** — Agent runs tests on the implementation
4. **REVIEW** — Agent reviews the git diff for quality/correctness
5. **PR_CREATION** — Agent creates a pull request

#### Evaluator (Judge):
After each phase (except PR_CREATION), an LLM evaluator assesses the output:
- `ADVANCE` — Work is sufficient, proceed to next phase
- `ITERATE` — Work needs improvement; feedback stored in memory for re-attempt
- `BLOCKED` — Cannot proceed without human intervention

#### Phase Iteration Limits:
Each phase has a configurable maximum iteration count (safety valve):
- PLAN: default 3
- IMPLEMENT: default 5
- TEST: default 5
- REVIEW: default 3
- PR_CREATION: 1 (terminal)

If max iterations are exhausted without ADVANCE, the controller force-advances to the next phase.

#### Visibility:
Every phase iteration and evaluator verdict is posted as a comment on the GitHub issue.

#### Backward Compatibility:
When `phase_loop.enabled` is false or absent, the controller falls back to single-iteration linear execution.

---

### FR-6.1: Memory System

The controller maintains a persistent memory store across iterations:

- **Signal types**: KEY_FACT, DECISION, STEP_DONE, ERROR, EVAL_FEEDBACK, PHASE_RESULT
- Memory carries context between phase iterations (plans, decisions, evaluator feedback)
- Context budget prevents unlimited memory growth
- Phase-transient memory (evaluator feedback) is cleared on phase advancement

---

### FR-6.2: Skills System

Phase-aware skill selection reduces prompt size and improves agent focus:

- Skills are associated with specific phases (e.g., planning skills vs. implementation skills)
- Skills are prioritized and selected based on the active phase
- The skill manifest defines available guidance per phase

---

### FR-6.3: Model Routing

The controller supports per-phase model assignment:

- Format: `adapter:model-id` (e.g., `claude-code:claude-opus-4`, `claude-code:claude-haiku`)
- Different phases can use different models (e.g., expensive model for planning, cheaper for implementation)
- Configured via routing rules in session config

---

### FR-6.4: Sub-Agent Delegation

Specialized sub-agents can be assigned to specific phases:

- Phase-to-agent routing via delegation mapping
- Enables cost optimization (cheaper agents for simpler phases)
- Supports adapter/model combinations per phase

---

### FR-7: Deterministic VM Termination Conditions

The VM **must self-terminate** when **any** of the following conditions occur:

1. All specified tasks have corresponding PRs opened or updated
2. Maximum iteration count is reached
3. Token budget is exhausted
4. Hard time limit is exceeded
5. Unrecoverable agent error occurs
6. Phase loop returns BLOCKED verdict
7. Explicit shutdown command is issued by the session controller

Container exit alone must **not** trigger VM termination.

Termination behavior must be **predictable and auditable**.

**Implementation Status:**
- All tasks complete (PR detection): implemented
- Maximum iterations reached: implemented
- Hard time limit exceeded: implemented
- Fatal error: implemented
- BLOCKED verdict: implemented
- Explicit shutdown: implemented
- Token budget exhaustion: planned (#18)

---

### FR-8: GitHub Authentication Model

Agents authenticate to GitHub using:

- A GitHub App (not a PAT)
- JWT generation from App private key
- Installation token exchange for repository access
- Permissions limited to:
  - Repository contents: read/write
  - Issues: read/write
  - Pull requests: read/write
  - Metadata: read

Agents must not:

- Access GitHub secrets
- Trigger deployments directly
- Bypass branch protection rules

**Implementation Status:** Fully implemented in both bootstrap system and Go controller. GitHub App JWT generation (#3), installation token exchange (#4), and controller integration (#5) are complete.

---

### FR-9: Secret Management

The agent VM must:

- Contain **no long-lived production secrets**
- Obtain GitHub App credentials at runtime via:
  - Cloud instance identity + secret manager (GCP Secret Manager implemented)
- Clear secrets from memory before termination
- Never persist credentials to the workspace filesystem

**Implementation Status:** GCP Secret Manager client implemented (#2). Credentials cleared on graceful shutdown. GitHub tokens generated per-session and never written to disk.

---

### FR-10: CI/CD as the Trust Boundary

CI/CD must:

- Own all deploy credentials
- Deploy previews on PR
- Deploy production on merge
- Enforce tests, linting, and optional human approval

Agent-generated code must never bypass CI.

---

### FR-11: Multi-Repository Reuse

The infrastructure must support:

- Many independent webapps
- Shared tooling and templates
- Minimal per-project configuration (ideally a single config file + `.agentium/AGENT.md`)

Adding a new project should be <15 minutes of human effort.

**Cloud Provider Support:**

| Provider | Provisioner | Secret Manager | Terraform Module |
|----------|-------------|----------------|------------------|
| GCP      | Complete    | Complete       | Complete         |
| AWS      | Planned     | Planned        | Not started      |
| Azure    | Planned     | Planned        | Not started      |

---

## 7. Logging, Observability, and Auditability

The system produces structured, durable logs for every agent session and container execution. Logging supports debugging, cost control, and post-hoc analysis of agent behavior.

### Required Log Fields (Minimum)

For each container execution attempt, the system records:

- Task identifier(s) or issue ID(s) the container attempted to address
- Session identifier
- Iteration number
- Container start timestamp
- Container end timestamp (if available)
- Execution duration (best-effort)
- Token consumption (best-effort)
- Exit status (success, retryable failure, fatal failure)
- Error classification and message (if any)
- Agent events (tool calls, thinking blocks, results) at DEBUG level

The system tolerates partial logs when containers terminate unexpectedly.

### Session-Level Logs

At the session level, the system records:

- Session start and end timestamps
- Aggregate iteration count
- Aggregate token consumption (best-effort)
- Tasks completed vs. attempted
- Phase transitions and evaluator verdicts
- Session termination reason

### Log Durability and Access

- Logs are emitted to GCP Cloud Logging (durable, external to VM lifecycle)
- VM termination does not result in log loss (10s flush timeout on shutdown)
- Logs are structured JSON with session metadata labels
- Agent events are truncated to 2000 chars (64KB Cloud Logging limit protection)

### Log Access CLI

```bash
agentium logs <session-id>              # View logs
agentium logs <session-id> --follow     # Real-time tail
agentium logs <session-id> --level error # Filter by severity
agentium logs <session-id> --events     # Agent event extraction
agentium logs <session-id> --since 30m  # Time-based filtering
```

### Logging Responsibility

- The session controller emits authoritative lifecycle and termination logs
- Agent runtime containers emit execution-level events via structured output
- Container exit without graceful shutdown still produces a controller-level record

**Implementation Status:** Cloud Logging integration complete (#6). Real-time session status via GCP metadata (#8) complete. Log streaming via CLI implemented (#111).

---

## 8. Non-Functional Requirements

### Security

- No production secrets in agent VMs
- No privilege escalation across iterations
- No persistence of credentials between sessions
- Credentials cleared from memory before VM termination

### Cost Control

- VMs billed per session
- Explicit iteration/time/token caps
- Guaranteed VM destruction
- Spot/preemptible instances by default

### Observability

- Session-level structured logs (Cloud Logging)
- Phase-level iteration tracking with evaluator verdicts
- GitHub issue comments for phase progress visibility
- PR history as canonical output

### Reliability

- Idempotent session startup
- Safe retry semantics (new VM, new session)
- Graceful shutdown with log flush (30s timeout)
- Force-advance on phase iteration exhaustion

---

## 9. Developer Experience (DX)

### Command-Line Interface (CLI)

The system provides a **command-line interface** for initiating and monitoring agent sessions.

The CLI allows a human operator to:
- Specify a target repository
- Specify one or more task sources (GitHub issue numbers, PR numbers)
- Configure session limits (iterations, time, token budgets)
- Choose an agent adapter
- Launch an agent session without manual cloud interaction
- Monitor session progress in real-time
- Stream structured logs
- Terminate sessions

The CLI must not:
- Require direct access to cloud provider consoles
- Expose production credentials
- Allow agents to bypass PR-based workflows

### CLI Commands

| Command | Description | Status |
|---------|-------------|--------|
| `agentium init` | Initialize project configuration | Implemented |
| `agentium run` | Launch an agent session | Implemented |
| `agentium status` | Check session status (with --watch) | Implemented |
| `agentium logs` | View/stream session logs | Implemented |
| `agentium destroy` | Terminate and clean up a session | Implemented |

### Human Workflow

1. Create a GitHub issue describing the desired change
2. Run `agentium run --issues <number>` to launch a session
3. Monitor progress via `agentium logs` or GitHub issue comments (optional)
4. Review resulting PR
5. Merge or reject

### Agent Workflow

1. VM boots via Terraform provisioner
2. Cloud-init installs Docker and pulls controller image
3. Session controller initializes, fetches secrets, mints GitHub token
4. Agent runtime container starts with cloned repo
5. Phase loop begins: PLAN → EVALUATE → IMPLEMENT → EVALUATE → TEST → EVALUATE → REVIEW → EVALUATE → PR_CREATION
6. Each phase iteration posts progress to GitHub issue
7. Session controller evaluates termination conditions
8. Credentials cleared, logs flushed, VM self-destructs

Humans should never need to SSH into the VM.

---

## 10. Configuration Model

### Project Configuration (`.agentium.yaml`)

```yaml
project:
  name: "my-project"
  repository: "github.com/org/repo"

github:
  app_id: 123456
  installation_id: 789012
  private_key_secret: "projects/gcp-project/secrets/github-app-key"

cloud:
  provider: "gcp"
  region: "us-central1"
  project: "my-gcp-project"
  machine_type: "e2-medium"
  use_spot: true
  disk_size_gb: 50

defaults:
  agent: "claude-code"
  max_iterations: 30
  max_duration: "2h"

phase_loop:
  enabled: true
  plan_max_iterations: 3
  build_max_iterations: 5
  review_max_iterations: 3
```

### Agent Instructions (`.agentium/AGENT.md`)

Project-specific guidance injected into agent prompts. Describes build commands, code conventions, and testing requirements for the target repository.

### Session Configuration

Each session accepts:
- Task set (issue numbers, PR numbers)
- Iteration cap
- Time limit
- Phase loop iteration limits
- Agent adapter selection
- Custom prompt (optional)

Configuration is machine-readable and prompt-injectable.

---

## 11. Session Controller

The session controller coordinates agent execution and enforces session lifecycle policies:

- Parses session configuration including task identifiers, iteration limits, and phase loop config
- Mints GitHub App installation tokens and provides them to agent containers via environment variables
- Launches agent runtime containers with controlled inputs
- Orchestrates the phase loop (plan, implement, test, review, evaluate)
- Maintains memory store across phase iterations
- Selects skills and routes models per phase
- Tracks iteration count and task completion state
- Posts phase progress as GitHub issue comments
- Enforces hard timeouts and termination limits
- Emits structured logs to Cloud Logging
- Clears credentials and flushes logs on shutdown
- Initiates VM termination as final action

The session controller is the **authority over session lifecycle and VM termination**, independent of individual container executions.

---

## 12. Success Criteria

The system is successful when:

- One VM can complete multiple related tasks safely using containers
- The phase loop produces higher-quality PRs through iterative self-review
- VM lifetimes are bounded and predictable
- Containers never outlive VMs
- No manual cleanup is required
- Human involvement is limited to issue creation and PR review
- The system scales horizontally across many small projects
- Phase progress is visible via GitHub issue comments

---

## 13. Implementation Status

### Phase 0: Bootstrap System (COMPLETE)

A working bootstrap system exists for GCP-only deployments:
- Local script-based session launching
- Terraform-based VM provisioning
- Cloud-init VM setup with all dependencies
- GitHub App authentication (JWT + installation token)
- Session lifecycle management
- Pre-baked GCP machine images for reduced cold start (#52)

### Phase 1: Go CLI & Controller (COMPLETE)

The Go-based CLI and controller are fully functional for GCP:
- CLI commands: init, run, status, logs, destroy
- Session controller with iteration tracking
- GCP provisioner with Terraform integration
- Agent adapters for Claude Code and Aider
- GitHub App JWT generation and token exchange (#3, #4, #5)
- GCP Secret Manager integration (#2)
- Cloud Logging integration (#6)
- Real-time session status via GCP metadata (#8)
- Graceful shutdown with log flush (#7)
- Task completion detection (#17, #113)
- Structured output parsing for Claude Code stream-json (#115)
- Pre-flight state detection and agent bouncing fix (#96)
- SYSTEM.md and AGENT.md injection into sessions (#78)

### Phase 2: Agent Intelligence (COMPLETE)

Controller-as-judge architecture with phase loop:
- Phase-aware skill selection (#91)
- Persistent memory across iterations (#92)
- LLM routing per phase (#93)
- Sub-agent delegation (#94)
- Controller-as-judge phase loop architecture (#117)

### Phase 3: Multi-Cloud Support (NOT STARTED)

AWS and Azure provisioners are planned but not implemented.

### Open Work Items

**Bugs:**
- Existing-work detection only checks 10 PRs (#130)
- Memory STEP_DONE resolution isn't task-scoped (#129)
- runSession doesn't call ValidateForRun (#128)
- `agentium run` requires --repo flag even when config provides it (#127)
- VM self-termination fails due to trailing whitespace in metadata (#126)
- Delegation mapping misses PhaseReview (#125)
- GCP destroy fallback leaves orphaned resources (#124)
- GCP gcloud calls ignore configured --project flag (#123)
- Potential deadlock in runAgentContainer (#122)

**Enhancements:**
- Add DOCS phase to phase loop (#131)
- Graceful shutdown with --force flag (#119)
- Agent-agnostic event abstraction (#116)
- Per-phase cost tracking (#106)
- Per-phase token limits and temperature routing (#105)
- Capability-based model selection (#104)
- Terraform workspaces for multi-instance awareness (#103)
- Simplified init with project scanner (#100)
- Codex CLI agent adapter (#99)
- GitHub Actions workflow trigger (#98)
- Installation token refresh (#62)
- Concurrent bootstrap sessions (#37)
- Security audit and hardening (#25)
- CI/CD pipeline (#23)
- Guided setup wizard (#20)
- Cost estimation (#19)
- Token budget enforcement (#18)

---

## 14. Intended Use as an Agent Prompt

This PRD is designed to:

- Be used verbatim as a system-level prompt
- Guide agents toward safe, efficient designs
- Encourage explicit lifecycle control via phase loops
- Prevent accidental persistence, privilege creep, or premature orchestration

Agents must treat **containerized execution**, **session-scoped VMs**, **controller-as-judge evaluation**, and **configurable termination** as mandatory constraints, not optional optimizations.
