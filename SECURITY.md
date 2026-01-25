# Security Model

This document describes the security model and controls implemented in Agentium.

## Overview

Agentium runs untrusted AI agents in isolated cloud environments to safely execute code changes. Security is enforced through multiple layers of isolation and access control.

## Architecture Security

### 1. Network Isolation

**No Ingress Access**
- VMs have no inbound firewall rules
- Cannot be accessed from the internet
- No SSH access enabled

**Limited Egress**
- Outbound only on ports 443, 80, 22
- Restricted to HTTPS, HTTP, and Git operations

### 2. Compute Isolation

**Ephemeral VMs**
- Session-scoped instances
- Automatic termination after task completion
- Maximum runtime limits enforced
- No persistent storage

**Container Isolation**
- Agents run in Docker containers
- Non-root execution
- Read-only credential mounts
- No privileged operations

### 3. Identity & Access Management

**Service Account Permissions (GCP)**
- `secretmanager.secretAccessor` - Read secrets only
- `logging.logWriter` - Write logs only
- `compute.instanceAdmin.v1` - Self-termination only

**GitHub App Permissions**
- Repository-scoped access
- Installation-based authorization
- Short-lived tokens (1 hour)
- No access to GitHub org settings

### 4. Secret Management

**No Persistent Credentials**
- Secrets fetched at runtime
- Stored in memory only
- Cleared on shutdown
- No secrets in code or images

**Secure Token Flow**
1. GitHub App private key in Secret Manager
2. Generate JWT with 10-minute expiry
3. Exchange for 1-hour installation token
4. Token cleared after use

### 5. Logging Security

**Log Sanitization**
- Automatic redaction of:
  - API keys and tokens
  - Private keys
  - Passwords and credentials
  - Cloud service account data
- Path sanitization
- Metadata filtering

**Audit Trail**
- Session tracking
- Iteration logging
- Structured cloud logs
- No sensitive data in logs

## Security Boundaries

### What Agents CAN Do
- Read/write repository code
- Create branches and PRs
- Run tests and builds
- Access GitHub API for assigned repos

### What Agents CANNOT Do
- Access production systems
- Create persistent credentials
- Modify IAM policies
- Access other cloud resources
- Establish network listeners
- Access VM host directly

## Threat Mitigation

### External Threats
- **Mitigation**: No ingress access, cloud firewall rules

### Malicious Agents
- **Mitigation**: Container isolation, least privilege IAM, automatic termination

### Credential Theft
- **Mitigation**: Memory-only secrets, log sanitization, short-lived tokens

### Resource Abuse
- **Mitigation**: Time limits, resource quotas, session scoping

## Security Best Practices

### For Operators

1. **Review Agent Permissions**
   - Verify GitHub App installation settings
   - Limit repository access appropriately

2. **Monitor Sessions**
   - Check cloud logs for anomalies
   - Review PR changes before merging

3. **Secret Rotation**
   - Rotate GitHub App private key periodically
   - Update cloud credentials as needed

### For Contributors

1. **Never Commit Secrets**
   - Use secret management services
   - Run security checks before commits

2. **Follow Least Privilege**
   - Request minimal permissions
   - Document permission requirements

3. **Security Testing**
   - Run `scripts/security-check.sh`
   - Add security tests for new features

## Incident Response

### Suspected Compromise

1. **Immediate Actions**
   - Terminate affected sessions
   - Revoke GitHub App access
   - Rotate credentials

2. **Investigation**
   - Review cloud logs
   - Check GitHub audit logs
   - Analyze PR/commit history

3. **Remediation**
   - Patch identified vulnerabilities
   - Update security controls
   - Document lessons learned

## Security Validation

Run security checks:
```bash
./scripts/security-check.sh
```

This validates:
- No hardcoded secrets
- IAM least privilege
- Container security
- Dependency safety

## Reporting Security Issues

Please report security vulnerabilities to the maintainers privately. Do not open public issues for security concerns.

## Compliance

Agentium is designed to support:
- Data protection (no persistent storage)
- Access control (IAM and GitHub permissions)
- Audit requirements (comprehensive logging)
- Secure development lifecycle

For specific compliance requirements, please review with your security team.