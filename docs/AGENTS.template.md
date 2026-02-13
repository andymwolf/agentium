# Project Agent Instructions

This file provides project-specific instructions for Agentium agents.
Copy this template to `.agentium/AGENTS.md` in your repository and customize it.

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

## Workflow Principles

These principles apply to ALL phases of work.

### Core Standards

- **Simplicity First**: Make every change as simple as possible. The right amount of complexity is the minimum needed for the current task.
- **No Laziness**: Find root causes. No temporary fixes. No placeholders. Senior developer standards.
- **Minimal Impact**: Changes should only touch what's necessary. Three similar lines of code is better than a premature abstraction.

### Autonomy

- When given a bug report or failing test: just fix it. Don't ask for hand-holding.
- Point at logs, errors, failing tests — then resolve them.
- Go fix failing CI tests without being told how.

### Verification

- Never mark a task complete without proving it works.
- Run tests, check output, demonstrate correctness.
- Ask yourself: "Would a senior engineer approve this?"

### Subagent Strategy

- Use subagents liberally to keep your main context clean.
- Offload research, exploration, and parallel analysis to subagents.
- One task per subagent for focused execution.

### Course Correction

- If something goes sideways, STOP and reassess immediately — don't keep pushing.
- When a fix feels hacky, pause and ask: "Is there a more elegant way?"
- Skip elegance checks for simple, obvious fixes — don't over-engineer.

## Commit Conventions

Use conventional commit prefixes:

| Prefix | When to use |
|--------|-------------|
| `fix:` | Bug fixes |
| `feat:` | New features |
| `chore:` | Maintenance (no release) |
| `docs:` | Documentation only |
| `refactor:` | Code refactoring |
| `test:` | Test changes only |

Format:
```
<prefix> <short summary>

<detailed description if needed>

Closes #<issue-number>
Co-Authored-By: Agentium Bot <noreply@agentium.dev>
```

## Branch Naming

Branch prefixes are determined by the first label on the issue:

- `<label>/issue-<number>-<short-description>`
- Examples: `bug/issue-123-fix-auth`, `enhancement/issue-456-add-cache`, `feature/issue-789-new-api`
- Default: `feature/issue-<number>-*` when no labels are present

## Code Conventions

- Follow the existing code style in the repository
- Use descriptive variable and function names
- Add comments for complex or non-obvious logic
- Keep functions focused and reasonably sized

## Self-Review Checklist

Before committing, critically review your changes:

- Does the code correctly implement the issue requirements?
- Are there edge cases not handled? (nil inputs, empty strings, trailing delimiters, whitespace-only values)
- Is the code readable and maintainable?
- Does it follow the project's coding conventions?
- **Data sensitivity:** If the code logs or sends data to external services, does it separate sensitive content (full command output, tool results) from safe summaries? Only summaries should cross trust boundaries.
- **External service constraints:** If integrating with external services, are platform limits respected? (e.g., label length limits, payload size restrictions). Prefer truncation over rejection.
- **Defensive coding:** Do public functions guard against nil arguments? Do file operations handle pre-existing files with wrong permissions? Are unused parameters removed?
- **Documentation:** Do help text, examples, and comments reference valid values? (correct phase names, valid flag combinations, accurate type lists)

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

---

# Package-Specific Instructions (Monorepos)

For pnpm workspace monorepos, you can create package-specific AGENTS.md files. Place them at `<package-path>/.agentium/AGENTS.md` and they will be merged with the root instructions when the agent targets that package.

**Example structure:**

```
my-monorepo/
├── .agentium/AGENTS.md          # Root instructions (applies to all packages)
├── packages/
│   ├── core/
│   │   └── .agentium/AGENTS.md  # Core-specific instructions
│   └── web/
│       └── .agentium/AGENTS.md  # Web-specific instructions
└── pnpm-workspace.yaml
```

**Example package AGENTS.md (`packages/web/.agentium/AGENTS.md`):**

```markdown
# Web Package Instructions

## Build & Test Commands

```bash
pnpm --filter web build
pnpm --filter web test
pnpm --filter web lint
```

## Framework Notes

- This package uses React with TypeScript
- State management via Zustand
- Styling with Tailwind CSS

## Testing Requirements

- Use React Testing Library for component tests
- Mock API calls with MSW
- Snapshot tests for complex UI components
```

The agent will see both root and package instructions, clearly separated.
