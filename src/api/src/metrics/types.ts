/**
 * Token metrics for a single LLM API call
 */
export interface TokenMetrics {
  input_tokens: number;
  output_tokens: number;
  model?: string;
}

/**
 * Aggregated token metrics with cost
 */
export interface TotalTokenMetrics {
  input_tokens: number;
  output_tokens: number;
  estimated_cost_usd: number;
}

/**
 * Token metrics from an LLM API response
 */
export interface LLMUsageResponse {
  usage: {
    input_tokens: number;
    output_tokens: number;
  };
  model: string;
}

/**
 * Model pricing (per 1M tokens)
 */
export interface ModelPricing {
  input: number;
  output: number;
}

/**
 * Price table for supported models
 */
export type ModelPricingTable = Record<string, ModelPricing>;
