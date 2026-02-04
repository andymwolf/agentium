import type {
  TracerConfig,
  TraceContext,
  SpanContext,
  GenerationInput,
  StartTraceOptions,
  StartSpanOptions,
  CompleteTraceOptions,
  CompleteSpanOptions,
  LangfuseClient,
  LangfuseTrace,
  LangfuseSpan,
} from './types.js';
import { createLangfuseClient } from './langfuse.js';

/**
 * LangfuseTracer
 *
 * Manages trace and span lifecycle for LLM observability.
 * Creates a hierarchy: Task (trace) -> Phase (span) -> W/R/J (generations)
 *
 * @example
 * ```typescript
 * const tracer = new LangfuseTracer(langfuseClient);
 *
 * // Start a trace for a task
 * const trace = tracer.startTrace('task-123', { workflow: 'default' });
 *
 * // Start a span for a phase
 * const span = tracer.startPhase(trace, 'PLAN');
 *
 * // Record generations for W/R/J
 * tracer.recordGeneration(span, {
 *   name: 'Worker',
 *   model: 'claude-sonnet-4-20250514',
 *   input: prompt,
 *   output: response,
 *   tokenMetrics: { input_tokens: 1000, output_tokens: 200 },
 *   status: 'completed',
 * });
 *
 * // Record skipped components
 * tracer.recordSkipped(span, 'Reviewer', 'empty_output');
 *
 * // Complete the span
 * tracer.endPhase(span, { status: 'completed', durationMs: 5000 });
 *
 * // Complete the trace
 * tracer.completeTrace(trace, {
 *   status: 'completed',
 *   totalTokens: { input_tokens: 3000, output_tokens: 600 },
 * });
 *
 * // Flush to ensure all data is sent
 * await tracer.flush();
 * ```
 */
export class LangfuseTracer {
  private client: LangfuseClient;
  private activeTraces: Map<string, LangfuseTrace> = new Map();
  private activeSpans: Map<string, LangfuseSpan> = new Map();

  constructor(client: LangfuseClient) {
    this.client = client;
  }

  /**
   * Start a new trace for a task
   *
   * @param taskId - The unique task identifier (used as trace ID for easy lookup)
   * @param options - Optional trace configuration
   * @returns The trace context
   */
  startTrace(taskId: string, options: StartTraceOptions = {}): TraceContext {
    const trace = this.client.trace({
      id: taskId, // Use task_id as trace ID for easy lookup in Langfuse UI
      name: options.workflow || 'custom',
      metadata: {
        repository: options.repository,
        workflow: options.workflow,
        triggered_by: options.triggeredBy,
      },
    });

    this.activeTraces.set(taskId, trace);

    return {
      traceId: taskId,
      taskId,
      metadata: {
        workflow: options.workflow,
        repository: options.repository,
        triggeredBy: options.triggeredBy,
      },
    };
  }

  /**
   * Start a new span for a phase within a trace
   *
   * @param traceContext - The parent trace context
   * @param phaseName - The name of the phase (e.g., 'PLAN', 'IMPLEMENT', 'DOCS')
   * @param options - Optional span configuration
   * @returns The span context
   */
  startPhase(
    traceContext: TraceContext,
    phaseName: string,
    options: StartSpanOptions = {}
  ): SpanContext {
    const trace = this.activeTraces.get(traceContext.traceId);
    if (!trace) {
      throw new Error(`Trace not found: ${traceContext.traceId}`);
    }

    const span = trace.span({
      name: phaseName,
      metadata: options.metadata,
    });

    const spanId = `${traceContext.traceId}-${phaseName}`;
    this.activeSpans.set(spanId, span);

    return {
      spanId,
      phaseName,
      traceId: traceContext.traceId,
    };
  }

  /**
   * Record a generation (LLM invocation) within a span
   *
   * @param spanContext - The parent span context
   * @param input - The generation details
   */
  recordGeneration(spanContext: SpanContext, input: GenerationInput): void {
    const span = this.activeSpans.get(spanContext.spanId);
    if (!span) {
      throw new Error(`Span not found: ${spanContext.spanId}`);
    }

    // If the component was skipped, record as an event instead
    if (input.status === 'skipped') {
      span.event({
        name: `${input.name} Skipped`,
        metadata: { skip_reason: input.skipReason },
      });
      return;
    }

    span.generation({
      name: input.name,
      model: input.model,
      input: input.input,
      output: input.output,
      usage: {
        input: input.tokenMetrics.input_tokens,
        output: input.tokenMetrics.output_tokens,
      },
      metadata: {
        model: input.tokenMetrics.model,
      },
    });
  }

  /**
   * Record a skipped component (Worker/Reviewer/Judge)
   *
   * @param spanContext - The parent span context
   * @param name - The component name
   * @param skipReason - The reason for skipping
   */
  recordSkipped(
    spanContext: SpanContext,
    name: 'Worker' | 'Reviewer' | 'Judge',
    skipReason: string
  ): void {
    const span = this.activeSpans.get(spanContext.spanId);
    if (!span) {
      throw new Error(`Span not found: ${spanContext.spanId}`);
    }

    span.event({
      name: `${name} Skipped`,
      metadata: { skip_reason: skipReason },
    });
  }

  /**
   * End a phase span
   *
   * @param spanContext - The span context to end
   * @param options - Completion options
   */
  endPhase(spanContext: SpanContext, options: CompleteSpanOptions = {}): void {
    const span = this.activeSpans.get(spanContext.spanId);
    if (!span) {
      throw new Error(`Span not found: ${spanContext.spanId}`);
    }

    span.end({
      metadata: {
        status: options.status,
        duration_ms: options.durationMs,
      },
    });

    this.activeSpans.delete(spanContext.spanId);
  }

  /**
   * Complete a trace
   *
   * @param traceContext - The trace context to complete
   * @param options - Completion options
   */
  completeTrace(
    traceContext: TraceContext,
    options: CompleteTraceOptions
  ): void {
    const trace = this.activeTraces.get(traceContext.traceId);
    if (!trace) {
      throw new Error(`Trace not found: ${traceContext.traceId}`);
    }

    const metadata: Record<string, unknown> = {
      status: options.status,
    };

    if (options.totalTokens) {
      metadata.total_input_tokens = options.totalTokens.input_tokens;
      metadata.total_output_tokens = options.totalTokens.output_tokens;
      if (options.totalTokens.estimated_cost_usd !== undefined) {
        metadata.estimated_cost_usd = options.totalTokens.estimated_cost_usd;
      }
    }

    trace.update({
      output: options.output,
      metadata,
    });

    this.activeTraces.delete(traceContext.traceId);
  }

  /**
   * Flush all pending traces to Langfuse
   *
   * Should be called before process exit to ensure all data is sent
   */
  async flush(): Promise<void> {
    await this.client.flush();
  }

  /**
   * Get the underlying Langfuse client
   * Useful for advanced operations or testing
   */
  getClient(): LangfuseClient {
    return this.client;
  }
}

/**
 * Create a new LangfuseTracer instance
 *
 * @param config - The tracer configuration
 * @returns A configured LangfuseTracer instance
 */
export function createTracer(config: TracerConfig): LangfuseTracer {
  const client = createLangfuseClient(config);
  return new LangfuseTracer(client);
}

/**
 * Create a LangfuseTracer from environment variables
 *
 * Expects:
 * - LANGFUSE_PUBLIC_KEY
 * - LANGFUSE_SECRET_KEY
 * - LANGFUSE_BASE_URL (optional)
 * - LANGFUSE_ENABLED (optional, defaults to 'true')
 *
 * @returns A configured LangfuseTracer instance
 */
export function createTracerFromEnv(): LangfuseTracer {
  const publicKey = process.env.LANGFUSE_PUBLIC_KEY || '';
  const secretKey = process.env.LANGFUSE_SECRET_KEY || '';
  const baseUrl = process.env.LANGFUSE_BASE_URL;
  const enabled = process.env.LANGFUSE_ENABLED !== 'false';

  return createTracer({
    publicKey,
    secretKey,
    baseUrl,
    enabled,
  });
}
