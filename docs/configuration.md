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

# Default session settings
defaults:
  agent: "claude-code"              # Default agent: claude-code, aider, codex
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

# Prompts configuration
prompts:
  system_md_url: ""                 # Override URL for SYSTEM.md prompt
  fetch_timeout: "5s"               # Timeout for fetching remote prompts

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
    adapter: "claude-code"
    model: "claude-opus-4-20250514"
  overrides:                        # Per-phase overrides
    IMPLEMENT:
      adapter: "claude-code"
      model: "claude-opus-4-20250514"
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

**Machine type defaults by provider:**

| Provider | Default | Examples |
|----------|---------|----------|
| GCP | `e2-medium` | `e2-micro`, `e2-small`, `e2-medium`, `e2-standard-2` |
| AWS | `t3.medium` | `t3.micro`, `t3.small`, `t3.medium`, `t3.large` |
| Azure | `Standard_B2s` | `Standard_B1s`, `Standard_B2s`, `Standard_B4ms` |

### defaults

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `agent` | string | No | `claude-code` | Default agent adapter (`claude-code`, `aider`, `codex`) |
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

> **Note:** To set up Codex credentials, install Codex (`npm install -g @openai/codex`) and run `codex --login`. Agentium reads the cached credentials and transfers them to the VM automatically.

### claude

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `auth_mode` | string | No | `api` | Authentication mode: `api` or `oauth` |
| `auth_json_path` | string | No | `~/.config/claude-code/auth.json` | OAuth credentials file path |

**Authentication modes:**

- **`api`** - Uses the `ANTHROPIC_API_KEY` environment variable. Simple setup, requires a long-lived API key.
- **`oauth`** - Uses Claude Code OAuth credentials from an `auth.json` file. More secure for local usage. On macOS, Agentium will also check the macOS Keychain for Claude Code credentials if the file is not found.

> **Note:** OAuth auth mode is only supported with the `claude-code` agent. To set up OAuth credentials, install Claude Code (`npm install -g @anthropic-ai/claude-code`) and run `claude login`.

### controller

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `image` | string | No | `ghcr.io/andymwolf/agentium-controller:latest` | Session controller container image |

### prompts

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `system_md_url` | string | No | Auto-fetched from repo | Override URL for the SYSTEM.md agent prompt |
| `fetch_timeout` | string | No | `5s` | Timeout for fetching remote prompt files |

### routing

Model routing enables per-phase model selection for optimizing cost and performance.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `default.adapter` | string | No | - | Default agent adapter (e.g., `claude-code`, `aider`) |
| `default.model` | string | No | - | Default model ID |
| `overrides.<PHASE>.adapter` | string | No | - | Agent adapter for specific phase |
| `overrides.<PHASE>.model` | string | No | - | Model for specific phase |

**Recognized phases:**

| Phase | Description |
|-------|-------------|
| `PLAN` | Planning the implementation approach |
| `IMPLEMENT` | Main feature implementation |
| `TEST` | Test execution and fixing |
| `PR_CREATION` | Creating pull requests |
| `REVIEW` | Reviewing own changes |
| `EVALUATE` | LLM evaluator phase (controller-as-judge) |
| `ANALYZE` | Analysis phase (used for PR reviews) |
| `COMPLETE` | Session completion |
| `BLOCKED` | Agent blocked, needs human intervention |
| `NOTHING_TO_DO` | No changes required |
| `PUSH` | Pushing changes to remote |

### phase_loop

Controls the controller-as-judge phase loop behavior. When enabled, the controller runs an LLM evaluator after each phase to decide whether to advance, iterate, or block.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `enabled` | bool | No | `false` | Enable the phase loop with evaluator |
| `plan_max_iterations` | int | No | `3` | Max iterations for the PLAN phase |
| `implement_max_iterations` | int | No | `5` | Max iterations for the IMPLEMENT phase |
| `test_max_iterations` | int | No | `5` | Max iterations for the TEST phase |
| `review_max_iterations` | int | No | `3` | Max iterations for the REVIEW phase |
| `eval_context_budget` | int | No | `8000` | Max characters of evaluator output to store as context |

**Phase loop sequence for issues:**

```
PLAN → [EVALUATE] → IMPLEMENT → [EVALUATE] → TEST → [EVALUATE] → REVIEW → [EVALUATE] → PR_CREATION → COMPLETE
```

After each phase, the evaluator produces a verdict:
- **ADVANCE** — Proceed to next phase
- **ITERATE** — Re-run current phase with feedback (up to max iterations)
- **BLOCKED** — Stop and signal human intervention needed

Evaluator feedback from ITERATE verdicts is stored in the memory system and provided as context in the next iteration of that phase.

### delegation

Sub-agent delegation (experimental feature).

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `enabled` | bool | No | `false` | Enable sub-agent delegation |
| `strategy` | string | No | `sequential` | Delegation strategy (only `sequential` is currently supported) |
| `sub_agents` | map | No | - | Named sub-agent configurations |

## Session Configuration

Session-level settings (repository, issues, agent, etc.) are derived at runtime from CLI flags and config file defaults. They are **not** intended to be set directly in the config file. Instead:

- `--repo`, `--issues`, `--prs`, `--agent`, `--max-iterations`, `--max-duration` are passed as CLI flags to `agentium run`
- If not provided, `agent`, `max_iterations`, and `max_duration` fall back to values in the `defaults` section
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
| `AGENTIUM_DEFAULTS_AGENT` | `defaults.agent` |
| `AGENTIUM_DEFAULTS_MAX_ITERATIONS` | `defaults.max_iterations` |
| `AGENTIUM_DEFAULTS_MAX_DURATION` | `defaults.max_duration` |
| `AGENTIUM_CLAUDE_AUTH_MODE` | `claude.auth_mode` |
| `AGENTIUM_CONTROLLER_IMAGE` | `controller.image` |

> **Note:** Agentium uses Viper's automatic environment variable binding. Any config field can be overridden by setting `AGENTIUM_` followed by the uppercase path with underscores replacing dots. For example, `cloud.disk_size_gb` becomes `AGENTIUM_CLOUD_DISK_SIZE_GB`.

## Project-Specific Agent Instructions

You can provide project-specific instructions to the AI agent by creating a `.agentium/AGENT.md` file in your repository root. This file is automatically injected into the agent's system prompt.

**Example `.agentium/AGENT.md`:**

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
  agent: "claude-code"
  max_iterations: 50
  max_duration: "4h"

claude:
  auth_mode: "api"

routing:
  default:
    adapter: "claude-code"
    model: "claude-opus-4-20250514"
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
  agent: "claude-code"
  max_iterations: 20
  max_duration: "1h"
```
