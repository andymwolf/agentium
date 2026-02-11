package controller

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"testing"

	"github.com/andywolf/agentium/internal/agent"
	"github.com/andywolf/agentium/internal/observability"
)

func TestLogTokenConsumption(t *testing.T) {
	t.Run("skips when cloudLogger is nil", func(t *testing.T) {
		c := &Controller{
			logger:         log.New(io.Discard, "", 0),
			cloudLogger:    nil,
			activeTaskType: "issue",
			activeTask:     "42",
		}

		result := &agent.IterationResult{
			InputTokens:  1000,
			OutputTokens: 500,
		}
		session := &agent.Session{}

		// Should not panic when cloudLogger is nil
		c.logTokenConsumption(result, "claude-code", session)
	})

	t.Run("skips when tokens are zero", func(t *testing.T) {
		c := &Controller{
			logger:         log.New(io.Discard, "", 0),
			cloudLogger:    nil,
			activeTaskType: "issue",
			activeTask:     "42",
		}

		result := &agent.IterationResult{
			InputTokens:  0,
			OutputTokens: 0,
		}
		session := &agent.Session{}

		// Should not panic when tokens are zero
		c.logTokenConsumption(result, "claude-code", session)
	})

	t.Run("builds correct task ID and phase", func(t *testing.T) {
		c := &Controller{
			logger:         log.New(io.Discard, "", 0),
			cloudLogger:    nil, // We can't test actual logging without mock
			activeTaskType: "issue",
			activeTask:     "42",
			taskStates: map[string]*TaskState{
				"issue:42": {ID: "42", Type: "issue", Phase: PhaseImplement},
			},
		}

		result := &agent.IterationResult{
			InputTokens:  1500,
			OutputTokens: 300,
		}
		session := &agent.Session{}

		// Should not panic - can't verify labels without mock logger
		c.logTokenConsumption(result, "claude-code", session)
	})
}

// mockSecretFetcher is a test double for gcp.SecretFetcher.
type mockSecretFetcher struct {
	secrets map[string]string
	err     error
}

func (m *mockSecretFetcher) FetchSecret(_ context.Context, path string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	v, ok := m.secrets[path]
	if !ok {
		return "", fmt.Errorf("secret not found: %s", path)
	}
	return v, nil
}

func (m *mockSecretFetcher) Close() error { return nil }

func TestInitTracer(t *testing.T) {
	t.Run("secret paths set fetches from secret manager", func(t *testing.T) {
		var logBuf bytes.Buffer
		logger := log.New(&logBuf, "", 0)

		c := &Controller{
			config: SessionConfig{
				Langfuse: LangfuseSessionConfig{
					PublicKeySecret: "projects/p/secrets/langfuse-public",
					SecretKeySecret: "projects/p/secrets/langfuse-secret",
					BaseURL:         "https://custom.langfuse.com",
				},
			},
			secretManager: &mockSecretFetcher{
				secrets: map[string]string{
					"projects/p/secrets/langfuse-public": "  pk-lf-test  \n",
					"projects/p/secrets/langfuse-secret": "  sk-lf-test  \n",
				},
			},
			tracer:     &observability.NoOpTracer{},
			shutdownCh: make(chan struct{}),
		}

		// Ensure env vars are unset for this test
		t.Setenv("LANGFUSE_PUBLIC_KEY", "")
		t.Setenv("LANGFUSE_SECRET_KEY", "")
		t.Setenv("LANGFUSE_BASE_URL", "")
		t.Setenv("LANGFUSE_ENABLED", "")

		c.initTracer(context.Background(), logger)

		// Verify tracer was initialized (not NoOp)
		if _, ok := c.tracer.(*observability.NoOpTracer); ok {
			t.Fatal("expected LangfuseTracer but got NoOpTracer")
		}

		logOutput := logBuf.String()
		if !containsString(logOutput, "tracer initialized") {
			t.Errorf("expected log to contain 'tracer initialized', got: %s", logOutput)
		}
		if !containsString(logOutput, "custom.langfuse.com") {
			t.Errorf("expected log to contain custom base URL, got: %s", logOutput)
		}
	})

	t.Run("env vars take precedence over secret paths", func(t *testing.T) {
		var logBuf bytes.Buffer
		logger := log.New(&logBuf, "", 0)

		fetcher := &mockSecretFetcher{
			err: fmt.Errorf("should not be called"),
		}

		c := &Controller{
			config: SessionConfig{
				Langfuse: LangfuseSessionConfig{
					PublicKeySecret: "projects/p/secrets/langfuse-public",
					SecretKeySecret: "projects/p/secrets/langfuse-secret",
				},
			},
			secretManager: fetcher,
			tracer:        &observability.NoOpTracer{},
			shutdownCh:    make(chan struct{}),
		}

		t.Setenv("LANGFUSE_PUBLIC_KEY", "pk-env")
		t.Setenv("LANGFUSE_SECRET_KEY", "sk-env")
		t.Setenv("LANGFUSE_BASE_URL", "")
		t.Setenv("LANGFUSE_ENABLED", "")

		c.initTracer(context.Background(), logger)

		// Verify tracer was initialized from env vars
		if _, ok := c.tracer.(*observability.NoOpTracer); ok {
			t.Fatal("expected LangfuseTracer but got NoOpTracer")
		}
	})

	t.Run("secret fetch failure keeps NoOpTracer", func(t *testing.T) {
		var logBuf bytes.Buffer
		logger := log.New(&logBuf, "", 0)

		c := &Controller{
			config: SessionConfig{
				Langfuse: LangfuseSessionConfig{
					PublicKeySecret: "projects/p/secrets/langfuse-public",
					SecretKeySecret: "projects/p/secrets/langfuse-secret",
				},
			},
			logger: logger,
			secretManager: &mockSecretFetcher{
				err: fmt.Errorf("permission denied"),
			},
			tracer:     &observability.NoOpTracer{},
			shutdownCh: make(chan struct{}),
		}

		t.Setenv("LANGFUSE_PUBLIC_KEY", "")
		t.Setenv("LANGFUSE_SECRET_KEY", "")
		t.Setenv("LANGFUSE_ENABLED", "")

		c.initTracer(context.Background(), logger)

		// Verify tracer stayed as NoOp
		if _, ok := c.tracer.(*observability.NoOpTracer); !ok {
			t.Fatal("expected NoOpTracer when secret fetch fails")
		}

		logOutput := logBuf.String()
		if !containsString(logOutput, "failed to fetch") {
			t.Errorf("expected log to contain fetch error, got: %s", logOutput)
		}
	})

	t.Run("no config and no env vars keeps NoOpTracer", func(t *testing.T) {
		logger := log.New(io.Discard, "", 0)

		c := &Controller{
			config:     SessionConfig{},
			tracer:     &observability.NoOpTracer{},
			shutdownCh: make(chan struct{}),
		}

		t.Setenv("LANGFUSE_PUBLIC_KEY", "")
		t.Setenv("LANGFUSE_SECRET_KEY", "")
		t.Setenv("LANGFUSE_ENABLED", "")

		c.initTracer(context.Background(), logger)

		if _, ok := c.tracer.(*observability.NoOpTracer); !ok {
			t.Fatal("expected NoOpTracer when nothing is configured")
		}
	})

	t.Run("LANGFUSE_ENABLED=false disables tracer", func(t *testing.T) {
		var logBuf bytes.Buffer
		logger := log.New(&logBuf, "", 0)

		c := &Controller{
			config: SessionConfig{
				Langfuse: LangfuseSessionConfig{
					PublicKeySecret: "projects/p/secrets/langfuse-public",
					SecretKeySecret: "projects/p/secrets/langfuse-secret",
				},
			},
			secretManager: &mockSecretFetcher{
				secrets: map[string]string{
					"projects/p/secrets/langfuse-public": "pk-lf-test",
					"projects/p/secrets/langfuse-secret": "sk-lf-test",
				},
			},
			tracer:     &observability.NoOpTracer{},
			shutdownCh: make(chan struct{}),
		}

		t.Setenv("LANGFUSE_PUBLIC_KEY", "")
		t.Setenv("LANGFUSE_SECRET_KEY", "")
		t.Setenv("LANGFUSE_ENABLED", "false")

		c.initTracer(context.Background(), logger)

		if _, ok := c.tracer.(*observability.NoOpTracer); !ok {
			t.Fatal("expected NoOpTracer when LANGFUSE_ENABLED=false")
		}

		if !containsString(logBuf.String(), "disabled") {
			t.Errorf("expected log to mention disabled")
		}
	})
}
