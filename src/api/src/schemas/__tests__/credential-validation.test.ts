import { describe, it, expect } from 'vitest';
import {
  validateCredentials,
  extractAdaptersFromPhases,
  getRequiredProvider,
} from '../credential-validation.js';
import type { Credentials } from '../task-config.js';

describe('credential-validation', () => {
  describe('getRequiredProvider', () => {
    it('should return anthropic for Claude adapters', () => {
      expect(getRequiredProvider('claude-code')).toBe('anthropic');
      expect(getRequiredProvider('claude-sonnet')).toBe('anthropic');
      expect(getRequiredProvider('claude-opus')).toBe('anthropic');
      expect(getRequiredProvider('claude-haiku')).toBe('anthropic');
    });

    it('should return openai for OpenAI adapters', () => {
      expect(getRequiredProvider('codex')).toBe('openai');
      expect(getRequiredProvider('gpt-4')).toBe('openai');
      expect(getRequiredProvider('gpt-4-turbo')).toBe('openai');
      expect(getRequiredProvider('gpt-4o')).toBe('openai');
    });

    it('should return undefined for unknown adapters', () => {
      expect(getRequiredProvider('aider')).toBeUndefined();
      expect(getRequiredProvider('unknown')).toBeUndefined();
    });
  });

  describe('extractAdaptersFromPhases', () => {
    it('should return empty array for undefined phases', () => {
      expect(extractAdaptersFromPhases(undefined)).toEqual([]);
    });

    it('should return empty array for phases without workers', () => {
      expect(extractAdaptersFromPhases([{ name: 'test' }])).toEqual([]);
    });

    it('should extract adapter from phase with worker', () => {
      const phases = [
        { name: 'implement', worker: { adapter: 'claude-code' } },
      ];
      expect(extractAdaptersFromPhases(phases)).toEqual(['claude-code']);
    });

    it('should extract unique adapters from multiple phases', () => {
      const phases = [
        { name: 'implement', worker: { adapter: 'claude-code' } },
        { name: 'test', worker: { adapter: 'claude-code' } },
        { name: 'review', worker: { adapter: 'codex' } },
      ];
      const result = extractAdaptersFromPhases(phases);
      expect(result).toContain('claude-code');
      expect(result).toContain('codex');
      expect(result.length).toBe(2);
    });
  });

  describe('validateCredentials', () => {
    it('should pass when no adapters require credentials', () => {
      const result = validateCredentials(['aider']);
      expect(result.valid).toBe(true);
      expect(result.missingCredentials).toEqual([]);
    });

    it('should pass when required anthropic credentials are provided', () => {
      const credentials: Credentials = {
        anthropic: { access_token: 'sk-ant-test', token_type: 'Bearer' },
      };
      const result = validateCredentials(['claude-code'], credentials);
      expect(result.valid).toBe(true);
      expect(result.missingCredentials).toEqual([]);
    });

    it('should pass when required openai credentials are provided', () => {
      const credentials: Credentials = {
        openai: { access_token: 'sk-test', token_type: 'Bearer' },
      };
      const result = validateCredentials(['codex'], credentials);
      expect(result.valid).toBe(true);
      expect(result.missingCredentials).toEqual([]);
    });

    it('should fail when required anthropic credentials are missing', () => {
      const result = validateCredentials(['claude-code']);
      expect(result.valid).toBe(false);
      expect(result.missingCredentials).toContain('anthropic');
      expect(result.error).toContain('anthropic');
    });

    it('should fail when required openai credentials are missing', () => {
      const result = validateCredentials(['codex']);
      expect(result.valid).toBe(false);
      expect(result.missingCredentials).toContain('openai');
      expect(result.error).toContain('openai');
    });

    it('should fail when only one of multiple required providers is present', () => {
      const credentials: Credentials = {
        anthropic: { access_token: 'sk-ant-test', token_type: 'Bearer' },
      };
      const result = validateCredentials(['claude-code', 'codex'], credentials);
      expect(result.valid).toBe(false);
      expect(result.missingCredentials).toContain('openai');
      expect(result.missingCredentials).not.toContain('anthropic');
    });

    it('should pass when all required providers have credentials', () => {
      const credentials: Credentials = {
        anthropic: { access_token: 'sk-ant-test', token_type: 'Bearer' },
        openai: { access_token: 'sk-test', token_type: 'Bearer' },
      };
      const result = validateCredentials(['claude-code', 'codex'], credentials);
      expect(result.valid).toBe(true);
      expect(result.missingCredentials).toEqual([]);
    });

    it('should handle empty adapter list', () => {
      const result = validateCredentials([]);
      expect(result.valid).toBe(true);
      expect(result.missingCredentials).toEqual([]);
    });

    it('should handle credentials with empty access_token', () => {
      const credentials: Credentials = {
        anthropic: { access_token: '', token_type: 'Bearer' },
      };
      const result = validateCredentials(['claude-code'], credentials);
      expect(result.valid).toBe(false);
      expect(result.missingCredentials).toContain('anthropic');
    });
  });
});
