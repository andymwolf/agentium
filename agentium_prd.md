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

- A single technical operator (human)
- Autonomous or semi-autonomous AI coding agents
- GitHub Actions (as the trusted deployment authority)

---

## 4. High-Level Architecture

### Planes of Responsibility

**Agentium(This PRD)**

- Ephemeral VM launched per **agent session**
- VM provides security boundary, identity, and billing scope
- VM runs one or more **agent runtime containers**
- Containers execute agent iterations linearly within a session
- VM terminates deterministically based on prompt-defined conditions

Agents never hold production credentials.

**Application Runtime (Out of Scope)**

- Hosting, deployment, and operation of applications produced by agents
- Defined in a separate Application Runtime Platform PRD.

---

## 5. Core Design Principles

1. **Container-first execution**
   - Agent logic always runs in containers
   - Containers are immutable and versioned
2. **Ephemerality with control**
   - VMs are temporary but may execute multiple tasks linearly
3. **Prompt-defined lifecycle**
   - VM destruction rules are specified in the prompt
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

### FR-5: Session Properties (Prompt-Configurable)

Each agent session must accept:

- One or more task identifiers (e.g., GitHub issue numbers)
- A maximum iteration count
- Optional wall-clock time limit
- Optional token budget
- Optional failure policy

#### Example Prompt Semantics

> “Complete issues 12, 17, and 24.\
> Max 30 iterations.\
> Shut down when all issues have PRs, iteration limit is reached, tokens are exhausted, or a fatal error occurs.”

---

### FR-6: Iteration-Based Agent Execution

Within a session:

- The agent operates in **discrete iterations**
- Each iteration may:
  - Read issues
  - Modify code
  - Run tests
  - Commit changes
  - Update PRs
- Iteration count is tracked centrally and persisted in logs

This enables:

- Cost control
- Debuggability
- Reproducibility

---

### FR-7: Deterministic VM Termination Conditions

The VM **must self-terminate** when **any** of the following conditions occur:

1. All specified tasks have:
   - Corresponding branches
   - Corresponding PRs opened or updated
2. Maximum iteration count is reached
3. Token budget is exhausted
4. Hard time limit is exceeded
5. Unrecoverable agent error occurs
6. Explicit shutdown command is issued by the session controller

Container exit alone must **not** trigger VM termination.

Termination behavior must be **predictable and auditable**.

---

### FR-8: GitHub Authentication Model

Agents must authenticate to GitHub using:

- A GitHub App (not a PAT)
- Permissions limited to:
  - Repository contents: read/write
  - Pull requests: read/write
  - Metadata: read

Agents must not:

- Access GitHub secrets
- Trigger deployments directly
- Bypass branch protection rules

---

### FR-9: Secret Management

The agent VM must:

- Contain **no long-lived production secrets**
- Obtain GitHub App credentials at runtime via:
  - Cloud instance identity + secret manager (preferred)
- Destroy or shred secrets before termination

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
- Minimal per-project configuration (ideally a single config file)

Adding a new project should be <15 minutes of human effort.

---

## 7. Logging, Observability, and Auditability

The system must produce structured, durable logs for every agent session and container execution. Logging is required to support debugging, cost control, and post-hoc analysis of agent behavior.

### Required Log Fields (Minimum)

For each container execution attempt, the system must attempt to record:

- Task identifier(s) or issue ID(s) the container attempted to address
- Session identifier
- Iteration number
- Container start timestamp
- Container end timestamp (if available)
- Execution duration (best-effort)
- Token consumption (best-effort)
- Exit status (success, retryable failure, fatal failure)
- Error classification and message (if any)

The system must tolerate partial logs when containers terminate unexpectedly. Missing duration or token data must not block session progress or VM termination.

### Session-Level Logs

At the session level, the system must record:

- Session start and end timestamps
- Aggregate iteration count
- Aggregate token consumption (best-effort)
- Tasks completed vs. attempted
- Session termination reason (e.g., all tasks complete, iteration limit reached, timeout, fatal error)

### Log Durability and Access

- Logs must be emitted to a durable sink external to the VM lifecycle
- VM termination must not result in log loss
- Logs must be machine-readable (e.g., structured text or JSON)

### Logging Responsibility

- The session controller is responsible for emitting authoritative lifecycle and termination logs
- Agent runtime containers may emit execution-level logs, but these are advisory
- Container exit without graceful shutdown must still produce a controller-level record

---

## 8. Non-Functional Requirements

### Security

- No production secrets in agent VMs
- No privilege escalation across iterations
- No persistence of credentials between sessions

### Cost Control

- VMs billed per session
- Explicit iteration/time/token caps
- Guaranteed VM destruction

### Observability

- Session-level logs
- Iteration-level logs
- PR history as canonical output

### Reliability

- Idempotent session startup
- Safe retry semantics (new VM, new session)

---

## 8. Developer Experience (DX)

### Command-Line Interface (CLI)

The system must provide a **command-line interface** for initiating agent sessions.

The CLI must allow a human operator to:
- Specify a target repository
- Specify one or more task sources (e.g., GitHub issue numbers)
- Provide or reference a prompt
- Configure session limits (iterations, time, token budgets)
- Launch an agent session without manual cloud interaction

The CLI must not:
- Require direct access to cloud provider consoles
- Expose production credentials
- Allow agents to bypass PR-based workflows

The CLI is the **primary human-facing control surface** for Agent Cloud.

### Human Workflow

1. Write or update a **session prompt**
2. Trigger an agent session
3. Monitor session progress (optional)
4. Review resulting PRs
5. Merge or reject

### Agent Workflow

1. VM boots
2. Session controller initializes
3. Agent runtime container starts
4. Issues/tasks are enumerated
5. Agent executes iteration loop
6. PRs are created/updated
7. Session controller evaluates termination conditions
8. VM self-destructs

Humans should never need to SSH into the VM.

---

## 9. Configuration Model (Agent-Friendly)

Each project must define:

- App metadata
- Repo information
- Deployment target
- DB choice
- Default agent session limits
- CI requirements

Each session must define:

- Task set
- Iteration cap
- Optional token/time budgets
- Termination behavior

Configuration must be machine-readable and prompt-injectable.

---

## 10. Session Controller (Conceptual Component)

The system must include a **session controller** responsible for coordinating agent execution and enforcing session lifecycle policies.

The session controller must be able to:

- Parse the prompt or session specification, including task identifiers, iteration limits, and budget constraints
- Mint or retrieve a GitHub App installation token and make it available to agent containers via strictly scoped inputs (e.g., environment variables or files)
- Launch agent runtime containers with controlled inputs and parameters
- Track iteration count and task completion state across the session
- Restart or replace agent containers as needed within the bounds of the session
- Enforce hard timeouts and other termination limits
- Emit structured logs and events to a durable sink
- Decide when session termination conditions have been met
- Initiate VM shutdown when the session ends

The session controller is the **authority over session lifecycle and VM termination**, independent of individual container executions.

---

## 11. Success Criteria

The system is successful when:

- One VM can complete multiple related tasks safely using containers
- VM lifetimes are bounded and predictable
- Containers never outlive VMs
- No manual cleanup is required
- Human involvement is limited to prompting and PR review
- The system scales horizontally across many small projects

---

## 12. Intended Use as an Agent Prompt

This PRD is designed to:

- Be used verbatim as a system-level prompt
- Guide agents toward safe, efficient designs
- Encourage explicit lifecycle control
- Prevent accidental persistence, privilege creep, or premature orchestration

Agents must treat **containerized execution**, **session-scoped VMs**, and **prompt-defined termination** as mandatory constraints, not optional optimizations.

