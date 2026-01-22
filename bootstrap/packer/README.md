# Agentium Pre-Baked GCP Image

This directory contains a [Packer](https://www.packer.io/) template for building a pre-baked GCP machine image with all Agentium dependencies pre-installed.

## Why?

The default bootstrap process installs all tools via cloud-init on every VM startup, which takes 3-8 minutes. By pre-baking these tools into a machine image, cold start time is reduced to under 1 minute.

## Pre-installed Tools

The image includes:
- **Docker** - Container runtime
- **Node.js v20** - JavaScript runtime (via NodeSource)
- **Claude Code CLI** - `@anthropic-ai/claude-code`
- **GitHub CLI** - `gh`
- **gcloud CLI** - Google Cloud SDK
- **Utilities** - git, curl, jq, unzip, openssl

## Prerequisites

1. [Packer](https://www.packer.io/downloads) >= 1.9.0
2. GCP credentials configured (`gcloud auth application-default login`)
3. A GCP project with Compute Engine API enabled

## Building the Image

```bash
cd bootstrap/packer

# Initialize Packer plugins
packer init .

# Validate the template
packer validate -var "project_id=YOUR_PROJECT_ID" .

# Build the image
packer build -var "project_id=YOUR_PROJECT_ID" .
```

Or using a variables file:

```bash
cp agentium.pkrvars.hcl.example agentium.pkrvars.hcl
# Edit agentium.pkrvars.hcl with your values
packer build -var-file="agentium.pkrvars.hcl" .
```

## Using the Image

Once built, the image will be available in your GCP project under the `agentium` image family. To use it with the bootstrap Terraform:

```bash
cd bootstrap

# Use the image family (latest image)
terraform apply -var "vm_image=projects/YOUR_PROJECT_ID/global/images/family/agentium"

# Or use a specific image
terraform apply -var "vm_image=projects/YOUR_PROJECT_ID/global/images/agentium-v1-1234567890"
```

When `vm_image` is set, the bootstrap uses a streamlined cloud-init (`cloud-init-prebaked.yaml`) that skips tool installation and only handles runtime configuration (metadata fetching and session startup).

## Versioning

Images are named with the pattern: `agentium-{version}-{timestamp}`

Use the `image_version` variable to tag builds:

```bash
packer build -var "project_id=YOUR_PROJECT_ID" -var "image_version=v2" .
```

## Rebuilding

The image should be rebuilt periodically to pick up:
- Claude Code CLI updates
- Node.js security patches
- Docker updates
- OS security updates

## Fallback

If the pre-baked image is unavailable or broken, simply omit the `vm_image` variable. The bootstrap will fall back to stock Ubuntu 22.04 with full cloud-init provisioning (the original 3-8 minute startup).
