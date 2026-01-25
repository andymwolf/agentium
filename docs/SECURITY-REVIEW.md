# Agentium Security Review

**Review Date**: January 2025
**Reviewer**: Claude Security Audit
**Status**: Completed

## Executive Summary

This document presents a comprehensive security audit of the Agentium codebase, focusing on secret handling, IAM permissions, credential leaks, VM isolation, and overall security posture. The review found that Agentium implements strong security practices with only minor areas for enhancement.

## 1. Secret Handling Audit

### 1.1 Current Implementation

**Strengths:**
- **Centralized Secret Management**: Uses GCP Secret Manager for secure storage and retrieval of sensitive data
- **Proper API Design**: The `SecretFetcher` interface in `internal/cloud/gcp/secrets.go` provides abstraction and testability
- **Timeout Protection**: 10-second timeout on secret fetching prevents hanging operations
- **Path Normalization**: Handles various secret path formats safely

**Areas of Concern:**
- **Environment Variable Exposure**: Multiple environment variables checked for project ID (`GOOGLE_CLOUD_PROJECT`, `GCP_PROJECT`, `GCLOUD_PROJECT`)
- **Metadata Server Access**: Falls back to metadata server which could be spoofed in compromised environments

### 1.2 GitHub App Authentication

**Strengths:**
- **RSA Key Handling**: Private keys properly parsed and validated (supports both PKCS#1 and PKCS#8)
- **JWT Token Expiry**: Enforces 10-minute maximum token duration per GitHub requirements
- **No Key Storage**: Private keys are fetched from Secret Manager, not stored locally

**Recommendations:**
- Consider implementing key rotation reminders or automation
- Add audit logging for JWT generation events

### 1.3 Agent Authentication Files

**Security Concerns:**
- **File Permissions**: Auth files are written with permissions 0644 (readable by all) in cloud-init
- **Base64 Encoding**: Auth data transmitted via base64 in Terraform variables (not encryption)

**Recommendation**: Change file permissions to 0600 for auth files in cloud-init configuration.

## 2. IAM Permissions Review

### 2.1 Service Account Permissions

The VM service account is granted the following roles:
```
- roles/secretmanager.secretAccessor  # Read secrets
- roles/logging.logWriter            # Write logs
- roles/compute.instanceAdmin.v1     # Self-deletion capability
```

**Assessment**: These follow the principle of least privilege appropriately:
- Secret access is read-only
- Logging is write-only
- Compute admin is necessary for self-termination

### 2.2 VM Network Access

**Current Configuration:**
- Egress allowed on ports: 443 (HTTPS), 80 (HTTP), 22 (SSH)
- No ingress rules defined (secure by default)
- Ephemeral public IP assigned

**Recommendation**: Consider restricting egress to only required services (GitHub API, container registries)

## 3. Credential Leak Prevention

### 3.1 Logging Security

**Strengths:**
- Structured logging with controlled field output
- Message truncation at 2000 characters prevents large credential dumps
- No automatic dumping of environment variables or configuration

**Concerns:**
- No explicit credential scrubbing in log messages
- Docker command arguments logged which could contain sensitive data

**Recommendation**: Implement a credential scrubber that removes common patterns (tokens, keys) from logs.

### 3.2 Configuration Handling

**Good Practices:**
- Sensitive fields marked with `sensitive: true` in Terraform
- Private key secrets referenced by path, not value
- Auth files mounted read-only in containers

## 4. VM Isolation Analysis

### 4.1 Container Isolation

**Strengths:**
- Agents run in Docker containers, not directly on VM
- Read-only mounts for authentication files
- Working directory mounted with limited scope
- No privileged container flags

**Concerns:**
- Docker socket mounted (`/var/run/docker.sock`) allows container escape
- Required for controller to manage agent containers

### 4.2 Network Isolation

**Implementation:**
- VMs use ephemeral IPs, not static
- Default VPC/subnet used (could be more isolated)
- Per-session firewall rules created and destroyed

**Recommendation**: Consider using a dedicated VPC with private subnets for agent VMs.

### 4.3 Lifecycle Management

**Strengths:**
- Hard timeout enforcement at cloud provider level
- Automatic termination on completion
- Session-scoped resources with unique naming
- Spot/preemptible instances reduce persistence risk

## 5. Security Model Documentation

### 5.1 Trust Boundaries

```
┌─────────────────────────────────────────────────────┐
│                   User Machine                      │
│  ┌─────────────┐                                   │
│  │ agentium CLI│ (GitHub App Auth)                 │
│  └──────┬──────┘                                   │
└─────────┼───────────────────────────────────────────┘
          │ HTTPS
┌─────────┼───────────────────────────────────────────┐
│         ▼              Cloud Provider               │
│  ┌─────────────┐                                   │
│  │ Provisioner │ (Creates VM)                      │
│  └──────┬──────┘                                   │
│         │                                           │
│  ┌──────▼──────────────────────────────┐          │
│  │        Agent VM                      │          │
│  │  ┌─────────────┐  ┌──────────────┐ │          │
│  │  │ Controller  │──│ Agent Docker │ │          │
│  │  └─────────────┘  └──────────────┘ │          │
│  │         │                           │          │
│  │         └─── GitHub API ────────────┤          │
│  └─────────────────────────────────────┘          │
└─────────────────────────────────────────────────────┘
```

### 5.2 Security Principles

1. **Ephemeral Infrastructure**: VMs exist only for session duration
2. **Least Privilege**: Each component has minimal required permissions
3. **Defense in Depth**: Multiple layers (VM, container, IAM)
4. **Audit Trail**: All actions logged with session context
5. **No Persistent State**: VMs self-destruct, no data persists

### 5.3 Threat Model

**Protected Against:**
- Credential theft (secrets never in code)
- Persistent compromise (ephemeral VMs)
- Lateral movement (isolated VMs)
- Data exfiltration (no production access)

**Accepts Risk:**
- Container escape via docker socket
- GitHub token compromise (mitigated by short-lived tokens)
- Supply chain attacks on base images

## 6. Findings Summary

### Critical Issues
None identified.

### High Priority Recommendations

1. **Fix Auth File Permissions**
   - Change permissions from 0644 to 0600 in cloud-init
   - File: `terraform/modules/vm/gcp/main.tf`, lines 141-151

2. **Implement Credential Scrubbing**
   - Add regex-based scrubber for common credential patterns
   - Apply to all log outputs before writing

3. **Restrict Network Egress**
   - Limit outbound connections to required endpoints only
   - Document required endpoints for each agent

### Medium Priority Enhancements

1. **Dedicated VPC**: Use isolated network for agent VMs
2. **Audit Logging**: Add security event logging (auth, access)
3. **Key Rotation**: Implement automated rotation for long-lived keys
4. **Image Scanning**: Regular vulnerability scans of container images

### Low Priority Improvements

1. **SAST Integration**: Add static analysis to CI/CD
2. **Security Headers**: If adding web interfaces, implement security headers
3. **Rate Limiting**: Add rate limits to prevent abuse

## 7. Compliance Considerations

The current implementation aligns with cloud security best practices:
- ✅ Encryption in transit (HTTPS/TLS)
- ✅ Encryption at rest (GCP Secret Manager)
- ✅ Access logging capability
- ✅ Least privilege IAM
- ✅ Ephemeral compute resources

## 8. Conclusion

Agentium demonstrates a security-conscious design with appropriate controls for its threat model. The ephemeral nature of resources and separation of concerns provides strong security guarantees. The identified recommendations would enhance the already robust security posture but are not critical vulnerabilities.

The system appropriately balances security with functionality for an AI agent execution platform.