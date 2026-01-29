# Codex CLI Runtime Dockerfile
# This image provides the runtime environment for OpenAI's Codex CLI

FROM node:20-slim

# Install system dependencies
RUN apt-get update && apt-get install -y \
    git \
    curl \
    jq \
    python3 \
    python3-pip \
    build-essential \
    sudo \
    unzip \
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

# Install bun for faster package installation
RUN curl -fsSL https://bun.sh/install | bash
ENV PATH="/root/.bun/bin:${PATH}"

# Install Codex CLI and symlink to system path for non-root user access
RUN bun add -g @openai/codex && \
    ln -s /root/.bun/bin/codex /usr/local/bin/codex

# Create non-root user (remove node user which owns UID 1000 in base image)
RUN userdel -r node && useradd -m -u 1000 agentium

# Allow agentium user to run sudo for package installation
RUN echo "agentium ALL=(ALL) NOPASSWD: ALL" >> /etc/sudoers

# Create workspace and Codex config directory
RUN mkdir -p /workspace && chown agentium:agentium /workspace && \
    mkdir -p /home/agentium/.codex && chown agentium:agentium /home/agentium/.codex

# Copy runtime installation scripts
COPY docker/scripts/install-runtime.sh /runtime-scripts/install-runtime.sh
COPY docker/scripts/agent-wrapper.sh /runtime-scripts/agent-wrapper.sh
COPY docker/scripts/setup-workspace.sh /runtime-scripts/setup-workspace.sh
RUN chmod +x /runtime-scripts/*.sh

# Set working directory
WORKDIR /workspace

# Switch to non-root user
USER agentium

# Configure git for the agentium user
RUN git config --global user.email "agentium@example.com" && \
    git config --global user.name "Agentium Bot" && \
    git config --global init.defaultBranch main

# Use wrapper script as entrypoint
ENTRYPOINT ["/runtime-scripts/agent-wrapper.sh", "codex"]
