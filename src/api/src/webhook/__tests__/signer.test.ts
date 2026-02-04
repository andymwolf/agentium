import { describe, it, expect } from 'vitest';
import { signPayload, verifySignature } from '../signer.js';

describe('signPayload', () => {
  it('should generate valid HMAC-SHA256 signature format', () => {
    const payload = '{"task_id": "test"}';
    const secret = 'my-secret';
    const signature = signPayload(payload, secret);

    // Signature should match format sha256=<64 hex chars>
    expect(signature).toMatch(/^sha256=[a-f0-9]{64}$/);
  });

  it('should generate deterministic signatures', () => {
    const payload = '{"task_id": "test"}';
    const secret = 'my-secret';

    const signature1 = signPayload(payload, secret);
    const signature2 = signPayload(payload, secret);

    expect(signature1).toBe(signature2);
  });

  it('should generate different signatures for different payloads', () => {
    const secret = 'my-secret';

    const signature1 = signPayload('{"task_id": "test1"}', secret);
    const signature2 = signPayload('{"task_id": "test2"}', secret);

    expect(signature1).not.toBe(signature2);
  });

  it('should generate different signatures for different secrets', () => {
    const payload = '{"task_id": "test"}';

    const signature1 = signPayload(payload, 'secret-1');
    const signature2 = signPayload(payload, 'secret-2');

    expect(signature1).not.toBe(signature2);
  });

  it('should handle empty payload', () => {
    const signature = signPayload('', 'secret');
    expect(signature).toMatch(/^sha256=[a-f0-9]{64}$/);
  });

  it('should handle empty secret', () => {
    const signature = signPayload('{"test": true}', '');
    expect(signature).toMatch(/^sha256=[a-f0-9]{64}$/);
  });

  it('should handle unicode characters in payload', () => {
    const payload = '{"message": "Hello, 世界!"}';
    const signature = signPayload(payload, 'secret');
    expect(signature).toMatch(/^sha256=[a-f0-9]{64}$/);
  });

  it('should handle special characters in secret', () => {
    const payload = '{"test": true}';
    const signature = signPayload(payload, 'secret!@#$%^&*()');
    expect(signature).toMatch(/^sha256=[a-f0-9]{64}$/);
  });

  it('should match known HMAC-SHA256 output', () => {
    // Known test vector: HMAC-SHA256 of 'test' with secret 'key'
    // We can verify this with: echo -n 'test' | openssl dgst -sha256 -hmac 'key'
    const payload = 'test';
    const secret = 'key';
    const signature = signPayload(payload, secret);

    // Expected: 02afb56304902c656fcb737cdd03de6205bb6d401da2812efd9b2d36a08af159
    expect(signature).toBe(
      'sha256=02afb56304902c656fcb737cdd03de6205bb6d401da2812efd9b2d36a08af159'
    );
  });
});

describe('verifySignature', () => {
  const payload = '{"task_id": "test-123"}';
  const secret = 'webhook-secret';

  it('should return true for valid signature', () => {
    const signature = signPayload(payload, secret);
    expect(verifySignature(payload, secret, signature)).toBe(true);
  });

  it('should return false for invalid signature', () => {
    expect(
      verifySignature(payload, secret, 'sha256=invalid')
    ).toBe(false);
  });

  it('should return false for modified payload', () => {
    const signature = signPayload(payload, secret);
    const modifiedPayload = '{"task_id": "test-456"}';
    expect(verifySignature(modifiedPayload, secret, signature)).toBe(false);
  });

  it('should return false for wrong secret', () => {
    const signature = signPayload(payload, secret);
    expect(verifySignature(payload, 'wrong-secret', signature)).toBe(false);
  });

  it('should return false for malformed signature', () => {
    expect(verifySignature(payload, secret, 'not-sha256')).toBe(false);
  });

  it('should return false for empty signature', () => {
    expect(verifySignature(payload, secret, '')).toBe(false);
  });

  it('should handle signature with wrong length', () => {
    expect(
      verifySignature(payload, secret, 'sha256=abc123')
    ).toBe(false);
  });
});
