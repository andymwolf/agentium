# Agentium

**Autonomous AI coding agents on ephemeral cloud VMs.** Create an issue, get a pull request.

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/go-1.19%2B-blue.svg)](https://golang.org/)
[![Status: Alpha](https://img.shields.io/badge/status-alpha-orange.svg)](#disclaimer)

Agentium implements the [Ralph Wiggum loop](https://github.com/ghuntley/how-to-ralph-wiggum) pattern for autonomous software development: a controller-as-judge architecture where AI agents plan, implement, test, and self-review code in iterative loops on disposable infrastructure. Each task runs on its own VM, eliminating merge conflicts and environment pollution.

## Key Features

- 🔄 **Ralph Wiggum Loop** — Controller-as-judge phase loop with LLM evaluation
- ☁️ **Ephemeral VMs** — One VM per task, automatically destroyed after completion
- 🚫 **No Code Conflicts** — Each session runs in isolation with a clean clone
- 🔐 **PR-Only Output** — Agents create pull requests for human review (no production access)
- 🚀 **Concurrent Sessions** — Launch multiple sessions in parallel on separate VMs
- 🤖 **Multi-Agent Support** — Claude Code and Aider (more coming soon)
- 💾 **Memory System** — Context persistence between phase iterations
- 🎯 **Model Routing** — Assign different models to different phases
- 🏗️ **Language Auto-Detection** — Automatically installs required runtimes
- 🐛 **Local Debugging** — Run locally with `--local` flag for interactive debugging

## Quick Start

```bash
# Install
git clone https://github.com/andymwolf/agentium.git
cd agentium
go build -o agentium ./cmd/agentium

# Initialize project
agentium init --repo your-org/your-repo --provider gcp

# Run an issue
agentium run --repo your-org/your-repo --issues 42

# Monitor progress
agentium logs agentium-abc12345 --follow
```

## Documentation

- 📖 **[Getting Started](docs/getting-started.md)** — Installation, prerequisites, and detailed quickstart
- ⚙️ **[Configuration Reference](docs/configuration.md)** — Full `.agentium.yaml` reference
- 🔧 **[CLI Reference](docs/cli-reference.md)** — All commands with examples
- ☁️ **[Cloud Setup Guides](docs/cloud-setup/)** — GCP, AWS, Azure setup instructions
- 🔑 **[GitHub App Setup](docs/github-app-setup.md)** — Creating and configuring your GitHub App
- 🆘 **[Troubleshooting](docs/troubleshooting.md)** — Common issues and solutions

## Safety & Security

Agentium's security model is built around **disposability and isolation**:

- **Ephemeral Infrastructure** — VMs are created on-demand and automatically destroyed
- **No Production Access** — Agents only have GitHub API access for creating PRs
- **Branch Protection** — Agents cannot commit directly to main/master branches
- **Session Isolation** — Each VM has its own workspace with no shared state

See [Security Model](docs/security-model.md) for detailed information.

## Roadmap

Based on open issues:

**Infrastructure & Providers**
- AWS and Azure support
- Multi-instance Terraform workspaces

**Agent Ecosystem**
- Codex CLI adapter
- Custom agent templates

**Intelligence & Routing**
- Per-phase cost tracking
- Capability-based model selection

**Developer Experience**
- GitHub Actions integration
- Auto-generated AGENT.md files
- Cost estimation

See all [open issues](https://github.com/andymwolf/agentium/issues) for the complete roadmap.

## Limitations

- **No Dependency Management** — Issues are processed independently
- **Single Cloud Provider** — Currently GCP only (AWS/Azure planned)
- **MacOS-Only OAuth Export** — Claude Code auth export is MacOS-specific
- **No Interactive Feedback** — Agents work autonomously until completion

## Development

```bash
# Build
go build -o agentium ./cmd/agentium

# Test
go test ./...
```

See [CLAUDE.md](CLAUDE.md) for development workflow and [Contributing Guide](docs/contributing.md) for guidelines.

## Disclaimer

This application was entirely vibe-coded with Claude Code. A number of issues have been dogfooded through Agentium itself. Expect rough edges in the current alpha state.

## License

MIT — see [LICENSE](LICENSE).

## Acknowledgments

- [how-to-ralph-wiggum](https://github.com/ghuntley/how-to-ralph-wiggum) — The Ralph Wiggum loop pattern
- [Claude Code](https://claude.ai/code) & [Aider](https://aider.chat/) — AI coding agents
- [Cobra](https://github.com/spf13/cobra) / [Viper](https://github.com/spf13/viper) — CLI framework