package gcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	compute "google.golang.org/api/compute/v1"
)

// mockMetadataAPI implements MetadataAPI for testing.
type mockMetadataAPI struct {
	getInstance  func(ctx context.Context, project, zone, instance string) (*compute.Instance, error)
	setMetadata  func(ctx context.Context, project, zone, instance string, metadata *compute.Metadata) error
	setCallCount int
	lastMetadata *compute.Metadata
}

func (m *mockMetadataAPI) GetInstance(ctx context.Context, project, zone, instance string) (*compute.Instance, error) {
	return m.getInstance(ctx, project, zone, instance)
}

func (m *mockMetadataAPI) SetMetadata(ctx context.Context, project, zone, instance string, metadata *compute.Metadata) error {
	m.setCallCount++
	m.lastMetadata = metadata
	return m.setMetadata(ctx, project, zone, instance, metadata)
}

func TestComputeMetadataUpdater_InterfaceCompliance(t *testing.T) {
	var _ MetadataUpdater = (*ComputeMetadataUpdater)(nil)
}

func TestComputeMetadataUpdater_UpdateStatus_AddsNewKey(t *testing.T) {
	existingValue := "existing-value"
	mock := &mockMetadataAPI{
		getInstance: func(ctx context.Context, project, zone, instance string) (*compute.Instance, error) {
			return &compute.Instance{
				Metadata: &compute.Metadata{
					Fingerprint: "fp123",
					Items: []*compute.MetadataItems{
						{Key: "other-key", Value: &existingValue},
					},
				},
			}, nil
		},
		setMetadata: func(ctx context.Context, project, zone, instance string, metadata *compute.Metadata) error {
			return nil
		},
	}

	updater := NewComputeMetadataUpdaterWithAPI(mock, "test-project", "us-central1-a", "test-instance")

	status := SessionStatusMetadata{
		Iteration:      2,
		MaxIterations:  5,
		CompletedTasks: []string{"1"},
		PendingTasks:   []string{"2", "3"},
	}

	err := updater.UpdateStatus(context.Background(), status)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.setCallCount != 1 {
		t.Fatalf("expected 1 SetMetadata call, got %d", mock.setCallCount)
	}

	// Should have 2 items: existing + new agentium-status
	if len(mock.lastMetadata.Items) != 2 {
		t.Fatalf("expected 2 metadata items, got %d", len(mock.lastMetadata.Items))
	}

	// Find the agentium-status item
	var found *compute.MetadataItems
	for _, item := range mock.lastMetadata.Items {
		if item.Key == "agentium-status" {
			found = item
			break
		}
	}
	if found == nil {
		t.Fatal("agentium-status key not found in metadata")
	}

	// Verify JSON content
	var parsed SessionStatusMetadata
	if err := json.Unmarshal([]byte(*found.Value), &parsed); err != nil {
		t.Fatalf("failed to parse agentium-status value: %v", err)
	}
	if parsed.Iteration != 2 {
		t.Errorf("expected iteration 2, got %d", parsed.Iteration)
	}
	if parsed.MaxIterations != 5 {
		t.Errorf("expected max_iterations 5, got %d", parsed.MaxIterations)
	}
	if len(parsed.CompletedTasks) != 1 || parsed.CompletedTasks[0] != "1" {
		t.Errorf("unexpected completed_tasks: %v", parsed.CompletedTasks)
	}
	if len(parsed.PendingTasks) != 2 {
		t.Errorf("unexpected pending_tasks: %v", parsed.PendingTasks)
	}
}

func TestComputeMetadataUpdater_UpdateStatus_UpdatesExistingKey(t *testing.T) {
	oldValue := `{"iteration":1,"max_iterations":5,"completed_tasks":[],"pending_tasks":["1","2"]}`
	mock := &mockMetadataAPI{
		getInstance: func(ctx context.Context, project, zone, instance string) (*compute.Instance, error) {
			return &compute.Instance{
				Metadata: &compute.Metadata{
					Fingerprint: "fp456",
					Items: []*compute.MetadataItems{
						{Key: "agentium-status", Value: &oldValue},
					},
				},
			}, nil
		},
		setMetadata: func(ctx context.Context, project, zone, instance string, metadata *compute.Metadata) error {
			return nil
		},
	}

	updater := NewComputeMetadataUpdaterWithAPI(mock, "test-project", "us-central1-a", "test-instance")

	status := SessionStatusMetadata{
		Iteration:      3,
		MaxIterations:  5,
		CompletedTasks: []string{"1", "2"},
		PendingTasks:   []string{},
	}

	err := updater.UpdateStatus(context.Background(), status)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should still have 1 item (updated in place)
	if len(mock.lastMetadata.Items) != 1 {
		t.Fatalf("expected 1 metadata item, got %d", len(mock.lastMetadata.Items))
	}

	var parsed SessionStatusMetadata
	if err := json.Unmarshal([]byte(*mock.lastMetadata.Items[0].Value), &parsed); err != nil {
		t.Fatalf("failed to parse value: %v", err)
	}
	if parsed.Iteration != 3 {
		t.Errorf("expected iteration 3, got %d", parsed.Iteration)
	}
	if len(parsed.CompletedTasks) != 2 {
		t.Errorf("expected 2 completed_tasks, got %d", len(parsed.CompletedTasks))
	}
	if len(parsed.PendingTasks) != 0 {
		t.Errorf("expected 0 pending_tasks, got %d", len(parsed.PendingTasks))
	}
}

func TestComputeMetadataUpdater_UpdateStatus_GetInstanceError(t *testing.T) {
	mock := &mockMetadataAPI{
		getInstance: func(ctx context.Context, project, zone, instance string) (*compute.Instance, error) {
			return nil, errors.New("api error: not found")
		},
		setMetadata: func(ctx context.Context, project, zone, instance string, metadata *compute.Metadata) error {
			return nil
		},
	}

	updater := NewComputeMetadataUpdaterWithAPI(mock, "test-project", "us-central1-a", "test-instance")

	err := updater.UpdateStatus(context.Background(), SessionStatusMetadata{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to get instance metadata") {
		t.Errorf("unexpected error message: %v", err)
	}

	if mock.setCallCount != 0 {
		t.Errorf("SetMetadata should not have been called, but was called %d times", mock.setCallCount)
	}
}

func TestComputeMetadataUpdater_UpdateStatus_SetMetadataError(t *testing.T) {
	mock := &mockMetadataAPI{
		getInstance: func(ctx context.Context, project, zone, instance string) (*compute.Instance, error) {
			return &compute.Instance{
				Metadata: &compute.Metadata{
					Fingerprint: "fp789",
					Items:       []*compute.MetadataItems{},
				},
			}, nil
		},
		setMetadata: func(ctx context.Context, project, zone, instance string, metadata *compute.Metadata) error {
			return errors.New("api error: permission denied")
		},
	}

	updater := NewComputeMetadataUpdaterWithAPI(mock, "test-project", "us-central1-a", "test-instance")

	err := updater.UpdateStatus(context.Background(), SessionStatusMetadata{Iteration: 1, MaxIterations: 3})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to set instance metadata") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestComputeMetadataUpdater_UpdateStatus_JSONFields(t *testing.T) {
	mock := &mockMetadataAPI{
		getInstance: func(ctx context.Context, project, zone, instance string) (*compute.Instance, error) {
			return &compute.Instance{
				Metadata: &compute.Metadata{Items: []*compute.MetadataItems{}},
			}, nil
		},
		setMetadata: func(ctx context.Context, project, zone, instance string, metadata *compute.Metadata) error {
			return nil
		},
	}

	updater := NewComputeMetadataUpdaterWithAPI(mock, "p", "z", "i")

	status := SessionStatusMetadata{
		Iteration:      4,
		MaxIterations:  10,
		CompletedTasks: []string{"issue:5", "pr:12"},
		PendingTasks:   []string{"issue:7"},
	}

	if err := updater.UpdateStatus(context.Background(), status); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	raw := *mock.lastMetadata.Items[0].Value

	// Verify it's valid JSON that matches provisioner's expected format
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	expectedKeys := []string{"iteration", "max_iterations", "completed_tasks", "pending_tasks"}
	for _, key := range expectedKeys {
		if _, ok := m[key]; !ok {
			t.Errorf("missing expected JSON key %q", key)
		}
	}

	if m["iteration"].(float64) != 4 {
		t.Errorf("expected iteration 4, got %v", m["iteration"])
	}
	if m["max_iterations"].(float64) != 10 {
		t.Errorf("expected max_iterations 10, got %v", m["max_iterations"])
	}
}

func TestComputeMetadataUpdater_Close(t *testing.T) {
	updater := NewComputeMetadataUpdaterWithAPI(nil, "p", "z", "i")
	if err := updater.Close(); err != nil {
		t.Errorf("Close() should return nil, got: %v", err)
	}
}
