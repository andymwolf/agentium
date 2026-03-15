package provisioner

import "testing"

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

func TestSharedServiceAccountEmail(t *testing.T) {
	// This test verifies that the shared service account email follows
	// the expected naming convention: agentium-shared@PROJECT.iam.gserviceaccount.com
	tests := []struct {
		name        string
		project     string
		wantSAEmail string
	}{
		{
			name:        "standard project",
			project:     "my-project",
			wantSAEmail: "agentium-shared@my-project.iam.gserviceaccount.com",
		},
		{
			name:        "short project",
			project:     "proj",
			wantSAEmail: "agentium-shared@proj.iam.gserviceaccount.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			saEmail := sharedServiceAccountName + "@" + tt.project + ".iam.gserviceaccount.com"
			if saEmail != tt.wantSAEmail {
				t.Errorf("service account email = %q, want %q", saEmail, tt.wantSAEmail)
			}
		})
	}
}

func TestExtractZoneFromPolicy(t *testing.T) {
	p := &GCPProvisioner{project: "my-project"}

	bindings := []iamBinding{
		{
			Role:    "roles/compute.instanceAdmin.v1",
			Members: []string{"serviceAccount:agentium-shared@my-project.iam.gserviceaccount.com"},
			Condition: &iamCondition{
				Title:      "agentium-instance-session-abc",
				Expression: "resource.name == 'projects/my-project/zones/us-central1-a/instances/session-abc'",
			},
		},
		{
			Role:    "roles/compute.instanceAdmin.v1",
			Members: []string{"serviceAccount:agentium-shared@my-project.iam.gserviceaccount.com"},
			Condition: &iamCondition{
				Title:      "agentium-instance-session-def",
				Expression: "resource.name == 'projects/my-project/zones/europe-west1-b/instances/session-def'",
			},
		},
		{
			Role:    "roles/secretmanager.secretAccessor",
			Members: []string{"serviceAccount:agentium-shared@my-project.iam.gserviceaccount.com"},
		},
	}

	tests := []struct {
		name      string
		sessionID string
		want      string
	}{
		{
			name:      "extracts zone from matching binding",
			sessionID: "session-abc",
			want:      "us-central1-a",
		},
		{
			name:      "extracts zone from second binding",
			sessionID: "session-def",
			want:      "europe-west1-b",
		},
		{
			name:      "returns empty for unknown session",
			sessionID: "session-unknown",
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.extractZoneFromPolicy(bindings, tt.sessionID)
			if got != tt.want {
				t.Errorf("extractZoneFromPolicy(%q) = %q, want %q", tt.sessionID, got, tt.want)
			}
		})
	}
}

func TestIsAlreadyExistsError(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "resource already exists",
			output: "ERROR: (gcloud.compute.firewall-rules.create) Could not create firewall rule: The resource 'projects/my-project/global/firewalls/agentium-allow-egress' already exists",
			want:   true,
		},
		{
			name:   "already exists lowercase",
			output: "the resource already exists in project",
			want:   true,
		},
		{
			name:   "not found error",
			output: "ERROR: The resource was not found",
			want:   false,
		},
		{
			name:   "permission denied error",
			output: "ERROR: Permission denied",
			want:   false,
		},
		{
			name:   "empty output",
			output: "",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isAlreadyExistsError(tt.output)
			if got != tt.want {
				t.Errorf("isAlreadyExistsError(%q) = %v, want %v", tt.output, got, tt.want)
			}
		})
	}
}
