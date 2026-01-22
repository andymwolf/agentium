# Claude Code Runtime Dockerfile
# This image provides the runtime environment for Claude Code

FROM node:20-slim

# Install system dependencies
RUN apt-get update && apt-get install -y \
    git \
    curl \
    jq \
    python3 \
    python3-pip \
    build-essential \
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

# Install Claude Code CLI
RUN npm install -g @anthropic-ai/claude-code

# Create non-root user (remove node user which owns UID 1000 in base image)
RUN userdel -r node && useradd -m -u 1000 agentium

# Create workspace
RUN mkdir -p /workspace && chown agentium:agentium /workspace

# Configure git for the user
RUN git config --global user.email "agentium@example.com" && \
    git config --global user.name "Agentium Bot" && \
    git config --global init.defaultBranch main

# Set working directory
WORKDIR /workspace

# Switch to non-root user
USER agentium

# Default entrypoint
ENTRYPOINT ["claude"]
