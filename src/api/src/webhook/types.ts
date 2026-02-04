import type { PhaseResult, BehaviorConfig } from '../session/types.js';
import type { TotalTokenMetrics } from '../metrics/types.js';

/**
 * Configuration for webhook delivery
 */
export interface WebhookConfig {
  url: string;
  secret: string;
}

/**
 * GitHub issue reference in webhook outputs
 */
export interface GitHubIssueOutput {
  number: number;
  url: string;
}

/**
 * Outputs section of webhook payload
 */
export interface WebhookOutputs {
  github_issues?: GitHubIssueOutput[];
}

/**
 * Judge result summary for webhook payload
 */
export interface WebhookJudgeResult {
  passed: boolean;
  reasoning: string;
}

/**
 * Webhook payload sent on task completion or failure
 */
export interface WebhookPayload {
  task_id: string;
  status: 'completed' | 'failed';
  workflow?: 'default';
  phases: PhaseResult[];
  judge_result?: WebhookJudgeResult;
  outputs?: WebhookOutputs;
  total_token_metrics: TotalTokenMetrics;
  behaviors_applied: BehaviorConfig;
  error?: string; // If status is 'failed'
}

/**
 * Result of webhook delivery attempt
 */
export interface WebhookDeliveryResult {
  success: boolean;
  status_code?: number;
  attempts: number;
  error?: string;
}

/**
 * Options for configuring webhook sender behavior
 */
export interface WebhookSenderOptions {
  /** Maximum number of retry attempts (default: 3) */
  maxRetries?: number;
  /** Backoff delays in milliseconds for each retry (default: [1000, 2000, 4000]) */
  backoffMs?: number[];
  /** Custom fetch implementation for testing */
  fetch?: typeof globalThis.fetch;
}
