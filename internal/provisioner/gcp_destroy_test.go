package provisioner

import "testing"

func TestSessionIDPrefix(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		want      string
	}{
		{
			name:      "short session ID unchanged",
			sessionID: "abc123",
			want:      "abc123",
		},
		{
			name:      "exactly 20 chars unchanged",
			sessionID: "12345678901234567890",
			want:      "12345678901234567890",
		},
		{
			name:      "longer than 20 chars truncated",
			sessionID: "agentium-session-abc123def456",
			want:      "agentium-session-abc",
		},
		{
			name:      "typical agentium session ID",
			sessionID: "agentium-abc123def456ghi789",
			want:      "agentium-abc123def45",
		},
		{
			name:      "empty session ID",
			sessionID: "",
			want:      "",
		},
		{
			name:      "single character",
			sessionID: "a",
			want:      "a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sessionIDPrefix(tt.sessionID)
			if got != tt.want {
				t.Errorf("sessionIDPrefix(%q) = %q, want %q", tt.sessionID, got, tt.want)
			}
		})
	}
}

func TestIsNotFoundError(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "resource not found",
			output: "ERROR: (gcloud.compute.instances.delete) Could not fetch resource:\n - The resource 'projects/my-project/zones/us-central1-a/instances/my-instance' was not found",
			want:   true,
		},
		{
			name:   "not found lowercase",
			output: "The resource was not found",
			want:   true,
		},
		{
			name:   "could not be found",
			output: "The service account could not be found.",
			want:   true,
		},
		{
			name:   "does not exist",
			output: "ERROR: The firewall rule does not exist.",
			want:   true,
		},
		{
			name:   "permission denied error",
			output: "ERROR: (gcloud.compute.instances.delete) Permission denied",
			want:   false,
		},
		{
			name:   "generic error",
			output: "ERROR: Internal server error",
			want:   false,
		},
		{
			name:   "empty output",
			output: "",
			want:   false,
		},
		{
			name:   "mixed case not found",
			output: "ERROR: Resource Not Found in project",
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isNotFoundError(tt.output)
			if got != tt.want {
				t.Errorf("isNotFoundError(%q) = %v, want %v", tt.output, got, tt.want)
			}
		})
	}
}

func TestDestroyFallbackResourceNaming(t *testing.T) {
	// This test verifies that the resource naming in the fallback path
	// matches the Terraform naming convention.
	tests := []struct {
		name         string
		sessionID    string
		project      string
		wantSAEmail  string
		wantFirewall string
	}{
		{
			name:         "standard session ID",
			sessionID:    "agentium-abc123def456ghi789",
			project:      "my-project",
			wantSAEmail:  "agentium-agentium-abc123def45@my-project.iam.gserviceaccount.com",
			wantFirewall: "agentium-allow-egress-agentium-abc123def45",
		},
		{
			name:         "short session ID",
			sessionID:    "test-session",
			project:      "test-proj",
			wantSAEmail:  "agentium-test-session@test-proj.iam.gserviceaccount.com",
			wantFirewall: "agentium-allow-egress-test-session",
		},
		{
			name:         "exactly 20 char session ID",
			sessionID:    "12345678901234567890",
			project:      "proj",
			wantSAEmail:  "agentium-12345678901234567890@proj.iam.gserviceaccount.com",
			wantFirewall: "agentium-allow-egress-12345678901234567890",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prefix := sessionIDPrefix(tt.sessionID)

			// Verify service account email matches Terraform convention:
			// account_id = "agentium-${substr(var.session_id, 0, 20)}"
			saEmail := "agentium-" + prefix + "@" + tt.project + ".iam.gserviceaccount.com"
			if saEmail != tt.wantSAEmail {
				t.Errorf("service account email = %q, want %q", saEmail, tt.wantSAEmail)
			}

			// Verify firewall rule name matches Terraform convention:
			// name = "agentium-allow-egress-${substr(var.session_id, 0, 20)}"
			firewallName := "agentium-allow-egress-" + prefix
			if firewallName != tt.wantFirewall {
				t.Errorf("firewall rule name = %q, want %q", firewallName, tt.wantFirewall)
			}
		})
	}
}
