variable "project_id" {
  description = "GCP project ID where the image will be created"
  type        = string
}

variable "zone" {
  description = "GCP zone for the build VM"
  type        = string
  default     = "us-central1-a"
}

variable "machine_type" {
  description = "Machine type for the build VM"
  type        = string
  default     = "e2-medium"
}

variable "disk_size_gb" {
  description = "Disk size for the build VM in GB"
  type        = number
  default     = 50
}

variable "image_version" {
  description = "Version tag for the image (used in naming and labels)"
  type        = string
  default     = "v1"
}
