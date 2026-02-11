# Google Cloud Platform (GCP) Setup

This guide covers setting up Agentium with Google Cloud Platform as the cloud provider.

## Status

**GCP is fully supported** and is the primary cloud provider for Agentium.

## Prerequisites

- A GCP project with billing enabled
- `gcloud` CLI installed and authenticated
- Terraform 1.0+ installed
- IAM permissions on the GCP project (see [Required IAM Roles](#required-iam-roles) below)

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
  iam.googleapis.com \
  logging.googleapis.com
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
bun add -g @openai/codex

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

## Required IAM Roles

The identity running `agentium run` (your user account or a service account) needs the following IAM roles on the GCP project. Terraform creates and manages per-session resources on your behalf, so these permissions are required for the caller:

| IAM Role | Purpose |
|----------|---------|
| `roles/iam.serviceAccountAdmin` | Create/delete per-session service accounts |
| `roles/iam.serviceAccountUser` | Attach service accounts to compute instances |
| `roles/compute.instanceAdmin.v1` | Create/delete compute instances |
| `roles/compute.securityAdmin` | Create/delete firewall rules |
| `roles/resourcemanager.projectIamAdmin` | Grant IAM roles to per-session service accounts |
| `roles/secretmanager.admin` | Create/manage secrets (for initial setup) |
| `roles/logging.viewer` | Read session logs (`agentium logs`) |

**Grant roles to a user account:**

```bash
PROJECT_ID="your-gcp-project-id"
USER="user:your-email@example.com"

for ROLE in \
  roles/iam.serviceAccountAdmin \
  roles/iam.serviceAccountUser \
  roles/compute.instanceAdmin.v1 \
  roles/compute.securityAdmin \
  roles/resourcemanager.projectIamAdmin \
  roles/logging.viewer; do
  gcloud projects add-iam-policy-binding "$PROJECT_ID" \
    --member="$USER" --role="$ROLE"
done
```

## Service Account Authentication

For production use or when your user account doesn't have direct permissions, create a dedicated service account with the required roles and authenticate with its key file:

### 1. Create the service account

```bash
PROJECT_ID="your-gcp-project-id"

gcloud iam service-accounts create agentium-provisioner \
  --display-name="Agentium Provisioner" \
  --project="$PROJECT_ID"
```

### 2. Grant the required roles

```bash
SA="serviceAccount:agentium-provisioner@$PROJECT_ID.iam.gserviceaccount.com"

for ROLE in \
  roles/iam.serviceAccountAdmin \
  roles/iam.serviceAccountUser \
  roles/compute.instanceAdmin.v1 \
  roles/compute.securityAdmin \
  roles/resourcemanager.projectIamAdmin \
  roles/logging.viewer; do
  gcloud projects add-iam-policy-binding "$PROJECT_ID" \
    --member="$SA" --role="$ROLE"
done
```

### 3. Create and download a key file

```bash
gcloud iam service-accounts keys create ~/.config/agentium/sa-key.json \
  --iam-account="agentium-provisioner@$PROJECT_ID.iam.gserviceaccount.com"

# Restrict file permissions
chmod 600 ~/.config/agentium/sa-key.json
```

### 4. Configure Agentium to use the key

```yaml
# .agentium.yaml
cloud:
  provider: "gcp"
  project: "your-gcp-project-id"
  region: "us-central1"
  service_account_key: "~/.config/agentium/sa-key.json"
```

When `service_account_key` is set, all Terraform and gcloud commands authenticate using that key instead of ambient gcloud credentials.

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

### "Permission denied" when creating VMs or service accounts

Agentium needs several IAM roles to provision infrastructure. See [Required IAM Roles](#required-iam-roles) for the full list. Common permission errors:

- `iam.serviceAccounts.create` denied → Grant `roles/iam.serviceAccountAdmin`
- `compute.firewalls.create` denied → Grant `roles/compute.securityAdmin`
- `compute.instances.create` denied → Grant `roles/compute.instanceAdmin.v1`
- `resourcemanager.projects.setIamPolicy` denied → Grant `roles/resourcemanager.projectIamAdmin`
- `PERMISSION_DENIED` on `agentium logs` → Grant `roles/logging.viewer`

If your user account doesn't have these permissions (e.g., after transferring project ownership), use a service account key instead. See [Service Account Authentication](#service-account-authentication).

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

## Langfuse Integration

To enable Langfuse observability tracing on session VMs, set the Langfuse API keys as environment variables. The controller reads these at startup and sends traces automatically.

### Option A: Environment Variables via Startup Script

Add the keys to the VM metadata startup script in your Terraform configuration (`terraform/modules/vm/main.tf`):

```bash
export LANGFUSE_PUBLIC_KEY="pk-lf-your-public-key"
export LANGFUSE_SECRET_KEY="sk-lf-your-secret-key"
```

### Option B: GCP Secret Manager (Recommended for Production)

Store the keys in Secret Manager alongside your other secrets:

```bash
# Create secrets
echo -n "pk-lf-your-public-key" | \
  gcloud secrets versions add langfuse-public-key --data-file=-
echo -n "sk-lf-your-secret-key" | \
  gcloud secrets versions add langfuse-secret-key --data-file=-
```

Then fetch them in the VM startup script:

```bash
export LANGFUSE_PUBLIC_KEY=$(gcloud secrets versions access latest \
  --secret="langfuse-public-key" --project="$PROJECT_ID")
export LANGFUSE_SECRET_KEY=$(gcloud secrets versions access latest \
  --secret="langfuse-secret-key" --project="$PROJECT_ID")
```

### Verifying Traces

After running a session, open your [Langfuse Cloud dashboard](https://cloud.langfuse.com) and navigate to **Traces**. Each task appears as a trace with phase spans and W/R/J generations nested underneath.

For the full setup guide, see [Langfuse Setup](../langfuse-setup.md).

## Next Steps

- [Configuration Reference](../configuration.md) - Full config options
- [CLI Reference](../cli-reference.md) - Command documentation
- [Langfuse Setup](../langfuse-setup.md) - Langfuse observability tracing
- [Troubleshooting](../troubleshooting.md) - General troubleshooting
