# Network Egress Restrictions Guide

This document describes how to implement restricted network egress for Agentium VMs to enhance security.

## Overview

By default, Agentium VMs allow outbound connections on ports 443 (HTTPS), 80 (HTTP), and 22 (SSH). To improve security, these should be restricted to only required endpoints.

## Required Endpoints by Agent

### Claude Code Agent
```
# GitHub API
api.github.com:443
github.com:443

# Container Registry
ghcr.io:443
pkg-containers.githubusercontent.com:443

# Anthropic API
api.anthropic.com:443

# OAuth (if using OAuth mode)
claude.ai:443
accounts.anthropic.com:443

# Package registries (if needed)
registry.npmjs.org:443
pypi.org:443
files.pythonhosted.org:443
```

### Aider Agent
```
# GitHub API
api.github.com:443
github.com:443

# Container Registry
ghcr.io:443
pkg-containers.githubusercontent.com:443

# OpenAI API (or configured LLM endpoint)
api.openai.com:443

# Package registries (if needed)
registry.npmjs.org:443
pypi.org:443
files.pythonhosted.org:443
```

### Codex Agent
```
# GitHub API
api.github.com:443
github.com:443

# Container Registry
ghcr.io:443
pkg-containers.githubusercontent.com:443

# Codex API endpoint
<your-codex-endpoint>:443
```

## GCP Implementation

### Using Cloud Armor

1. Create a Cloud Armor security policy:

```bash
gcloud compute security-policies create agentium-egress-policy \
    --description "Restrict egress to required endpoints only"
```

2. Add rules for allowed destinations:

```bash
# Allow GitHub
gcloud compute security-policies rules create 100 \
    --security-policy agentium-egress-policy \
    --expression "destination.ip in ['140.82.112.0/20', '192.30.252.0/22']" \
    --action allow

# Allow Anthropic
gcloud compute security-policies rules create 200 \
    --security-policy agentium-egress-policy \
    --expression "destination.host == 'api.anthropic.com'" \
    --action allow

# Deny all other traffic
gcloud compute security-policies rules create 1000 \
    --security-policy agentium-egress-policy \
    --expression "true" \
    --action deny-403
```

### Using VPC Firewall Rules with Tags

Update the Terraform module to use more restrictive rules:

```hcl
# terraform/modules/vm/gcp/main.tf

# Remove the broad egress rule and add specific ones
resource "google_compute_firewall" "agentium_github_egress" {
  name    = "agentium-github-egress-${substr(var.session_id, 0, 20)}"
  network = var.network
  project = var.project_id

  direction = "EGRESS"

  allow {
    protocol = "tcp"
    ports    = ["443"]
  }

  destination_ranges = [
    "140.82.112.0/20",    # GitHub
    "192.30.252.0/22",    # GitHub
  ]

  target_tags = ["agentium"]
}

resource "google_compute_firewall" "agentium_anthropic_egress" {
  count = var.session_config.agent == "claude-code" ? 1 : 0

  name    = "agentium-anthropic-egress-${substr(var.session_id, 0, 20)}"
  network = var.network
  project = var.project_id

  direction = "EGRESS"

  allow {
    protocol = "tcp"
    ports    = ["443"]
  }

  # Note: This requires DNS resolution; consider using IP ranges
  destination_ranges = ["104.18.0.0/16"]  # Anthropic uses Cloudflare

  target_tags = ["agentium"]
}

# Add rules for container registry
resource "google_compute_firewall" "agentium_ghcr_egress" {
  name    = "agentium-ghcr-egress-${substr(var.session_id, 0, 20)}"
  network = var.network
  project = var.project_id

  direction = "EGRESS"

  allow {
    protocol = "tcp"
    ports    = ["443"]
  }

  destination_ranges = [
    "140.82.112.0/20",  # GitHub Container Registry
  ]

  target_tags = ["agentium"]
}
```

### Using Private Google Access

For GCP services, enable Private Google Access to avoid public internet:

```hcl
resource "google_compute_subnetwork" "agentium_subnet" {
  name          = "agentium-subnet"
  ip_cidr_range = "10.0.1.0/24"
  network       = google_compute_network.agentium_vpc.id

  private_ip_google_access = true
}
```

## AWS Implementation

### Using Security Groups

```hcl
resource "aws_security_group" "agentium" {
  name_prefix = "agentium-"
  vpc_id      = var.vpc_id

  # No ingress rules

  # Specific egress rules
  egress {
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = [
      "140.82.112.0/20",    # GitHub
      "192.30.252.0/22",    # GitHub
    ]
  }

  # Add more specific rules as needed
}
```

### Using VPC Endpoints

Create VPC endpoints for AWS services to avoid internet routing:

```hcl
resource "aws_vpc_endpoint" "s3" {
  vpc_id       = var.vpc_id
  service_name = "com.amazonaws.${var.region}.s3"
}

resource "aws_vpc_endpoint" "ecr" {
  vpc_id              = var.vpc_id
  service_name        = "com.amazonaws.${var.region}.ecr.dkr"
  vpc_endpoint_type   = "Interface"
  subnet_ids          = var.subnet_ids
}
```

## Azure Implementation

### Using Network Security Groups

```hcl
resource "azurerm_network_security_group" "agentium" {
  name                = "agentium-nsg"
  location            = var.location
  resource_group_name = var.resource_group_name

  # Specific outbound rules
  security_rule {
    name                       = "AllowGitHub"
    priority                   = 100
    direction                  = "Outbound"
    access                     = "Allow"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_range     = "443"
    source_address_prefix      = "*"
    destination_address_prefixes = [
      "140.82.112.0/20",
      "192.30.252.0/22"
    ]
  }

  # Deny all other outbound
  security_rule {
    name                       = "DenyAllOutbound"
    priority                   = 1000
    direction                  = "Outbound"
    access                     = "Deny"
    protocol                   = "*"
    source_port_range          = "*"
    destination_port_range     = "*"
    source_address_prefix      = "*"
    destination_address_prefix = "*"
  }
}
```

## DNS Considerations

When restricting by IP ranges:

1. **Dynamic IPs**: Some services use CDNs with changing IPs
2. **Resolution**: Consider using DNS filtering instead of IP-based rules
3. **Monitoring**: Log DNS queries to identify required endpoints

## Monitoring and Compliance

1. **Enable VPC Flow Logs** to monitor actual network traffic
2. **Set up alerts** for denied connections
3. **Regular audits** of allowed endpoints
4. **Document exceptions** when broader access is temporarily needed

## Testing Restrictions

Before applying in production:

```bash
# Test from VM
curl -I https://api.github.com  # Should work
curl -I https://example.com     # Should fail

# Check logs
gcloud logging read "resource.type=gce_subnetwork AND jsonPayload.connection.dest_port=443" \
    --limit 50 --format json
```

## Rollback Plan

Keep the original broad rules disabled but not deleted:

```hcl
resource "google_compute_firewall" "agentium_egress_fallback" {
  name     = "agentium-egress-fallback-${substr(var.session_id, 0, 20)}"
  network  = var.network
  project  = var.project_id
  disabled = true  # Enable if restrictions cause issues

  direction = "EGRESS"

  allow {
    protocol = "tcp"
    ports    = ["443", "80", "22"]
  }

  target_tags = ["agentium"]
}