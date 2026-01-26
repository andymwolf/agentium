#!/bin/bash
# Workspace setup script for cloning repository inside container
# This runs before the agent when AGENTIUM_CLONE_INSIDE=true

set -e

# Function to log messages
log() {
    echo "[setup-workspace] $1" >&2
}

# Check if workspace already has content (repo already cloned)
check_workspace() {
    if [ -d "/workspace/.git" ]; then
        log "Repository already exists in workspace, skipping clone"
        return 0
    fi
    return 1
}

# Authenticate with GitHub using GITHUB_TOKEN if available
auth_with_token() {
    if [ -n "$GITHUB_TOKEN" ]; then
        log "Authenticating with GitHub using GITHUB_TOKEN..."
        echo "$GITHUB_TOKEN" | gh auth login --with-token
        return $?
    fi
    return 1
}

# Check if gh is already authenticated
check_gh_auth() {
    if gh auth status >/dev/null 2>&1; then
        log "GitHub CLI is already authenticated"
        return 0
    fi
    return 1
}

# Interactive authentication via gh auth login
auth_interactive() {
    # Check if we have a TTY for interactive input
    if [ -t 0 ]; then
        log "No GITHUB_TOKEN found. Starting interactive GitHub authentication..."
        log "Please follow the prompts to authenticate with GitHub."
        echo
        gh auth login
        return $?
    else
        log "ERROR: No TTY available for interactive authentication"
        log "Please set GITHUB_TOKEN environment variable or run with -it flag"
        return 1
    fi
}

# Clone the repository
clone_repository() {
    if [ -z "$AGENTIUM_REPOSITORY" ]; then
        log "ERROR: AGENTIUM_REPOSITORY environment variable not set"
        return 1
    fi

    log "Cloning repository: $AGENTIUM_REPOSITORY"

    # Clone to a temp directory first, then move contents to /workspace
    # This handles the case where /workspace already exists but is empty
    TEMP_DIR=$(mktemp -d)

    if gh repo clone "$AGENTIUM_REPOSITORY" "$TEMP_DIR" -- --depth 1; then
        # Move all contents (including hidden files) to workspace
        shopt -s dotglob
        mv "$TEMP_DIR"/* /workspace/ 2>/dev/null || true
        shopt -u dotglob
        rm -rf "$TEMP_DIR"
        log "Repository cloned successfully"
        return 0
    else
        rm -rf "$TEMP_DIR"
        log "ERROR: Failed to clone repository"
        return 1
    fi
}

# Main setup logic
main() {
    log "Setting up workspace..."

    # Skip if workspace already has the repository
    if check_workspace; then
        return 0
    fi

    # Try to authenticate
    if ! check_gh_auth; then
        # Try token-based auth first, fall back to interactive
        if ! auth_with_token; then
            if ! auth_interactive; then
                log "ERROR: GitHub authentication failed"
                exit 1
            fi
        fi
    fi

    # Clone the repository
    if ! clone_repository; then
        exit 1
    fi

    log "Workspace setup complete"
}

# Run main function
main
