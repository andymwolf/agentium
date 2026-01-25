# Agentium Security Model

This document describes the security model and best practices for Agentium.

## Overview

Agentium runs AI coding agents in ephemeral, isolated cloud VMs with strict security controls. The framework is designed with defense-in-depth principles to minimize risk from both external attackers and potentially compromised agents.

## Security Architecture

### 1. Trust Boundaries

The system has several trust boundaries:

- **User → CLI**: Fully trusted (local execution)
- **CLI → Cloud Provider**: Trusted (TLS + cloud authentication)
- **Controller → Agent**: Partially trusted (same VM, different containers)
- **Agent → GitHub**: Trusted (HTTPS + token authentication)
- **Agent → Internet**: Untrusted (egress filtering applied)

### 2. Isolation Layers

#### VM Isolation
- Each session runs in its own VM
- VMs are ephemeral and self-terminate after completion
- No persistent storage between sessions
- Network isolated with egress-only rules

#### Container Isolation
- Agents run in Docker containers
- Non-root user execution (UID 1000)
- Read-only bind mounts for credentials
- Separate workspaces per container

#### Permission Isolation
- Each VM has a unique service account
- Service accounts have minimal required permissions
- Resource-scoped IAM bindings restrict access to self only
- Time-limited credentials (GitHub tokens expire in 1 hour)

### 3. Secret Management

#### Secret Storage
- Secrets stored in cloud provider's secret management service
- Never stored in code or configuration files
- Environment-specific access controls

#### Secret Handling
- Secrets are fetched at runtime only
- Passed to containers via environment variables or mounted files
- Automatic scrubbing from logs and output
- No long-lived credentials

### 4. Network Security

#### Ingress
- No inbound connections allowed
- VMs not accessible from internet
- Management access only via cloud provider console

#### Egress
- Restricted to ports 443 (HTTPS), 80 (HTTP), and 22 (Git SSH)
- All other outbound traffic blocked
- No direct database or internal service access

## Security Controls

### 1. Authentication & Authorization

#### GitHub Authentication
- GitHub App authentication with JWT + installation tokens
- Short-lived tokens (1 hour expiration)
- Scoped to specific repositories
- No personal access tokens

#### Cloud Provider Authentication
- Service account authentication
- Workload identity where available
- No hardcoded credentials

### 2. Input Validation

All user inputs are validated:
- Session IDs must be valid UUIDs
- Git references are sanitized
- File paths are restricted to workspace
- Command arguments are validated against injection

### 3. Output Sanitization

#### Log Scrubbing
- Comprehensive regex patterns detect credentials
- All logs are scrubbed before output
- Covers common credential formats:
  - API keys and tokens
  - Bearer tokens
  - Cloud provider credentials
  - GitHub tokens
  - JWT tokens
  - SSH keys
  - Passwords

#### Structured Logging
- Cloud logging integration for audit trails
- Security events logged separately
- Metadata scrubbed before logging

### 4. Resource Limits

#### VM Limits
- Maximum runtime (2 hours default)
- CPU and memory limits
- Disk space limits
- Preemptible/spot instances

#### Container Limits
- CPU and memory limits via Docker
- Disk space restricted to workspace
- Network bandwidth implicitly limited

## Best Practices

### 1. Least Privilege

Always follow the principle of least privilege:
- Grant minimum required permissions
- Use resource-scoped IAM bindings
- Rotate credentials regularly
- Remove unused permissions

### 2. Defense in Depth

Multiple layers of security:
- Network isolation
- Container isolation
- Permission boundaries
- Input validation
- Output sanitization

### 3. Monitoring & Auditing

- Enable cloud audit logs
- Monitor for anomalous behavior
- Regular security reviews
- Incident response procedures

### 4. Secure Development

- No hardcoded secrets
- Security reviews for PRs
- Dependency scanning
- Container image scanning
- Regular security updates

## Threat Model

### External Threats

1. **Network Attacks**: Mitigated by no ingress, TLS everywhere
2. **Credential Theft**: Mitigated by short-lived tokens, secret management
3. **Supply Chain**: Mitigated by image scanning, dependency review

### Internal Threats

1. **Compromised Agent**: Limited by container isolation, permission boundaries
2. **Privilege Escalation**: Mitigated by non-root execution, minimal permissions
3. **Data Exfiltration**: Limited by network egress rules

### Operational Threats

1. **Misconfiguration**: Mitigated by infrastructure as code, validation
2. **Insider Threat**: Audit logs provide attribution
3. **Denial of Service**: Resource limits and quotas

## Incident Response

In case of a security incident:

1. **Immediate**: Terminate affected VMs
2. **Investigation**: Review audit logs
3. **Containment**: Revoke compromised credentials
4. **Recovery**: Deploy patched version
5. **Lessons Learned**: Update security controls

## Compliance

The framework is designed to support:
- SOC 2 compliance (audit logging, access controls, encryption)
- GDPR compliance (data minimization, right to erasure)
- Industry best practices (OWASP, CIS benchmarks)

## Security Checklist

Before deploying Agentium:

- [ ] Configure cloud provider secret management
- [ ] Set up restricted IAM roles and bindings
- [ ] Enable audit logging
- [ ] Configure network isolation (VPC/firewall rules)
- [ ] Review and sign container images
- [ ] Set appropriate resource limits
- [ ] Document incident response procedures
- [ ] Train operators on security practices

## Reporting Security Issues

To report security vulnerabilities:
1. Do NOT create public GitHub issues
2. Email security concerns to: [security contact]
3. Include detailed description and reproduction steps
4. Allow time for patching before disclosure

## Updates and Patches

Security updates are released as:
- Critical: Immediate patches for active exploits
- High: Patches within 7 days
- Medium: Patches within 30 days
- Low: Patches in regular releases

Always use the latest version for optimal security.