# Aider Runtime Dockerfile
# This image provides the runtime environment for Aider

FROM python:3.11-slim

# Install system dependencies
RUN apt-get update && apt-get install -y \
    git \
    curl \
    jq \
    build-essential \
    sudo \
    && rm -rf /var/lib/apt/lists/*

# Install GitHub CLI
RUN curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | \
    dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg && \
    chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg && \
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | \
    tee /etc/apt/sources.list.d/github-cli.list > /dev/null && \
    apt-get update && \
    apt-get install -y gh && \
    rm -rf /var/lib/apt/lists/*

# Install Aider
RUN pip install --no-cache-dir aider-chat

# Create non-root user (remove any existing UID 1000 user from base image)
RUN getent passwd 1000 > /dev/null 2>&1 && userdel -r $(getent passwd 1000 | cut -d: -f1) || true && \
    useradd -m -u 1000 agentium

# Allow agentium user to run sudo for package installation
RUN echo "agentium ALL=(ALL) NOPASSWD: ALL" >> /etc/sudoers

# Create workspace
RUN mkdir -p /workspace && chown agentium:agentium /workspace

# Copy runtime installation scripts
COPY docker/scripts/install-runtime.sh /runtime-scripts/install-runtime.sh
COPY docker/scripts/agent-wrapper.sh /runtime-scripts/agent-wrapper.sh
COPY docker/scripts/setup-workspace.sh /runtime-scripts/setup-workspace.sh
RUN chmod +x /runtime-scripts/*.sh

# Configure git for the user
RUN git config --global user.email "agentium@example.com" && \
    git config --global user.name "Agentium Bot" && \
    git config --global init.defaultBranch main

# Set working directory
WORKDIR /workspace

# Switch to non-root user
USER agentium

# Use wrapper script as entrypoint
ENTRYPOINT ["/runtime-scripts/agent-wrapper.sh", "aider"]
