# Agentium Reporting Module (GCP)

Terraform module that creates a Cloud Logging sink and BigQuery dataset for querying agentium token consumption across sessions.

## Overview

Agentium sessions log token usage to GCP Cloud Logging. This module routes those log entries into BigQuery and creates views for reporting on token consumption by issue, phase, agent, and model.

**This is one-time project infrastructure.** Apply it once per GCP project — it persists across all agentium sessions.

## Usage

```hcl
module "reporting" {
  source     = "./terraform/modules/reporting/gcp"
  project_id = "my-gcp-project"
}
```

```bash
terraform init
terraform apply -var="project_id=my-gcp-project"
```

## Resources Created

| Resource | Description |
|----------|-------------|
| `google_bigquery_dataset` | Dataset for token usage data (default: `agentium_reporting`) |
| `google_logging_project_sink` | Routes `token_usage` log entries to BigQuery |
| `google_bigquery_dataset_iam_member` | Grants the sink write access to the dataset |
| 6x `google_bigquery_table` (views) | Reporting views (see below) |

## Views

| View | Groups By | Purpose |
|------|-----------|---------|
| `token_usage_flat` | (base view) | Flattens Cloud Logging labels into named columns |
| `usage_by_issue` | `task_id` | Total tokens per issue across all phases |
| `usage_by_phase` | `phase`, `agent` | Token breakdown by phase with avg/median/p95 stats |
| `usage_by_worker` | `agent` | Token usage per agent adapter |
| `usage_by_model` | `model` | Token usage per model |
| `usage_combined` | `task_id` x `phase` x `agent` x `model` | Full pivot view |

## Example Queries

```sql
-- Top 10 most expensive issues
SELECT task_id, total_tokens, iterations
FROM `my-project.agentium_reporting.usage_by_issue`
ORDER BY total_tokens DESC
LIMIT 10;

-- Token usage by phase with p95
SELECT phase, agent, iterations, total_tokens, avg_tokens, p95_tokens
FROM `my-project.agentium_reporting.usage_by_phase`
ORDER BY total_tokens DESC;

-- Daily token spend
SELECT
  DATE(first_seen) AS day,
  SUM(total_tokens) AS daily_tokens,
  SUM(iterations) AS daily_iterations
FROM `my-project.agentium_reporting.usage_combined`
GROUP BY day
ORDER BY day DESC;
```

## Variables

| Name | Description | Default |
|------|-------------|---------|
| `project_id` | GCP project ID | (required) |
| `dataset_id` | BigQuery dataset ID | `agentium_reporting` |
| `location` | BigQuery dataset location | `US` |
| `log_name` | Cloud Logging log name | `agentium-session` |
| `default_table_expiration_days` | Table expiration in days | `90` |
| `sink_name` | Cloud Logging sink name | `agentium-token-usage-sink` |
| `labels` | Labels for the BigQuery dataset | `{managed_by="terraform", purpose="agentium-reporting"}` |

## Notes

- The BigQuery table (`agentium_session`) is pre-created by Terraform so that views can reference it immediately. Cloud Logging extends the schema with additional columns (`insertId`, `resource`, `receiveTimestamp`, etc.) on the first matching log entry.
- Cloud Logging exports labels as a `NULLABLE RECORD` where each label key becomes a named column (e.g., `labels.session_id`). Views reference these fields directly via dot-notation.
- Token counts are stored as strings in labels and converted via `SAFE_CAST` to `INT64`.
