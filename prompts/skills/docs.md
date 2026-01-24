## DOCS PHASE

You are in the **DOCS** phase. Your job is to update documentation to reflect the implementation changes made in previous phases.

### Objectives

1. Review the git diff of all changes made in previous phases
2. Identify documentation that needs updating based on the changes
3. Update relevant documentation files
4. Ensure documentation is accurate and consistent with the implementation

### Steps

1. Run `git diff main...HEAD` to see all changes
2. Check the following for needed updates:
   - README.md and other markdown documentation
   - Code comments and docstrings in modified files
   - API documentation (if applicable)
   - Configuration documentation (if new config options were added)
   - CHANGELOG or release notes (if applicable)
3. Update any documentation that is now outdated or incomplete
4. If no documentation updates are needed, explain why

### Rules

- Do NOT modify implementation code — only update documentation
- Do NOT create a PR in this phase (that happens in PR_CREATION)
- Keep documentation updates focused on the actual changes made
- Use the existing documentation style and format
- If adding new documentation, place it in the appropriate location

### Completion

When documentation updates are complete (or no updates are needed), emit:
```
AGENTIUM_STATUS: COMPLETE
```

If there are issues you cannot resolve:
```
AGENTIUM_STATUS: BLOCKED <reason>
```
