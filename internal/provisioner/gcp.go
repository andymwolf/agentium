package provisioner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// GCPProvisioner implements Provisioner for Google Cloud Platform
type GCPProvisioner struct {
	verbose     bool
	terraformDir string
}

// NewGCPProvisioner creates a new GCP provisioner
func NewGCPProvisioner(verbose bool) (*GCPProvisioner, error) {
	// Find terraform modules directory
	execPath, err := os.Executable()
	if err != nil {
		execPath = "."
	}

	// Look for terraform modules in a few common locations
	possiblePaths := []string{
		filepath.Join(filepath.Dir(execPath), "..", "terraform", "modules", "vm", "gcp"),
		filepath.Join(".", "terraform", "modules", "vm", "gcp"),
		filepath.Join(os.Getenv("HOME"), ".agentium", "terraform", "modules", "vm", "gcp"),
	}

	var terraformDir string
	for _, p := range possiblePaths {
		if _, err := os.Stat(filepath.Join(p, "main.tf")); err == nil {
			terraformDir = p
			break
		}
	}

	if terraformDir == "" {
		// Use embedded terraform or default location
		terraformDir = filepath.Join(".", "terraform", "modules", "vm", "gcp")
	}

	return &GCPProvisioner{
		verbose:     verbose,
		terraformDir: terraformDir,
	}, nil
}

// Provision creates a new GCP VM for an agent session
func (p *GCPProvisioner) Provision(ctx context.Context, config VMConfig) (*ProvisionResult, error) {
	// Create working directory for this session
	workDir := filepath.Join(os.TempDir(), "agentium", config.Session.ID)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create work directory: %w", err)
	}

	// Write session config as JSON for cloud-init
	sessionJSON, err := json.Marshal(config.Session)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal session config: %w", err)
	}

	// Create terraform.tfvars
	tfvars := fmt.Sprintf(`
session_id         = "%s"
region             = "%s"
machine_type       = "%s"
use_spot           = %t
disk_size_gb       = %d
controller_image   = "%s"
session_config     = %s
`,
		config.Session.ID,
		config.Region,
		config.MachineType,
		config.UseSpot,
		config.DiskSizeGB,
		config.ControllerImage,
		string(sessionJSON),
	)

	tfvarsPath := filepath.Join(workDir, "terraform.tfvars")
	if err := os.WriteFile(tfvarsPath, []byte(tfvars), 0644); err != nil {
		return nil, fmt.Errorf("failed to write tfvars: %w", err)
	}

	// Copy terraform files to work directory
	if err := p.copyTerraformFiles(workDir); err != nil {
		return nil, fmt.Errorf("failed to copy terraform files: %w", err)
	}

	// Run terraform init
	if err := p.runTerraform(ctx, workDir, "init"); err != nil {
		return nil, fmt.Errorf("terraform init failed: %w", err)
	}

	// Run terraform apply
	if err := p.runTerraform(ctx, workDir, "apply", "-auto-approve"); err != nil {
		return nil, fmt.Errorf("terraform apply failed: %w", err)
	}

	// Get outputs
	output, err := p.getTerraformOutput(ctx, workDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get terraform output: %w", err)
	}

	return &ProvisionResult{
		InstanceID: output["instance_id"],
		PublicIP:   output["public_ip"],
		Zone:       output["zone"],
		SessionID:  config.Session.ID,
	}, nil
}

// List returns all active Agentium sessions on GCP
func (p *GCPProvisioner) List(ctx context.Context) ([]SessionStatus, error) {
	// List all instances with the agentium label
	cmd := exec.CommandContext(ctx, "gcloud", "compute", "instances", "list",
		"--filter=labels.agentium=true",
		"--format=json",
	)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list instances: %w", err)
	}

	var instances []struct {
		Name              string `json:"name"`
		Status            string `json:"status"`
		Zone              string `json:"zone"`
		CreationTimestamp string `json:"creationTimestamp"`
		NetworkInterfaces []struct {
			AccessConfigs []struct {
				NatIP string `json:"natIP"`
			} `json:"accessConfigs"`
		} `json:"networkInterfaces"`
		Metadata struct {
			Items []struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			} `json:"items"`
		} `json:"metadata"`
	}

	if err := json.Unmarshal(output, &instances); err != nil {
		return nil, fmt.Errorf("failed to parse instances: %w", err)
	}

	sessions := make([]SessionStatus, 0, len(instances))
	for _, inst := range instances {
		status := SessionStatus{
			SessionID:  inst.Name,
			InstanceID: inst.Name,
			Zone:       filepath.Base(inst.Zone),
		}

		// Map GCP status to our status
		switch inst.Status {
		case "RUNNING":
			status.State = "running"
		case "TERMINATED", "STOPPED":
			status.State = "terminated"
		case "STAGING", "PROVISIONING":
			status.State = "starting"
		default:
			status.State = strings.ToLower(inst.Status)
		}

		// Parse creation time
		if t, err := time.Parse(time.RFC3339, inst.CreationTimestamp); err == nil {
			status.StartTime = t
		}

		// Get public IP
		if len(inst.NetworkInterfaces) > 0 && len(inst.NetworkInterfaces[0].AccessConfigs) > 0 {
			status.PublicIP = inst.NetworkInterfaces[0].AccessConfigs[0].NatIP
		}

		// Get session metadata
		for _, item := range inst.Metadata.Items {
			if item.Key == "agentium-status" {
				var sessionStatus struct {
					Iteration      int      `json:"iteration"`
					MaxIterations  int      `json:"max_iterations"`
					CompletedTasks []string `json:"completed_tasks"`
					PendingTasks   []string `json:"pending_tasks"`
				}
				if err := json.Unmarshal([]byte(item.Value), &sessionStatus); err == nil {
					status.CurrentIteration = sessionStatus.Iteration
					status.MaxIterations = sessionStatus.MaxIterations
					status.CompletedTasks = sessionStatus.CompletedTasks
					status.PendingTasks = sessionStatus.PendingTasks
				}
			}
		}

		sessions = append(sessions, status)
	}

	return sessions, nil
}

// Status gets the current status of a GCP session
func (p *GCPProvisioner) Status(ctx context.Context, sessionID string) (*SessionStatus, error) {
	// Get instance status via gcloud
	cmd := exec.CommandContext(ctx, "gcloud", "compute", "instances", "describe",
		sessionID,
		"--format=json",
	)

	output, err := cmd.Output()
	if err != nil {
		// Check if instance doesn't exist
		if strings.Contains(err.Error(), "not found") {
			return &SessionStatus{
				SessionID: sessionID,
				State:     "terminated",
			}, nil
		}
		return nil, fmt.Errorf("failed to get instance status: %w", err)
	}

	var instance struct {
		Status            string `json:"status"`
		Zone              string `json:"zone"`
		CreationTimestamp string `json:"creationTimestamp"`
		NetworkInterfaces []struct {
			AccessConfigs []struct {
				NatIP string `json:"natIP"`
			} `json:"accessConfigs"`
		} `json:"networkInterfaces"`
		Metadata struct {
			Items []struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			} `json:"items"`
		} `json:"metadata"`
	}

	if err := json.Unmarshal(output, &instance); err != nil {
		return nil, fmt.Errorf("failed to parse instance status: %w", err)
	}

	status := &SessionStatus{
		SessionID:  sessionID,
		InstanceID: sessionID,
		Zone:       filepath.Base(instance.Zone),
	}

	// Map GCP status to our status
	switch instance.Status {
	case "RUNNING":
		status.State = "running"
	case "TERMINATED", "STOPPED":
		status.State = "terminated"
	case "STAGING", "PROVISIONING":
		status.State = "starting"
	default:
		status.State = strings.ToLower(instance.Status)
	}

	// Parse creation time
	if t, err := time.Parse(time.RFC3339, instance.CreationTimestamp); err == nil {
		status.StartTime = t
	}

	// Get public IP
	if len(instance.NetworkInterfaces) > 0 && len(instance.NetworkInterfaces[0].AccessConfigs) > 0 {
		status.PublicIP = instance.NetworkInterfaces[0].AccessConfigs[0].NatIP
	}

	// Get session metadata
	for _, item := range instance.Metadata.Items {
		if item.Key == "agentium-status" {
			var sessionStatus struct {
				Iteration      int      `json:"iteration"`
				MaxIterations  int      `json:"max_iterations"`
				CompletedTasks []string `json:"completed_tasks"`
				PendingTasks   []string `json:"pending_tasks"`
			}
			if err := json.Unmarshal([]byte(item.Value), &sessionStatus); err == nil {
				status.CurrentIteration = sessionStatus.Iteration
				status.MaxIterations = sessionStatus.MaxIterations
				status.CompletedTasks = sessionStatus.CompletedTasks
				status.PendingTasks = sessionStatus.PendingTasks
			}
		}
	}

	return status, nil
}

// Logs retrieves logs from a GCP session
func (p *GCPProvisioner) Logs(ctx context.Context, sessionID string, opts LogsOptions) (<-chan LogEntry, <-chan error) {
	logCh := make(chan LogEntry, 100)
	errCh := make(chan error, 1)

	go func() {
		defer close(logCh)
		defer close(errCh)

		// Build gcloud logging read command
		args := []string{
			"logging", "read",
			fmt.Sprintf(`resource.type="gce_instance" AND resource.labels.instance_id="%s"`, sessionID),
			"--format=json",
		}

		if opts.Tail > 0 {
			args = append(args, fmt.Sprintf("--limit=%d", opts.Tail))
		}

		if !opts.Since.IsZero() {
			args = append(args, fmt.Sprintf(`--freshness=%s`, time.Since(opts.Since).Round(time.Second)))
		}

		for {
			cmd := exec.CommandContext(ctx, "gcloud", args...)
			output, err := cmd.Output()
			if err != nil {
				errCh <- fmt.Errorf("failed to read logs: %w", err)
				return
			}

			var entries []struct {
				Timestamp   string `json:"timestamp"`
				TextPayload string `json:"textPayload"`
				Severity    string `json:"severity"`
			}

			if err := json.Unmarshal(output, &entries); err != nil {
				errCh <- fmt.Errorf("failed to parse logs: %w", err)
				return
			}

			for i := len(entries) - 1; i >= 0; i-- {
				entry := entries[i]
				ts, _ := time.Parse(time.RFC3339Nano, entry.Timestamp)
				logCh <- LogEntry{
					Timestamp: ts,
					Message:   entry.TextPayload,
					Level:     entry.Severity,
				}
			}

			if !opts.Follow {
				return
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
				// Continue polling
			}
		}
	}()

	return logCh, errCh
}

// Destroy terminates a GCP session
func (p *GCPProvisioner) Destroy(ctx context.Context, sessionID string) error {
	workDir := filepath.Join(os.TempDir(), "agentium", sessionID)

	// Check if terraform state exists
	if _, err := os.Stat(filepath.Join(workDir, "terraform.tfstate")); err == nil {
		// Use terraform destroy
		if err := p.runTerraform(ctx, workDir, "destroy", "-auto-approve"); err != nil {
			// Fall back to gcloud if terraform fails
			if p.verbose {
				fmt.Fprintf(os.Stderr, "terraform destroy failed, falling back to gcloud: %v\n", err)
			}
		} else {
			// Clean up work directory
			os.RemoveAll(workDir)
			return nil
		}
	}

	// Use gcloud to delete the instance
	cmd := exec.CommandContext(ctx, "gcloud", "compute", "instances", "delete",
		sessionID,
		"--quiet",
	)

	if p.verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to delete instance: %w", err)
	}

	// Clean up work directory
	os.RemoveAll(workDir)

	return nil
}

func (p *GCPProvisioner) copyTerraformFiles(destDir string) error {
	// Read all .tf files from terraform dir and copy to dest
	entries, err := os.ReadDir(p.terraformDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tf") {
			continue
		}

		src := filepath.Join(p.terraformDir, entry.Name())
		dst := filepath.Join(destDir, entry.Name())

		data, err := os.ReadFile(src)
		if err != nil {
			return err
		}

		if err := os.WriteFile(dst, data, 0644); err != nil {
			return err
		}
	}

	return nil
}

func (p *GCPProvisioner) runTerraform(ctx context.Context, workDir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "terraform", args...)
	cmd.Dir = workDir

	if p.verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	return cmd.Run()
}

func (p *GCPProvisioner) getTerraformOutput(ctx context.Context, workDir string) (map[string]string, error) {
	cmd := exec.CommandContext(ctx, "terraform", "output", "-json")
	cmd.Dir = workDir

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var outputs map[string]struct {
		Value string `json:"value"`
	}

	if err := json.Unmarshal(output, &outputs); err != nil {
		return nil, err
	}

	result := make(map[string]string)
	for k, v := range outputs {
		result[k] = v.Value
	}

	return result, nil
}
