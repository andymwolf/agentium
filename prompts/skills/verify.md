## VERIFY PHASE

You are in the **VERIFY** phase. Your job is to verify that CI checks pass on the PR and merge it.

### Steps

1. Check CI status: `gh pr checks <PR_NUMBER> --repo <REPOSITORY>`
2. If checks are **pending**, wait briefly and re-check (up to a few minutes)
3. If checks **pass**: merge the PR with `gh pr merge <PR_NUMBER> --squash --delete-branch --repo <REPOSITORY>`
4. If checks **fail**:
   - Read the CI logs to identify the failure
   - Diagnose the root cause
   - Fix the code
   - Commit and push the fix
   - Re-check CI status after pushing

### Rules

- Use `gh pr checks` to monitor CI status — do NOT guess whether checks passed
- If merge conflicts exist, rebase: `git pull --rebase origin main && git push --force-with-lease`
- Only merge when ALL required checks pass
- Use squash merge (`--squash`) to keep history clean
- Always delete the branch after merge (`--delete-branch`)

### Completion

When verification is complete, emit a structured handoff signal:

```
AGENTIUM_HANDOFF: {
  "checks_passed": true,
  "merge_successful": true,
  "merge_sha": "abc123",
  "failures_resolved": ["lint: fixed unused import"],
  "remaining_failures": []
}
```

If checks fail and you cannot resolve them:
```
AGENTIUM_HANDOFF: {
  "checks_passed": false,
  "merge_successful": false,
  "failures_resolved": [],
  "remaining_failures": ["test: TestFoo times out"]
}
```

Then emit:
```
AGENTIUM_STATUS: COMPLETE
```
