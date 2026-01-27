package gcp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"google.golang.org/api/option"
)

// SecretManagerClient wraps the GCP Secret Manager client
type SecretManagerClient struct {
	client    *secretmanager.Client
	projectID string
}

// SecretFetcher defines the interface for fetching secrets
type SecretFetcher interface {
	FetchSecret(ctx context.Context, secretPath string) (string, error)
	Close() error
}

// NewSecretManagerClient creates a new Secret Manager client
func NewSecretManagerClient(ctx context.Context, opts ...option.ClientOption) (*SecretManagerClient, error) {
	client, err := secretmanager.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create secret manager client: %w", err)
	}

	// Get the project ID from environment or metadata server
	projectID, err := getProjectID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get project ID: %w", err)
	}

	return &SecretManagerClient{
		client:    client,
		projectID: projectID,
	}, nil
}

// getProjectID retrieves the GCP project ID from environment variable or metadata server
func getProjectID(ctx context.Context) (string, error) {
	// Try environment variables first
	if projectID := os.Getenv("GOOGLE_CLOUD_PROJECT"); projectID != "" {
		return projectID, nil
	}
	if projectID := os.Getenv("GCP_PROJECT"); projectID != "" {
		return projectID, nil
	}
	if projectID := os.Getenv("GCLOUD_PROJECT"); projectID != "" {
		return projectID, nil
	}

	// Fall back to metadata server (works on GCP VMs, Cloud Run, etc.)
	return getProjectIDFromMetadata(ctx)
}

// getProjectIDFromMetadata fetches the project ID from GCP metadata server
func getProjectIDFromMetadata(ctx context.Context) (string, error) {
	const metadataURL = "http://metadata.google.internal/computeMetadata/v1/project/project-id"

	// Create request with timeout
	req, err := http.NewRequestWithContext(ctx, "GET", metadataURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create metadata request: %w", err)
	}

	// Required header for GCP metadata server
	req.Header.Set("Metadata-Flavor", "Google")

	// Use a short timeout for metadata server
	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch project ID from metadata server: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("metadata server returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read metadata response: %w", err)
	}

	projectID := strings.TrimSpace(string(body))
	if projectID == "" {
		return "", fmt.Errorf("empty project ID from metadata server")
	}

	return projectID, nil
}

// FetchSecret retrieves a secret from GCP Secret Manager
// secretPath can be in one of the following formats:
// - projects/PROJECT_ID/secrets/SECRET_NAME/versions/VERSION
// - projects/PROJECT_ID/secrets/SECRET_NAME (defaults to latest)
// - SECRET_NAME (requires projectID from environment)
func (c *SecretManagerClient) FetchSecret(ctx context.Context, secretPath string) (string, error) {
	// Add timeout to prevent hanging if the API is slow
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Normalize the secret path
	name := c.normalizeSecretPath(secretPath)

	// Create the request
	req := &secretmanagerpb.AccessSecretVersionRequest{
		Name: name,
	}

	// Call the API
	result, err := c.client.AccessSecretVersion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to access secret version: %w", err)
	}

	// Return the secret payload
	return string(result.Payload.Data), nil
}

// normalizeSecretPath ensures the secret path is in the correct format
// If the path is just a secret name, it constructs the full path with "latest" version
func (c *SecretManagerClient) normalizeSecretPath(secretPath string) string {
	// If it's already a full path with version, return as-is
	if strings.HasPrefix(secretPath, "projects/") && strings.Contains(secretPath, "/versions/") {
		return secretPath
	}

	// If it's a full path without version, append /versions/latest
	if strings.HasPrefix(secretPath, "projects/") && strings.Contains(secretPath, "/secrets/") {
		return secretPath + "/versions/latest"
	}

	// If it's just a secret name, construct the full path using the project ID
	secretName := path.Base(secretPath)
	return fmt.Sprintf("projects/%s/secrets/%s/versions/latest", c.projectID, secretName)
}

// Close closes the Secret Manager client
func (c *SecretManagerClient) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}
