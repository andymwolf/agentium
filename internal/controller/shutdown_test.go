package controller

import (
	"context"
	"errors"
	"io"
	"log"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGracefulShutdown_FlushLogs(t *testing.T) {
	c := &Controller{
		logger:     newTestLogger(),
		shutdownCh: make(chan struct{}),
	}

	flushed := false
	c.logFlushFn = func() error {
		flushed = true
		return nil
	}

	c.gracefulShutdown()

	if !flushed {
		t.Error("expected logFlushFn to be called during shutdown")
	}
}

func TestGracefulShutdown_FlushLogsWithError(t *testing.T) {
	c := &Controller{
		logger:     newTestLogger(),
		shutdownCh: make(chan struct{}),
	}

	c.logFlushFn = func() error {
		return errors.New("flush error")
	}

	// Should not panic even if flush returns error
	c.gracefulShutdown()
}

func TestGracefulShutdown_FlushLogsTimeout(t *testing.T) {
	c := &Controller{
		logger:     newTestLogger(),
		shutdownCh: make(chan struct{}),
	}

	// Simulate a log flush that hangs
	c.logFlushFn = func() error {
		time.Sleep(5 * time.Second) // Longer than LogFlushTimeout
		return nil
	}

	start := time.Now()
	c.gracefulShutdown()
	elapsed := time.Since(start)

	// Should complete within LogFlushTimeout + small buffer, not wait for full 5s
	if elapsed > LogFlushTimeout+2*time.Second {
		t.Errorf("shutdown took %v, expected to timeout within %v", elapsed, LogFlushTimeout+2*time.Second)
	}
}

func TestGracefulShutdown_NoFlushFunc(t *testing.T) {
	c := &Controller{
		logger:     newTestLogger(),
		shutdownCh: make(chan struct{}),
	}

	// No logFlushFn set - should not panic
	c.gracefulShutdown()
}

func TestGracefulShutdown_RunsHooksInOrder(t *testing.T) {
	c := &Controller{
		logger:     newTestLogger(),
		shutdownCh: make(chan struct{}),
	}

	var order []int
	var mu sync.Mutex

	c.AddShutdownHook(func(ctx context.Context) error {
		mu.Lock()
		order = append(order, 1)
		mu.Unlock()
		return nil
	})
	c.AddShutdownHook(func(ctx context.Context) error {
		mu.Lock()
		order = append(order, 2)
		mu.Unlock()
		return nil
	})
	c.AddShutdownHook(func(ctx context.Context) error {
		mu.Lock()
		order = append(order, 3)
		mu.Unlock()
		return nil
	})

	c.gracefulShutdown()

	mu.Lock()
	defer mu.Unlock()

	if len(order) != 3 {
		t.Fatalf("expected 3 hooks to run, got %d", len(order))
	}
	for i, v := range order {
		if v != i+1 {
			t.Errorf("hook %d ran in position %d", v, i)
		}
	}
}

func TestGracefulShutdown_HookError(t *testing.T) {
	c := &Controller{
		logger:     newTestLogger(),
		shutdownCh: make(chan struct{}),
	}

	hookCalled := false
	c.AddShutdownHook(func(ctx context.Context) error {
		return errors.New("hook error")
	})
	c.AddShutdownHook(func(ctx context.Context) error {
		hookCalled = true
		return nil
	})

	// Should continue to next hook even if one fails
	c.gracefulShutdown()

	if !hookCalled {
		t.Error("second hook should still be called after first hook error")
	}
}

func TestGracefulShutdown_ClearsSensitiveData(t *testing.T) {
	c := &Controller{
		logger:      newTestLogger(),
		shutdownCh:  make(chan struct{}),
		gitHubToken: "secret-token-123",
		config: SessionConfig{
			Prompt: "sensitive prompt content",
		},
	}
	c.config.ClaudeAuth.AuthJSONBase64 = "base64-credentials"
	c.config.GitHub.PrivateKeySecret = "projects/x/secrets/key"

	c.gracefulShutdown()

	if c.gitHubToken != "" {
		t.Error("gitHubToken should be cleared")
	}
	if c.config.Prompt != "" {
		t.Error("prompt should be cleared")
	}
	if c.config.ClaudeAuth.AuthJSONBase64 != "" {
		t.Error("AuthJSONBase64 should be cleared")
	}
	if c.config.GitHub.PrivateKeySecret != "" {
		t.Error("PrivateKeySecret should be cleared")
	}
}

func TestGracefulShutdown_OnlyRunsOnce(t *testing.T) {
	c := &Controller{
		logger:     newTestLogger(),
		shutdownCh: make(chan struct{}),
	}

	var callCount int32
	c.logFlushFn = func() error {
		atomic.AddInt32(&callCount, 1)
		return nil
	}

	// Call shutdown multiple times concurrently
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.gracefulShutdown()
		}()
	}
	wg.Wait()

	count := atomic.LoadInt32(&callCount)
	if count != 1 {
		t.Errorf("expected flush to be called exactly once, got %d", count)
	}
}

func TestGracefulShutdown_ClosesShutdownChannel(t *testing.T) {
	c := &Controller{
		logger:     newTestLogger(),
		shutdownCh: make(chan struct{}),
	}

	c.gracefulShutdown()

	// Channel should be closed
	select {
	case <-c.shutdownCh:
		// Expected - channel is closed
	default:
		t.Error("shutdownCh should be closed after gracefulShutdown")
	}
}

func TestSetupSignalHandler_CancelContext(t *testing.T) {
	c := &Controller{
		logger:     newTestLogger(),
		shutdownCh: make(chan struct{}),
	}

	ctx := context.Background()
	ctx, cancel := c.setupSignalHandler(ctx)
	defer cancel()

	// Cancel should work without error
	cancel()

	select {
	case <-ctx.Done():
		// Expected
	case <-time.After(time.Second):
		t.Error("context should be cancelled immediately")
	}
}

func TestAddShutdownHook(t *testing.T) {
	c := &Controller{
		logger:     newTestLogger(),
		shutdownCh: make(chan struct{}),
	}

	if len(c.shutdownHooks) != 0 {
		t.Errorf("expected 0 hooks initially, got %d", len(c.shutdownHooks))
	}

	c.AddShutdownHook(func(ctx context.Context) error { return nil })
	c.AddShutdownHook(func(ctx context.Context) error { return nil })

	if len(c.shutdownHooks) != 2 {
		t.Errorf("expected 2 hooks, got %d", len(c.shutdownHooks))
	}
}

func TestSetLogFlushFunc(t *testing.T) {
	c := &Controller{
		logger:     newTestLogger(),
		shutdownCh: make(chan struct{}),
	}

	called := false
	c.SetLogFlushFunc(func() error {
		called = true
		return nil
	})

	c.gracefulShutdown()

	if !called {
		t.Error("expected SetLogFlushFunc callback to be called during shutdown")
	}
}

func TestFlushLogs_RespectsContextTimeout(t *testing.T) {
	c := &Controller{
		logger:     newTestLogger(),
		shutdownCh: make(chan struct{}),
	}

	c.logFlushFn = func() error {
		time.Sleep(5 * time.Second)
		return nil
	}

	// Use a very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	c.flushLogs(ctx)
	elapsed := time.Since(start)

	// Should timeout quickly
	if elapsed > 2*time.Second {
		t.Errorf("flushLogs should respect context timeout, took %v", elapsed)
	}
}

func TestGracefulShutdown_ShutdownSequenceOrder(t *testing.T) {
	c := &Controller{
		logger:      newTestLogger(),
		shutdownCh:  make(chan struct{}),
		gitHubToken: "token",
	}

	var sequence []string
	var mu sync.Mutex

	c.logFlushFn = func() error {
		mu.Lock()
		sequence = append(sequence, "flush")
		mu.Unlock()
		return nil
	}

	c.AddShutdownHook(func(ctx context.Context) error {
		mu.Lock()
		sequence = append(sequence, "hook")
		mu.Unlock()
		return nil
	})

	c.gracefulShutdown()

	mu.Lock()
	defer mu.Unlock()

	// Verify order: flush before hooks
	if len(sequence) < 2 {
		t.Fatalf("expected at least 2 sequence entries, got %d", len(sequence))
	}
	if sequence[0] != "flush" {
		t.Errorf("expected flush first, got %q", sequence[0])
	}
	if sequence[1] != "hook" {
		t.Errorf("expected hook second, got %q", sequence[1])
	}

	// Verify sensitive data was cleared (happens after hooks)
	if c.gitHubToken != "" {
		t.Error("sensitive data should be cleared after hooks run")
	}
}

// newTestLogger creates a logger for testing that discards output
func newTestLogger() *log.Logger {
	return log.New(io.Discard, "[test] ", log.LstdFlags)
}
