/**
 * Token metrics for tracking API usage
 */
export interface TokenMetrics {
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
}

/**
 * Skip conditions for phase-level configuration
 * - 'empty_output': Skip if WORKER produced no output
 * - 'simple_output': Skip if output is trivial (few lines)
 * - 'no_code_changes': Skip if no files were modified
 * - true: Always skip
 */
export type SkipCondition = 'empty_output' | 'simple_output' | 'no_code_changes' | true;

/**
 * Worker result from executing a phase
 */
export interface WorkerResult {
  status: 'completed' | 'failed';
  output?: string;
  token_metrics?: TokenMetrics;
  error?: string;
}

/**
 * Reviewer result from critiquing worker output
 */
export interface ReviewerResult {
  status: 'completed' | 'skipped';
  skip_reason?: string;
  feedback?: string;
  token_metrics?: TokenMetrics;
}

/**
 * Judge result from evaluating phase output
 */
export interface JudgeResult {
  status: 'completed' | 'skipped';
  skip_reason?: string;
  passed?: boolean;
  reasoning?: string;
  token_metrics?: TokenMetrics;
}

/**
 * Result of executing a single phase
 */
export interface PhaseResult {
  name: string;
  status: 'completed' | 'failed';
  duration_ms: number;
  worker: WorkerResult;
  reviewer: ReviewerResult;
  judge: JudgeResult;
}

/**
 * Behavior configuration for session execution
 */
export interface BehaviorConfig {
  skip_reviewer_on_empty?: boolean;
  skip_judge_on_simple?: boolean;
  judge_on_failure?: boolean;
  auto_retry_on_rate_limit?: boolean;
}

/**
 * Repository configuration
 */
export interface RepositoryConfig {
  url: string;
  branch: string;
}

/**
 * Prompt context configuration
 */
export interface PromptContext {
  issue_url?: string;
  instructions?: string;
}

/**
 * Session context that accumulates state across phases
 */
export interface SessionContext {
  task_id: string;
  repository: RepositoryConfig;
  prompt_context: PromptContext;
  behaviors: BehaviorConfig;
  // Accumulated state
  file_tree?: string;
  changed_files?: string[];
  git_diff?: string;
  phase_outputs: Record<string, string>;
}

/**
 * Reviewer configuration for a phase
 */
export interface ReviewerConfig {
  skip?: boolean;
  skip_on?: SkipCondition;
  prompt?: string;
}

/**
 * Judge configuration for a phase
 */
export interface JudgeConfig {
  skip?: boolean;
  skip_on?: SkipCondition;
  criteria?: string;
}

/**
 * Phase definition in a workflow
 */
export interface PhaseDefinition {
  name: string;
  max_iterations?: number;
  worker_prompt: string;
  reviewer_prompt: string;
  judge_prompt: string;
  // Optional phase-level configurations
  reviewer?: ReviewerConfig;
  judge?: JudgeConfig;
}

/**
 * Workflow definition
 */
export interface WorkflowDefinition {
  name: string;
  phases: PhaseDefinition[];
}

/**
 * Session execution request
 */
export interface SessionRequest {
  task_id?: string;
  workflow?: 'default';
  phases?: PhaseDefinition[];
  repository: RepositoryConfig;
  prompt_context?: PromptContext;
  behaviors?: BehaviorConfig;
}

/**
 * Session execution result
 */
export interface SessionResult {
  task_id: string;
  status: 'completed' | 'failed';
  duration_ms: number;
  phases: PhaseResult[];
  behaviors_applied: BehaviorConfig;
}

/**
 * Adapter interface for executing agent invocations
 * This will be implemented by different agent adapters (Claude Code, Aider, etc.)
 */
export interface AgentAdapter {
  invoke(prompt: string): Promise<{ output: string; token_metrics?: TokenMetrics }>;
}
