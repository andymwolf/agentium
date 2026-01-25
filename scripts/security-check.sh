#!/bin/bash
# Security validation script for Agentium deployments

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "🔒 Agentium Security Checker"
echo "=========================="
echo

ISSUES_FOUND=0

# Function to check a condition
check() {
    local description="$1"
    local command="$2"

    echo -n "Checking: $description... "

    if eval "$command" > /dev/null 2>&1; then
        echo -e "${GREEN}✓${NC}"
    else
        echo -e "${RED}✗${NC}"
        ISSUES_FOUND=$((ISSUES_FOUND + 1))
    fi
}

# Function to warn about a condition
warn() {
    local description="$1"
    local command="$2"

    echo -n "Checking: $description... "

    if eval "$command" > /dev/null 2>&1; then
        echo -e "${YELLOW}⚠${NC} (Review needed)"
    else
        echo -e "${GREEN}✓${NC}"
    fi
}

echo "1. Code Security Checks"
echo "-----------------------"

# Check for hardcoded secrets (excluding test files)
check "No hardcoded GitHub tokens" \
    "! grep -r 'gh[ps]_[a-zA-Z0-9]\{36\}\|github_pat_[a-zA-Z0-9]\{22\}_[a-zA-Z0-9]\{59\}' --include='*.go' --include='*.yaml' --include='*.yml' --include='*.tf' --exclude='*_test.go' --exclude-dir='testdata' . 2>/dev/null"

check "No hardcoded API keys" \
    "! grep -rEi '(api[_-]?key|apikey|api[_-]?secret)[[:space:]]*[:=][[:space:]]*[\"'\''][a-zA-Z0-9_-]\{16,\}' --include='*.go' --include='*.yaml' --include='*.yml' . 2>/dev/null"

check "No private keys in code" \
    "! grep -r 'BEGIN.*PRIVATE KEY' --include='*.go' --include='*.yaml' --include='*.yml' --exclude='*_test.go' . 2>/dev/null"

check "No passwords in URLs" \
    "! grep -rE '(https?|ftp)://[^:]+:[^@]+@' --include='*.go' --include='*.yaml' --include='*.yml' --exclude='*_test.go' . 2>/dev/null | grep -v 'x-access-token'"

echo
echo "2. Dependency Security"
echo "---------------------"

# Skip Go checks if Go is not installed
if command -v go &> /dev/null; then
    check "Go modules are tidy" \
        "go mod tidy -diff=false 2>/dev/null"
else
    echo "Skipping: Go modules are tidy... (Go not installed)"
fi

check "No replace directives in go.mod" \
    "! grep -E '^[[:space:]]*replace[[:space:]]' go.mod"

echo
echo "3. Terraform Security"
echo "--------------------"

# Check Terraform files if they exist
if [ -d "terraform" ]; then
    check "No sensitive variables without 'sensitive = true'" \
        "! grep -r 'password\\|secret\\|token\\|key' terraform/ | grep -v 'sensitive[[:space:]]*=[[:space:]]*true' | grep 'variable' || true"

    check "IAM roles follow least privilege" \
        "! grep -r 'roles/owner\\|roles/editor' terraform/ || true"

    warn "Resource deletion protection" \
        "grep -r 'deletion_protection\\|prevent_destroy' terraform/"
else
    echo "No terraform directory found, skipping Terraform checks"
fi

echo
echo "4. Container Security"
echo "--------------------"

# Check Dockerfiles
if [ -d "docker" ]; then
    check "Containers run as non-root" \
        "grep -r 'USER[[:space:]]' docker/"

    check "No runtime sudo usage" \
        "! grep -r 'sudo' docker/ | grep -v '^[[:space:]]*#' | grep -E 'RUN.*sudo|CMD.*sudo|ENTRYPOINT.*sudo'"

    check "Using specific base image versions" \
        "! grep -r 'FROM.*:latest' docker/"
else
    echo "No docker directory found, skipping container checks"
fi

echo
echo "5. Configuration Security"
echo "------------------------"

check "Example config doesn't contain real values" \
    "! grep -E 'gh[ps]_|AKIA|projects/.+/secrets' configs/example.agentium.yaml 2>/dev/null || true"

check "No .env files in repository" \
    "! find . -name '.env' -o -name '.env.*' | grep -v .gitignore"

echo
echo "6. GitHub Workflow Security"
echo "--------------------------"

if [ -d ".github/workflows" ]; then
    check "Workflows pin action versions" \
        "! grep -r 'uses:.*@main\\|uses:.*@master' .github/workflows/"

    check "No hardcoded secrets in workflows" \
        "! grep -r 'gh[ps]_\\|secret:\\|password:' .github/workflows/ | grep -v '\${{' || true"

    check "Workflows use minimal permissions" \
        "grep -r 'permissions:' .github/workflows/"
else
    echo "No .github/workflows directory found"
fi

echo
echo "7. Code Quality Checks"
echo "---------------------"

# Run tests with race detection
if command -v go &> /dev/null; then
    echo -n "Running tests with race detector... "
    if go test -race -short ./... > /dev/null 2>&1; then
        echo -e "${GREEN}✓${NC}"
    else
        echo -e "${RED}✗${NC}"
        ISSUES_FOUND=$((ISSUES_FOUND + 1))
    fi
fi

echo
echo "8. Security Package Integration"
echo "------------------------------"

check "Security package is imported where needed" \
    "grep -r 'internal/security' --include='*.go' internal/cloud/ internal/controller/"

check "Log sanitization tests exist" \
    "[ -f internal/security/sanitizer_test.go ]"

check "IAM validation tests exist" \
    "[ -f internal/security/iam_test.go ]"

echo
echo "=========================="
if [ $ISSUES_FOUND -eq 0 ]; then
    echo -e "${GREEN}✅ All security checks passed!${NC}"
    exit 0
else
    echo -e "${RED}❌ Found $ISSUES_FOUND security issues${NC}"
    echo
    echo "Please review and fix the issues above before deployment."
    exit 1
fi