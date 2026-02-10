# Configuration Reference

Agentium is configured through a YAML file (`.agentium.yaml`) in your project root. Configuration values can also be set via environment variables or CLI flags.

## Configuration Precedence

Values are resolved in this order (highest priority first):

1. **CLI flags** - Command-line arguments
2. **Environment variables** - Prefixed with `AGENTIUM_`
3. **Config file** - `.agentium.yaml`
4. **Defaults** - Built-in default values

## Creating a Config File

Use `agentium init` to generate a config file with sensible defaults:

```bash
agentium init --repo github.com/org/repo --provider gcp --region us-central1
```

## Full Configuration Reference

```yaml
# Project metadata
project:
  name: "my-project"                # Project identifier (defaults to directory name)
  repository: "github.com/org/repo" # GitHub repository URL (required for run)

# GitHub App authentication
github:
  app_id: 123456                    # GitHub App ID (required)
  installation_id: 789012           # GitHub App Installation ID (required)
  private_key_secret: "projects/my-gcp-project/secrets/github-app-key"
                                    # Cloud secret path for the private key (required)

# Cloud provider configuration
cloud:
  provider: "gcp"                   # Cloud provider: gcp, aws, azure (required)
  region: "us-central1"             # Cloud region (required)
  project: "my-gcp-project"         # GCP project ID (required for GCP)
  machine_type: "e2-medium"         # VM instance type
  use_spot: true                    # Use spot/preemptible instances
  disk_size_gb: 50                  # Root disk size in GB
  service_account_key: ""           # Path to GCP service account JSON key file (optional)

# Default session settings
defaults:
  max_iterations: 30                # Maximum agent iterations per session
  max_duration: "2h"                # Maximum session duration (Go duration format)

# Codex agent authentication
codex:
  auth_json_path: "~/.codex/auth.json"  # Path to Codex OAuth credentials

# Claude AI authentication
claude:
  auth_mode: "api"                  # Authentication mode: api, oauth
  auth_json_path: "~/.config/claude-code/auth.json"
                                    # Path to OAuth credentials (for oauth mode)

# Session controller
controller:
  image: "ghcr.io/andymwolf/agentium-controller:latest"
                                    # Controller container image

# Phase loop (controller-as-judge)
phase_loop:
  enabled: true                     # Enable evaluator-driven phase loop
  plan_max_iterations: 3            # Max PLAN phase iterations
  implement_max_iterations: 5       # Max IMPLEMENT phase iterations
  test_max_iterations: 5            # Max TEST phase iterations
  review_max_iterations: 3          # Max REVIEW phase iterations

# Model routing (per-phase model selection)
routing:
  default:                          # Default model for all phases
    adapter: "claude-code"          # Default agent adapter (required)
    model: "claude-opus-4-20250514"
    fallback_enabled: true          # Fallback to claude-code on adapter failure
  overrides:                        # Per-phase overrides
    IMPLEMENT:
      adapter: "claude-code"
      model: "claude-opus-4-20250514"
    IMPLEMENT_REVIEW:               # Higher reasoning for code review
      adapter: "codex"
      model: "o3"
      reasoning: "high"
    TEST:
      adapter: "aider"
      model: "claude-3-5-sonnet-20241022"

# Sub-agent delegation (experimental)
delegation:
  enabled: false                    # Enable sub-agent delegation
  strategy: "sequential"            # Delegation strategy (only "sequential" supported)
  sub_agents:                       # Sub-agent definitions by task type
    review:
      agent: "claude-code"
      model:
        adapter: "claude-code"
        model: "claude-opus-4-20250514"
      skills:
        - "code_review"
        - "lint_detection"

# Monorepo support (auto-detected for pnpm workspaces)
monorepo:
  enabled: true                     # Enable package scope enforcement
  label_prefix: "pkg"               # Prefix for package labels (pkg:core, pkg:web)
```

## Configuration Sections

### project

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | No | Directory name | Human-readable project identifier |
| `repository` | string | Yes (for `run`) | - | GitHub repository in `github.com/org/repo` format |

### github

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `app_id` | int64 | Yes | - | GitHub App ID from app settings page |
| `installation_id` | int64 | Yes | - | Installation ID for your org/repo |
| `private_key_secret` | string | Yes | - | Cloud secret path containing the private key PEM |

### cloud

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `provider` | string | Yes | - | Cloud provider: `gcp`, `aws`, `azure` |
| `region` | string | Yes | - | Cloud region (e.g., `us-central1`, `us-east-1`) |
| `project` | string | For GCP | - | GCP project ID |
| `machine_type` | string | No | `e2-medium` | VM instance type (see below) |
| `use_spot` | bool | No | `false` | Use spot/preemptible instances for cost savings (recommended: set to `true` explicitly) |
| `disk_size_gb` | int | No | `50` | Root disk size in GB |
| `service_account_key` | string | No | - | Path to GCP service account JSON key file. When set, all Terraform and gcloud commands authenticate with this key instead of ambient credentials. See [GCP Service Account Authentication](cloud-setup/gcp.md#service-account-authentication). |

**Machine type defaults by provider:**

| Provider | Default | Examples |
|----------|---------|----------|
| GCP | `e2-medium` | `e2-micro`, `e2-small`, `e2-medium`, `e2-standard-2` |
| AWS | `t3.medium` | `t3.micro`, `t3.small`, `t3.medium`, `t3.large` |
| Azure | `Standard_B2s` | `Standard_B1s`, `Standard_B2s`, `Standard_B4ms` |

### defaults

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `max_iterations` | int | No | `30` | Max iterations before session termination |
| `max_duration` | string | No | `2h` | Max session duration (Go duration: `30m`, `2h`, `4h`) |

### codex

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `auth_json_path` | string | No | `~/.codex/auth.json` | Path to Codex OAuth credentials file. On macOS, Agentium also checks the Keychain. |

Example:

```yaml
codex:
  auth_json_path: "~/.codex/auth.json"
```

> **Note:** To set up Codex credentials, install Codex (`bun add -g @openai/codex`) and run `codex --login`. Agentium reads the cached credentials and transfers them to the VM automatically.

### claude

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `auth_mode` | string | No | `api` | Authentication mode: `api` or `oauth` |
| `auth_json_path` | string | No | `~/.config/claude-code/auth.json` | OAuth credentials file path |

**Authentication modes:**

- **`api`** - Uses the `ANTHROPIC_API_KEY` environment variable. Simple setup, requires a long-lived API key.
- **`oauth`** - Uses Claude Code OAuth credentials from an `auth.json` file. More secure for local usage. On macOS, Agentium will also check the macOS Keychain for Claude Code credentials if the file is not found.

> **Note:** OAuth auth mode is only supported with the `claude-code` agent. To set up OAuth credentials, install Claude Code (`bun add -g @anthropic-ai/claude-code`) and run `claude login`.

### controller

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `image` | string | No | `ghcr.io/andymwolf/agentium-controller:latest` | Session controller container image |

### routing

Model routing enables per-phase model selection for optimizing cost and performance. The `routing.default.adapter` field specifies the default agent adapter used for all phases.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `default.adapter` | string | No | `claude-code` | Default agent adapter (`claude-code`, `aider`, `codex`) |
| `default.model` | string | No | - | Default model ID |
| `default.reasoning` | string | No | - | Reasoning effort level (codex only) |
| `default.fallback_enabled` | bool | No | `false` | Enable fallback to `claude-code` on adapter failure |
| `overrides.<PHASE>.adapter` | string | No | - | Agent adapter for specific phase |
| `overrides.<PHASE>.model` | string | No | - | Model for specific phase |
| `overrides.<PHASE>.reasoning` | string | No | - | Reasoning effort level for phase (codex only) |

**Adapter fallback:**

When `fallback_enabled` is `true`, if the primary adapter fails with a startup or infrastructure error (e.g., missing auth file, Docker error, permission denied), the controller automatically retries with `claude-code`. This prevents session failures due to adapter configuration issues.

**Reasoning effort levels (codex agent only):**

| Level | Description |
|-------|-------------|
| `minimal` | Minimal reasoning, fastest response |
| `low` | Low reasoning effort |
| `medium` | Medium reasoning effort (recommended default) |
| `high` | High reasoning effort |
| `xhigh` | Extra high reasoning, longest thinking time (model-dependent) |

**Recognized phases:**

| Phase | Description |
|-------|-------------|
| `PLAN` | Planning the implementation approach |
| `IMPLEMENT` | Main feature implementation |
| `DOCS` | Documentation updates |
| `COMPLETE` | Session completion |
| `BLOCKED` | Agent blocked, needs human intervention |
| `NOTHING_TO_DO` | No changes required |
| `PLAN_REVIEW` | Review of plan phase output |
| `IMPLEMENT_REVIEW` | Review of implementation phase output |
| `DOCS_REVIEW` | Review of documentation phase output |
| `JUDGE` | Judge phase for evaluating work |
| `PLAN_JUDGE` | Judge for plan phase |
| `IMPLEMENT_JUDGE` | Judge for implementation phase |
| `DOCS_JUDGE` | Judge for documentation phase |

### phase_loop

Controls the controller-as-judge phase loop behavior. When enabled, the controller runs an LLM evaluator after each phase to decide whether to advance, iterate, or block.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `enabled` | bool | No | `false` | Enable the phase loop with evaluator |
| `plan_max_iterations` | int | No | `3` | Max iterations for the PLAN phase |
| `implement_max_iterations` | int | No | `5` | Max iterations for the IMPLEMENT phase |
| `docs_max_iterations` | int | No | `3` | Max iterations for the DOCS phase |
| `judge_context_budget` | int | No | `8000` | Max characters of judge output to store as context |

**Phase loop sequence for issues:**

```
PLAN → [PLAN_JUDGE] → IMPLEMENT → [IMPLEMENT_JUDGE] → DOCS → [DOCS_JUDGE] → COMPLETE
```

After each phase, the judge produces a verdict:
- **ADVANCE** — Proceed to next phase
- **ITERATE** — Re-run current phase with feedback (up to max iterations)
- **BLOCKED** — Stop and signal human intervention needed

Evaluator feedback from ITERATE verdicts is stored in the memory system and provided as context in the next iteration of that phase.

### monorepo

Configuration for pnpm workspace monorepo support. Automatically set by `agentium init` when `pnpm-workspace.yaml` is detected.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `enabled` | bool | No | `false` | Enable monorepo mode with package scope enforcement |
| `label_prefix` | string | No | `pkg` | Prefix for package labels (e.g., `pkg:core`, `pkg:web`) |

**Monorepo behavior:**

When `monorepo.enabled` is `true`:
- Issues must have a `<prefix>:<package-name>` label to specify the target package
- The agent can only modify files within the target package directory
- Out-of-scope file changes are automatically reset and block the iteration
- Allowed exceptions: root `package.json`, `pnpm-lock.yaml`, `pnpm-workspace.yaml`, `.github/workflows/`
- Hierarchical AGENTS.md loading: root + package-specific instructions are merged

**Example:**

```yaml
monorepo:
  enabled: true
  label_prefix: "pkg"  # Issues need labels like pkg:core, pkg:api
```

**Package-specific agent instructions:**

Create `AGENTS.md` within a package directory to provide package-specific instructions (e.g., `packages/core/AGENTS.md`). These are merged with the root `AGENTS.md` when the agent targets that package.

### delegation

Sub-agent delegation (experimental feature).

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `enabled` | bool | No | `false` | Enable sub-agent delegation |
| `strategy` | string | No | `sequential` | Delegation strategy (only `sequential` is currently supported) |
| `sub_agents` | map | No | - | Named sub-agent configurations |

## Session Configuration

Session-level settings (repository, issues, agent, etc.) are derived at runtime from CLI flags and config file defaults. They are **not** intended to be set directly in the config file. Instead:

- `--repo`, `--issues`, `--agent`, `--max-iterations`, `--max-duration` are passed as CLI flags to `agentium run`
- If `--agent` is not provided, it falls back to `routing.default.adapter` (or `claude-code` if not configured)
- `max_iterations` and `max_duration` fall back to values in the `defaults` section
- The `repository` field falls back to `project.repository` if `--repo` is not provided (though `--repo` is always required for `run`)

## Environment Variables

All configuration values can be set via environment variables with the `AGENTIUM_` prefix. Nested fields use underscores:

| Environment Variable | Config Field |
|---------------------|-------------|
| `AGENTIUM_PROJECT_NAME` | `project.name` |
| `AGENTIUM_PROJECT_REPOSITORY` | `project.repository` |
| `AGENTIUM_GITHUB_APP_ID` | `github.app_id` |
| `AGENTIUM_GITHUB_INSTALLATION_ID` | `github.installation_id` |
| `AGENTIUM_CLOUD_PROVIDER` | `cloud.provider` |
| `AGENTIUM_CLOUD_REGION` | `cloud.region` |
| `AGENTIUM_CLOUD_PROJECT` | `cloud.project` |
| `AGENTIUM_DEFAULTS_MAX_ITERATIONS` | `defaults.max_iterations` |
| `AGENTIUM_DEFAULTS_MAX_DURATION` | `defaults.max_duration` |
| `AGENTIUM_ROUTING_DEFAULT_ADAPTER` | `routing.default.adapter` |
| `AGENTIUM_CLAUDE_AUTH_MODE` | `claude.auth_mode` |
| `AGENTIUM_CONTROLLER_IMAGE` | `controller.image` |

> **Note:** Agentium uses Viper's automatic environment variable binding. Any config field can be overridden by setting `AGENTIUM_` followed by the uppercase path with underscores replacing dots. For example, `cloud.disk_size_gb` becomes `AGENTIUM_CLOUD_DISK_SIZE_GB`.

## Project-Specific Agent Instructions

You can provide project-specific instructions to the AI agent by creating an `AGENTS.md` file in your repository root. This file is automatically injected into the agent's system prompt.

**Example `AGENTS.md`:**

```markdown
# Project Agent Instructions

## Build & Test Commands
```bash
go build ./...
go test ./...
golangci-lint run
```

## Code Conventions
- Follow existing code style
- Use descriptive variable names
- Add comments for complex logic

## Architecture Notes
- This is a Go project using standard library HTTP server
- Database access uses sqlx with PostgreSQL
- Configuration uses Viper

## Testing Requirements
- All new code must have unit tests
- Integration tests go in `*_integration_test.go` files
- Run `make test` before submitting

## Off-Limits Areas
- `.github/workflows/` - Do not modify CI/CD pipelines
- `migrations/` - Database migrations need manual review
```

## Example Configurations

### Minimal Configuration

```yaml
project:
  repository: "github.com/org/repo"

github:
  app_id: 123456
  installation_id: 789012
  private_key_secret: "projects/my-project/secrets/github-app-key"

cloud:
  provider: "gcp"
  region: "us-central1"
  project: "my-gcp-project"
```

### Full-Featured Configuration

```yaml
project:
  name: "my-webapp"
  repository: "github.com/org/repo"

github:
  app_id: 123456
  installation_id: 789012
  private_key_secret: "projects/my-project/secrets/github-app-key"

cloud:
  provider: "gcp"
  region: "us-central1"
  project: "my-gcp-project"
  machine_type: "e2-standard-2"
  use_spot: true
  disk_size_gb: 100

defaults:
  max_iterations: 50
  max_duration: "4h"

claude:
  auth_mode: "api"

routing:
  default:
    adapter: "claude-code"
    model: "claude-opus-4-20250514"
    fallback_enabled: true
  overrides:
    IMPLEMENT:
      adapter: "claude-code"
      model: "claude-opus-4-20250514"
```

### Cost-Optimized Configuration

```yaml
project:
  repository: "github.com/org/repo"

github:
  app_id: 123456
  installation_id: 789012
  private_key_secret: "projects/my-project/secrets/github-app-key"

cloud:
  provider: "gcp"
  region: "us-central1"
  project: "my-gcp-project"
  machine_type: "e2-small"
  use_spot: true
  disk_size_gb: 30

defaults:
  max_iterations: 20
  max_duration: "1h"

routing:
  default:
    adapter: "claude-code"
    model: "claude-sonnet-4-20250514"  # Use Sonnet for cost savings
```
