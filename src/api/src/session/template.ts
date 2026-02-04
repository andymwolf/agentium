import { SessionContext } from './types.js';

/**
 * Template variable patterns that can be injected into prompts
 */
export type TemplateVariable =
  | 'issue_url'
  | 'issue_body'
  | 'instructions'
  | 'repository_url'
  | 'branch'
  | 'file_tree'
  | 'changed_files'
  | 'git_diff'
  | 'worker_output'
  | 'reviewer_output'
  | 'previous_phase_output';

/**
 * Additional context for template injection
 */
export interface TemplateContext {
  worker_output?: string;
  reviewer_output?: string;
  previous_phase_output?: string;
  issue_body?: string;
}

/**
 * Inject template variables into a prompt string
 *
 * Replaces {{variable_name}} patterns with actual values from the context.
 * If a variable is not found, it will be replaced with an empty string.
 *
 * @param template - The template string containing {{variable}} patterns
 * @param context - The session context with accumulated state
 * @param additionalContext - Additional context values (worker output, etc.)
 * @returns The template with all variables injected
 */
export function injectTemplateVariables(
  template: string,
  context: SessionContext,
  additionalContext: TemplateContext = {}
): string {
  // Build the variable map from context and additional context
  const variables: Record<string, string> = {
    // From prompt context
    issue_url: context.prompt_context.issue_url || '',
    instructions: context.prompt_context.instructions || '',

    // From repository config
    repository_url: context.repository.url,
    branch: context.repository.branch,

    // From accumulated state
    file_tree: context.file_tree || '',
    changed_files: formatChangedFiles(context.changed_files),
    git_diff: context.git_diff || '',

    // From additional context
    worker_output: additionalContext.worker_output || '',
    reviewer_output: additionalContext.reviewer_output || '',
    previous_phase_output: additionalContext.previous_phase_output || '',
    issue_body: additionalContext.issue_body || '',
  };

  // Replace all {{variable}} patterns
  return template.replace(/\{\{(\w+)\}\}/g, (match, variableName) => {
    if (variableName in variables) {
      return variables[variableName];
    }
    // Return empty string for unknown variables
    console.warn(`Unknown template variable: ${variableName}`);
    return '';
  });
}

/**
 * Format changed files array into a string
 */
function formatChangedFiles(files?: string[]): string {
  if (!files || files.length === 0) {
    return '';
  }
  return files.join('\n');
}

/**
 * Get the output from a previous phase
 *
 * @param context - The session context
 * @param phaseName - The name of the previous phase
 * @returns The output from the previous phase, or empty string if not found
 */
export function getPreviousPhaseOutput(
  context: SessionContext,
  phaseName: string
): string {
  return context.phase_outputs[phaseName] || '';
}

/**
 * Extract all template variables from a string
 *
 * @param template - The template string
 * @returns Array of variable names found in the template
 */
export function extractTemplateVariables(template: string): string[] {
  const matches = template.matchAll(/\{\{(\w+)\}\}/g);
  const variables = new Set<string>();
  for (const match of matches) {
    variables.add(match[1]);
  }
  return Array.from(variables);
}
