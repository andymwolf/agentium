#!/bin/bash
# Language runtime detection and installation script for agent containers

set -e

# Function to log messages
log() {
    echo "[runtime-installer] $1" >&2
}

# Function to check if a file exists in the workspace
file_exists() {
    [ -f "/workspace/$1" ]
}

# Function to install Go
install_go() {
    log "Detected Go project (go.mod found)"
    log "Installing Go runtime..."

    # Download and install Go
    GO_VERSION="1.22.0"
    curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -o /tmp/go.tar.gz
    sudo tar -C /usr/local -xzf /tmp/go.tar.gz
    rm /tmp/go.tar.gz

    # Add Go to PATH
    echo 'export PATH=/usr/local/go/bin:$PATH' >> ~/.bashrc
    export PATH=/usr/local/go/bin:$PATH

    log "Go ${GO_VERSION} installed successfully"
}

# Function to install Rust
install_rust() {
    log "Detected Rust project (Cargo.toml found)"
    log "Installing Rust runtime..."

    # Install Rust via rustup
    curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y --default-toolchain stable

    # Source cargo env
    source "$HOME/.cargo/env"

    log "Rust installed successfully"
}

# Function to install Java
install_java() {
    log "Detected Java project (pom.xml or build.gradle found)"
    log "Installing Java runtime..."

    # Install OpenJDK 17
    sudo apt-get update
    sudo apt-get install -y openjdk-17-jdk

    # Set JAVA_HOME
    echo 'export JAVA_HOME=/usr/lib/jvm/java-17-openjdk-amd64' >> ~/.bashrc
    export JAVA_HOME=/usr/lib/jvm/java-17-openjdk-amd64

    log "OpenJDK 17 installed successfully"
}

# Function to install Ruby
install_ruby() {
    log "Detected Ruby project (Gemfile found)"
    log "Installing Ruby runtime..."

    # Install Ruby via apt (simpler than rbenv/rvm for CI environments)
    sudo apt-get update
    sudo apt-get install -y ruby-full ruby-bundler

    log "Ruby installed successfully"
}

# Function to install .NET
install_dotnet() {
    log "Detected .NET project (*.csproj or *.sln found)"
    log "Installing .NET runtime..."

    # Install .NET SDK
    curl -fsSL -o packages-microsoft-prod.deb https://packages.microsoft.com/config/ubuntu/20.04/packages-microsoft-prod.deb
    sudo dpkg -i packages-microsoft-prod.deb
    rm packages-microsoft-prod.deb

    sudo apt-get update
    sudo apt-get install -y dotnet-sdk-8.0

    log ".NET SDK 8.0 installed successfully"
}

# Main detection and installation logic
main() {
    log "Checking project type in /workspace..."

    # Go detection
    if file_exists "go.mod"; then
        install_go
    # Rust detection
    elif file_exists "Cargo.toml"; then
        install_rust
    # Java detection
    elif file_exists "pom.xml" || file_exists "build.gradle" || file_exists "build.gradle.kts"; then
        install_java
    # Ruby detection
    elif file_exists "Gemfile"; then
        install_ruby
    # .NET detection
    elif ls /workspace/*.csproj >/dev/null 2>&1 || ls /workspace/*.sln >/dev/null 2>&1; then
        install_dotnet
    # Node.js and Python are already installed in base image
    elif file_exists "package.json"; then
        log "Detected Node.js project (package.json found) - runtime already available"
    elif file_exists "requirements.txt" || file_exists "pyproject.toml" || file_exists "setup.py"; then
        log "Detected Python project - runtime already available"
    else
        log "No specific language detected - using base runtime environment"
    fi

    log "Runtime detection and installation complete"
}

# Run main function
main