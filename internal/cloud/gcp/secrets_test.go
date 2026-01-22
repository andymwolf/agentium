package gcp

import (
	"context"
	"errors"
	"os"
	"testing"
)

// mockSecretFetcher implements SecretFetcher for testing
type mockSecretFetcher struct {
	fetchFunc func(ctx context.Context, secretPath string) (string, error)
	closeFunc func() error
}

func (m *mockSecretFetcher) FetchSecret(ctx context.Context, secretPath string) (string, error) {
	if m.fetchFunc != nil {
		return m.fetchFunc(ctx, secretPath)
	}
	return "", errors.New("mock fetch not implemented")
}

func (m *mockSecretFetcher) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

func TestNormalizeSecretPath(t *testing.T) {
	tests := []struct {
		name       string
		projectID  string
		secretPath string
		want       string
	}{
		{
			name:       "full path with version",
			projectID:  "test-project",
			secretPath: "projects/my-project/secrets/my-secret/versions/1",
			want:       "projects/my-project/secrets/my-secret/versions/1",
		},
		{
			name:       "full path with latest version",
			projectID:  "test-project",
			secretPath: "projects/my-project/secrets/my-secret/versions/latest",
			want:       "projects/my-project/secrets/my-secret/versions/latest",
		},
		{
			name:       "full path without version",
			projectID:  "test-project",
			secretPath: "projects/my-project/secrets/my-secret",
			want:       "projects/my-project/secrets/my-secret/versions/latest",
		},
		{
			name:       "secret name only",
			projectID:  "test-project",
			secretPath: "my-secret",
			want:       "projects/test-project/secrets/my-secret/versions/latest",
		},
		{
			name:       "secret name with path prefix",
			projectID:  "test-project",
			secretPath: "path/to/my-secret",
			want:       "projects/test-project/secrets/my-secret/versions/latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &SecretManagerClient{
				projectID: tt.projectID,
			}
			got := client.normalizeSecretPath(tt.secretPath)
			if got != tt.want {
				t.Errorf("normalizeSecretPath(%q) = %q, want %q", tt.secretPath, got, tt.want)
			}
		})
	}
}

func TestMockSecretFetcher_FetchSecret_Success(t *testing.T) {
	expectedSecret := "super-secret-value"
	expectedPath := "projects/test-project/secrets/test-secret/versions/latest"

	mock := &mockSecretFetcher{
		fetchFunc: func(ctx context.Context, secretPath string) (string, error) {
			if secretPath != expectedPath {
				t.Errorf("FetchSecret called with path %q, want %q", secretPath, expectedPath)
			}
			return expectedSecret, nil
		},
	}

	ctx := context.Background()
	secret, err := mock.FetchSecret(ctx, expectedPath)

	if err != nil {
		t.Errorf("FetchSecret() unexpected error: %v", err)
	}

	if secret != expectedSecret {
		t.Errorf("FetchSecret() = %q, want %q", secret, expectedSecret)
	}
}

func TestMockSecretFetcher_FetchSecret_Error(t *testing.T) {
	tests := []struct {
		name          string
		secretPath    string
		expectedError string
	}{
		{
			name:          "secret not found",
			secretPath:    "projects/test-project/secrets/missing-secret/versions/latest",
			expectedError: "secret not found",
		},
		{
			name:          "permission denied",
			secretPath:    "projects/test-project/secrets/forbidden-secret/versions/latest",
			expectedError: "permission denied",
		},
		{
			name:          "invalid secret path",
			secretPath:    "invalid-path",
			expectedError: "invalid secret path format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockSecretFetcher{
				fetchFunc: func(ctx context.Context, secretPath string) (string, error) {
					return "", errors.New(tt.expectedError)
				},
			}

			ctx := context.Background()
			secret, err := mock.FetchSecret(ctx, tt.secretPath)

			if err == nil {
				t.Errorf("FetchSecret() expected error, got nil")
			}

			if secret != "" {
				t.Errorf("FetchSecret() = %q, want empty string on error", secret)
			}

			if err.Error() != tt.expectedError {
				t.Errorf("FetchSecret() error = %q, want %q", err.Error(), tt.expectedError)
			}
		})
	}
}

func TestMockSecretFetcher_Close(t *testing.T) {
	tests := []struct {
		name      string
		closeFunc func() error
		wantErr   bool
	}{
		{
			name: "successful close",
			closeFunc: func() error {
				return nil
			},
			wantErr: false,
		},
		{
			name: "close with error",
			closeFunc: func() error {
				return errors.New("close failed")
			},
			wantErr: true,
		},
		{
			name:      "close with nil function",
			closeFunc: nil,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockSecretFetcher{
				closeFunc: tt.closeFunc,
			}

			err := mock.Close()

			if tt.wantErr && err == nil {
				t.Errorf("Close() expected error, got nil")
			}

			if !tt.wantErr && err != nil {
				t.Errorf("Close() unexpected error: %v", err)
			}
		})
	}
}

func TestSecretFetcherInterface(t *testing.T) {
	// Verify that SecretManagerClient implements SecretFetcher
	var _ SecretFetcher = (*SecretManagerClient)(nil)

	// Verify that mockSecretFetcher implements SecretFetcher
	var _ SecretFetcher = (*mockSecretFetcher)(nil)
}

func TestSecretManagerClient_Close_Nil(t *testing.T) {
	// Test that Close handles nil client gracefully
	client := &SecretManagerClient{
		client: nil,
	}

	err := client.Close()
	if err != nil {
		t.Errorf("Close() with nil client unexpected error: %v", err)
	}
}

func TestGetProjectID(t *testing.T) {
	tests := []struct {
		name      string
		envVars   map[string]string
		wantErr   bool
		checkFunc func(t *testing.T, projectID string)
	}{
		{
			name: "GOOGLE_CLOUD_PROJECT env var",
			envVars: map[string]string{
				"GOOGLE_CLOUD_PROJECT": "test-project-1",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, projectID string) {
				if projectID != "test-project-1" {
					t.Errorf("getProjectID() = %q, want %q", projectID, "test-project-1")
				}
			},
		},
		{
			name: "GCP_PROJECT env var",
			envVars: map[string]string{
				"GCP_PROJECT": "test-project-2",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, projectID string) {
				if projectID != "test-project-2" {
					t.Errorf("getProjectID() = %q, want %q", projectID, "test-project-2")
				}
			},
		},
		{
			name: "GCLOUD_PROJECT env var",
			envVars: map[string]string{
				"GCLOUD_PROJECT": "test-project-3",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, projectID string) {
				if projectID != "test-project-3" {
					t.Errorf("getProjectID() = %q, want %q", projectID, "test-project-3")
				}
			},
		},
		{
			name: "GOOGLE_CLOUD_PROJECT takes precedence",
			envVars: map[string]string{
				"GOOGLE_CLOUD_PROJECT": "test-project-1",
				"GCP_PROJECT":          "test-project-2",
				"GCLOUD_PROJECT":       "test-project-3",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, projectID string) {
				if projectID != "test-project-1" {
					t.Errorf("getProjectID() = %q, want %q", projectID, "test-project-1")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore environment
			oldEnv := map[string]string{
				"GOOGLE_CLOUD_PROJECT": os.Getenv("GOOGLE_CLOUD_PROJECT"),
				"GCP_PROJECT":          os.Getenv("GCP_PROJECT"),
				"GCLOUD_PROJECT":       os.Getenv("GCLOUD_PROJECT"),
			}
			defer func() {
				for k, v := range oldEnv {
					if v == "" {
						os.Unsetenv(k)
					} else {
						os.Setenv(k, v)
					}
				}
			}()

			// Clear all project-related env vars
			os.Unsetenv("GOOGLE_CLOUD_PROJECT")
			os.Unsetenv("GCP_PROJECT")
			os.Unsetenv("GCLOUD_PROJECT")

			// Set test env vars
			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}

			ctx := context.Background()
			projectID, err := getProjectID(ctx)

			if tt.wantErr {
				if err == nil {
					t.Errorf("getProjectID() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("getProjectID() unexpected error: %v", err)
				}
				if tt.checkFunc != nil {
					tt.checkFunc(t, projectID)
				}
			}
		})
	}
}
