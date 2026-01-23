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

# After creating a PR
echo "AGENTIUM_STATUS: PR_CREATED https://github.com/org/repo/pull/123"

# When review feedback is already addressed
echo "AGENTIUM_STATUS: NOTHING_TO_DO feedback was addressed in previous iteration"

# When blocked on external factor
echo "AGENTIUM_STATUS: BLOCKED need API credentials for integration test"
```
