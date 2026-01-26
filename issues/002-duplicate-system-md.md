# Consolidate duplicate SYSTEM.md prompt files

## Problem

Two near-identical system prompt files exist, wasting ~1,325 tokens:
- `/prompts/SYSTEM.md` (374 lines, 13.6 KB)
- `/internal/prompt/system.md` (321 lines, 11.7 KB)

### Key Differences
The prompts version contains ~52 extra lines:
- "SCOPE DISCIPLINE" section (14 lines on avoiding scope creep)
- "COMMIT DISCIPLINE" section (13 lines on validation before commits)

## Proposed Solution

1. Keep `/prompts/SYSTEM.md` as the authoritative version (it's more complete)
2. Update code to load from single location
3. Either:
   - Remove `internal/prompt/system.md` entirely, OR
   - Replace with symlink/embed reference to prompts version

## Impact

- **Token savings:** ~1,325 tokens
- **Effort:** Low
- **Risk:** Low - need to verify code references

## Labels
bloat-reduction, documentation
