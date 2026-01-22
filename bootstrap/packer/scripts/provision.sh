#!/bin/bash
# Agentium Image Provisioning Script
#
# This script installs all required tools into the base Ubuntu 22.04 image
# so that VMs can start sessions without lengthy cloud-init installations.
#
# Tools installed:
#   - Docker
#   - Node.js v20
#   - Claude Code CLI (@anthropic-ai/claude-code)
#   - GitHub CLI (gh)
#   - gcloud CLI (if not already present on GCP)
#   - Standard utilities (jq, curl, git, unzip, openssl)

set -euo pipefail

echo "=== Agentium Image Provisioning ==="
echo "Started at: $(date)"

# -----------------------------------------------
# System packages
# -----------------------------------------------
echo ">>> Updating system packages..."
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get upgrade -y
apt-get install -y \
  git \
  curl \
  jq \
  unzip \
  apt-transport-https \
  ca-certificates \
  gnupg \
  lsb-release \
  openssl \
  software-properties-common

# -----------------------------------------------
# Docker
# -----------------------------------------------
echo ">>> Installing Docker..."
curl -fsSL https://get.docker.com | sh
systemctl enable docker

# -----------------------------------------------
# Node.js v20 (via NodeSource)
# -----------------------------------------------
echo ">>> Installing Node.js v20..."
curl -fsSL https://deb.nodesource.com/setup_20.x | bash -
apt-get install -y nodejs

echo "Node.js version: $(node --version)"
echo "npm version: $(npm --version)"

# -----------------------------------------------
# Claude Code CLI
# -----------------------------------------------
echo ">>> Installing Claude Code CLI..."
npm install -g @anthropic-ai/claude-code

# -----------------------------------------------
# GitHub CLI
# -----------------------------------------------
echo ">>> Installing GitHub CLI..."
curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg \
  | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg
chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg
echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" \
  | tee /etc/apt/sources.list.d/github-cli.list > /dev/null
apt-get update
apt-get install -y gh

# -----------------------------------------------
# gcloud CLI
# -----------------------------------------------
echo ">>> Installing gcloud CLI..."
if ! command -v gcloud &> /dev/null; then
  export CLOUDSDK_INSTALL_DIR=/opt
  curl -fsSL https://sdk.cloud.google.com | bash -s -- --disable-prompts --install-dir=/opt
  cat > /etc/profile.d/gcloud.sh << 'GCLOUD_PROFILE'
if [ -f /opt/google-cloud-sdk/path.bash.inc ]; then
  source /opt/google-cloud-sdk/path.bash.inc
fi
GCLOUD_PROFILE
else
  echo "gcloud already installed: $(gcloud --version | head -1)"
fi

# -----------------------------------------------
# Create agentium user
# -----------------------------------------------
echo ">>> Creating agentium user..."
if ! id agentium &>/dev/null; then
  useradd -m -s /bin/bash agentium
  echo "agentium ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/agentium
  chmod 0440 /etc/sudoers.d/agentium
fi
usermod -aG docker agentium

# -----------------------------------------------
# Create workspace directory
# -----------------------------------------------
echo ">>> Creating workspace directory..."
mkdir -p /workspace
chown agentium:agentium /workspace
chmod 755 /workspace

# -----------------------------------------------
# Set up agentium user home directory
# -----------------------------------------------
mkdir -p /home/agentium/.config
chown -R agentium:agentium /home/agentium

# -----------------------------------------------
# Git configuration
# -----------------------------------------------
echo ">>> Configuring git..."
cat > /etc/gitconfig << 'GITCONFIG'
[user]
    name = Agentium Bot
    email = bot@agentium.dev
[init]
    defaultBranch = main
[safe]
    directory = /workspace
GITCONFIG

# -----------------------------------------------
# Session script placeholder
# -----------------------------------------------
echo ">>> Installing session launcher..."
cat > /usr/local/bin/agentium-session << 'SESSION'
#!/bin/bash
# Fetch and run the latest session script from agentium repo
curl -sL https://raw.githubusercontent.com/andymwolf/agentium/main/bootstrap/session.sh \
  -o /tmp/session.sh
chmod +x /tmp/session.sh
exec /tmp/session.sh "$@"
SESSION
chmod 755 /usr/local/bin/agentium-session

# -----------------------------------------------
# Version marker
# -----------------------------------------------
echo ">>> Writing version marker..."
cat > /etc/agentium-image << EOF
image_build_date=$(date -u +%Y-%m-%dT%H:%M:%SZ)
node_version=$(node --version)
npm_version=$(npm --version)
docker_version=$(docker --version)
gh_version=$(gh --version | head -1)
EOF

echo "=== Agentium Image Provisioning Complete ==="
echo "Completed at: $(date)"
