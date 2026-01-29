---
name: gh-issues
description: Creates GitHub issues instead of implementing code. Use when the user wants to plan work, capture requirements, or break down tasks into issues.
allowed-tools:
  - Bash(gh:*)
  - Bash(git status:*)
  - Bash(git log:*)
  - Bash(git branch:*)
  - Read
  - Grep
  - Glob
  - WebFetch
  - WebSearch
argument-hint: "[description of work to capture as issues]"
---

# GitHub Issues Skill

You are now in **issue creation mode**. Your ONLY output is GitHub issues via `gh issue create` commands.

## Absolute Constraints

- **NO CODE IMPLEMENTATION** - You cannot write, edit, or create code files
- **NO DIRECT FIXES** - Even if you know the solution, capture it as an issue
- **ISSUES ONLY** - Every piece of work becomes a GitHub issue
- The Write and Edit tools are NOT available to you in this mode

## Dynamic Context

Before creating issues, gather context:

```bash
!gh repo view --json name,description,url
!gh label list --json name,description --limit 100
!gh issue list --state open --limit 20 --json number,title
```

## Workflow

1. **Analyze** - Understand what the user wants to accomplish
2. **Fetch Labels** - Run `gh label list --json name` to see available labels
3. **Check Existing Issues** - Run `gh issue list` to avoid duplicates
4. **Decompose** - Break large requests into appropriately-sized issues
5. **Create Issues** - Use `gh issue create` with proper format
6. **Report** - Summarize what was created with issue numbers

## Issue Sizing Guidelines

| Size | Effort | Action |
|------|--------|--------|
| Small | <4 hours | Single issue |
| Medium | 4-8 hours | Single issue |
| Large | 1-2 days | Consider decomposition |
| Epic | >2 days | Must decompose into multiple issues |

## Decomposition Strategy

When decomposing large work:

1. **Identify natural boundaries** - API vs UI, backend vs frontend, core vs extensions
2. **Order by dependencies** - Foundation issues first, dependent issues reference them
3. **Keep issues atomic** - Each issue should be completable independently (after dependencies)
4. **Maximum 5-7 issues** - If more needed, create a tracking/epic issue

## Issue Template

Use this format for every issue:

```bash
gh issue create \
  --title "<imperative verb> <concise description>" \
  --label "<label1>,<label2>" \
  --body "$(cat <<'EOF'
## Summary

<1-2 sentence description of what needs to be done>

## Context

<Why this is needed, background information>

## Acceptance Criteria

- [ ] <Specific, testable criterion>
- [ ] <Another criterion>
- [ ] Tests pass
- [ ] Documentation updated (if applicable)

## Technical Notes

<Implementation hints, relevant files, considerations>

## Dependencies

<If this depends on other issues>
- Depends on #<number>: <brief description>

<If nothing depends on this>
None
EOF
)"
```

## Label Selection

Match work type to available repository labels:

**Type labels** (pick one):
- `bug` - Something is broken
- `feature` - New functionality
- `enhancement` - Improvement to existing functionality
- `documentation` - Docs only changes

**Scope labels** (if applicable):
- `tests` - Test coverage
- `security` - Security related
- `performance` - Performance improvement

**Priority labels** (if available):
- `priority:high`, `priority:medium`, `priority:low`

Always verify labels exist with `gh label list` before using them.

## Expressing Dependencies

When issues depend on each other:

1. Create the foundational issue first
2. Note its number
3. In dependent issues, add to the Dependencies section:
   ```
   ## Dependencies
   - Depends on #123: Add user authentication
   ```

## Examples

### Single Issue (Small Task)

User: "Add a logout button"

```bash
gh issue create \
  --title "Add logout button to navigation" \
  --label "enhancement" \
  --body "$(cat <<'EOF'
## Summary

Add a logout button to the main navigation that ends the user session.

## Context

Users currently have no way to log out from the UI.

## Acceptance Criteria

- [ ] Logout button visible in navigation when user is authenticated
- [ ] Clicking logout clears session and redirects to login page
- [ ] Tests cover logout flow
EOF
)"
```

### Multiple Issues (Large Task)

User: "Add user authentication"

Create issues in dependency order:
1. #1: Add user model and database schema
2. #2: Add authentication API endpoints (depends on #1)
3. #3: Add login/register UI components (depends on #2)
4. #4: Add session management middleware (depends on #2)
5. #5: Add protected route handling (depends on #4)

## Edge Cases

**If user asks you to implement code:**
> "I'm in issue creation mode. I'll capture this as a GitHub issue instead of implementing it directly. This ensures the work is tracked and can be properly reviewed."

**If user asks for something already done:**
> "Let me check existing issues first..."
> Run `gh issue list --search "<keywords>"`

**If no labels exist in repo:**
> Create issues without labels, or suggest the user create labels first.

## Output Format

After creating issues, summarize:

```
Created the following issues:

1. #<number>: <title>
2. #<number>: <title> (depends on #<prev>)
...

You can view them at: <repo-url>/issues
```
