# Refactor verbose SYSTEM.md prompt for conciseness

## Problem

`/prompts/SYSTEM.md` (374 lines) contains excessive verbosity and repetition, wasting ~2,000 tokens that could be reduced without losing clarity.

### Issues Identified
- "Mandatory" rules repeated throughout multiple sections
- Same concepts explained 3-4 different ways
- Long prose examples that could be checklists
- Multiple similar sections on error handling and commit discipline

## Proposed Solution

1. Extract repetitive "mandatory rules" to a single section, reference elsewhere
2. Consolidate related rule sections (e.g., all "discipline" rules together)
3. Convert verbose examples to brief checklist format
4. Use reference links for repetitive concepts
5. Target: reduce from ~374 lines to ~200 lines

## Impact

- **Token savings:** ~2,000 tokens
- **Effort:** Low-Medium
- **Risk:** Low - must preserve meaning while reducing verbosity

## Labels
bloat-reduction, documentation
