import {
  SkipCondition,
  SessionContext,
  BehaviorConfig,
  ReviewerConfig,
  JudgeConfig,
} from './types.js';

/**
 * Context available for evaluating skip conditions
 */
export interface SkipEvaluationContext {
  worker_output?: string;
  changed_files: string[];
}

/**
 * Result of skip condition evaluation
 */
export interface SkipEvaluationResult {
  should_skip: boolean;
  skip_reason?: string;
}

/**
 * Check if output is empty or whitespace only
 */
export function isOutputEmpty(output?: string): boolean {
  return !output || output.trim().length === 0;
}

/**
 * Check if output is considered "simple" (for skip_judge_on_simple)
 * Simple outputs are short and don't contain complex changes.
 * Considers output simple if it has fewer than 5 non-empty lines.
 */
export function isSimpleOutput(output?: string): boolean {
  if (!output) return true;

  const lines = output.split('\n').filter((l) => l.trim().length > 0);
  return lines.length < 5;
}

/**
 * Evaluate a single skip condition against the context
 */
export function evaluateSkipCondition(
  condition: SkipCondition,
  evalContext: SkipEvaluationContext
): SkipEvaluationResult {
  switch (condition) {
    case 'empty_output':
      if (isOutputEmpty(evalContext.worker_output)) {
        return { should_skip: true, skip_reason: 'empty_output' };
      }
      return { should_skip: false };

    case 'simple_output':
      if (isSimpleOutput(evalContext.worker_output)) {
        return { should_skip: true, skip_reason: 'simple_output' };
      }
      return { should_skip: false };

    case 'no_code_changes':
      if (evalContext.changed_files.length === 0) {
        return { should_skip: true, skip_reason: 'no_code_changes' };
      }
      return { should_skip: false };

    case true:
      return { should_skip: true, skip_reason: 'always_skip' };

    default:
      return { should_skip: false };
  }
}

/**
 * Determine if reviewer should be skipped based on phase and behavior configuration.
 *
 * Skip evaluation order:
 * 1. Check phase-level `skip: true` first
 * 2. Check phase-level `skip_on` condition
 * 3. Check workflow-level behavior defaults
 */
export function shouldSkipReviewer(
  reviewerConfig: ReviewerConfig | undefined,
  behaviors: BehaviorConfig,
  evalContext: SkipEvaluationContext
): SkipEvaluationResult {
  // 1. Check phase-level skip: true
  if (reviewerConfig?.skip === true) {
    return { should_skip: true, skip_reason: 'phase_config' };
  }

  // 2. Check phase-level skip_on condition
  if (reviewerConfig?.skip_on !== undefined) {
    const result = evaluateSkipCondition(reviewerConfig.skip_on, evalContext);
    if (result.should_skip) {
      return result;
    }
  }

  // 3. Check workflow-level behavior: skip_reviewer_on_empty
  if (behaviors.skip_reviewer_on_empty && isOutputEmpty(evalContext.worker_output)) {
    return { should_skip: true, skip_reason: 'empty_output' };
  }

  return { should_skip: false };
}

/**
 * Determine if judge should be skipped based on phase and behavior configuration.
 *
 * Skip evaluation order:
 * 1. Check phase-level `skip: true` first
 * 2. Check phase-level `skip_on` condition
 * 3. Check workflow-level behavior defaults
 */
export function shouldSkipJudge(
  judgeConfig: JudgeConfig | undefined,
  behaviors: BehaviorConfig,
  evalContext: SkipEvaluationContext
): SkipEvaluationResult {
  // 1. Check phase-level skip: true
  if (judgeConfig?.skip === true) {
    return { should_skip: true, skip_reason: 'phase_config' };
  }

  // 2. Check phase-level skip_on condition
  if (judgeConfig?.skip_on !== undefined) {
    const result = evaluateSkipCondition(judgeConfig.skip_on, evalContext);
    if (result.should_skip) {
      return result;
    }
  }

  // 3. Check workflow-level behavior: skip_judge_on_simple
  if (behaviors.skip_judge_on_simple && isSimpleOutput(evalContext.worker_output)) {
    return { should_skip: true, skip_reason: 'simple_output' };
  }

  return { should_skip: false };
}
