package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParsePnpmWorkspace(t *testing.T) {
	// Create a temporary directory with a pnpm-workspace.yaml
	tmpDir := t.TempDir()

	// Create workspace structure
	workspaceContent := `packages:
  - 'packages/*'
  - 'apps/*'
`
	if err := os.WriteFile(filepath.Join(tmpDir, "pnpm-workspace.yaml"), []byte(workspaceContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create package directories
	for _, pkg := range []string{"packages/core", "packages/shared", "apps/web"} {
		if err := os.MkdirAll(filepath.Join(tmpDir, pkg), 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Test parsing
	packages, err := ParsePnpmWorkspace(tmpDir)
	if err != nil {
		t.Fatalf("ParsePnpmWorkspace() error = %v", err)
	}

	expected := []string{"apps/web", "packages/core", "packages/shared"}
	if len(packages) != len(expected) {
		t.Errorf("ParsePnpmWorkspace() got %d packages, want %d", len(packages), len(expected))
	}

	// Check that all expected packages are present
	pkgSet := make(map[string]bool)
	for _, p := range packages {
		pkgSet[p] = true
	}
	for _, exp := range expected {
		if !pkgSet[exp] {
			t.Errorf("ParsePnpmWorkspace() missing expected package %q", exp)
		}
	}
}

func TestParsePnpmWorkspace_NotFound(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := ParsePnpmWorkspace(tmpDir)
	if err == nil {
		t.Error("ParsePnpmWorkspace() expected error for missing file, got nil")
	}
}

func TestValidatePackage(t *testing.T) {
	tmpDir := t.TempDir()

	workspaceContent := `packages:
  - 'packages/*'
`
	if err := os.WriteFile(filepath.Join(tmpDir, "pnpm-workspace.yaml"), []byte(workspaceContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "packages/core"), 0755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		packagePath string
		wantErr     bool
	}{
		{
			name:        "valid package",
			packagePath: "packages/core",
			wantErr:     false,
		},
		{
			name:        "invalid package",
			packagePath: "packages/nonexistent",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePackage(tmpDir, tt.packagePath)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePackage() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNormalizePackagePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"./packages/core", "packages/core"},
		{"packages/core/", "packages/core"},
		{"./packages/core/", "packages/core"},
		{"packages/core", "packages/core"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizePackagePath(tt.input)
			if got != tt.expected {
				t.Errorf("NormalizePackagePath(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestHasPnpmWorkspace(t *testing.T) {
	tmpDir := t.TempDir()

	// Initially no workspace file
	if HasPnpmWorkspace(tmpDir) {
		t.Error("HasPnpmWorkspace() = true, want false (no file)")
	}

	// Create workspace file
	if err := os.WriteFile(filepath.Join(tmpDir, "pnpm-workspace.yaml"), []byte("packages: []"), 0644); err != nil {
		t.Fatal(err)
	}

	if !HasPnpmWorkspace(tmpDir) {
		t.Error("HasPnpmWorkspace() = false, want true (file exists)")
	}
}

func TestExtractPackageFromLabel(t *testing.T) {
	tests := []struct {
		label    string
		prefix   string
		wantPkg  string
		wantBool bool
	}{
		{"pkg:core", "pkg", "core", true},
		{"pkg:shared/utils", "pkg", "shared/utils", true},
		{"package:core", "pkg", "", false},
		{"bug", "pkg", "", false},
		{"scope:core", "scope", "core", true},
	}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			pkg, ok := ExtractPackageFromLabel(tt.label, tt.prefix)
			if ok != tt.wantBool {
				t.Errorf("ExtractPackageFromLabel(%q, %q) ok = %v, want %v", tt.label, tt.prefix, ok, tt.wantBool)
			}
			if pkg != tt.wantPkg {
				t.Errorf("ExtractPackageFromLabel(%q, %q) pkg = %q, want %q", tt.label, tt.prefix, pkg, tt.wantPkg)
			}
		})
	}
}

func TestFindPackageLabel(t *testing.T) {
	tests := []struct {
		name   string
		labels []string
		prefix string
		want   string
	}{
		{
			name:   "found first",
			labels: []string{"bug", "pkg:core", "priority:high"},
			prefix: "pkg",
			want:   "core",
		},
		{
			name:   "not found",
			labels: []string{"bug", "priority:high"},
			prefix: "pkg",
			want:   "",
		},
		{
			name:   "empty labels",
			labels: []string{},
			prefix: "pkg",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FindPackageLabel(tt.labels, tt.prefix)
			if got != tt.want {
				t.Errorf("FindPackageLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolvePackagePath(t *testing.T) {
	tmpDir := t.TempDir()

	workspaceContent := `packages:
  - 'packages/*'
  - 'apps/*'
`
	if err := os.WriteFile(filepath.Join(tmpDir, "pnpm-workspace.yaml"), []byte(workspaceContent), 0644); err != nil {
		t.Fatal(err)
	}
	for _, pkg := range []string{"packages/core", "packages/shared", "apps/web"} {
		if err := os.MkdirAll(filepath.Join(tmpDir, pkg), 0755); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name        string
		packageName string
		wantPath    string
		wantErr     bool
	}{
		{
			name:        "resolve by base name",
			packageName: "core",
			wantPath:    "packages/core",
			wantErr:     false,
		},
		{
			name:        "resolve by full path",
			packageName: "packages/shared",
			wantPath:    "packages/shared",
			wantErr:     false,
		},
		{
			name:        "resolve app",
			packageName: "web",
			wantPath:    "apps/web",
			wantErr:     false,
		},
		{
			name:        "not found",
			packageName: "nonexistent",
			wantPath:    "",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolvePackagePath(tmpDir, tt.packageName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ResolvePackagePath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.wantPath {
				t.Errorf("ResolvePackagePath() = %q, want %q", got, tt.wantPath)
			}
		})
	}
}
