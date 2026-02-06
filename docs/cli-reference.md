# CLI Reference

Complete reference for all Agentium CLI commands, flags, and usage examples.

## Global Flags

These flags are available on all commands:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--config` | string | `.agentium.yaml` | Path to configuration file |
| `--verbose` | bool | `false` | Enable verbose output |
| `--help` | bool | - | Show help message |

## Commands

### `agentium init`

Initialize Agentium configuration for a project. Creates a `.agentium.yaml` file in the current directory with sensible defaults.

**Usage:**

```bash
agentium init [flags]
```

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | Directory name | Project name |
| `--repo` | string | - | GitHub repository (e.g., `github.com/org/repo`) |
| `--provider` | string | `gcp` | Cloud provider: `gcp`, `aws`, `azure` |
| `--region` | string | `us-central1` | Cloud region |
| `--app-id` | int64 | - | GitHub App ID |
| `--installation-id` | int64 | - | GitHub App Installation ID |
| `--force` | bool | `false` | Overwrite existing config file |
| `--greenfield` | bool | `false` | Skip project scanning, create minimal AGENTS.md for new projects |
| `--skip-agent-md` | bool | `false` | Skip AGENTS.md generation entirely |
| `--non-interactive` | bool | `false` | Use auto-detected values without prompting |

**Examples:**

```bash
# Basic initialization (scans project and generates AGENTS.md)
agentium init --repo github.com/myorg/myrepo --provider gcp

# Full initialization with GitHub App credentials
agentium init \
  --repo github.com/myorg/myrepo \
  --provider gcp \
  --region us-east1 \
  --app-id 123456 \
  --installation-id 789012

# Non-interactive mode (accept detected values)
agentium init --repo github.com/myorg/myrepo --non-interactive

# New project without existing code
agentium init --repo github.com/myorg/newrepo --greenfield

# Skip AGENTS.md generation
agentium init --repo github.com/myorg/myrepo --skip-agent-md

# Overwrite existing config
agentium init --repo github.com/myorg/myrepo --force
```

**Output:**

Creates `.agentium.yaml` with the specified configuration. Also generates `.agentium/AGENTS.md` with auto-detected project information (build commands, test commands, project structure) unless `--skip-agent-md` is specified. If a config file already exists and `--force` is not set, the command will exit with an error.

**Monorepo Detection:**

If a `pnpm-workspace.yaml` file is present, `agentium init` automatically:
- Sets `monorepo.enabled: true` in the config
- Sets `monorepo.label_prefix: "pkg"` as the default prefix
- Outputs: `Detected pnpm-workspace.yaml - monorepo support enabled`

This enables per-package scope enforcement, requiring issues to have `pkg:<package-name>` labels.

---

### `agentium refresh`

Regenerate `.agentium/AGENTS.md` by rescanning the project. Preserves custom content you've added outside the auto-generated sections.

**Usage:**

```bash
agentium refresh [flags]
```

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--non-interactive` | bool | `false` | Use detected values without prompting |
| `--force` | bool | `false` | Regenerate without confirmation prompt |

**Examples:**

```bash
# Regenerate AGENTS.md (prompts for confirmation if custom content exists)
agentium refresh

# Non-interactive mode
agentium refresh --non-interactive

# Force regeneration without confirmation
agentium refresh --force
```

**Output:**

Updates `.agentium/AGENTS.md` with fresh project analysis while preserving any custom sections you've added. Requires `.agentium.yaml` to exist (run `agentium init` first).

---

### `agentium run`

Launch an ephemeral agent session to work on GitHub issues or review PRs.

**Usage:**

```bash
agentium run [flags]
```

**Required Flags:**

- `--repo` is always required (enforced by the CLI; you must provide it on the command line)
- `--issues` must be specified

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--repo` | string | **Required** | GitHub repository (e.g., `github.com/org/repo`) |
| `--issues` | string | - | Issue numbers to work on (comma-separated, supports ranges like `1-5`) |
| `--agent` | string | `claude-code` | Agent to use: `claude-code`, `aider`, `codex` |
| `--max-iterations` | int | `30` | Maximum iterations before termination |
| `--max-duration` | string | `2h` | Maximum session duration |
| `--provider` | string | From config | Cloud provider: `gcp`, `aws`, `azure` (required if not in config) |
| `--region` | string | From config | Cloud region (overrides config) |
| `--prompt` | string | - | Custom prompt/instructions for the agent |
| `--model` | string | - | Override model for all phases (format: `adapter:model`) |
| `--phase-model` | string | - | Per-phase model override (repeatable, format: `PHASE=adapter:model`) |
| `--claude-auth-mode` | string | `api` | Claude authentication: `api`, `oauth` |
| `--dry-run` | bool | `false` | Show what would be provisioned without creating resources |
| `--local` | bool | `false` | Run locally for interactive debugging (no VM provisioning) |

**Examples:**

```bash
# Work on a single issue
agentium run --repo github.com/org/repo --issues 42

# Work on multiple issues (comma-separated)
agentium run --repo github.com/org/repo --issues 42,43,44 --max-iterations 50

# Work on a range of issues
agentium run --repo github.com/org/repo --issues 42-50

# Mixed syntax (ranges and individual numbers)
agentium run --repo github.com/org/repo --issues 42-45,48,50-52

# Use Aider agent
agentium run --repo github.com/org/repo --issues 42 --agent aider

# Use Codex agent (requires codex --login first for OAuth credentials)
agentium run --repo github.com/org/repo --issues 42 --agent codex

# Override model globally
agentium run --repo github.com/org/repo --issues 42 --model claude-code:claude-opus-4-20250514

# Per-phase model selection
agentium run --repo github.com/org/repo --issues 42 \
  --phase-model "IMPLEMENT=claude-code:claude-opus-4-20250514" \
  --phase-model "IMPLEMENT_REVIEW=aider:claude-3-5-sonnet-20241022"

# Custom instructions
agentium run --repo github.com/org/repo --issues 42 --prompt "Focus on error handling and add tests"

# Longer session with higher iteration limit
agentium run --repo github.com/org/repo --issues 42,43 --max-iterations 100 --max-duration 4h

# Preview without provisioning
agentium run --repo github.com/org/repo --issues 42 --dry-run

# Run locally for interactive debugging (no VM)
export GITHUB_TOKEN=<your-token>
agentium run --local --repo github.com/org/repo --issues 42
```

**Local Mode:**

The `--local` flag runs the controller directly on your machine instead of provisioning a cloud VM. The agent runs in interactive mode, prompting for permission approvals so you can watch and interact with execution in real-time.

Requirements:
- `GITHUB_TOKEN` environment variable must be set
- Docker must be installed and running
- No GitHub App configuration required

This is useful for debugging agent behavior, testing prompt changes, and watching tool calls in real-time.

**Output:**

On success, displays:
- Session ID (e.g., `agentium-abc12345`)
- Instance ID and public IP
- Cloud zone
- Instructions for monitoring with `status`, `logs`, and `destroy`

---

### `agentium status`

Check the status of active sessions.

**Usage:**

```bash
agentium status [session-id] [flags]
```

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `session-id` | No | Specific session to check. If omitted, lists all active sessions. |

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--watch` | bool | `false` | Watch for status changes (polls continuously) |
| `--interval` | duration | `10s` | Watch poll interval (Go duration format, e.g., `10s`, `30s`, `1m`) |

**Examples:**

```bash
# List all active sessions
agentium status

# Check specific session
agentium status agentium-abc12345

# Watch session progress
agentium status agentium-abc12345 --watch

# Watch with custom interval
agentium status agentium-abc12345 --watch --interval 30s
```

**Output (list mode):**

```
SESSION ID              STATE     IP              ZONE            UPTIME
agentium-abc12345       running   34.123.45.67    us-central1-a   15m
agentium-def67890       running   34.123.45.68    us-central1-b   5m
```

**Output (detail mode):**

```
Session:     agentium-abc12345
State:       running
Instance:    agentium-abc12345 (34.123.45.67)
Zone:        us-central1-a
Started:     2024-01-15T10:30:00Z
Uptime:      15m32s
Iterations:  5/30
Tasks:       1 completed, 2 pending
```

---

### `agentium logs`

Retrieve and stream logs from a session.

**Usage:**

```bash
agentium logs <session-id> [flags]
```

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `session-id` | Yes | The session to get logs from |

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--follow`, `-f` | bool | `false` | Follow log output (like `tail -f`) |
| `--tail` | int | `100` | Number of lines from end to display |
| `--since` | string | - | Show logs since timestamp (RFC3339) or duration (e.g., `1h`, `30m`) |
| `--events` | bool | `false` | Show agent events (tool calls, decisions); implies `--level=debug` |
| `--level` | string | `info` | Minimum log level: `debug`, `info`, `warning`, `error` |
| `--type` | string | - | Filter by event type (comma-separated: `text`, `thinking`, `tool_use`, `tool_result`, `command`, `file_change`, `error`, `system`). Specifying `--type` implicitly enables `--events`. |
| `--iteration` | int | `0` | Filter by iteration number (0 = all iterations). Specifying `--iteration` implicitly enables `--events`. |

**Examples:**

```bash
# Get last 100 lines of logs
agentium logs agentium-abc12345

# Follow logs in real-time
agentium logs agentium-abc12345 --follow

# Get last 50 lines
agentium logs agentium-abc12345 --tail 50

# Logs from the last 2 hours
agentium logs agentium-abc12345 --since 2h

# Logs since a specific time
agentium logs agentium-abc12345 --since "2024-01-15T10:30:00Z"

# Show agent events (tool calls, phase transitions)
agentium logs agentium-abc12345 --events

# Filter events by type
agentium logs agentium-abc12345 --events --type tool_use,thinking

# Filter events by iteration
agentium logs agentium-abc12345 --events --iteration 3

# Show only warnings and errors
agentium logs agentium-abc12345 --level warning
```

---

### `agentium destroy`

Force-terminate a session and destroy all associated cloud resources.

**Usage:**

```bash
agentium destroy <session-id> [flags]
```

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `session-id` | Yes | The session to destroy |

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--force`, `-f` | bool | `false` | Skip confirmation prompt |

**Examples:**

```bash
# Destroy with confirmation prompt
agentium destroy agentium-abc12345

# Force destroy (no prompt)
agentium destroy agentium-abc12345 --force
```

**Output:**

```
Are you sure you want to destroy session agentium-abc12345? [y/N] y
Destroying session agentium-abc12345...
Session destroyed successfully.
```

---

## Model Format

When specifying models with `--model` or `--phase-model`, use the format:

```
ADAPTER:MODEL_ID
```

If no colon is present, the entire string is treated as the model with the session's default adapter.

**Examples:**

| Format | Description |
|--------|-------------|
| `claude-code:claude-3-5-sonnet-20241022` | Claude 3.5 Sonnet via Claude Code |
| `claude-code:claude-opus-4-20250514` | Claude Opus 4 via Claude Code |
| `aider:claude-3-5-sonnet-20241022` | Claude 3.5 Sonnet via Aider |
| `claude-opus-4-20250514` | Claude Opus 4 using session's default adapter |

## Phase Model Override Format

For `--phase-model`, use the format:

```
PHASE=ADAPTER:MODEL_ID
```

Where `PHASE` is one of: `PLAN`, `IMPLEMENT`, `DOCS`, `COMPLETE`, `BLOCKED`, `NOTHING_TO_DO`, `PLAN_REVIEW`, `IMPLEMENT_REVIEW`, `DOCS_REVIEW`, `JUDGE`, `PLAN_JUDGE`, `IMPLEMENT_JUDGE`, `DOCS_JUDGE`.

**Example:**

```bash
agentium run --repo github.com/org/repo --issues 42 \
  --phase-model "IMPLEMENT=claude-code:claude-opus-4-20250514" \
  --phase-model "IMPLEMENT_REVIEW=aider:claude-3-5-sonnet-20241022"
```

## Duration Format

Duration values use Go's duration format:

| Format | Description |
|--------|-------------|
| `30m` | 30 minutes |
| `1h` | 1 hour |
| `2h` | 2 hours |
| `1h30m` | 1 hour 30 minutes |
| `4h` | 4 hours |

## Exit Codes

| Code | Description |
|------|-------------|
| `0` | Success |
| `1` | Error (configuration, cloud provider, authentication, or other) |
