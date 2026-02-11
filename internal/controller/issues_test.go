package controller

import (
	"testing"
)

func TestBranchPrefixForLabels(t *testing.T) {
	tests := []struct {
		name   string
		labels []issueLabel
		want   string
	}{
		{
			name:   "no labels - default to feature",
			labels: nil,
			want:   "feature",
		},
		{
			name:   "empty labels - default to feature",
			labels: []issueLabel{},
			want:   "feature",
		},
		{
			name:   "bug label",
			labels: []issueLabel{{Name: "bug"}},
			want:   "bug",
		},
		{
			name:   "enhancement label",
			labels: []issueLabel{{Name: "enhancement"}},
			want:   "enhancement",
		},
		{
			name:   "multiple labels - use first",
			labels: []issueLabel{{Name: "bug"}, {Name: "urgent"}},
			want:   "bug",
		},
		{
			name:   "label with space - sanitized",
			labels: []issueLabel{{Name: "good first issue"}},
			want:   "good-first-issue",
		},
		{
			name:   "uppercase label - lowercased",
			labels: []issueLabel{{Name: "Feature"}},
			want:   "feature",
		},
		{
			name:   "mixed case with space",
			labels: []issueLabel{{Name: "Help Wanted"}},
			want:   "help-wanted",
		},
		{
			name:   "label with colon - sanitized",
			labels: []issueLabel{{Name: "type: bug"}},
			want:   "type-bug",
		},
		{
			name:   "label with question mark - sanitized",
			labels: []issueLabel{{Name: "priority?high"}},
			want:   "priority-high",
		},
		{
			name:   "label with slash - sanitized",
			labels: []issueLabel{{Name: "ui/ux"}},
			want:   "ui-ux",
		},
		{
			name:   "label with multiple special chars - sanitized",
			labels: []issueLabel{{Name: "type: bug [critical]"}},
			want:   "type-bug-critical",
		},
		{
			name:   "label with consecutive special chars - collapsed",
			labels: []issueLabel{{Name: "type::bug"}},
			want:   "type-bug",
		},
		{
			name:   "label starting with special char - trimmed",
			labels: []issueLabel{{Name: ":bug"}},
			want:   "bug",
		},
		{
			name:   "label ending with special char - trimmed",
			labels: []issueLabel{{Name: "bug:"}},
			want:   "bug",
		},
		{
			name:   "label that becomes empty after sanitization - default to feature",
			labels: []issueLabel{{Name: ":::"}},
			want:   "feature",
		},
		{
			name:   "label with numbers",
			labels: []issueLabel{{Name: "priority-1"}},
			want:   "priority-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := branchPrefixForLabels(tt.labels)
			if got != tt.want {
				t.Errorf("branchPrefixForLabels() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSanitizeBranchPrefix(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"bug", "bug"},
		{"Bug", "bug"},
		{"type: bug", "type-bug"},
		{"priority?high", "priority-high"},
		{"ui/ux", "ui-ux"},
		{"good first issue", "good-first-issue"},
		{"type::bug", "type-bug"},
		{":bug", "bug"},
		{"bug:", "bug"},
		{":::", ""},
		{"a~b^c", "a-b-c"},
		{"test*case", "test-case"},
		{"feature[1]", "feature-1"},
		{"path\\name", "path-name"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeBranchPrefix(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeBranchPrefix(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatExternalComments(t *testing.T) {
	tests := []struct {
		name     string
		comments []issueComment
		want     string
	}{
		{
			name:     "no comments",
			comments: nil,
			want:     "",
		},
		{
			name: "single external comment",
			comments: []issueComment{
				{
					Author:    issueCommentAuthor{Login: "alice"},
					Body:      "This approach looks wrong.",
					CreatedAt: "2025-01-15T10:30:00Z",
				},
			},
			want: "**@alice** (2025-01-15):\n> This approach looks wrong.\n\n",
		},
		{
			name: "filters agentium comments",
			comments: []issueComment{
				{
					Author:    issueCommentAuthor{Login: "alice"},
					Body:      "Please fix the tests.",
					CreatedAt: "2025-01-15T10:30:00Z",
				},
				{
					Author:    issueCommentAuthor{Login: "agentium-bot"},
					Body:      "Phase complete.\n\n<!-- agentium:gcp:agentium-abc123 -->",
					CreatedAt: "2025-01-15T11:00:00Z",
				},
			},
			want: "**@alice** (2025-01-15):\n> Please fix the tests.\n\n",
		},
		{
			name: "all agentium comments returns empty",
			comments: []issueComment{
				{
					Author:    issueCommentAuthor{Login: "agentium-bot"},
					Body:      "Status update.\n\n<!-- agentium:gcp:agentium-abc123 -->",
					CreatedAt: "2025-01-15T11:00:00Z",
				},
			},
			want: "",
		},
		{
			name: "multiline comment body",
			comments: []issueComment{
				{
					Author:    issueCommentAuthor{Login: "bob"},
					Body:      "Line one.\nLine two.\nLine three.",
					CreatedAt: "2025-02-01T08:00:00Z",
				},
			},
			want: "**@bob** (2025-02-01):\n> Line one.\n> Line two.\n> Line three.\n\n",
		},
		{
			name: "short createdAt preserved as-is",
			comments: []issueComment{
				{
					Author:    issueCommentAuthor{Login: "carol"},
					Body:      "Short date.",
					CreatedAt: "2025-03",
				},
			},
			want: "**@carol** (2025-03):\n> Short date.\n\n",
		},
		{
			name: "multiple external comments in order",
			comments: []issueComment{
				{
					Author:    issueCommentAuthor{Login: "alice"},
					Body:      "First comment.",
					CreatedAt: "2025-01-10T09:00:00Z",
				},
				{
					Author:    issueCommentAuthor{Login: "bob"},
					Body:      "Second comment.",
					CreatedAt: "2025-01-11T10:00:00Z",
				},
			},
			want: "**@alice** (2025-01-10):\n> First comment.\n\n**@bob** (2025-01-11):\n> Second comment.\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatExternalComments(tt.comments)
			if got != tt.want {
				t.Errorf("formatExternalComments() =\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}
