# Agentium

An open-source platform for running ephemeral, containerized AI coding agents on cloud VMs. Agentium enables autonomous AI agents to implement GitHub issues, run tests, and create pull requests for human review.

## Overview

Agentium provides secure, automated infrastructure for AI coding agents. When you point it at a GitHub issue, Agentium:

1. Provisions an ephemeral cloud VM
2. Runs an AI agent (Claude Code or Aider) in an isolated container
3. The agent implements the issue, runs tests, and creates a PR
4. The VM automatically terminates when complete

All changes go through pull requests—agents never have production access.

## Key Features

- **Ephemeral Infrastructure**: VMs are created on-demand and automatically destroyed after sessions
- **Container Isolation**: Agents run in isolated Docker containers with minimal privileges
- **Multi-Agent Support**: Works with Claude Code and Aider (extensible to other agents)
- **Multi-Cloud**: GCP support complete, AWS and Azure planned
- **Cost Optimized**: Uses spot/preemptible instances by default
- **PR-Driven Workflow**: All changes require human review via pull requests
- **GitHub App Auth**: Secure authentication without personal access tokens

## Architecture

```
┌─────────────┐     ┌─────────────┐     ┌─────────────────────────────┐
│  Agentium   │────▶│ Provisioner │────▶│      Cloud VM               │
│    CLI      │     │ (Terraform) │     │  ┌───────────────────────┐  │
└─────────────┘     └─────────────┘     │  │  Session Controller   │  │
                                        │  └───────────┬───────────┘  │
                                        │              │              │
                                        │  ┌───────────▼───────────┐  │
                                        │  │   Agent Container     │  │
                                        │  │  (Claude Code/Aider)  │  │
                                        │  └───────────┬───────────┘  │
                                        └──────────────┼──────────────┘
                                                       │
                                                       ▼
                                               ┌───────────────┐
                                               │  GitHub API   │
                                               │  (PRs/Issues) │
                                               └───────────────┘
```

## Supported Agents

| Agent | Description | Status |
|-------|-------------|--------|
| **Claude Code** | Full autonomous coding with Claude 3.5 Sonnet | ✅ Ready |
| **Aider** | Code-focused modifications via Aider CLI | ✅ Ready |

## Supported Cloud Providers

| Provider | Status |
|----------|--------|
| **Google Cloud Platform (GCP)** | ✅ Complete |
| **Amazon Web Services (AWS)** | 🔜 Planned |
| **Microsoft Azure** | 🔜 Planned |

## Getting Started

### Prerequisites

- Go 1.19+
- Terraform 1.0+
- gcloud CLI (authenticated)
- Docker
- A GitHub App with:
  - Repository read/write permissions
  - Issues read/write permissions
  - Pull requests read/write permissions

### Installation

```bash
# Clone the repository
git clone https://github.com/yourusername/agentium.git
cd agentium

# Build the CLI
go build -o agentium ./cmd/agentium

# (Optional) Move to your PATH
mv agentium /usr/local/bin/
```

### GCP Setup

1. Enable required APIs:
   ```bash
   gcloud services enable compute.googleapis.com secretmanager.googleapis.com
   ```

2. Store your GitHub App private key in Secret Manager:
   ```bash
   gcloud secrets create github-app-key --data-file=path/to/private-key.pem
   ```

3. Ensure your account has permissions for Compute Engine and Secret Manager.

### Initialize a Project

```bash
agentium init \
  --repo your-org/your-repo \
  --provider gcp \
  --region us-central1 \
  --app-id YOUR_GITHUB_APP_ID \
  --installation-id YOUR_INSTALLATION_ID
```

This creates a `.agentium.yaml` configuration file in your project.

### Run a Session

```bash
# Implement a single issue
agentium run --repo your-org/your-repo --issues 42

# Implement multiple issues
agentium run --repo your-org/your-repo --issues 42,43,44

# Review and address PR comments
agentium run --repo your-org/your-repo --prs 50

# Use a specific agent
agentium run --repo your-org/your-repo --issues 42 --agent aider
```

### Monitor Sessions

```bash
# List active sessions
agentium status

# Check specific session
agentium status agentium-abc12345

# Watch status in real-time
agentium status agentium-abc12345 --watch

# View logs
agentium logs agentium-abc12345

# Follow logs
agentium logs agentium-abc12345 --follow

# Terminate a session
agentium destroy agentium-abc12345
```

## CLI Commands

| Command | Description |
|---------|-------------|
| `agentium init` | Initialize project configuration |
| `agentium run` | Launch an agent session |
| `agentium status` | Check session status |
| `agentium logs` | View session logs |
| `agentium destroy` | Terminate and clean up a session |

### Run Command Options

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

## Configuration

### Configuration File (`.agentium.yaml`)

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
```

### Environment Variables

All configuration options can be set via environment variables with the `AGENTIUM_` prefix:

```bash
export AGENTIUM_REPO=org/repo
export AGENTIUM_ISSUES=42,43
export AGENTIUM_MAX_ITERATIONS=50
```

### Project-Specific Agent Instructions

Create `.agentium/AGENT.md` in your repository to provide project-specific guidance to agents:

```markdown
# Project Instructions

## Build Commands
- `npm install` - Install dependencies
- `npm run build` - Build the project
- `npm test` - Run tests

## Code Conventions
- Use TypeScript strict mode
- Follow existing patterns in the codebase
- Add tests for new functionality
```

## Session Workflow

1. **Provisioning**: CLI creates a cloud VM with session configuration
2. **Initialization**: Cloud-init installs Docker and pulls the controller image
3. **Controller Start**: Session controller loads config and starts the agent container
4. **Agent Execution**: Agent clones repo, creates branch, implements changes
5. **Iteration Loop**: Agent iterates—implement, test, commit, push—until done
6. **PR Creation**: Agent creates a pull request for human review
7. **Termination**: VM automatically destroys itself when complete

### Termination Conditions

Sessions end when any of these conditions are met:
- All tasks have PRs created
- Maximum iteration count reached
- Session duration limit exceeded
- Agent reports completion or unrecoverable error

## Security Model

- **No Production Credentials**: VMs only have GitHub access—no database, API, or infrastructure credentials
- **Ephemeral Infrastructure**: VMs are automatically deleted after sessions
- **GitHub App Authentication**: Uses GitHub Apps instead of personal access tokens
- **Least Privilege**: Service accounts have minimal required permissions
- **Branch Protection**: Agents cannot commit directly to main/master
- **PR-Based Workflow**: All changes require human review

## Project Structure

```
agentium/
├── cmd/
│   ├── agentium/           # CLI entry point
│   └── controller/         # Session controller
├── internal/
│   ├── cli/                # CLI commands
│   ├── config/             # Configuration management
│   ├── controller/         # Session orchestration
│   ├── agent/              # Agent adapters
│   │   ├── claudecode/     # Claude Code adapter
│   │   └── aider/          # Aider adapter
│   ├── provisioner/        # Cloud provisioners
│   ├── cloud/              # Cloud provider clients
│   └── github/             # GitHub integration
├── terraform/modules/      # Terraform modules
├── docker/                 # Agent Dockerfiles
├── bootstrap/              # Standalone bootstrap system
└── configs/                # Configuration examples
```

## Bootstrap System

For users who want to run sessions without building the Go CLI, a standalone bootstrap system is available:

```bash
cd bootstrap
./run.sh \
  --repo org/repo \
  --issue 42 \
  --app-id 123456 \
  --installation-id 789012 \
  --private-key-secret github-app-key
```

This provides a fully functional GCP-based implementation using only shell scripts and Terraform.

## Development

### Building

```bash
# Build CLI
go build -o agentium ./cmd/agentium

# Build controller
go build -o controller ./cmd/controller
```

### Testing

```bash
# Run all tests
go test ./...

# Run with verbose output
go test ./... -v

# Run specific package
go test ./internal/config/...
```

### Adding a New Agent

1. Create a new package under `internal/agent/`
2. Implement the `Agent` interface
3. Register the adapter in `internal/agent/registry.go`
4. Create a Dockerfile in `docker/`

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/my-feature`)
3. Make your changes
4. Run tests (`go test ./...`)
5. Commit with clear messages
6. Push and create a pull request

See [CLAUDE.md](CLAUDE.md) for detailed development workflow and coding standards.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

Built with:
- [Cobra](https://github.com/spf13/cobra) - CLI framework
- [Viper](https://github.com/spf13/viper) - Configuration management
- [Terraform](https://www.terraform.io/) - Infrastructure provisioning
- [Claude Code](https://claude.ai/code) - AI coding agent
- [Aider](https://aider.chat/) - AI pair programming
