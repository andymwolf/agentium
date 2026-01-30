# Getting Started with Agentium

Agentium implements the Ralph Wiggum loop pattern for autonomous software development: a controller-as-judge architecture where AI agents plan, implement, test, and self-review code in iterative loops on disposable cloud VMs. Create a GitHub issue, get a pull request.

## Who Is This For?

Agentium is intended for people comfortable using agentic coding tools from the command line. If you've used Claude Code, Aider, or similar CLI-based AI tools, you'll feel at home. That said, the workflow is designed to be accessible to non-coders too: create a well-described GitHub issue, and Agentium handles the rest.

## How It Works

Agentium's core workflow mirrors how a developer works on a ticket:

1. You create a GitHub issue describing what you want
2. Agentium spins up an ephemeral VM
3. The agent enters a phase loop:

```
PLAN --> [EVALUATE] --> IMPLEMENT --> [EVALUATE] --> TEST --> [EVALUATE] --> REVIEW --> [EVALUATE] --> PR_CREATION --> COMPLETE
```

Each phase runs in a clean context window. After each phase, an LLM evaluator (the "judge") decides:
- **ADVANCE** — Work is sufficient, proceed to next phase
- **ITERATE** — Work needs improvement, loop back with feedback
- **BLOCKED** — Cannot proceed, needs human intervention

This means the agent can iterate on its own implementation multiple times before moving on—catching bugs, improving code quality, and fixing test failures without human involvement.

4. The agent creates a pull request for your review
5. The VM self-destructs

## Prerequisites

Before using Agentium, ensure you have the following installed:

- **Go 1.19+** - Required to build the CLI from source
- **Terraform 1.0+** - Used for cloud VM provisioning
- **gcloud CLI** - Required for GCP provider (authenticated with `gcloud auth login`)
- **GitHub CLI** (`gh`) - Recommended for GitHub App setup verification
- **Docker** (optional) - For building custom agent container images
- **A GitHub App** - For repository access (see [GitHub App Setup](github-app-setup.md))

## Installation

### Build from Source

```bash
git clone https://github.com/andymwolf/agentium.git
cd agentium
go build -o agentium ./cmd/agentium
```

Optionally, move the binary to your PATH:

```bash
mv agentium /usr/local/bin/
```

### Verify Installation

```bash
agentium --help
```

You should see usage information for the CLI with available commands (`init`, `run`, `status`, `logs`, `destroy`).

## Quick Start

### 1. Create a GitHub App

Follow the [GitHub App Setup Guide](github-app-setup.md) to create and install a GitHub App with the required permissions.

### 2. Store Credentials in Cloud Secret Manager

For GCP:

```bash
# Enable required APIs
gcloud services enable compute.googleapis.com secretmanager.googleapis.com

# Store the GitHub App private key
gcloud secrets create github-app-key \
  --replication-policy="automatic"
gcloud secrets versions add github-app-key \
  --data-file=/path/to/your-app-private-key.pem

# Optionally store Anthropic API key (for Claude Code with API auth mode)
gcloud secrets create anthropic-api-key \
  --replication-policy="automatic"
echo -n "sk-ant-your-api-key" | \
  gcloud secrets versions add anthropic-api-key --data-file=-
```

> **Security Note:** Never commit API keys or private keys to your repository. Always use cloud secret managers for credential storage.

### 3. Initialize Your Project

Navigate to your project repository and run:

```bash
agentium init \
  --repo github.com/your-org/your-repo \
  --provider gcp \
  --region us-central1
```

This creates a `.agentium.yaml` configuration file. Edit it to add your GitHub App credentials:

```yaml
github:
  app_id: 123456
  installation_id: 789012
  private_key_secret: "projects/my-gcp-project/secrets/github-app-key"
```

See the [Configuration Reference](configuration.md) for all available options.

### 4. Run Your First Session

```bash
agentium run --repo github.com/your-org/your-repo --issues 42
```

This will:
- Provision an ephemeral GCP VM
- Clone your repository
- Run the AI agent (Claude Code by default) to implement issue #42
- Create a pull request with the changes
- Terminate the VM

### 5. Monitor the Session

```bash
# Check session status
agentium status

# Watch a specific session
agentium status agentium-abc12345 --watch

# Stream logs
agentium logs agentium-abc12345 --follow

# View agent events (tool calls, decisions)
agentium logs agentium-abc12345 --events
```

### 6. Review the Pull Request

Once the agent creates a PR, review it on GitHub as you would any other contribution. The PR will reference the original issue.

## Common Workflows

### Fix a Single Issue

```bash
agentium run --repo github.com/org/repo --issues 42
```

### Work on Multiple Issues

```bash
agentium run --repo github.com/org/repo --issues 42,43,44
```

### PR Review Session

Have the agent address code review feedback on an existing PR:

```bash
agentium run --repo github.com/org/repo --prs 50
```

### Use a Different Agent

```bash
agentium run --repo github.com/org/repo --issues 42 --agent aider
```

### Use a Specific Model

```bash
agentium run --repo github.com/org/repo --issues 42 \
  --model claude-code:claude-opus-4-20250514
```

### Dry Run (Preview Without Provisioning)

```bash
agentium run --repo github.com/org/repo --issues 42 --dry-run
```

### Working with Monorepos (pnpm Workspaces)

For projects using pnpm workspaces, Agentium provides per-package scope enforcement:

```bash
# Agentium auto-detects pnpm-workspace.yaml during init
agentium init --repo github.com/org/monorepo --provider gcp
# Output: Detected pnpm-workspace.yaml - monorepo support enabled
```

When monorepo mode is enabled:
- Issues **must** have a `pkg:<package-name>` label to specify scope
- Agents can only modify files within the target package directory
- Out-of-scope file changes are automatically reset and block iteration
- Root-level files (`package.json`, `pnpm-lock.yaml`, `.github/workflows/`) are allowed

**Creating package-scoped issues:**

Use the `/gh-issues` skill which automatically handles package labels:

```bash
# The skill will prompt for package selection in monorepos
/gh-issues Add validation to the form component
```

Or create issues manually with the required label:

```bash
gh label create "pkg:web" --color "0052CC"
gh issue create --title "Add form validation" --label "pkg:web,enhancement"
```

**Package-specific AGENT.md:**

You can provide per-package agent instructions by creating `.agentium/AGENT.md` within a package directory. These are merged with the root AGENT.md:

```
my-monorepo/
├── .agentium/AGENT.md          # Repository-wide instructions
├── packages/
│   ├── core/
│   │   └── .agentium/AGENT.md  # Core package instructions
│   └── web/
│       └── .agentium/AGENT.md  # Web package instructions
└── pnpm-workspace.yaml
```

### Local Interactive Mode (Debugging)

Run the controller locally without provisioning a VM. The agent runs in interactive mode, prompting for permission approvals:

```bash
export GITHUB_TOKEN=<your-token>
agentium run --local --repo github.com/org/repo --issues 42
```

This is useful for:
- Debugging agent behavior and prompt issues
- Testing changes to agent configuration
- Watching tool calls and decisions in real-time
- Interactively approving or denying agent actions

**Note:** Local mode requires Docker and a `GITHUB_TOKEN` environment variable. No GitHub App configuration is needed.

## Session Lifecycle

Each Agentium session follows this lifecycle:

```
Provision VM → Phase Loop (PLAN → IMPLEMENT → TEST → REVIEW → PR_CREATION) → Terminate VM
```

Within the phase loop, an LLM evaluator judges each phase's output and decides:
- **ADVANCE** — Work is sufficient, proceed to next phase
- **ITERATE** — Work needs improvement, re-run the phase with feedback
- **BLOCKED** — Cannot proceed, needs human intervention

Each phase has configurable iteration limits to prevent runaway loops:

```yaml
phase_loop:
  enabled: true
  plan_max_iterations: 3
  build_max_iterations: 5
  review_max_iterations: 3
```

Every phase iteration and evaluator verdict is posted as a comment on the GitHub issue, giving you full visibility into the agent's reasoning.

Sessions automatically terminate when:
- All assigned issues have PRs created
- Maximum iteration count is reached (default: 30)
- Maximum duration is exceeded (default: 2 hours)
- The evaluator returns a BLOCKED verdict
- An unrecoverable error occurs

You can also manually terminate a session:

```bash
agentium destroy agentium-abc12345
```

## Project-Specific Agent Instructions

The `agentium init` command automatically generates a `.agentium/AGENT.md` file by scanning your project. This file is included in the agent's system prompt when a session runs.

Auto-detected information includes:
- Primary language and framework
- Build, test, and lint commands
- Project structure (source dirs, test dirs, entry points)
- CI/CD configuration

You can customize the generated file by adding your own sections:
- Coding conventions and style preferences
- Architecture notes and important patterns
- Off-limits areas (files the agent should not modify)

To regenerate AGENT.md after making project changes:

```bash
agentium refresh
```

See the [Configuration Reference](configuration.md#project-specific-agent-instructions) for a detailed example.

## Claude Code Authorization

Agentium supports two authentication modes for the Claude Code agent:

### API Mode (Recommended)
Uses a standard Anthropic API key. Set via environment variable or config. Straightforward and cross-platform.

### OAuth Mode (MacOS Only)
Uses stored credentials from `~/.config/claude-code/auth.json`. This allows the agent to use your existing Claude Code subscription rather than a separate API key.

**Important considerations:**
- Copying OAuth credentials from your local Keychain to a remote VM may violate the [Claude Code Terms of Service](https://www.anthropic.com/terms), which generally prohibit credential sharing or automated access outside approved channels.
- OAuth tokens are scoped to your personal account—usage on remote VMs counts against your subscription.
- This feature exists for convenience during development but should be used with awareness of the TOS implications.
- API mode with a dedicated API key is the recommended production approach.

## Architecture Overview

```
┌─────────────┐     ┌─────────────┐     ┌─────────────────────────────────┐
│  Agentium   │────>│ Provisioner │────>│         Ephemeral Cloud VM       │
│    CLI      │     │ (Terraform) │     │                                   │
└─────────────┘     └─────────────┘     │  ┌─────────────────────────────┐ │
                                        │  │    Session Controller        │ │
                                        │  │  ┌───────────────────────┐  │ │
                                        │  │  │ Phase Loop (Judge)    │  │ │
                                        │  │  │ PLAN→IMPL→TEST→REVIEW │  │ │
                                        │  │  └───────────────────────┘  │ │
                                        │  └──────────────┬──────────────┘ │
                                        │                 │                 │
                                        │  ┌──────────────v──────────────┐ │
                                        │  │     Agent Container         │ │
                                        │  │   (Claude Code / Aider)     │ │
                                        │  └──────────────┬──────────────┘ │
                                        └─────────────────┼─────────────────┘
                                                          │
                                                          v
                                                  ┌───────────────┐
                                                  │  GitHub API   │
                                                  │  (PRs/Issues) │
                                                  └───────────────┘
```

### Component Breakdown

| Component | Purpose |
|-----------|---------|
| **CLI** (`cmd/agentium/`) | User interface for launching and monitoring sessions |
| **Controller** (`cmd/controller/`) | Orchestrates agent execution on the VM |
| **Phase Loop** (`internal/controller/phase_loop.go`) | Implements the Ralph Wiggum loop with evaluator |
| **Agent Adapters** (`internal/agent/`) | Pluggable adapters for Claude Code, Aider, etc. |
| **Provisioner** (`internal/provisioner/`) | Creates and manages cloud VMs |
| **Memory Store** (`internal/memory/`) | Persistent context between phase iterations |
| **Skills** (`internal/skills/`) | Phase-aware prompt selection |
| **Model Routing** (`internal/routing/`) | Per-phase model assignment |

## What's Next?

- [Configuration Reference](configuration.md) - All configuration options
- [CLI Reference](cli-reference.md) - Complete command documentation
- [Cloud Setup Guides](cloud-setup/) - Provider-specific setup
- [GitHub App Setup](github-app-setup.md) - Detailed GitHub App configuration
- [Troubleshooting](troubleshooting.md) - Common issues and solutions
