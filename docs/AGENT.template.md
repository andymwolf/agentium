# Project Agent Instructions

This file provides project-specific instructions for Agentium agents.
Copy this template to `.agentium/AGENT.md` in your repository and customize it.

## Build & Test Commands

Run these commands before creating a PR:

```bash
# Build the project
# Example: go build ./...
# Example: npm run build
# Example: cargo build

# Run tests
# Example: go test ./...
# Example: npm test
# Example: cargo test

# Run linter
# Example: golangci-lint run
# Example: npm run lint
# Example: cargo clippy
```

## Code Conventions

- Follow the existing code style in the repository
- Use descriptive variable and function names
- Add comments for complex or non-obvious logic
- Keep functions focused and reasonably sized

## Architecture Notes

Describe your project's architecture here:

- List important directories and their purposes
- Explain key design patterns used
- Note any important dependencies or integrations

## Testing Requirements

- Always add tests for new functionality
- Update existing tests when changing behavior
- Ensure all tests pass before creating a PR

## Additional Constraints

Add any project-specific rules:

- Do not modify CI/CD configuration
- Do not add new dependencies without discussion
- Follow semantic versioning for changes
- Use conventional commit messages

## Common Patterns

Document patterns the agent should follow:

### Error Handling

```
// Example error handling pattern
```

### Logging

```
// Example logging pattern
```

### Configuration

```
// Example configuration pattern
```

## Off-Limits Areas

List files or directories the agent should not modify:

- `.github/workflows/` - CI/CD pipelines
- `secrets/` - Sensitive configuration
- `vendor/` - Vendored dependencies

## Contact

If the agent encounters issues outside its scope, it should note them in the PR description for human follow-up.
