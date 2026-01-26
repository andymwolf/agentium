# Reduce README/getting-started.md documentation overlap

## Problem

`README.md` (358 lines) and `docs/getting-started.md` (220 lines) contain significant overlapping content, wasting ~1,500 tokens.

### Overlapping Content
Both files cover:
- "How It Works" architecture explanation
- "Ralph Wiggum Loop" concept
- Prerequisites
- Installation steps
- Quick Start guide

## Proposed Solution

1. Convert `README.md` to a concise landing page (max 100-150 lines):
   - Brief project description
   - Key features (bulleted)
   - Links to detailed docs
   - Badge/status info
2. Move all detailed getting-started content to `docs/getting-started.md`
3. Ensure single source of truth for each concept

## Impact

- **Token savings:** ~1,500 tokens
- **Effort:** Medium
- **Risk:** Low - documentation only

## Labels
bloat-reduction, documentation
