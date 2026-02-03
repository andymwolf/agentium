package controller

import (
	"errors"
	"testing"
	"time"

	"github.com/andywolf/agentium/internal/agent"
)

func TestIsAdapterExecutionFailure(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		stderr   string
		duration time.Duration
		want     bool
	}{
		{
			name:     "nil error returns false",
			err:      nil,
			stderr:   "",
			duration: time.Minute,
			want:     false,
		},
		{
			name:     "EISDIR in error message",
			err:      errors.New("Is a directory (os error 21)"),
			stderr:   "",
			duration: time.Minute,
			want:     true,
		},
		{
			name:     "is a directory in stderr",
			err:      errors.New("container failed"),
			stderr:   "Error: Is a directory",
			duration: time.Minute,
			want:     true,
		},
		{
			name:     "ENOENT error",
			err:      errors.New("no such file or directory"),
			stderr:   "",
			duration: time.Minute,
			want:     true,
		},
		{
			name:     "permission denied",
			err:      errors.New("permission denied"),
			stderr:   "",
			duration: time.Minute,
			want:     true,
		},
		{
			name:     "docker error",
			err:      errors.New("docker: error response from daemon"),
			stderr:   "",
			duration: time.Minute,
			want:     true,
		},
		{
			name:     "no such image",
			err:      errors.New("no such image: ghcr.io/foo/bar:latest"),
			stderr:   "",
			duration: time.Minute,
			want:     true,
		},
		{
			name:     "connection refused",
			err:      errors.New("dial tcp 127.0.0.1:2375: connect: connection refused"),
			stderr:   "",
			duration: time.Minute,
			want:     true,
		},
		{
			name:     "auth file error",
			err:      errors.New("auth file not found"),
			stderr:   "",
			duration: time.Minute,
			want:     true,
		},
		{
			name:     "OCI runtime error",
			err:      errors.New("OCI runtime create failed"),
			stderr:   "",
			duration: time.Minute,
			want:     true,
		},
		{
			name:     "short execution with error is startup failure",
			err:      errors.New("exit status 1"),
			stderr:   "unknown error",
			duration: 5 * time.Second,
			want:     true,
		},
		{
			name:     "normal task failure after long execution",
			err:      errors.New("exit status 1"),
			stderr:   "test failed",
			duration: 5 * time.Minute,
			want:     false,
		},
		{
			name:     "error without known patterns after 30+ seconds",
			err:      errors.New("something went wrong"),
			stderr:   "details here",
			duration: 45 * time.Second,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isAdapterExecutionFailure(tt.err, tt.stderr, tt.duration)
			if got != tt.want {
				t.Errorf("isAdapterExecutionFailure(%v, %q, %v) = %v, want %v",
					tt.err, tt.stderr, tt.duration, got, tt.want)
			}
		})
	}
}

func TestGetFallbackAdapter(t *testing.T) {
	tests := []struct {
		name   string
		config SessionConfig
		want   string
	}{
		{
			name:   "nil fallback config returns empty",
			config: SessionConfig{},
			want:   "",
		},
		{
			name: "disabled fallback returns empty",
			config: SessionConfig{
				Fallback: &FallbackConfig{Enabled: false},
			},
			want: "",
		},
		{
			name: "enabled with no adapter uses default",
			config: SessionConfig{
				Fallback: &FallbackConfig{Enabled: true},
			},
			want: DefaultFallbackAdapter,
		},
		{
			name: "enabled with custom adapter",
			config: SessionConfig{
				Fallback: &FallbackConfig{
					Enabled:        true,
					DefaultAdapter: "aider",
				},
			},
			want: "aider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Controller{config: tt.config}
			got := c.getFallbackAdapter()
			if got != tt.want {
				t.Errorf("getFallbackAdapter() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCanFallback(t *testing.T) {
	tests := []struct {
		name           string
		config         SessionConfig
		adapterNames   []string // adapters to add
		currentAdapter string
		want           bool
	}{
		{
			name:           "fallback disabled returns false",
			config:         SessionConfig{},
			adapterNames:   []string{"claude-code"},
			currentAdapter: "codex",
			want:           false,
		},
		{
			name: "current adapter is fallback returns false",
			config: SessionConfig{
				Fallback: &FallbackConfig{
					Enabled:        true,
					DefaultAdapter: "claude-code",
				},
			},
			adapterNames:   []string{"claude-code"},
			currentAdapter: "claude-code",
			want:           false,
		},
		{
			name: "fallback adapter not in adapters returns false",
			config: SessionConfig{
				Fallback: &FallbackConfig{
					Enabled:        true,
					DefaultAdapter: "claude-code",
				},
			},
			adapterNames:   []string{"codex"},
			currentAdapter: "codex",
			want:           false,
		},
		{
			name: "can fallback from codex to claude-code",
			config: SessionConfig{
				Fallback: &FallbackConfig{
					Enabled:        true,
					DefaultAdapter: "claude-code",
				},
			},
			adapterNames:   []string{"claude-code", "codex"},
			currentAdapter: "codex",
			want:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Controller{
				config:   tt.config,
				adapters: make(map[string]agent.Agent),
			}
			for _, name := range tt.adapterNames {
				c.adapters[name] = &mockFallbackAgent{name: name}
			}

			got := c.canFallback(tt.currentAdapter)
			if got != tt.want {
				t.Errorf("canFallback(%q) = %v, want %v", tt.currentAdapter, got, tt.want)
			}
		})
	}
}

func TestValidateAuthFile(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		authName  string
		wantErr   bool
		errSubstr string
	}{
		{
			name:      "file does not exist",
			path:      "/nonexistent/path/to/file.json",
			authName:  "Claude",
			wantErr:   true,
			errSubstr: "not found",
		},
		{
			name:      "path is a directory",
			path:      "/tmp",
			authName:  "Codex",
			wantErr:   true,
			errSubstr: "is a directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Controller{}
			err := c.validateAuthFile(tt.path, tt.authName)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateAuthFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil {
				if tt.errSubstr != "" && !containsIgnoreCase(err.Error(), tt.errSubstr) {
					t.Errorf("validateAuthFile() error = %q, want to contain %q", err.Error(), tt.errSubstr)
				}
			}
		})
	}
}

func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		len(substr) == 0 ||
		(len(s) > 0 && containsSubstring(s, substr)))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if equalFoldSubstring(s[i:i+len(substr)], substr) {
			return true
		}
	}
	return false
}

func equalFoldSubstring(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// mockFallbackAgent implements agent.Agent for testing
type mockFallbackAgent struct {
	name string
}

func (m *mockFallbackAgent) Name() string               { return m.name }
func (m *mockFallbackAgent) ContainerImage() string     { return "test-image" }
func (m *mockFallbackAgent) BuildEnv(s *agent.Session, i int) map[string]string {
	return nil
}
func (m *mockFallbackAgent) BuildCommand(s *agent.Session, i int) []string { return nil }
func (m *mockFallbackAgent) BuildPrompt(s *agent.Session, i int) string    { return "" }
func (m *mockFallbackAgent) ParseOutput(exit int, stdout, stderr string) (*agent.IterationResult, error) {
	return nil, nil
}
func (m *mockFallbackAgent) Validate() error { return nil }
