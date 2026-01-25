# Security Model

This document describes the security architecture, credential handling, and isolation mechanisms used by Agentium.

## Overview

Agentium runs autonomous AI agents on ephemeral cloud VMs. Security is designed around several principles:

1. **Least privilege**: Each component has minimal permissions needed for its function
2. **Ephemeral infrastructure**: VMs are short-lived and automatically cleaned up
3. **Defense in depth**: Multiple layers protect against credential leaks
4. **Isolation**: Agents run in containers with restricted access

## Secret Handling

### GitHub App Authentication

Agentium uses GitHub App authentication for repository access:

1. **Private key storage**: The GitHub App private key is stored in GCP Secret Manager
2. **JWT generation**: The controller generates a short-lived JWT (10 minutes) using the private key
3. **Token exchange**: The JWT is exchanged for an installation access token (1 hour expiry)
4. **Automatic refresh**: Tokens are regenerated per-session, never cached to disk

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────┐
│  Secret Manager │────>│ Controller (JWT) │────>│   GitHub    │
│  (private key)  │     │   (10min TTL)    │     │ (1hr token) │
└─────────────────┘     └──────────────────┘     └─────────────┘
```

### Claude/Codex Authentication

For OAuth-authenticated agents:

1. **Local credentials**: OAuth tokens are stored in macOS Keychain locally
2. **VM credentials**: Tokens are passed via cloud-init with 0600 permissions
3. **Container access**: Auth files are mounted read-only into agent containers
4. **Cleanup**: Credentials are cleared from memory during shutdown

### Credential Protection

- **File permissions**: All credential files use 0600 (owner read/write only)
- **Memory clearing**: Sensitive data is zeroed during graceful shutdown
- **No disk persistence**: Tokens are never written to logs or persistent storage
- **URL sanitization**: Tokens are never embedded in URLs to prevent log leakage

## IAM Permissions

### Service Account Roles

Each session VM uses a dedicated service account with these roles:

| Role | Purpose | Scope |
|------|---------|-------|
| `secretmanager.secretAccessor` | Read GitHub private key | Project-wide (key is shared) |
| `logging.logWriter` | Write session logs | Project-wide |
| `compute.instanceAdmin.v1` | Self-terminate VM | **Restricted to own instance** |

### IAM Conditions

The `compute.instanceAdmin.v1` role uses an IAM condition to restrict it to the session's own VM:

```hcl
condition {
  title       = "self-deletion-only"
  description = "Restrict instance admin to this session's VM only"
  expression  = "resource.name == 'projects/${project}/zones/${zone}/instances/${session_id}'"
}
```

This prevents a compromised VM from affecting other instances in the project.

### OAuth Scopes

VMs use the `cloud-platform` OAuth scope because GCP Secret Manager has no specific scope. Access is controlled entirely through IAM roles, not OAuth scopes.

## VM Isolation

### Network Security

- **No ingress**: VMs have no inbound firewall rules (SSH access is via IAP if needed)
- **Limited egress**: Outbound traffic is restricted to ports 443 (HTTPS), 80 (HTTP), and 22 (SSH for git)
- **No public services**: VMs do not expose any listening ports

### Container Isolation

Agent processes run in Docker containers with these restrictions:

- **Non-root user**: Containers run as `agentium` (UID 1000)
- **Read-only mounts**: Credential files are mounted read-only (`:ro`)
- **No privileged mode**: Containers run without elevated privileges
- **Resource limits**: Containers inherit VM resource constraints

### Ephemeral Infrastructure

- **Short-lived VMs**: Maximum run duration is enforced at the cloud level
- **Automatic cleanup**: VMs self-terminate on completion
- **Spot instances**: Default configuration uses preemptible VMs (auto-deleted on preemption)
- **No persistent state**: All session state is discarded on termination

## Credential Flow

### Session Startup

```
1. CLI authenticates user (local keychain)
2. CLI provisions VM via Terraform
3. Terraform passes auth data via cloud-init (base64, 0600)
4. Controller fetches GitHub private key from Secret Manager
5. Controller generates installation token
6. Controller clones repo using http.extraHeader (not URL-embedded token)
7. Agent container runs with mounted credentials
```

### Session Shutdown

```
1. Controller clears sensitive fields from memory
2. Controller flushes logs with timeout
3. Controller closes cloud clients
4. VM self-terminates via gcloud
5. Terraform state cleanup removes resources
```

## Threat Model

### Assumptions

- GCP project access is restricted to authorized operators
- GitHub App is installed only on authorized repositories
- Local machine running CLI is trusted
- Container images are pulled from trusted registries

### In Scope

| Threat | Mitigation |
|--------|------------|
| Token leak in logs | Tokens never logged; URL embedding avoided; error sanitization |
| VM compromise spreading | IAM conditions restrict compute.instanceAdmin to self |
| Credential file exposure | 0600 permissions; root:root ownership; read-only mounts |
| Long-lived credential abuse | Short TTLs (1hr tokens, 10min JWTs); per-session generation |
| Orphaned resources | Automatic VM cleanup; terraform destroy fallback |

### Out of Scope

| Threat | Notes |
|--------|-------|
| Malicious agent code | Agents have full shell access within their container |
| GCP project compromise | Requires securing GCP IAM separately |
| Supply chain attacks | Image provenance is user's responsibility |
| Physical security | Relies on GCP's physical security |

## Known Limitations

1. **cloud-platform scope**: Required for Secret Manager; broader than ideal
2. **Container escape**: Agents have docker socket access for nested containers
3. **Network egress**: Limited but not fully restricted (HTTPS to any host)
4. **Shared private key**: GitHub App key is shared across all sessions

## Incident Response

### Credential Rotation

If credentials are compromised:

1. **GitHub App**: Regenerate private key in GitHub App settings, update Secret Manager
2. **OAuth tokens**: Revoke tokens in respective provider (Anthropic/OpenAI dashboard)
3. **Service accounts**: Delete and recreate via `agentium destroy` + reprovisioning

### Emergency Shutdown

To terminate all running sessions:

```bash
# List all active sessions
agentium list

# Destroy specific session
agentium destroy <session-id>

# Or via gcloud directly
gcloud compute instances list --filter="labels.agentium=true" --format="value(name,zone)" | \
  while read name zone; do
    gcloud compute instances delete "$name" --zone="$zone" --quiet
  done
```

### Log Analysis

Session logs are available via:

```bash
# View session logs
agentium logs <session-id>

# Or via gcloud
gcloud logging read 'logName=~"agentium-session"' --format=json
```

## Production Recommendations

### GCP Project Setup

1. **Dedicated project**: Use a separate GCP project for Agentium workloads
2. **VPC isolation**: Consider a dedicated VPC with stricter egress rules
3. **Audit logging**: Enable Cloud Audit Logs for IAM and compute operations
4. **Alerting**: Set up alerts for unusual instance creation patterns

### GitHub App Configuration

1. **Repository permissions**: Grant access only to specific repositories, not organization-wide
2. **Permission scope**: Request minimum permissions needed (contents: write, pull_requests: write)
3. **Webhook security**: If using webhooks, validate signatures

### Operational Security

1. **Credential storage**: Store GitHub App private key only in Secret Manager
2. **Access control**: Limit who can run `agentium provision` in production
3. **Monitoring**: Review Cloud Logging for session activity
4. **Regular audits**: Periodically review IAM bindings and active sessions

## Changelog

| Date | Change |
|------|--------|
| 2024-01-24 | Initial security documentation |
| 2024-01-24 | Added IAM condition for compute.instanceAdmin.v1 |
| 2024-01-24 | Fixed file permissions (0600 for all credential files) |
| 2024-01-24 | Implemented token leak mitigation (http.extraHeader) |
