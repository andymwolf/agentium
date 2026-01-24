variable "session_id" {
  description = "Unique session identifier"
  type        = string
}

variable "project_id" {
  description = "GCP project ID"
  type        = string
}

variable "region" {
  description = "GCP region"
  type        = string
  default     = "us-central1"
}

variable "zone" {
  description = "GCP zone (defaults to region-a)"
  type        = string
  default     = ""
}

variable "machine_type" {
  description = "GCP machine type"
  type        = string
  default     = "e2-medium"
}

variable "use_spot" {
  description = "Use spot/preemptible instances"
  type        = bool
  default     = true
}

variable "disk_size_gb" {
  description = "Boot disk size in GB"
  type        = number
  default     = 50
}

variable "controller_image" {
  description = "Docker image for the session controller"
  type        = string
  default     = "ghcr.io/andymwolf/agentium-controller:latest"
}

variable "session_config" {
  description = "Session configuration JSON"
  type        = string
}

variable "max_run_duration" {
  description = "Maximum VM run duration (e.g., 7200s for 2 hours)"
  type        = string
  default     = "7200s"
}

variable "vm_image" {
  description = "Custom VM image to use. If empty, uses Container-Optimized OS (cos-stable)."
  type        = string
  default     = ""
}

variable "claude_auth_mode" {
  description = "Claude authentication mode: api or oauth"
  type        = string
  default     = "api"
}

variable "claude_auth_json" {
  description = "Base64-encoded Claude auth.json content (for oauth mode)"
  type        = string
  default     = ""
  sensitive   = true
}

variable "codex_auth_json" {
  description = "Base64-encoded Codex auth.json content (for codex agent)"
  type        = string
  default     = ""
  sensitive   = true
}

variable "network" {
  description = "VPC network name"
  type        = string
  default     = "default"
}

variable "subnetwork" {
  description = "VPC subnetwork name"
  type        = string
  default     = ""
}

locals {
  zone = var.zone != "" ? var.zone : "${var.region}-a"
}

# Service account for the VM
resource "google_service_account" "agentium" {
  account_id   = "agentium-${substr(var.session_id, 0, 20)}"
  display_name = "Agentium Session ${var.session_id}"
  project      = var.project_id
}

# Grant secret accessor role
resource "google_project_iam_member" "secret_accessor" {
  project = var.project_id
  role    = "roles/secretmanager.secretAccessor"
  member  = "serviceAccount:${google_service_account.agentium.email}"
}

# Grant logging writer role
resource "google_project_iam_member" "logging_writer" {
  project = var.project_id
  role    = "roles/logging.logWriter"
  member  = "serviceAccount:${google_service_account.agentium.email}"
}

# Grant compute instance admin (for self-deletion)
resource "google_project_iam_member" "compute_admin" {
  project = var.project_id
  role    = "roles/compute.instanceAdmin.v1"
  member  = "serviceAccount:${google_service_account.agentium.email}"
}

# Cloud-init script
locals {
  claude_auth_volume = var.claude_auth_mode == "oauth" ? "-v /etc/agentium/claude-auth.json:/home/agentium/.claude/.credentials.json:ro" : ""
  codex_auth_volume  = var.codex_auth_json != "" ? "-v /etc/agentium/codex-auth.json:/home/agentium/.codex/auth.json:ro" : ""

  cloud_init = <<-EOF
#cloud-config
write_files:
  - path: /etc/agentium/session.json
    permissions: '0600'
    content: |
      ${var.session_config}
%{ if var.claude_auth_mode == "oauth" && var.claude_auth_json != "" ~}
  - path: /etc/agentium/claude-auth.json
    permissions: '0644'
    encoding: b64
    content: ${var.claude_auth_json}
%{ endif ~}
%{ if var.codex_auth_json != "" ~}
  - path: /etc/agentium/codex-auth.json
    permissions: '0600'
    encoding: b64
    content: ${var.codex_auth_json}
%{ endif ~}

runcmd:
  - |
    # Pull and run controller
    mkdir -p /home/workspace
    docker pull ${var.controller_image}
    docker run --rm \
      -v /var/run/docker.sock:/var/run/docker.sock \
      -v /etc/agentium:/etc/agentium:ro \
      -v /home/workspace:/home/workspace \
      ${local.claude_auth_volume} \
      ${local.codex_auth_volume} \
      -e AGENTIUM_CONFIG_PATH=/etc/agentium/session.json \
      -e AGENTIUM_AUTH_MODE=${var.claude_auth_mode} \
      -e AGENTIUM_WORKDIR=/home/workspace \
      --name agentium-controller \
      ${var.controller_image}
EOF
}

# Compute instance
resource "google_compute_instance" "agentium" {
  name         = var.session_id
  machine_type = var.machine_type
  zone         = local.zone
  project      = var.project_id

  boot_disk {
    initialize_params {
      image = var.vm_image != "" ? var.vm_image : "cos-cloud/cos-stable"
      size  = var.disk_size_gb
      type  = "pd-ssd"
    }
  }

  network_interface {
    network    = var.network
    subnetwork = var.subnetwork != "" ? var.subnetwork : null

    access_config {
      # Ephemeral public IP
    }
  }

  service_account {
    email  = google_service_account.agentium.email
    scopes = ["cloud-platform"]
  }

  metadata = {
    user-data           = local.cloud_init
    agentium-session-id = var.session_id
  }

  scheduling {
    preemptible                 = var.use_spot
    automatic_restart           = false
    on_host_maintenance         = var.use_spot ? "TERMINATE" : "MIGRATE"
    provisioning_model          = var.use_spot ? "SPOT" : "STANDARD"
    instance_termination_action = var.use_spot ? "DELETE" : null

    # Hard timeout at cloud level
    dynamic "max_run_duration" {
      for_each = var.max_run_duration != "" ? [1] : []
      content {
        seconds = tonumber(trimsuffix(var.max_run_duration, "s"))
      }
    }
  }

  labels = {
    agentium = "true"
    session  = var.session_id
  }

  tags = ["agentium", "allow-egress"]

  lifecycle {
    ignore_changes = [
      metadata["agentium-status"]
    ]
  }
}

# Firewall rule to allow outbound traffic
resource "google_compute_firewall" "agentium_egress" {
  name    = "agentium-allow-egress-${substr(var.session_id, 0, 20)}"
  network = var.network
  project = var.project_id

  direction = "EGRESS"

  allow {
    protocol = "tcp"
    ports    = ["443", "80", "22"]
  }

  target_tags = ["agentium"]
}

output "instance_id" {
  description = "The instance ID"
  value       = google_compute_instance.agentium.name
}

output "public_ip" {
  description = "The public IP address"
  value       = google_compute_instance.agentium.network_interface[0].access_config[0].nat_ip
}

output "zone" {
  description = "The zone where the instance is running"
  value       = google_compute_instance.agentium.zone
}

output "service_account" {
  description = "The service account email"
  value       = google_service_account.agentium.email
}
