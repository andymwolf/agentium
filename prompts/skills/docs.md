## DOCS PHASE

You are in the **DOCS** phase. Your job is to make MINIMAL documentation updates for the implementation changes.

### Principle: Less is More

- Update ONLY documentation that MUST change for the code changes to be understood
- Do NOT create new documentation files unless the issue explicitly requires it
- A single, focused update is better than multiple scattered files
- If you're about to create a new .md file, ask: "Is this required by the issue?"

### Steps

1. Run `git diff` against the appropriate base branch to see all changes
2. Check if existing documentation needs small updates (README, inline comments)
3. Make minimal, targeted updates
4. If no documentation updates are strictly necessary, that's fine — emit COMPLETE

### Rules

- Do NOT create new documentation files unless explicitly required by the issue
- Do NOT write comprehensive security reviews, audit reports, or guides unless asked
- Do NOT modify README unless the changes affect how users interact with the project
- Prefer updating existing docs over creating new ones
- One focused doc file is better than many overlapping ones

### Completion

When documentation is updated (or no updates needed), emit a structured handoff signal:

```
AGENTIUM_HANDOFF: {
  "docs_updated": ["README.md", "docs/api.md"],
  "readme_changed": true
}
```

If no documentation updates were needed:
```
AGENTIUM_HANDOFF: {
  "docs_updated": [],
  "readme_changed": false
}
```

Then emit:
```
AGENTIUM_STATUS: COMPLETE
```
