import type { TokenMetrics, TotalTokenMetrics } from './types.js';
import { getModelPricing, DEFAULT_PRICING } from './pricing.js';

/**
 * Metrics Aggregator
 *
 * Aggregates token metrics across phases and calculates estimated costs.
 */
export class MetricsAggregator {
  /**
   * Aggregate multiple token metrics into totals
   *
   * @param metrics - Array of token metrics to aggregate
   * @returns Aggregated token counts (without cost)
   */
  aggregate(metrics: TokenMetrics[]): { input_tokens: number; output_tokens: number } {
    return metrics.reduce(
      (acc, m) => ({
        input_tokens: acc.input_tokens + m.input_tokens,
        output_tokens: acc.output_tokens + m.output_tokens,
      }),
      { input_tokens: 0, output_tokens: 0 }
    );
  }

  /**
   * Calculate the estimated cost for a set of token metrics
   *
   * @param metrics - Array of token metrics with model information
   * @returns Estimated cost in USD
   */
  calculateCost(metrics: TokenMetrics[]): number {
    return metrics.reduce((total, m) => {
      const pricing = m.model ? getModelPricing(m.model) : DEFAULT_PRICING;
      const inputCost = (m.input_tokens / 1_000_000) * pricing.input;
      const outputCost = (m.output_tokens / 1_000_000) * pricing.output;
      return total + inputCost + outputCost;
    }, 0);
  }

  /**
   * Aggregate metrics and calculate cost in one call
   *
   * @param metrics - Array of token metrics to aggregate
   * @returns Total token metrics with estimated cost
   */
  aggregateWithCost(metrics: TokenMetrics[]): TotalTokenMetrics {
    const { input_tokens, output_tokens } = this.aggregate(metrics);
    const estimated_cost_usd = this.calculateCost(metrics);

    return {
      input_tokens,
      output_tokens,
      estimated_cost_usd: Math.round(estimated_cost_usd * 100) / 100, // Round to 2 decimal places
    };
  }

  /**
   * Create empty total token metrics
   *
   * @returns Total token metrics with zero values
   */
  empty(): TotalTokenMetrics {
    return {
      input_tokens: 0,
      output_tokens: 0,
      estimated_cost_usd: 0,
    };
  }
}

/**
 * Create a new metrics aggregator instance
 */
export function createMetricsAggregator(): MetricsAggregator {
  return new MetricsAggregator();
}
