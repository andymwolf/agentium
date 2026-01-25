# Create a custom IAM role with minimal permissions for VM self-deletion
resource "google_project_iam_custom_role" "agentium_vm_self_delete" {
  role_id     = "agentiumVMSelfDelete"
  title       = "Agentium VM Self Delete"
  description = "Minimal permissions for Agentium VMs to delete themselves"
  project     = var.project_id

  permissions = [
    # Minimal permissions needed for self-deletion
    "compute.instances.delete",
    "compute.instances.get",
    "compute.zones.get",
    "compute.zones.list"
  ]
}