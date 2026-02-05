// Package version provides build-time version information for Agentium.
package version

import (
	"fmt"
	"runtime"
)

// Build-time variables set via ldflags.
// Example: go build -ldflags="-X github.com/andywolf/agentium/internal/version.Version=v1.0.0"
var (
	// Version is the semantic version (e.g., "v1.2.3"). Set via ldflags.
	Version = "dev"

	// Commit is the git commit SHA. Set via ldflags.
	Commit = "unknown"

	// BuildDate is the RFC3339 timestamp of the build. Set via ldflags.
	BuildDate = "unknown"
)

// Short returns the version string (e.g., "v1.2.3" or "dev").
func Short() string {
	return Version
}

// Info returns a single-line version string with commit and build info.
// Format: "agentium v1.2.3 (commit: abc1234, built: 2024-01-15T10:30:00Z, go: go1.25.x)"
func Info() string {
	commitShort := Commit
	if len(commitShort) > 7 {
		commitShort = commitShort[:7]
	}
	return fmt.Sprintf("agentium %s (commit: %s, built: %s, go: %s)",
		Version, commitShort, BuildDate, runtime.Version())
}

// Full returns a multi-line verbose version output.
func Full() string {
	return fmt.Sprintf(`agentium %s
  Commit:     %s
  Built:      %s
  Go version: %s
  OS/Arch:    %s/%s`,
		Version, Commit, BuildDate, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}
