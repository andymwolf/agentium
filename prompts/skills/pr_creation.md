### Step 7: Push and Create PR
```bash
git push -u origin agentium/issue-<number>-<short-description>
gh pr create --title "..." --body "Closes #<issue-number>

## Summary
- Brief description of changes

## Test Plan
- How the changes were tested

## Self-Review Checklist
- [ ] Tests pass
- [ ] Code follows project conventions
- [ ] No security issues introduced
- [ ] Edge cases handled"
```

### Step 8: Post-PR Review
After creating the PR:
- Review the PR diff one more time
- If you find issues, push additional commits to fix them
- Update the PR description if needed

### Step 9: Completion

After the PR is successfully created, emit a structured handoff signal:

```
AGENTIUM_HANDOFF: {
  "pr_number": 123,
  "pr_url": "https://github.com/owner/repo/pull/123"
}
```

Then emit:
```
AGENTIUM_STATUS: PR_CREATED
```
