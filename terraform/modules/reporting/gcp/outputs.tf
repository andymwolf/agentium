output "dataset_id" {
  description = "The BigQuery dataset ID."
  value       = google_bigquery_dataset.token_usage.dataset_id
}

output "sink_name" {
  description = "The Cloud Logging sink name."
  value       = google_logging_project_sink.token_usage.name
}

output "sink_writer_identity" {
  description = "The service account identity used by the logging sink to write to BigQuery."
  value       = google_logging_project_sink.token_usage.writer_identity
}

output "view_token_usage_flat" {
  description = "Table ID of the token_usage_flat view."
  value       = google_bigquery_table.token_usage_flat.table_id
}

output "view_usage_by_issue" {
  description = "Table ID of the usage_by_issue view."
  value       = google_bigquery_table.usage_by_issue.table_id
}

output "view_usage_by_phase" {
  description = "Table ID of the usage_by_phase view."
  value       = google_bigquery_table.usage_by_phase.table_id
}

output "view_usage_by_worker" {
  description = "Table ID of the usage_by_worker view."
  value       = google_bigquery_table.usage_by_worker.table_id
}

output "view_usage_by_model" {
  description = "Table ID of the usage_by_model view."
  value       = google_bigquery_table.usage_by_model.table_id
}

output "view_usage_combined" {
  description = "Table ID of the usage_combined view."
  value       = google_bigquery_table.usage_combined.table_id
}
