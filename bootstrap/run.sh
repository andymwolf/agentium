#!/bin/bash
# Agentium Bootstrap Launch Script
#
# This script runs locally to launch an Agentium session on GCP.
# It handles Terraform setup, VM creation, and optional log tailing.
#
# Prerequisites:
#   - GCP project with Compute Engine and Secret Manager APIs enabled
#   - GitHub App private key stored in Secret Manager
#   - gcloud CLI authenticated
#   - Terraform installed
#
# Usage:
#   ./run.sh --repo owner/repo --issue 42
#   ./run.sh --repo owner/repo --issue 42 --follow
#   ./run.sh --destroy

set -euo pipefail

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Default values
PROJECT_ID=""
REGION="us-central1"
ZONE="us-central1-a"
GITHUB_APP_ID=""
GITHUB_INSTALLATION_ID=""
GITHUB_PRIVATE_KEY_SECRET=""
ANTHROPIC_API_KEY_SECRET=""
REPOSITORY=""
ISSUES=""
FOLLOW_LOGS=false
DESTROY_MODE=false
AUTO_APPROVE=false

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Print functions
print_info() {
    echo -e "${BLUE}[INFO]${NC} $*"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $*"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $*"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $*" >&2
}

# Show usage
usage() {
    cat << EOF
Usage: $(basename "$0") [OPTIONS]

Launch an Agentium session on GCP to work on GitHub issues.

Required options:
  --repo OWNER/REPO       Target GitHub repository
  --issue NUMBER          Issue number(s) to work on (comma-separated)

GCP options:
  --project PROJECT_ID    GCP project ID (default: from gcloud config)
  --region REGION         GCP region (default: us-central1)
  --zone ZONE             GCP zone (default: us-central1-a)

GitHub App options:
  --app-id ID             GitHub App ID
  --installation-id ID    GitHub App installation ID
  --private-key-secret NAME  Secret Manager secret name for private key

Other options:
  --anthropic-secret NAME Secret Manager secret name for Anthropic API key
  --follow, -f            Follow session logs after VM starts
  --auto-approve, -y      Skip confirmation prompts
  --destroy               Destroy the current session
  --help, -h              Show this help message

Examples:
  # Launch a session
  $(basename "$0") --repo andymwolf/agentium --issue 42

  # Launch and follow logs
  $(basename "$0") --repo andymwolf/agentium --issue 42 --follow

  # Work on multiple issues
  $(basename "$0") --repo andymwolf/agentium --issue 42,43,44

  # Destroy the session
  $(basename "$0") --destroy

Environment variables:
  AGENTIUM_PROJECT_ID           GCP project ID
  AGENTIUM_GITHUB_APP_ID        GitHub App ID
  AGENTIUM_GITHUB_INSTALLATION_ID  GitHub App installation ID
  AGENTIUM_GITHUB_PRIVATE_KEY_SECRET  Secret Manager secret name

EOF
    exit 0
}

# Parse command line arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            --repo)
                REPOSITORY="$2"
                shift 2
                ;;
            --issue)
                ISSUES="$2"
                shift 2
                ;;
            --project)
                PROJECT_ID="$2"
                shift 2
                ;;
            --region)
                REGION="$2"
                shift 2
                ;;
            --zone)
                ZONE="$2"
                shift 2
                ;;
            --app-id)
                GITHUB_APP_ID="$2"
                shift 2
                ;;
            --installation-id)
                GITHUB_INSTALLATION_ID="$2"
                shift 2
                ;;
            --private-key-secret)
                GITHUB_PRIVATE_KEY_SECRET="$2"
                shift 2
                ;;
            --anthropic-secret)
                ANTHROPIC_API_KEY_SECRET="$2"
                shift 2
                ;;
            --follow|-f)
                FOLLOW_LOGS=true
                shift
                ;;
            --auto-approve|-y)
                AUTO_APPROVE=true
                shift
                ;;
            --destroy)
                DESTROY_MODE=true
                shift
                ;;
            --help|-h)
                usage
                ;;
            *)
                print_error "Unknown option: $1"
                usage
                ;;
        esac
    done
}

# Load from environment variables
load_env() {
    PROJECT_ID="${PROJECT_ID:-${AGENTIUM_PROJECT_ID:-}}"
    GITHUB_APP_ID="${GITHUB_APP_ID:-${AGENTIUM_GITHUB_APP_ID:-}}"
    GITHUB_INSTALLATION_ID="${GITHUB_INSTALLATION_ID:-${AGENTIUM_GITHUB_INSTALLATION_ID:-}}"
    GITHUB_PRIVATE_KEY_SECRET="${GITHUB_PRIVATE_KEY_SECRET:-${AGENTIUM_GITHUB_PRIVATE_KEY_SECRET:-}}"
    ANTHROPIC_API_KEY_SECRET="${ANTHROPIC_API_KEY_SECRET:-${AGENTIUM_ANTHROPIC_API_KEY_SECRET:-}}"
}

# Validate prerequisites
validate_prerequisites() {
    print_info "Checking prerequisites..."

    # Check gcloud
    if ! command -v gcloud &> /dev/null; then
        print_error "gcloud CLI is not installed"
        exit 1
    fi

    # Check terraform
    if ! command -v terraform &> /dev/null; then
        print_error "Terraform is not installed"
        exit 1
    fi

    # Check gcloud auth
    if ! gcloud auth list --filter=status:ACTIVE --format="value(account)" &> /dev/null; then
        print_error "gcloud is not authenticated. Run 'gcloud auth login'"
        exit 1
    fi

    # Get project ID from gcloud if not set
    if [[ -z "$PROJECT_ID" ]]; then
        PROJECT_ID=$(gcloud config get-value project 2>/dev/null || true)
        if [[ -z "$PROJECT_ID" ]]; then
            print_error "No GCP project configured. Use --project or run 'gcloud config set project PROJECT_ID'"
            exit 1
        fi
    fi

    print_success "Prerequisites check passed"
}

# Validate required options
validate_options() {
    if [[ "$DESTROY_MODE" == true ]]; then
        return 0
    fi

    local missing=()

    if [[ -z "$REPOSITORY" ]]; then
        missing+=("--repo")
    fi

    if [[ -z "$ISSUES" ]]; then
        missing+=("--issue")
    fi

    if [[ -z "$GITHUB_APP_ID" ]]; then
        missing+=("--app-id or AGENTIUM_GITHUB_APP_ID")
    fi

    if [[ -z "$GITHUB_INSTALLATION_ID" ]]; then
        missing+=("--installation-id or AGENTIUM_GITHUB_INSTALLATION_ID")
    fi

    if [[ -z "$GITHUB_PRIVATE_KEY_SECRET" ]]; then
        missing+=("--private-key-secret or AGENTIUM_GITHUB_PRIVATE_KEY_SECRET")
    fi

    if [[ ${#missing[@]} -gt 0 ]]; then
        print_error "Missing required options:"
        for opt in "${missing[@]}"; do
            echo "  - $opt"
        done
        echo ""
        echo "Run '$(basename "$0") --help' for usage information"
        exit 1
    fi
}

# Initialize Terraform
init_terraform() {
    print_info "Initializing Terraform..."

    cd "$SCRIPT_DIR"

    # Check if terraform files exist
    if [[ ! -f "main.tf" ]]; then
        print_error "Terraform files not found in $SCRIPT_DIR"
        print_error "Make sure main.tf and variables.tf exist"
        exit 1
    fi

    terraform init -upgrade

    print_success "Terraform initialized"
}

# Run Terraform apply
apply_terraform() {
    print_info "Creating Agentium session..."
    print_info "  Repository: $REPOSITORY"
    print_info "  Issues: $ISSUES"
    print_info "  Project: $PROJECT_ID"
    print_info "  Zone: $ZONE"

    cd "$SCRIPT_DIR"

    local tf_args=(
        -var="project_id=$PROJECT_ID"
        -var="region=$REGION"
        -var="zone=$ZONE"
        -var="repository=$REPOSITORY"
        -var="issues=$ISSUES"
        -var="github_app_id=$GITHUB_APP_ID"
        -var="github_installation_id=$GITHUB_INSTALLATION_ID"
        -var="github_private_key_secret=$GITHUB_PRIVATE_KEY_SECRET"
    )

    if [[ -n "$ANTHROPIC_API_KEY_SECRET" ]]; then
        tf_args+=(-var="anthropic_api_key_secret=$ANTHROPIC_API_KEY_SECRET")
    fi

    if [[ "$AUTO_APPROVE" == true ]]; then
        tf_args+=("-auto-approve")
    fi

    terraform apply "${tf_args[@]}"

    print_success "Session created!"

    # Get outputs
    echo ""
    print_info "Session details:"
    terraform output -json | jq -r '
        "  Session ID: \(.session_id.value)\n" +
        "  Instance: \(.instance_name.value)\n" +
        "  IP Address: \(.instance_ip.value)\n" +
        "  Zone: \(.instance_zone.value)"
    '

    echo ""
    print_info "Useful commands:"
    terraform output -raw ssh_command
    echo ""
    terraform output -raw logs_command
    echo ""
    terraform output -raw destroy_command
    echo ""
}

# Destroy Terraform resources
destroy_terraform() {
    print_warning "Destroying Agentium session..."

    cd "$SCRIPT_DIR"

    local tf_args=()

    if [[ "$AUTO_APPROVE" == true ]]; then
        tf_args+=("-auto-approve")
    fi

    terraform destroy "${tf_args[@]}"

    print_success "Session destroyed"
}

# Follow logs
follow_logs() {
    print_info "Waiting for VM to be ready..."

    cd "$SCRIPT_DIR"

    local instance_name=$(terraform output -raw instance_name 2>/dev/null || true)
    local zone=$(terraform output -raw instance_zone 2>/dev/null || true)

    if [[ -z "$instance_name" ]] || [[ -z "$zone" ]]; then
        print_error "Could not get instance details from Terraform state"
        exit 1
    fi

    # Wait for SSH to be available
    local max_attempts=30
    local attempt=1

    while [[ $attempt -le $max_attempts ]]; do
        if gcloud compute ssh "$instance_name" --zone="$zone" --command="echo ready" &> /dev/null; then
            break
        fi
        print_info "Waiting for VM... (attempt $attempt/$max_attempts)"
        sleep 10
        ((attempt++))
    done

    if [[ $attempt -gt $max_attempts ]]; then
        print_error "VM did not become ready in time"
        exit 1
    fi

    print_success "VM is ready, tailing logs..."
    echo ""

    gcloud compute ssh "$instance_name" --zone="$zone" \
        --command="tail -f /var/log/agentium-session.log 2>/dev/null || tail -f /var/log/cloud-init-output.log"
}

# Main function
main() {
    parse_args "$@"
    load_env

    if [[ "$DESTROY_MODE" == true ]]; then
        validate_prerequisites
        init_terraform
        destroy_terraform
        exit 0
    fi

    validate_prerequisites
    validate_options
    init_terraform
    apply_terraform

    if [[ "$FOLLOW_LOGS" == true ]]; then
        follow_logs
    fi
}

# Run main
main "$@"
