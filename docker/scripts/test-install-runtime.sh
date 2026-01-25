#!/bin/bash
# Test script to verify runtime installation works correctly

set -e

# Function to log messages
log() {
    echo "[test-install] $1" >&2
}

# Test Go installation
test_go_install() {
    log "Testing Go installation..."

    # Create temporary workspace
    export TEST_WORKSPACE=$(mktemp -d)
    mkdir -p "$TEST_WORKSPACE/workspace"
    cd "$TEST_WORKSPACE/workspace"

    # Create go.mod
    cat > go.mod << 'EOF'
module testproject

go 1.21
EOF

    # Run install script
    log "Running install script for Go project..."
    bash /runtime-scripts/install-runtime.sh

    # Test Go installation
    if command -v go &> /dev/null; then
        log "✓ Go installed successfully"
        go version
    else
        log "✗ Go installation failed"
        return 1
    fi

    # Cleanup
    cd /
    rm -rf "$TEST_WORKSPACE"
}

# Test Rust installation
test_rust_install() {
    log "Testing Rust installation..."

    # Create temporary workspace
    export TEST_WORKSPACE=$(mktemp -d)
    mkdir -p "$TEST_WORKSPACE/workspace"
    cd "$TEST_WORKSPACE/workspace"

    # Create Cargo.toml
    cat > Cargo.toml << 'EOF'
[package]
name = "testproject"
version = "0.1.0"
edition = "2021"
EOF

    # Run install script
    log "Running install script for Rust project..."
    bash /runtime-scripts/install-runtime.sh

    # Source cargo env
    if [ -f "$HOME/.cargo/env" ]; then
        source "$HOME/.cargo/env"
    fi

    # Test Rust installation
    if command -v cargo &> /dev/null; then
        log "✓ Rust installed successfully"
        cargo --version
    else
        log "✗ Rust installation failed"
        return 1
    fi

    # Cleanup
    cd /
    rm -rf "$TEST_WORKSPACE"
}

# Test Java installation
test_java_install() {
    log "Testing Java installation..."

    # Create temporary workspace
    export TEST_WORKSPACE=$(mktemp -d)
    mkdir -p "$TEST_WORKSPACE/workspace"
    cd "$TEST_WORKSPACE/workspace"

    # Create pom.xml
    cat > pom.xml << 'EOF'
<project>
    <modelVersion>4.0.0</modelVersion>
    <groupId>com.test</groupId>
    <artifactId>testproject</artifactId>
    <version>1.0</version>
</project>
EOF

    # Run install script
    log "Running install script for Java project..."
    bash /runtime-scripts/install-runtime.sh

    # Test Java installation
    if command -v java &> /dev/null; then
        log "✓ Java installed successfully"
        java --version
    else
        log "✗ Java installation failed"
        return 1
    fi

    # Cleanup
    cd /
    rm -rf "$TEST_WORKSPACE"
}

# Main test function
main() {
    log "Starting runtime installation tests..."

    # Test only if running in Docker container with script available
    if [ -f "/runtime-scripts/install-runtime.sh" ]; then
        test_go_install
        test_rust_install
        test_java_install
        log "All tests completed!"
    else
        log "Not running in agent container - skipping tests"
    fi
}

# Run if executed directly
if [ "${BASH_SOURCE[0]}" = "${0}" ]; then
    main "$@"
fi