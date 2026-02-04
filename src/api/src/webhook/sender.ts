import { signPayload } from './signer.js';
import type {
  WebhookConfig,
  WebhookPayload,
  WebhookDeliveryResult,
  WebhookSenderOptions,
} from './types.js';

/** Default maximum retry attempts */
const DEFAULT_MAX_RETRIES = 3;

/** Default exponential backoff delays in milliseconds */
const DEFAULT_BACKOFF_MS = [1000, 2000, 4000];

/**
 * Sleep for specified milliseconds
 */
function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/**
 * Delivers a webhook payload to the configured URL
 *
 * @param config - Webhook configuration (URL and secret)
 * @param payload - The webhook payload to deliver
 * @param fetchFn - Optional fetch function for testing
 * @returns Delivery result with success status and response info
 */
export async function deliverWebhook(
  config: WebhookConfig,
  payload: WebhookPayload,
  fetchFn: typeof globalThis.fetch = globalThis.fetch
): Promise<WebhookDeliveryResult> {
  const body = JSON.stringify(payload);
  const signature = signPayload(body, config.secret);

  try {
    const response = await fetchFn(config.url, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Agentium-Signature': signature,
      },
      body,
    });

    return {
      success: response.ok,
      status_code: response.status,
      attempts: 1,
    };
  } catch (error) {
    return {
      success: false,
      attempts: 1,
      error: error instanceof Error ? error.message : 'Unknown error',
    };
  }
}

/**
 * Delivers a webhook with retry logic and exponential backoff
 *
 * @param config - Webhook configuration
 * @param payload - The webhook payload to deliver
 * @param options - Retry and backoff options
 * @returns Delivery result with total attempts made
 */
export async function deliverWithRetry(
  config: WebhookConfig,
  payload: WebhookPayload,
  options: WebhookSenderOptions = {}
): Promise<WebhookDeliveryResult> {
  const maxRetries = options.maxRetries ?? DEFAULT_MAX_RETRIES;
  const backoffMs = options.backoffMs ?? DEFAULT_BACKOFF_MS;
  const fetchFn = options.fetch ?? globalThis.fetch;

  let lastError: string | undefined;

  for (let attempt = 1; attempt <= maxRetries; attempt++) {
    const result = await deliverWebhook(config, payload, fetchFn);

    if (result.success) {
      return { ...result, attempts: attempt };
    }

    lastError = result.error ?? `HTTP ${result.status_code}`;

    // Don't sleep after the last attempt
    if (attempt < maxRetries) {
      const delay = backoffMs[attempt - 1] ?? backoffMs[backoffMs.length - 1];
      await sleep(delay);
    }
  }

  return {
    success: false,
    attempts: maxRetries,
    error: lastError ?? 'Max retries exceeded',
  };
}

/**
 * WebhookSender class for delivering webhooks with configuration
 */
export class WebhookSender {
  private config: WebhookConfig;
  private options: WebhookSenderOptions;

  constructor(config: WebhookConfig, options: WebhookSenderOptions = {}) {
    this.config = config;
    this.options = options;
  }

  /**
   * Delivers a webhook payload with retry logic
   */
  async deliver(payload: WebhookPayload): Promise<WebhookDeliveryResult> {
    return deliverWithRetry(this.config, payload, this.options);
  }

  /**
   * Delivers a webhook payload without retry
   */
  async deliverOnce(payload: WebhookPayload): Promise<WebhookDeliveryResult> {
    const fetchFn = this.options.fetch ?? globalThis.fetch;
    return deliverWebhook(this.config, payload, fetchFn);
  }
}

/**
 * Factory function to create a WebhookSender
 */
export function createWebhookSender(
  config: WebhookConfig,
  options: WebhookSenderOptions = {}
): WebhookSender {
  return new WebhookSender(config, options);
}
