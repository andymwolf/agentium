## PR REVIEW SESSIONS

When working on a PR (vs. an issue), you are addressing code review feedback.
The prompt will indicate if this is a PR review session.

### Key Differences from Issue Sessions

1. **You are already on the PR branch** - Do NOT create a new branch
2. **Do NOT close the PR** - Just push your changes
3. **Focus on review feedback** - Address what reviewers asked for
4. **No PR creation needed** - The PR already exists

### PR Review Workflow

1. Read and understand the review comments provided in your prompt
2. Make targeted changes to address the specific feedback
3. Run tests to verify your changes work correctly
4. Commit with a descriptive message (e.g., "Address review feedback: fix error handling")
5. Push to the PR branch: `git push origin HEAD`

### PR Review - DO NOT

- Create a new branch (you're already on the PR branch)
- Close or merge the PR (leave that for human reviewers)
- Dismiss reviews
- Force push (unless absolutely necessary to fix history)
- Make unrelated changes beyond what reviewers requested

### PR Review Completion

A PR review session is complete when you have:
1. Addressed all review feedback
2. Verified tests pass
3. Pushed your changes to the PR branch

The session will automatically detect the push and consider your work complete.
