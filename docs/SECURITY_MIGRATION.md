# Security Migration Guide

This guide helps you migrate to the enhanced security configuration introduced in the security audit.

## Overview of Changes

The security audit identified several areas for improvement. The main changes are:

1. **Restricted IAM permissions** - Replace broad `compute.instanceAdmin.v1` with minimal custom role
2. **Resource-scoped bindings** - Limit VM access to only its own instance
3. **Input validation** - Prevent command injection attacks
4. **Output scrubbing** - Prevent credential leaks from agent output

## Migration Steps

### 1. Update Terraform Configuration

The Terraform module now includes a new `iam.tf` file that creates more restrictive IAM roles.

```bash
# Initialize Terraform to fetch the new configuration
terraform init -upgrade

# Plan to see the changes
terraform plan

# Apply the changes (this will recreate service accounts)
terraform apply
```

**Note**: Existing VMs will need to be recreated to use the new service accounts.

### 2. Update Controller Code

If you're building from source, the controller now includes:
- Input validation for commands
- Output scrubbing for agent logs
- Updated security utilities

```bash
# Rebuild the controller
go build -o agentium-controller ./cmd/controller

# Rebuild the Docker image
docker build -f docker/controller/Dockerfile -t agentium-controller:latest .
```

### 3. Update Secret Naming Convention

For enhanced security, secrets should now follow the naming pattern:
- `agentium-github-private-key` (not `github-private-key`)
- `agentium-*` prefix for all Agentium-related secrets

This allows the IAM condition to restrict access to only Agentium secrets.

### 4. Verify Security Controls

After migration, verify:

```bash
# Check that VMs can still self-delete
gcloud compute instances list --filter="name:agentium-*"

# Verify secret access is working
gcloud secrets versions access latest --secret=agentium-github-private-key

# Check audit logs for any permission denied errors
gcloud logging read "resource.type=gce_instance AND severity>=WARNING" --limit 50
```

## Breaking Changes

1. **Service Account Names**: New service accounts use `agentium-vm-` prefix instead of `agentium-`
2. **IAM Roles**: Custom role `agentiumVMSelfDelete` must be created before use
3. **Secret Names**: Must use `agentium-` prefix for the IAM condition to allow access

## Rollback Plan

If you need to rollback:

1. Comment out the new `iam.tf` file
2. Uncomment the original IAM configuration in `main.tf`
3. Run `terraform apply` to restore previous permissions

## Security Benefits

The new configuration provides:

- **Reduced blast radius**: Compromised VM can only affect itself
- **Audit compliance**: Meets principle of least privilege
- **Defense in depth**: Multiple layers of security controls
- **Credential protection**: Prevents accidental credential exposure

## Troubleshooting

### Permission Denied Errors

If VMs can't self-delete:
```bash
# Check the custom role exists
gcloud iam roles describe agentiumVMSelfDelete --project=PROJECT_ID

# Check IAM bindings
gcloud compute instances get-iam-policy INSTANCE_NAME --zone=ZONE
```

### Secret Access Issues

If VMs can't access secrets:
```bash
# Verify secret name starts with 'agentium-'
gcloud secrets list --filter="name:agentium-*"

# Check IAM conditions
gcloud projects get-iam-policy PROJECT_ID --flatten="bindings[].members" \
  --filter="bindings.members:serviceAccount:agentium-vm-*"
```

## Questions or Issues

For questions about the security migration:
1. Check the SECURITY.md documentation
2. Review the security audit in SECURITY_REVIEW.md
3. Contact the security team for assistance