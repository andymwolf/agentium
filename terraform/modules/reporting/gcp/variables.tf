variable "project_id" {
  description = "GCP project ID where Cloud Logging and BigQuery resources are created."
  type        = string
}

variable "dataset_id" {
  description = "BigQuery dataset ID for token usage reporting."
  type        = string
  default     = "agentium_reporting"
}

variable "location" {
  description = "BigQuery dataset location."
  type        = string
  default     = "US"
}

variable "log_name" {
  description = "Cloud Logging log name used by agentium sessions."
  type        = string
  default     = "agentium-session"
}

variable "default_table_expiration_days" {
  description = "Default expiration in days for tables in the dataset."
  type        = number
  default     = 90
}

variable "sink_name" {
  description = "Name of the Cloud Logging sink that routes token usage logs to BigQuery."
  type        = string
  default     = "agentium-token-usage-sink"
}

variable "labels" {
  description = "Labels to apply to the BigQuery dataset."
  type        = map(string)
  default = {
    managed_by = "terraform"
    purpose    = "agentium-reporting"
  }
}
