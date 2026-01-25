# Security Documentation

This document describes the security architecture and best practices for Agentium.

## Overview

Agentium implements defense-in-depth security with multiple layers:

1. **Cloud-level security** - IAM roles, VPC isolation, ephemeral VMs
2. **Container security** - Non-root users, capability dropping, read-only filesystems
3. **Secret management** - GCP Secret Manager integration, no hardcoded secrets
4. **Authentication** - GitHub App authentication with short-lived tokens
5. **Logging security** - Sanitized logs, no credential exposure

## Architecture

```
┌─────────────────────┐
│   User (CLI)        │
└──────────┬──────────┘
           │
           ├─── Provisions VM with limited IAM role
           │
┌──────────▼──────────┐
│   GCP VM Instance   │
│ ┌─────────────────┐ │
│ │   Controller    │ │──── Fetches secrets from Secret Manager
│ │   Container     │ │
│ └────────┬────────┘ │
│          │          │
│ ┌────────▼────────┐ │
│ │   Agent         │ │──── Limited to GitHub API access only
│ │   Container     │ │
│ └─────────────────┘ │
└─────────────────────┘
```

## Secret Management

### Storage

- All secrets stored in GCP Secret Manager
- No secrets in code, configuration files, or container images
- Secrets accessed via service account with `secretmanager.secretAccessor` role

### Access Patterns

```go
// Secrets are fetched through the SecretFetcher interface
secretManager, err := gcp.NewSecretManagerClient(ctx)
secret, err := secretManager.FetchSecret(ctx, "projects/PROJECT/secrets/SECRET/versions/latest")
```

### Best Practices

1. **Never log secrets** - All logging is sanitized to remove credentials
2. **Use short-lived tokens** - GitHub tokens expire after 1 hour
3. **Minimal permissions** - Service accounts have only required permissions
4. **Encrypted transport** - All secrets transmitted over HTTPS/TLS

## IAM Configuration

### Service Account Permissions

The VM service account has these minimal roles:

1. **Custom Role: agentiumVMSelfDelete**
   - `compute.instances.delete` - Delete self
   - `compute.instances.get` - Get instance metadata
   - `compute.zones.get` - Get zone information
   - `compute.zones.list` - List zones

2. **roles/secretmanager.secretAccessor**
   - Access secrets from Secret Manager

3. **roles/logging.logWriter**
   - Write logs to Cloud Logging

### Principle of Least Privilege

- No admin roles
- No project-wide permissions
- Scoped to specific resources where possible

## Container Security

### Runtime Security

All agent containers run with:

```bash
docker run \
  --security-opt=no-new-privileges \  # Prevent privilege escalation
  --cap-drop=ALL \                    # Drop all Linux capabilities
  --read-only \                       # Read-only root filesystem
  --tmpfs /tmp \                      # Writable temp directory
  --memory=4g \                       # Memory limit
  --cpus=2 \                          # CPU limit
  ...
```

### User Security

- Containers run as non-root user (UID 1000)
- No sudo access in containers
- Home directory owned by container user

### Image Security

- Base images regularly updated
- Minimal attack surface (slim images)
- No unnecessary packages installed

## Network Security

### Firewall Rules

Egress-only firewall allowing:
- Port 443 (HTTPS) - GitHub API, package registries
- Port 80 (HTTP) - Package downloads
- Port 22 (SSH) - Git operations

No ingress allowed except established connections.

### VM Isolation

- Each session runs in isolated VM
- No persistent storage between sessions
- VMs self-terminate after completion
- No SSH access to VMs

## Authentication

### GitHub App Authentication

1. Private key stored in Secret Manager
2. JWT generated with 10-minute expiration
3. JWT exchanged for 1-hour installation token
4. Token never logged or persisted

### OAuth Authentication (Claude)

- OAuth tokens stored in memory only
- Tokens mounted read-only in containers
- Tokens cleared after session

## Logging Security

### Log Sanitization

All logs are sanitized to remove:
- GitHub access tokens
- Base64 encoded credentials
- JWT tokens
- URLs with embedded credentials

Example sanitization:

```go
func sanitizeForLogging(s string) string {
    // Remove tokens from URLs
    s = regexp.MustCompile(`https://x-access-token:[^@]+@`).
        ReplaceAllString(s, "https://[REDACTED]@")

    // Remove JWTs
    s = regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`).
        ReplaceAllString(s, "[REDACTED-JWT]")

    return s
}
```

### Structured Logging

- Sensitive fields explicitly excluded
- Cloud Logging integration for audit trail
- No credentials in error messages

## Incident Response

### Security Incident Checklist

1. **Credential Exposure**
   - Rotate affected credentials immediately
   - Review logs for unauthorized access
   - Update Secret Manager versions

2. **VM Compromise**
   - VM will auto-terminate (max 2 hours)
   - Review Cloud Logging for suspicious activity
   - Check for unauthorized GitHub API calls

3. **Container Escape**
   - Limited impact due to VM isolation
   - Review container security settings
   - Update base images

### Monitoring

- Cloud Logging for all API calls
- GitHub audit logs for repository access
- VM metadata for session tracking

## Security Checklist

### Before Deployment

- [ ] All secrets in Secret Manager
- [ ] IAM roles follow least privilege
- [ ] Container images up to date
- [ ] Security fixes applied

### During Operation

- [ ] Monitor Cloud Logging
- [ ] Review GitHub audit logs
- [ ] Check for failed authentication

### After Completion

- [ ] VM terminated
- [ ] No residual data
- [ ] Logs properly archived

## Reporting Security Issues

Please report security vulnerabilities to: security@agentium.dev

Include:
- Description of vulnerability
- Steps to reproduce
- Potential impact
- Suggested fixes