package security

import (
	"testing"
)

func TestCommandValidator_ValidateCommand(t *testing.T) {
	validator := NewCommandValidator()

	tests := []struct {
		name    string
		cmd     string
		args    []string
		wantErr bool
	}{
		{
			name:    "allowed command with safe args",
			cmd:     "git",
			args:    []string{"status", "--porcelain"},
			wantErr: false,
		},
		{
			name:    "disallowed command",
			cmd:     "rm",
			args:    []string{"-rf", "/"},
			wantErr: true,
		},
		{
			name:    "command substitution attempt",
			cmd:     "git",
			args:    []string{"commit", "-m", "$(cat /etc/passwd)"},
			wantErr: true,
		},
		{
			name:    "command chaining attempt",
			cmd:     "git",
			args:    []string{"status", "&&", "rm", "-rf", "/"},
			wantErr: true,
		},
		{
			name:    "pipe attempt",
			cmd:     "git",
			args:    []string{"log", "|", "grep", "secret"},
			wantErr: true,
		},
		{
			name:    "variable expansion attempt",
			cmd:     "git",
			args:    []string{"commit", "-m", "${USER}"},
			wantErr: true,
		},
		{
			name:    "backtick command substitution",
			cmd:     "git",
			args:    []string{"commit", "-m", "`whoami`"},
			wantErr: true,
		},
		{
			name:    "newline injection",
			cmd:     "git",
			args:    []string{"commit", "-m", "test\nrm -rf /"},
			wantErr: true,
		},
		{
			name:    "redirect attempt",
			cmd:     "git",
			args:    []string{"log", ">", "/etc/passwd"},
			wantErr: true,
		},
		{
			name:    "background execution",
			cmd:     "git",
			args:    []string{"clone", "repo", "&"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateCommand(tt.cmd, tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCommand() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCommandValidator_ValidateGitRef(t *testing.T) {
	validator := NewCommandValidator()

	tests := []struct {
		name    string
		ref     string
		wantErr bool
	}{
		{"valid branch", "main", false},
		{"valid feature branch", "feature/add-login", false},
		{"valid tag", "v1.0.0", false},
		{"valid commit", "abc123def456", false},
		{"command injection", "main;rm -rf /", true},
		{"space injection", "main test", true},
		{"newline injection", "main\nrm -rf /", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateGitRef(tt.ref)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateGitRef() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCommandValidator_ValidatePath(t *testing.T) {
	validator := NewCommandValidator()

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"relative path", "src/main.go", false},
		{"workspace absolute", "/workspace/src/main.go", false},
		{"path traversal", "../../../etc/passwd", true},
		{"sneaky traversal", "src/../../etc/passwd", true},
		{"outside workspace", "/etc/passwd", true},
		{"hidden traversal", "src/../../../etc/passwd", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidatePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePath() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCommandValidator_ValidateSessionID(t *testing.T) {
	validator := NewCommandValidator()

	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"valid UUID", "123e4567-e89b-12d3-a456-426614174000", false},
		{"invalid format", "not-a-uuid", true},
		{"command injection", "123e4567-e89b-12d3-a456-426614174000;rm -rf /", true},
		{"too short", "123e4567", true},
		{"uppercase", "123E4567-E89B-12D3-A456-426614174000", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateSessionID(tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSessionID() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSanitizeForShell(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple string", "hello", "'hello'"},
		{"with spaces", "hello world", "'hello world'"},
		{"with single quote", "don't", "'don'\"'\"'t'"},
		{"complex injection", "'; rm -rf /; echo '", "''\"'\"'; rm -rf /; echo '\"'\"''"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeForShell(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeForShell() = %v, want %v", got, tt.want)
			}
		})
	}
}