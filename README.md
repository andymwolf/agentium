# Agentium

**Autonomous AI coding agents on ephemeral cloud VMs.** Create an issue, get a pull request.

Agentium implements the [Ralph Wiggum loop](https://github.com/ghuntley/how-to-ralph-wiggum) pattern for autonomous software development: a controller-as-judge architecture where AI agents plan, implement, test, and self-review code in iterative loops on disposable infrastructure. Each task runs on its own VM, so there are no merge conflicts, no local environment pollution, and no risk to production.

Inspired by the Ralph Wiggum philosophy of "let the agent ralph"—lean on iteration and self-correction rather than prescribing everything upfront.

## Who Is This For?

Agentium is intended for people comfortable using agentic coding tools from the command line. If you've used Claude Code, Aider, or similar CLI-based AI tools, you'll feel at home. That said, the workflow is designed to be accessible to non-coders too: create a well-described GitHub issue, and Agentium handles the rest.

## The Ralph Wiggum Loop

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

### Phase Iterations

Each phase has configurable iteration limits to prevent runaway loops:

```yaml
phase_loop:
  enabled: true
  plan_max_iterations: 3
  build_max_iterations: 5
  review_max_iterations: 3
```

Every phase iteration and evaluator verdict is posted as a comment on the GitHub issue, giving you full visibility into the agent's reasoning.

## Key Features

- **Ralph Wiggum Loop Architecture** — Controller-as-judge phase loop with LLM evaluation between phases
- **Ephemeral VMs** — One VM per task, automatically destroyed after completion. No state leakage between sessions
- **No Code Conflicts** — Each session runs in isolation on its own VM with a clean clone, eliminating the merge conflicts that plague local multi-agent setups
- **PR-Only Output** — Agents never have production access. All changes go through pull requests for human review
- **Concurrent Sessions** — Launch multiple sessions in parallel, each on its own VM
- **Multi-Agent Support** — Works with Claude Code and Aider (extensible to other agents)
- **Multi-Cloud** — GCP support complete, AWS and Azure planned
- **Structured Logging** — Cloud-native structured logging with real-time streaming, severity filtering, and agent event extraction
- **Cost Optimized** — Uses spot/preemptible instances by default
- **Memory System** — Persistent memory carries context (key facts, decisions, evaluator feedback) between phase iterations
- **Model Routing** — Assign different models to different phases (e.g., Opus for planning, Haiku for implementation)
- **Language Runtime Auto-Detection** — Automatically installs Go, Rust, Java, Ruby, or .NET runtimes based on project type

## Safety and Security

Running AI agents autonomously requires careful guardrails. Agentium's security model is built around disposability and isolation:

### Ephemeral Infrastructure
- VMs are created on-demand and **automatically destroyed** after sessions
- No persistent state between sessions—each run starts clean
- Credentials are cleared from memory before VM termination
- Spot instances reduce cost and reinforce the ephemeral model

### Permission Model
Agents run with `--dangerously-skip-permissions` (Claude Code's headless mode) on the ephemeral VM. This is safe because:
- The VM has **no production credentials**—only GitHub API access
- The agent can only create branches and pull requests
- The VM self-terminates, so any filesystem changes are destroyed
- All output is a PR that requires human review before merging

### Isolation Benefits
- **No shared filesystem** — Unlike running multiple agents locally, each VM has its own workspace
- **No credential exposure** — GitHub tokens are generated per-session via GitHub App JWT exchange, never persisted to disk
- **No lateral movement** — VMs have no access to other infrastructure
- **Branch protection enforced** — Agents cannot commit directly to main/master

### Security Enhancements (v0.2.0)
- **Credential Scrubbing** — All logs are automatically scrubbed of sensitive patterns (API keys, tokens, passwords)
- **Secret Management** — Secrets stored in cloud provider secret managers, never in code
- **Least Privilege IAM** — VMs run with minimal permissions: read secrets, write logs, self-delete
- **Secure Auth Files** — Authentication files mounted with restrictive permissions (0600)
- **Network Restrictions** — Documented approach for limiting egress to required endpoints only

For detailed security information, see [SECURITY-REVIEW.md](docs/SECURITY-REVIEW.md).

### Disclaimer
This application was entirely vibe-coded with Claude Code (with some Codex review). A number of issues have been dogfooded through Agentium itself, and that is the intended long-term maintenance approach. Evals have been difficult without the full logging infrastructure in place, so expect rough edges in the current state.

## Limitations

- **No Dependency Management** — Issues are processed independently. If issue B depends on issue A, you must ensure A is completed and merged before authorizing B. Users need to be dependency-aware when queuing work.
- **Single Cloud Provider** — Currently GCP only. AWS and Azure support is planned.
- **MacOS-Only Auth Export** — The OAuth credential export feature (copying Claude Code auth from local machine) is currently MacOS-specific.
- **No Interactive Feedback** — Once a session starts, you cannot provide mid-session guidance. The agent works autonomously until completion or failure.

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

## Getting Started

### Prerequisites

- Go 1.19+
- Terraform 1.0+
- gcloud CLI (authenticated)
- Docker
- A GitHub App with repository, issues, and pull request read/write permissions

### Installation

```bash
git clone https://github.com/andymwolf/agentium.git
cd agentium
go build -o agentium ./cmd/agentium
mv agentium /usr/local/bin/  # Optional
```

### Quick Start

```bash
# 1. Initialize your project
agentium init \
  --repo your-org/your-repo \
  --provider gcp \
  --region us-central1 \
  --app-id YOUR_GITHUB_APP_ID \
  --installation-id YOUR_INSTALLATION_ID

# 2. Run an issue
agentium run --repo your-org/your-repo --issues 42

# 3. Monitor progress
agentium logs agentium-abc12345 --follow

# 4. Review the PR when it appears
```

### Running Multiple Issues

```bash
# Multiple issues on one VM, completed sequentially
agentium run --repo your-org/your-repo --issues 42,43,44

# For true concurrency, launch separate sessions
agentium run --repo your-org/your-repo --issues 42 &
agentium run --repo your-org/your-repo --issues 43 &
agentium run --repo your-org/your-repo --issues 44 &
```

### Monitoring

```bash
agentium status                              # List active sessions
agentium status agentium-abc12345 --watch    # Real-time status
agentium logs agentium-abc12345 --follow     # Stream logs
agentium destroy agentium-abc12345           # Terminate a session
```

## Documentation

For comprehensive documentation, see the [`docs/`](docs/) directory:

- **[Getting Started](docs/getting-started.md)** - Installation, prerequisites, and quickstart
- **[Configuration Reference](docs/configuration.md)** - Full `.agentium.yaml` reference
- **[CLI Reference](docs/cli-reference.md)** - All commands with flags and examples
- **[Cloud Setup Guides](docs/cloud-setup/)** - Provider-specific setup (GCP, AWS, Azure)
- **[GitHub App Setup](docs/github-app-setup.md)** - Creating and configuring your GitHub App
- **[Troubleshooting](docs/troubleshooting.md)** - Common issues and solutions

## Configuration

### `.agentium.yaml`

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

### Project-Specific Agent Instructions

Create `.agentium/AGENT.md` in your repository to guide the agent:

```markdown
# Project Instructions

## Build Commands
- `npm install` - Install dependencies
- `npm test` - Run tests

## Code Conventions
- Use TypeScript strict mode
- Add tests for new functionality
```

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

## Roadmap

Based on open issues, here's what's planned:

### Infrastructure & Providers
- AWS and Azure provisioner support
- Terraform workspaces for multi-instance awareness (#103)
- Graceful shutdown with `--force` flag (#119)

### Agent Ecosystem
- Codex CLI agent adapter (#99)
- Agent-agnostic event abstraction (#116)
- Custom agent template support (#14)

### Intelligence & Routing
- Per-phase cost tracking and reporting (#106)
- Per-phase token limits and temperature routing (#105)
- Capability-based model selection (#104)

### Developer Experience
- GitHub Actions workflow for triggering from issues (#98)
- Simplified init with project scanner + AGENT.md auto-generation (#100)
- Guided infrastructure setup wizard (#20)
- Cost estimation for sessions (#19)

### Reliability
- Installation token refresh mechanism (#62)
- CI/CD pipeline (#23)
- Security audit and hardening (#25)

## CLI Reference

| Command | Description |
|---------|-------------|
| `agentium init` | Initialize project configuration |
| `agentium run` | Launch an agent session |
| `agentium status` | Check session status |
| `agentium logs` | View session logs |
| `agentium destroy` | Terminate and clean up a session |

### `agentium run` Options

| Flag | Description | Default |
|------|-------------|---------|
| `--repo` | Target GitHub repository | From config |
| `--issues` | Issue numbers (comma-separated) | - |
| `--prs` | PR numbers for review sessions | - |
| `--agent` | Agent to use (`claude-code`, `aider`) | `claude-code` |
| `--max-iterations` | Maximum iteration count | 30 |
| `--max-duration` | Session timeout | 2h |
| `--provider` | Cloud provider | From config |
| `--region` | Cloud region | From config |
| `--dry-run` | Preview without creating resources | false |
| `--prompt` | Custom prompt for agent | - |

## Development

```bash
# Build
go build -o agentium ./cmd/agentium
go build -o controller ./cmd/controller

# Test
go test ./...

# Add a new agent adapter
# 1. Create package under internal/agent/
# 2. Implement the Agent interface
# 3. Register in internal/agent/registry.go
# 4. Create Dockerfile in docker/
```

See [CLAUDE.md](CLAUDE.md) for detailed development workflow and coding standards.

## Acknowledgments

- [how-to-ralph-wiggum](https://github.com/ghuntley/how-to-ralph-wiggum) — The Ralph Wiggum loop pattern that inspired Agentium's architecture
- [Claude Code](https://claude.ai/code) — AI coding agent
- [Aider](https://aider.chat/) — AI pair programming
- [Cobra](https://github.com/spf13/cobra) / [Viper](https://github.com/spf13/viper) — CLI framework
- [Terraform](https://www.terraform.io/) — Infrastructure provisioning

## License

MIT — see [LICENSE](LICENSE).
