# Create condensed project summary file for Claude context

## Problem

Claude currently needs to read multiple files to understand the project structure and key concepts. A condensed summary would provide faster context loading and leave more room for actual work.

## Proposed Solution

1. Create `PROJECT_SUMMARY.md` (or similar) containing:
   - Project architecture overview (condensed)
   - Key file locations and their purposes
   - Important patterns and conventions
   - Quick reference for common tasks
   - Links to detailed docs for deep dives

2. Target size: 200-300 lines max (~750-1000 tokens)

3. Structure:
   ```markdown
   # Agentium Project Summary

   ## Architecture
   [Brief overview with key components]

   ## Key Files
   | Path | Purpose |
   |------|---------|
   | cmd/agentium/ | CLI entry point |
   | ... | ... |

   ## Patterns
   - Error handling: [brief]
   - Testing: [brief]

   ## Common Tasks
   - Build: `go build ./...`
   - Test: `go test ./...`
   ```

## Impact

- **Token savings:** Indirect - allows Claude to load summary instead of full files
- **Effort:** Medium
- **Risk:** None - additive improvement

## Acceptance Criteria
- [ ] Summary file created
- [ ] Covers all key project aspects
- [ ] Under 300 lines
- [ ] Referenced in CLAUDE.md for discoverability

## Labels
developer-experience, documentation
