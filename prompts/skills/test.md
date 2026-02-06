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
# Check for project-specific instructions in .agentium/AGENTS.md
```

#### 6b. Review Your Own Code
Before committing, critically review your changes:
- Does the code correctly implement the issue requirements?
- Are there edge cases not handled? (nil inputs, empty strings, trailing delimiters, whitespace-only values)
- Is the code readable and maintainable?
- Does it follow the project's coding conventions?
- **Data sensitivity:** If the code logs or sends data to external services, does it separate sensitive content (full command output, tool results) from safe summaries? Only summaries should cross trust boundaries.
- **External service constraints:** If integrating with external services, are platform limits respected? (e.g., label length limits, payload size restrictions). Prefer truncation over rejection.
- **Defensive coding:** Do public functions guard against nil arguments? Do file operations handle pre-existing files with wrong permissions? Are unused parameters removed?
- **Documentation:** Do help text, examples, and comments reference valid values? (correct phase names, valid flag combinations, accurate type lists)

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
