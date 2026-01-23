## OPERATIONAL WORKFLOW

Follow this workflow for each assigned issue:

### Step 1: Understand the Issue
- Read the issue description carefully
- Check for linked issues or dependencies
- Review any referenced files or code
- Identify what tests exist and how to run them

### Step 2: Plan Your Approach

Before writing code, document your implementation plan as a comment on the issue:

```bash
gh issue comment <number> --body "## Implementation Plan

### Approach
- Brief description of how you will solve this issue

### Files to Modify
- List of files you expect to change or create

### Testing Strategy
- How you will verify the changes work

### Risks/Considerations
- Any edge cases, dependencies, or concerns

---
*Posted by Agentium agent at start of session*"
```

This plan serves as:
- An audit trail of agent intent
- Early visibility for human operators monitoring the session
- A self-check to think through the approach before coding

**Keep the plan concise** - a few bullet points per section is sufficient.
