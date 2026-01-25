package security

import (
	"testing"
)

func TestIAMValidator_ValidatePolicy(t *testing.T) {
	validator := NewIAMValidator()

	tests := []struct {
		name    string
		policy  IAMPolicy
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid GCP policy",
			policy: IAMPolicy{
				Provider: "gcp",
				Role:     "custom-role",
				Actions: []string{
					"secretmanager.versions.access",
					"logging.logEntries.create",
					"compute.instances.get",
				},
				Resource: "projects/my-project/instances/my-instance",
			},
			wantErr: false,
		},
		{
			name: "dangerous GCP action",
			policy: IAMPolicy{
				Provider: "gcp",
				Role:     "dangerous-role",
				Actions: []string{
					"secretmanager.versions.access",
					"iam.serviceAccountKeys.create", // Dangerous!
				},
				Resource: "projects/my-project",
			},
			wantErr: true,
			errMsg:  "dangerous action detected",
		},
		{
			name: "wildcard permission",
			policy: IAMPolicy{
				Provider: "gcp",
				Role:     "too-broad",
				Actions: []string{
					"compute.*", // Too broad!
				},
				Resource: "projects/my-project",
			},
			wantErr: true,
			errMsg:  "wildcard permission detected",
		},
		{
			name: "acceptable wildcard - logs",
			policy: IAMPolicy{
				Provider: "gcp",
				Role:     "logging-role",
				Actions: []string{
					"logs:*",
				},
				Resource: "projects/my-project/logs/*",
			},
			wantErr: false,
		},
		{
			name: "resource too broad",
			policy: IAMPolicy{
				Provider: "aws",
				Role:     "broad-resource",
				Actions: []string{
					"secretsmanager:GetSecretValue",
				},
				Resource: "*", // Too broad!
			},
			wantErr: true,
			errMsg:  "resource scope too broad",
		},
		{
			name: "valid AWS policy",
			policy: IAMPolicy{
				Provider: "aws",
				Role:     "minimal-role",
				Actions: []string{
					"secretsmanager:GetSecretValue",
					"logs:CreateLogGroup",
					"logs:CreateLogStream",
					"logs:PutLogEvents",
				},
				Resource: "arn:aws:secretsmanager:us-east-1:123456789012:secret:MySecret",
			},
			wantErr: false,
		},
		{
			name: "dangerous AWS action",
			policy: IAMPolicy{
				Provider: "aws",
				Role:     "dangerous-aws",
				Actions: []string{
					"iam:CreateAccessKey", // Dangerous!
				},
				Resource: "arn:aws:iam::123456789012:user/TestUser",
			},
			wantErr: true,
			errMsg:  "dangerous action detected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidatePolicy(tt.policy)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePolicy() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
				t.Errorf("ValidatePolicy() error = %v, want error containing %v", err, tt.errMsg)
			}
		})
	}
}

func TestIAMValidator_CheckRequiredPermissions(t *testing.T) {
	validator := NewIAMValidator()

	tests := []struct {
		name           string
		provider       string
		grantedActions []string
		wantErr        bool
		errMsg         string
	}{
		{
			name:     "all required GCP permissions present",
			provider: "gcp",
			grantedActions: []string{
				"secretmanager.versions.access",
				"logging.logEntries.create",
				"compute.instances.delete",
				"compute.instances.get",
			},
			wantErr: false,
		},
		{
			name:     "missing GCP permission",
			provider: "gcp",
			grantedActions: []string{
				"secretmanager.versions.access",
				"logging.logEntries.create",
				// Missing compute.instances.delete
				"compute.instances.get",
			},
			wantErr: true,
			errMsg:  "missing required permissions",
		},
		{
			name:     "wildcard covers required",
			provider: "gcp",
			grantedActions: []string{
				"secretmanager.*",
				"logging.*",
				"compute.*",
			},
			wantErr: false,
		},
		{
			name:     "all required AWS permissions present",
			provider: "aws",
			grantedActions: []string{
				"secretsmanager:GetSecretValue",
				"logs:CreateLogGroup",
				"logs:CreateLogStream",
				"logs:PutLogEvents",
				"ec2:TerminateInstances",
			},
			wantErr: false,
		},
		{
			name:     "unknown provider",
			provider: "unknown",
			grantedActions: []string{
				"some:permission",
			},
			wantErr: true,
			errMsg:  "unknown provider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.CheckRequiredPermissions(tt.provider, tt.grantedActions)
			if (err != nil) != tt.wantErr {
				t.Errorf("CheckRequiredPermissions() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
				t.Errorf("CheckRequiredPermissions() error = %v, want error containing %v", err, tt.errMsg)
			}
		})
	}
}

func TestMatchesAction(t *testing.T) {
	tests := []struct {
		name     string
		granted  string
		required string
		want     bool
	}{
		{
			name:     "exact match",
			granted:  "compute.instances.delete",
			required: "compute.instances.delete",
			want:     true,
		},
		{
			name:     "wildcard match with dot",
			granted:  "compute.*",
			required: "compute.instances.delete",
			want:     true,
		},
		{
			name:     "wildcard match with colon",
			granted:  "ec2:*",
			required: "ec2:TerminateInstances",
			want:     true,
		},
		{
			name:     "no match",
			granted:  "storage.buckets.list",
			required: "compute.instances.delete",
			want:     false,
		},
		{
			name:     "partial match not allowed",
			granted:  "compute.inst",
			required: "compute.instances.delete",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchesAction(tt.granted, tt.required); got != tt.want {
				t.Errorf("matchesAction() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr || len(s) > len(substr) && contains(s[1:], substr)
}