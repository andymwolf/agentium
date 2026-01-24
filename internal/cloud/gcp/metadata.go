package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
)

// MetadataUpdater defines the interface for updating GCP instance metadata.
type MetadataUpdater interface {
	UpdateStatus(ctx context.Context, status SessionStatusMetadata) error
	Close() error
}

// SessionStatusMetadata is the JSON structure written to the "agentium-status"
// instance metadata key. It matches the format parsed by the provisioner.
type SessionStatusMetadata struct {
	Iteration      int      `json:"iteration"`
	MaxIterations  int      `json:"max_iterations"`
	CompletedTasks []string `json:"completed_tasks"`
	PendingTasks   []string `json:"pending_tasks"`
}

// MetadataAPI is a thin interface around the Compute API methods needed
// for metadata updates. This enables testing with mocks.
type MetadataAPI interface {
	GetInstance(ctx context.Context, project, zone, instance string) (*compute.Instance, error)
	SetMetadata(ctx context.Context, project, zone, instance string, metadata *compute.Metadata) error
}

// computeMetadataAPI wraps the real Compute API service.
type computeMetadataAPI struct {
	service *compute.Service
}

func (a *computeMetadataAPI) GetInstance(ctx context.Context, project, zone, instance string) (*compute.Instance, error) {
	return a.service.Instances.Get(project, zone, instance).Context(ctx).Do()
}

func (a *computeMetadataAPI) SetMetadata(ctx context.Context, project, zone, instance string, metadata *compute.Metadata) error {
	_, err := a.service.Instances.SetMetadata(project, zone, instance, metadata).Context(ctx).Do()
	return err
}

// ComputeMetadataUpdater implements MetadataUpdater using the GCP Compute API.
type ComputeMetadataUpdater struct {
	api      MetadataAPI
	project  string
	zone     string
	instance string
}

// NewComputeMetadataUpdater creates a MetadataUpdater that auto-discovers
// project, zone, and instance name from the GCP metadata server.
func NewComputeMetadataUpdater(ctx context.Context, opts ...option.ClientOption) (*ComputeMetadataUpdater, error) {
	service, err := compute.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create compute service: %w", err)
	}

	project, err := getInstanceMetadataField(ctx, "project/project-id")
	if err != nil {
		return nil, fmt.Errorf("failed to get project ID: %w", err)
	}

	zoneRaw, err := getInstanceMetadataField(ctx, "instance/zone")
	if err != nil {
		return nil, fmt.Errorf("failed to get zone: %w", err)
	}
	// Zone comes as "projects/PROJECT/zones/ZONE", extract last segment
	parts := strings.Split(zoneRaw, "/")
	zone := parts[len(parts)-1]

	instance, err := getInstanceMetadataField(ctx, "instance/name")
	if err != nil {
		return nil, fmt.Errorf("failed to get instance name: %w", err)
	}

	return &ComputeMetadataUpdater{
		api:      &computeMetadataAPI{service: service},
		project:  project,
		zone:     zone,
		instance: instance,
	}, nil
}

// NewComputeMetadataUpdaterWithAPI creates a MetadataUpdater with an injected
// MetadataAPI implementation. Used for testing.
func NewComputeMetadataUpdaterWithAPI(api MetadataAPI, project, zone, instance string) *ComputeMetadataUpdater {
	return &ComputeMetadataUpdater{
		api:      api,
		project:  project,
		zone:     zone,
		instance: instance,
	}
}

// UpdateStatus writes the session status to the "agentium-status" metadata key.
// It fetches current metadata first to obtain the fingerprint for atomic updates.
func (u *ComputeMetadataUpdater) UpdateStatus(ctx context.Context, status SessionStatusMetadata) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Get current instance metadata (we need the fingerprint)
	inst, err := u.api.GetInstance(ctx, u.project, u.zone, u.instance)
	if err != nil {
		return fmt.Errorf("failed to get instance metadata: %w", err)
	}

	// Marshal the status to JSON
	statusJSON, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("failed to marshal status: %w", err)
	}

	statusStr := string(statusJSON)

	// Upsert the agentium-status key in metadata items
	metadata := inst.Metadata
	found := false
	for _, item := range metadata.Items {
		if item.Key == "agentium-status" {
			item.Value = &statusStr
			found = true
			break
		}
	}
	if !found {
		metadata.Items = append(metadata.Items, &compute.MetadataItems{
			Key:   "agentium-status",
			Value: &statusStr,
		})
	}

	// Set updated metadata
	if err := u.api.SetMetadata(ctx, u.project, u.zone, u.instance, metadata); err != nil {
		return fmt.Errorf("failed to set instance metadata: %w", err)
	}

	return nil
}

// Close is a no-op for the compute metadata updater (the service has no Close method).
func (u *ComputeMetadataUpdater) Close() error {
	return nil
}

// IsRunningOnGCP returns true if the GCP metadata server is reachable,
// indicating the code is running on a GCP instance. Uses a short timeout
// to avoid blocking startup on non-GCP environments.
func IsRunningOnGCP() bool {
	client := &http.Client{Timeout: 200 * time.Millisecond}
	req, err := http.NewRequest("GET", "http://metadata.google.internal/computeMetadata/v1/", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Metadata-Flavor", "Google")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// getInstanceMetadataField fetches a single field from the GCP metadata server.
// The field should be relative to the metadata root, e.g. "instance/name" or "project/project-id".
func getInstanceMetadataField(ctx context.Context, field string) (string, error) {
	url := fmt.Sprintf("http://metadata.google.internal/computeMetadata/v1/%s", field)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create metadata request: %w", err)
	}
	req.Header.Set("Metadata-Flavor", "Google")

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch metadata field %s: %w", field, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("metadata server returned status %d for field %s", resp.StatusCode, field)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read metadata response: %w", err)
	}

	value := strings.TrimSpace(string(body))
	if value == "" {
		return "", fmt.Errorf("empty value for metadata field %s", field)
	}

	return value, nil
}
