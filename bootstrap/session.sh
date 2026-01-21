#!/bin/bash
# Agentium Session Bootstrap Script
#
# This script runs on cloud VMs to orchestrate agent sessions.
# It handles GitHub authentication, repository cloning, and Claude invocation.
#
# Required environment variables:
#   AGENTIUM_SESSION_ID      - Unique session identifier
#   AGENTIUM_REPOSITORY      - Target repository (owner/repo format)
#   AGENTIUM_ISSUES          - Comma-separated list of issue numbers
#   GITHUB_APP_ID            - GitHub App ID for authentication
#   GITHUB_INSTALLATION_ID   - GitHub App installation ID
#   GITHUB_PRIVATE_KEY_SECRET - GCP Secret Manager path for private key
#
# Optional environment variables:
#   AGENTIUM_MAX_ITERATIONS  - Maximum iterations (default: 5)
#   AGENTIUM_BRANCH          - Base branch to work from (default: main)
#   ANTHROPIC_API_KEY        - API key for Claude (or fetched from secrets)

set -euo pipefail

# Configuration
WORKSPACE="/workspace"
SYSTEM_MD_URL="https://raw.githubusercontent.com/andymwolf/agentium/main/bootstrap/SYSTEM.md"
LOG_PREFIX="[agentium]"

# Logging functions
log_info() {
    echo "${LOG_PREFIX} [INFO] $(date '+%Y-%m-%d %H:%M:%S') $*"
}

log_error() {
    echo "${LOG_PREFIX} [ERROR] $(date '+%Y-%m-%d %H:%M:%S') $*" >&2
}

log_debug() {
    if [[ "${AGENTIUM_DEBUG:-0}" == "1" ]]; then
        echo "${LOG_PREFIX} [DEBUG] $(date '+%Y-%m-%d %H:%M:%S') $*"
    fi
}

# Fetch secret from GCP Secret Manager
fetch_secret() {
    local secret_name="$1"
    log_debug "Fetching secret: ${secret_name}"
    gcloud secrets versions access latest --secret="${secret_name}"
}

# Generate JWT for GitHub App authentication
generate_jwt() {
    local app_id="$1"
    local private_key="$2"

    local now=$(date +%s)
    local iat=$((now - 60))
    local exp=$((now + 600))

    local header=$(echo -n '{"alg":"RS256","typ":"JWT"}' | base64 | tr -d '=' | tr '/+' '_-' | tr -d '\n')
    local payload=$(echo -n "{\"iat\":${iat},\"exp\":${exp},\"iss\":\"${app_id}\"}" | base64 | tr -d '=' | tr '/+' '_-' | tr -d '\n')

    local unsigned="${header}.${payload}"
    local signature=$(echo -n "${unsigned}" | openssl dgst -sha256 -sign <(echo "${private_key}") | base64 | tr -d '=' | tr '/+' '_-' | tr -d '\n')

    echo "${unsigned}.${signature}"
}

# Get installation access token from GitHub
get_installation_token() {
    local jwt="$1"
    local installation_id="$2"

    local response=$(curl -s -X POST \
        -H "Authorization: Bearer ${jwt}" \
        -H "Accept: application/vnd.github+json" \
        "https://api.github.com/app/installations/${installation_id}/access_tokens")

    echo "${response}" | jq -r '.token'
}

# Clone repository with authentication
clone_repository() {
    local repo="$1"
    local token="$2"
    local branch="${3:-main}"

    log_info "Cloning repository: ${repo}"

    local clone_url="https://x-access-token:${token}@github.com/${repo}.git"

    if [[ -d "${WORKSPACE}/.git" ]]; then
        log_info "Repository already cloned, pulling latest changes"
        cd "${WORKSPACE}"
        git fetch origin
        git checkout "${branch}"
        git pull origin "${branch}"
    else
        git clone --branch "${branch}" "${clone_url}" "${WORKSPACE}"
        cd "${WORKSPACE}"
    fi

    # Configure git for commits
    git config user.name "Agentium Bot"
    git config user.email "bot@agentium.dev"
}

# Fetch issue details from GitHub
fetch_issue_details() {
    local repo="$1"
    local issue_number="$2"

    gh issue view "${issue_number}" --repo "${repo}" --json number,title,body
}

# Build the prompt with issue context
build_prompt() {
    local repo="$1"
    local issues="$2"

    local prompt="You are working on repository: ${repo}\n\n"
    prompt+="Complete the following GitHub issue(s):\n\n"

    IFS=',' read -ra ISSUE_ARRAY <<< "${issues}"
    for issue_num in "${ISSUE_ARRAY[@]}"; do
        issue_num=$(echo "${issue_num}" | tr -d ' ')
        local issue_json=$(fetch_issue_details "${repo}" "${issue_num}" 2>/dev/null || echo '{}')

        if [[ "${issue_json}" != "{}" ]]; then
            local title=$(echo "${issue_json}" | jq -r '.title // "Unknown"')
            local body=$(echo "${issue_json}" | jq -r '.body // "No description"' | head -c 2000)
            prompt+="Issue #${issue_num}: ${title}\n"
            prompt+="Description: ${body}\n\n"
        else
            prompt+="Issue #${issue_num}\n\n"
        fi
    done

    echo -e "${prompt}"
}

# Fetch SYSTEM.md from agentium repository
fetch_system_md() {
    local output_path="/tmp/SYSTEM.md"

    log_info "Fetching SYSTEM.md from agentium repository"

    if curl -sL "${SYSTEM_MD_URL}" -o "${output_path}"; then
        log_info "SYSTEM.md fetched successfully"
        echo "${output_path}"
    else
        log_error "Failed to fetch SYSTEM.md, using embedded fallback"
        # Fallback: copy from local if available (baked into image)
        if [[ -f "/etc/agentium/SYSTEM.md" ]]; then
            cp "/etc/agentium/SYSTEM.md" "${output_path}"
            echo "${output_path}"
        else
            log_error "No SYSTEM.md available"
            return 1
        fi
    fi
}

# Run Claude Code with layered instructions
run_claude() {
    local prompt="$1"
    local system_md="$2"
    local iteration="${3:-1}"

    log_info "Running Claude Code (iteration ${iteration})"

    # Build Claude command
    local claude_args=(
        "--print"
        "--dangerously-skip-permissions"
        "--system-prompt" "${system_md}"
    )

    # Check for project-specific instructions
    if [[ -f "${WORKSPACE}/.agentium/AGENT.md" ]]; then
        log_info "Found project-specific instructions: .agentium/AGENT.md"
        claude_args+=("--append-system-prompt" "${WORKSPACE}/.agentium/AGENT.md")
    fi

    # Export session variables for Claude
    export AGENTIUM_ITERATION="${iteration}"

    # Run Claude
    cd "${WORKSPACE}"
    claude "${claude_args[@]}" "${prompt}"
}

# Main execution
main() {
    log_info "Starting Agentium session: ${AGENTIUM_SESSION_ID:-unknown}"
    log_info "Repository: ${AGENTIUM_REPOSITORY}"
    log_info "Issues: ${AGENTIUM_ISSUES}"

    # Validate required environment variables
    : "${AGENTIUM_SESSION_ID:?AGENTIUM_SESSION_ID is required}"
    : "${AGENTIUM_REPOSITORY:?AGENTIUM_REPOSITORY is required}"
    : "${AGENTIUM_ISSUES:?AGENTIUM_ISSUES is required}"

    # Get GitHub token
    local github_token=""

    if [[ -n "${GITHUB_TOKEN:-}" ]]; then
        log_info "Using GITHUB_TOKEN from environment"
        github_token="${GITHUB_TOKEN}"
    elif [[ -n "${GITHUB_APP_ID:-}" ]] && [[ -n "${GITHUB_INSTALLATION_ID:-}" ]] && [[ -n "${GITHUB_PRIVATE_KEY_SECRET:-}" ]]; then
        log_info "Generating GitHub App installation token"

        # Fetch private key from Secret Manager
        local private_key=$(fetch_secret "${GITHUB_PRIVATE_KEY_SECRET}")

        # Generate JWT
        local jwt=$(generate_jwt "${GITHUB_APP_ID}" "${private_key}")

        # Get installation token
        github_token=$(get_installation_token "${jwt}" "${GITHUB_INSTALLATION_ID}")

        if [[ -z "${github_token}" ]] || [[ "${github_token}" == "null" ]]; then
            log_error "Failed to get installation token"
            exit 1
        fi

        log_info "Installation token obtained successfully"
    else
        log_error "No GitHub authentication configured"
        exit 1
    fi

    # Export token for gh CLI
    export GITHUB_TOKEN="${github_token}"

    # Create workspace
    mkdir -p "${WORKSPACE}"

    # Clone repository
    clone_repository "${AGENTIUM_REPOSITORY}" "${github_token}" "${AGENTIUM_BRANCH:-main}"

    # Fetch SYSTEM.md
    local system_md=$(fetch_system_md)
    if [[ -z "${system_md}" ]]; then
        log_error "Failed to fetch SYSTEM.md"
        exit 1
    fi

    # Build prompt with issue context
    local prompt=$(build_prompt "${AGENTIUM_REPOSITORY}" "${AGENTIUM_ISSUES}")

    # Run iterations
    local max_iterations="${AGENTIUM_MAX_ITERATIONS:-5}"
    local iteration=1

    while [[ ${iteration} -le ${max_iterations} ]]; do
        log_info "=== Iteration ${iteration}/${max_iterations} ==="

        if run_claude "${prompt}" "${system_md}" "${iteration}"; then
            log_info "Iteration ${iteration} completed successfully"
        else
            log_error "Iteration ${iteration} failed"
        fi

        # Check if PRs were created (simple check)
        local pr_count=$(gh pr list --repo "${AGENTIUM_REPOSITORY}" --author "@me" --state open --json number | jq '. | length')
        if [[ ${pr_count} -gt 0 ]]; then
            log_info "PRs created, session complete"
            break
        fi

        ((iteration++))
    done

    log_info "Session complete"
}

# Run main function
main "$@"
