import { describe, it, expect } from 'vitest';
import {
  TokenMetricsCollector,
  createTokenMetricsCollector,
} from '../collector.js';
import { MetricsAggregator, createMetricsAggregator } from '../aggregator.js';
import { getModelPricing, MODEL_PRICING, DEFAULT_PRICING } from '../pricing.js';
import type { TokenMetrics, LLMUsageResponse } from '../types.js';

describe('TokenMetricsCollector', () => {
  let collector: TokenMetricsCollector;

  beforeEach(() => {
    collector = createTokenMetricsCollector();
  });

  describe('collect', () => {
    it('should collect metrics from LLM response', () => {
      const response: LLMUsageResponse = {
        usage: { input_tokens: 1000, output_tokens: 200 },
        model: 'claude-sonnet-4-20250514',
      };

      const metrics = collector.collect(response);

      expect(metrics.input_tokens).toBe(1000);
      expect(metrics.output_tokens).toBe(200);
      expect(metrics.model).toBe('claude-sonnet-4-20250514');
    });

    it('should handle large token counts', () => {
      const response: LLMUsageResponse = {
        usage: { input_tokens: 1_000_000, output_tokens: 500_000 },
        model: 'claude-opus-4-5-20251101',
      };

      const metrics = collector.collect(response);

      expect(metrics.input_tokens).toBe(1_000_000);
      expect(metrics.output_tokens).toBe(500_000);
    });
  });

  describe('collectFromUsage', () => {
    it('should collect metrics from usage object', () => {
      const metrics = collector.collectFromUsage(
        { input_tokens: 500, output_tokens: 100 },
        'claude-haiku-3-5-20241022'
      );

      expect(metrics.input_tokens).toBe(500);
      expect(metrics.output_tokens).toBe(100);
      expect(metrics.model).toBe('claude-haiku-3-5-20241022');
    });

    it('should handle missing model', () => {
      const metrics = collector.collectFromUsage({
        input_tokens: 500,
        output_tokens: 100,
      });

      expect(metrics.input_tokens).toBe(500);
      expect(metrics.output_tokens).toBe(100);
      expect(metrics.model).toBeUndefined();
    });
  });

  describe('empty', () => {
    it('should create empty metrics', () => {
      const metrics = collector.empty();

      expect(metrics.input_tokens).toBe(0);
      expect(metrics.output_tokens).toBe(0);
      expect(metrics.model).toBeUndefined();
    });

    it('should create empty metrics with model', () => {
      const metrics = collector.empty('claude-sonnet-4-20250514');

      expect(metrics.input_tokens).toBe(0);
      expect(metrics.output_tokens).toBe(0);
      expect(metrics.model).toBe('claude-sonnet-4-20250514');
    });
  });
});

describe('MetricsAggregator', () => {
  let aggregator: MetricsAggregator;

  beforeEach(() => {
    aggregator = createMetricsAggregator();
  });

  describe('aggregate', () => {
    it('should aggregate phase metrics', () => {
      const metrics: TokenMetrics[] = [
        { input_tokens: 1000, output_tokens: 100 },
        { input_tokens: 2000, output_tokens: 200 },
      ];

      const total = aggregator.aggregate(metrics);

      expect(total.input_tokens).toBe(3000);
      expect(total.output_tokens).toBe(300);
    });

    it('should handle empty array', () => {
      const total = aggregator.aggregate([]);

      expect(total.input_tokens).toBe(0);
      expect(total.output_tokens).toBe(0);
    });

    it('should aggregate metrics from W/R/J within a phase', () => {
      // Simulating worker, reviewer, judge metrics for one phase
      const workerMetrics: TokenMetrics = { input_tokens: 2500, output_tokens: 400, model: 'claude-sonnet-4-20250514' };
      const reviewerMetrics: TokenMetrics = { input_tokens: 800, output_tokens: 150, model: 'claude-sonnet-4-20250514' };
      const judgeMetrics: TokenMetrics = { input_tokens: 500, output_tokens: 100, model: 'claude-sonnet-4-20250514' };

      const total = aggregator.aggregate([workerMetrics, reviewerMetrics, judgeMetrics]);

      expect(total.input_tokens).toBe(3800);
      expect(total.output_tokens).toBe(650);
    });
  });

  describe('calculateCost', () => {
    it('should calculate estimated cost for Sonnet', () => {
      const metrics: TokenMetrics[] = [
        { input_tokens: 1_000_000, output_tokens: 100_000, model: 'claude-sonnet-4-20250514' },
      ];

      const cost = aggregator.calculateCost(metrics);

      // Input: 1M * $3/1M = $3, Output: 100K * $15/1M = $1.50
      expect(cost).toBe(4.5);
    });

    it('should calculate estimated cost for Opus', () => {
      const metrics: TokenMetrics[] = [
        { input_tokens: 1_000_000, output_tokens: 100_000, model: 'claude-opus-4-5-20251101' },
      ];

      const cost = aggregator.calculateCost(metrics);

      // Input: 1M * $15/1M = $15, Output: 100K * $75/1M = $7.50
      expect(cost).toBe(22.5);
    });

    it('should calculate estimated cost for Haiku', () => {
      const metrics: TokenMetrics[] = [
        { input_tokens: 1_000_000, output_tokens: 100_000, model: 'claude-haiku-3-5-20241022' },
      ];

      const cost = aggregator.calculateCost(metrics);

      // Input: 1M * $0.80/1M = $0.80, Output: 100K * $4/1M = $0.40
      expect(cost).toBeCloseTo(1.2, 2);
    });

    it('should use default pricing for unknown models', () => {
      const metrics: TokenMetrics[] = [
        { input_tokens: 1_000_000, output_tokens: 100_000, model: 'unknown-model' },
      ];

      const cost = aggregator.calculateCost(metrics);

      // Default uses Sonnet pricing: Input: 1M * $3/1M = $3, Output: 100K * $15/1M = $1.50
      expect(cost).toBe(4.5);
    });

    it('should use default pricing when model is undefined', () => {
      const metrics: TokenMetrics[] = [{ input_tokens: 1_000_000, output_tokens: 100_000 }];

      const cost = aggregator.calculateCost(metrics);

      // Default uses Sonnet pricing
      expect(cost).toBe(4.5);
    });

    it('should aggregate costs across multiple metrics', () => {
      const metrics: TokenMetrics[] = [
        { input_tokens: 500_000, output_tokens: 50_000, model: 'claude-sonnet-4-20250514' },
        { input_tokens: 500_000, output_tokens: 50_000, model: 'claude-sonnet-4-20250514' },
      ];

      const cost = aggregator.calculateCost(metrics);

      // Each: Input: 500K * $3/1M = $1.50, Output: 50K * $15/1M = $0.75
      // Total: 2 * ($1.50 + $0.75) = $4.50
      expect(cost).toBe(4.5);
    });
  });

  describe('aggregateWithCost', () => {
    it('should aggregate metrics and calculate cost', () => {
      const metrics: TokenMetrics[] = [
        { input_tokens: 2500, output_tokens: 400, model: 'claude-sonnet-4-20250514' },
        { input_tokens: 800, output_tokens: 150, model: 'claude-sonnet-4-20250514' },
      ];

      const total = aggregator.aggregateWithCost(metrics);

      expect(total.input_tokens).toBe(3300);
      expect(total.output_tokens).toBe(550);
      expect(total.estimated_cost_usd).toBeGreaterThan(0);
    });

    it('should round cost to 2 decimal places', () => {
      const metrics: TokenMetrics[] = [
        { input_tokens: 12345, output_tokens: 6789, model: 'claude-sonnet-4-20250514' },
      ];

      const total = aggregator.aggregateWithCost(metrics);

      // Cost should be rounded to 2 decimal places
      const decimalPlaces = total.estimated_cost_usd.toString().split('.')[1]?.length ?? 0;
      expect(decimalPlaces).toBeLessThanOrEqual(2);
    });
  });

  describe('empty', () => {
    it('should create empty total metrics', () => {
      const total = aggregator.empty();

      expect(total.input_tokens).toBe(0);
      expect(total.output_tokens).toBe(0);
      expect(total.estimated_cost_usd).toBe(0);
    });
  });
});

describe('getModelPricing', () => {
  it('should return pricing for known models', () => {
    const sonnetPricing = getModelPricing('claude-sonnet-4-20250514');
    expect(sonnetPricing).toEqual({ input: 3.0, output: 15.0 });

    const opusPricing = getModelPricing('claude-opus-4-5-20251101');
    expect(opusPricing).toEqual({ input: 15.0, output: 75.0 });

    const haikuPricing = getModelPricing('claude-haiku-3-5-20241022');
    expect(haikuPricing).toEqual({ input: 0.8, output: 4.0 });
  });

  it('should return default pricing for unknown models', () => {
    const pricing = getModelPricing('unknown-model-xyz');
    expect(pricing).toEqual(DEFAULT_PRICING);
  });
});

describe('MODEL_PRICING', () => {
  it('should have pricing for Claude 4.5 Opus', () => {
    expect(MODEL_PRICING['claude-opus-4-5-20251101']).toEqual({ input: 15.0, output: 75.0 });
  });

  it('should have pricing for Claude 4 Sonnet', () => {
    expect(MODEL_PRICING['claude-sonnet-4-20250514']).toEqual({ input: 3.0, output: 15.0 });
  });

  it('should have pricing for Claude 3.5 Haiku', () => {
    expect(MODEL_PRICING['claude-haiku-3-5-20241022']).toEqual({ input: 0.8, output: 4.0 });
  });
});

describe('createTokenMetricsCollector', () => {
  it('should create a collector instance', () => {
    const collector = createTokenMetricsCollector();
    expect(collector).toBeInstanceOf(TokenMetricsCollector);
  });
});

describe('createMetricsAggregator', () => {
  it('should create an aggregator instance', () => {
    const aggregator = createMetricsAggregator();
    expect(aggregator).toBeInstanceOf(MetricsAggregator);
  });
});
