# Agentium Bootstrap Terraform Variables

# =============================================================================
# Required Variables
# =============================================================================

variable "project_id" {
  description = "GCP project ID"
  type        = string
}

variable "repository" {
  description = "Target GitHub repository (owner/repo format)"
  type        = string
}

variable "issues" {
  description = "Comma-separated list of issue numbers to work on"
  type        = string
}

variable "github_app_id" {
  description = "GitHub App ID for authentication"
  type        = string
}

variable "github_installation_id" {
  description = "GitHub App installation ID"
  type        = string
}

variable "github_private_key_secret" {
  description = "Name of the Secret Manager secret containing the GitHub App private key"
  type        = string
}

# =============================================================================
# Optional Variables
# =============================================================================

variable "region" {
  description = "GCP region"
  type        = string
  default     = "us-central1"
}

variable "zone" {
  description = "GCP zone"
  type        = string
  default     = "us-central1-a"
}

variable "machine_type" {
  description = "GCP machine type for the VM"
  type        = string
  default     = "e2-medium"
}

variable "boot_disk_size_gb" {
  description = "Size of the boot disk in GB"
  type        = number
  default     = 50
}

variable "session_id" {
  description = "Optional session ID (auto-generated if not provided)"
  type        = string
  default     = ""
}

variable "anthropic_api_key_secret" {
  description = "Optional: Name of the Secret Manager secret containing the Anthropic API key"
  type        = string
  default     = ""
}

variable "max_session_hours" {
  description = "Maximum session duration in hours before auto-cleanup"
  type        = number
  default     = 2
}

variable "enable_auto_cleanup" {
  description = "Enable automatic cleanup of the VM after max_session_hours"
  type        = bool
  default     = true
}
