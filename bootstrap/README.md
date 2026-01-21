# Agentium Bootstrap

Bootstrap scripts for running Agentium agent sessions on GCP virtual machines.

## Overview

The bootstrap system launches ephemeral GCP VMs that run Claude Code to implement GitHub issues. Each session:

1. Creates a preemptible VM with required tools installed
2. Authenticates with GitHub using a GitHub App
3. Clones the target repository
4. Runs Claude Code with safety guardrails
5. Creates pull requests for human review
6. Self-terminates after completion

```
┌─────────────────┐      ┌──────────────────┐      ┌─────────────────┐
│   Local CLI     │─────▶│   GCP VM         │─────▶│   GitHub        │
│   (run.sh)      │      │   (Claude Code)  │      │   (PRs)         │
└─────────────────┘      └──────────────────┘      └─────────────────┘
        │                        │                         │
        │                        │                         │
   Terraform              Secret Manager              Human Review
   (VM setup)             (credentials)               (approve/reject)
```

## Prerequisites

### Required Tools

- **gcloud CLI**: Authenticated with a GCP project
- **Terraform**: v1.0 or later
- **jq**: For parsing JSON outputs
- **GitHub CLI**: For creating the GitHub App (optional)

### GCP Setup

1. Create or select a GCP project
2. Enable required APIs:
   ```bash
   gcloud services enable compute.googleapis.com
   gcloud services enable secretmanager.googleapis.com
   ```

### GitHub App Setup

1. Go to your GitHub account/organization Settings > Developer settings > GitHub Apps
2. Click "New GitHub App"
3. Configure the app:
   - **Name**: `agentium-bot` (or your preferred name)
   - **Homepage URL**: Your repository URL
   - **Webhook**: Uncheck "Active" (not needed for bootstrap)
   - **Permissions**:
     - Repository permissions:
       - Contents: Read and write
       - Issues: Read
       - Pull requests: Read and write
       - Metadata: Read
   - **Where can this GitHub App be installed?**: Only on this account
4. Click "Create GitHub App"
5. Note the **App ID** shown on the app page
6. Generate a private key:
   - Scroll to "Private keys"
   - Click "Generate a private key"
   - Save the downloaded `.pem` file

### GCP Secret Manager Setup

Store your GitHub App private key in Secret Manager:

```bash
# Create the secret
gcloud secrets create github-app-key --replication-policy="automatic"

# Add the private key
gcloud secrets versions add github-app-key --data-file=/path/to/your-key.pem
```

Store your Anthropic API key (optional, can also use environment variable):

```bash
gcloud secrets create anthropic-api-key --replication-policy="automatic"
echo -n "sk-ant-..." | gcloud secrets versions add anthropic-api-key --data-file=-
```

### Install GitHub App

1. Go to your GitHub App settings
2. Click "Install App" in the sidebar
3. Select your account/organization
4. Choose repositories to grant access
5. Note the **Installation ID** from the URL after installation (e.g., `/installations/12345678`)

## Running a Session

### Basic Usage

```bash
cd bootstrap

./run.sh \
  --repo andymwolf/agentium \
  --issue 42 \
  --app-id 123456 \
  --installation-id 789012 \
  --private-key-secret github-app-key
```

### With Environment Variables

Set credentials once:

```bash
export AGENTIUM_PROJECT_ID=my-gcp-project
export AGENTIUM_GITHUB_APP_ID=123456
export AGENTIUM_GITHUB_INSTALLATION_ID=789012
export AGENTIUM_GITHUB_PRIVATE_KEY_SECRET=github-app-key
export AGENTIUM_ANTHROPIC_API_KEY_SECRET=anthropic-api-key
```

Then run:

```bash
./run.sh --repo andymwolf/agentium --issue 42
```

### Follow Logs

Watch the session in real-time:

```bash
./run.sh --repo andymwolf/agentium --issue 42 --follow
```

### Multiple Issues

Work on multiple issues in one session:

```bash
./run.sh --repo andymwolf/agentium --issue 42,43,44
```

### Destroy Session

Clean up resources:

```bash
./run.sh --destroy
```

## Agent Instruction Architecture

The agent receives instructions from two sources:

### 1. System Instructions (SYSTEM.md)

Core safety guardrails that apply to all sessions. These are fetched at runtime from the Agentium repository, enabling hot updates without rebuilding VM images.

**Critical constraints enforced:**
- Never commit directly to main/master
- Always create feature branches
- Work only on assigned issues
- No access to production systems

### 2. Project Instructions (.agentium/AGENT.md)

Optional project-specific instructions that you can add to your repository. Create `.agentium/AGENT.md` with:

- Build and test commands
- Code conventions
- Architecture notes
- Additional constraints

See `AGENT.template.md` for a template.

### Instruction Loading

```
1. VM boots with cloud-init
2. session.sh fetches SYSTEM.md from agentium repo
3. session.sh checks for .agentium/AGENT.md in target repo
4. Claude Code runs with layered system prompts:
   --system-prompt /tmp/SYSTEM.md
   --append-system-prompt .agentium/AGENT.md (if present)
```

## Security Model

| Layer | Mechanism | Enforcement |
|-------|-----------|-------------|
| Instructions | SYSTEM.md guardrails | Soft (LLM follows instructions) |
| Credentials | No production secrets in VM | Hard (secrets don't exist) |
| Branch protection | GitHub branch rules | Hard (enforced by GitHub) |
| Output review | Human PR approval | Hard (manual gate) |
| Time limits | 2hr max via Terraform | Hard (VM auto-terminates) |

### Why `--dangerously-skip-permissions` is Safe

The flag name sounds scary, but it's safe in this context because:

1. **Input control**: Prompts come only from GitHub issues
2. **Output control**: All changes go through PRs for human review
3. **Isolation**: Ephemeral VMs with no production access
4. **No credentials**: The VM has no secrets beyond GitHub access

## Customizing Agent Behavior

### Creating .agentium/AGENT.md

Add project-specific instructions by creating `.agentium/AGENT.md` in your repository:

```markdown
# Project Agent Instructions

## Build & Test Commands

```bash
# Build the project
go build ./...

# Run tests
go test ./...

# Run linter
golangci-lint run
```

## Code Conventions

- Use descriptive variable names
- Add comments for complex logic
- Keep functions under 50 lines

## Architecture Notes

This project uses a clean architecture pattern:
- `cmd/` - Entry points
- `internal/` - Private packages
- `pkg/` - Public packages

## Additional Constraints

- Do not modify the CI/CD pipeline
- Always update tests when changing code
- Use conventional commit messages
```

## Monitoring & Logs

### Session Logs

SSH into the VM and tail logs:

```bash
gcloud compute ssh agentium-session-XXXX --zone=us-central1-a \
  --command="tail -f /var/log/agentium-session.log"
```

### Cloud-init Logs

Check VM setup logs:

```bash
gcloud compute ssh agentium-session-XXXX --zone=us-central1-a \
  --command="tail -f /var/log/cloud-init-output.log"
```

### Terraform State

View current session info:

```bash
cd bootstrap
terraform show
```

## Troubleshooting

### VM Fails to Start

1. Check cloud-init logs:
   ```bash
   gcloud compute ssh VM_NAME --zone=ZONE \
     --command="cat /var/log/cloud-init-output.log"
   ```

2. Verify APIs are enabled:
   ```bash
   gcloud services list --enabled | grep -E "(compute|secretmanager)"
   ```

### GitHub Authentication Fails

1. Verify the secret exists:
   ```bash
   gcloud secrets versions access latest --secret=github-app-key | head -1
   ```

2. Check the App ID and Installation ID are correct

3. Verify the app has required permissions on the repository

### Claude Code Fails

1. Check if Anthropic API key is set:
   ```bash
   gcloud secrets versions access latest --secret=anthropic-api-key | head -1
   ```

2. SSH into the VM and check Claude Code installation:
   ```bash
   gcloud compute ssh VM_NAME --zone=ZONE --command="claude --version"
   ```

### Session Times Out

Sessions have a 2-hour maximum duration. For longer tasks:

1. Break the issue into smaller parts
2. Launch multiple sessions
3. Consider increasing `max_session_hours` in Terraform (not recommended)

## File Structure

```
bootstrap/
├── README.md                  # This file
├── AGENT.template.md          # Template for project-specific instructions
├── SYSTEM.md                  # Core safety guardrails (fetched at runtime)
├── cloud-init.yaml            # VM setup script
├── main.tf                    # Terraform configuration
├── variables.tf               # Terraform variables
├── terraform.tfvars.example   # Example variable values
├── run.sh                     # Local launch script
└── session.sh                 # VM session orchestration script
```
