# Troubleshooting Guide

This guide covers common issues you may encounter when using Agentium and how to resolve them.

## Diagnostic Commands

Before troubleshooting, gather information with these commands:

```bash
# Check session status
agentium status

# View session logs
agentium logs SESSION_ID --tail 200

# Follow logs in real-time
agentium logs SESSION_ID --follow

# Verbose output for any command
agentium run --repo github.com/org/repo --issues 42 --verbose

# Dry run to test configuration (validates config without provisioning)
agentium run --repo github.com/org/repo --issues 42 --dry-run
```

## Configuration Issues

### "Repository is required"

**Cause:** The `--repo` flag was not provided. This flag is always required for the `run` command.

**Fix:** Always pass `--repo` on the command line:

```bash
agentium run --repo github.com/org/repo --issues 42
```

### "Cloud provider not configured"

**Cause:** Missing or invalid `cloud.provider` in configuration.

**Fix:** Ensure your config specifies a valid provider:

```yaml
cloud:
  provider: "gcp"  # Currently only "gcp" is fully supported
  region: "us-central1"
  project: "your-gcp-project"
```

### "Config file not found"

**Cause:** No `.agentium.yaml` in the current directory.

**Fix:** Initialize configuration:

```bash
agentium init --repo github.com/org/repo --provider gcp
```

Or specify the config path:

```bash
agentium run --config /path/to/.agentium.yaml --issues 42
```

### Invalid configuration values

**Cause:** Typos or incorrect types in config file.

**Fix:** Validate your YAML syntax and check field types:

```bash
# Check YAML syntax
python3 -c "import yaml; yaml.safe_load(open('.agentium.yaml'))"
```

Common mistakes:
- `app_id` must be a number, not a string
- `max_duration` must be a valid Go duration (e.g., `2h`, `30m`)
- `use_spot` must be `true`/`false`, not `"yes"`/`"no"`

## Cloud Provider Issues

### GCP: "Permission denied"

**Cause:** Insufficient IAM permissions.

**Fix:** Ensure your account has required roles:

```bash
# Check current authentication
gcloud auth list

# Grant Compute Engine access
gcloud projects add-iam-policy-binding PROJECT_ID \
  --member="user:you@example.com" \
  --role="roles/compute.instanceAdmin.v1"

# Grant Secret Manager access
gcloud projects add-iam-policy-binding PROJECT_ID \
  --member="user:you@example.com" \
  --role="roles/secretmanager.secretAccessor"
```

### GCP: "API not enabled"

**Cause:** Required GCP APIs haven't been enabled.

**Fix:**

```bash
gcloud services enable \
  compute.googleapis.com \
  secretmanager.googleapis.com
```

### GCP: "Quota exceeded"

**Cause:** Hit GCP resource quotas.

**Fix:** Check and request quota increases:

```bash
gcloud compute project-info describe --project PROJECT_ID
```

Or visit the GCP Console: **IAM & Admin** > **Quotas**.

### GCP: VM fails to provision

**Cause:** Various (region unavailable, machine type not available, etc.)

**Fix:**
1. Try a different zone/region
2. Try a different machine type
3. Check GCP status page: https://status.cloud.google.com/

```bash
# List available zones
gcloud compute zones list --filter="region:us-central1"

# Check machine type availability
gcloud compute machine-types list --zones=us-central1-a
```

### AWS/Azure: "Not yet implemented"

**Cause:** AWS and Azure providers are not yet available.

**Fix:** Use GCP as the cloud provider, or use the bootstrap system:

```bash
cd bootstrap
./run.sh --repo org/repo --issue 42 \
  --app-id 123456 --installation-id 789012 \
  --private-key-secret github-app-key
```

## GitHub Authentication Issues

### "Bad credentials"

**Cause:** Invalid GitHub App ID or private key.

**Fix:**
1. Verify App ID matches your GitHub App settings page
2. Check the private key in secret storage:

```bash
# GCP: Verify key exists and is readable
gcloud secrets versions access latest --secret="github-app-key" | head -1
# Should show: -----BEGIN RSA PRIVATE KEY-----
```

3. Ensure the key hasn't been rotated on GitHub

### "Not Found" for installation

**Cause:** Invalid Installation ID or app not installed on repo.

**Fix:**
1. Verify the Installation ID:
   - Go to GitHub Settings > Applications > Installed Apps
   - Click "Configure" on your Agentium app
   - The Installation ID is in the URL: `/installations/INSTALL_ID`

2. Check the app is installed on the target repository

### "Resource not accessible by integration"

**Cause:** GitHub App lacks required permissions.

**Fix:** Update app permissions:
1. Go to GitHub App settings page
2. Under "Permissions", ensure:
   - Contents: Read & write
   - Issues: Read
   - Pull requests: Read & write
   - Metadata: Read
3. After updating, re-accept the new permissions on the installation

### Token generation/JWT errors

**Cause:** Private key format issues.

**Fix:**
1. Verify key format (PKCS#1 or PKCS#8 are supported):
   ```bash
   openssl rsa -in private-key.pem -check -noout
   ```

2. Ensure the full key is stored (including BEGIN/END lines):
   ```bash
   gcloud secrets versions access latest --secret="github-app-key" | wc -l
   # Should be ~27-28 lines for a 2048-bit key
   ```

3. Re-generate the key on GitHub if corrupted

## Session Issues

### Session times out

**Cause:** Session exceeded `max_duration` before completing.

**Fix:** Increase the duration limit:

```bash
agentium run --repo github.com/org/repo --issues 42 --max-duration 4h
```

Or reduce scope by working on fewer issues at once.

### Session reaches max iterations

**Cause:** Agent couldn't complete in the allotted iterations.

**Fix:**

```bash
# Increase iteration limit
agentium run --repo github.com/org/repo --issues 42 --max-iterations 50

# Or break complex issues into smaller ones
agentium run --repo github.com/org/repo --issues 42   # Just one issue
```

### Agent creates wrong/incomplete PR

**Cause:** Issue description may be ambiguous or too complex.

**Fix:**
1. Add more detail to the issue description
2. Create a `.agentium/AGENT.md` with project-specific instructions
3. Use `--prompt` for additional guidance:
   ```bash
   agentium run --repo github.com/org/repo --issues 42 --prompt "Focus on error handling, add unit tests"
   ```

### Session stuck / not progressing

**Cause:** Agent may be in a loop or blocked.

**Fix:**

```bash
# Check logs for status
agentium logs SESSION_ID --follow

# Force destroy if stuck
agentium destroy SESSION_ID --force
```

### "BLOCKED" status signal

**Cause:** Agent cannot proceed without human intervention.

**Fix:** Check the logs for the blocking reason:

```bash
agentium logs SESSION_ID --tail 50
```

Common blockers:
- Missing dependencies or tools
- Unclear requirements
- Permission issues within the repo

## Container Issues

### Controller container fails to start

**Cause:** Docker image pull failure or container configuration issue.

**Fix:**
1. Verify the controller image is accessible:
   ```bash
   docker pull ghcr.io/andymwolf/agentium-controller:latest
   ```

2. Check cloud-init logs on the VM:
   ```bash
   gcloud compute ssh INSTANCE_NAME --zone=ZONE \
     --command="cat /var/log/cloud-init-output.log"
   ```

3. If using a custom controller image, verify the full path is correct in your config:
   ```yaml
   controller:
     image: "your-registry.example.com/agentium-controller:v1.0"
   ```

### Building custom container images

If you need a custom controller image (e.g., with additional tools pre-installed), build from the Dockerfiles in the `docker/` directory:

```bash
# Build controller image
docker build -t your-registry/agentium-controller:custom -f docker/Dockerfile.controller .

# Push to your registry
docker push your-registry/agentium-controller:custom
```

Then update your `.agentium.yaml`:

```yaml
controller:
  image: "your-registry/agentium-controller:custom"
```

### Agent container fails

**Cause:** Missing environment variables or container issues.

**Fix:** Check the session logs:

```bash
agentium logs SESSION_ID --tail 200
```

Look for:
- Missing `GITHUB_TOKEN` - GitHub authentication failed
- Missing `ANTHROPIC_API_KEY` - Claude API key not provided (when using `api` auth mode)
- Container exit codes (non-zero indicates failure)

### "Image not found" errors

**Cause:** Container image doesn't exist or isn't accessible.

**Fix:**
1. Verify the image exists:
   ```bash
   docker pull ghcr.io/andymwolf/agentium-controller:latest
   ```

2. Check if using a custom controller image in config:
   ```yaml
   controller:
     image: "ghcr.io/andymwolf/agentium-controller:latest"
   ```

3. For private registries, ensure the VM has credentials to pull images

## Network Issues

### VM can't reach GitHub

**Cause:** Network/firewall configuration blocking outbound HTTPS.

**Fix:**
1. Verify firewall rules allow outbound port 443
2. Check if the VM has a public IP (required for GitHub access)
3. For GCP, check VPC firewall rules:
   ```bash
   gcloud compute firewall-rules list
   ```

### Terraform fails to connect

**Cause:** Network issues or expired credentials.

**Fix:**
1. Re-authenticate:
   ```bash
   gcloud auth application-default login
   ```

2. Check internet connectivity
3. Verify Terraform provider versions

## Cost and Resource Issues

### Unexpected charges

**Cause:** VMs not terminating properly.

**Fix:**
1. List active sessions:
   ```bash
   agentium status
   ```

2. Destroy any lingering sessions:
   ```bash
   agentium destroy SESSION_ID --force
   ```

3. Check for orphaned VMs in the cloud console:
   ```bash
   gcloud compute instances list --filter="name~agentium"
   ```

4. Delete orphaned resources:
   ```bash
   gcloud compute instances delete INSTANCE_NAME --zone=ZONE
   ```

### Spot/preemptible VM terminated early

**Cause:** Cloud provider reclaimed the spot instance.

**Fix:**
1. Re-run the session:
   ```bash
   agentium run --repo github.com/org/repo --issues 42
   ```

2. Or disable spot instances for critical sessions:
   ```yaml
   cloud:
     use_spot: false
   ```

## Debugging Tips

### 1. Enable verbose output

```bash
agentium run --repo github.com/org/repo --issues 42 --verbose
```

### 2. Use dry-run mode

```bash
agentium run --repo github.com/org/repo --issues 42 --dry-run
```

This validates configuration and shows what would be provisioned without creating resources.

### 3. Check session logs

```bash
# Real-time log streaming
agentium logs SESSION_ID --follow

# Last 200 lines
agentium logs SESSION_ID --tail 200

# Logs from last hour
agentium logs SESSION_ID --since 1h
```

### 4. SSH into the VM (GCP)

For deep debugging, SSH into the running VM:

```bash
gcloud compute ssh INSTANCE_NAME --zone=ZONE
```

Once connected:

```bash
# Check Docker containers
docker ps -a

# View controller logs
docker logs $(docker ps -q --filter name=controller)

# View agent container logs
docker logs $(docker ps -q --filter name=agent)

# Check cloud-init
cat /var/log/cloud-init-output.log

# Check system resources
free -h
df -h
```

### 5. Check Terraform state

If provisioning fails:

```bash
cd terraform
terraform show
terraform plan
```

## Getting Help

If you're still stuck:

1. **Check existing issues:** [GitHub Issues](https://github.com/andymwolf/agentium/issues)
2. **Open a new issue** with:
   - Agentium version / commit hash
   - Cloud provider and region
   - Configuration (redact secrets!)
   - Full error messages
   - Session logs (if available)
3. **Include reproduction steps** so the issue can be investigated

## Common Error Messages Reference

| Error Message | Likely Cause | Quick Fix |
|--------------|-------------|-----------|
| "required flag(s) \"repo\" not set" | Missing `--repo` flag | Always provide `--repo` on the command line |
| "at least one issue or PR is required (use --issues or --prs)" | Missing task specification | Provide `--issues` or `--prs` flag |
| "cloud provider is required (use --provider or set in config)" | Missing provider | Set `cloud.provider` in config or pass `--provider` |
| "invalid cloud provider: X (must be gcp, aws, or azure)" | Typo in provider name | Use one of: `gcp`, `aws`, `azure` |
| "invalid agent: X (must be claude-code or aider)" | Invalid agent name | Use `claude-code` or `aider` |
| "GitHub App ID is required" | Missing `github.app_id` in config | Set `github.app_id` in `.agentium.yaml` |
| "GitHub App Installation ID is required" | Missing `github.installation_id` | Set `github.installation_id` in config |
| "GitHub App private key secret path is required" | Missing `github.private_key_secret` | Set `github.private_key_secret` in config |
| "oauth auth_mode is only supported with the claude-code agent" | Using OAuth with Aider | Switch to `--agent claude-code` or use `api` auth mode |
| "AWS provisioner not yet implemented" | Using unsupported provider | Switch to `gcp` |
| "Azure provisioner not yet implemented" | Using unsupported provider | Switch to `gcp` |
| "bad credentials" | Invalid GitHub App auth | Verify App ID and private key |
| "not found" | Invalid Installation ID | Check installation settings |
| "resource not accessible" | Missing app permissions | Update app permissions |
| "quota exceeded" | Hit cloud resource limits | Request quota increase |
| "permission denied" | Missing IAM permissions | Grant required roles |
| "image not found" | Container image missing | Pull or verify image exists |
