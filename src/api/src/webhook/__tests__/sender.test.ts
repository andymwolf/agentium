import { describe, it, expect, vi, beforeEach } from 'vitest';
import {
  deliverWebhook,
  deliverWithRetry,
  WebhookSender,
  createWebhookSender,
} from '../sender.js';
import { signPayload } from '../signer.js';
import type {
  WebhookConfig,
  WebhookPayload,
  WebhookDeliveryResult,
} from '../types.js';

// Create a mock payload for testing
function createTestPayload(): WebhookPayload {
  return {
    task_id: 'test-task-123',
    status: 'completed',
    workflow: 'default',
    phases: [
      {
        name: 'implement',
        status: 'completed',
        duration_ms: 5000,
        worker: {
          status: 'completed',
          output: 'Implementation complete',
          token_metrics: { input_tokens: 1000, output_tokens: 200, total_tokens: 1200 },
        },
        reviewer: {
          status: 'completed',
          feedback: 'Looks good',
          token_metrics: { input_tokens: 500, output_tokens: 100, total_tokens: 600 },
        },
        judge: {
          status: 'completed',
          passed: true,
          reasoning: 'All criteria met',
          token_metrics: { input_tokens: 300, output_tokens: 50, total_tokens: 350 },
        },
      },
    ],
    total_token_metrics: {
      input_tokens: 1800,
      output_tokens: 350,
      estimated_cost_usd: 0.05,
    },
    behaviors_applied: {
      skip_reviewer_on_empty: false,
      skip_judge_on_simple: false,
    },
  };
}

function createTestConfig(): WebhookConfig {
  return {
    url: 'https://webhook.example.com/callback',
    secret: 'test-secret-123',
  };
}

describe('deliverWebhook', () => {
  it('should include X-Agentium-Signature header', async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
    });

    const config = createTestConfig();
    const payload = createTestPayload();

    await deliverWebhook(config, payload, mockFetch);

    expect(mockFetch).toHaveBeenCalledWith(
      config.url,
      expect.objectContaining({
        method: 'POST',
        headers: expect.objectContaining({
          'X-Agentium-Signature': expect.stringMatching(/^sha256=[a-f0-9]{64}$/),
        }),
      })
    );
  });

  it('should include Content-Type header', async () => {
    const mockFetch = vi.fn().mockResolvedValue({ ok: true, status: 200 });
    const config = createTestConfig();
    const payload = createTestPayload();

    await deliverWebhook(config, payload, mockFetch);

    expect(mockFetch).toHaveBeenCalledWith(
      config.url,
      expect.objectContaining({
        headers: expect.objectContaining({
          'Content-Type': 'application/json',
        }),
      })
    );
  });

  it('should send correct signature for payload', async () => {
    const mockFetch = vi.fn().mockResolvedValue({ ok: true, status: 200 });
    const config = createTestConfig();
    const payload = createTestPayload();

    await deliverWebhook(config, payload, mockFetch);

    const expectedBody = JSON.stringify(payload);
    const expectedSignature = signPayload(expectedBody, config.secret);

    expect(mockFetch).toHaveBeenCalledWith(
      config.url,
      expect.objectContaining({
        body: expectedBody,
        headers: expect.objectContaining({
          'X-Agentium-Signature': expectedSignature,
        }),
      })
    );
  });

  it('should return success for 2xx response', async () => {
    const mockFetch = vi.fn().mockResolvedValue({ ok: true, status: 200 });
    const config = createTestConfig();
    const payload = createTestPayload();

    const result = await deliverWebhook(config, payload, mockFetch);

    expect(result.success).toBe(true);
    expect(result.status_code).toBe(200);
    expect(result.attempts).toBe(1);
    expect(result.error).toBeUndefined();
  });

  it('should return failure for 4xx response', async () => {
    const mockFetch = vi.fn().mockResolvedValue({ ok: false, status: 400 });
    const config = createTestConfig();
    const payload = createTestPayload();

    const result = await deliverWebhook(config, payload, mockFetch);

    expect(result.success).toBe(false);
    expect(result.status_code).toBe(400);
    expect(result.attempts).toBe(1);
  });

  it('should return failure for 5xx response', async () => {
    const mockFetch = vi.fn().mockResolvedValue({ ok: false, status: 500 });
    const config = createTestConfig();
    const payload = createTestPayload();

    const result = await deliverWebhook(config, payload, mockFetch);

    expect(result.success).toBe(false);
    expect(result.status_code).toBe(500);
  });

  it('should handle network errors', async () => {
    const mockFetch = vi.fn().mockRejectedValue(new Error('Network error'));
    const config = createTestConfig();
    const payload = createTestPayload();

    const result = await deliverWebhook(config, payload, mockFetch);

    expect(result.success).toBe(false);
    expect(result.error).toBe('Network error');
    expect(result.attempts).toBe(1);
    expect(result.status_code).toBeUndefined();
  });

  it('should handle unknown errors', async () => {
    const mockFetch = vi.fn().mockRejectedValue('string error');
    const config = createTestConfig();
    const payload = createTestPayload();

    const result = await deliverWebhook(config, payload, mockFetch);

    expect(result.success).toBe(false);
    expect(result.error).toBe('Unknown error');
  });
});

describe('deliverWithRetry', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('should succeed on first attempt', async () => {
    const mockFetch = vi.fn().mockResolvedValue({ ok: true, status: 200 });
    const config = createTestConfig();
    const payload = createTestPayload();

    const resultPromise = deliverWithRetry(config, payload, { fetch: mockFetch });
    await vi.runAllTimersAsync();
    const result = await resultPromise;

    expect(result.success).toBe(true);
    expect(result.attempts).toBe(1);
    expect(mockFetch).toHaveBeenCalledTimes(1);
  });

  it('should retry on failure and succeed', async () => {
    const mockFetch = vi
      .fn()
      .mockResolvedValueOnce({ ok: false, status: 500 })
      .mockResolvedValueOnce({ ok: false, status: 500 })
      .mockResolvedValueOnce({ ok: true, status: 200 });

    const config = createTestConfig();
    const payload = createTestPayload();

    const resultPromise = deliverWithRetry(config, payload, {
      fetch: mockFetch,
      backoffMs: [10, 20, 40], // Fast backoff for testing
    });
    await vi.runAllTimersAsync();
    const result = await resultPromise;

    expect(result.success).toBe(true);
    expect(result.attempts).toBe(3);
    expect(mockFetch).toHaveBeenCalledTimes(3);
  });

  it('should retry on network error', async () => {
    const mockFetch = vi
      .fn()
      .mockRejectedValueOnce(new Error('Network error'))
      .mockRejectedValueOnce(new Error('Network error'))
      .mockResolvedValueOnce({ ok: true, status: 200 });

    const config = createTestConfig();
    const payload = createTestPayload();

    const resultPromise = deliverWithRetry(config, payload, {
      fetch: mockFetch,
      backoffMs: [10, 20, 40],
    });
    await vi.runAllTimersAsync();
    const result = await resultPromise;

    expect(result.success).toBe(true);
    expect(result.attempts).toBe(3);
  });

  it('should fail after max retries exceeded', async () => {
    const mockFetch = vi.fn().mockResolvedValue({ ok: false, status: 500 });

    const config = createTestConfig();
    const payload = createTestPayload();

    const resultPromise = deliverWithRetry(config, payload, {
      fetch: mockFetch,
      maxRetries: 3,
      backoffMs: [10, 20, 40],
    });
    await vi.runAllTimersAsync();
    const result = await resultPromise;

    expect(result.success).toBe(false);
    expect(result.attempts).toBe(3);
    expect(result.error).toBe('HTTP 500');
    expect(mockFetch).toHaveBeenCalledTimes(3);
  });

  it('should use default options when not specified', async () => {
    const mockFetch = vi.fn().mockResolvedValue({ ok: true, status: 200 });
    const config = createTestConfig();
    const payload = createTestPayload();

    const resultPromise = deliverWithRetry(config, payload, { fetch: mockFetch });
    await vi.runAllTimersAsync();
    const result = await resultPromise;

    expect(result.success).toBe(true);
    expect(result.attempts).toBe(1);
  });

  it('should respect custom maxRetries', async () => {
    const mockFetch = vi.fn().mockResolvedValue({ ok: false, status: 500 });
    const config = createTestConfig();
    const payload = createTestPayload();

    const resultPromise = deliverWithRetry(config, payload, {
      fetch: mockFetch,
      maxRetries: 5,
      backoffMs: [10, 20, 40, 80, 160],
    });
    await vi.runAllTimersAsync();
    const result = await resultPromise;

    expect(result.attempts).toBe(5);
    expect(mockFetch).toHaveBeenCalledTimes(5);
  });

  it('should include last error in result', async () => {
    const mockFetch = vi.fn().mockRejectedValue(new Error('Connection refused'));
    const config = createTestConfig();
    const payload = createTestPayload();

    const resultPromise = deliverWithRetry(config, payload, {
      fetch: mockFetch,
      maxRetries: 2,
      backoffMs: [10, 20],
    });
    await vi.runAllTimersAsync();
    const result = await resultPromise;

    expect(result.success).toBe(false);
    expect(result.error).toBe('Connection refused');
  });
});

describe('WebhookSender', () => {
  it('should deliver with retry by default', async () => {
    vi.useFakeTimers();

    const mockFetch = vi
      .fn()
      .mockResolvedValueOnce({ ok: false, status: 500 })
      .mockResolvedValueOnce({ ok: true, status: 200 });

    const sender = new WebhookSender(createTestConfig(), {
      fetch: mockFetch,
      backoffMs: [10],
    });

    const resultPromise = sender.deliver(createTestPayload());
    await vi.runAllTimersAsync();
    const result = await resultPromise;

    expect(result.success).toBe(true);
    expect(result.attempts).toBe(2);

    vi.useRealTimers();
  });

  it('should deliver once without retry', async () => {
    const mockFetch = vi.fn().mockResolvedValue({ ok: false, status: 500 });

    const sender = new WebhookSender(createTestConfig(), { fetch: mockFetch });

    const result = await sender.deliverOnce(createTestPayload());

    expect(result.success).toBe(false);
    expect(result.attempts).toBe(1);
    expect(mockFetch).toHaveBeenCalledTimes(1);
  });
});

describe('createWebhookSender', () => {
  it('should create a WebhookSender instance', () => {
    const sender = createWebhookSender(createTestConfig());
    expect(sender).toBeInstanceOf(WebhookSender);
  });

  it('should accept custom options', () => {
    const mockFetch = vi.fn().mockResolvedValue({ ok: true, status: 200 });
    const sender = createWebhookSender(createTestConfig(), {
      fetch: mockFetch,
      maxRetries: 5,
    });

    expect(sender).toBeInstanceOf(WebhookSender);
  });
});

describe('Webhook payload with error', () => {
  it('should deliver failed task payload with error field', async () => {
    const mockFetch = vi.fn().mockResolvedValue({ ok: true, status: 200 });
    const config = createTestConfig();

    const payload: WebhookPayload = {
      task_id: 'test-task-failed',
      status: 'failed',
      phases: [],
      total_token_metrics: {
        input_tokens: 500,
        output_tokens: 100,
        estimated_cost_usd: 0.01,
      },
      behaviors_applied: {},
      error: 'Worker failed to complete implementation',
    };

    const result = await deliverWebhook(config, payload, mockFetch);

    expect(result.success).toBe(true);

    const [, callOptions] = mockFetch.mock.calls[0];
    const sentPayload = JSON.parse(callOptions.body);

    expect(sentPayload.status).toBe('failed');
    expect(sentPayload.error).toBe('Worker failed to complete implementation');
  });
});

describe('Webhook payload with outputs', () => {
  it('should deliver payload with GitHub issues output', async () => {
    const mockFetch = vi.fn().mockResolvedValue({ ok: true, status: 200 });
    const config = createTestConfig();

    const payload: WebhookPayload = {
      ...createTestPayload(),
      outputs: {
        github_issues: [
          { number: 123, url: 'https://github.com/org/repo/issues/123' },
          { number: 124, url: 'https://github.com/org/repo/issues/124' },
        ],
      },
    };

    const result = await deliverWebhook(config, payload, mockFetch);

    expect(result.success).toBe(true);

    const [, callOptions] = mockFetch.mock.calls[0];
    const sentPayload = JSON.parse(callOptions.body);

    expect(sentPayload.outputs.github_issues).toHaveLength(2);
    expect(sentPayload.outputs.github_issues[0].number).toBe(123);
  });
});

describe('Webhook payload with judge_result', () => {
  it('should deliver payload with judge result', async () => {
    const mockFetch = vi.fn().mockResolvedValue({ ok: true, status: 200 });
    const config = createTestConfig();

    const payload: WebhookPayload = {
      ...createTestPayload(),
      judge_result: {
        passed: true,
        reasoning: 'All acceptance criteria met',
      },
    };

    const result = await deliverWebhook(config, payload, mockFetch);

    expect(result.success).toBe(true);

    const [, callOptions] = mockFetch.mock.calls[0];
    const sentPayload = JSON.parse(callOptions.body);

    expect(sentPayload.judge_result.passed).toBe(true);
    expect(sentPayload.judge_result.reasoning).toBe('All acceptance criteria met');
  });
});
