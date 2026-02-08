locals {
  # Cloud Logging converts hyphens to underscores in auto-created BigQuery table names.
  bigquery_table_name = replace(var.log_name, "-", "_")
}

# -----------------------------------------------------------------------------
# BigQuery Dataset
# -----------------------------------------------------------------------------

resource "google_bigquery_dataset" "token_usage" {
  project    = var.project_id
  dataset_id = var.dataset_id
  location   = var.location
  labels     = var.labels

  default_table_expiration_ms = var.default_table_expiration_days * 24 * 60 * 60 * 1000
}

# -----------------------------------------------------------------------------
# Cloud Logging Sink → BigQuery
# -----------------------------------------------------------------------------

resource "google_logging_project_sink" "token_usage" {
  project = var.project_id
  name    = var.sink_name

  destination = "bigquery.googleapis.com/projects/${var.project_id}/datasets/${google_bigquery_dataset.token_usage.dataset_id}"

  filter = "logName = \"projects/${var.project_id}/logs/${var.log_name}\" AND labels.log_type = \"token_usage\""

  unique_writer_identity = true

  bigquery_options {
    use_partitioned_tables = true
  }
}

# Grant the sink's writer identity permission to write to the dataset.
resource "google_bigquery_dataset_iam_member" "sink_writer" {
  project    = var.project_id
  dataset_id = google_bigquery_dataset.token_usage.dataset_id
  role       = "roles/bigquery.dataEditor"
  member     = google_logging_project_sink.token_usage.writer_identity
}

# -----------------------------------------------------------------------------
# BigQuery Base Table (placeholder for Cloud Logging)
# -----------------------------------------------------------------------------

# Cloud Logging auto-creates this table on the first log entry, but views
# cannot be created until the table exists. This placeholder uses the minimum
# schema needed by our views. Cloud Logging will extend it with additional
# columns (severity, insertId, resource, etc.) on first write.
resource "google_bigquery_table" "log_entries" {
  project             = var.project_id
  dataset_id          = google_bigquery_dataset.token_usage.dataset_id
  table_id            = local.bigquery_table_name
  deletion_protection = false

  time_partitioning {
    type  = "DAY"
    field = "timestamp"
  }

  schema = jsonencode([
    {
      name = "timestamp"
      type = "TIMESTAMP"
      mode = "NULLABLE"
    },
    {
      name = "severity"
      type = "STRING"
      mode = "NULLABLE"
    },
    {
      name = "textPayload"
      type = "STRING"
      mode = "NULLABLE"
    },
    {
      name = "logName"
      type = "STRING"
      mode = "NULLABLE"
    },
    {
      name = "labels"
      type = "RECORD"
      mode = "REPEATED"
      fields = [
        {
          name = "key"
          type = "STRING"
          mode = "NULLABLE"
        },
        {
          name = "value"
          type = "STRING"
          mode = "NULLABLE"
        }
      ]
    }
  ])

  depends_on = [google_bigquery_dataset.token_usage]
}

# -----------------------------------------------------------------------------
# BigQuery Views
# -----------------------------------------------------------------------------

# Base view: flattens the REPEATED RECORD labels column into named columns.
resource "google_bigquery_table" "token_usage_flat" {
  project             = var.project_id
  dataset_id          = google_bigquery_dataset.token_usage.dataset_id
  table_id            = "token_usage_flat"
  deletion_protection = false

  view {
    query = <<-SQL
      SELECT
        timestamp,
        (SELECT value FROM UNNEST(labels) WHERE key = 'task_id')       AS task_id,
        (SELECT value FROM UNNEST(labels) WHERE key = 'phase')         AS phase,
        (SELECT value FROM UNNEST(labels) WHERE key = 'agent')         AS agent,
        (SELECT value FROM UNNEST(labels) WHERE key = 'model')         AS model,
        (SELECT value FROM UNNEST(labels) WHERE key = 'session_id')    AS session_id,
        (SELECT value FROM UNNEST(labels) WHERE key = 'repository')    AS repository,
        SAFE_CAST((SELECT value FROM UNNEST(labels) WHERE key = 'input_tokens')  AS INT64) AS input_tokens,
        SAFE_CAST((SELECT value FROM UNNEST(labels) WHERE key = 'output_tokens') AS INT64) AS output_tokens,
        SAFE_CAST((SELECT value FROM UNNEST(labels) WHERE key = 'total_tokens')  AS INT64) AS total_tokens
      FROM
        `${var.project_id}.${var.dataset_id}.${local.bigquery_table_name}`
    SQL
    use_legacy_sql = false
  }

  depends_on = [google_bigquery_table.log_entries]
}

# Usage by issue (task_id): total tokens per issue across all phases.
resource "google_bigquery_table" "usage_by_issue" {
  project             = var.project_id
  dataset_id          = google_bigquery_dataset.token_usage.dataset_id
  table_id            = "usage_by_issue"
  deletion_protection = false

  view {
    query = <<-SQL
      SELECT
        task_id,
        COUNT(*)            AS iterations,
        SUM(input_tokens)   AS total_input_tokens,
        SUM(output_tokens)  AS total_output_tokens,
        SUM(total_tokens)   AS total_tokens,
        MIN(timestamp)      AS first_seen,
        MAX(timestamp)      AS last_seen
      FROM
        `${var.project_id}.${var.dataset_id}.token_usage_flat`
      GROUP BY
        task_id
    SQL
    use_legacy_sql = false
  }

  depends_on = [google_bigquery_table.token_usage_flat]
}

# Usage by phase: token breakdown by phase and agent with statistics.
resource "google_bigquery_table" "usage_by_phase" {
  project             = var.project_id
  dataset_id          = google_bigquery_dataset.token_usage.dataset_id
  table_id            = "usage_by_phase"
  deletion_protection = false

  view {
    query = <<-SQL
      SELECT
        phase,
        agent,
        COUNT(*)                                                         AS iterations,
        SUM(total_tokens)                                                AS total_tokens,
        AVG(total_tokens)                                                AS avg_tokens,
        APPROX_QUANTILES(total_tokens, 100)[OFFSET(50)]                 AS median_tokens,
        APPROX_QUANTILES(total_tokens, 100)[OFFSET(95)]                 AS p95_tokens,
        SUM(input_tokens)                                                AS total_input_tokens,
        SUM(output_tokens)                                               AS total_output_tokens
      FROM
        `${var.project_id}.${var.dataset_id}.token_usage_flat`
      GROUP BY
        phase, agent
    SQL
    use_legacy_sql = false
  }

  depends_on = [google_bigquery_table.token_usage_flat]
}

# Usage by worker (agent adapter).
resource "google_bigquery_table" "usage_by_worker" {
  project             = var.project_id
  dataset_id          = google_bigquery_dataset.token_usage.dataset_id
  table_id            = "usage_by_worker"
  deletion_protection = false

  view {
    query = <<-SQL
      SELECT
        agent,
        COUNT(*)            AS iterations,
        SUM(input_tokens)   AS total_input_tokens,
        SUM(output_tokens)  AS total_output_tokens,
        SUM(total_tokens)   AS total_tokens,
        AVG(total_tokens)   AS avg_tokens_per_iteration
      FROM
        `${var.project_id}.${var.dataset_id}.token_usage_flat`
      GROUP BY
        agent
    SQL
    use_legacy_sql = false
  }

  depends_on = [google_bigquery_table.token_usage_flat]
}

# Usage by model.
resource "google_bigquery_table" "usage_by_model" {
  project             = var.project_id
  dataset_id          = google_bigquery_dataset.token_usage.dataset_id
  table_id            = "usage_by_model"
  deletion_protection = false

  view {
    query = <<-SQL
      SELECT
        model,
        COUNT(*)            AS iterations,
        SUM(input_tokens)   AS total_input_tokens,
        SUM(output_tokens)  AS total_output_tokens,
        SUM(total_tokens)   AS total_tokens,
        AVG(total_tokens)   AS avg_tokens_per_iteration
      FROM
        `${var.project_id}.${var.dataset_id}.token_usage_flat`
      GROUP BY
        model
    SQL
    use_legacy_sql = false
  }

  depends_on = [google_bigquery_table.token_usage_flat]
}

# Combined pivot view: full breakdown by task, phase, agent, and model.
resource "google_bigquery_table" "usage_combined" {
  project             = var.project_id
  dataset_id          = google_bigquery_dataset.token_usage.dataset_id
  table_id            = "usage_combined"
  deletion_protection = false

  view {
    query = <<-SQL
      SELECT
        task_id,
        phase,
        agent,
        model,
        session_id,
        repository,
        COUNT(*)            AS iterations,
        SUM(input_tokens)   AS total_input_tokens,
        SUM(output_tokens)  AS total_output_tokens,
        SUM(total_tokens)   AS total_tokens,
        MIN(timestamp)      AS first_seen,
        MAX(timestamp)      AS last_seen
      FROM
        `${var.project_id}.${var.dataset_id}.token_usage_flat`
      GROUP BY
        task_id, phase, agent, model, session_id, repository
    SQL
    use_legacy_sql = false
  }

  depends_on = [google_bigquery_table.token_usage_flat]
}
