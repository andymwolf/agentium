package provisioner

import (
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "gcp provider",
			provider: "gcp",
			wantErr:  false,
		},
		{
			name:     "aws provider not implemented",
			provider: "aws",
			wantErr:  true,
			errMsg:   "not yet implemented",
		},
		{
			name:     "azure provider not implemented",
			provider: "azure",
			wantErr:  true,
			errMsg:   "not yet implemented",
		},
		{
			name:     "unknown provider",
			provider: "unknown",
			wantErr:  true,
			errMsg:   "unknown cloud provider",
		},
		{
			name:     "empty provider",
			provider: "",
			wantErr:  true,
			errMsg:   "unknown cloud provider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prov, err := New(tt.provider, false, "test-project")
			if tt.wantErr {
				if err == nil {
					t.Errorf("New(%q) expected error, got nil", tt.provider)
					return
				}
				if tt.errMsg != "" && !containsString(err.Error(), tt.errMsg) {
					t.Errorf("New(%q) error = %q, want error containing %q", tt.provider, err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("New(%q) unexpected error: %v", tt.provider, err)
					return
				}
				if prov == nil {
					t.Errorf("New(%q) returned nil provisioner", tt.provider)
				}
			}
		})
	}
}

func TestVMConfig(t *testing.T) {
	config := VMConfig{
		Region:      "us-central1",
		MachineType: "e2-medium",
		UseSpot:     true,
		DiskSizeGB:  50,
		Session: SessionConfig{
			ID:            "test-session",
			Repository:    "github.com/org/repo",
			Tasks:         []string{"1", "2", "3"},
			Agent:         "claude-code",
			MaxIterations: 30,
			MaxDuration:   "2h",
			Prompt:        "Test prompt",
			GitHub: GitHubConfig{
				AppID:            123456,
				InstallationID:   789012,
				PrivateKeySecret: "projects/test/secrets/key",
			},
		},
		ControllerImage: "ghcr.io/test/controller:latest",
	}

	// Test that all fields are accessible
	if config.Region != "us-central1" {
		t.Errorf("Region = %q, want %q", config.Region, "us-central1")
	}
	if config.Session.ID != "test-session" {
		t.Errorf("Session.ID = %q, want %q", config.Session.ID, "test-session")
	}
	if len(config.Session.Tasks) != 3 {
		t.Errorf("len(Session.Tasks) = %d, want 3", len(config.Session.Tasks))
	}
	if config.Session.GitHub.AppID != 123456 {
		t.Errorf("Session.GitHub.AppID = %d, want 123456", config.Session.GitHub.AppID)
	}
}

func TestSessionStatus(t *testing.T) {
	now := time.Now()
	status := SessionStatus{
		SessionID:        "test-session",
		State:            "running",
		InstanceID:       "instance-123",
		PublicIP:         "35.192.0.1",
		Zone:             "us-central1-a",
		StartTime:        now.Add(-1 * time.Hour),
		EndTime:          time.Time{},
		CurrentIteration: 5,
		MaxIterations:    30,
		CompletedTasks:   []string{"1", "2"},
		PendingTasks:     []string{"3"},
		LastError:        "",
	}

	if status.SessionID != "test-session" {
		t.Errorf("SessionID = %q, want %q", status.SessionID, "test-session")
	}
	if status.State != "running" {
		t.Errorf("State = %q, want %q", status.State, "running")
	}
	if !status.EndTime.IsZero() {
		t.Error("EndTime should be zero for running session")
	}
	if len(status.CompletedTasks) != 2 {
		t.Errorf("len(CompletedTasks) = %d, want 2", len(status.CompletedTasks))
	}
}

func TestLogsOptions(t *testing.T) {
	since := time.Now().Add(-1 * time.Hour)
	opts := LogsOptions{
		Follow: true,
		Tail:   100,
		Since:  since,
	}

	if !opts.Follow {
		t.Error("Follow should be true")
	}
	if opts.Tail != 100 {
		t.Errorf("Tail = %d, want 100", opts.Tail)
	}
	if opts.Since != since {
		t.Errorf("Since = %v, want %v", opts.Since, since)
	}
}

func TestLogEntry(t *testing.T) {
	ts := time.Now()
	entry := LogEntry{
		Timestamp: ts,
		Message:   "Test log message",
		Level:     "INFO",
		Source:    "controller",
	}

	if entry.Timestamp != ts {
		t.Errorf("Timestamp = %v, want %v", entry.Timestamp, ts)
	}
	if entry.Message != "Test log message" {
		t.Errorf("Message = %q, want %q", entry.Message, "Test log message")
	}
	if entry.Level != "INFO" {
		t.Errorf("Level = %q, want %q", entry.Level, "INFO")
	}
	if entry.Source != "controller" {
		t.Errorf("Source = %q, want %q", entry.Source, "controller")
	}
}

func TestProvisionResult(t *testing.T) {
	result := ProvisionResult{
		InstanceID: "instance-abc123",
		PublicIP:   "35.192.0.1",
		Zone:       "us-central1-a",
		SessionID:  "agentium-abc123",
	}

	if result.InstanceID != "instance-abc123" {
		t.Errorf("InstanceID = %q, want %q", result.InstanceID, "instance-abc123")
	}
	if result.PublicIP != "35.192.0.1" {
		t.Errorf("PublicIP = %q, want %q", result.PublicIP, "35.192.0.1")
	}
	if result.Zone != "us-central1-a" {
		t.Errorf("Zone = %q, want %q", result.Zone, "us-central1-a")
	}
	if result.SessionID != "agentium-abc123" {
		t.Errorf("SessionID = %q, want %q", result.SessionID, "agentium-abc123")
	}
}

func TestBuildListArgs(t *testing.T) {
	tests := []struct {
		name     string
		project  string
		wantArgs []string
	}{
		{
			name:    "includes project flag when set",
			project: "my-gcp-project",
			wantArgs: []string{
				"compute", "instances", "list",
				"--filter=labels.agentium=true",
				"--format=json",
				"--project=my-gcp-project",
			},
		},
		{
			name:    "no project flag when empty",
			project: "",
			wantArgs: []string{
				"compute", "instances", "list",
				"--filter=labels.agentium=true",
				"--format=json",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &GCPProvisioner{project: tt.project}
			got := p.buildListArgs()
			if len(got) != len(tt.wantArgs) {
				t.Fatalf("buildListArgs() returned %d args, want %d\ngot:  %v\nwant: %v", len(got), len(tt.wantArgs), got, tt.wantArgs)
			}
			for i := range got {
				if got[i] != tt.wantArgs[i] {
					t.Errorf("arg[%d] = %q, want %q", i, got[i], tt.wantArgs[i])
				}
			}
		})
	}
}

func TestBuildStatusArgs(t *testing.T) {
	tests := []struct {
		name      string
		project   string
		sessionID string
		wantArgs  []string
	}{
		{
			name:      "includes project flag when set",
			project:   "my-gcp-project",
			sessionID: "agentium-session-123",
			wantArgs: []string{
				"compute", "instances", "describe",
				"agentium-session-123",
				"--format=json",
				"--project=my-gcp-project",
			},
		},
		{
			name:      "no project flag when empty",
			project:   "",
			sessionID: "agentium-session-123",
			wantArgs: []string{
				"compute", "instances", "describe",
				"agentium-session-123",
				"--format=json",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &GCPProvisioner{project: tt.project}
			got := p.buildStatusArgs(tt.sessionID)
			if len(got) != len(tt.wantArgs) {
				t.Fatalf("buildStatusArgs() returned %d args, want %d\ngot:  %v\nwant: %v", len(got), len(tt.wantArgs), got, tt.wantArgs)
			}
			for i := range got {
				if got[i] != tt.wantArgs[i] {
					t.Errorf("arg[%d] = %q, want %q", i, got[i], tt.wantArgs[i])
				}
			}
		})
	}
}

func TestBuildDestroyArgs(t *testing.T) {
	tests := []struct {
		name      string
		project   string
		sessionID string
		wantArgs  []string
	}{
		{
			name:      "includes project flag when set",
			project:   "my-gcp-project",
			sessionID: "agentium-session-456",
			wantArgs: []string{
				"compute", "instances", "delete",
				"agentium-session-456",
				"--quiet",
				"--project=my-gcp-project",
			},
		},
		{
			name:      "no project flag when empty",
			project:   "",
			sessionID: "agentium-session-456",
			wantArgs: []string{
				"compute", "instances", "delete",
				"agentium-session-456",
				"--quiet",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &GCPProvisioner{project: tt.project}
			got := p.buildDestroyArgs(tt.sessionID)
			if len(got) != len(tt.wantArgs) {
				t.Fatalf("buildDestroyArgs() returned %d args, want %d\ngot:  %v\nwant: %v", len(got), len(tt.wantArgs), got, tt.wantArgs)
			}
			for i := range got {
				if got[i] != tt.wantArgs[i] {
					t.Errorf("arg[%d] = %q, want %q", i, got[i], tt.wantArgs[i])
				}
			}
		})
	}
}

func TestBuildLogsArgs(t *testing.T) {
	tests := []struct {
		name      string
		project   string
		sessionID string
		opts      LogsOptions
		wantArgs  []string
	}{
		{
			name:      "basic args with project",
			project:   "my-project",
			sessionID: "agentium-abc123",
			opts:      LogsOptions{},
			wantArgs: []string{
				"logging", "read",
				`logName=~"agentium-session" AND jsonPayload.session_id="agentium-abc123" AND severity >= "INFO"`,
				"--format=json",
				"--project=my-project",
			},
		},
		{
			name:      "no project flag when empty",
			project:   "",
			sessionID: "agentium-abc123",
			opts:      LogsOptions{},
			wantArgs: []string{
				"logging", "read",
				`logName=~"agentium-session" AND jsonPayload.session_id="agentium-abc123" AND severity >= "INFO"`,
				"--format=json",
			},
		},
		{
			name:      "with tail limit",
			project:   "my-project",
			sessionID: "agentium-abc123",
			opts:      LogsOptions{Tail: 50},
			wantArgs: []string{
				"logging", "read",
				`logName=~"agentium-session" AND jsonPayload.session_id="agentium-abc123" AND severity >= "INFO"`,
				"--format=json",
				"--project=my-project",
				"--limit=50",
			},
		},
		{
			name:      "with show events (no severity filter)",
			project:   "my-project",
			sessionID: "agentium-abc123",
			opts:      LogsOptions{ShowEvents: true},
			wantArgs: []string{
				"logging", "read",
				`logName=~"agentium-session" AND jsonPayload.session_id="agentium-abc123"`,
				"--format=json",
				"--project=my-project",
			},
		},
		{
			name:      "with debug level (no severity filter)",
			project:   "my-project",
			sessionID: "agentium-abc123",
			opts:      LogsOptions{MinLevel: "debug"},
			wantArgs: []string{
				"logging", "read",
				`logName=~"agentium-session" AND jsonPayload.session_id="agentium-abc123"`,
				"--format=json",
				"--project=my-project",
			},
		},
		{
			name:      "with warning level",
			project:   "my-project",
			sessionID: "agentium-abc123",
			opts:      LogsOptions{MinLevel: "warning"},
			wantArgs: []string{
				"logging", "read",
				`logName=~"agentium-session" AND jsonPayload.session_id="agentium-abc123" AND severity >= "WARNING"`,
				"--format=json",
				"--project=my-project",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &GCPProvisioner{project: tt.project}
			got := p.buildLogsArgs(tt.sessionID, tt.opts)
			if len(got) != len(tt.wantArgs) {
				t.Fatalf("buildLogsArgs() returned %d args, want %d\ngot:  %v\nwant: %v", len(got), len(tt.wantArgs), got, tt.wantArgs)
			}
			for i := range got {
				if got[i] != tt.wantArgs[i] {
					t.Errorf("arg[%d] = %q, want %q", i, got[i], tt.wantArgs[i])
				}
			}
		})
	}
}

func TestParseLogEntries(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []LogEntry
		wantErr bool
	}{
		{
			name:  "textPayload entries",
			input: `[{"timestamp":"2024-01-15T10:00:01.000Z","textPayload":"hello world","severity":"INFO"},{"timestamp":"2024-01-15T10:00:00.000Z","textPayload":"starting up","severity":"DEBUG"}]`,
			want: []LogEntry{
				{Message: "starting up", Level: "DEBUG"},
				{Message: "hello world", Level: "INFO"},
			},
		},
		{
			name:  "jsonPayload entries",
			input: `[{"timestamp":"2024-01-15T10:00:00.000Z","severity":"DEFAULT","jsonPayload":{"message":"controller ready","severity":"INFO"}}]`,
			want: []LogEntry{
				{Message: "controller ready", Level: "INFO"},
			},
		},
		{
			name:  "jsonPayload overrides textPayload",
			input: `[{"timestamp":"2024-01-15T10:00:00.000Z","textPayload":"raw text","severity":"WARNING","jsonPayload":{"message":"structured msg","severity":"ERROR"}}]`,
			want: []LogEntry{
				{Message: "structured msg", Level: "ERROR"},
			},
		},
		{
			name:  "jsonPayload without severity uses top-level severity",
			input: `[{"timestamp":"2024-01-15T10:00:00.000Z","severity":"WARNING","jsonPayload":{"message":"no level"}}]`,
			want: []LogEntry{
				{Message: "no level", Level: "WARNING"},
			},
		},
		{
			name:  "empty message entries are skipped",
			input: `[{"timestamp":"2024-01-15T10:00:00.000Z","severity":"INFO"},{"timestamp":"2024-01-15T09:59:00.000Z","textPayload":"visible","severity":"INFO"}]`,
			want: []LogEntry{
				{Message: "visible", Level: "INFO"},
			},
		},
		{
			name:  "entries returned in chronological order",
			input: `[{"timestamp":"2024-01-15T10:00:02.000Z","textPayload":"third","severity":"INFO"},{"timestamp":"2024-01-15T10:00:01.000Z","textPayload":"second","severity":"INFO"},{"timestamp":"2024-01-15T10:00:00.000Z","textPayload":"first","severity":"INFO"}]`,
			want: []LogEntry{
				{Message: "first", Level: "INFO"},
				{Message: "second", Level: "INFO"},
				{Message: "third", Level: "INFO"},
			},
		},
		{
			name:    "invalid JSON returns error",
			input:   `not json`,
			wantErr: true,
		},
		{
			name:  "empty array",
			input: `[]`,
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseLogEntries([]byte(tt.input))
			if tt.wantErr {
				if err == nil {
					t.Fatal("parseLogEntries() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseLogEntries() unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("parseLogEntries() returned %d entries, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i].Message != tt.want[i].Message {
					t.Errorf("entry[%d].Message = %q, want %q", i, got[i].Message, tt.want[i].Message)
				}
				if got[i].Level != tt.want[i].Level {
					t.Errorf("entry[%d].Level = %q, want %q", i, got[i].Level, tt.want[i].Level)
				}
			}
		})
	}
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
