import { WorkflowDefinition, PhaseDefinition } from './types.js';

/**
 * Default PLAN phase definition
 */
const PLAN_PHASE: PhaseDefinition = {
  name: 'PLAN',
  max_iterations: 3,
  worker_prompt: `You are a planning agent. Analyze the task and create an implementation plan.

Task Context:
- Issue URL: {{issue_url}}
- Instructions: {{instructions}}
- Repository: {{repository_url}}
- Branch: {{branch}}

File Tree:
{{file_tree}}

Create a detailed implementation plan that includes:
1. Files to be modified or created
2. Key changes needed
3. Testing strategy
4. Potential risks or considerations

Output a clear, actionable plan.`,
  reviewer_prompt: `You are a code review agent. Review the implementation plan for completeness and correctness.

Original Task:
- Issue URL: {{issue_url}}
- Instructions: {{instructions}}

Worker Output:
{{worker_output}}

Review the plan and provide feedback on:
1. Is the plan complete and actionable?
2. Are there any missing steps?
3. Are there any potential issues with the approach?

Provide constructive feedback.`,
  judge_prompt: `You are a judge agent. Evaluate whether the plan meets the requirements.

Original Task:
- Issue URL: {{issue_url}}
- Instructions: {{instructions}}

Worker Output:
{{worker_output}}

Reviewer Feedback:
{{reviewer_output}}

Determine if the plan is acceptable. Output a JSON object:
{
  "passed": true/false,
  "reasoning": "explanation"
}`,
};

/**
 * Default IMPLEMENT phase definition
 */
const IMPLEMENT_PHASE: PhaseDefinition = {
  name: 'IMPLEMENT',
  max_iterations: 5,
  worker_prompt: `You are an implementation agent. Execute the implementation plan.

Task Context:
- Issue URL: {{issue_url}}
- Instructions: {{instructions}}
- Repository: {{repository_url}}
- Branch: {{branch}}

Previous Phase Output (Plan):
{{previous_phase_output}}

File Tree:
{{file_tree}}

Implement the changes according to the plan. Make sure to:
1. Follow the repository's coding conventions
2. Write clean, maintainable code
3. Add appropriate tests
4. Handle edge cases

Provide the implementation output including files changed.`,
  reviewer_prompt: `You are a code review agent. Review the implementation for quality and correctness.

Original Task:
- Issue URL: {{issue_url}}
- Instructions: {{instructions}}

Plan:
{{previous_phase_output}}

Worker Output:
{{worker_output}}

Changed Files:
{{changed_files}}

Git Diff:
{{git_diff}}

Review the implementation for:
1. Code quality and style
2. Correctness and completeness
3. Test coverage
4. Potential bugs or issues

Provide constructive feedback.`,
  judge_prompt: `You are a judge agent. Evaluate whether the implementation is acceptable.

Original Task:
- Issue URL: {{issue_url}}
- Instructions: {{instructions}}

Plan:
{{previous_phase_output}}

Worker Output:
{{worker_output}}

Reviewer Feedback:
{{reviewer_output}}

Changed Files:
{{changed_files}}

Determine if the implementation is acceptable. Output a JSON object:
{
  "passed": true/false,
  "reasoning": "explanation"
}`,
};

/**
 * Default DOCS phase definition
 */
const DOCS_PHASE: PhaseDefinition = {
  name: 'DOCS',
  max_iterations: 2,
  worker_prompt: `You are a documentation agent. Update documentation based on the implementation.

Task Context:
- Issue URL: {{issue_url}}
- Instructions: {{instructions}}
- Repository: {{repository_url}}
- Branch: {{branch}}

Implementation Phase Output:
{{previous_phase_output}}

Changed Files:
{{changed_files}}

Git Diff:
{{git_diff}}

Update relevant documentation:
1. README updates if applicable
2. API documentation for new endpoints
3. Code comments for complex logic
4. Any other relevant documentation

Output the documentation updates made.`,
  reviewer_prompt: `You are a documentation review agent. Review the documentation updates.

Original Task:
- Issue URL: {{issue_url}}
- Instructions: {{instructions}}

Implementation Changes:
{{previous_phase_output}}

Worker Output (Documentation):
{{worker_output}}

Review the documentation for:
1. Accuracy and completeness
2. Clarity and readability
3. Consistency with implementation

Provide feedback on any improvements needed.`,
  judge_prompt: `You are a judge agent. Evaluate whether the documentation is acceptable.

Original Task:
- Issue URL: {{issue_url}}
- Instructions: {{instructions}}

Worker Output:
{{worker_output}}

Reviewer Feedback:
{{reviewer_output}}

Determine if the documentation is acceptable. Output a JSON object:
{
  "passed": true/false,
  "reasoning": "explanation"
}`,
};

/**
 * Default workflow definition with PLAN -> IMPLEMENT -> DOCS phases
 */
export const DEFAULT_WORKFLOW: WorkflowDefinition = {
  name: 'default',
  phases: [PLAN_PHASE, IMPLEMENT_PHASE, DOCS_PHASE],
};

/**
 * Get workflow definition by name
 */
export function getWorkflow(name: 'default'): WorkflowDefinition {
  if (name === 'default') {
    return DEFAULT_WORKFLOW;
  }
  throw new Error(`Unknown workflow: ${name}`);
}
