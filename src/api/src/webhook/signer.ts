import { createHmac } from 'crypto';

/**
 * Signs a webhook payload using HMAC-SHA256
 *
 * @param payload - The JSON string payload to sign
 * @param secret - The secret key for HMAC signing
 * @returns The signature in format "sha256=<hex>"
 */
export function signPayload(payload: string, secret: string): string {
  const hmac = createHmac('sha256', secret);
  hmac.update(payload);
  return 'sha256=' + hmac.digest('hex');
}

/**
 * Verifies a webhook payload signature
 *
 * @param payload - The JSON string payload
 * @param secret - The secret key for HMAC signing
 * @param signature - The signature to verify (format: "sha256=<hex>")
 * @returns True if signature is valid, false otherwise
 */
export function verifySignature(
  payload: string,
  secret: string,
  signature: string
): boolean {
  const expected = signPayload(payload, secret);
  // Constant-time comparison to prevent timing attacks
  if (expected.length !== signature.length) {
    return false;
  }
  let result = 0;
  for (let i = 0; i < expected.length; i++) {
    result |= expected.charCodeAt(i) ^ signature.charCodeAt(i);
  }
  return result === 0;
}
