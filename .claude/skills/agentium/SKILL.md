---
name: agentium
description: Helps compose and execute agentium CLI commands for launching, monitoring, and managing ephemeral AI agent sessions
allowed-tools:
  - Bash(agentium:*)
  - Read
  - Grep
  - Glob
argument-hint: "[command or question]"
---

# Agentium CLI Skill

Agentium provisions ephemeral cloud VMs to run AI coding agents (Claude Code, Aider) on GitHub issues and PRs. Each session creates an isolated environment that clones the repository, executes the agent, and automatically terminates when tasks complete or limits are reached.

## Dynamic Context

```
!cat .agentium.yaml 2>/dev/null || echo "No .agentium.yaml found in current directory"
```

## Commands Reference

### Global Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--config` | `.agentium.yaml` | Config file path |
| `--verbose` | `false` | Enable verbose output |

### `agentium init`

Initialize project configuration. Creates a `.agentium.yaml` file with sensible defaults.

| Flag | Default | Description |
|------|---------|-------------|
| `--name` | current directory name | Project name |
| `--repo` | | GitHub repository (e.g., `github.com/org/repo`) |
| `--provider` | `gcp` | Cloud provider (`gcp`, `aws`, `azure`) |
| `--region` | `us-central1` | Cloud region |
| `--app-id` | | GitHub App ID |
| `--installation-id` | | GitHub App Installation ID |
| `--force` | `false` | Overwrite existing config |

### `agentium run`

Launch an ephemeral AI agent session to work on GitHub issues or PR feedback.

| Flag | Default | Description |
|------|---------|-------------|
| `--repo` | (required) | GitHub repository (e.g., `github.com/org/repo`) |
| `--issues` | | Issue numbers to work on (comma-separated) |
| `--prs` | | PR numbers to address review feedback (comma-separated) |
| `--agent` | `claude-code` | Agent to use (`claude-code`, `aider`, `codex`) |
| `--max-iterations` | `30` | Maximum number of iterations |
| `--max-duration` | `2h` | Maximum session duration |
| `--provider` | from config | Cloud provider (`gcp`, `aws`, `azure`) |
| `--region` | from config | Cloud region |
| `--dry-run` | `false` | Show what would be provisioned without creating resources |
| `--prompt` | | Custom prompt for the agent |
| `--claude-auth-mode` | `api` | Claude auth mode: `api` or `oauth` |
| `--model` | | Override model for all phases (format: `adapter:model`) |
| `--phase-model` | | Per-phase model override (format: `PHASE=adapter:model`, repeatable) |
| `--local` | `false` | Run locally for interactive debugging (no VM provisioning) |

**Note:** At least one of `--issues` or `--prs` is required.

### `agentium status [session-id]`

Check session status. Without arguments, lists all active sessions. With a session ID, shows detailed status.

| Flag | Default | Description |
|------|---------|-------------|
| `--watch` | `false` | Watch for status changes |
| `--interval` | `10s` | Watch interval |

### `agentium logs <session-id>`

Retrieve logs from an Agentium session.

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--follow` | `-f` | `false` | Follow log output |
| `--tail` | | `100` | Number of lines to show from the end |
| `--since` | | | Show logs since timestamp (RFC3339) or duration (e.g., `1h`) |
| `--events` | | `false` | Show agent events (tool calls, decisions); implies `--level=debug` |
| `--level` | | `info` | Minimum log level: `debug`, `info`, `warning`, `error` |

### `agentium destroy <session-id>`

Force-terminate a session and destroy all associated cloud resources. Any uncommitted work will be lost.

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--force` | `-f` | `false` | Skip confirmation prompt |

## Configuration Reference

The `.agentium.yaml` file supports the following structure:

```yaml
# Project metadata
project:
  name: my-project
  repository: github.com/org/repo

# GitHub App credentials for repo access
github:
  app_id: 123456
  installation_id: 789012
  private_key_secret: projects/MY_GCP_PROJECT/secrets/github-key

# Cloud provider settings
cloud:
  provider: gcp                    # gcp, aws, azure
  region: us-central1
  project: my-gcp-project          # GCP project ID
  machine_type: e2-medium          # VM instance type (auto-detected per provider)
  use_spot: false                  # Use spot/preemptible instances
  disk_size_gb: 50

# Controller settings
controller:
  image: ghcr.io/andymwolf/agentium-controller:latest

# Default session settings (can be overridden per-run)
defaults:
  agent: claude-code               # claude-code, aider
  max_iterations: 30
  max_duration: 2h

# Session settings (applied per-run, merged with defaults)
session:
  repository: github.com/org/repo
  agent: claude-code
  max_iterations: 30
  max_duration: 2h
  prompt: ""                       # Custom agent prompt

# Claude authentication
claude:
  auth_mode: api                   # api or oauth
  auth_json_path: ~/.config/claude-code/auth.json

# Model routing: control which model runs each phase
routing:
  default:
    adapter: claude-code
    model: claude-sonnet-4-20250514
  overrides:
    PLAN:
      adapter: claude-code
      model: claude-opus-4-20250514
    IMPLEMENT:
      adapter: claude-code
      model: claude-sonnet-4-20250514

# Sub-agent delegation
delegation:
  enabled: false
  strategy: parallel               # parallel or sequential
  sub_agents:
    tests:
      agent: claude-code
      model:
        adapter: claude-code
        model: claude-sonnet-4-20250514
      skills: [testing]
```

## Phase Routing

Valid phases for `--phase-model` and routing config overrides:

| Phase | Description |
|-------|-------------|
| `PLAN` | Planning and analysis |
| `IMPLEMENT` | Code implementation |
| `TEST` | Test writing and execution |
| `PR_CREATION` | Pull request creation |
| `REVIEW` | Code review feedback |
| `DOCS` | Documentation updates |
| `EVALUATE` | Evaluation of results |
| `ANALYZE` | Issue/PR analysis |
| `PUSH` | Git push operations |

Format: `PHASE=adapter:model` (e.g., `PLAN=claude-code:claude-opus-4-20250514`)

## Common Workflows

### First-time setup

```bash
agentium init --repo github.com/org/repo --provider gcp --region us-central1
# Then edit .agentium.yaml to add GitHub App credentials
```

### Run agent on issues

```bash
agentium run --repo github.com/org/repo --issues 42,43,44
```

### Run agent on PR feedback

```bash
agentium run --repo github.com/org/repo --prs 50
```

### Monitor a session

```bash
agentium status                           # List all sessions
agentium status agentium-abc12345         # Detailed status
agentium status agentium-abc12345 --watch # Live updates
```

### View session logs

```bash
agentium logs agentium-abc12345
agentium logs agentium-abc12345 --follow          # Stream live
agentium logs agentium-abc12345 --events          # Show tool calls
agentium logs agentium-abc12345 --since 30m       # Last 30 minutes
```

### Dry run (preview without provisioning)

```bash
agentium run --repo github.com/org/repo --issues 42 --dry-run
```

### Custom model per phase

```bash
agentium run --repo github.com/org/repo --issues 42 \
  --model claude-code:claude-sonnet-4-20250514 \
  --phase-model PLAN=claude-code:claude-opus-4-20250514
```

### Emergency termination

```bash
agentium destroy agentium-abc12345 --force
```

### Local interactive mode (debugging)

Run the controller locally without provisioning a VM. The agent runs in interactive mode, prompting for permission approvals so you can watch and interact with execution.

**IMPORTANT:** Local mode requires a TTY for interactive permission prompts. When the user asks to run locally, **do NOT execute the command directly**. Instead, output a copyable bash command for them to run in Terminal:

```bash
GITHUB_TOKEN=$(gh auth token) ./agentium run --local --repo github.com/org/repo --issues 42 --max-iterations 1
```

**Requirements:**
- Must be run in a real terminal (not from within Claude Code)
- `gh` CLI must be authenticated (`gh auth status`)
- Docker must be installed and running

This is useful for:
- Debugging agent behavior
- Testing prompt changes
- Watching tool calls in real-time
- Interactively approving/denying agent actions

## Response Guidelines

When the user invokes `/agentium`, follow these rules:

1. **Check `$ARGUMENTS`**: If the user provided arguments, interpret whether they want to run a command or ask a question about agentium.

2. **Check dynamic context**: Review the `.agentium.yaml` output above. If a config exists, use its values as defaults when composing commands. If no config exists and the user wants to run something, suggest `agentium init` first.

3. **Compose commands**: When the user describes what they want in natural language, translate it into the appropriate `agentium` command with correct flags. Always show the full command before executing.

4. **Validate required fields**: Before running `agentium run`, ensure `--repo` and at least one of `--issues`/`--prs` are specified (either via flags or config).

5. **Warn about destructive operations**: Before running `agentium destroy`, confirm with the user unless they explicitly say to force it.

6. **Use dry-run for uncertainty**: If the user seems unsure or is exploring, suggest `--dry-run` first.

7. **Show next steps**: After running a command, suggest relevant follow-up commands (e.g., after `run`, suggest `status` and `logs`).

8. **Local mode - output command only**: When the user wants to run with `--local`, **do NOT execute the command**. Instead, output a copyable bash command for them to run in Terminal. Local mode requires a TTY for interactive permission prompts which isn't available when running from within Claude Code. Format:
   ```bash
   GITHUB_TOKEN=$(gh auth token) ./agentium run --local --repo <repo> --issues <issues> --max-iterations 1
   ```
   Adjust `--issues`/`--prs` and other flags as needed based on what the user requested.
