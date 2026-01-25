# Agentium Security Review Document

Date: January 2026
Reviewer: Security Audit Bot
Version: 1.0

## Executive Summary

This security review identifies the current security posture of the Agentium codebase and provides recommendations for hardening. The review covers secret handling, IAM permissions, logging practices, VM isolation, and overall security architecture.

### Key Findings

1. **Secret Handling**: Generally well-implemented with GCP Secret Manager integration, but some improvements recommended
2. **IAM Permissions**: Some overly broad permissions that should be scoped down
3. **Logging**: Risk of credential leaks in certain log statements
4. **VM Isolation**: Good network isolation but container security could be enhanced
5. **Authentication**: Strong GitHub App authentication implementation

## Detailed Findings and Recommendations

### 1. Secret Handling

#### Current Implementation

**Strengths:**
- Secrets are stored in GCP Secret Manager (`internal/cloud/gcp/secrets.go`)
- Proper abstraction with `SecretFetcher` interface for testing
- Fallback to gcloud CLI for compatibility
- Sensitive configuration fields marked with `sensitive = true` in Terraform

**Weaknesses:**
- GitHub token is logged with expiration time (line 658 in controller.go)
- Secret paths might be exposed in error messages
- Auth JSON content is base64 encoded but stored in plain text in VM metadata

#### Recommendations

1. **Remove sensitive data from logs:**
   - Replace `c.logger.Printf("Generated installation token (expires at %s)", token.ExpiresAt.Format(time.RFC3339))` with a generic message
   - Sanitize error messages that might contain secret paths

2. **Enhance secret storage:**
   - Consider encrypting auth JSON content before base64 encoding
   - Use GCP Secret Manager for all secrets instead of VM metadata

### 2. IAM Permissions

#### Current Implementation

**Findings:**
- Service account has `roles/compute.instanceAdmin.v1` - This is overly broad
- `roles/secretmanager.secretAccessor` - Appropriately scoped
- `roles/logging.logWriter` - Appropriately scoped
- Service account uses `cloud-platform` scope which is very broad

#### Recommendations

1. **Reduce compute permissions:**
   ```hcl
   # Instead of roles/compute.instanceAdmin.v1, use custom role with only:
   - compute.instances.delete
   - compute.instances.get
   ```

2. **Scope down service account:**
   ```hcl
   scopes = [
     "https://www.googleapis.com/auth/logging.write",
     "https://www.googleapis.com/auth/secretmanager",
     "https://www.googleapis.com/auth/compute"
   ]
   ```

### 3. Logging Practices

#### Current Implementation

**Security Risks Identified:**
1. GitHub token passed in command environment (lines 837, 874, 896, 910, 1049)
2. Clone URL contains embedded token (line 741)
3. Potential for secrets in error messages

#### Recommendations

1. **Sanitize logs:**
   - Implement a log sanitizer that redacts tokens, URLs with embedded credentials
   - Use structured logging with explicit field masking

2. **Secure command execution:**
   ```go
   // Instead of logging full commands, log sanitized versions
   sanitizedCmd := sanitizeCommand(cmd.String())
   c.logger.Printf("Running command: %s", sanitizedCmd)
   ```

### 4. VM Isolation

#### Current Implementation

**Strengths:**
- VMs are ephemeral and self-terminating
- Network firewall restricts egress to specific ports (443, 80, 22)
- Containers run as non-root user (UID 1000)

**Weaknesses:**
- Docker socket mounted in controller container (`-v /var/run/docker.sock`)
- Containers have sudo access (`NOPASSWD: ALL`)
- No resource limits on containers
- No security policies (AppArmor/SELinux)

#### Recommendations

1. **Container hardening:**
   ```dockerfile
   # Remove sudo access
   # RUN echo "agentium ALL=(ALL) NOPASSWD: ALL" >> /etc/sudoers

   # Add security options
   --security-opt=no-new-privileges
   --cap-drop=ALL
   --cap-add=NET_BIND_SERVICE
   ```

2. **Resource limits:**
   ```bash
   docker run --memory="2g" --cpus="2" ...
   ```

3. **Network isolation:**
   - Consider using Docker networks instead of host networking
   - Implement internal-only communication between controller and agent containers

### 5. Authentication & Authorization

#### Current Implementation

**Strengths:**
- Strong GitHub App authentication with JWT generation
- Token expiration handling
- Multiple auth modes support (API, OAuth)

**Weaknesses:**
- No token rotation during long-running sessions
- Auth JSON stored in container filesystem

#### Recommendations

1. **Token rotation:**
   - Implement automatic token refresh before expiration
   - Add token refresh monitoring

2. **Auth storage:**
   - Use in-memory storage for auth credentials
   - Clear credentials from disk after loading

## Security Model Documentation

### Architecture Overview

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
│ │   Agent         │ │──── Limited to GitHub API access
│ │   Container     │ │
│ └─────────────────┘ │
└─────────────────────┘
```

### Trust Boundaries

1. **User → Cloud**: Authenticated via cloud provider credentials
2. **VM → GitHub**: Authenticated via GitHub App installation token
3. **Controller → Agent**: Trust via shared filesystem and environment
4. **VM → GCP Services**: Authenticated via service account

### Threat Model

1. **External Threats:**
   - GitHub API compromise → Mitigated by ephemeral tokens
   - Network attacks → Mitigated by firewall rules
   - VM escape → Mitigated by VM isolation and termination

2. **Internal Threats:**
   - Malicious agent code → Limited by container isolation
   - Secret exposure → Mitigated by Secret Manager
   - Privilege escalation → Should be mitigated by removing sudo

## Implementation Priority

### High Priority (Security Critical)

1. Remove sensitive data from logs
2. Reduce IAM permissions to least privilege
3. Remove sudo access from containers
4. Implement log sanitization

### Medium Priority (Defense in Depth)

1. Add container resource limits
2. Implement token rotation
3. Use specific OAuth scopes
4. Add security policies

### Low Priority (Nice to Have)

1. Implement network segmentation
2. Add audit logging
3. Implement secret rotation

## Testing Recommendations

1. **Security Testing:**
   - Penetration testing of VM isolation
   - Secret scanning in logs
   - IAM permission validation

2. **Compliance Checks:**
   - CIS Docker Benchmark
   - GCP Security Best Practices
   - OWASP Container Security

## Conclusion

The Agentium project has a solid security foundation with good use of cloud-native security features. The main areas for improvement are:

1. Preventing credential leaks in logs
2. Implementing true least-privilege IAM
3. Hardening container security
4. Enhancing secret management

Implementing the high-priority recommendations will significantly improve the security posture while maintaining functionality.