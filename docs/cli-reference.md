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

**Examples:**

```bash
# Basic initialization
agentium init --repo github.com/myorg/myrepo --provider gcp

# Full initialization with GitHub App credentials
agentium init \
  --repo github.com/myorg/myrepo \
  --provider gcp \
  --region us-east1 \
  --app-id 123456 \
  --installation-id 789012

# Overwrite existing config
agentium init --repo github.com/myorg/myrepo --force
```

**Output:**

Creates `.agentium.yaml` with the specified configuration. If a file already exists and `--force` is not set, the command will exit with an error.

---

### `agentium run`

Launch an ephemeral agent session to work on GitHub issues or review PRs.

**Usage:**

```bash
agentium run [flags]
```

**Required Flags:**

- `--repo` is always required (must be provided on the command line; setting `project.repository` in config is not sufficient)
- At least one of `--issues` or `--prs` must be specified

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--repo` | string | **Required** | GitHub repository (e.g., `github.com/org/repo`) |
| `--issues` | string | - | Issue numbers to work on (comma-separated) |
| `--prs` | string | - | PR numbers for review sessions (comma-separated) |
| `--agent` | string | `claude-code` | Agent to use: `claude-code`, `aider` |
| `--max-iterations` | int | `30` | Maximum iterations before termination |
| `--max-duration` | string | `2h` | Maximum session duration |
| `--provider` | string | From config | Cloud provider: `gcp`, `aws`, `azure` (required if not in config) |
| `--region` | string | From config | Cloud region (overrides config) |
| `--prompt` | string | - | Custom prompt/instructions for the agent |
| `--model` | string | - | Override model for all phases (format: `adapter:model`) |
| `--phase-model` | string | - | Per-phase model override (repeatable, format: `PHASE=adapter:model`) |
| `--claude-auth-mode` | string | `api` | Claude authentication: `api`, `oauth` |
| `--dry-run` | bool | `false` | Show what would be provisioned without creating resources |

**Examples:**

```bash
# Work on a single issue
agentium run --repo github.com/org/repo --issues 42

# Work on multiple issues
agentium run --repo github.com/org/repo --issues 42,43,44 --max-iterations 50

# PR review session
agentium run --repo github.com/org/repo --prs 50

# Use Aider agent
agentium run --repo github.com/org/repo --issues 42 --agent aider

# Override model globally
agentium run --repo github.com/org/repo --issues 42 --model claude-code:claude-opus-4-20250514

# Per-phase model selection
agentium run --repo github.com/org/repo --issues 42 \
  --phase-model "IMPLEMENT=claude-code:claude-opus-4-20250514" \
  --phase-model "TEST=aider:claude-3-5-sonnet-20241022"

# Custom instructions
agentium run --repo github.com/org/repo --issues 42 --prompt "Focus on error handling and add tests"

# Longer session with higher iteration limit
agentium run --repo github.com/org/repo --issues 42,43 --max-iterations 100 --max-duration 4h

# Preview without provisioning
agentium run --repo github.com/org/repo --issues 42 --dry-run
```

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

Where `PHASE` is one of: `IMPLEMENT`, `TEST`, `PR_CREATION`, `REVIEW`, `ANALYZE`, `COMPLETE`, `BLOCKED`, `NOTHING_TO_DO`, `PUSH`.

**Example:**

```bash
agentium run --repo github.com/org/repo --issues 42 \
  --phase-model "IMPLEMENT=claude-code:claude-opus-4-20250514" \
  --phase-model "TEST=aider:claude-3-5-sonnet-20241022"
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
