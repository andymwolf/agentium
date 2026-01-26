# Getting Started with Agentium

Agentium implements the Ralph Wiggum loop pattern for autonomous software development: a controller-as-judge architecture where AI agents plan, implement, test, and self-review code in iterative loops on disposable cloud VMs. Create a GitHub issue, get a pull request.

## How It Works

1. You point Agentium at a GitHub issue
2. Agentium provisions an ephemeral cloud VM
3. The agent enters a phase loop (`PLAN → IMPLEMENT → TEST → REVIEW → PR_CREATION`), with an LLM evaluator judging each phase before advancing
4. The agent creates a pull request with the changes
5. The VM self-destructs after the session completes
6. You review and merge the PR

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

Each phase has configurable iteration limits (e.g., PLAN: 3, IMPLEMENT: 5, TEST: 5, REVIEW: 3) to prevent runaway loops. Phase verdicts are posted as comments on the GitHub issue.

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

You can provide custom instructions to the AI agent by creating a `.agentium/AGENT.md` file in your repository root. This file is automatically included in the agent's system prompt when a session runs.

Common things to include:
- Build and test commands for your project
- Coding conventions and style preferences
- Architecture notes and important patterns
- Off-limits areas (files the agent should not modify)

See the [Configuration Reference](configuration.md#project-specific-agent-instructions) for a detailed example.

## What's Next?

- [Configuration Reference](configuration.md) - All configuration options
- [CLI Reference](cli-reference.md) - Complete command documentation
- [Cloud Setup Guides](cloud-setup/) - Provider-specific setup
- [GitHub App Setup](github-app-setup.md) - Detailed GitHub App configuration
- [Troubleshooting](troubleshooting.md) - Common issues and solutions
