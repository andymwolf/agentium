# Microsoft Azure Setup

This guide covers setting up Agentium with Microsoft Azure as the cloud provider.

## Status

> **Note:** Azure support is currently **planned but not yet implemented**. This guide documents the intended setup process for when Azure support becomes available.

## Planned Architecture

When implemented, Azure support will use:

- **Compute:** Azure Virtual Machines for agent sessions
- **Secrets:** Azure Key Vault for credential storage
- **Networking:** Virtual Network with Network Security Groups
- **VM Sizes:** B-series (burstable) by default

## Prerequisites (Planned)

- An Azure subscription
- Azure CLI installed and authenticated (`az login`)
- Terraform 1.0+ installed
- Permissions for:
  - Virtual Machine management
  - Key Vault access
  - Resource group management

## Planned Configuration

```yaml
# .agentium.yaml
cloud:
  provider: "azure"
  region: "eastus"
  machine_type: "Standard_B2s"
  use_spot: true
  disk_size_gb: 50

github:
  app_id: 123456
  installation_id: 789012
  private_key_secret: "https://my-vault.vault.azure.net/secrets/github-app-key"
```

## Planned VM Sizes

| VM Size | vCPUs | Memory | Use Case |
|---------|-------|--------|----------|
| `Standard_B1s` | 1 | 1 GB | Very simple tasks |
| `Standard_B1ms` | 1 | 2 GB | Simple fixes |
| `Standard_B2s` | 2 | 4 GB | **Default.** Most tasks |
| `Standard_B2ms` | 2 | 8 GB | Large repos |
| `Standard_B4ms` | 4 | 16 GB | Complex multi-issue sessions |

## Planned Regions

| Region | Location |
|--------|----------|
| `eastus` | Virginia, USA |
| `eastus2` | Virginia, USA |
| `westus2` | Washington, USA |
| `westeurope` | Netherlands |
| `northeurope` | Ireland |
| `southeastasia` | Singapore |
| `japaneast` | Tokyo, Japan |

## Planned Setup Steps

### 1. Authenticate with Azure

```bash
az login
az account set --subscription YOUR_SUBSCRIPTION_ID
```

### 2. Create Resource Group

```bash
az group create \
  --name agentium-rg \
  --location eastus
```

### 3. Create Key Vault

```bash
az keyvault create \
  --name agentium-vault \
  --resource-group agentium-rg \
  --location eastus
```

### 4. Store GitHub App Private Key

```bash
az keyvault secret set \
  --vault-name agentium-vault \
  --name github-app-key \
  --file /path/to/private-key.pem
```

### 5. Store Anthropic API Key (Optional)

```bash
az keyvault secret set \
  --vault-name agentium-vault \
  --name anthropic-api-key \
  --value "sk-ant-your-api-key"
```

## Contributing

If you'd like to contribute to Azure support, see the provisioner interface in `internal/provisioner/` and the GCP implementation as a reference. The key files are:

- `internal/provisioner/provisioner.go` - Interface definition
- `internal/provisioner/gcp.go` - Reference implementation
- `internal/cloud/` - Cloud provider client abstractions

## Current Workaround

Until Azure support is implemented, you can use the GCP provider or run Agentium locally using the bootstrap system:

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
