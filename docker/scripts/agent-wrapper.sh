#!/bin/bash
# Wrapper script that sets up workspace and installs language runtime before executing the agent

set -e

# Clone repository inside container if requested
if [ "${AGENTIUM_CLONE_INSIDE:-}" = "true" ]; then
    /runtime-scripts/setup-workspace.sh
fi

# Install runtime based on project type
/runtime-scripts/install-runtime.sh

# Source updated environment variables
source ~/.bashrc

# Also source cargo env if it exists (for Rust)
if [ -f "$HOME/.cargo/env" ]; then
    source "$HOME/.cargo/env"
fi

# Execute the original command (the agent)
exec "$@"