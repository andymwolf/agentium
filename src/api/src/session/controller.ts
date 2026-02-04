import { v4 as uuidv4 } from 'uuid';
import {
  SessionRequest,
  SessionResult,
  SessionContext,
  PhaseResult,
  PhaseDefinition,
  AgentAdapter,
  BehaviorConfig,
} from './types.js';
import { getWorkflow } from './workflow.js';
import { executePhase } from './phase-executor.js';

/**
 * Session controller options
 */
export interface SessionControllerOptions {
  adapter: AgentAdapter;
}

/**
 * Session Controller
 *
 * Orchestrates the execution of workflow phases, running the Worker -> Reviewer -> Judge
 * loop for each phase and managing context propagation between phases.
 */
export class SessionController {
  private adapter: AgentAdapter;

  constructor(options: SessionControllerOptions) {
    this.adapter = options.adapter;
  }

  /**
   * Execute a session with the given request configuration
   *
   * @param request - The session request containing workflow, repository, and context
   * @returns The session result with all phase results
   */
  async execute(request: SessionRequest): Promise<SessionResult> {
    const startTime = Date.now();
    const taskId = request.task_id || uuidv4();

    // Initialize session context
    const context = this.createSessionContext(taskId, request);

    // Resolve phases from workflow or custom phases
    const phases = this.resolvePhases(request);

    // Execute each phase
    const phaseResults: PhaseResult[] = [];
    let previousPhaseOutput: string | undefined;

    for (const phase of phases) {
      const result = await executePhase(phase, context, {
        adapter: this.adapter,
        previousPhaseOutput,
      });

      phaseResults.push(result);

      // Update context with phase output for next phase
      if (result.worker.output) {
        context.phase_outputs[phase.name] = result.worker.output;
        previousPhaseOutput = result.worker.output;
      }

      // Update accumulated state after implementation phase
      if (phase.name === 'IMPLEMENT' && result.status === 'completed') {
        // In a real implementation, this would query the repository state
        // For now, we simulate the state update
        context.changed_files = this.extractChangedFiles(result.worker.output);
        context.git_diff = this.extractGitDiff(result.worker.output);
      }

      // If phase failed, we still continue to collect results but might want to
      // stop execution based on configuration (future enhancement)
    }

    // Determine overall session status
    const status = this.determineSessionStatus(phaseResults);

    return {
      task_id: taskId,
      status,
      duration_ms: Date.now() - startTime,
      phases: phaseResults,
      behaviors_applied: context.behaviors,
    };
  }

  /**
   * Create initial session context from request
   */
  private createSessionContext(
    taskId: string,
    request: SessionRequest
  ): SessionContext {
    return {
      task_id: taskId,
      repository: request.repository,
      prompt_context: request.prompt_context || {},
      behaviors: this.normalizeBehaviors(request.behaviors),
      phase_outputs: {},
    };
  }

  /**
   * Normalize behavior config with defaults
   */
  private normalizeBehaviors(behaviors?: BehaviorConfig): BehaviorConfig {
    return {
      skip_reviewer_on_empty: behaviors?.skip_reviewer_on_empty ?? false,
      skip_judge_on_simple: behaviors?.skip_judge_on_simple ?? false,
      judge_on_failure: behaviors?.judge_on_failure ?? false,
      auto_retry_on_rate_limit: behaviors?.auto_retry_on_rate_limit ?? true,
    };
  }

  /**
   * Resolve phases from workflow name or custom phases
   */
  private resolvePhases(request: SessionRequest): PhaseDefinition[] {
    if (request.workflow === 'default') {
      const workflow = getWorkflow('default');
      return workflow.phases;
    }

    if (request.phases && request.phases.length > 0) {
      return request.phases;
    }

    // Default to the default workflow if nothing specified
    const workflow = getWorkflow('default');
    return workflow.phases;
  }

  /**
   * Determine overall session status based on phase results
   */
  private determineSessionStatus(
    phaseResults: PhaseResult[]
  ): 'completed' | 'failed' {
    // Session fails if any phase failed
    const hasFailed = phaseResults.some((phase) => phase.status === 'failed');
    return hasFailed ? 'failed' : 'completed';
  }

  /**
   * Extract changed files from worker output
   * This is a placeholder - in a real implementation, this would
   * actually query the git repository
   */
  private extractChangedFiles(output?: string): string[] {
    if (!output) return [];

    // Simple heuristic: look for file paths in the output
    const filePattern = /(?:^|\s)((?:[\w-]+\/)*[\w-]+\.[a-z]+)(?:\s|$)/gm;
    const files: string[] = [];
    let match;

    while ((match = filePattern.exec(output)) !== null) {
      files.push(match[1]);
    }

    return [...new Set(files)]; // Remove duplicates
  }

  /**
   * Extract git diff from worker output
   * This is a placeholder - in a real implementation, this would
   * actually run git diff
   */
  private extractGitDiff(output?: string): string {
    if (!output) return '';

    // Look for diff-like content
    const diffPattern = /```diff([\s\S]*?)```/g;
    const diffs: string[] = [];
    let match;

    while ((match = diffPattern.exec(output)) !== null) {
      diffs.push(match[1].trim());
    }

    return diffs.join('\n');
  }
}

/**
 * Create a new session controller instance
 */
export function createSessionController(
  options: SessionControllerOptions
): SessionController {
  return new SessionController(options);
}
