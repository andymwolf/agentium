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

## Workflow Requirements

### Critical Rules

**NEVER make code changes directly on the `main` branch.** All work must follow the branch workflow below.

**ALWAYS create a feature branch BEFORE writing any code.** If you find yourself on `main`, switch to a feature branch immediately.

**When given a plan or implementation instructions:**
1. First update the relevant GitHub issue(s) with the plan details (use `gh issue edit`)
2. Create a feature branch
3. Then implement the changes

### Branch and PR Workflow

When implementing any GitHub issue:

1. **Update the GitHub issue** with implementation details if needed:
   ```bash
   gh issue edit <number> --body "$(gh issue view <number> --json body -q .body)

   ## Implementation Plan
   <details added from plan>"
   ```

2. **Create a feature branch** from `main`:
   ```bash
   git checkout main
   git pull origin main
   git checkout -b <label>/issue-<number>-<short-description>
   ```

3. **Make commits** with clear messages referencing the issue:
   ```bash
   git commit -m "Add feature X

   Implements #<issue-number>"
   ```

4. **Push and create a PR**:
   ```bash
   git push -u origin <branch-name>
   gh pr create --title "..." --body "Closes #<issue-number>"
   ```

5. **Never commit directly to `main`** - This is strictly enforced.

### Branch Naming Convention

Branch prefixes are determined by the first label on the issue:
- `<label>/issue-<number>-<short-description>` - Based on first issue label
- Examples: `bug/issue-123-fix-auth`, `enhancement/issue-456-add-cache`, `feature/issue-789-new-api`
- Default: `feature/issue-<number>-*` when no labels are present

### Before Starting Work

1. **Verify you are NOT on main:** `git branch --show-current`
2. Read the issue description fully
3. Check for dependencies on other issues
4. Ensure you're on a clean working tree
5. Pull latest `main` and create feature branch

### Code Standards

- Run `go build ./...` before committing
- Run `go test ./...` before pushing
- Add tests for new functionality
- Update documentation if adding new features

### Commit Messages

Use conventional commit prefixes for automated releases:

| Prefix | When to use | Version bump |
|--------|-------------|--------------|
| `fix:` | Bug fixes | Patch (0.1.0 → 0.1.1) |
| `feat:` | New features | Minor (0.1.0 → 0.2.0) |
| `chore:` | Maintenance (no release) | None |
| `docs:` | Documentation only | None |
| `refactor:` | Code refactoring | None |
| `test:` | Test changes only | None |

Format:
```
<prefix> <short summary>

<detailed description if needed>

Closes #<issue-number>
Co-Authored-By: Claude <noreply@anthropic.com>
```
