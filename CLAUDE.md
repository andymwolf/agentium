# Claude Code Instructions for Agentium

## CRITICAL WORKFLOW RULES

**NEVER make code changes directly on the `main` branch.** All work must follow the branch workflow below.

**ALWAYS create a feature branch BEFORE writing any code.** If you find yourself on `main`, switch to a feature branch immediately.

**When given a plan or implementation instructions:**
1. First update the relevant GitHub issue(s) with the plan details (use `gh issue edit`)
2. Create a feature branch
3. Then implement the changes

## Workflow Requirements

### Branch and PR Workflow (REQUIRED)

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
   git checkout -b feature/issue-<number>-<short-description>
   ```

3. **Make commits** with clear messages referencing the issue:
   ```bash
   git commit -m "Add feature X

   Implements #<issue-number>"
   ```

4. **Push and create a PR**:
   ```bash
   git push -u origin feature/issue-<number>-<short-description>
   gh pr create --title "..." --body "Closes #<issue-number>"
   ```

5. **Never commit directly to `main`** - This is strictly enforced.

### Branch Naming Convention

- `feature/issue-<number>-<short-description>` - New features
- `fix/issue-<number>-<short-description>` - Bug fixes
- `docs/issue-<number>-<short-description>` - Documentation

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

Format:
```
<short summary>

<detailed description if needed>

Closes #<issue-number>
Co-Authored-By: Claude <noreply@anthropic.com>
```

## Project Structure

```
agentium/
├── cmd/agentium/       # CLI entry point
├── cmd/controller/     # Session controller entry point
├── internal/
│   ├── cli/            # CLI commands
│   ├── config/         # Configuration
│   ├── controller/     # Session controller
│   ├── agent/          # Agent adapters
│   ├── provisioner/    # Cloud provisioners
│   └── cloud/          # Cloud provider clients
├── terraform/modules/  # Terraform modules
├── docker/             # Dockerfiles
└── bootstrap/          # Bootstrap scripts (Phase 0)
```

## Testing

```bash
# Run all tests
go test ./...

# Run with verbose output
go test ./... -v

# Run specific package
go test ./internal/config/...
```

## Building

```bash
# Build CLI
go build -o agentium ./cmd/agentium

# Build controller
go build -o controller ./cmd/controller
```
