# Langfuse Observability Setup

This guide covers setting up Langfuse tracing for Agentium sessions. When enabled, every session produces structured traces in Langfuse showing the full Worker/Reviewer/Judge lifecycle with token metrics.

## What You Get

Langfuse traces follow the Agentium phase loop hierarchy:

```
Task (Trace)
  └── Phase (Span): PLAN, IMPLEMENT, DOCS, VERIFY
        ├── Worker (Generation) - LLM invocation with token counts
        ├── Reviewer (Generation or Event if skipped)
        └── Judge (Generation or Event if skipped)
```

Each generation records input/output token counts, model name, and execution status. Skipped components (e.g., reviewer skipped due to `reviewer_skip=true`) appear as events with the skip reason.

## Prerequisites

- An Agentium installation (see [Getting Started](getting-started.md))
- A [Langfuse Cloud](https://cloud.langfuse.com) account (or self-hosted Langfuse instance)

## Setup Steps

### 1. Create a Langfuse Account

Sign up at [cloud.langfuse.com](https://cloud.langfuse.com) and create a new project for your Agentium deployment.

### 2. Get Your API Keys

1. In the Langfuse dashboard, navigate to **Settings** > **API Keys**
2. Click **Create API Key**
3. Copy the **Public Key** and **Secret Key**

> **Note:** The public key starts with `pk-lf-` and the secret key starts with `sk-lf-`. Keep the secret key secure.

### 3a. Set Environment Variables (Local Dev)

For local development or testing:

```bash
export LANGFUSE_PUBLIC_KEY="pk-lf-your-public-key"
export LANGFUSE_SECRET_KEY="sk-lf-your-secret-key"
```

For self-hosted Langfuse instances, also set the base URL:

```bash
export LANGFUSE_BASE_URL="https://langfuse.your-company.com"
```

> **Note:** Environment variables set on your local machine are **not** available on GCP VMs. For VM sessions, use GCP Secret Manager (see below).

### 3b. Configure GCP Secret Manager (VM Sessions)

For production VM sessions, store Langfuse keys in GCP Secret Manager so the controller can fetch them at startup.

1. **Create the secrets** (one-time setup):

```bash
echo -n "pk-lf-your-public-key" | gcloud secrets create langfuse-public-key \
  --project=YOUR_GCP_PROJECT --data-file=-

echo -n "sk-lf-your-secret-key" | gcloud secrets create langfuse-secret-key \
  --project=YOUR_GCP_PROJECT --data-file=-
```

or, if updating a pre-existing secret use `versions add` in place of `create`

2. **Grant access** to the VM service account (if not already granted):

```bash
gcloud secrets add-iam-policy-binding langfuse-public-key \
  --project=YOUR_GCP_PROJECT \
  --member="serviceAccount:YOUR_SA@YOUR_GCP_PROJECT.iam.gserviceaccount.com" \
  --role="roles/secretmanager.secretAccessor"

gcloud secrets add-iam-policy-binding langfuse-secret-key \
  --project=YOUR_GCP_PROJECT \
  --member="serviceAccount:YOUR_SA@YOUR_GCP_PROJECT.iam.gserviceaccount.com" \
  --role="roles/secretmanager.secretAccessor"
```

> **Tip:** If your service account already has `roles/secretmanager.secretAccessor` at the project level (e.g., for `github-app-key`), no additional IAM bindings are needed.

3. **Add the paths to `.agentium.yaml`**:

```yaml
langfuse:
  public_key_secret: "projects/YOUR_GCP_PROJECT/secrets/langfuse-public-key/versions/latest"
  secret_key_secret: "projects/YOUR_GCP_PROJECT/secrets/langfuse-secret-key/versions/latest"
```

The controller will fetch these secrets at startup and initialize the Langfuse tracer automatically.

### 4. Run a Session

Run an Agentium session as usual. Traces are sent automatically:

```bash
agentium run --issue 42
```

### 5. View Traces in Langfuse

1. Open your Langfuse project dashboard
2. Navigate to **Traces**
3. Each task appears as a trace named after its task ID (e.g., `issue:42`)
4. Click a trace to see the phase spans and W/R/J generations
5. Token usage is visible on each generation and aggregated at the trace level

## Configuration Reference

| Environment Variable | Required | Default | Description |
|---------------------|----------|---------|-------------|
| `LANGFUSE_PUBLIC_KEY` | Yes | - | Langfuse public API key (`pk-lf-...`) |
| `LANGFUSE_SECRET_KEY` | Yes | - | Langfuse secret API key (`sk-lf-...`) |
| `LANGFUSE_BASE_URL` | No | `https://cloud.langfuse.com` | Langfuse API base URL (for self-hosted) |
| `LANGFUSE_ENABLED` | No | `true` (when keys are set) | Set to `false` to disable tracing even with keys present |

Tracing is enabled automatically when both `LANGFUSE_PUBLIC_KEY` and `LANGFUSE_SECRET_KEY` are set. To explicitly disable it (e.g., for debugging), set `LANGFUSE_ENABLED=false`.

## How It Works

### Go Controller (Production VMs)

The Go controller uses the Langfuse REST ingestion API directly. Events are buffered in a background goroutine and flushed every 5 seconds or on shutdown. No additional Go dependencies are required.

- **Initialization**: The controller reads env vars at startup and creates a `LangfuseTracer` (or `NoOpTracer` if disabled)
- **Instrumentation**: `runPhaseLoop()` records trace/span/generation events at each step
- **Shutdown**: The tracer is registered as a shutdown hook, ensuring all buffered events are flushed before the VM terminates

### TypeScript API

The TypeScript API uses the official `langfuse` npm package, wrapped in an adapter that matches the existing `LangfuseClient` interface.

- **Initialization**: `createTracerFromEnv()` reads the same env vars
- **Instrumentation**: `SessionController` manages trace lifecycle, `PhaseExecutor` records generations per step
- **Flush**: `tracer.flush()` is called after session completion

## Interpreting Traces

### Trace Metadata

Each trace includes:

| Field | Description |
|-------|-------------|
| `repository` | GitHub repository (e.g., `org/repo`) |
| `session_id` | Agentium session ID |
| `workflow` | Workflow name (e.g., `phase_loop`) |
| `status` | Final status: `COMPLETE`, `BLOCKED`, `NOTHING_TO_DO` |
| `total_input_tokens` | Aggregate input tokens across all phases |
| `total_output_tokens` | Aggregate output tokens across all phases |

### Phase Span Metadata

Each phase span includes:

| Field | Description |
|-------|-------------|
| `iteration` | Current iteration number |
| `max_iterations` | Maximum allowed iterations for this phase |
| `status` | `completed`, `exhausted`, or `blocked` |
| `duration_ms` | Phase wall-clock duration in milliseconds |

### Generation Fields

Each W/R/J generation includes:

| Field | Description |
|-------|-------------|
| `name` | `Worker`, `Reviewer`, or `Judge` |
| `model` | Agent adapter name |
| `usage.input` | Input token count |
| `usage.output` | Output token count |

### Skip Events

When a Reviewer or Judge is skipped, an event is recorded:

| Field | Description |
|-------|-------------|
| `name` | `Reviewer Skipped` or `Judge Skipped` |
| `skip_reason` | Reason string (e.g., `reviewer_skip=true`, `empty_output`) |

## Troubleshooting

### No traces appearing

1. Verify env vars are set: `echo $LANGFUSE_PUBLIC_KEY` (local dev only)
2. Check controller logs for `Langfuse: tracer initialized` message
3. If logs show `Langfuse: not configured`, add the `langfuse:` section to `.agentium.yaml` with Secret Manager paths (see [3b](#3b-configure-gcp-secret-manager-vm-sessions))
4. Ensure `LANGFUSE_ENABLED` is not set to `false`
5. Verify network access to `cloud.langfuse.com` from the VM

> **Common pitfall:** Local environment variables (`LANGFUSE_PUBLIC_KEY`, `LANGFUSE_SECRET_KEY`) are not available on GCP VMs. For VM sessions, you must configure Secret Manager paths in `.agentium.yaml`.

### Traces appear but missing generations

- Check if reviewer/judge are configured with `skip=true` — skipped components appear as events, not generations
- Verify the phase loop is enabled (`phase_loop.enabled: true`)

### Authentication errors in logs

- Verify the public key starts with `pk-lf-` and secret key starts with `sk-lf-`
- Check that the keys belong to the correct Langfuse project
- For self-hosted instances, verify `LANGFUSE_BASE_URL` is correct

## Next Steps

- [Configuration Reference](configuration.md#observability) — Full env var reference
- [Deployment Guide](cloud-setup/gcp.md#langfuse-integration) — Setting up keys on GCP VMs
- [Workflow Reference](WORKFLOW.md) — Understanding the phase loop that generates traces
