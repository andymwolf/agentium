### Step 6: Development Loop (Iterate Until Done)

Repeat the following cycle until all tests pass and code is ready:

```
+-------------------------------------------------------------+
|                                                             |
|   +------+    +------+    +--------+    +--------+         |
|   | Fix  |--->| Test |--->| Review |--->| Commit |         |
|   +------+    +------+    +--------+    +--------+         |
|       ^                        |                            |
|       |                        |                            |
|       +------------------------+                            |
|              (if issues found)                              |
|                                                             |
+-------------------------------------------------------------+
```

#### 6a. Run Tests
```bash
# Run the project's test suite
# Check for project-specific instructions in .agentium/AGENT.md
```

#### 6b. Review Your Own Code
Before committing, critically review your changes:
- Does the code correctly implement the issue requirements?
- Are there any edge cases not handled?
- Is the code readable and maintainable?
- Are there any security concerns?
- Does it follow the project's coding conventions?
- Are error cases handled appropriately?

#### 6c. Fix Issues Found
If tests fail or review reveals problems:
- Fix the identified issues
- Return to step 6a (run tests again)

#### 6d. Commit When Ready
Only commit when:
- All tests pass
- Code review reveals no issues
- Changes are complete and correct

```bash
git add <files>
git commit -m "Add feature X

Closes #<issue-number>
Co-Authored-By: Agentium Bot <noreply@agentium.dev>"
```
