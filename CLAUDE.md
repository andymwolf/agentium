# Claude Code Instructions for Agentium

## Workflow Requirements

### Branch and PR Workflow (REQUIRED)

When implementing any GitHub issue:

1. **Create a feature branch** from `main`:
   ```bash
   git checkout main
   git pull origin main
   git checkout -b feature/issue-<number>-<short-description>
   ```

2. **Make commits** with clear messages referencing the issue:
   ```bash
   git commit -m "Add feature X

   Implements #<issue-number>"
   ```

3. **Push and create a PR**:
   ```bash
   git push -u origin feature/issue-<number>-<short-description>
   gh pr create --title "..." --body "Closes #<issue-number>"
   ```

4. **Never commit directly to `main`**

### Branch Naming Convention

- `feature/issue-<number>-<short-description>` - New features
- `fix/issue-<number>-<short-description>` - Bug fixes
- `docs/issue-<number>-<short-description>` - Documentation

### Before Starting Work

1. Read the issue description fully
2. Check for dependencies on other issues
3. Ensure you're on a clean working tree
4. Pull latest `main`

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
