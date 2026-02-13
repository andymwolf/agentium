package observability

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	// defaultBaseURL is the Langfuse Cloud ingestion endpoint.
	defaultBaseURL = "https://cloud.langfuse.com"

	// ingestionPath is the batched ingestion API path.
	ingestionPath = "/api/public/ingestion"

	// flushInterval is how often the background goroutine flushes events.
	flushInterval = 5 * time.Second

	// maxBatchSize is the maximum number of events to send in one request.
	maxBatchSize = 50

	// eventBufferSize is the channel buffer size for incoming events.
	eventBufferSize = 1024

	// retryDelay is the delay between send retries.
	retryDelay = 500 * time.Millisecond
)

// LangfuseConfig holds Langfuse connection parameters.
type LangfuseConfig struct {
	PublicKey string
	SecretKey string
	BaseURL   string // Defaults to https://cloud.langfuse.com
}

// LangfuseTracer sends trace/span/generation events to the Langfuse
// ingestion API using batched HTTP requests. Events are buffered in a
// channel and flushed periodically or on explicit Flush() calls.
type LangfuseTracer struct {
	config     LangfuseConfig
	authHeader string
	client     *http.Client
	events     chan ingestionEvent
	logger     *log.Logger

	wg       sync.WaitGroup
	stopCh   chan struct{}
	stopOnce sync.Once
	flushMu  sync.Mutex // protects concurrent drain operations
}

// NewLangfuseTracer creates a new LangfuseTracer and starts its background
// flush goroutine. Call Flush() or close the tracer to ensure all events
// are sent.
func NewLangfuseTracer(cfg LangfuseConfig, logger *log.Logger) *LangfuseTracer {
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}

	auth := base64.StdEncoding.EncodeToString([]byte(cfg.PublicKey + ":" + cfg.SecretKey))

	t := &LangfuseTracer{
		config:     cfg,
		authHeader: "Basic " + auth,
		client:     &http.Client{Timeout: 10 * time.Second},
		events:     make(chan ingestionEvent, eventBufferSize),
		logger:     logger,
		stopCh:     make(chan struct{}),
	}

	t.wg.Add(1)
	go t.flushLoop()

	return t
}

// StartTrace creates a new Langfuse trace for a task.
func (t *LangfuseTracer) StartTrace(taskID string, opts TraceOptions) TraceContext {
	traceID := taskID // Use task ID as trace ID for easy lookup

	t.enqueue(ingestionEvent{
		Type: "trace-create",
		Body: map[string]interface{}{
			"id":   traceID,
			"name": opts.Workflow,
			"metadata": map[string]interface{}{
				"repository": opts.Repository,
				"session_id": opts.SessionID,
				"workflow":   opts.Workflow,
			},
		},
	})

	return TraceContext{
		TraceID: traceID,
		TaskID:  taskID,
		Metadata: map[string]string{
			"workflow":   opts.Workflow,
			"repository": opts.Repository,
		},
	}
}

// StartPhase creates a new Langfuse span for a phase within a trace.
func (t *LangfuseTracer) StartPhase(trace TraceContext, phase string, opts SpanOptions) SpanContext {
	spanID := uuid.New().String()

	metadata := map[string]interface{}{
		"max_iterations": opts.MaxIterations,
	}
	if opts.Iteration > 0 {
		metadata["iteration"] = opts.Iteration
	}
	for k, v := range opts.Metadata {
		metadata[k] = v
	}

	t.enqueue(ingestionEvent{
		Type: "span-create",
		Body: map[string]interface{}{
			"id":        spanID,
			"traceId":   trace.TraceID,
			"name":      phase,
			"metadata":  metadata,
			"startTime": time.Now().UTC().Format(time.RFC3339Nano),
		},
	})

	return SpanContext{
		SpanID:    spanID,
		PhaseName: phase,
		TraceID:   trace.TraceID,
	}
}

// RecordGeneration records an LLM invocation as a Langfuse generation.
func (t *LangfuseTracer) RecordGeneration(span SpanContext, gen GenerationInput) {
	t.enqueue(ingestionEvent{
		Type: "generation-create",
		Body: map[string]interface{}{
			"id":                  uuid.New().String(),
			"traceId":             span.TraceID,
			"parentObservationId": span.SpanID,
			"name":                gen.Name,
			"model":               gen.Model,
			"input":               gen.Input,
			"usage": map[string]interface{}{
				"input":  gen.InputTokens,
				"output": gen.OutputTokens,
			},
			"metadata": map[string]interface{}{
				"status":      gen.Status,
				"duration_ms": gen.DurationMs,
			},
			"startTime": time.Now().UTC().Format(time.RFC3339Nano),
		},
	})
}

// RecordSkipped records a skipped component as a Langfuse event.
func (t *LangfuseTracer) RecordSkipped(span SpanContext, component string, reason string) {
	t.enqueue(ingestionEvent{
		Type: "event-create",
		Body: map[string]interface{}{
			"id":                  uuid.New().String(),
			"traceId":             span.TraceID,
			"parentObservationId": span.SpanID,
			"name":                component + " Skipped",
			"metadata": map[string]interface{}{
				"skip_reason": reason,
			},
			"startTime": time.Now().UTC().Format(time.RFC3339Nano),
		},
	})
}

// EndPhase closes a Langfuse span with a status and duration.
func (t *LangfuseTracer) EndPhase(span SpanContext, status string, durationMs int64) {
	t.enqueue(ingestionEvent{
		Type: "span-update",
		Body: map[string]interface{}{
			"id":      span.SpanID,
			"traceId": span.TraceID,
			"metadata": map[string]interface{}{
				"status":      status,
				"duration_ms": durationMs,
			},
			"endTime": time.Now().UTC().Format(time.RFC3339Nano),
		},
	})
}

// CompleteTrace updates a Langfuse trace with final status and token totals.
func (t *LangfuseTracer) CompleteTrace(trace TraceContext, opts CompleteOptions) {
	t.enqueue(ingestionEvent{
		Type: "trace-create",
		Body: map[string]interface{}{
			"id": trace.TraceID,
			"metadata": map[string]interface{}{
				"status":              opts.Status,
				"total_input_tokens":  opts.TotalInputTokens,
				"total_output_tokens": opts.TotalOutputTokens,
			},
		},
	})
}

// Flush sends all buffered events to Langfuse and waits for completion.
// Safe to call concurrently with the background flush loop.
func (t *LangfuseTracer) Flush(ctx context.Context) error {
	t.flushMu.Lock()
	defer t.flushMu.Unlock()

	var batch []ingestionEvent
	for {
		select {
		case evt := <-t.events:
			batch = append(batch, evt)
		default:
			// Channel drained
			if len(batch) > 0 {
				if err := t.sendBatchWithRetry(ctx, batch); err != nil {
					return fmt.Errorf("langfuse flush: %w", err)
				}
			}
			return nil
		}
	}
}

// enqueue adds an event to the buffer. If the buffer is full, the event
// is dropped with a warning log.
func (t *LangfuseTracer) enqueue(evt ingestionEvent) {
	evt.ID = uuid.New().String()
	evt.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)

	select {
	case t.events <- evt:
	default:
		t.logger.Printf("Warning: Langfuse event buffer full, dropping event: %s", evt.Type)
	}
}

// flushLoop periodically drains the event buffer and sends batches.
func (t *LangfuseTracer) flushLoop() {
	defer t.wg.Done()

	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-t.stopCh:
			// Final drain before exiting to minimize data loss
			t.drainAndSend()
			return
		case <-ticker.C:
			t.drainAndSend()
		}
	}
}

// drainAndSend collects all buffered events and sends them.
// Uses a mutex to prevent racing with concurrent Flush() calls.
func (t *LangfuseTracer) drainAndSend() {
	t.flushMu.Lock()
	defer t.flushMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var batch []ingestionEvent
	for {
		select {
		case evt := <-t.events:
			batch = append(batch, evt)
			if len(batch) >= maxBatchSize {
				if err := t.sendBatchWithRetry(ctx, batch); err != nil {
					t.logger.Printf("Warning: Langfuse batch send failed: %v", err)
				}
				batch = nil
			}
		default:
			if len(batch) > 0 {
				if err := t.sendBatchWithRetry(ctx, batch); err != nil {
					t.logger.Printf("Warning: Langfuse batch send failed: %v", err)
				}
			}
			return
		}
	}
}

// sendBatchWithRetry sends a batch with a single retry on failure.
func (t *LangfuseTracer) sendBatchWithRetry(ctx context.Context, batch []ingestionEvent) error {
	err := t.sendBatch(ctx, batch)
	if err == nil {
		return nil
	}
	t.logger.Printf("Warning: Langfuse batch send failed, retrying: %v", err)
	time.Sleep(retryDelay)
	return t.sendBatch(ctx, batch)
}

// sendBatch sends a batch of events to the Langfuse ingestion API.
func (t *LangfuseTracer) sendBatch(ctx context.Context, batch []ingestionEvent) error {
	payload := ingestionPayload{
		Batch: batch,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal batch: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.config.BaseURL+ingestionPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", t.authHeader)

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	if resp.StatusCode >= 400 {
		return fmt.Errorf("langfuse API returned %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse the response to detect per-event rejections.
	var result ingestionResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		t.logger.Printf("Warning: Langfuse: could not parse response body: %v", err)
		return nil
	}

	for _, e := range result.Errors {
		t.logger.Printf("Warning: Langfuse: event %s rejected (status=%d): %s", e.ID, e.Status, e.Message)
	}

	t.logger.Printf("Langfuse: batch sent (events=%d, accepted=%d, rejected=%d, status=%d)",
		len(batch), len(result.Successes), len(result.Errors), resp.StatusCode)

	return nil
}

// Stop shuts down the background flush goroutine and flushes remaining events.
// Safe to call multiple times; subsequent calls are no-ops.
func (t *LangfuseTracer) Stop(ctx context.Context) error {
	t.stopOnce.Do(func() { close(t.stopCh) })
	t.wg.Wait()
	// After goroutine exits, drain any stragglers enqueued after the final drain
	return t.Flush(ctx)
}

// Ping sends a minimal trace-create event to verify that the Langfuse API
// is reachable and credentials are valid. The trace is named
// "agentium-connectivity-test" so it is easy to identify in the Langfuse UI.
func (t *LangfuseTracer) Ping(ctx context.Context) error {
	event := ingestionEvent{
		ID:        uuid.New().String(),
		Type:      "trace-create",
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Body: map[string]interface{}{
			"id":   "agentium-ping-" + uuid.New().String(),
			"name": "agentium-connectivity-test",
		},
	}

	payload := ingestionPayload{Batch: []ingestionEvent{event}}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal ping: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.config.BaseURL+ingestionPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create ping request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", t.authHeader)

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("ping request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	if resp.StatusCode >= 400 {
		return fmt.Errorf("langfuse ping returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result ingestionResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("ping: could not parse response: %w", err)
	}

	if len(result.Errors) > 0 {
		return fmt.Errorf("ping event rejected: %s", result.Errors[0].Message)
	}

	return nil
}

// BaseURL returns the configured Langfuse base URL.
func (t *LangfuseTracer) BaseURL() string {
	return t.config.BaseURL
}

// ingestionEvent is a single event in the Langfuse ingestion API batch.
type ingestionEvent struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Timestamp string                 `json:"timestamp"`
	Body      map[string]interface{} `json:"body"`
}

// ingestionPayload is the top-level payload for the Langfuse ingestion API.
type ingestionPayload struct {
	Batch []ingestionEvent `json:"batch"`
}

// ingestionResponse is the Langfuse ingestion API response body.
type ingestionResponse struct {
	Successes []ingestionSuccess `json:"successes"`
	Errors    []ingestionError   `json:"errors"`
}

type ingestionSuccess struct {
	ID     string `json:"id"`
	Status int    `json:"status"`
}

type ingestionError struct {
	ID      string `json:"id"`
	Status  int    `json:"status"`
	Message string `json:"message,omitempty"`
	Error   any    `json:"error,omitempty"`
}
