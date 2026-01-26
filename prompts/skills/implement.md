### Step 3: Pre-Flight Check (MANDATORY)

Before creating any branch or PR, ALWAYS check for existing work:

```bash
# Check for existing remote branches for this issue
git branch -r --list "origin/agentium/issue-<number>-*"

# Check for existing open PRs for this issue
gh pr list --search "head:agentium/issue-<number>" --state open
```

**If an existing branch or PR is found:**
- Check out the existing branch: `git checkout <branch-name>`
- Review the current state and continue from where it left off
- Push updates to the existing branch
- Do NOT create a new branch or PR

**Only create a new branch if NO existing work is found.**

### Step 4: Create Feature Branch (Only If No Existing Work)
```bash
git checkout -b agentium/issue-<number>-<short-description>
```
Use a short, descriptive suffix (e.g., `agentium/issue-42-add-login-button`)

### Step 5: Implement Changes
- Make focused, minimal changes that address the issue
- Follow existing code style and patterns
- Add tests for new functionality when appropriate

### Step 6: Run Tests
- Run the project's test suite to verify changes
- Fix any failing tests before proceeding

### Step 7: Commit Changes
- Write clear, descriptive commit messages
- Reference the issue number in commits

### Completion

When implementation is complete, emit a structured handoff signal:

```
AGENTIUM_HANDOFF: {
  "branch_name": "<the branch you created or are working on>",
  "commits": [{"sha": "<commit sha>", "message": "<commit message>"}],
  "files_changed": ["<list of files you modified>"],
  "tests_passed": true,
  "test_output": "<brief test output summary>"
}
```

Then emit the completion status:
```
AGENTIUM_STATUS: TESTS_PASSED
```

Or if tests failed:
```
AGENTIUM_STATUS: TESTS_FAILED <summary of failures>
```
