#!/bin/bash
# Agentium Session Bootstrap Script
#
# This script runs on cloud VMs to orchestrate agent sessions.
# It handles GitHub authentication, repository cloning, and Claude invocation.
#
# Required environment variables:
#   AGENTIUM_SESSION_ID      - Unique session identifier
#   AGENTIUM_REPOSITORY      - Target repository (owner/repo format)
#   AGENTIUM_ISSUES          - Comma-separated list of issue numbers (for issue sessions)
#   AGENTIUM_PRS             - Comma-separated list of PR numbers (for PR review sessions)
#   GITHUB_APP_ID            - GitHub App ID for authentication
#   GITHUB_INSTALLATION_ID   - GitHub App installation ID
#   GITHUB_PRIVATE_KEY_SECRET - GCP Secret Manager path for private key
#
# Note: At least one of AGENTIUM_ISSUES or AGENTIUM_PRS must be provided.
#
# Optional environment variables:
#   AGENTIUM_MAX_ITERATIONS  - Maximum iterations (default: 5)
#   AGENTIUM_BRANCH          - Base branch to work from (default: main)
#   ANTHROPIC_API_KEY        - API key for Claude (or fetched from secrets)

set -euo pipefail

# Cleanup sensitive files on exit
cleanup() {
    rm -f /tmp/SYSTEM.md /tmp/git-askpass.sh 2>/dev/null || true
}
trap cleanup EXIT

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

# Fetch PR details from GitHub
fetch_pr_details() {
    local repo="$1"
    local pr_number="$2"

    gh pr view "${pr_number}" --repo "${repo}" \
        --json number,title,body,headRefName
}

# Fetch PR review comments
fetch_pr_reviews() {
    local repo="$1"
    local pr_number="$2"

    gh api "repos/${repo}/pulls/${pr_number}/reviews" \
        --jq '[.[] | select(.state == "CHANGES_REQUESTED" or .state == "COMMENTED") | {state: .state, body: .body}]' 2>/dev/null || echo "[]"
}

# Fetch PR inline comments
fetch_pr_comments() {
    local repo="$1"
    local pr_number="$2"

    gh api "repos/${repo}/pulls/${pr_number}/comments" \
        --jq '[.[] | {path: .path, line: .line, body: .body}]' 2>/dev/null || echo "[]"
}

# Build the prompt for PR review sessions
build_pr_prompt() {
    local repo="$1"
    local prs="$2"

    local prompt="You are working on repository: ${repo}\n\n"
    prompt+="## PR REVIEW SESSION\n\n"
    prompt+="You are addressing code review feedback on existing pull request(s).\n\n"

    IFS=',' read -ra PR_ARRAY <<< "${prs}"
    for pr_num in "${PR_ARRAY[@]}"; do
        pr_num=$(echo "${pr_num}" | tr -d ' ')
        local pr_json=$(fetch_pr_details "${repo}" "${pr_num}" 2>/dev/null || echo '{}')

        if [[ "${pr_json}" != "{}" ]]; then
            local title=$(echo "${pr_json}" | jq -r '.title // "Unknown"')
            local branch=$(echo "${pr_json}" | jq -r '.headRefName // "unknown"')

            prompt+="### PR #${pr_num}: ${title}\n"
            prompt+="Branch: ${branch}\n\n"

            # Fetch and add review comments
            local reviews=$(fetch_pr_reviews "${repo}" "${pr_num}")
            if [[ "${reviews}" != "[]" ]] && [[ -n "${reviews}" ]]; then
                prompt+="**Review Feedback:**\n"
                prompt+=$(echo "${reviews}" | jq -r '.[] | "- [\(.state)] \(.body)"' | head -c 1000)
                prompt+="\n\n"
            fi

            # Fetch and add inline comments
            local comments=$(fetch_pr_comments "${repo}" "${pr_num}")
            if [[ "${comments}" != "[]" ]] && [[ -n "${comments}" ]]; then
                prompt+="**Inline Comments:**\n"
                prompt+=$(echo "${comments}" | jq -r '.[] | "- File: \(.path) (line \(.line))\n  Comment: \(.body)"' | head -c 1500)
                prompt+="\n\n"
            fi
        else
            prompt+="### PR #${pr_num}\n\n"
        fi
    done

    prompt+="## Instructions\n\n"
    prompt+="1. You are ALREADY on the PR branch - do NOT create a new branch\n"
    prompt+="2. Read and understand the review comments\n"
    prompt+="3. Make targeted changes to address the feedback\n"
    prompt+="4. Run tests to verify your changes\n"
    prompt+="5. Commit with message: \"Address review feedback\"\n"
    prompt+="6. Push your changes: \`git push origin HEAD\`\n\n"
    prompt+="## DO NOT\n\n"
    prompt+="- Create a new branch (you're already on the PR branch)\n"
    prompt+="- Close or merge the PR\n"
    prompt+="- Dismiss reviews\n"
    prompt+="- Force push (unless absolutely necessary)\n"
    prompt+="- Make unrelated changes\n"

    echo -e "${prompt}"
}

# Checkout PR branch
checkout_pr_branch() {
    local repo="$1"
    local pr_number="$2"

    local pr_json=$(fetch_pr_details "${repo}" "${pr_number}" 2>/dev/null || echo '{}')
    if [[ "${pr_json}" == "{}" ]]; then
        log_error "Failed to fetch PR #${pr_number} details"
        return 1
    fi

    local branch=$(echo "${pr_json}" | jq -r '.headRefName')
    if [[ -z "${branch}" ]] || [[ "${branch}" == "null" ]]; then
        log_error "Failed to get branch name for PR #${pr_number}"
        return 1
    fi

    log_info "Checking out PR branch: ${branch}"
    git fetch origin "${branch}" 2>/dev/null || true
    git checkout "${branch}"
}

# Detect if changes were pushed (for PR completion detection)
detect_push_completion() {
    local output="$1"
    # Look for git push success pattern
    if echo "${output}" | grep -qE 'To (github\.com|git@github\.com).*\n.*[a-f0-9]+\.\.[a-f0-9]+'; then
        return 0
    fi
    return 1
}

# Parse AGENTIUM_STATUS signals from output
# Returns the last status signal found (most recent)
parse_agent_status() {
    local output="$1"
    echo "${output}" | grep -oE 'AGENTIUM_STATUS:\s*\w+' | tail -1 | sed 's/AGENTIUM_STATUS:\s*//'
}

# Check if status signal indicates completion
is_completion_status() {
    local status="$1"
    case "${status}" in
        "COMPLETE"|"PUSHED"|"PR_CREATED"|"NOTHING_TO_DO")
            return 0
            ;;
        *)
            return 1
            ;;
    esac
}

# Check if status signal indicates blocked/failed state
is_terminal_failure_status() {
    local status="$1"
    case "${status}" in
        "BLOCKED"|"FAILED")
            return 0
            ;;
        *)
            return 1
            ;;
    esac
}

# Fetch SYSTEM.md from agentium repository
fetch_system_md() {
    local output_path="/tmp/SYSTEM.md"

    log_info "Fetching SYSTEM.md from agentium repository"

    if curl -sfL "${SYSTEM_MD_URL}" -o "${output_path}"; then
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
    # Note: --system-prompt expects content, not a file path
    local claude_args=(
        "--print"
        "--dangerously-skip-permissions"
        "--system-prompt" "$(cat "${system_md}")"
    )

    # Check for project-specific instructions
    if [[ -f "${WORKSPACE}/.agentium/AGENT.md" ]]; then
        log_info "Found project-specific instructions: .agentium/AGENT.md"
        claude_args+=("--append-system-prompt" "$(cat "${WORKSPACE}/.agentium/AGENT.md")")
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
    if [[ -n "${AGENTIUM_ISSUES:-}" ]]; then
        log_info "Issues: ${AGENTIUM_ISSUES}"
    fi
    if [[ -n "${AGENTIUM_PRS:-}" ]]; then
        log_info "PRs: ${AGENTIUM_PRS}"
    fi

    # Validate required environment variables
    : "${AGENTIUM_SESSION_ID:?AGENTIUM_SESSION_ID is required}"
    : "${AGENTIUM_REPOSITORY:?AGENTIUM_REPOSITORY is required}"

    # Require at least issues or PRs
    if [[ -z "${AGENTIUM_ISSUES:-}" ]] && [[ -z "${AGENTIUM_PRS:-}" ]]; then
        log_error "At least one of AGENTIUM_ISSUES or AGENTIUM_PRS is required"
        exit 1
    fi

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

    # Fetch SYSTEM.md
    local system_md=$(fetch_system_md)
    if [[ -z "${system_md}" ]]; then
        log_error "Failed to fetch SYSTEM.md"
        exit 1
    fi

    local max_iterations="${AGENTIUM_MAX_ITERATIONS:-5}"

    # Clone repository first (to main branch)
    clone_repository "${AGENTIUM_REPOSITORY}" "${github_token}" "${AGENTIUM_BRANCH:-main}"

    # Build processing queue: PRs first, then issues
    local WORK_QUEUE=()

    if [[ -n "${AGENTIUM_PRS:-}" ]]; then
        IFS=',' read -ra PR_ARRAY <<< "${AGENTIUM_PRS}"
        for pr_num in "${PR_ARRAY[@]}"; do
            WORK_QUEUE+=("pr:$(echo "${pr_num}" | tr -d ' ')")
        done
    fi

    if [[ -n "${AGENTIUM_ISSUES:-}" ]]; then
        IFS=',' read -ra ISSUE_ARRAY <<< "${AGENTIUM_ISSUES}"
        for issue_num in "${ISSUE_ARRAY[@]}"; do
            WORK_QUEUE+=("issue:$(echo "${issue_num}" | tr -d ' ')")
        done
    fi

    log_info "Work queue: ${WORK_QUEUE[*]}"

    # Process each item in queue
    for queue_item in "${WORK_QUEUE[@]}"; do
        IFS=':' read -r item_type item_num <<< "${queue_item}"

        log_info "=== Processing ${item_type} #${item_num} ==="

        local prompt=""
        if [[ "${item_type}" == "pr" ]]; then
            checkout_pr_branch "${AGENTIUM_REPOSITORY}" "${item_num}"
            prompt=$(build_pr_prompt "${AGENTIUM_REPOSITORY}" "${item_num}")
        else
            # Return to main branch for issues
            cd "${WORKSPACE}"
            git checkout "${AGENTIUM_BRANCH:-main}"
            git pull origin "${AGENTIUM_BRANCH:-main}" || true
            prompt=$(build_prompt "${AGENTIUM_REPOSITORY}" "${item_num}")
        fi

        # Inner iteration loop for this item
        local item_iteration=1
        while [[ ${item_iteration} -le ${max_iterations} ]]; do
            log_info "=== ${item_type} #${item_num} - Iteration ${item_iteration}/${max_iterations} ==="

            local output_file=$(mktemp)
            if run_claude "${prompt}" "${system_md}" "${item_iteration}" 2>&1 | tee "${output_file}"; then
                log_info "Iteration ${item_iteration} completed successfully"
            else
                log_error "Iteration ${item_iteration} failed"
            fi

            local output=$(cat "${output_file}")
            rm -f "${output_file}"

            # Check agent status signals first (preferred method)
            local agent_status=$(parse_agent_status "${output}")

            if [[ -n "${agent_status}" ]]; then
                log_info "${item_type} #${item_num} - agent status: ${agent_status}"

                # Check for completion signals
                if is_completion_status "${agent_status}"; then
                    log_info "${item_type} #${item_num} complete - agent signaled: ${agent_status}"
                    break
                fi

                # Check for blocked/failed signals
                if is_terminal_failure_status "${agent_status}"; then
                    log_error "${item_type} #${item_num} - agent signaled: ${agent_status}"
                    break
                fi
            fi

            # Fall back to existing detection methods if no status signal
            if [[ "${item_type}" == "pr" ]]; then
                # For PR items: detect if changes were pushed
                if echo "${output}" | grep -qE 'To (github\.com|git@github\.com)'; then
                    log_info "PR #${item_num} complete - changes pushed (detected from git output)"
                    break
                fi
            else
                # For issue items: check if PR was created
                local pr_count=$(gh pr list --repo "${AGENTIUM_REPOSITORY}" --state open \
                    --search "Closes #${item_num} in:body" --json number 2>/dev/null | jq '. | length')
                if [[ ${pr_count} -gt 0 ]]; then
                    log_info "Issue #${item_num} complete - PR created (detected from GitHub)"
                    break
                fi
            fi

            ((item_iteration++))
        done
    done

    log_info "Session complete"
}

# Run main function
main "$@"
