import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import request from 'supertest';
import { createApp } from '../index.js';

describe('POST /api/v1/tasks', () => {
  const app = createApp();
  const validApiKey = 'test-api-key-12345';

  beforeEach(() => {
    // Set up valid API keys
    process.env.AGENTIUM_API_KEYS = validApiKey;
  });

  afterEach(() => {
    delete process.env.AGENTIUM_API_KEYS;
    vi.restoreAllMocks();
  });

  describe('Authentication', () => {
    it('should return 401 for missing Authorization header', async () => {
      const response = await request(app)
        .post('/api/v1/tasks')
        .send({
          repository: { url: 'https://github.com/test/repo', branch: 'main' },
        });

      expect(response.status).toBe(401);
      expect(response.body.error).toBe('Unauthorized');
      expect(response.body.message).toBe('Missing Authorization header');
    });

    it('should return 401 for invalid Authorization header format', async () => {
      const response = await request(app)
        .post('/api/v1/tasks')
        .set('Authorization', 'Basic invalid')
        .send({
          repository: { url: 'https://github.com/test/repo', branch: 'main' },
        });

      expect(response.status).toBe(401);
      expect(response.body.error).toBe('Unauthorized');
    });

    it('should return 401 for invalid API key', async () => {
      const response = await request(app)
        .post('/api/v1/tasks')
        .set('Authorization', 'Bearer invalid-key')
        .send({
          repository: { url: 'https://github.com/test/repo', branch: 'main' },
        });

      expect(response.status).toBe(401);
      expect(response.body.error).toBe('Unauthorized');
      expect(response.body.message).toBe('Invalid API key');
    });
  });

  describe('Validation', () => {
    it('should return 400 when both workflow and phases are provided', async () => {
      const response = await request(app)
        .post('/api/v1/tasks')
        .set('Authorization', `Bearer ${validApiKey}`)
        .send({
          repository: { url: 'https://github.com/test/repo', branch: 'main' },
          workflow: 'default',
          phases: [{ name: 'custom' }],
        });

      expect(response.status).toBe(400);
      expect(response.body.error).toBe('Validation failed');
      expect(response.body.details).toBeDefined();
      expect(response.body.details.some((e: { message: string }) =>
        e.message.includes('mutually exclusive')
      )).toBe(true);
    });

    it('should return 400 for missing repository', async () => {
      const response = await request(app)
        .post('/api/v1/tasks')
        .set('Authorization', `Bearer ${validApiKey}`)
        .send({
          workflow: 'default',
        });

      expect(response.status).toBe(400);
      expect(response.body.error).toBe('Validation failed');
    });

    it('should return 400 for non-GitHub repository URL', async () => {
      const response = await request(app)
        .post('/api/v1/tasks')
        .set('Authorization', `Bearer ${validApiKey}`)
        .send({
          repository: { url: 'https://gitlab.com/test/repo', branch: 'main' },
        });

      expect(response.status).toBe(400);
      expect(response.body.error).toBe('Validation failed');
    });

    it('should return 400 for non-HTTPS webhook URL', async () => {
      const response = await request(app)
        .post('/api/v1/tasks')
        .set('Authorization', `Bearer ${validApiKey}`)
        .send({
          repository: { url: 'https://github.com/test/repo', branch: 'main' },
          webhook: { url: 'http://example.com/hook', secret: 'mysecret' },
        });

      expect(response.status).toBe(400);
      expect(response.body.error).toBe('Validation failed');
    });

    it('should return 400 for invalid task_id format', async () => {
      const response = await request(app)
        .post('/api/v1/tasks')
        .set('Authorization', `Bearer ${validApiKey}`)
        .send({
          task_id: 'not-a-uuid',
          repository: { url: 'https://github.com/test/repo', branch: 'main' },
        });

      expect(response.status).toBe(400);
      expect(response.body.error).toBe('Validation failed');
    });
  });

  describe('Successful requests', () => {
    it('should accept valid task config with default workflow', async () => {
      const response = await request(app)
        .post('/api/v1/tasks')
        .set('Authorization', `Bearer ${validApiKey}`)
        .send({
          repository: { url: 'https://github.com/test/repo', branch: 'main' },
          workflow: 'default',
        });

      expect(response.status).toBe(200);
      expect(response.body.status).toBe('accepted');
      expect(response.body.workflow).toBe('default');
      expect(response.body.task_id).toBeDefined();
      expect(response.body.estimated_start).toBeDefined();
      // Verify estimated_start is a valid ISO date
      expect(() => new Date(response.body.estimated_start)).not.toThrow();
    });

    it('should accept valid task config without workflow', async () => {
      const response = await request(app)
        .post('/api/v1/tasks')
        .set('Authorization', `Bearer ${validApiKey}`)
        .send({
          repository: { url: 'https://github.com/test/repo', branch: 'main' },
        });

      expect(response.status).toBe(200);
      expect(response.body.status).toBe('accepted');
      expect(response.body.workflow).toBeUndefined();
      expect(response.body.task_id).toBeDefined();
    });

    it('should accept valid task config with custom phases', async () => {
      const response = await request(app)
        .post('/api/v1/tasks')
        .set('Authorization', `Bearer ${validApiKey}`)
        .send({
          repository: { url: 'https://github.com/test/repo', branch: 'main' },
          phases: [
            { name: 'analyze', max_iterations: 2 },
            { name: 'implement', max_iterations: 5 },
          ],
        });

      expect(response.status).toBe(200);
      expect(response.body.status).toBe('accepted');
    });

    it('should use client-provided task_id when valid', async () => {
      const clientTaskId = '550e8400-e29b-41d4-a716-446655440000';
      const response = await request(app)
        .post('/api/v1/tasks')
        .set('Authorization', `Bearer ${validApiKey}`)
        .send({
          task_id: clientTaskId,
          repository: { url: 'https://github.com/test/repo', branch: 'main' },
        });

      expect(response.status).toBe(200);
      expect(response.body.task_id).toBe(clientTaskId);
    });

    it('should auto-generate task_id when not provided', async () => {
      const response = await request(app)
        .post('/api/v1/tasks')
        .set('Authorization', `Bearer ${validApiKey}`)
        .send({
          repository: { url: 'https://github.com/test/repo', branch: 'main' },
        });

      expect(response.status).toBe(200);
      // Verify it's a valid UUID format
      const uuidRegex = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
      expect(response.body.task_id).toMatch(uuidRegex);
    });

    it('should accept task config with all optional fields', async () => {
      const response = await request(app)
        .post('/api/v1/tasks')
        .set('Authorization', `Bearer ${validApiKey}`)
        .send({
          repository: { url: 'https://github.com/test/repo', branch: 'main' },
          workflow: 'default',
          prompt_context: {
            issue_url: 'https://github.com/test/repo/issues/1',
            instructions: 'Additional context',
          },
          behaviors: {
            skip_reviewer_on_empty: true,
            skip_judge_on_simple: false,
            judge_on_failure: true,
            auto_retry_on_rate_limit: true,
          },
          webhook: {
            url: 'https://example.com/webhook',
            secret: 'webhook-secret',
          },
        });

      expect(response.status).toBe(200);
      expect(response.body.status).toBe('accepted');
    });
  });
});

describe('Health check', () => {
  const app = createApp();

  it('should return ok status', async () => {
    const response = await request(app).get('/health');

    expect(response.status).toBe(200);
    expect(response.body.status).toBe('ok');
  });
});
