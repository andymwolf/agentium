package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func newTestLogger() *log.Logger {
	return log.New(io.Discard, "", 0)
}

func TestNoOpTracer(t *testing.T) {
	tracer := &NoOpTracer{}

	// All methods should be callable without panic
	trace := tracer.StartTrace("task-1", TraceOptions{Workflow: "default"})
	span := tracer.StartPhase(trace, "PLAN", SpanOptions{})
	tracer.RecordGeneration(span, GenerationInput{
		Name:         "Worker",
		InputTokens:  100,
		OutputTokens: 50,
	})
	tracer.RecordSkipped(span, "Reviewer", "empty_output")
	tracer.EndPhase(span, "completed", 1000)
	tracer.CompleteTrace(trace, CompleteOptions{Status: "completed"})

	if err := tracer.Flush(context.Background()); err != nil {
		t.Errorf("NoOpTracer.Flush() returned error: %v", err)
	}
	if err := tracer.Stop(context.Background()); err != nil {
		t.Errorf("NoOpTracer.Stop() returned error: %v", err)
	}
}

func TestNoOpTracerInterface(t *testing.T) {
	// Verify NoOpTracer satisfies the Tracer interface
	var _ Tracer = &NoOpTracer{}
}

func TestLangfuseTracerInterface(t *testing.T) {
	// Verify LangfuseTracer satisfies the Tracer interface
	var _ Tracer = &LangfuseTracer{}
}

func TestLangfuseTracerSendsBatches(t *testing.T) {
	var mu sync.Mutex
	var receivedBatches []ingestionPayload

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != ingestionPath {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		// Verify auth header
		auth := r.Header.Get("Authorization")
		if auth == "" {
			t.Error("missing Authorization header")
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read body: %v", err)
			http.Error(w, "read error", http.StatusInternalServerError)
			return
		}

		var payload ingestionPayload
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("failed to unmarshal body: %v", err)
			http.Error(w, "parse error", http.StatusBadRequest)
			return
		}

		mu.Lock()
		receivedBatches = append(receivedBatches, payload)
		mu.Unlock()

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"successes":[],"errors":[]}`))
	}))
	defer server.Close()

	tracer := NewLangfuseTracer(LangfuseConfig{
		PublicKey: "pk-test",
		SecretKey: "sk-test",
		BaseURL:   server.URL,
	}, newTestLogger())

	// Record a full trace lifecycle
	trace := tracer.StartTrace("task-123", TraceOptions{
		Workflow:   "default",
		Repository: "owner/repo",
		SessionID:  "session-1",
	})

	span := tracer.StartPhase(trace, "IMPLEMENT", SpanOptions{
		Iteration:     1,
		MaxIterations: 5,
	})

	tracer.RecordGeneration(span, GenerationInput{
		Name:         "Worker",
		Model:        "claude-sonnet-4-20250514",
		InputTokens:  1500,
		OutputTokens: 300,
		Status:       "completed",
		DurationMs:   5000,
	})

	tracer.RecordSkipped(span, "Reviewer", "reviewer_skip=true")

	tracer.RecordGeneration(span, GenerationInput{
		Name:         "Judge",
		Model:        "claude-sonnet-4-20250514",
		InputTokens:  800,
		OutputTokens: 50,
		Status:       "completed",
		DurationMs:   2000,
	})

	tracer.EndPhase(span, "completed", 7000)
	tracer.CompleteTrace(trace, CompleteOptions{
		Status:            "completed",
		TotalInputTokens:  2300,
		TotalOutputTokens: 350,
	})

	// Stop flushes remaining events and shuts down the background goroutine
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := tracer.Stop(ctx); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	// Verify we received events
	totalEvents := 0
	for _, batch := range receivedBatches {
		totalEvents += len(batch.Batch)
	}

	// Expected: trace-create, span-create, generation-create (Worker),
	// event-create (Reviewer skipped), generation-create (Judge),
	// span-update, trace-create (complete)
	expectedEvents := 7
	if totalEvents != expectedEvents {
		t.Errorf("expected %d events, got %d", expectedEvents, totalEvents)
		for i, batch := range receivedBatches {
			for j, evt := range batch.Batch {
				t.Logf("batch[%d][%d]: type=%s", i, j, evt.Type)
			}
		}
	}

	// Verify event types
	eventTypes := make(map[string]int)
	for _, batch := range receivedBatches {
		for _, evt := range batch.Batch {
			eventTypes[evt.Type]++
		}
	}

	expectations := map[string]int{
		"trace-create":      2, // create + complete
		"span-create":       1,
		"generation-create": 2, // Worker + Judge
		"event-create":      1, // Reviewer skipped
		"span-update":       1,
	}

	for evtType, expected := range expectations {
		if got := eventTypes[evtType]; got != expected {
			t.Errorf("expected %d %s events, got %d", expected, evtType, got)
		}
	}
}

func TestLangfuseTracerAuthHeader(t *testing.T) {
	var receivedAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"successes":[],"errors":[]}`))
	}))
	defer server.Close()

	tracer := NewLangfuseTracer(LangfuseConfig{
		PublicKey: "pk-abc",
		SecretKey: "sk-xyz",
		BaseURL:   server.URL,
	}, newTestLogger())

	tracer.StartTrace("task-1", TraceOptions{})

	ctx := context.Background()
	if err := tracer.Flush(ctx); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}
	_ = tracer.Stop(ctx)

	// Verify Basic auth: base64("pk-abc:sk-xyz")
	expectedAuth := "Basic cGstYWJjOnNrLXh5eg=="
	if receivedAuth != expectedAuth {
		t.Errorf("expected auth %q, got %q", expectedAuth, receivedAuth)
	}
}

func TestLangfuseTracerDefaultBaseURL(t *testing.T) {
	tracer := NewLangfuseTracer(LangfuseConfig{
		PublicKey: "pk",
		SecretKey: "sk",
	}, newTestLogger())
	defer func() { _ = tracer.Stop(context.Background()) }()

	if tracer.config.BaseURL != defaultBaseURL {
		t.Errorf("expected default base URL %q, got %q", defaultBaseURL, tracer.config.BaseURL)
	}
}

func TestLangfuseTracerAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer server.Close()

	tracer := NewLangfuseTracer(LangfuseConfig{
		PublicKey: "bad-key",
		SecretKey: "bad-secret",
		BaseURL:   server.URL,
	}, newTestLogger())

	tracer.StartTrace("task-1", TraceOptions{})

	err := tracer.Flush(context.Background())
	if err == nil {
		t.Error("expected error for 401 response, got nil")
	}
	_ = tracer.Stop(context.Background())
}

func TestLangfuseTracerTraceContext(t *testing.T) {
	tracer := NewLangfuseTracer(LangfuseConfig{
		PublicKey: "pk",
		SecretKey: "sk",
		BaseURL:   "http://localhost:1", // Won't connect; we only test context creation
	}, newTestLogger())
	defer func() { _ = tracer.Stop(context.Background()) }()

	trace := tracer.StartTrace("issue:42", TraceOptions{
		Workflow:   "default",
		Repository: "owner/repo",
		SessionID:  "sess-1",
	})

	if trace.TraceID != "issue:42" {
		t.Errorf("expected TraceID 'issue:42', got %q", trace.TraceID)
	}
	if trace.TaskID != "issue:42" {
		t.Errorf("expected TaskID 'issue:42', got %q", trace.TaskID)
	}
	if trace.Metadata["workflow"] != "default" {
		t.Errorf("expected workflow 'default', got %q", trace.Metadata["workflow"])
	}
}

func TestLangfuseTracerSpanContext(t *testing.T) {
	tracer := NewLangfuseTracer(LangfuseConfig{
		PublicKey: "pk",
		SecretKey: "sk",
		BaseURL:   "http://localhost:1",
	}, newTestLogger())
	defer func() { _ = tracer.Stop(context.Background()) }()

	trace := tracer.StartTrace("task-1", TraceOptions{})
	span := tracer.StartPhase(trace, "PLAN", SpanOptions{
		Iteration:     2,
		MaxIterations: 3,
	})

	if span.PhaseName != "PLAN" {
		t.Errorf("expected PhaseName 'PLAN', got %q", span.PhaseName)
	}
	if span.TraceID != trace.TraceID {
		t.Errorf("expected span TraceID %q, got %q", trace.TraceID, span.TraceID)
	}
	if span.SpanID == "" {
		t.Error("expected non-empty SpanID")
	}
}

// newCapturingLogger returns a logger that writes to a buffer for assertions.
func newCapturingLogger() (*log.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return log.New(&buf, "", 0), &buf
}

func TestLangfuseTracerResponseParsing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"successes":[{"id":"a","status":201},{"id":"b","status":201}],"errors":[]}`))
	}))
	defer server.Close()

	logger, buf := newCapturingLogger()
	tracer := NewLangfuseTracer(LangfuseConfig{
		PublicKey: "pk-test",
		SecretKey: "sk-test",
		BaseURL:   server.URL,
	}, logger)

	tracer.StartTrace("task-1", TraceOptions{Workflow: "test"})

	ctx := context.Background()
	if err := tracer.Flush(ctx); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}
	_ = tracer.Stop(ctx)

	output := buf.String()
	if !strings.Contains(output, "batch sent") {
		t.Errorf("expected summary log, got: %s", output)
	}
	if !strings.Contains(output, "accepted=2") {
		t.Errorf("expected accepted=2 in log, got: %s", output)
	}
	if !strings.Contains(output, "rejected=0") {
		t.Errorf("expected rejected=0 in log, got: %s", output)
	}
}

func TestLangfuseTracerPartialRejection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = w.Write([]byte(`{"successes":[{"id":"a","status":201}],"errors":[{"id":"b","status":400,"message":"invalid field"}]}`))
	}))
	defer server.Close()

	logger, buf := newCapturingLogger()
	tracer := NewLangfuseTracer(LangfuseConfig{
		PublicKey: "pk-test",
		SecretKey: "sk-test",
		BaseURL:   server.URL,
	}, logger)

	tracer.StartTrace("task-1", TraceOptions{Workflow: "test"})
	tracer.StartTrace("task-2", TraceOptions{Workflow: "test"})

	ctx := context.Background()
	if err := tracer.Flush(ctx); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}
	_ = tracer.Stop(ctx)

	output := buf.String()
	if !strings.Contains(output, "event b rejected") {
		t.Errorf("expected per-event rejection log, got: %s", output)
	}
	if !strings.Contains(output, "invalid field") {
		t.Errorf("expected error message in log, got: %s", output)
	}
	if !strings.Contains(output, "accepted=1") {
		t.Errorf("expected accepted=1 in log, got: %s", output)
	}
	if !strings.Contains(output, "rejected=1") {
		t.Errorf("expected rejected=1 in log, got: %s", output)
	}
}

func TestLangfuseTracerAllEventsRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"successes":[],"errors":[{"id":"a","status":400,"message":"bad format"},{"id":"b","status":422,"message":"unknown type"}]}`))
	}))
	defer server.Close()

	logger, buf := newCapturingLogger()
	tracer := NewLangfuseTracer(LangfuseConfig{
		PublicKey: "pk-test",
		SecretKey: "sk-test",
		BaseURL:   server.URL,
	}, logger)

	tracer.StartTrace("task-1", TraceOptions{Workflow: "test"})
	tracer.StartTrace("task-2", TraceOptions{Workflow: "test"})

	ctx := context.Background()
	if err := tracer.Flush(ctx); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}
	_ = tracer.Stop(ctx)

	output := buf.String()
	if !strings.Contains(output, "event a rejected") {
		t.Errorf("expected rejection for event a, got: %s", output)
	}
	if !strings.Contains(output, "event b rejected") {
		t.Errorf("expected rejection for event b, got: %s", output)
	}
	if !strings.Contains(output, "accepted=0") {
		t.Errorf("expected accepted=0 in log, got: %s", output)
	}
	if !strings.Contains(output, "rejected=2") {
		t.Errorf("expected rejected=2 in log, got: %s", output)
	}
}

func TestLangfuseTracerMalformedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not json at all`))
	}))
	defer server.Close()

	logger, buf := newCapturingLogger()
	tracer := NewLangfuseTracer(LangfuseConfig{
		PublicKey: "pk-test",
		SecretKey: "sk-test",
		BaseURL:   server.URL,
	}, logger)

	tracer.StartTrace("task-1", TraceOptions{Workflow: "test"})

	ctx := context.Background()
	// Should not return an error — malformed response is logged but not fatal.
	if err := tracer.Flush(ctx); err != nil {
		t.Fatalf("Flush should not fail on malformed response, got: %v", err)
	}
	_ = tracer.Stop(ctx)

	output := buf.String()
	if !strings.Contains(output, "could not parse response") {
		t.Errorf("expected parse warning in log, got: %s", output)
	}
}

func TestLangfuseTracerPing(t *testing.T) {
	var mu sync.Mutex
	var receivedPayload ingestionPayload

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		_ = json.Unmarshal(body, &receivedPayload)
		mu.Unlock()

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"successes":[{"id":"ping","status":201}],"errors":[]}`))
	}))
	defer server.Close()

	tracer := NewLangfuseTracer(LangfuseConfig{
		PublicKey: "pk-test",
		SecretKey: "sk-test",
		BaseURL:   server.URL,
	}, newTestLogger())
	defer func() { _ = tracer.Stop(context.Background()) }()

	if err := tracer.Ping(context.Background()); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(receivedPayload.Batch) != 1 {
		t.Fatalf("expected 1 event in ping batch, got %d", len(receivedPayload.Batch))
	}

	evt := receivedPayload.Batch[0]
	if evt.Type != "trace-create" {
		t.Errorf("expected trace-create event, got %s", evt.Type)
	}
	name, _ := evt.Body["name"].(string)
	if name != "agentium-connectivity-test" {
		t.Errorf("expected name 'agentium-connectivity-test', got %q", name)
	}
	id, _ := evt.Body["id"].(string)
	if !strings.HasPrefix(id, "agentium-ping-") {
		t.Errorf("expected id prefix 'agentium-ping-', got %q", id)
	}
}

func TestLangfuseTracerPingAuthFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer server.Close()

	tracer := NewLangfuseTracer(LangfuseConfig{
		PublicKey: "bad-key",
		SecretKey: "bad-secret",
		BaseURL:   server.URL,
	}, newTestLogger())
	defer func() { _ = tracer.Stop(context.Background()) }()

	err := tracer.Ping(context.Background())
	if err == nil {
		t.Fatal("expected Ping to return error for 401 response")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected error to mention 401, got: %v", err)
	}
}

func TestLangfuseTracerDoubleStop(t *testing.T) {
	tracer := NewLangfuseTracer(LangfuseConfig{
		PublicKey: "pk",
		SecretKey: "sk",
		BaseURL:   "http://localhost:1",
	}, newTestLogger())

	ctx := context.Background()

	// First stop should succeed
	if err := tracer.Stop(ctx); err != nil {
		t.Fatalf("first Stop() returned error: %v", err)
	}

	// Second stop must not panic (double-close on channel)
	if err := tracer.Stop(ctx); err != nil {
		t.Fatalf("second Stop() returned error: %v", err)
	}
}
