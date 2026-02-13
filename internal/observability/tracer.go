package observability

import "context"

// Tracer defines the interface for observability tracing.
// Implementations track the lifecycle of tasks through phases,
// recording LLM invocations (generations) and skipped components.
//
// Trace hierarchy:
//
//	Task (Trace)
//	  └── Phase (Span): PLAN, IMPLEMENT, DOCS, VERIFY
//	        ├── Worker (Generation)
//	        ├── Reviewer (Generation or Event if skipped)
//	        └── Judge (Generation or Event if skipped)
type Tracer interface {
	StartTrace(taskID string, opts TraceOptions) TraceContext
	StartPhase(trace TraceContext, phase string, opts SpanOptions) SpanContext
	RecordGeneration(span SpanContext, gen GenerationInput)
	RecordSkipped(span SpanContext, component string, reason string)
	EndPhase(span SpanContext, status string, durationMs int64)
	CompleteTrace(trace TraceContext, opts CompleteOptions)
	Flush(ctx context.Context) error
	Stop(ctx context.Context) error
}

// TraceContext holds the context for an active trace (task level).
type TraceContext struct {
	TraceID  string
	TaskID   string
	Metadata map[string]string
}

// SpanContext holds the context for an active span (phase level).
type SpanContext struct {
	SpanID    string
	PhaseName string
	TraceID   string
}

// TraceOptions configures a new trace.
type TraceOptions struct {
	Workflow   string
	Repository string
	SessionID  string
}

// SpanOptions configures a new span.
type SpanOptions struct {
	Iteration     int
	MaxIterations int
	Metadata      map[string]string
}

// GenerationInput describes an LLM invocation to record.
type GenerationInput struct {
	Name         string // "Worker", "Reviewer", or "Judge"
	Model        string
	Input        string // Prompt text sent to the LLM
	Output       string // Response text from the LLM
	InputTokens  int
	OutputTokens int
	Status       string // "completed" or "error"
	DurationMs   int64
}

// CompleteOptions configures trace completion.
type CompleteOptions struct {
	Status            string // "completed", "failed", "blocked"
	TotalInputTokens  int
	TotalOutputTokens int
}
