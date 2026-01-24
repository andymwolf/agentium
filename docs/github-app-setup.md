# GitHub App Setup Guide

Agentium uses a GitHub App for authenticating with your repositories. This guide walks you through creating, configuring, and installing a GitHub App.

## Why a GitHub App?

GitHub Apps provide:
- **Fine-grained permissions** - Only grant access to what's needed
- **Per-repository access** - Control which repos the agent can access
- **Short-lived tokens** - Installation tokens expire in 1 hour
- **No personal account dependency** - Works independently of user accounts
- **Audit trail** - All actions are attributed to the app

## Step 1: Create the GitHub App

1. Go to **GitHub Settings** > **Developer Settings** > **GitHub Apps**
   - Direct URL: `https://github.com/settings/apps`
   - For organizations: `https://github.com/organizations/YOUR_ORG/settings/apps`

2. Click **"New GitHub App"**

3. Fill in the basic information:

| Field | Value |
|-------|-------|
| **GitHub App name** | `agentium-bot` (or your preferred name) |
| **Homepage URL** | Your repository URL or `https://github.com/andymwolf/agentium` |
| **Webhook** | Uncheck **"Active"** (webhooks are not needed) |

## Step 2: Set Permissions

Under **"Permissions"**, configure the following:

### Repository Permissions

| Permission | Access Level | Purpose |
|-----------|-------------|---------|
| **Contents** | Read & write | Clone repos, push branches |
| **Issues** | Read | Read issue descriptions and comments |
| **Pull requests** | Read & write | Create and update PRs |
| **Metadata** | Read | Access basic repository info |

### Organization Permissions

No organization permissions are required.

### Account Permissions

No account permissions are required.

## Step 3: Configure Additional Settings

| Setting | Value |
|---------|-------|
| **Where can this app be installed?** | "Only on this account" (recommended for security) |
| **Request user authorization (OAuth)** | Leave unchecked |

## Step 4: Create the App

Click **"Create GitHub App"**.

After creation, note the **App ID** displayed on the app settings page.

## Step 5: Generate a Private Key

1. On the app settings page, scroll down to **"Private keys"**
2. Click **"Generate a private key"**
3. A `.pem` file will be downloaded automatically
4. **Store this file securely** - it's used to authenticate as the app

> **Important:** Keep the private key secure. Never commit it to a repository.

## Step 6: Install the App

1. On the app settings page, click **"Install App"** in the left sidebar
2. Select your account or organization
3. Choose which repositories to grant access:
   - **"All repositories"** - Access all current and future repos
   - **"Only select repositories"** - Recommended for security; select specific repos
4. Click **"Install"**

## Step 7: Note the Installation ID

After installation, you'll be redirected to a URL like:

```
https://github.com/settings/installations/12345678
```

The number at the end (`12345678`) is your **Installation ID**.

Alternatively, find it via the GitHub API:

```bash
# List installations (requires a JWT - see below)
curl -H "Authorization: Bearer YOUR_JWT" \
  https://api.github.com/app/installations
```

## Step 8: Store the Private Key in Cloud Secrets

### GCP (Secret Manager)

```bash
# Create the secret
gcloud secrets create github-app-key \
  --replication-policy="automatic"

# Upload the private key
gcloud secrets versions add github-app-key \
  --data-file=/path/to/your-downloaded-key.pem

# Verify
gcloud secrets versions access latest --secret="github-app-key" | head -1
# Should output: -----BEGIN RSA PRIVATE KEY-----
```

### AWS (Secrets Manager) - Planned

```bash
aws secretsmanager create-secret \
  --name github-app-key \
  --secret-binary fileb:///path/to/your-downloaded-key.pem
```

### Azure (Key Vault) - Planned

```bash
az keyvault secret set \
  --vault-name your-vault \
  --name github-app-key \
  --file /path/to/your-downloaded-key.pem
```

## Step 9: Configure Agentium

Add the GitHub App credentials to your `.agentium.yaml`:

```yaml
github:
  app_id: 123456                    # From Step 4
  installation_id: 12345678         # From Step 7
  private_key_secret: "projects/your-gcp-project/secrets/github-app-key"
                                    # From Step 8
```

## Verify Setup

Test that authentication works:

```bash
agentium run --repo github.com/your-org/your-repo --issues 1 --dry-run
```

A successful dry run confirms that Agentium can:
- Read the configuration
- Access the secret
- Generate a valid GitHub token

## Authentication Flow

Understanding how Agentium uses the GitHub App:

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────┐
│   Agentium CLI  │     │  Secret Manager  │     │   GitHub    │
│                 │     │                  │     │             │
│  1. Read config │     │                  │     │             │
│     (app_id,    │     │                  │     │             │
│      install_id)│     │                  │     │             │
│                 │     │                  │     │             │
│  2. Fetch key ──┼────▶│  Return .pem     │     │             │
│                 │◀────┼──────────────────│     │             │
│                 │     │                  │     │             │
│  3. Generate    │     │                  │     │             │
│     JWT (10min) │     │                  │     │             │
│                 │     │                  │     │             │
│  4. Request ────┼─────┼──────────────────┼────▶│ Validate    │
│     install     │     │                  │     │ JWT         │
│     token       │     │                  │     │             │
│                 │◀────┼──────────────────┼─────│ Return      │
│  5. Use token   │     │                  │     │ token (1hr) │
│     for API     │     │                  │     │             │
└─────────────────┘     └──────────────────┘     └─────────────┘
```

**Key details:**
- JWTs are valid for 10 minutes (GitHub's maximum)
- Installation tokens expire after 1 hour
- Tokens are scoped to the installed repositories only
- Both PKCS#1 and PKCS#8 key formats are supported

## Security Best Practices

1. **Minimal permissions** - Only grant the permissions listed above
2. **Repository selection** - Install only on repos that need automation
3. **Key rotation** - Periodically rotate the private key
4. **Secret management** - Always use cloud secret managers, never local files in production
5. **Single-account installation** - Set "Only on this account" to prevent unauthorized installations
6. **Audit regularly** - Review the app's activity in GitHub Settings > Applications

## Troubleshooting

### "Bad credentials" error

- Verify the App ID matches your GitHub App
- Check that the private key hasn't been rotated
- Ensure the secret in cloud storage contains the full PEM file

### "Not Found" for installation

- Verify the Installation ID is correct
- Check that the app is still installed on the target repository
- Ensure the app has access to the specific repository

### "Resource not accessible by integration"

- The app lacks required permissions
- Go to app settings and verify all permissions listed above are set
- After changing permissions, you may need to re-accept on the installation

### Token generation fails

- Private key may be in wrong format
- Agentium supports both PKCS#1 (`BEGIN RSA PRIVATE KEY`) and PKCS#8 (`BEGIN PRIVATE KEY`)
- Verify the key file isn't corrupted:
  ```bash
  openssl rsa -in private-key.pem -check -noout
  ```

## Managing Multiple Repositories

To use Agentium across multiple repositories:

1. **Same organization** - Install the app on additional repos via the installation settings
2. **Different organizations** - Create separate installations for each org (each gets its own Installation ID)
3. **Configuration** - Use `--repo` flag or separate `.agentium.yaml` files per project

## Revoking Access

To revoke Agentium's access:

1. Go to **Settings** > **Applications** > **Installed GitHub Apps**
2. Find your Agentium app
3. Click **"Configure"**
4. Click **"Uninstall"** to remove access

This immediately revokes all tokens and prevents further API access.
