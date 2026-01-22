# Agentium Controller Dockerfile
# Multi-stage build for minimal image size

# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /build

# Install dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files first for caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the controller binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /agentium-controller ./cmd/controller

# Runtime stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    docker-cli \
    git \
    curl \
    jq \
    bash \
    python3

# Install gcloud CLI (static tarball for Alpine compatibility)
RUN curl -sSL https://dl.google.com/dl/cloudsdk/channels/rapid/google-cloud-sdk.tar.gz | \
    tar xz -C /opt && \
    /opt/google-cloud-sdk/install.sh --quiet --path-update=false
ENV PATH="/opt/google-cloud-sdk/bin:${PATH}"

# Install GitHub CLI
RUN curl -sSL https://github.com/cli/cli/releases/download/v2.40.0/gh_2.40.0_linux_amd64.tar.gz | \
    tar xz -C /tmp && \
    mv /tmp/gh_2.40.0_linux_amd64/bin/gh /usr/local/bin/ && \
    rm -rf /tmp/gh_*

# Create non-root user
RUN adduser -D -u 1000 agentium

# Copy binary from builder
COPY --from=builder /agentium-controller /usr/local/bin/agentium-controller

# Create workspace directory
RUN mkdir -p /workspace /etc/agentium && chown agentium:agentium /workspace

# Set working directory
WORKDIR /workspace

# Run as non-root user (but may need root for docker operations)
# USER agentium

# Default command
ENTRYPOINT ["/usr/local/bin/agentium-controller"]
