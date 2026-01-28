#!/bin/bash
set -e

# Only run in web/remote environments
if [ "$CLAUDE_CODE_REMOTE" != "true" ]; then
  echo "Skipping gh CLI install (not a remote environment)"
  exit 0
fi

# Check if gh is already installed
if command -v gh &> /dev/null; then
  echo "GitHub CLI already installed: $(gh --version | head -1)"
  exit 0
fi

echo "Installing GitHub CLI..."

# Install gh CLI (Debian/Ubuntu-based cloud images)
(
  sudo mkdir -p /etc/apt/keyrings
  curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | sudo tee /etc/apt/keyrings/githubcli-archive-keyring.gpg > /dev/null
  sudo chmod go+r /etc/apt/keyrings/githubcli-archive-keyring.gpg
  echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | sudo tee /etc/apt/sources.list.d/github-cli.list > /dev/null
  sudo apt-get update > /dev/null 2>&1
  sudo apt-get install -y gh > /dev/null 2>&1
) 2>&1

echo "GitHub CLI installed: $(gh --version | head -1)"

# Persist GH_TOKEN if GITHUB_TOKEN is available (for authentication)
if [ -n "$CLAUDE_ENV_FILE" ] && [ -n "$GITHUB_TOKEN" ]; then
  echo "export GH_TOKEN=\"\$GITHUB_TOKEN\"" >> "$CLAUDE_ENV_FILE"
  echo "GitHub CLI authentication configured via GITHUB_TOKEN"
fi

exit 0
