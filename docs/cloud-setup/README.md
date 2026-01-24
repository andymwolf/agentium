# Cloud Provider Setup Guides

Agentium supports multiple cloud providers for provisioning ephemeral agent VMs. Choose the guide for your preferred provider:

| Provider | Status | Guide |
|----------|--------|-------|
| [Google Cloud Platform (GCP)](gcp.md) | **Fully Supported** | Production-ready |
| [Amazon Web Services (AWS)](aws.md) | Planned | Not yet implemented |
| [Microsoft Azure](azure.md) | Planned | Not yet implemented |

## Choosing a Provider

Currently, **GCP is the only fully supported provider**. AWS and Azure support is planned for future releases.

### GCP Advantages
- First-class support with complete Terraform modules
- Preemptible VMs for cost savings
- Google Cloud Secret Manager integration
- Tested and production-ready

## Common Requirements

Regardless of cloud provider, you will need:

1. **A cloud account** with billing enabled
2. **CLI tools** installed and authenticated (`gcloud`, `aws`, or `az`)
3. **Terraform 1.0+** for VM provisioning
4. **A GitHub App** with appropriate permissions (see [GitHub App Setup](../github-app-setup.md))
5. **Secret storage** for the GitHub App private key and optional API keys

## Architecture Overview

```
┌─────────────────────────────────────────────────────┐
│                   Your Machine                       │
│                                                     │
│  agentium run --issues 42                           │
│       │                                             │
│       ▼                                             │
│  ┌─────────────┐                                    │
│  │ Agentium CLI│──── Terraform ────┐               │
│  └─────────────┘                   │               │
└────────────────────────────────────┼───────────────┘
                                     │
                                     ▼
┌─────────────────────────────────────────────────────┐
│               Cloud Provider (GCP/AWS/Azure)         │
│                                                     │
│  ┌───────────────────────────────────────────┐      │
│  │           Ephemeral VM                     │      │
│  │                                           │      │
│  │  ┌─────────────┐    ┌──────────────┐     │      │
│  │  │ Controller  │    │ Agent        │     │      │
│  │  │ Container   │───▶│ Container    │     │      │
│  │  └─────────────┘    │ (Claude Code │     │      │
│  │                      │  or Aider)   │     │      │
│  │                      └──────────────┘     │      │
│  └───────────────────────────────────────────┘      │
│                                                     │
│  ┌──────────────┐                                   │
│  │ Secret       │  (GitHub App key, API keys)       │
│  │ Manager      │                                   │
│  └──────────────┘                                   │
└─────────────────────────────────────────────────────┘
```

Each session creates an isolated VM that:
- Clones the target repository
- Runs the AI agent in a container
- Creates PRs for human review
- Self-destructs on completion
