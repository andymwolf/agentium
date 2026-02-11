package observability

import "context"

// NoOpTracer is a tracer that does nothing. It is used when Langfuse
// is not configured or is explicitly disabled.
type NoOpTracer struct{}

func (n *NoOpTracer) StartTrace(_ string, _ TraceOptions) TraceContext {
	return TraceContext{}
}

func (n *NoOpTracer) StartPhase(_ TraceContext, _ string, _ SpanOptions) SpanContext {
	return SpanContext{}
}

func (n *NoOpTracer) RecordGeneration(_ SpanContext, _ GenerationInput) {}

func (n *NoOpTracer) RecordSkipped(_ SpanContext, _ string, _ string) {}

func (n *NoOpTracer) EndPhase(_ SpanContext, _ string, _ int64) {}

func (n *NoOpTracer) CompleteTrace(_ TraceContext, _ CompleteOptions) {}

func (n *NoOpTracer) Flush(_ context.Context) error { return nil }

func (n *NoOpTracer) Stop(_ context.Context) error { return nil }
