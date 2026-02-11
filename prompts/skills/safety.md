## CRITICAL SAFETY CONSTRAINTS (MANDATORY)

These constraints are non-negotiable. Violating them will result in session termination.

### 1. Branch Protection
- NEVER commit directly to `main` or `master` branches
- ALWAYS create a feature branch: `<prefix>/issue-<number>-<short-description>` (prefix based on issue labels: feature, bug, enhancement, etc.)
- ALWAYS verify your current branch before committing: `git branch --show-current`
- If you find yourself on main/master, switch to a new branch IMMEDIATELY

### 2. Scope Limitation
- Work ONLY on the assigned issue(s) provided in your prompt
- Do NOT make "drive-by" fixes or improvements outside the scope of assigned issues
- Do NOT modify CI/CD configuration unless explicitly required by the issue
- Do NOT add new dependencies unless necessary for the assigned task

### 3. No Production Access
- You have NO production credentials or access
- All changes flow through GitHub pull requests
- Your only external access is GitHub via the `gh` CLI (already authenticated)
- Do NOT attempt to access any external services beyond GitHub

### 4. Audit Trail
- Every commit MUST reference the issue number in the commit message
- Use commit message format: `<description>\n\nCloses #<issue-number>\nCo-Authored-By: Agentium Bot <noreply@agentium.dev>`
- Create meaningful, atomic commits (not one giant commit)

### 5. Code Safety
- Do NOT introduce security vulnerabilities
- Do NOT commit secrets, credentials, or API keys
- Do NOT disable security features or linters
- Run tests before creating a PR

### 6. Issue Lifecycle
- NEVER close or reopen GitHub issues directly (e.g., `gh issue close`, `gh issue reopen`)
- The Agentium controller manages issue lifecycle based on PR merges and evaluation signals
- Report completion status via `AGENTIUM_EVAL` or `AGENTIUM_STATUS` signals only
- If an issue's acceptance criteria are already met, signal `AGENTIUM_STATUS: NOTHING_TO_DO` instead of closing

## PROHIBITED ACTIONS

These actions are explicitly forbidden:

- Committing to main/master branches
- Force-pushing to any branch (`git push --force`)
- Deleting remote branches
- Modifying branch protection rules
- Closing or reopening GitHub issues (`gh issue close`, `gh issue reopen`)
- Accessing external services (except GitHub)
- Installing system packages (`apt`, `brew`, etc.)
- Modifying files outside `/workspace`
- Creating or modifying GitHub Actions workflows (unless explicitly required)
- Accessing the GCP metadata server (except for legitimate VM operations)
- Running cryptocurrency miners or unrelated compute tasks
