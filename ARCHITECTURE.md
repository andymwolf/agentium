# Agentium Architecture

Agentium is an ephemeral, safe execution framework for AI coding agents. This document describes the core architecture and design principles.

## Purpose & Goals

Design and implement a **repeatable, low-touch cloud execution framework** for running AI coding agents safely and cheaply, where:

- Agents run in **ephemeral or session-scoped execution environments**
- Agent execution is **containerized for portability and determinism**
- Human time is spent **authoring prompts and reviewing PRs**, not managing infrastructure
- Agent environments have **minimal privileges** and **no production secrets**
- Agent execution lifetime is **prompt-configurable**
- The system is **largely automated and reusable across many repositories**

This document covers **agent execution infrastructure only**. Application hosting and runtime concerns are out of scope.

## Non-Goals

- Supporting high-traffic or latency-critical workloads
- Supporting always-on agent compute
- Allowing agents to deploy directly to production
- Operating long-lived development VMs or clusters
- Introducing container orchestration platforms (e.g., Kubernetes)

## Target Users

- A single technical operator (human) comfortable with CLI-based agentic tools
- Autonomous or semi-autonomous AI coding agents
- GitHub Actions (as the trusted deployment authority)

## High-Level Architecture

### Planes of Responsibility

**Agentium**

- Ephemeral VM launched per **agent session**
- VM provides security boundary, identity, and billing scope
- VM runs one or more **agent runtime containers**
- A **session controller** orchestrates a **phase loop** (plan, implement, test, review) with LLM evaluation between phases
- Containers execute tasks **sequentially** within the session
- VM terminates deterministically based on configured conditions

Agents never hold production credentials.

**Application Runtime (Out of Scope)**

- Hosting, deployment, and operation of applications produced by agents
- Defined separately from this system

## Core Design Principles

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

## Execution Model

### Agent Sessions

- A single VM hosts one **agent session**
- The session runs one or more **agent runtime containers**
- Containers execute tasks **sequentially** within the session
- Containers may be reused across iterations within the same session
- Containers must not persist beyond the lifetime of the VM

The VM is the **security, identity, and billing boundary**.

### Agent Runtime Container

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

### Phase Loop

Within a session, the agent operates via a **controller-as-judge phase loop**:

**Core Phases (in order):**
1. **PLAN** - Agent understands the issue and creates an implementation plan
2. **IMPLEMENT** - Agent writes code following the plan
3. **DOCS** - Agent updates documentation as needed

**Judge Phases:**
After each core phase, an LLM judge assesses the output:
- `ADVANCE` - Work is sufficient, proceed to next phase
- `ITERATE` - Work needs improvement; feedback stored in memory for re-attempt
- `BLOCKED` - Cannot proceed without human intervention

**Reviewer Phases:**
Per-iteration review is available via `PLAN_REVIEW`, `IMPLEMENT_REVIEW`, and `DOCS_REVIEW` phases.

Every phase iteration and judge verdict is posted as a comment on the GitHub issue.

### Monorepo Scope Enforcement

For pnpm workspace monorepos, Agentium enforces per-package scope:

**Package Identification:**
- Issues must have a `pkg:<name>` label (e.g., `pkg:core`, `pkg:web`)
- The label maps to a package directory in `pnpm-workspace.yaml`
- Sessions without a package label are rejected in monorepo mode

**Scope Validation:**
- After each iteration, the controller validates file changes
- Only files within the target package directory are allowed
- Allowed exceptions: `package.json`, `pnpm-lock.yaml`, `.github/workflows/`
- Out-of-scope changes trigger:
  1. Automatic `git checkout .` and `git clean -fd` to reset changes
  2. Iteration marked as failed with scope violation feedback
  3. Agent continues with feedback about the violation

**Hierarchical Instructions:**
- Root `.agentium/AGENTS.md` provides repository-wide instructions
- Package `.agentium/AGENTS.md` provides package-specific instructions
- Both are merged and injected into the agent prompt

### VM Termination Conditions

The VM **must self-terminate** when **any** of the following conditions occur:

1. All specified tasks have corresponding PRs opened or updated
2. Maximum iteration count is reached
3. Token budget is exhausted
4. Hard time limit is exceeded
5. Unrecoverable agent error occurs
6. Phase loop returns BLOCKED verdict
7. Explicit shutdown command is issued by the session controller

Container exit alone must **not** trigger VM termination. Termination behavior must be **predictable and auditable**.

## Security Model

### GitHub Authentication

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

### Secret Management

The agent VM must:

- Contain **no long-lived production secrets**
- Obtain GitHub App credentials at runtime via cloud instance identity + secret manager
- Clear secrets from memory before termination
- Never persist credentials to the workspace filesystem

### CI/CD as the Trust Boundary

CI/CD must:

- Own all deploy credentials
- Deploy previews on PR
- Deploy production on merge
- Enforce tests, linting, and optional human approval

Agent-generated code must never bypass CI.

## Non-Functional Requirements

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
- Graceful shutdown with log flush
- Force-advance on phase iteration exhaustion

## Configuration

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

### Agent Instructions (`.agentium/AGENTS.md`)

Project-specific guidance injected into agent prompts. Describes build commands, code conventions, and testing requirements for the target repository.

## Human Workflow

1. Create a GitHub issue describing the desired change
2. Run `agentium run --issues <number>` to launch a session
3. Monitor progress via `agentium logs` or GitHub issue comments (optional)
4. Review resulting PR
5. Merge or reject

## Agent Workflow

1. VM boots via Terraform provisioner
2. Cloud-init installs Docker and pulls controller image
3. Session controller initializes, fetches secrets, mints GitHub token
4. Agent runtime container starts with cloned repo
5. Phase loop begins: PLAN -> PLAN_JUDGE -> IMPLEMENT -> IMPLEMENT_JUDGE -> DOCS -> DOCS_JUDGE
6. Each phase iteration posts progress to GitHub issue
7. Session controller evaluates termination conditions
8. Credentials cleared, logs flushed, VM self-destructs

Humans should never need to SSH into the VM.

## Success Criteria

The system is successful when:

- One VM can complete multiple related tasks safely using containers
- The phase loop produces higher-quality PRs through iterative self-review
- VM lifetimes are bounded and predictable
- Containers never outlive VMs
- No manual cleanup is required
- Human involvement is limited to issue creation and PR review
- The system scales horizontally across many small projects
- Phase progress is visible via GitHub issue comments
