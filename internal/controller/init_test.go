package controller

import "testing"

func TestParseSecretName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{
			input: "projects/my-project/secrets/langfuse-public-key/versions/latest",
			want:  "langfuse-public-key",
		},
		{
			input: "projects/my-project/secrets/langfuse-public-key",
			want:  "langfuse-public-key",
		},
		{
			input: "langfuse-public-key",
			want:  "langfuse-public-key",
		},
		{
			input: "projects/123/secrets/my-secret/versions/3",
			want:  "my-secret",
		},
		{
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseSecretName(tt.input)
			if got != tt.want {
				t.Errorf("parseSecretName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
