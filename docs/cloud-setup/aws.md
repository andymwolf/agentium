# Amazon Web Services (AWS) Setup

This guide covers setting up Agentium with AWS as the cloud provider.

## Status

> **Note:** AWS support is currently **planned but not yet implemented**. This guide documents the intended setup process for when AWS support becomes available.

## Planned Architecture

When implemented, AWS support will use:

- **Compute:** EC2 instances for agent VMs
- **Secrets:** AWS Systems Manager Parameter Store or Secrets Manager
- **Networking:** Default VPC with security groups
- **Instance Types:** T3 family (burstable) by default

## Prerequisites (Planned)

- An AWS account with appropriate permissions
- AWS CLI installed and configured (`aws configure`)
- Terraform 1.0+ installed
- IAM permissions for:
  - EC2 instance management
  - SSM Parameter Store or Secrets Manager access
  - IAM role creation (for instance profiles)

## Planned Configuration

```yaml
# .agentium.yaml
cloud:
  provider: "aws"
  region: "us-east-1"
  machine_type: "t3.medium"
  use_spot: true
  disk_size_gb: 50

github:
  app_id: 123456
  installation_id: 789012
  private_key_secret: "arn:aws:secretsmanager:us-east-1:123456789:secret:github-app-key"
```

## Planned Machine Types

| Instance Type | vCPUs | Memory | Use Case |
|--------------|-------|--------|----------|
| `t3.micro` | 2 | 1 GB | Very simple tasks |
| `t3.small` | 2 | 2 GB | Simple fixes |
| `t3.medium` | 2 | 4 GB | **Default.** Most tasks |
| `t3.large` | 2 | 8 GB | Large repos |
| `t3.xlarge` | 4 | 16 GB | Complex multi-issue sessions |

## Planned Regions

| Region | Location |
|--------|----------|
| `us-east-1` | N. Virginia |
| `us-east-2` | Ohio |
| `us-west-2` | Oregon |
| `eu-west-1` | Ireland |
| `eu-central-1` | Frankfurt |
| `ap-southeast-1` | Singapore |
| `ap-northeast-1` | Tokyo |

## Planned Setup Steps

### 1. Configure AWS CLI

```bash
aws configure
# Enter your Access Key ID, Secret Access Key, and default region
```

### 2. Store GitHub App Private Key

```bash
aws secretsmanager create-secret \
  --name github-app-key \
  --secret-binary fileb:///path/to/private-key.pem
```

### 3. Store Anthropic API Key (Optional)

```bash
aws secretsmanager create-secret \
  --name anthropic-api-key \
  --secret-string "sk-ant-your-api-key"
```

### 4. Create IAM Role for Instances

```bash
# Create instance profile for agent VMs
aws iam create-role \
  --role-name agentium-agent \
  --assume-role-policy-document file://trust-policy.json

# Attach Secrets Manager read policy
aws iam attach-role-policy \
  --role-name agentium-agent \
  --policy-arn arn:aws:iam::aws:policy/SecretsManagerReadWrite
```

## Contributing

If you'd like to contribute to AWS support, see the provisioner interface in `internal/provisioner/` and the GCP implementation as a reference. The key files are:

- `internal/provisioner/provisioner.go` - Interface definition
- `internal/provisioner/gcp.go` - Reference implementation
- `internal/cloud/` - Cloud provider client abstractions

## Current Workaround

Until AWS support is implemented, you can use the GCP provider or run Agentium locally using the bootstrap system:

```bash
cd bootstrap
./run.sh --repo org/repo --issue 42 \
  --app-id 123456 --installation-id 789012 \
  --private-key-secret github-app-key
```

## Next Steps

- [GCP Setup](gcp.md) - Currently supported provider
- [Configuration Reference](../configuration.md) - Full config options
- [Getting Started](../getting-started.md) - Quick start guide
