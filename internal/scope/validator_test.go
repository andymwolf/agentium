package scope

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestScopeValidator_isInScope(t *testing.T) {
	v := &ScopeValidator{
		PackagePath: "packages/core",
		WorkDir:     "/workspace",
	}

	tests := []struct {
		name        string
		filePath    string
		packagePath string
		want        bool
	}{
		{
			name:        "file in package",
			filePath:    "packages/core/src/index.ts",
			packagePath: "packages/core",
			want:        true,
		},
		{
			name:        "file in package subdirectory",
			filePath:    "packages/core/src/lib/utils.ts",
			packagePath: "packages/core",
			want:        true,
		},
		{
			name:        "file outside package",
			filePath:    "packages/shared/src/index.ts",
			packagePath: "packages/core",
			want:        false,
		},
		{
			name:        "file at root",
			filePath:    "package.json",
			packagePath: "packages/core",
			want:        false,
		},
		{
			name:        "similar prefix but different package",
			filePath:    "packages/core-utils/index.ts",
			packagePath: "packages/core",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := v.isInScope(tt.filePath, filepath.Clean(tt.packagePath))
			if got != tt.want {
				t.Errorf("isInScope(%q, %q) = %v, want %v", tt.filePath, tt.packagePath, got, tt.want)
			}
		})
	}
}

func TestScopeValidator_isExempt(t *testing.T) {
	v := &ScopeValidator{
		PackagePath: "packages/core",
		WorkDir:     "/workspace",
	}

	tests := []struct {
		name     string
		filePath string
		want     bool
	}{
		{
			name:     "root package.json",
			filePath: "package.json",
			want:     true,
		},
		{
			name:     "pnpm-lock.yaml",
			filePath: "pnpm-lock.yaml",
			want:     true,
		},
		{
			name:     "workflow file",
			filePath: ".github/workflows/ci.yml",
			want:     true,
		},
		{
			name:     "random file at root",
			filePath: "README.md",
			want:     false,
		},
		{
			name:     "package inside other package",
			filePath: "packages/shared/package.json",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := v.isExempt(tt.filePath)
			if got != tt.want {
				t.Errorf("isExempt(%q) = %v, want %v", tt.filePath, got, tt.want)
			}
		})
	}
}

func TestScopeValidator_validateFiles(t *testing.T) {
	v := &ScopeValidator{
		PackagePath: "packages/core",
		WorkDir:     "/workspace",
	}

	tests := []struct {
		name              string
		files             []string
		wantValid         bool
		wantOutOfScope    int
		wantAllowedExempt int
	}{
		{
			name: "all in scope",
			files: []string{
				"packages/core/src/index.ts",
				"packages/core/package.json",
			},
			wantValid:         true,
			wantOutOfScope:    0,
			wantAllowedExempt: 0,
		},
		{
			name: "with exemptions",
			files: []string{
				"packages/core/src/index.ts",
				"package.json",
				"pnpm-lock.yaml",
			},
			wantValid:         true,
			wantOutOfScope:    0,
			wantAllowedExempt: 2,
		},
		{
			name: "out of scope files",
			files: []string{
				"packages/core/src/index.ts",
				"packages/shared/src/index.ts",
			},
			wantValid:      false,
			wantOutOfScope: 1,
		},
		{
			name: "mixed",
			files: []string{
				"packages/core/src/index.ts",
				"packages/shared/src/index.ts",
				"package.json",
			},
			wantValid:         false,
			wantOutOfScope:    1,
			wantAllowedExempt: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := v.validateFiles(tt.files)
			if err != nil {
				t.Fatalf("validateFiles() error = %v", err)
			}

			if result.Valid != tt.wantValid {
				t.Errorf("validateFiles() Valid = %v, want %v", result.Valid, tt.wantValid)
			}
			if len(result.OutOfScopeFiles) != tt.wantOutOfScope {
				t.Errorf("validateFiles() OutOfScopeFiles count = %d, want %d", len(result.OutOfScopeFiles), tt.wantOutOfScope)
			}
			if len(result.AllowedExempt) != tt.wantAllowedExempt {
				t.Errorf("validateFiles() AllowedExempt count = %d, want %d", len(result.AllowedExempt), tt.wantAllowedExempt)
			}
		})
	}
}

func TestScopeValidator_NoPackageScope(t *testing.T) {
	v := &ScopeValidator{
		PackagePath: "", // Empty = no scope restriction
		WorkDir:     "/workspace",
	}

	result, err := v.ValidateChanges()
	if err != nil {
		t.Fatalf("ValidateChanges() error = %v", err)
	}

	if !result.Valid {
		t.Error("ValidateChanges() with no package scope should always be valid")
	}
}

func TestScopeValidator_FormatViolationError(t *testing.T) {
	v := &ScopeValidator{
		PackagePath: "packages/core",
		WorkDir:     "/workspace",
	}

	result := &ValidationResult{
		Valid:           false,
		OutOfScopeFiles: []string{"packages/shared/index.ts", "src/main.ts"},
	}

	msg := v.FormatViolationError(result)

	// Verify the message contains key information
	if msg == "" {
		t.Error("FormatViolationError() returned empty string for violation")
	}

	expectedContains := []string{
		"SCOPE VIOLATION",
		"2 file(s)",
		"packages/core",
		"packages/shared/index.ts",
		"src/main.ts",
	}

	for _, expected := range expectedContains {
		if !strings.Contains(msg, expected) {
			t.Errorf("FormatViolationError() missing expected content: %q", expected)
		}
	}
}

func TestScopeValidator_FormatViolationError_Valid(t *testing.T) {
	v := &ScopeValidator{
		PackagePath: "packages/core",
		WorkDir:     "/workspace",
	}

	result := &ValidationResult{Valid: true}
	msg := v.FormatViolationError(result)

	if msg != "" {
		t.Errorf("FormatViolationError() for valid result should be empty, got %q", msg)
	}
}

func TestNewValidator(t *testing.T) {
	v := NewValidator("/workspace", "packages/core")

	if v.WorkDir != "/workspace" {
		t.Errorf("NewValidator() WorkDir = %q, want %q", v.WorkDir, "/workspace")
	}
	if v.PackagePath != "packages/core" {
		t.Errorf("NewValidator() PackagePath = %q, want %q", v.PackagePath, "packages/core")
	}
}

func TestScopeValidator_ResetChanges(t *testing.T) {
	// Create a temp git repo
	tmpDir := t.TempDir()

	// Initialize git repo
	if err := runGitCmd(tmpDir, "init"); err != nil {
		t.Skipf("Git not available: %v", err)
	}

	// Configure git (required for commit)
	if err := runGitCmd(tmpDir, "config", "user.email", "test@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := runGitCmd(tmpDir, "config", "user.name", "Test User"); err != nil {
		t.Fatal(err)
	}

	// Create initial file and commit
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("original content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := runGitCmd(tmpDir, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := runGitCmd(tmpDir, "commit", "-m", "Initial commit"); err != nil {
		t.Fatal(err)
	}

	// Modify the file
	if err := os.WriteFile(testFile, []byte("modified content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Verify file is modified
	content, _ := os.ReadFile(testFile)
	if string(content) != "modified content" {
		t.Fatal("File should be modified before reset")
	}

	// Reset changes
	v := NewValidator(tmpDir, "packages/core")
	if err := v.ResetChanges(); err != nil {
		t.Fatalf("ResetChanges() error = %v", err)
	}

	// Verify file is reset
	content, _ = os.ReadFile(testFile)
	if string(content) != "original content" {
		t.Errorf("ResetChanges() did not restore file, got %q", string(content))
	}
}

// runGitCmd is a helper to execute git commands in tests
func runGitCmd(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd.Run()
}
