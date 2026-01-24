# Google Cloud Platform (GCP) Setup

This guide covers setting up Agentium with Google Cloud Platform as the cloud provider.

## Status

**GCP is fully supported** and is the primary cloud provider for Agentium.

## Prerequisites

- A GCP project with billing enabled
- `gcloud` CLI installed and authenticated
- Terraform 1.0+ installed
- IAM permissions to:
  - Create Compute Engine instances
  - Create and manage secrets in Secret Manager
  - Create service accounts (optional, for least-privilege)

## Setup Steps

### 1. Authenticate with GCP

```bash
# Login to your GCP account
gcloud auth login

# Set your project
gcloud config set project YOUR_PROJECT_ID

# Enable Application Default Credentials (for Terraform)
gcloud auth application-default login
```

### 2. Enable Required APIs

```bash
gcloud services enable \
  compute.googleapis.com \
  secretmanager.googleapis.com \
  iam.googleapis.com
```

### 3. Store GitHub App Private Key

```bash
# Create the secret
gcloud secrets create github-app-key \
  --replication-policy="automatic"

# Add the private key
gcloud secrets versions add github-app-key \
  --data-file=/path/to/your-app-private-key.pem
```

### 4. Store Anthropic API Key (Optional)

If using Claude Code with API authentication mode:

```bash
# Create the secret
gcloud secrets create anthropic-api-key \
  --replication-policy="automatic"

# Add the API key
echo -n "sk-ant-your-api-key-here" | \
  gcloud secrets versions add anthropic-api-key --data-file=-
```

### 4b. Set Up Codex Authentication (Optional, for Codex agent)

If using the Codex agent, authenticate on a machine with a browser first:

```bash
# Install Codex
npm install -g @openai/codex

# Login (opens browser for OAuth)
codex --login
```

Agentium will automatically read the cached credentials from `~/.codex/auth.json` (or the macOS Keychain) and transfer them to the VM. Treat `~/.codex/auth.json` like a password — it contains access tokens.

### 5. Configure Agentium

```yaml
# .agentium.yaml
cloud:
  provider: "gcp"
  region: "us-central1"
  project: "your-gcp-project-id"
  machine_type: "e2-medium"
  use_spot: true
  disk_size_gb: 50

github:
  app_id: 123456
  installation_id: 789012
  private_key_secret: "projects/your-gcp-project-id/secrets/github-app-key"
```

## Available Regions

Choose a region close to your team for lower latency:

| Region | Location |
|--------|----------|
| `us-central1` | Iowa, USA |
| `us-east1` | South Carolina, USA |
| `us-west1` | Oregon, USA |
| `europe-west1` | Belgium |
| `europe-west2` | London, UK |
| `asia-east1` | Taiwan |
| `asia-northeast1` | Tokyo, Japan |
| `australia-southeast1` | Sydney, Australia |

See [GCP regions list](https://cloud.google.com/compute/docs/regions-zones) for all options.

## Machine Types

Choose a machine type based on your workload:

| Machine Type | vCPUs | Memory | Use Case |
|-------------|-------|--------|----------|
| `e2-micro` | 0.25 | 1 GB | Very simple tasks |
| `e2-small` | 0.5 | 2 GB | Simple fixes, small repos |
| `e2-medium` | 1 | 4 GB | **Default.** Most tasks |
| `e2-standard-2` | 2 | 8 GB | Large repos, complex tasks |
| `e2-standard-4` | 4 | 16 GB | Very large repos, multiple issues |

## Spot/Preemptible Instances

Enable `use_spot: true` for significant cost savings (60-91% discount). Spot instances may be preempted by GCP, but since Agentium sessions are ephemeral and typically short-lived, this is usually acceptable.

```yaml
cloud:
  use_spot: true
```

**Trade-offs:**
- **Pro:** Significantly cheaper (often 60-91% less)
- **Con:** Can be preempted (terminated) by GCP at any time
- **Mitigation:** Agentium sessions are designed to be ephemeral; if preempted, re-run the session

## Service Account (Optional)

For production use, create a dedicated service account with minimal permissions:

```bash
# Create service account
gcloud iam service-accounts create agentium-agent \
  --display-name="Agentium Agent"

# Grant Compute Engine access
gcloud projects add-iam-policy-binding YOUR_PROJECT_ID \
  --member="serviceAccount:agentium-agent@YOUR_PROJECT_ID.iam.gserviceaccount.com" \
  --role="roles/compute.instanceAdmin.v1"

# Grant Secret Manager access
gcloud projects add-iam-policy-binding YOUR_PROJECT_ID \
  --member="serviceAccount:agentium-agent@YOUR_PROJECT_ID.iam.gserviceaccount.com" \
  --role="roles/secretmanager.secretAccessor"
```

## Networking

By default, Agentium VMs are created with:
- A public IP address (for GitHub API access)
- Default VPC network
- Firewall rules allowing outbound HTTPS traffic

For advanced networking requirements (VPC, private IPs, NAT), you can customize the Terraform modules in `terraform/modules/vm/`.

## Cost Estimation

Approximate costs per session (US regions):

| Machine Type | On-Demand (per hour) | Spot (per hour) |
|-------------|---------------------|-----------------|
| `e2-micro` | ~$0.008 | ~$0.003 |
| `e2-small` | ~$0.017 | ~$0.005 |
| `e2-medium` | ~$0.034 | ~$0.010 |
| `e2-standard-2` | ~$0.067 | ~$0.020 |

*Prices are approximate. Check [GCP Pricing Calculator](https://cloud.google.com/products/calculator) for current rates.*

**Example:** A typical 30-minute session on `e2-medium` with spot pricing costs approximately $0.005.

## Troubleshooting

### "Permission denied" when creating VMs

Ensure your account has the `Compute Instance Admin` role:

```bash
gcloud projects add-iam-policy-binding YOUR_PROJECT_ID \
  --member="user:your-email@example.com" \
  --role="roles/compute.instanceAdmin.v1"
```

### "Secret not found"

Verify the secret exists and you have access:

```bash
gcloud secrets list
gcloud secrets versions access latest --secret="github-app-key"
```

### "API not enabled"

Enable the required APIs:

```bash
gcloud services enable compute.googleapis.com secretmanager.googleapis.com
```

### "Quota exceeded"

Check your project quotas:

```bash
gcloud compute project-info describe --project YOUR_PROJECT_ID
```

Request quota increases in the GCP Console under IAM & Admin > Quotas.

### VM fails to start

Check cloud-init logs on the VM:

```bash
gcloud compute ssh INSTANCE_NAME \
  --zone=ZONE \
  --command="cat /var/log/cloud-init-output.log"
```

## Next Steps

- [Configuration Reference](../configuration.md) - Full config options
- [CLI Reference](../cli-reference.md) - Command documentation
- [Troubleshooting](../troubleshooting.md) - General troubleshooting
