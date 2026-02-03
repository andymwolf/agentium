package controller

import (
	"strings"
	"testing"
)

func TestSummarizeForComment(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		maxLines int
		wantLen  int    // expected line count (0 = don't check)
		contains string // substring that should be present
		excludes string // substring that should NOT be present
	}{
		{
			name:     "empty content",
			content:  "",
			maxLines: 100,
			wantLen:  0,
		},
		{
			name:     "plain text preserved",
			content:  "Line 1\nLine 2\nLine 3",
			maxLines: 100,
			wantLen:  3,
			contains: "Line 2",
		},
		{
			name: "diff block excluded",
			content: `Some explanation here.

diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -1,3 +1,4 @@
 package main
+import "fmt"
 func main() {}

More explanation after diff.`,
			maxLines: 100,
			contains: "Some explanation here",
			excludes: "diff --git",
		},
		{
			name: "hunk header excluded",
			content: `Summary of changes.
@@ -10,5 +10,6 @@
 unchanged
+added
-removed
End of review.`,
			maxLines: 100,
			contains: "Summary of changes",
			excludes: "@@ -10,5",
		},
		{
			name: "truncation with omission message",
			content: strings.Join(func() []string {
				lines := make([]string, 100)
				for i := range lines {
					lines[i] = "Line content"
				}
				return lines
			}(), "\n"),
			maxLines: 20,
			contains: "lines omitted",
		},
		{
			name:     "status signals preserved",
			content:  "Starting work...\nAGENTIUM_STATUS: COMPLETE All done\nFinished.",
			maxLines: 100,
			contains: "AGENTIUM_STATUS: COMPLETE",
		},
		{
			name: "multiple diff blocks excluded",
			content: `First explanation.

diff --git a/a.go b/a.go
+line

Second explanation.

diff --git b/b.go b/b.go
-removed

Third explanation.`,
			maxLines: 100,
			contains: "Third explanation",
			excludes: "+line",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SummarizeForComment(tt.content, tt.maxLines)

			if tt.wantLen > 0 {
				gotLines := len(strings.Split(got, "\n"))
				if gotLines != tt.wantLen {
					t.Errorf("SummarizeForComment() got %d lines, want %d", gotLines, tt.wantLen)
				}
			}

			if tt.contains != "" && !strings.Contains(got, tt.contains) {
				t.Errorf("SummarizeForComment() should contain %q, got:\n%s", tt.contains, got)
			}

			if tt.excludes != "" && strings.Contains(got, tt.excludes) {
				t.Errorf("SummarizeForComment() should NOT contain %q, got:\n%s", tt.excludes, got)
			}
		})
	}
}

func TestSummarizeForComment_MaxLinesZero(t *testing.T) {
	content := "Line 1\nLine 2\nLine 3\nLine 4\nLine 5"
	got := SummarizeForComment(content, 0)
	if got != content {
		t.Errorf("SummarizeForComment with maxLines=0 should not truncate, got:\n%s", got)
	}
}
