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
    type = "DAY"
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
      name = "jsonPayload"
      type = "RECORD"
      mode = "NULLABLE"
      fields = [
        { name = "timestamp", type = "STRING", mode = "NULLABLE" },
        { name = "severity", type = "STRING", mode = "NULLABLE" },
        { name = "message", type = "STRING", mode = "NULLABLE" },
        { name = "session_id", type = "STRING", mode = "NULLABLE" },
        { name = "iteration", type = "INTEGER", mode = "NULLABLE" },
        {
          name = "labels"
          type = "RECORD"
          mode = "NULLABLE"
          fields = [
            { name = "session_id", type = "STRING", mode = "NULLABLE" },
            { name = "repository", type = "STRING", mode = "NULLABLE" },
            { name = "log_type", type = "STRING", mode = "NULLABLE" },
            { name = "task_id", type = "STRING", mode = "NULLABLE" },
            { name = "phase", type = "STRING", mode = "NULLABLE" },
            { name = "agent", type = "STRING", mode = "NULLABLE" },
            { name = "model", type = "STRING", mode = "NULLABLE" },
            { name = "input_tokens", type = "STRING", mode = "NULLABLE" },
            { name = "output_tokens", type = "STRING", mode = "NULLABLE" },
            { name = "total_tokens", type = "STRING", mode = "NULLABLE" }
          ]
        }
      ]
    },
    {
      name = "logName"
      type = "STRING"
      mode = "NULLABLE"
    },
    {
      name = "labels"
      type = "RECORD"
      mode = "NULLABLE"
      fields = [
        { name = "session_id", type = "STRING", mode = "NULLABLE" },
        { name = "repository", type = "STRING", mode = "NULLABLE" },
        { name = "log_type", type = "STRING", mode = "NULLABLE" },
        { name = "task_id", type = "STRING", mode = "NULLABLE" },
        { name = "phase", type = "STRING", mode = "NULLABLE" },
        { name = "agent", type = "STRING", mode = "NULLABLE" },
        { name = "model", type = "STRING", mode = "NULLABLE" },
        { name = "input_tokens", type = "STRING", mode = "NULLABLE" },
        { name = "output_tokens", type = "STRING", mode = "NULLABLE" },
        { name = "total_tokens", type = "STRING", mode = "NULLABLE" }
      ]
    }
  ])

  depends_on = [google_bigquery_dataset.token_usage]
}

# -----------------------------------------------------------------------------
# BigQuery Views
# -----------------------------------------------------------------------------

# Base view: extracts named label fields into top-level columns.
resource "google_bigquery_table" "token_usage_flat" {
  project             = var.project_id
  dataset_id          = google_bigquery_dataset.token_usage.dataset_id
  table_id            = "token_usage_flat"
  deletion_protection = false

  view {
    query = <<-SQL
      SELECT
        timestamp,
        labels.task_id                              AS task_id,
        labels.phase                                AS phase,
        labels.agent                                AS agent,
        labels.model                                AS model,
        labels.session_id                           AS session_id,
        labels.repository                           AS repository,
        SAFE_CAST(labels.input_tokens  AS INT64)    AS input_tokens,
        SAFE_CAST(labels.output_tokens AS INT64)    AS output_tokens,
        SAFE_CAST(labels.total_tokens  AS INT64)    AS total_tokens
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
