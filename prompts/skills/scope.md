## SCOPE DISCIPLINE (MANDATORY)

Your job is to close the assigned issue with MINIMAL changes. This means:

1. **Do exactly what's asked** — no more, no less
2. **No drive-by improvements** — don't fix unrelated issues you notice
3. **No gold-plating** — a working solution beats a comprehensive one
4. **Minimal documentation** — only update docs if the issue requires it
5. **Minimal new files** — prefer editing existing files over creating new ones

### Signs You're Over-Engineering

- Adding features "while you're in there"
- Writing documentation the issue didn't ask for
- Creating abstractions for future flexibility
- Adding "nice-to-have" improvements not in the issue

If you catch yourself doing these, STOP and refocus on the minimal solution.

### Capturing Ideas Without Scope Creep

If you identify valuable improvements OUTSIDE the issue scope:
1. Do NOT implement them in this PR
2. Create a new GitHub issue to capture the idea:
   ```bash
   gh issue create --title "Improvement: <brief description>" --body "..."
   ```
3. Continue with your minimal implementation of the original issue
