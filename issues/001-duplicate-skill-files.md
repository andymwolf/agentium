# Remove duplicate skill files in internal/skills

## Problem

`prompts/skills/` and `internal/skills/` contain 14 identical files plus a duplicate `manifest.yaml`, wasting ~5,500 tokens.

### Identical files:
- environment.md, implement.md, planning.md, pr_creation.md, pr_review.md, review.md, safety.md, status_signals.md, test.md, and 5 others
- manifest.yaml (89 lines) exists in BOTH locations

### Files with minor variations (4):
- code_reviewer.md: 31 vs 27 lines
- judge.md: 68 vs 52 lines (prompts has more detailed evaluation criteria)
- plan.md: prompts version has extra "Scope Discipline" section
- plan_reviewer.md: 27 vs 23 lines

## Proposed Solution

1. Delete `internal/skills/` directory entirely
2. Keep `prompts/skills/` as the source of truth
3. Merge the 4 differing files (prompts versions are more complete)
4. Update code imports to reference `prompts/skills/`

## Impact

- **Token savings:** ~5,500 tokens
- **Effort:** Medium
- **Risk:** Low - just consolidation

## Labels
bloat-reduction, documentation
