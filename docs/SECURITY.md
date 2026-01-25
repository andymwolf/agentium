# Agentium Security Model

## Overview

Agentium implements a defense-in-depth security model designed to safely execute AI coding agents in cloud environments. This document describes the security architecture, controls, and best practices.

## Core Security Principles

### 1. Ephemeral Infrastructure
- **Short-lived VMs**: All agent VMs self-terminate after task completion
- **No persistent state**: VMs are stateless; all changes flow through Git
- **Automatic cleanup**: Failed sessions are cleaned up by cloud provider timeouts

### 2. Least Privilege Access
- **Minimal IAM permissions**: VMs only have permissions for:
  - Reading secrets (GitHub App private key)
  - Writing logs
  - Self-termination
  - No network administration
  - No IAM modification
  - No access to other cloud resources

### 3. Network Isolation
- **Egress-only**: VMs can only make outbound connections
- **No ingress**: No inbound ports are exposed
- **VPC isolation**: Each VM runs in an isolated network environment
- **No peer-to-peer**: VMs cannot communicate with each other

### 4. Container Isolation
- **Agent containerization**: All agent code runs in Docker containers
- **Non-root execution**: Containers run as non-privileged users
- **Resource limits**: CPU and memory constraints prevent runaway processes
- **Read-only mounts**: Configuration files mounted as read-only

## Authentication & Secrets

### GitHub App Authentication
- **JWT-based**: Short-lived JWTs (10 minutes max)
- **Private key storage**: Stored in cloud provider secret managers
- **No persistent tokens**: Installation tokens expire after 1 hour
- **Scoped permissions**: Only repository read/write access

### Secret Management
```
┌─────────────────┐
│ Secret Manager  │
│ (Cloud Provider)│
└────────┬────────┘
         │ Read-only access
         ▼
┌─────────────────┐
│    Agent VM     │
│  ┌───────────┐  │
│  │Controller │  │
│  └─────┬─────┘  │
│        │        │
│  ┌─────▼─────┐  │
│  │   Agent   │  │
│  │ Container │  │
│  └───────────┘  │
└─────────────────┘
```

### Credential Handling
- **No hardcoded secrets**: All secrets from environment/secret manager
- **Automatic expiration**: Credentials expire with VM termination
- **No credential persistence**: No credentials written to disk

## Log Security

### Log Sanitization
The `SecureCloudLogger` automatically sanitizes logs to prevent credential leakage:

- GitHub tokens (ghp_, ghs_, github_pat_)
- API keys and secrets
- Bearer tokens
- JWTs
- Private keys
- Passwords in URLs
- Cloud provider credentials

### Sanitization Examples
```
Input:  "Using token ghp_abcd1234..."
Output: "Using token [REDACTED-GITHUB-TOKEN]"

Input:  "Connecting to https://user:pass@example.com"
Output: "Connecting to https://[REDACTED]@example.com"
```

## VM Security

### Boot Security
- **Cloud-init**: Minimal configuration via cloud-init
- **No SSH**: SSH disabled; no remote access
- **Metadata service**: Limited to reading own instance metadata

### Runtime Security
- **Automatic termination**: Multiple termination mechanisms:
  1. Controller-initiated shutdown
  2. Cloud provider max-runtime limit
  3. Spot instance interruption
  4. Manual termination via CLI

### Resource Limits
```yaml
machine_type: e2-medium    # 2 vCPU, 4GB RAM
disk_size_gb: 50          # Limited disk space
max_run_duration: 7200s   # 2-hour hard limit
```

## Code Execution Safety

### Branch Protection
- **No main branch commits**: Agents cannot push to main/master
- **Feature branches only**: All work on `agentium/issue-*` branches
- **PR-based workflow**: All changes through pull requests

### Scope Limitations
- **Issue-scoped**: Agents work only on assigned issues
- **No drive-by fixes**: Cannot modify unrelated code
- **No CI/CD changes**: Cannot modify GitHub Actions
- **No dependency changes**: Without explicit permission

### Prohibited Actions
- Force pushing
- Deleting branches
- Modifying branch protection
- Creating/modifying workflows
- Installing system packages
- Accessing external services (except GitHub)

## Network Security

### Egress Filtering (Planned)
Future enhancement to restrict outbound connections to:
- GitHub API (api.github.com)
- GitHub (github.com)
- Package registries (npm, pypi, etc.)
- Docker Hub

### DNS Security (Planned)
- Use secure DNS (DoH/DoT)
- Block known malicious domains
- Log DNS queries

## Monitoring & Audit

### Audit Trail
Every action is logged:
- Git commits with issue references
- API calls to GitHub
- VM lifecycle events
- Container execution logs

### Security Monitoring (Planned)
- Anomaly detection for unusual patterns
- Alert on security policy violations
- Track failed authentication attempts

## Incident Response

### Security Incident Procedure
1. **Immediate**: Terminate affected VM(s)
2. **Investigate**: Review logs and audit trail
3. **Remediate**: Patch vulnerabilities
4. **Document**: Update security procedures

### Emergency Controls
- **Kill switch**: `agentium destroy --all`
- **Revoke access**: Disable GitHub App
- **Cloud console**: Manual VM termination

## Best Practices for Users

### Repository Setup
1. Enable branch protection on main/master
2. Require PR reviews
3. Use CODEOWNERS files
4. Enable secret scanning

### Session Configuration
```yaml
# Minimal permissions
agent:
  type: claudecode

# Short timeouts
execution:
  timeout_minutes: 30

# Specific issue scope
prompt:
  issue: 123  # Work on single issue
```

### Security Checklist
- [ ] Branch protection enabled
- [ ] Secrets in secret manager (not in code)
- [ ] Appropriate VM size and timeout
- [ ] Clear issue scope in prompt
- [ ] Review PR before merging

## Compliance & Standards

### Security Standards
- **OWASP**: Following OWASP guidelines for application security
- **CIS Benchmarks**: VM hardening based on CIS benchmarks
- **Least Privilege**: NIST principle of least privilege

### Future Enhancements
1. **Container scanning**: Vulnerability scanning of agent images
2. **SAST integration**: Static analysis of generated code
3. **Network policies**: Kubernetes-style network policies
4. **Compliance scanning**: CIS/STIG compliance checks

## Security Contact

For security issues or concerns:
- Create a security advisory on GitHub
- Email: security@agentium.dev (planned)
- Do not create public issues for security vulnerabilities