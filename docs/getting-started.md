# Getting Started with Agentium

Agentium is an open-source platform for running ephemeral, containerized AI coding agents on cloud VMs. It automates GitHub issue implementation using AI agents (Claude Code or Aider) and creates pull requests for human review.

## How It Works

1. You point Agentium at a GitHub issue
2. Agentium provisions an ephemeral cloud VM
3. An AI agent runs in a container, implements the issue, and creates a PR
4. The VM self-destructs after the session completes
5. You review and merge the PR

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

## Session Lifecycle

Each Agentium session follows this lifecycle:

```
Provision VM → Initialize → Run Agent → Create PR → Terminate VM
```

Sessions automatically terminate when:
- All assigned issues have PRs created
- Maximum iteration count is reached (default: 30)
- Maximum duration is exceeded (default: 2 hours)
- An unrecoverable error occurs

You can also manually terminate a session:

```bash
agentium destroy agentium-abc12345
```

## What's Next?

- [Configuration Reference](configuration.md) - All configuration options
- [CLI Reference](cli-reference.md) - Complete command documentation
- [Cloud Setup Guides](cloud-setup/) - Provider-specific setup
- [GitHub App Setup](github-app-setup.md) - Detailed GitHub App configuration
- [Troubleshooting](troubleshooting.md) - Common issues and solutions
