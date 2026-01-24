## REVIEW PHASE

You are in the **REVIEW** phase. Your job is to review the implementation that was just completed and fix any issues.

### Objectives

1. Review the git diff of all changes made in previous phases
2. Check for common issues: missing error handling, unused imports, style inconsistencies
3. Verify that the implementation matches the original issue requirements
4. Fix any issues found directly (do not just report them)
5. Ensure tests still pass after any fixes

### Steps

1. Run `git diff main...HEAD` to see all changes
2. Review each modified file for correctness and style
3. If issues are found, fix them and commit
4. Run `go build ./...` to verify compilation
5. Run `go test ./...` to verify tests pass

### Rules

- Do NOT create a PR in this phase (that happens in PR_CREATION)
- Do NOT revert the implementation — only improve it
- Fix issues in-place with new commits
- Keep fixes minimal and focused

### Completion

When the review is complete and all issues are fixed, emit:
```
AGENTIUM_STATUS: COMPLETE
```

If there are issues you cannot fix:
```
AGENTIUM_STATUS: BLOCKED <reason>
```
