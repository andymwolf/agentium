# Custom IAM role for VM self-deletion with least privilege
resource "google_project_iam_custom_role" "agentium_vm_self_delete" {
  role_id     = "agentiumVMSelfDelete"
  title       = "Agentium VM Self Delete"
  description = "Minimal permissions for a VM to delete itself and update its metadata"
  project     = var.project_id

  permissions = [
    # Required for self-deletion
    "compute.instances.delete",
    "compute.instances.get",

    # Required for metadata updates (status tracking)
    "compute.instances.setMetadata",

    # Required for cleanup operations
    "compute.disks.delete",  # Delete attached disks
    "compute.addresses.delete",  # Delete ephemeral IPs
  ]
}

# Create a more restrictive service account
resource "google_service_account" "agentium_restricted" {
  account_id   = "agentium-vm-${substr(var.session_id, 0, 20)}"
  display_name = "Agentium Session ${var.session_id} (Restricted)"
  project      = var.project_id
}

# Grant custom role with resource-level binding (self-only access)
resource "google_compute_instance_iam_member" "self_delete" {
  instance_name = google_compute_instance.agentium.name
  zone          = google_compute_instance.agentium.zone
  project       = var.project_id
  role          = google_project_iam_custom_role.agentium_vm_self_delete.id
  member        = "serviceAccount:${google_service_account.agentium_restricted.email}"
}

# Grant secret accessor with condition for specific secrets only
resource "google_project_iam_member" "secret_accessor_restricted" {
  project = var.project_id
  role    = "roles/secretmanager.secretAccessor"
  member  = "serviceAccount:${google_service_account.agentium_restricted.email}"

  # Only allow access to agentium-specific secrets
  condition {
    title       = "Only Agentium secrets"
    description = "Restrict access to secrets with agentium- prefix"
    expression  = "resource.name.startsWith(\"projects/${var.project_id}/secrets/agentium-\")"
  }
}

# Logging permissions remain unchanged
resource "google_project_iam_member" "logging_writer_restricted" {
  project = var.project_id
  role    = "roles/logging.logWriter"
  member  = "serviceAccount:${google_service_account.agentium_restricted.email}"
}

# Output the restricted service account for use in instance config
output "restricted_service_account" {
  description = "The restricted service account email"
  value       = google_service_account.agentium_restricted.email
}