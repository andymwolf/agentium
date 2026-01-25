#!/bin/bash
# Wrapper script that installs language runtime before executing the agent

set -e

# Install runtime based on project type
/runtime-scripts/install-runtime.sh

# Source updated environment variables
source ~/.bashrc

# Execute the original command (the agent)
exec "$@"