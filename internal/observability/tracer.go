package observability

import (
	"context"
	"time"
)

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
	EndPhase(span SpanContext, opts EndPhaseOptions)
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

// EndPhaseOptions configures EndPhase with optional structured I/O.
type EndPhaseOptions struct {
	Status     string
	DurationMs int64
	Input      interface{} // JSON-serializable, may be nil
	Output     interface{} // JSON-serializable, may be nil
}

// GenerationInput describes an LLM invocation to record.
type GenerationInput struct {
	Name         string // "Worker", "Reviewer", or "Judge"
	Model        string
	Input        string // Prompt text sent to the LLM
	Output       string // Response text from the LLM
	SystemPrompt string // System/skills prompt used for this invocation
	InputTokens  int
	OutputTokens int
	Status       string    // "completed" or "error"
	StartTime    time.Time // When the LLM invocation started
	EndTime      time.Time // When the LLM invocation finished
}

// CompleteOptions configures trace completion.
type CompleteOptions struct {
	Status            string // "completed", "failed", "blocked"
	TotalInputTokens  int
	TotalOutputTokens int
}
