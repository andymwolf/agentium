## Implementation Principles

### Subagent Strategy
- Use subagents liberally to keep your main context clean
- Offload research, exploration, and parallel analysis to subagents
- One task per subagent for focused execution

### Course Correction
- If something goes sideways, STOP and reassess immediately — don't keep pushing
- When a fix feels hacky, pause and ask: "Is there a more elegant way?"
- Skip elegance checks for simple, obvious fixes — don't over-engineer

### Prove Your Work
- Never push code without running tests locally first
- If tests fail, fix them before continuing — don't leave broken state

### Step 3: Pre-Flight Check (MANDATORY)

Before creating any branch or PR, ALWAYS check for existing work:

```bash
# Check for existing remote branches for this issue (any prefix)
git branch -r | grep "/issue-<number>-"

# Check for existing open PRs for this issue
gh pr list --state open --json headRefName | grep "issue-<number>-"
```

**If an existing branch or PR is found:**
- Check out the existing branch: `git checkout <branch-name>`
- Review the current state and continue from where it left off
- Push updates to the existing branch
- Do NOT create a new branch or PR

**Only create a new branch if NO existing work is found.**

### Step 4: Create Feature Branch (Only If No Existing Work)
```bash
git checkout -b <prefix>/issue-<number>-<short-description>
```
Use the branch prefix from your context (e.g., `feature`, `bug`, `enhancement`).
Example: `feature/issue-42-add-login-button` or `bug/issue-123-fix-auth`

### Step 5: Implement Changes
- Make focused, minimal changes that address the issue
- Follow existing code style and patterns
- Add tests for new functionality when appropriate

### Step 6: Push Changes and Create Draft PR

After implementing changes and running tests, push your commits:

**First iteration (no existing PR):**
```bash
git push -u origin <prefix>/issue-<number>-<short-description>
gh pr create --draft \
  --title "Issue #<number>: <brief description>" \
  --body "Closes #<issue-number>

## Summary
Brief description of changes made.

## Status
This is a draft PR - implementation is in progress."
```

**Subsequent iterations (PR already exists):**
```bash
# Just push new commits - PR is automatically updated
git push origin <prefix>/issue-<number>-<short-description>
```

**Note:** The draft PR is created once during the first iteration with commits.
Subsequent iterations just push new commits to the same branch, which automatically
updates the PR. Do NOT create a new PR on each iteration.

### Step 7: Completion

When implementation is complete and tests pass, emit a structured handoff signal:

```
AGENTIUM_HANDOFF: {
  "branch_name": "<prefix>/issue-<number>-<description>",
  "commits": [
    {"hash": "<actual_hash>", "message": "<actual_message>"},
    {"hash": "<actual_hash>", "message": "<actual_message>"}
  ],
  "files_changed": ["<actual_file_path>", "<actual_file_path>"],
  "tests_passed": true,
  "test_output": "Summary of test results (optional)",
  "draft_pr_number": <actual PR number from gh pr create>,
  "draft_pr_url": "<actual PR URL from gh pr create>"
}
```

Then emit the appropriate status signal:
```
AGENTIUM_STATUS: TESTS_PASSED
```

Or if tests fail:
```
AGENTIUM_STATUS: TESTS_FAILED <brief description of failure>
```
