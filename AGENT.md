# Agentium Project Agent Instructions

This file provides project-specific context for agents working on the Agentium codebase.

## Project Overview

Agentium is an ephemeral cloud execution framework for running AI coding agents safely. Agents run in containerized environments on session-scoped VMs that self-terminate after completing tasks.

## Project Structure

```
agentium/
├── cmd/
│   ├── agentium/       # CLI entry point (main user interface)
│   └── controller/     # Session controller entry point (runs on VM)
├── internal/
│   ├── cli/            # CLI command implementations
│   ├── config/         # Configuration loading and validation
│   ├── controller/     # Session lifecycle management
│   ├── agent/          # Agent adapters (claudecode, aider)
│   ├── provisioner/    # Cloud VM provisioning
│   └── cloud/          # Cloud provider clients (aws, gcp, azure)
├── terraform/modules/  # Terraform modules for VM/IAM/networking
└── docker/             # Agent runtime container Dockerfiles
```

## Core Concepts

- **Session**: A single VM lifecycle executing one or more tasks
- **Iteration**: One agent execution cycle within a session
- **Agent Adapter**: Pluggable interface for different AI agents (Claude Code, Aider)
- **Provisioner**: Cloud-specific VM lifecycle management
- **Session Controller**: Runs on the VM, orchestrates agent containers, enforces termination

## Key Design Constraints

1. **Container-first**: Agent logic always runs in containers, never directly on the VM
2. **Ephemeral VMs**: VMs are session-scoped and self-destruct on completion
3. **PR-driven changes**: Agents create PRs, never deploy directly
4. **Least privilege**: Agent VMs have only GitHub read/write access
5. **Prompt-defined lifecycle**: Iteration limits, time budgets, termination rules come from the prompt

## Build & Test Commands

```bash
# Build all binaries
go build ./...

# Run all tests
go test ./...

# Build specific binaries
go build -o agentium ./cmd/agentium
go build -o controller ./cmd/controller
```

## Current Implementation Status

- **GCP**: Fully functional (provisioner, terraform)
- **AWS/Azure**: Planned, not yet implemented
- **GitHub App auth**: Implemented in Go controller
- **Cloud Logging**: Not yet implemented

## Architecture Reference

For detailed requirements, design rationale, and functional specifications, see `agentium_prd.md` in the repository root.

## Off-Limits Areas

- Do not modify GitHub Actions workflows without explicit approval
- Do not add cloud provider dependencies without considering multi-cloud support
- Do not store secrets or credentials in code
