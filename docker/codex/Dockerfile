# Codex CLI Runtime Dockerfile
# This image provides the runtime environment for OpenAI's Codex CLI

FROM node:25-slim

# Version build args
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

# OCI image labels
LABEL org.opencontainers.image.title="Agentium Codex Agent"
LABEL org.opencontainers.image.description="Codex CLI runtime environment for Agentium"
LABEL org.opencontainers.image.version="${VERSION}"
LABEL org.opencontainers.image.revision="${COMMIT}"
LABEL org.opencontainers.image.created="${BUILD_DATE}"
LABEL org.opencontainers.image.source="https://github.com/andywolf/agentium"

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

# Install Codex CLI native binary directly from GitHub releases
# The npm package is a JS wrapper that spawns the vendored binary, but the vendor
# path setup doesn't work correctly with bun. Download the native binary directly.
RUN CODEX_VERSION=$(curl -s https://api.github.com/repos/openai/codex/releases/latest | jq -r '.tag_name') && \
    echo "Installing Codex CLI ${CODEX_VERSION}..." && \
    curl -fsSL "https://github.com/openai/codex/releases/download/${CODEX_VERSION}/codex-x86_64-unknown-linux-musl.tar.gz" -o /tmp/codex.tar.gz && \
    tar -xzf /tmp/codex.tar.gz -C /tmp && \
    mv /tmp/codex-x86_64-unknown-linux-musl /usr/local/bin/codex && \
    chmod +x /usr/local/bin/codex && \
    rm /tmp/codex.tar.gz && \
    codex --version

# Create non-root user (remove node user which owns UID 1000 in base image)
RUN userdel -r node && useradd -m -u 1000 agentium

# Allow agentium user to run sudo for package installation
RUN echo "agentium ALL=(ALL) NOPASSWD: ALL" >> /etc/sudoers

# Create workspace and Codex config directory
RUN mkdir -p /workspace && chown agentium:agentium /workspace && \
    mkdir -p /home/agentium/.codex && chown agentium:agentium /home/agentium/.codex

# Write version file for runtime introspection
RUN echo "${VERSION}" > /etc/agentium-version

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
