import type { TokenMetrics } from '../metrics/types.js';

/**
 * Configuration for the Langfuse tracer
 */
export interface TracerConfig {
  publicKey: string;
  secretKey: string;
  baseUrl?: string; // For self-hosted instances
  enabled?: boolean; // Allow disabling observability
}

/**
 * Context for a trace (task level)
 */
export interface TraceContext {
  traceId: string;
  taskId: string;
  metadata: TraceMetadata;
}

/**
 * Metadata attached to a trace
 */
export interface TraceMetadata {
  workflow?: string;
  repository?: string;
  triggeredBy?: string;
}

/**
 * Context for a span (phase level)
 */
export interface SpanContext {
  spanId: string;
  phaseName: string;
  traceId: string;
}

/**
 * Input for recording a generation (W/R/J invocation)
 */
export interface GenerationInput {
  name: 'Worker' | 'Reviewer' | 'Judge';
  model: string;
  input: string;
  output: string;
  tokenMetrics: TokenMetrics;
  status: 'completed' | 'skipped';
  skipReason?: string;
}

/**
 * Input for recording a skipped component
 */
export interface SkippedInput {
  name: 'Worker' | 'Reviewer' | 'Judge';
  skipReason: string;
}

/**
 * Options for starting a trace
 */
export interface StartTraceOptions {
  workflow?: string;
  repository?: string;
  triggeredBy?: string;
}

/**
 * Options for starting a span
 */
export interface StartSpanOptions {
  metadata?: Record<string, unknown>;
}

/**
 * Options for completing a trace
 */
export interface CompleteTraceOptions {
  status: 'completed' | 'failed';
  output?: string;
  totalTokens?: {
    input_tokens: number;
    output_tokens: number;
    estimated_cost_usd?: number;
  };
}

/**
 * Options for completing a span
 */
export interface CompleteSpanOptions {
  status?: 'completed' | 'failed';
  durationMs?: number;
}

/**
 * Langfuse trace object interface
 */
export interface LangfuseTrace {
  id: string;
  span(options: LangfuseSpanOptions): LangfuseSpan;
  update(options: LangfuseUpdateOptions): void;
}

/**
 * Langfuse span object interface
 */
export interface LangfuseSpan {
  id: string;
  generation(options: LangfuseGenerationOptions): void;
  event(options: LangfuseEventOptions): void;
  end(options?: LangfuseEndOptions): void;
}

/**
 * Options for creating a Langfuse span
 */
export interface LangfuseSpanOptions {
  name: string;
  metadata?: Record<string, unknown>;
}

/**
 * Options for creating a Langfuse generation
 */
export interface LangfuseGenerationOptions {
  name: string;
  model: string;
  input: string;
  output: string;
  usage?: {
    input: number;
    output: number;
  };
  metadata?: Record<string, unknown>;
}

/**
 * Options for creating a Langfuse event
 */
export interface LangfuseEventOptions {
  name: string;
  metadata?: Record<string, unknown>;
}

/**
 * Options for updating a Langfuse trace
 */
export interface LangfuseUpdateOptions {
  output?: string;
  metadata?: Record<string, unknown>;
}

/**
 * Options for ending a Langfuse span
 */
export interface LangfuseEndOptions {
  metadata?: Record<string, unknown>;
}

/**
 * Langfuse client interface
 * This abstracts the actual Langfuse SDK for easier testing
 */
export interface LangfuseClient {
  trace(options: LangfuseTraceOptions): LangfuseTrace;
  flush(): Promise<void>;
}

/**
 * Options for creating a Langfuse trace
 */
export interface LangfuseTraceOptions {
  id: string;
  name: string;
  metadata?: Record<string, unknown>;
}
