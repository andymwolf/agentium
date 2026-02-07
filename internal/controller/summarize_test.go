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

func TestStripAgentiumSignals(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string
		excludes string
		want     string // exact match (empty = skip exact check)
	}{
		{
			name:  "empty input",
			input: "",
			want:  "",
		},
		{
			name:     "removes STATUS signal",
			input:    "Some text\nAGENTIUM_STATUS: COMPLETE All done\nMore text",
			contains: "More text",
			excludes: "AGENTIUM_STATUS",
		},
		{
			name:     "removes MEMORY signal",
			input:    "Some text\nAGENTIUM_MEMORY: FEEDBACK_RESPONSE ADDRESSED fixed the bug\nMore text",
			contains: "More text",
			excludes: "AGENTIUM_MEMORY",
		},
		{
			name: "removes multi-line HANDOFF block",
			input: `Plan summary here.

AGENTIUM_HANDOFF: {
  "summary": "Add feature X",
  "files_to_modify": ["a.go"]
}

Next section.`,
			contains: "Next section",
			excludes: "AGENTIUM_HANDOFF",
		},
		{
			name:     "preserves non-signal content",
			input:    "This is a normal plan.\nWith multiple lines.\nNo signals here.",
			contains: "With multiple lines",
		},
		{
			name: "removes multiple signal types",
			input: `Start
AGENTIUM_STATUS: WORKING on plan
Middle
AGENTIUM_MEMORY: FEEDBACK_RESPONSE DECLINED not needed
End`,
			contains: "End",
			excludes: "AGENTIUM_STATUS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripAgentiumSignals(tt.input)
			if tt.want != "" && got != tt.want {
				t.Errorf("StripAgentiumSignals() = %q, want %q", got, tt.want)
			}
			if tt.contains != "" && !strings.Contains(got, tt.contains) {
				t.Errorf("StripAgentiumSignals() should contain %q, got:\n%s", tt.contains, got)
			}
			if tt.excludes != "" && strings.Contains(got, tt.excludes) {
				t.Errorf("StripAgentiumSignals() should NOT contain %q, got:\n%s", tt.excludes, got)
			}
		})
	}
}

func TestStripPreamble(t *testing.T) {
	tests := []struct {
		name string
		input string
		want string
	}{
		{
			name:  "empty input",
			input: "",
			want:  "",
		},
		{
			name:  "no preamble",
			input: "## Plan Summary\n\nHere is the plan.",
			want:  "## Plan Summary\n\nHere is the plan.",
		},
		{
			name:  "strips Let me preamble",
			input: "Let me analyze the codebase.\n\n## Plan Summary\n\nHere is the plan.",
			want:  "## Plan Summary\n\nHere is the plan.",
		},
		{
			name:  "strips multiple preamble lines",
			input: "Excellent - I now have a comprehensive understanding.\nLet me create the plan.\n\n## Plan Summary\n\nHere is the plan.",
			want:  "## Plan Summary\n\nHere is the plan.",
		},
		{
			name:  "strips preamble with blank lines between",
			input: "I'll review the changes.\n\nLooking at the code...\n\n## Feedback\n\nThe code looks good.",
			want:  "## Feedback\n\nThe code looks good.",
		},
		{
			name:  "all preamble returns empty",
			input: "Let me think about this.\nI need to analyze more.\nExcellent progress.",
			want:  "",
		},
		{
			name:  "preserves content starting with markdown header",
			input: "## Review Feedback\n\n- Issue 1\n- Issue 2",
			want:  "## Review Feedback\n\n- Issue 1\n- Issue 2",
		},
		{
			name:  "strips file operation lines",
			input: "File created successfully.\nFile updated at path/to/file.\n\n## Summary\n\nDone.",
			want:  "## Summary\n\nDone.",
		},
		{
			name:  "case insensitive matching",
			input: "LOOKING AT the implementation...\n\n## Plan\n\nStep 1.",
			want:  "## Plan\n\nStep 1.",
		},
		{
			name:  "I now have preamble",
			input: "I now have a comprehensive understanding of the codebase.\n\n## Plan\n\nDo things.",
			want:  "## Plan\n\nDo things.",
		},
		{
			name:  "based on preamble",
			input: "Based on my analysis of the issue.\n\n## Plan\n\nDo things.",
			want:  "## Plan\n\nDo things.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripPreamble(tt.input)
			if got != tt.want {
				t.Errorf("StripPreamble() = %q, want %q", got, tt.want)
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
