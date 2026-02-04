import { describe, it, expect, beforeEach } from 'vitest';
import {
  LangfuseTracer,
  createTracer,
  MockLangfuseClient,
  createMockClient,
  createNoOpClient,
} from '../index.js';
import type { TraceContext, SpanContext, GenerationInput } from '../types.js';

describe('LangfuseTracer', () => {
  let mockLangfuse: MockLangfuseClient;
  let tracer: LangfuseTracer;

  beforeEach(() => {
    mockLangfuse = createMockClient();
    tracer = new LangfuseTracer(mockLangfuse);
  });

  describe('startTrace', () => {
    it('should create trace with task_id', () => {
      const traceContext = tracer.startTrace('task-123', { workflow: 'default' });

      expect(mockLangfuse.traces.length).toBe(1);
      expect(mockLangfuse.traces[0].options.id).toBe('task-123');
      expect(mockLangfuse.traces[0].options.name).toBe('default');
      expect(traceContext.traceId).toBe('task-123');
      expect(traceContext.taskId).toBe('task-123');
    });

    it('should use custom workflow name as trace name', () => {
      tracer.startTrace('task-456', { workflow: 'api-parity-demo' });

      const trace = mockLangfuse.getTraceById('task-456');
      expect(trace?.options.name).toBe('api-parity-demo');
    });

    it('should default to custom when no workflow specified', () => {
      tracer.startTrace('task-789');

      const trace = mockLangfuse.getTraceById('task-789');
      expect(trace?.options.name).toBe('custom');
    });

    it('should include metadata in trace', () => {
      tracer.startTrace('task-meta', {
        workflow: 'default',
        repository: 'https://github.com/org/repo',
        triggeredBy: 'webhook',
      });

      const trace = mockLangfuse.getTraceById('task-meta');
      expect(trace?.options.metadata).toEqual({
        repository: 'https://github.com/org/repo',
        workflow: 'default',
        triggered_by: 'webhook',
      });
    });
  });

  describe('startPhase', () => {
    let traceContext: TraceContext;

    beforeEach(() => {
      traceContext = tracer.startTrace('task-123', { workflow: 'default' });
    });

    it('should create span for each phase', () => {
      const spanContext = tracer.startPhase(traceContext, 'PLAN');

      const trace = mockLangfuse.getTraceById('task-123');
      expect(trace?.spans.length).toBe(1);
      expect(trace?.spans[0].options.name).toBe('PLAN');
      expect(spanContext.phaseName).toBe('PLAN');
      expect(spanContext.traceId).toBe('task-123');
    });

    it('should support multiple phases', () => {
      tracer.startPhase(traceContext, 'PLAN');
      tracer.startPhase(traceContext, 'IMPLEMENT');
      tracer.startPhase(traceContext, 'DOCS');

      const trace = mockLangfuse.getTraceById('task-123');
      expect(trace?.spans.length).toBe(3);
      expect(trace?.spans.map((s) => s.options.name)).toEqual(['PLAN', 'IMPLEMENT', 'DOCS']);
    });

    it('should include span metadata', () => {
      tracer.startPhase(traceContext, 'PLAN', {
        metadata: { max_iterations: 3 },
      });

      const trace = mockLangfuse.getTraceById('task-123');
      expect(trace?.spans[0].options.metadata).toEqual({ max_iterations: 3 });
    });

    it('should throw error for unknown trace', () => {
      const invalidContext: TraceContext = {
        traceId: 'unknown',
        taskId: 'unknown',
        metadata: {},
      };

      expect(() => tracer.startPhase(invalidContext, 'PLAN')).toThrow(
        'Trace not found: unknown'
      );
    });
  });

  describe('recordGeneration', () => {
    let traceContext: TraceContext;
    let spanContext: SpanContext;

    beforeEach(() => {
      traceContext = tracer.startTrace('task-123');
      spanContext = tracer.startPhase(traceContext, 'PLAN');
    });

    it('should record generation with token metrics', () => {
      const input: GenerationInput = {
        name: 'Worker',
        model: 'claude-sonnet-4-20250514',
        input: 'test prompt',
        output: 'test response',
        tokenMetrics: { input_tokens: 1000, output_tokens: 200 },
        status: 'completed',
      };

      tracer.recordGeneration(spanContext, input);

      const trace = mockLangfuse.getTraceById('task-123');
      const span = trace?.getSpanByName('PLAN');
      expect(span?.generations.length).toBe(1);
      expect(span?.generations[0]).toMatchObject({
        name: 'Worker',
        model: 'claude-sonnet-4-20250514',
        input: 'test prompt',
        output: 'test response',
        usage: { input: 1000, output: 200 },
      });
    });

    it('should record Worker generation', () => {
      tracer.recordGeneration(spanContext, {
        name: 'Worker',
        model: 'claude-sonnet-4-20250514',
        input: 'worker prompt',
        output: 'worker output',
        tokenMetrics: { input_tokens: 2500, output_tokens: 400 },
        status: 'completed',
      });

      const trace = mockLangfuse.getTraceById('task-123');
      const span = trace?.getSpanByName('PLAN');
      const gen = span?.getGenerationByName('Worker');
      expect(gen).toBeDefined();
      expect(gen?.usage).toEqual({ input: 2500, output: 400 });
    });

    it('should record Reviewer generation', () => {
      tracer.recordGeneration(spanContext, {
        name: 'Reviewer',
        model: 'claude-sonnet-4-20250514',
        input: 'reviewer prompt',
        output: 'reviewer feedback',
        tokenMetrics: { input_tokens: 800, output_tokens: 150 },
        status: 'completed',
      });

      const trace = mockLangfuse.getTraceById('task-123');
      const span = trace?.getSpanByName('PLAN');
      const gen = span?.getGenerationByName('Reviewer');
      expect(gen).toBeDefined();
      expect(gen?.usage).toEqual({ input: 800, output: 150 });
    });

    it('should record Judge generation', () => {
      tracer.recordGeneration(spanContext, {
        name: 'Judge',
        model: 'claude-sonnet-4-20250514',
        input: 'judge prompt',
        output: 'judge verdict',
        tokenMetrics: { input_tokens: 500, output_tokens: 100 },
        status: 'completed',
      });

      const trace = mockLangfuse.getTraceById('task-123');
      const span = trace?.getSpanByName('PLAN');
      const gen = span?.getGenerationByName('Judge');
      expect(gen).toBeDefined();
      expect(gen?.usage).toEqual({ input: 500, output: 100 });
    });

    it('should record skipped generation as event', () => {
      tracer.recordGeneration(spanContext, {
        name: 'Reviewer',
        model: 'claude-sonnet-4-20250514',
        input: '',
        output: '',
        tokenMetrics: { input_tokens: 0, output_tokens: 0 },
        status: 'skipped',
        skipReason: 'empty_output',
      });

      const trace = mockLangfuse.getTraceById('task-123');
      const span = trace?.getSpanByName('PLAN');
      expect(span?.generations.length).toBe(0);
      expect(span?.events.length).toBe(1);
      expect(span?.events[0].name).toBe('Reviewer Skipped');
      expect(span?.events[0].metadata).toEqual({ skip_reason: 'empty_output' });
    });

    it('should throw error for unknown span', () => {
      const invalidSpan: SpanContext = {
        spanId: 'unknown-span',
        phaseName: 'PLAN',
        traceId: 'task-123',
      };

      expect(() =>
        tracer.recordGeneration(invalidSpan, {
          name: 'Worker',
          model: 'test',
          input: '',
          output: '',
          tokenMetrics: { input_tokens: 0, output_tokens: 0 },
          status: 'completed',
        })
      ).toThrow('Span not found: unknown-span');
    });
  });

  describe('recordSkipped', () => {
    let traceContext: TraceContext;
    let spanContext: SpanContext;

    beforeEach(() => {
      traceContext = tracer.startTrace('task-123');
      spanContext = tracer.startPhase(traceContext, 'PLAN');
    });

    it('should record skipped components as events', () => {
      tracer.recordSkipped(spanContext, 'Reviewer', 'empty_output');

      const trace = mockLangfuse.getTraceById('task-123');
      const span = trace?.getSpanByName('PLAN');
      expect(span?.events.length).toBe(1);
      expect(span?.events[0]).toEqual({
        name: 'Reviewer Skipped',
        metadata: { skip_reason: 'empty_output' },
      });
    });

    it('should record Worker skipped', () => {
      tracer.recordSkipped(spanContext, 'Worker', 'failed_dependency');

      const trace = mockLangfuse.getTraceById('task-123');
      const span = trace?.getSpanByName('PLAN');
      expect(span?.events[0].name).toBe('Worker Skipped');
    });

    it('should record Judge skipped', () => {
      tracer.recordSkipped(spanContext, 'Judge', 'simple_output');

      const trace = mockLangfuse.getTraceById('task-123');
      const span = trace?.getSpanByName('PLAN');
      expect(span?.events[0].name).toBe('Judge Skipped');
      expect(span?.events[0].metadata).toEqual({ skip_reason: 'simple_output' });
    });

    it('should throw error for unknown span', () => {
      const invalidSpan: SpanContext = {
        spanId: 'unknown-span',
        phaseName: 'PLAN',
        traceId: 'task-123',
      };

      expect(() => tracer.recordSkipped(invalidSpan, 'Worker', 'test')).toThrow(
        'Span not found: unknown-span'
      );
    });
  });

  describe('endPhase', () => {
    let traceContext: TraceContext;
    let spanContext: SpanContext;

    beforeEach(() => {
      traceContext = tracer.startTrace('task-123');
      spanContext = tracer.startPhase(traceContext, 'PLAN');
    });

    it('should end span with status', () => {
      tracer.endPhase(spanContext, { status: 'completed', durationMs: 5000 });

      const trace = mockLangfuse.getTraceById('task-123');
      const span = trace?.getSpanByName('PLAN');
      expect(span?.endOptions).toEqual({
        metadata: { status: 'completed', duration_ms: 5000 },
      });
    });

    it('should end span with failed status', () => {
      tracer.endPhase(spanContext, { status: 'failed' });

      const trace = mockLangfuse.getTraceById('task-123');
      const span = trace?.getSpanByName('PLAN');
      expect(span?.endOptions?.metadata?.status).toBe('failed');
    });

    it('should throw error for unknown span', () => {
      const invalidSpan: SpanContext = {
        spanId: 'unknown-span',
        phaseName: 'PLAN',
        traceId: 'task-123',
      };

      expect(() => tracer.endPhase(invalidSpan, { status: 'completed' })).toThrow(
        'Span not found: unknown-span'
      );
    });
  });

  describe('completeTrace', () => {
    let traceContext: TraceContext;

    beforeEach(() => {
      traceContext = tracer.startTrace('task-123');
    });

    it('should update trace with completion status', () => {
      tracer.completeTrace(traceContext, {
        status: 'completed',
        output: 'Task completed successfully',
      });

      const trace = mockLangfuse.getTraceById('task-123');
      expect(trace?.updates.length).toBe(1);
      expect(trace?.updates[0]).toMatchObject({
        output: 'Task completed successfully',
        metadata: { status: 'completed' },
      });
    });

    it('should include total token metrics', () => {
      tracer.completeTrace(traceContext, {
        status: 'completed',
        totalTokens: {
          input_tokens: 10000,
          output_tokens: 2000,
          estimated_cost_usd: 0.15,
        },
      });

      const trace = mockLangfuse.getTraceById('task-123');
      expect(trace?.updates[0].metadata).toMatchObject({
        status: 'completed',
        total_input_tokens: 10000,
        total_output_tokens: 2000,
        estimated_cost_usd: 0.15,
      });
    });

    it('should handle failed status', () => {
      tracer.completeTrace(traceContext, {
        status: 'failed',
        output: 'Task failed: timeout',
      });

      const trace = mockLangfuse.getTraceById('task-123');
      expect(trace?.updates[0]).toMatchObject({
        output: 'Task failed: timeout',
        metadata: { status: 'failed' },
      });
    });

    it('should throw error for unknown trace', () => {
      const invalidContext: TraceContext = {
        traceId: 'unknown',
        taskId: 'unknown',
        metadata: {},
      };

      expect(() =>
        tracer.completeTrace(invalidContext, { status: 'completed' })
      ).toThrow('Trace not found: unknown');
    });
  });

  describe('flush', () => {
    it('should call client flush', async () => {
      let flushed = false;
      const customClient = {
        trace: () => ({ id: 'test', span: () => ({}), update: () => {} }),
        flush: async () => {
          flushed = true;
        },
      } as unknown as MockLangfuseClient;

      const customTracer = new LangfuseTracer(customClient);
      await customTracer.flush();

      expect(flushed).toBe(true);
    });
  });

  describe('getClient', () => {
    it('should return the underlying client', () => {
      const client = tracer.getClient();
      expect(client).toBe(mockLangfuse);
    });
  });
});

describe('createTracer', () => {
  it('should create a tracer with config', () => {
    const tracer = createTracer({
      publicKey: 'pk-test',
      secretKey: 'sk-test',
      baseUrl: 'https://langfuse.example.com',
    });

    expect(tracer).toBeInstanceOf(LangfuseTracer);
  });

  it('should create disabled tracer when enabled is false', () => {
    const tracer = createTracer({
      publicKey: 'pk-test',
      secretKey: 'sk-test',
      enabled: false,
    });

    // Should not throw when used
    const trace = tracer.startTrace('test-task');
    const span = tracer.startPhase(trace, 'TEST');
    tracer.recordGeneration(span, {
      name: 'Worker',
      model: 'test',
      input: '',
      output: '',
      tokenMetrics: { input_tokens: 0, output_tokens: 0 },
      status: 'completed',
    });

    expect(true).toBe(true); // Just verifying no exceptions
  });
});

describe('createNoOpClient', () => {
  it('should create a no-op client', () => {
    const client = createNoOpClient();
    const trace = client.trace({ id: 'test', name: 'test' });

    expect(trace.id).toBe('noop');

    const span = trace.span({ name: 'test' });
    expect(span.id).toBe('noop');

    // These should not throw
    span.generation({ name: 'test', model: 'test', input: '', output: '' });
    span.event({ name: 'test' });
    span.end();
    trace.update({});
  });

  it('should flush without error', async () => {
    const client = createNoOpClient();
    await expect(client.flush()).resolves.toBeUndefined();
  });
});

describe('MockLangfuseClient', () => {
  let client: MockLangfuseClient;

  beforeEach(() => {
    client = createMockClient();
  });

  it('should track created traces', () => {
    client.trace({ id: 'trace-1', name: 'test1' });
    client.trace({ id: 'trace-2', name: 'test2' });

    expect(client.getTraces().length).toBe(2);
  });

  it('should find trace by ID', () => {
    client.trace({ id: 'trace-1', name: 'test1' });
    client.trace({ id: 'trace-2', name: 'test2' });

    const trace = client.getTraceById('trace-2');
    expect(trace?.options.name).toBe('test2');
  });

  it('should clear all data', () => {
    client.trace({ id: 'trace-1', name: 'test' });
    client.clear();

    expect(client.getTraces().length).toBe(0);
  });
});

describe('Integration: Full trace lifecycle', () => {
  it('should create trace hierarchy matching demo structure', () => {
    const mockClient = createMockClient();
    const tracer = new LangfuseTracer(mockClient);

    // Start task trace
    const trace = tracer.startTrace('api-parity-demo', {
      workflow: 'default',
      repository: 'https://github.com/org/repo',
    });

    // PLAN phase
    const planSpan = tracer.startPhase(trace, 'PLAN');

    tracer.recordGeneration(planSpan, {
      name: 'Worker',
      model: 'claude-sonnet-4-20250514',
      input: 'Plan the implementation...',
      output: 'Implementation plan: ...',
      tokenMetrics: { input_tokens: 2500, output_tokens: 400 },
      status: 'completed',
    });

    tracer.recordSkipped(planSpan, 'Reviewer', 'empty_output');

    tracer.recordGeneration(planSpan, {
      name: 'Judge',
      model: 'claude-sonnet-4-20250514',
      input: 'Evaluate the plan...',
      output: '{"passed": true}',
      tokenMetrics: { input_tokens: 500, output_tokens: 100 },
      status: 'completed',
    });

    tracer.endPhase(planSpan, { status: 'completed', durationMs: 3000 });

    // IMPLEMENT phase
    const implSpan = tracer.startPhase(trace, 'IMPLEMENT');

    tracer.recordGeneration(implSpan, {
      name: 'Worker',
      model: 'claude-sonnet-4-20250514',
      input: 'Implement the changes...',
      output: 'Implementation complete...',
      tokenMetrics: { input_tokens: 3000, output_tokens: 800 },
      status: 'completed',
    });

    tracer.endPhase(implSpan, { status: 'completed', durationMs: 5000 });

    // Complete trace
    tracer.completeTrace(trace, {
      status: 'completed',
      output: 'Task completed',
      totalTokens: {
        input_tokens: 6000,
        output_tokens: 1300,
        estimated_cost_usd: 0.045,
      },
    });

    // Verify structure
    const langfuseTrace = mockClient.getTraceById('api-parity-demo');
    expect(langfuseTrace).toBeDefined();
    expect(langfuseTrace?.options.name).toBe('default');

    // Verify PLAN span
    const planLfSpan = langfuseTrace?.getSpanByName('PLAN');
    expect(planLfSpan).toBeDefined();
    expect(planLfSpan?.generations.length).toBe(2); // Worker + Judge
    expect(planLfSpan?.events.length).toBe(1); // Reviewer skipped

    // Verify IMPLEMENT span
    const implLfSpan = langfuseTrace?.getSpanByName('IMPLEMENT');
    expect(implLfSpan).toBeDefined();
    expect(implLfSpan?.generations.length).toBe(1); // Worker only

    // Verify trace completion
    expect(langfuseTrace?.updates.length).toBe(1);
    expect(langfuseTrace?.updates[0].metadata?.total_input_tokens).toBe(6000);
    expect(langfuseTrace?.updates[0].metadata?.total_output_tokens).toBe(1300);
  });
});
