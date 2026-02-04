import type {
  TracerConfig,
  LangfuseClient,
  LangfuseTrace,
  LangfuseSpan,
  LangfuseTraceOptions,
  LangfuseSpanOptions,
  LangfuseGenerationOptions,
  LangfuseEventOptions,
  LangfuseUpdateOptions,
  LangfuseEndOptions,
} from './types.js';

/**
 * Create a Langfuse client instance
 *
 * In production, this would use the actual Langfuse SDK.
 * For now, this provides a type-safe wrapper that can be swapped out
 * once the langfuse package is added as a dependency.
 *
 * @param config - The tracer configuration
 * @returns A Langfuse client instance
 */
export function createLangfuseClient(config: TracerConfig): LangfuseClient {
  // If observability is disabled, return a no-op client
  if (config.enabled === false) {
    return createNoOpClient();
  }

  // In a real implementation, this would initialize the Langfuse SDK:
  // import { Langfuse } from 'langfuse';
  // return new Langfuse({
  //   publicKey: config.publicKey,
  //   secretKey: config.secretKey,
  //   baseUrl: config.baseUrl,
  // });

  // For now, return a mock implementation for testing
  return createMockClient();
}

/**
 * Create a no-op client that does nothing
 * Used when observability is disabled
 */
export function createNoOpClient(): LangfuseClient {
  const noOpSpan: LangfuseSpan = {
    id: 'noop',
    generation: () => {},
    event: () => {},
    end: () => {},
  };

  const noOpTrace: LangfuseTrace = {
    id: 'noop',
    span: () => noOpSpan,
    update: () => {},
  };

  return {
    trace: () => noOpTrace,
    flush: async () => {},
  };
}

/**
 * Create a mock client for testing
 * Records all calls for verification
 */
export function createMockClient(): MockLangfuseClient {
  return new MockLangfuseClient();
}

/**
 * Mock Langfuse client for testing
 */
export class MockLangfuseClient implements LangfuseClient {
  public readonly traces: MockLangfuseTrace[] = [];
  private traceCounter = 0;

  trace(options: LangfuseTraceOptions): LangfuseTrace {
    const trace = new MockLangfuseTrace(options, ++this.traceCounter);
    this.traces.push(trace);
    return trace;
  }

  async flush(): Promise<void> {
    // No-op for mock
  }

  /**
   * Get all recorded traces
   */
  getTraces(): MockLangfuseTrace[] {
    return this.traces;
  }

  /**
   * Get a trace by ID
   */
  getTraceById(id: string): MockLangfuseTrace | undefined {
    return this.traces.find((t) => t.options.id === id);
  }

  /**
   * Clear all recorded data
   */
  clear(): void {
    this.traces.length = 0;
    this.traceCounter = 0;
  }
}

/**
 * Mock Langfuse trace for testing
 */
export class MockLangfuseTrace implements LangfuseTrace {
  public readonly spans: MockLangfuseSpan[] = [];
  public updates: LangfuseUpdateOptions[] = [];
  private spanCounter = 0;

  constructor(
    public readonly options: LangfuseTraceOptions,
    private readonly counter: number
  ) {}

  get id(): string {
    return this.options.id;
  }

  span(options: LangfuseSpanOptions): LangfuseSpan {
    const span = new MockLangfuseSpan(options, this.id, ++this.spanCounter);
    this.spans.push(span);
    return span;
  }

  update(options: LangfuseUpdateOptions): void {
    this.updates.push(options);
  }

  /**
   * Get all recorded spans
   */
  getSpans(): MockLangfuseSpan[] {
    return this.spans;
  }

  /**
   * Get a span by name
   */
  getSpanByName(name: string): MockLangfuseSpan | undefined {
    return this.spans.find((s) => s.options.name === name);
  }
}

/**
 * Mock Langfuse span for testing
 */
export class MockLangfuseSpan implements LangfuseSpan {
  public readonly generations: LangfuseGenerationOptions[] = [];
  public readonly events: LangfuseEventOptions[] = [];
  public endOptions?: LangfuseEndOptions;

  constructor(
    public readonly options: LangfuseSpanOptions,
    private readonly traceId: string,
    private readonly counter: number
  ) {}

  get id(): string {
    return `${this.traceId}-span-${this.counter}`;
  }

  generation(options: LangfuseGenerationOptions): void {
    this.generations.push(options);
  }

  event(options: LangfuseEventOptions): void {
    this.events.push(options);
  }

  end(options?: LangfuseEndOptions): void {
    this.endOptions = options;
  }

  /**
   * Get all recorded generations
   */
  getGenerations(): LangfuseGenerationOptions[] {
    return this.generations;
  }

  /**
   * Get a generation by name
   */
  getGenerationByName(name: string): LangfuseGenerationOptions | undefined {
    return this.generations.find((g) => g.name === name);
  }

  /**
   * Get all recorded events
   */
  getEvents(): LangfuseEventOptions[] {
    return this.events;
  }
}
