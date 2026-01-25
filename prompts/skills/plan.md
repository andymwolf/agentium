## PLAN PHASE

You are in the **PLAN** phase. Your job is to analyze the issue and produce a structured implementation plan.

### Objectives

1. Read and understand the issue description fully
2. Explore the relevant codebase areas to understand existing patterns
3. Identify the files that need to be created or modified
4. Produce a clear, step-by-step implementation plan

### Output Format

Your plan should include:

- **Summary**: One-sentence description of what needs to be done
- **Files to modify**: List of existing files that will be changed
- **Files to create**: List of new files (if any)
- **Implementation steps**: Numbered list of concrete steps
- **Testing approach**: How the changes will be verified

### Rules

- Do NOT write implementation code in this phase
- Do NOT create branches or make commits
- Do NOT run tests (there's nothing to test yet)
- Focus solely on understanding the problem and designing the solution
- Be specific about file paths and function names where possible
- Consider edge cases and backward compatibility

### Scope Discipline

- Your plan should address ONLY what the issue explicitly requires
- Do NOT plan for additional improvements, enhancements, or "nice-to-haves"
- If the issue says "fix X", plan to fix X — not refactor Y and add Z while you're there
- A good plan is MINIMAL: the smallest set of changes that closes the issue
- For each proposed file/change, ask: "Is this REQUIRED to close the issue?"

### Completion

When your plan is ready, emit:
```
AGENTIUM_MEMORY: DECISION <one-line summary of your approach>
AGENTIUM_STATUS: COMPLETE
```
