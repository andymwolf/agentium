# Modularize skill prompts for on-demand loading

## Problem

Currently all skill prompts may be loaded together, consuming context space even when only specific skills are needed for a task.

### Current State
- 18+ skill files in `prompts/skills/`
- Total: ~5,500 tokens for all skills
- Many tasks only need 1-3 specific skills

## Proposed Solution

1. **Implement lazy loading** - only load skill prompts when needed
2. **Create skill manifest** with metadata:
   ```yaml
   skills:
     - name: implement
       file: implement.md
       triggers: ["implement", "code", "build"]
       tokens: ~300
     - name: test
       file: test.md
       triggers: ["test", "verify"]
       tokens: ~250
   ```
3. **Add skill selection logic** - determine required skills based on task
4. **Cache loaded skills** - avoid reloading within same session

## Implementation Options

### Option A: Controller-level loading
Load skills in controller based on phase/task type

### Option B: Prompt composition
Build prompts dynamically with only needed skills

### Option C: Include directives
Use include syntax in prompts, resolve at runtime

## Impact

- **Token savings:** ~3,000-4,000 tokens per typical task
- **Effort:** Medium-High
- **Risk:** Medium - requires code changes to prompt loading

## Acceptance Criteria
- [ ] Skills loaded on-demand based on task
- [ ] Manifest defines skill metadata
- [ ] No functionality regression
- [ ] Documented skill loading behavior

## Labels
enhancement, performance
