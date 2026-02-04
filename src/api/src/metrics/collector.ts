import type { TokenMetrics, LLMUsageResponse } from './types.js';

/**
 * Token Metrics Collector
 *
 * Extracts token usage metrics from LLM API responses.
 */
export class TokenMetricsCollector {
  /**
   * Collect token metrics from an LLM API response
   *
   * @param response - The LLM API response containing usage information
   * @returns Token metrics extracted from the response
   */
  collect(response: LLMUsageResponse): TokenMetrics {
    return {
      input_tokens: response.usage.input_tokens,
      output_tokens: response.usage.output_tokens,
      model: response.model,
    };
  }

  /**
   * Collect token metrics from a partial response (when model may not be present)
   *
   * @param usage - The usage object with token counts
   * @param model - Optional model name
   * @returns Token metrics
   */
  collectFromUsage(
    usage: { input_tokens: number; output_tokens: number },
    model?: string
  ): TokenMetrics {
    return {
      input_tokens: usage.input_tokens,
      output_tokens: usage.output_tokens,
      model,
    };
  }

  /**
   * Create empty/zero token metrics
   *
   * @param model - Optional model name
   * @returns Token metrics with zero counts
   */
  empty(model?: string): TokenMetrics {
    return {
      input_tokens: 0,
      output_tokens: 0,
      model,
    };
  }
}

/**
 * Create a new token metrics collector instance
 */
export function createTokenMetricsCollector(): TokenMetricsCollector {
  return new TokenMetricsCollector();
}
