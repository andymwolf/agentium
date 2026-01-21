# Agentium Bootstrap Terraform Configuration
#
# Creates a GCP VM for running Claude Code agent sessions.
# Uses preemptible instances to minimize cost.

terraform {
  required_version = ">= 1.0"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.0"
    }
  }
}

provider "google" {
  project = var.project_id
  region  = var.region
  zone    = var.zone
}

# Generate unique session ID if not provided
resource "random_id" "session" {
  byte_length = 8
}

locals {
  session_id = var.session_id != "" ? var.session_id : "agentium-${random_id.session.hex}"
  vm_name    = "agentium-session-${random_id.session.hex}"
}

# Service account for the VM
resource "google_service_account" "agentium" {
  account_id   = "agentium-session-${random_id.session.hex}"
  display_name = "Agentium Session ${local.session_id}"
  description  = "Service account for Agentium agent session"
}

# Grant Secret Manager access to the service account
resource "google_secret_manager_secret_iam_member" "github_key" {
  secret_id = var.github_private_key_secret
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.agentium.email}"
}

# Grant Secret Manager access for Anthropic API key (if configured)
resource "google_secret_manager_secret_iam_member" "anthropic_key" {
  count     = var.anthropic_api_key_secret != "" ? 1 : 0
  secret_id = var.anthropic_api_key_secret
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.agentium.email}"
}

# Read cloud-init configuration
data "local_file" "cloud_init" {
  filename = "${path.module}/cloud-init.yaml"
}

# Compute instance
resource "google_compute_instance" "agentium" {
  name         = local.vm_name
  machine_type = var.machine_type
  zone         = var.zone

  # Use preemptible/spot instance for cost savings
  scheduling {
    preemptible                 = true
    automatic_restart           = false
    on_host_maintenance         = "TERMINATE"
    provisioning_model          = "SPOT"
    instance_termination_action = "DELETE"
  }

  # Boot disk with SSD
  boot_disk {
    initialize_params {
      image = "ubuntu-os-cloud/ubuntu-2204-lts"
      size  = var.boot_disk_size_gb
      type  = "pd-ssd"
    }
    auto_delete = true
  }

  # Network interface
  network_interface {
    network = "default"
    access_config {
      # Ephemeral external IP
    }
  }

  # Service account
  service_account {
    email  = google_service_account.agentium.email
    scopes = ["cloud-platform"]
  }

  # Instance metadata
  metadata = {
    # Cloud-init configuration
    user-data = data.local_file.cloud_init.content

    # Agentium session configuration
    agentium-autostart           = "true"
    agentium-session-id          = local.session_id
    agentium-repository          = var.repository
    agentium-issues              = var.issues
    github-app-id                = var.github_app_id
    github-installation-id       = var.github_installation_id
    github-private-key-secret    = var.github_private_key_secret
    anthropic-api-key-secret     = var.anthropic_api_key_secret
  }

  # Labels for tracking
  labels = {
    agentium     = "true"
    session-id   = local.session_id
    repository   = replace(replace(var.repository, "/", "-"), ".", "-")
  }

  # Allow the instance to be deleted by Terraform
  allow_stopping_for_update = true

  # Tags for firewall rules (if needed)
  tags = ["agentium-session"]

  depends_on = [
    google_secret_manager_secret_iam_member.github_key,
  ]
}

# Auto-delete the VM after max_session_hours
# Uses a Cloud Scheduler job to delete the instance
resource "google_cloud_scheduler_job" "cleanup" {
  count = var.enable_auto_cleanup ? 1 : 0

  name        = "agentium-cleanup-${random_id.session.hex}"
  description = "Auto-cleanup for Agentium session ${local.session_id}"
  schedule    = "0 */${var.max_session_hours} * * *"
  time_zone   = "UTC"

  http_target {
    http_method = "DELETE"
    uri         = "https://compute.googleapis.com/compute/v1/projects/${var.project_id}/zones/${var.zone}/instances/${local.vm_name}"
    oauth_token {
      service_account_email = google_service_account.agentium.email
    }
  }

  # Run once after the max session time
  retry_config {
    retry_count = 0
  }
}

# Outputs
output "session_id" {
  description = "The unique session ID"
  value       = local.session_id
}

output "instance_name" {
  description = "The name of the created VM instance"
  value       = google_compute_instance.agentium.name
}

output "instance_ip" {
  description = "The external IP address of the VM"
  value       = google_compute_instance.agentium.network_interface[0].access_config[0].nat_ip
}

output "instance_zone" {
  description = "The zone where the VM is running"
  value       = google_compute_instance.agentium.zone
}

output "ssh_command" {
  description = "SSH command to connect to the VM"
  value       = "gcloud compute ssh ${google_compute_instance.agentium.name} --zone=${google_compute_instance.agentium.zone}"
}

output "logs_command" {
  description = "Command to tail session logs"
  value       = "gcloud compute ssh ${google_compute_instance.agentium.name} --zone=${google_compute_instance.agentium.zone} --command='tail -f /var/log/agentium-session.log'"
}

output "destroy_command" {
  description = "Command to destroy this session"
  value       = "terraform destroy -auto-approve"
}
