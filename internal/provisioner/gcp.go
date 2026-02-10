package provisioner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/andywolf/agentium/terraform"
)

// GCPProvisioner implements Provisioner for Google Cloud Platform
type GCPProvisioner struct {
	verbose           bool
	project           string
	serviceAccountKey string // path to SA JSON key file; empty = use ambient credentials
}

// NewGCPProvisioner creates a new GCP provisioner.
// When serviceAccountKey is non-empty, all terraform and gcloud commands
// authenticate using that key file instead of ambient gcloud credentials.
func NewGCPProvisioner(verbose bool, project, serviceAccountKey string) (*GCPProvisioner, error) {
	// Expand ~ in service account key path
	if strings.HasPrefix(serviceAccountKey, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to resolve home directory for service_account_key: %w", err)
		}
		serviceAccountKey = filepath.Join(home, serviceAccountKey[2:])
	}

	return &GCPProvisioner{
		verbose:           verbose,
		project:           project,
		serviceAccountKey: serviceAccountKey,
	}, nil
}

// setCredentialEnv configures the command environment to use a service account
// key file when one is configured. It sets both GOOGLE_APPLICATION_CREDENTIALS
// (for Terraform's Google provider) and CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE
// (for gcloud CLI) so that all GCP operations authenticate consistently.
func (p *GCPProvisioner) setCredentialEnv(cmd *exec.Cmd) {
	if p.serviceAccountKey == "" {
		return
	}
	cmd.Env = append(os.Environ(),
		"GOOGLE_APPLICATION_CREDENTIALS="+p.serviceAccountKey,
		"CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE="+p.serviceAccountKey,
	)
}

// Provision creates a new GCP VM for an agent session
func (p *GCPProvisioner) Provision(ctx context.Context, config VMConfig) (result *ProvisionResult, err error) {
	// Create working directory for this session with restricted permissions (0700)
	// to protect sensitive tfvars content
	workDir := filepath.Join(os.TempDir(), "agentium", config.Session.ID)
	if err = os.MkdirAll(workDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create work directory: %w", err)
	}

	// Ensure cleanup on error - workDir is removed on all error paths
	// Using named return value 'err' so defer can check final error state
	defer func() {
		if err != nil {
			_ = os.RemoveAll(workDir)
		}
	}()

	// Write session config as JSON for cloud-init
	sessionJSON, marshalErr := json.Marshal(config.Session)
	if marshalErr != nil {
		err = fmt.Errorf("failed to marshal session config: %w", marshalErr)
		return nil, err
	}

	// Convert max_duration from Go duration format (e.g. "6h") to seconds for Terraform
	maxRunDuration := "7200s" // default 2h
	if config.Session.MaxDuration != "" {
		if d, parseErr := time.ParseDuration(config.Session.MaxDuration); parseErr == nil {
			maxRunDuration = fmt.Sprintf("%ds", int(d.Seconds()))
		}
	}

	// Create terraform.tfvars
	tfvars := fmt.Sprintf(`
session_id         = "%s"
project_id         = "%s"
region             = "%s"
machine_type       = "%s"
use_spot           = %t
disk_size_gb       = %d
controller_image   = "%s"
session_config     = %s
claude_auth_mode   = "%s"
max_run_duration   = "%s"
`,
		config.Session.ID,
		config.Project,
		config.Region,
		config.MachineType,
		config.UseSpot,
		config.DiskSizeGB,
		config.ControllerImage,
		fmt.Sprintf("%q", string(sessionJSON)),
		config.Session.ClaudeAuth.AuthMode,
		maxRunDuration,
	)

	// Add auth JSON only when oauth mode with auth data present
	if config.Session.ClaudeAuth.AuthMode == "oauth" && config.Session.ClaudeAuth.AuthJSONBase64 != "" {
		tfvars += fmt.Sprintf("claude_auth_json   = \"%s\"\n", config.Session.ClaudeAuth.AuthJSONBase64)
	}

	// Add Codex auth JSON when present
	if config.Session.CodexAuth.AuthJSONBase64 != "" {
		tfvars += fmt.Sprintf("codex_auth_json    = \"%s\"\n", config.Session.CodexAuth.AuthJSONBase64)
	}

	tfvarsPath := filepath.Join(workDir, "terraform.tfvars")
	// Use 0600 permissions: tfvars may contain sensitive auth tokens
	if err = os.WriteFile(tfvarsPath, []byte(tfvars), 0600); err != nil {
		return nil, fmt.Errorf("failed to write tfvars: %w", err)
	}

	// Copy terraform files to work directory
	if err = p.copyTerraformFiles(workDir); err != nil {
		return nil, fmt.Errorf("failed to copy terraform files: %w", err)
	}

	// Run terraform init
	if err = p.runTerraform(ctx, workDir, "init"); err != nil {
		return nil, fmt.Errorf("terraform init failed: %w", err)
	}

	// Run terraform apply
	if err = p.runTerraform(ctx, workDir, "apply", "-auto-approve"); err != nil {
		return nil, fmt.Errorf("terraform apply failed: %w", err)
	}

	// Get outputs
	var output map[string]string
	output, err = p.getTerraformOutput(ctx, workDir)
	if err != nil {
		err = fmt.Errorf("failed to get terraform output: %w", err)
		return nil, err
	}

	return &ProvisionResult{
		InstanceID: output["instance_id"],
		PublicIP:   output["public_ip"],
		Zone:       output["zone"],
		SessionID:  config.Session.ID,
	}, nil
}

// buildListArgs constructs the gcloud arguments for listing instances.
func (p *GCPProvisioner) buildListArgs() []string {
	args := []string{"compute", "instances", "list",
		"--filter=labels.agentium=true",
		"--format=json",
	}
	if p.project != "" {
		args = append(args, fmt.Sprintf("--project=%s", p.project))
	}
	return args
}

// List returns all active Agentium sessions on GCP
func (p *GCPProvisioner) List(ctx context.Context) ([]SessionStatus, error) {
	// List all instances with the agentium label
	args := p.buildListArgs()
	cmd := exec.CommandContext(ctx, "gcloud", args...)
	p.setCredentialEnv(cmd)

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

// buildStatusArgs constructs the gcloud arguments for describing an instance.
// The zone parameter is required for gcloud compute instances describe.
func (p *GCPProvisioner) buildStatusArgs(sessionID, zone string) []string {
	args := []string{"compute", "instances", "describe",
		sessionID,
		fmt.Sprintf("--zone=%s", zone),
		"--format=json",
	}
	if p.project != "" {
		args = append(args, fmt.Sprintf("--project=%s", p.project))
	}
	return args
}

// Status gets the current status of a GCP session
func (p *GCPProvisioner) Status(ctx context.Context, sessionID string) (*SessionStatus, error) {
	// First, find the zone by listing instances with the agentium label
	// This is required because gcloud compute instances describe requires a zone
	sessions, err := p.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list instances to find zone: %w", err)
	}

	var zone string
	for _, s := range sessions {
		if s.SessionID == sessionID {
			zone = s.Zone
			break
		}
	}

	// If the instance wasn't found in the list, it's likely terminated
	if zone == "" {
		return &SessionStatus{
			SessionID: sessionID,
			State:     "terminated",
		}, nil
	}

	// Get instance status via gcloud with the zone
	args := p.buildStatusArgs(sessionID, zone)
	cmd := exec.CommandContext(ctx, "gcloud", args...)
	p.setCredentialEnv(cmd)

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

// gcpLogEntry represents a raw log entry from Cloud Logging JSON output.
type gcpLogEntry struct {
	Timestamp   string            `json:"timestamp"`
	TextPayload string            `json:"textPayload"`
	Severity    string            `json:"severity"`
	Labels      map[string]string `json:"labels"`
	JSONPayload struct {
		Message  string            `json:"message"`
		Severity string            `json:"severity"`
		Labels   map[string]string `json:"labels,omitempty"`
	} `json:"jsonPayload"`
}

// buildLogsArgs constructs the gcloud logging read arguments.
func (p *GCPProvisioner) buildLogsArgs(sessionID string, opts LogsOptions) []string {
	filter := fmt.Sprintf(`logName=~"agentium-session" AND jsonPayload.session_id="%s"`, sessionID)

	// Apply severity filter
	minLevel := strings.ToUpper(opts.MinLevel)
	switch minLevel {
	case "DEBUG":
		// No severity filter — show everything
	case "WARNING":
		filter += ` AND severity >= "WARNING"`
	case "ERROR":
		filter += ` AND severity >= "ERROR"`
	default:
		// Default: INFO and above (hides DEBUG events unless --events is set)
		if !opts.ShowEvents {
			filter += ` AND severity >= "INFO"`
		}
	}

	// Filter by event type (e.g., tool_use,thinking)
	if opts.EventType != "" {
		eventTypes := strings.Split(opts.EventType, ",")
		// Trim whitespace and skip empty entries (e.g., trailing commas)
		var validTypes []string
		for _, et := range eventTypes {
			et = strings.TrimSpace(et)
			if et != "" {
				validTypes = append(validTypes, et)
			}
		}
		if len(validTypes) == 1 {
			filter += fmt.Sprintf(` AND jsonPayload.labels.event_type="%s"`, validTypes[0])
		} else if len(validTypes) > 1 {
			var typeFilters []string
			for _, et := range validTypes {
				typeFilters = append(typeFilters, fmt.Sprintf(`jsonPayload.labels.event_type="%s"`, et))
			}
			filter += fmt.Sprintf(` AND (%s)`, strings.Join(typeFilters, " OR "))
		}
	}

	// Filter by iteration number
	if opts.Iteration > 0 {
		filter += fmt.Sprintf(` AND jsonPayload.labels.iteration="%d"`, opts.Iteration)
	}

	args := []string{
		"logging", "read",
		filter,
		"--format=json",
	}
	if p.project != "" {
		args = append(args, fmt.Sprintf("--project=%s", p.project))
	}
	if opts.Tail > 0 {
		args = append(args, fmt.Sprintf("--limit=%d", opts.Tail))
	}
	if !opts.Since.IsZero() {
		args = append(args, fmt.Sprintf(`--freshness=%s`, time.Since(opts.Since).Round(time.Second)))
	}
	return args
}

// parseLogEntries parses raw gcloud JSON output into LogEntry slices in chronological order.
func parseLogEntries(data []byte) ([]LogEntry, error) {
	var entries []gcpLogEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("failed to parse logs: %w", err)
	}

	var result []LogEntry
	// Cloud Logging returns entries in reverse chronological order
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		ts, _ := time.Parse(time.RFC3339Nano, entry.Timestamp)
		msg := entry.TextPayload
		level := entry.Severity
		if entry.JSONPayload.Message != "" {
			msg = entry.JSONPayload.Message
			if entry.JSONPayload.Severity != "" {
				level = entry.JSONPayload.Severity
			}
		}
		if msg == "" {
			continue
		}

		// Extract event labels (from top-level labels or jsonPayload.labels)
		labels := entry.Labels
		if len(labels) == 0 {
			labels = entry.JSONPayload.Labels
		}

		logEntry := LogEntry{
			Timestamp: ts,
			Message:   msg,
			Level:     level,
			EventType: labels["event_type"],
			ToolName:  labels["tool_name"],
		}
		result = append(result, logEntry)
	}
	return result, nil
}

// Logs retrieves logs from a GCP session
func (p *GCPProvisioner) Logs(ctx context.Context, sessionID string, opts LogsOptions) (<-chan LogEntry, <-chan error) {
	logCh := make(chan LogEntry, 100)
	errCh := make(chan error, 1)

	go func() {
		defer close(logCh)
		defer close(errCh)

		args := p.buildLogsArgs(sessionID, opts)

		for {
			cmd := exec.CommandContext(ctx, "gcloud", args...)
			p.setCredentialEnv(cmd)
			output, err := cmd.Output()
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
					errCh <- fmt.Errorf("failed to read logs: %s", strings.TrimSpace(string(exitErr.Stderr)))
				} else {
					errCh <- fmt.Errorf("failed to read logs: %w", err)
				}
				return
			}

			parsed, err := parseLogEntries(output)
			if err != nil {
				errCh <- err
				return
			}

			for _, entry := range parsed {
				logCh <- entry
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

// buildDestroyArgs constructs the gcloud arguments for deleting an instance.
func (p *GCPProvisioner) buildDestroyArgs(sessionID string) []string {
	args := []string{"compute", "instances", "delete",
		sessionID,
		"--quiet",
	}
	if p.project != "" {
		args = append(args, fmt.Sprintf("--project=%s", p.project))
	}
	return args
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
			_ = os.RemoveAll(workDir)
			return nil
		}
	}

	// Fallback: use gcloud to delete all associated resources
	if err := p.destroyFallback(ctx, sessionID); err != nil {
		return err
	}

	// Clean up work directory
	_ = os.RemoveAll(workDir)

	return nil
}

// destroyFallback deletes all GCP resources associated with a session using
// gcloud commands directly. This covers the case where Terraform state is
// missing or Terraform destroy failed. Resources are deleted in dependency
// order: instance, firewall rule, IAM bindings, service account.
func (p *GCPProvisioner) destroyFallback(ctx context.Context, sessionID string) error {
	prefix := sessionIDPrefix(sessionID)
	saEmail := fmt.Sprintf("agentium-%s@%s.iam.gserviceaccount.com", prefix, p.project)

	var errors []string

	// 1. Delete the compute instance
	if err := p.deleteInstance(ctx, sessionID); err != nil {
		errors = append(errors, fmt.Sprintf("instance: %v", err))
	}

	// 2. Delete the firewall rule
	firewallName := fmt.Sprintf("agentium-allow-egress-%s", prefix)
	if err := p.deleteFirewallRule(ctx, firewallName); err != nil {
		errors = append(errors, fmt.Sprintf("firewall: %v", err))
	}

	// 3. Remove IAM bindings for the service account
	iamRoles := []string{
		"roles/secretmanager.secretAccessor",
		"roles/logging.logWriter",
		"roles/compute.instanceAdmin.v1",
	}
	for _, role := range iamRoles {
		if err := p.removeIAMBinding(ctx, saEmail, role); err != nil {
			errors = append(errors, fmt.Sprintf("iam(%s): %v", role, err))
		}
	}

	// 4. Delete the service account
	if err := p.deleteServiceAccount(ctx, saEmail); err != nil {
		errors = append(errors, fmt.Sprintf("service-account: %v", err))
	}

	if len(errors) > 0 {
		return fmt.Errorf("fallback cleanup encountered errors: %s", strings.Join(errors, "; "))
	}

	return nil
}

// sessionIDPrefix returns the first 20 characters of a session ID, matching
// the Terraform naming convention: substr(var.session_id, 0, 20).
func sessionIDPrefix(sessionID string) string {
	if len(sessionID) > 20 {
		return sessionID[:20]
	}
	return sessionID
}

// deleteInstance deletes a GCP compute instance by name.
func (p *GCPProvisioner) deleteInstance(ctx context.Context, instanceName string) error {
	args := p.buildDestroyArgs(instanceName)
	cmd := exec.CommandContext(ctx, "gcloud", args...)
	p.setCredentialEnv(cmd)
	output, err := cmd.CombinedOutput()
	if p.verbose && len(output) > 0 {
		fmt.Fprintf(os.Stderr, "%s", output)
	}
	if err != nil {
		// Treat "not found" as success (resource already deleted)
		if isNotFoundError(string(output)) {
			if p.verbose {
				fmt.Fprintf(os.Stderr, "instance %s already deleted\n", instanceName)
			}
			return nil
		}
		return fmt.Errorf("failed to delete instance %s: %w", instanceName, err)
	}
	return nil
}

// deleteFirewallRule deletes a GCP firewall rule by name.
func (p *GCPProvisioner) deleteFirewallRule(ctx context.Context, ruleName string) error {
	args := []string{"compute", "firewall-rules", "delete", ruleName, "--quiet"}
	if p.project != "" {
		args = append(args, "--project="+p.project)
	}

	cmd := exec.CommandContext(ctx, "gcloud", args...)
	p.setCredentialEnv(cmd)
	output, err := cmd.CombinedOutput()
	if p.verbose && len(output) > 0 {
		fmt.Fprintf(os.Stderr, "%s", output)
	}
	if err != nil {
		if isNotFoundError(string(output)) {
			if p.verbose {
				fmt.Fprintf(os.Stderr, "firewall rule %s already deleted\n", ruleName)
			}
			return nil
		}
		return fmt.Errorf("failed to delete firewall rule %s: %w", ruleName, err)
	}
	return nil
}

// removeIAMBinding removes an IAM policy binding for a service account.
func (p *GCPProvisioner) removeIAMBinding(ctx context.Context, saEmail, role string) error {
	if p.project == "" {
		return fmt.Errorf("project is required to remove IAM bindings")
	}
	member := "serviceAccount:" + saEmail
	args := []string{
		"projects", "remove-iam-policy-binding", p.project,
		"--member=" + member,
		"--role=" + role,
		"--quiet",
	}

	cmd := exec.CommandContext(ctx, "gcloud", args...)
	p.setCredentialEnv(cmd)
	output, err := cmd.CombinedOutput()
	if p.verbose && len(output) > 0 {
		fmt.Fprintf(os.Stderr, "%s", output)
	}
	if err != nil {
		// Treat "not found" or "not bound" as success
		if isNotFoundError(string(output)) {
			if p.verbose {
				fmt.Fprintf(os.Stderr, "IAM binding %s for %s already removed\n", role, saEmail)
			}
			return nil
		}
		return fmt.Errorf("failed to remove IAM binding %s for %s: %w", role, saEmail, err)
	}
	return nil
}

// deleteServiceAccount deletes a GCP service account by email.
func (p *GCPProvisioner) deleteServiceAccount(ctx context.Context, saEmail string) error {
	args := []string{"iam", "service-accounts", "delete", saEmail, "--quiet"}
	if p.project != "" {
		args = append(args, "--project="+p.project)
	}

	cmd := exec.CommandContext(ctx, "gcloud", args...)
	p.setCredentialEnv(cmd)
	output, err := cmd.CombinedOutput()
	if p.verbose && len(output) > 0 {
		fmt.Fprintf(os.Stderr, "%s", output)
	}
	if err != nil {
		if isNotFoundError(string(output)) {
			if p.verbose {
				fmt.Fprintf(os.Stderr, "service account %s already deleted\n", saEmail)
			}
			return nil
		}
		return fmt.Errorf("failed to delete service account %s: %w", saEmail, err)
	}
	return nil
}

// isNotFoundError checks if gcloud command output indicates a resource was not found.
func isNotFoundError(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "not found") ||
		strings.Contains(lower, "was not found") ||
		strings.Contains(lower, "could not be found") ||
		strings.Contains(lower, "does not exist")
}

func (p *GCPProvisioner) copyTerraformFiles(destDir string) error {
	return terraform.WriteVMFiles(terraform.ProviderGCP, destDir)
}

func (p *GCPProvisioner) runTerraform(ctx context.Context, workDir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "terraform", args...)
	cmd.Dir = workDir
	p.setCredentialEnv(cmd)

	var stderr bytes.Buffer
	if p.verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
	} else {
		cmd.Stderr = &stderr
	}

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("%w: %s", err, stderr.String())
		}
		return err
	}
	return nil
}

func (p *GCPProvisioner) getTerraformOutput(ctx context.Context, workDir string) (map[string]string, error) {
	cmd := exec.CommandContext(ctx, "terraform", "output", "-json")
	cmd.Dir = workDir
	p.setCredentialEnv(cmd)

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
