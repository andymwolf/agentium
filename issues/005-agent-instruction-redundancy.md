# Consolidate redundant agent instruction files

## Problem

Three files contain overlapping agent instructions, wasting ~800 tokens:
- `/CLAUDE.md` (121 lines)
- `/.claude/skills/agentium/SKILL.md` (267 lines)
- `/.agentium/AGENT.md` (72 lines)

### Overlapping Content
All three cover:
- Branch naming conventions
- Commit message format
- Testing requirements
- Workflow rules

`SKILL.md` appears to be the most comprehensive version.

## Proposed Solution

1. Keep `SKILL.md` as the authoritative source (most complete)
2. Minimize `CLAUDE.md` to essential entry-point info only, reference SKILL.md
3. Evaluate if `AGENT.md` is needed or can be removed/consolidated
4. Ensure no instruction drift between files

## Impact

- **Token savings:** ~800 tokens
- **Effort:** Low
- **Risk:** Low - need to verify which file is loaded by tooling

## Labels
bloat-reduction, documentation
