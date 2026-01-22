# Agentium Bootstrap Terraform Variables

# =============================================================================
# Required Variables
# =============================================================================

variable "project_id" {
  description = "GCP project ID"
  type        = string
  default     = "agentium-485102"
}

variable "repository" {
  description = "Target GitHub repository (owner/repo format)"
  type        = string
  default     = ""

  validation {
    condition     = var.repository == "" || can(regex("^[a-zA-Z0-9_-]+/[a-zA-Z0-9_.-]+$", var.repository))
    error_message = "Repository must be in owner/repo format (e.g., 'andymwolf/agentium')."
  }
}

variable "issues" {
  description = "Comma-separated list of issue numbers to work on"
  type        = string
  default     = ""

  validation {
    condition     = var.issues == "" || can(regex("^[0-9]+(,[0-9]+)*$", var.issues))
    error_message = "Issues must be comma-separated numbers (e.g., '42' or '42,43,44')."
  }
}

variable "github_app_id" {
  description = "GitHub App ID for authentication"
  type        = string
  default     = ""
}

variable "github_installation_id" {
  description = "GitHub App installation ID"
  type        = string
  default     = ""
}

variable "github_private_key_secret" {
  description = "Name of the Secret Manager secret containing the GitHub App private key"
  type        = string
  default     = ""
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

variable "network" {
  description = "GCP network to use for the VM"
  type        = string
  default     = "default"
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
  description = "Maximum session duration in hours (VM auto-terminates after this)"
  type        = number
  default     = 2

  validation {
    condition     = var.max_session_hours >= 1 && var.max_session_hours <= 24
    error_message = "Max session hours must be between 1 and 24."
  }
}
