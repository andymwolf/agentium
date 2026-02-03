## STATUS SIGNALING

Emit status signals to indicate progress and completion to the Agentium controller.
Print these signals on their own line in the format: `AGENTIUM_STATUS: STATUS_NAME [optional message]`

### Issue Sessions

Use these signals when working on GitHub issues:

- `AGENTIUM_STATUS: TESTS_RUNNING` - About to run tests
- `AGENTIUM_STATUS: TESTS_PASSED` - All tests pass successfully
- `AGENTIUM_STATUS: TESTS_FAILED <summary>` - Tests failed (include brief summary)
- `AGENTIUM_STATUS: PR_CREATED <url>` - PR successfully created (include URL)
- `AGENTIUM_STATUS: COMPLETE` - All work for this issue is done

### PR Review Sessions

Use these signals when addressing code review feedback:

- `AGENTIUM_STATUS: ANALYZING` - Reading and understanding review feedback
- `AGENTIUM_STATUS: NOTHING_TO_DO` - Review feedback already addressed or no changes required
- `AGENTIUM_STATUS: PUSHED` - Changes have been pushed to the PR branch
- `AGENTIUM_STATUS: COMPLETE` - All review feedback has been addressed

### Any Session

These signals can be used in any session type:

- `AGENTIUM_STATUS: BLOCKED <reason>` - Cannot proceed without human intervention
- `AGENTIUM_STATUS: FAILED <reason>` - Unrecoverable error occurred

### Important Notes

1. **Always signal completion** - Even if no changes were made, signal `NOTHING_TO_DO` or `COMPLETE`
2. **Signal before long operations** - Emit `TESTS_RUNNING` before test suites
3. **Include context in messages** - Add brief explanations to help operators understand status
4. **Use the last signal** - The controller uses the most recent status signal for decision making

### Examples

```
# After tests pass
echo "AGENTIUM_STATUS: TESTS_PASSED"

# After creating a PR (use actual URL from gh pr create output)
echo "AGENTIUM_STATUS: PR_CREATED <actual PR URL>"

# When review feedback is already addressed
echo "AGENTIUM_STATUS: NOTHING_TO_DO feedback was addressed in previous iteration"

# When blocked on external factor
echo "AGENTIUM_STATUS: BLOCKED need API credentials for integration test"
```

## MEMORY SIGNALING

Emit memory signals to persist context across iterations. The controller captures these
and injects a summarized context into your prompt on subsequent iterations.

Format: `AGENTIUM_MEMORY: TYPE content`

### Signal Types

- `AGENTIUM_MEMORY: KEY_FACT <fact>` - Important discovery or context (e.g., "API requires auth header")
- `AGENTIUM_MEMORY: DECISION <decision>` - Architecture or approach decision made (e.g., "Using JWT for auth")
- `AGENTIUM_MEMORY: STEP_DONE <description>` - Completed implementation step
- `AGENTIUM_MEMORY: STEP_PENDING <description>` - Step still to be done in a future iteration
- `AGENTIUM_MEMORY: FILE_MODIFIED <path>` - File that was created or modified
- `AGENTIUM_MEMORY: ERROR <description>` - Error encountered that may need addressing

### Examples

```
# Record a key discovery
echo "AGENTIUM_MEMORY: KEY_FACT The database schema uses UUID primary keys"

# Record a decision
echo "AGENTIUM_MEMORY: DECISION Using middleware pattern for auth instead of decorators"

# Track progress
echo "AGENTIUM_MEMORY: STEP_DONE Implemented user registration endpoint"
echo "AGENTIUM_MEMORY: STEP_PENDING Add rate limiting to registration endpoint"

# Track files
echo "AGENTIUM_MEMORY: FILE_MODIFIED internal/auth/handler.go"

# Record errors for future reference
echo "AGENTIUM_MEMORY: ERROR Integration tests require REDIS_URL env var"
```

### Tips

1. **Be concise** - Memory entries have a budget; keep content short and actionable
2. **Signal pending steps** - Helps the next iteration know where to continue
3. **Record decisions** - Avoids re-evaluating the same choices across iterations
4. **Note key facts** - Especially about the codebase structure or API contracts

## EVALUATOR SIGNALING

When acting as a phase evaluator, emit a verdict signal to indicate whether the phase should advance, iterate, or is blocked.

Format: `AGENTIUM_EVAL: VERDICT [optional feedback]`

### Verdicts

- `AGENTIUM_EVAL: ADVANCE` - Phase output is acceptable, move to next phase
- `AGENTIUM_EVAL: ITERATE <feedback>` - Phase needs another iteration with the given feedback
- `AGENTIUM_EVAL: BLOCKED <reason>` - Cannot proceed without human intervention

### Critical Formatting Rules

**IMPORTANT:** Emit the verdict on its own line with NO surrounding markdown formatting.
Do NOT wrap in code blocks or backticks. The signal must appear at the start of a line.

Correct:
```
AGENTIUM_EVAL: ADVANCE
```

Wrong (will not be detected):
- `` `AGENTIUM_EVAL: ADVANCE` `` - wrapped in backticks
- Inside a markdown code fence with other content before it
- `Here is my verdict: AGENTIUM_EVAL: ADVANCE` - not at line start

### Examples

```
# Phase completed successfully
AGENTIUM_EVAL: ADVANCE

# Tests failed, need fixes
AGENTIUM_EVAL: ITERATE Tests failed in auth/handler_test.go - fix the nil pointer in TestLogin

# Cannot proceed
AGENTIUM_EVAL: BLOCKED Issue requirements are ambiguous - need clarification on auth method
```
