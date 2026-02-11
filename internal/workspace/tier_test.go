package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

// setupTierWorkspace creates a temp workspace with packages for tier tests.
func setupTierWorkspace(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()

	workspaceContent := `packages:
  - 'packages/*'
  - 'apps/*'
`
	if err := os.WriteFile(filepath.Join(tmpDir, "pnpm-workspace.yaml"), []byte(workspaceContent), 0644); err != nil {
		t.Fatal(err)
	}
	for _, pkg := range []string{
		"packages/db",
		"packages/config",
		"packages/api",
		"apps/booking",
		"apps/admin",
	} {
		if err := os.MkdirAll(filepath.Join(tmpDir, pkg), 0755); err != nil {
			t.Fatal(err)
		}
	}
	return tmpDir
}

func TestClassifyPackageLabels(t *testing.T) {
	workDir := setupTierWorkspace(t)
	tiers := map[string][]string{
		"infra": {"packages/db", "packages/config"},
	}

	tests := []struct {
		name       string
		labels     []string
		wantCount  int
		wantErr    bool
		wantTier   string // Expected tier of first classification
		wantDomain bool   // Expected IsDomainApp of first classification
	}{
		{
			name:      "no pkg labels",
			labels:    []string{"bug", "priority:high"},
			wantCount: 0,
		},
		{
			name:       "single domain label",
			labels:     []string{"pkg:booking"},
			wantCount:  1,
			wantTier:   "",
			wantDomain: true,
		},
		{
			name:       "single tiered label",
			labels:     []string{"pkg:db"},
			wantCount:  1,
			wantTier:   "infra",
			wantDomain: false,
		},
		{
			name:       "mixed labels",
			labels:     []string{"bug", "pkg:booking", "pkg:db", "priority:high"},
			wantCount:  2,
			wantTier:   "",
			wantDomain: true, // First pkg label is booking (domain/app)
		},
		{
			name:       "full path label",
			labels:     []string{"pkg:packages/db"},
			wantCount:  1,
			wantTier:   "infra",
			wantDomain: false,
		},
		{
			name:       "short name label resolves to domain",
			labels:     []string{"pkg:api"},
			wantCount:  1,
			wantTier:   "",
			wantDomain: true,
		},
		{
			name:    "unknown package error",
			labels:  []string{"pkg:nonexistent"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ClassifyPackageLabels(tt.labels, "pkg", tiers, workDir)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ClassifyPackageLabels() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(got) != tt.wantCount {
				t.Fatalf("ClassifyPackageLabels() got %d classifications, want %d", len(got), tt.wantCount)
			}
			if tt.wantCount > 0 {
				if got[0].Tier != tt.wantTier {
					t.Errorf("first classification tier = %q, want %q", got[0].Tier, tt.wantTier)
				}
				if got[0].IsDomainApp != tt.wantDomain {
					t.Errorf("first classification IsDomainApp = %v, want %v", got[0].IsDomainApp, tt.wantDomain)
				}
			}
		})
	}
}

func TestValidatePackageLabels(t *testing.T) {
	tests := []struct {
		name            string
		classifications []PackageClassification
		wantPath        string
		wantErr         bool
	}{
		{
			name:            "zero classifications",
			classifications: nil,
			wantErr:         true,
		},
		{
			name: "single domain/app",
			classifications: []PackageClassification{
				{Label: "pkg:booking", PackagePath: "apps/booking", IsDomainApp: true},
			},
			wantPath: "apps/booking",
		},
		{
			name: "two domain/app rejected",
			classifications: []PackageClassification{
				{Label: "pkg:booking", PackagePath: "apps/booking", IsDomainApp: true},
				{Label: "pkg:admin", PackagePath: "apps/admin", IsDomainApp: true},
			},
			wantErr: true,
		},
		{
			name: "one domain plus one infra",
			classifications: []PackageClassification{
				{Label: "pkg:booking", PackagePath: "apps/booking", IsDomainApp: true},
				{Label: "pkg:db", PackagePath: "packages/db", Tier: "infra", IsDomainApp: false},
			},
			wantPath: "apps/booking",
		},
		{
			name: "all infra - alphabetical routing",
			classifications: []PackageClassification{
				{Label: "pkg:db", PackagePath: "packages/db", Tier: "infra", IsDomainApp: false},
				{Label: "pkg:config", PackagePath: "packages/config", Tier: "infra", IsDomainApp: false},
			},
			wantPath: "packages/config",
		},
		{
			name: "one domain plus multiple infra",
			classifications: []PackageClassification{
				{Label: "pkg:db", PackagePath: "packages/db", Tier: "infra", IsDomainApp: false},
				{Label: "pkg:booking", PackagePath: "apps/booking", IsDomainApp: true},
				{Label: "pkg:config", PackagePath: "packages/config", Tier: "infra", IsDomainApp: false},
			},
			wantPath: "apps/booking",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidatePackageLabels(tt.classifications)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidatePackageLabels() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.wantPath {
				t.Errorf("ValidatePackageLabels() = %q, want %q", got, tt.wantPath)
			}
		})
	}
}
