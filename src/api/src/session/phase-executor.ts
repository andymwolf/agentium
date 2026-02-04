import {
  PhaseDefinition,
  PhaseResult,
  WorkerResult,
  ReviewerResult,
  JudgeResult,
  SessionContext,
  AgentAdapter,
} from './types.js';
import { injectTemplateVariables, TemplateContext } from './template.js';
import {
  shouldSkipReviewer,
  shouldSkipJudge,
  SkipEvaluationContext,
} from './skip-conditions.js';

/**
 * Options for phase execution
 */
export interface PhaseExecutionOptions {
  adapter: AgentAdapter;
  previousPhaseOutput?: string;
}

/**
 * Execute a single phase with the Worker -> Reviewer -> Judge loop
 *
 * @param phase - The phase definition to execute
 * @param context - The session context
 * @param options - Execution options including the agent adapter
 * @returns The result of executing the phase
 */
export async function executePhase(
  phase: PhaseDefinition,
  context: SessionContext,
  options: PhaseExecutionOptions
): Promise<PhaseResult> {
  const startTime = Date.now();
  const { adapter, previousPhaseOutput } = options;

  // Build template context for this phase
  const templateContext: TemplateContext = {
    previous_phase_output: previousPhaseOutput || '',
  };

  // Step 1: Run Worker
  const workerResult = await executeWorker(phase, context, adapter, templateContext);

  // Update template context with worker output
  templateContext.worker_output = workerResult.output || '';

  // Step 2: Run Reviewer (may be skipped)
  const reviewerResult = await executeReviewer(
    phase,
    context,
    adapter,
    templateContext,
    workerResult
  );

  // Update template context with reviewer output
  templateContext.reviewer_output = reviewerResult.feedback || '';

  // Step 3: Run Judge (may be skipped)
  const judgeResult = await executeJudge(
    phase,
    context,
    adapter,
    templateContext,
    workerResult,
    reviewerResult
  );

  // Determine overall phase status
  const status = determinePhaseStatus(workerResult, judgeResult);

  return {
    name: phase.name,
    status,
    duration_ms: Date.now() - startTime,
    worker: workerResult,
    reviewer: reviewerResult,
    judge: judgeResult,
  };
}

/**
 * Execute the worker step
 */
async function executeWorker(
  phase: PhaseDefinition,
  context: SessionContext,
  adapter: AgentAdapter,
  templateContext: TemplateContext
): Promise<WorkerResult> {
  try {
    // Inject template variables into the worker prompt
    const prompt = injectTemplateVariables(
      phase.worker_prompt,
      context,
      templateContext
    );

    // Invoke the agent
    const result = await adapter.invoke(prompt);

    return {
      status: 'completed',
      output: result.output,
      token_metrics: result.token_metrics,
    };
  } catch (error) {
    return {
      status: 'failed',
      error: error instanceof Error ? error.message : String(error),
    };
  }
}

/**
 * Execute the reviewer step
 */
async function executeReviewer(
  phase: PhaseDefinition,
  context: SessionContext,
  adapter: AgentAdapter,
  templateContext: TemplateContext,
  workerResult: WorkerResult
): Promise<ReviewerResult> {
  // Check if reviewer should be skipped due to worker failure
  if (workerResult.status === 'failed') {
    return {
      status: 'skipped',
      skip_reason: 'worker_failed',
    };
  }

  // Build evaluation context
  const evalContext: SkipEvaluationContext = {
    worker_output: workerResult.output,
    changed_files: context.changed_files || [],
  };

  // Evaluate skip conditions using the new evaluator
  const skipResult = shouldSkipReviewer(phase.reviewer, context.behaviors, evalContext);
  if (skipResult.should_skip) {
    return {
      status: 'skipped',
      skip_reason: skipResult.skip_reason,
    };
  }

  try {
    // Inject template variables into the reviewer prompt
    const prompt = injectTemplateVariables(
      phase.reviewer_prompt,
      context,
      templateContext
    );

    // Invoke the agent
    const result = await adapter.invoke(prompt);

    return {
      status: 'completed',
      feedback: result.output,
      token_metrics: result.token_metrics,
    };
  } catch (error) {
    // Reviewer failure is non-fatal
    return {
      status: 'skipped',
      skip_reason: `reviewer_error: ${error instanceof Error ? error.message : String(error)}`,
    };
  }
}

/**
 * Execute the judge step
 */
async function executeJudge(
  phase: PhaseDefinition,
  context: SessionContext,
  adapter: AgentAdapter,
  templateContext: TemplateContext,
  workerResult: WorkerResult,
  reviewerResult: ReviewerResult
): Promise<JudgeResult> {
  // Check if judge should be skipped due to worker failure (unless judge_on_failure is set)
  if (workerResult.status === 'failed' && !context.behaviors.judge_on_failure) {
    return {
      status: 'skipped',
      skip_reason: 'worker_failed',
    };
  }

  // Build evaluation context
  const evalContext: SkipEvaluationContext = {
    worker_output: workerResult.output,
    changed_files: context.changed_files || [],
  };

  // Evaluate skip conditions using the new evaluator
  const skipResult = shouldSkipJudge(phase.judge, context.behaviors, evalContext);
  if (skipResult.should_skip) {
    return {
      status: 'skipped',
      skip_reason: skipResult.skip_reason,
      passed: true, // Auto-pass when skipped due to conditions
    };
  }

  try {
    // Inject template variables into the judge prompt
    const prompt = injectTemplateVariables(
      phase.judge_prompt,
      context,
      templateContext
    );

    // Invoke the agent
    const result = await adapter.invoke(prompt);

    // Parse the judge's response
    const judgment = parseJudgeResponse(result.output);

    return {
      status: 'completed',
      passed: judgment.passed,
      reasoning: judgment.reasoning,
      token_metrics: result.token_metrics,
    };
  } catch (error) {
    // Judge failure defaults to not passed
    return {
      status: 'skipped',
      skip_reason: `judge_error: ${error instanceof Error ? error.message : String(error)}`,
      passed: false,
    };
  }
}

/**
 * Parse the judge's response to extract passed/reasoning
 */
function parseJudgeResponse(output: string): { passed: boolean; reasoning: string } {
  try {
    // Try to extract JSON from the output
    const jsonMatch = output.match(/\{[\s\S]*?"passed"[\s\S]*?\}/);
    if (jsonMatch) {
      const parsed = JSON.parse(jsonMatch[0]);
      return {
        passed: Boolean(parsed.passed),
        reasoning: parsed.reasoning || '',
      };
    }
  } catch {
    // Fall through to heuristic parsing
  }

  // Fallback: look for pass/fail keywords
  const lowerOutput = output.toLowerCase();
  const passed =
    lowerOutput.includes('passed') ||
    lowerOutput.includes('acceptable') ||
    lowerOutput.includes('approved');

  return {
    passed,
    reasoning: output,
  };
}

/**
 * Determine the overall phase status based on worker and judge results
 */
function determinePhaseStatus(
  workerResult: WorkerResult,
  judgeResult: JudgeResult
): 'completed' | 'failed' {
  // If worker failed, phase failed
  if (workerResult.status === 'failed') {
    return 'failed';
  }

  // If judge ran and didn't pass, phase failed
  if (judgeResult.status === 'completed' && judgeResult.passed === false) {
    return 'failed';
  }

  return 'completed';
}
