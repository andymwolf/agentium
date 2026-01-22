package gcp

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"google.golang.org/api/option"
)

// SecretManagerClient wraps the GCP Secret Manager client
type SecretManagerClient struct {
	client *secretmanager.Client
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

	return &SecretManagerClient{
		client: client,
	}, nil
}

// FetchSecret retrieves a secret from GCP Secret Manager
// secretPath can be in one of the following formats:
// - projects/PROJECT_ID/secrets/SECRET_NAME/versions/VERSION
// - projects/PROJECT_ID/secrets/SECRET_NAME (defaults to latest)
// - SECRET_NAME (requires projectID from environment)
func (c *SecretManagerClient) FetchSecret(ctx context.Context, secretPath string) (string, error) {
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

	// If it's just a secret name, we need to construct the path
	// This is a simplified case - in production, you'd need project ID from config
	secretName := filepath.Base(secretPath)
	return fmt.Sprintf("projects/*/secrets/%s/versions/latest", secretName)
}

// Close closes the Secret Manager client
func (c *SecretManagerClient) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}
