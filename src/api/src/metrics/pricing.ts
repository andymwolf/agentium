import type { ModelPricingTable } from './types.js';

/**
 * Model pricing (per 1M tokens) for Claude models
 * Source: https://www.anthropic.com/pricing
 */
export const MODEL_PRICING: ModelPricingTable = {
  // Claude 4.5 Opus
  'claude-opus-4-5-20251101': { input: 15.0, output: 75.0 },

  // Claude 4 Sonnet
  'claude-sonnet-4-20250514': { input: 3.0, output: 15.0 },

  // Claude 3.5 Haiku
  'claude-haiku-3-5-20241022': { input: 0.8, output: 4.0 },

  // Claude 3.5 Sonnet (legacy)
  'claude-3-5-sonnet-20241022': { input: 3.0, output: 15.0 },
  'claude-3-5-sonnet-20240620': { input: 3.0, output: 15.0 },

  // Claude 3 models (legacy)
  'claude-3-opus-20240229': { input: 15.0, output: 75.0 },
  'claude-3-sonnet-20240229': { input: 3.0, output: 15.0 },
  'claude-3-haiku-20240307': { input: 0.25, output: 1.25 },
};

/**
 * Default pricing to use when model is unknown
 * Uses Claude Sonnet pricing as a reasonable default
 */
export const DEFAULT_PRICING = { input: 3.0, output: 15.0 };

/**
 * Get pricing for a specific model
 *
 * @param model - The model name
 * @returns Pricing for the model, or default pricing if not found
 */
export function getModelPricing(model: string): { input: number; output: number } {
  return MODEL_PRICING[model] ?? DEFAULT_PRICING;
}
