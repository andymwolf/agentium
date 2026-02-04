import { describe, it, expect } from 'vitest';
import {
  isOutputEmpty,
  isSimpleOutput,
  evaluateSkipCondition,
  shouldSkipReviewer,
  shouldSkipJudge,
  SkipEvaluationContext,
} from '../skip-conditions.js';
import { BehaviorConfig, ReviewerConfig, JudgeConfig } from '../types.js';

describe('isOutputEmpty', () => {
  it('should return true for undefined output', () => {
    expect(isOutputEmpty(undefined)).toBe(true);
  });

  it('should return true for empty string', () => {
    expect(isOutputEmpty('')).toBe(true);
  });

  it('should return true for whitespace only', () => {
    expect(isOutputEmpty('   \n\t  ')).toBe(true);
  });

  it('should return false for non-empty output', () => {
    expect(isOutputEmpty('Hello')).toBe(false);
  });
});

describe('isSimpleOutput', () => {
  it('should return true for undefined output', () => {
    expect(isSimpleOutput(undefined)).toBe(true);
  });

  it('should return true for empty string', () => {
    expect(isSimpleOutput('')).toBe(true);
  });

  it('should return true for output with less than 5 non-empty lines', () => {
    expect(isSimpleOutput('Line 1\nLine 2\nLine 3\nLine 4')).toBe(true);
  });

  it('should return false for output with 5 or more non-empty lines', () => {
    expect(isSimpleOutput('Line 1\nLine 2\nLine 3\nLine 4\nLine 5')).toBe(false);
  });

  it('should ignore empty lines when counting', () => {
    expect(isSimpleOutput('Line 1\n\nLine 2\n\n\nLine 3\n\nLine 4\n\n')).toBe(true);
  });
});

describe('evaluateSkipCondition', () => {
  const emptyContext: SkipEvaluationContext = {
    worker_output: '',
    changed_files: [],
  };

  const simpleContext: SkipEvaluationContext = {
    worker_output: 'Done.',
    changed_files: [],
  };

  const complexContext: SkipEvaluationContext = {
    worker_output: 'Line 1\nLine 2\nLine 3\nLine 4\nLine 5\nLine 6',
    changed_files: ['file1.ts', 'file2.ts'],
  };

  it('should skip on empty_output when output is empty', () => {
    const result = evaluateSkipCondition('empty_output', emptyContext);
    expect(result.should_skip).toBe(true);
    expect(result.skip_reason).toBe('empty_output');
  });

  it('should not skip on empty_output when output has content', () => {
    const result = evaluateSkipCondition('empty_output', simpleContext);
    expect(result.should_skip).toBe(false);
  });

  it('should skip on simple_output when output is trivial', () => {
    const result = evaluateSkipCondition('simple_output', simpleContext);
    expect(result.should_skip).toBe(true);
    expect(result.skip_reason).toBe('simple_output');
  });

  it('should not skip on simple_output when output is complex', () => {
    const result = evaluateSkipCondition('simple_output', complexContext);
    expect(result.should_skip).toBe(false);
  });

  it('should skip on no_code_changes when no files changed', () => {
    const result = evaluateSkipCondition('no_code_changes', simpleContext);
    expect(result.should_skip).toBe(true);
    expect(result.skip_reason).toBe('no_code_changes');
  });

  it('should not skip on no_code_changes when files changed', () => {
    const result = evaluateSkipCondition('no_code_changes', complexContext);
    expect(result.should_skip).toBe(false);
  });

  it('should always skip when condition is true', () => {
    const result = evaluateSkipCondition(true, complexContext);
    expect(result.should_skip).toBe(true);
    expect(result.skip_reason).toBe('always_skip');
  });
});

describe('shouldSkipReviewer', () => {
  const defaultBehaviors: BehaviorConfig = {
    skip_reviewer_on_empty: false,
    skip_judge_on_simple: false,
    judge_on_failure: false,
    auto_retry_on_rate_limit: true,
  };

  const emptyContext: SkipEvaluationContext = {
    worker_output: '',
    changed_files: [],
  };

  const simpleContext: SkipEvaluationContext = {
    worker_output: 'Done.',
    changed_files: [],
  };

  it('should skip when phase-level skip: true is set', () => {
    const reviewerConfig: ReviewerConfig = { skip: true };
    const result = shouldSkipReviewer(reviewerConfig, defaultBehaviors, simpleContext);
    expect(result.should_skip).toBe(true);
    expect(result.skip_reason).toBe('phase_config');
  });

  it('should skip based on phase-level skip_on condition', () => {
    const reviewerConfig: ReviewerConfig = { skip_on: 'empty_output' };
    const result = shouldSkipReviewer(reviewerConfig, defaultBehaviors, emptyContext);
    expect(result.should_skip).toBe(true);
    expect(result.skip_reason).toBe('empty_output');
  });

  it('should not skip when phase-level skip_on condition is not met', () => {
    const reviewerConfig: ReviewerConfig = { skip_on: 'empty_output' };
    const result = shouldSkipReviewer(reviewerConfig, defaultBehaviors, simpleContext);
    expect(result.should_skip).toBe(false);
  });

  it('should skip based on workflow-level skip_reviewer_on_empty behavior', () => {
    const behaviors: BehaviorConfig = { ...defaultBehaviors, skip_reviewer_on_empty: true };
    const result = shouldSkipReviewer(undefined, behaviors, emptyContext);
    expect(result.should_skip).toBe(true);
    expect(result.skip_reason).toBe('empty_output');
  });

  it('should not skip when no conditions are met', () => {
    const result = shouldSkipReviewer(undefined, defaultBehaviors, simpleContext);
    expect(result.should_skip).toBe(false);
  });

  it('should prioritize phase-level skip: true over other conditions', () => {
    const reviewerConfig: ReviewerConfig = { skip: true, skip_on: 'no_code_changes' };
    const result = shouldSkipReviewer(reviewerConfig, defaultBehaviors, simpleContext);
    expect(result.should_skip).toBe(true);
    expect(result.skip_reason).toBe('phase_config');
  });
});

describe('shouldSkipJudge', () => {
  const defaultBehaviors: BehaviorConfig = {
    skip_reviewer_on_empty: false,
    skip_judge_on_simple: false,
    judge_on_failure: false,
    auto_retry_on_rate_limit: true,
  };

  const simpleContext: SkipEvaluationContext = {
    worker_output: 'Done.',
    changed_files: [],
  };

  const complexContext: SkipEvaluationContext = {
    worker_output: 'Line 1\nLine 2\nLine 3\nLine 4\nLine 5\nLine 6',
    changed_files: ['file1.ts', 'file2.ts'],
  };

  it('should skip when phase-level skip: true is set', () => {
    const judgeConfig: JudgeConfig = { skip: true };
    const result = shouldSkipJudge(judgeConfig, defaultBehaviors, complexContext);
    expect(result.should_skip).toBe(true);
    expect(result.skip_reason).toBe('phase_config');
  });

  it('should skip based on phase-level skip_on condition', () => {
    const judgeConfig: JudgeConfig = { skip_on: 'simple_output' };
    const result = shouldSkipJudge(judgeConfig, defaultBehaviors, simpleContext);
    expect(result.should_skip).toBe(true);
    expect(result.skip_reason).toBe('simple_output');
  });

  it('should not skip when phase-level skip_on condition is not met', () => {
    const judgeConfig: JudgeConfig = { skip_on: 'simple_output' };
    const result = shouldSkipJudge(judgeConfig, defaultBehaviors, complexContext);
    expect(result.should_skip).toBe(false);
  });

  it('should skip based on workflow-level skip_judge_on_simple behavior', () => {
    const behaviors: BehaviorConfig = { ...defaultBehaviors, skip_judge_on_simple: true };
    const result = shouldSkipJudge(undefined, behaviors, simpleContext);
    expect(result.should_skip).toBe(true);
    expect(result.skip_reason).toBe('simple_output');
  });

  it('should not skip when no conditions are met', () => {
    const result = shouldSkipJudge(undefined, defaultBehaviors, complexContext);
    expect(result.should_skip).toBe(false);
  });

  it('should skip on no_code_changes condition', () => {
    const judgeConfig: JudgeConfig = { skip_on: 'no_code_changes' };
    const result = shouldSkipJudge(judgeConfig, defaultBehaviors, simpleContext);
    expect(result.should_skip).toBe(true);
    expect(result.skip_reason).toBe('no_code_changes');
  });
});
