# Add .claudeignore to exclude non-essential files from context

## Problem

When Claude loads the repository context, it includes files that aren't useful for most tasks, consuming valuable context window space.

### Files to Exclude
- `go.sum` (~14,000 tokens) - auto-generated, rarely needed
- Generated files and build artifacts
- Verbose documentation that's rarely referenced
- Test fixtures and mock data files
- Vendor directories (if any)

## Proposed Solution

1. Create `.claudeignore` file at repository root
2. Add patterns for:
   ```
   go.sum
   *.pb.go
   *_generated.go
   vendor/
   .git/
   *.test
   testdata/
   ```
3. Document the file's purpose in a comment header

## Impact

- **Token savings:** ~15,000+ tokens (context-specific)
- **Effort:** Low
- **Risk:** None - only affects Claude context loading

## Acceptance Criteria
- [ ] `.claudeignore` file created
- [ ] Tested that Claude respects the ignore patterns
- [ ] Documented in CLAUDE.md or README

## Labels
developer-experience, tooling
