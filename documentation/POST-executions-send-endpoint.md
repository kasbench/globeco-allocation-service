# POST /api/v1/executions/send Endpoint

This document describes the step-by-step behavior of the `POST /api/v1/executions/send` endpoint in the GlobeCo Allocation Service, with a focus on how it interacts with Kubernetes.

## Overview

The `/api/v1/executions/send` endpoint triggers a batch process that:
1. Collects closed executions that are ready to send
2. Generates a CSV file formatted for the Portfolio Accounting CLI
3. Invokes the Portfolio Accounting CLI to process the file, either via a **Kubernetes batch Job** (default) or via **direct CLI** invocation (legacy mode)

The execution mode is selected based on the `KUBERNETES_BATCH_ENABLED` configuration. When enabled, the service dynamically creates and submits a Kubernetes Job that runs the CLI container.

---

## Request / Response

**Request:** `POST /api/v1/executions/send` (no body required)

**Response (success):**
```json
{
  "processedCount": 42,
  "fileName": "transactions_20260722_143012.csv",
  "status": "success",
  "message": "Kubernetes batch job executed successfully",
  "jobName": "portfolio-cli-20260722-143012-1753192212",
  "jobStatus": "succeeded",
  "executionMode": "kubernetes"
}
```

**Error codes:**
- `409 Conflict` - A batch process is already in progress (duplicate batch detected)
- `500 Internal Server Error` - Processing failure

---

## Step-by-Step Flow (Kubernetes Mode)

### Step 1: Mode Selection

The handler (`internal/handler/execution.go`) calls `ExecutionService.Send()`. The service checks the configuration:

```
if config.KubernetesBatch.Enabled && kubernetesBatchInvoker != nil:
    → Kubernetes batch job mode
else:
    → Direct CLI mode (legacy)
```

### Step 2: Get Previous Batch Start Time

The service queries the `batch_history` table for `MAX(start_time)`. This timestamp defines the lower bound of which executions to include in this batch.

```sql
SELECT MAX(start_time) FROM batch_history
```

If no records exist, the zero time is used (effectively selecting all ready executions).

### Step 3: Create Batch History Record

A new `batch_history` record is inserted with:
- `start_time` = current UTC time
- `previous_start_time` = the value retrieved in Step 2
- `version` = 1

This record acts as a concurrency guard. If a duplicate insert is attempted (uniqueness constraint), the endpoint returns `409 Conflict`.

### Step 4: Query Executions for This Batch

Executions are retrieved from the `execution` table using the time window between the previous batch start time and the current time:

```sql
SELECT * FROM execution
WHERE ready_to_send_timestamp >= $previous_start_time
  AND ready_to_send_timestamp < $current_time
ORDER BY ready_to_send_timestamp ASC
```

If no executions are found, the endpoint returns immediately with `processedCount: 0`.

### Step 5: Generate Portfolio Accounting CSV File

The `FileGeneratorService` creates a CSV file at the configured output directory (default: `/data`):

- **Filename pattern:** `transactions_YYYYMMDD_HHMMSS.csv`
- **CSV header:** `portfolio_id,security_id,source_id,transaction_type,quantity,price,transaction_date`
- **Source ID format:** `AC{execution.ID}`
- **Trade date format:** `YYYYMMDD`

This file is written to a shared NFS volume accessible by both the allocation service pod and the batch Job pod.

### Step 6: Create and Submit Kubernetes Batch Job

This is where the core Kubernetes interaction happens. The `KubernetesBatchInvokerService` performs the following:

#### 6a. Generate Unique Job Name

```
portfolio-cli-YYYYMMDD-HHMMSS-{unix_timestamp}
```

Names are lowercased and truncated to 63 characters to conform to DNS-1123 subdomain naming rules.

#### 6b. Build the Job Manifest

A `batchv1.Job` object is constructed programmatically (not from a static YAML template). Key properties:

| Field | Value |
|-------|-------|
| Namespace | `globeco` (configurable) |
| Image | `kasbench/globeco-portfolio-accounting-service-cli:latest` |
| Service Account | `globeco-allocation-service` |
| Restart Policy | `Never` |
| Backoff Limit | `2` (configurable via `KUBERNETES_BATCH_JOB_RETRY_LIMIT`) |
| Active Deadline | `1800s` (30 min, configurable) |
| TTL After Finished | `3600s` (auto-cleanup after 1 hour) |
| Completions | `1` |
| Parallelism | `1` |

**Container configuration:**
- Command: `/usr/local/bin/cli`
- Args: `process --file /data/{filename} --output-dir /data --config /etc/config/config.yaml`
- Resource requests: 512Mi memory, 250m CPU
- Resource limits: 1Gi memory, 500m CPU
- Security context: non-root (UID 1000), read-only root filesystem, no privilege escalation

**Volumes:**
| Volume | Source | Mount Path | Purpose |
|--------|--------|------------|---------|
| `nfs-storage` | PVC `nfs-pvc` | `/data` | Shared file storage for the CSV input and CLI output |
| `cli-config` | ConfigMap `portfolio-cli-config` | `/etc/config` | CLI configuration file |

**Environment variables injected into the Job pod:**
- `GLOBECO_PA_SERVER_HOST` = `globeco-allocation-service`
- `GLOBECO_PA_SERVER_PORT` = `8089`
- `JOB_NAME` = the generated job name
- `INPUT_FILENAME` = the CSV filename

**Labels applied (for identification and querying):**
- `app: portfolio-accounting-cli`
- `component: batch-processor`
- `managed-by: globeco-allocation-service`
- `globeco.io/service: allocation-service`
- `globeco.io/job-type: portfolio-accounting`
- `globeco.io/created-by: kubernetes-batch-invoker`
- `globeco.io/filename: {sanitized_filename}`

#### 6c. Validate the Job Manifest

Before submission, the service validates:
- Job name and namespace are set
- At least one container is defined with an image
- NFS storage volume mount exists
- NFS PVC volume is configured

#### 6d. Submit the Job with Retry Logic

The Job is submitted to the Kubernetes API via the `client-go` library using **in-cluster configuration** (`rest.InClusterConfig()`). This means the allocation service must be running inside the same Kubernetes cluster.

Submission uses exponential backoff with jitter:
- Max retries: 5
- Base delay: 1s
- Max delay: 30s
- Backoff multiplier: 2.0
- Jitter: up to 25% of calculated delay

**Retryable errors** (will retry): connection refused, timeout, 429 Too Many Requests, 500/502/503/504 server errors.

**Non-retryable errors** (fail immediately): 400 Bad Request, 401 Unauthorized, 403 Forbidden, 404 Not Found, 409 Conflict, 422 Unprocessable Entity.

### Step 7: Monitor Job Completion

After submission, the service polls the Kubernetes API to watch for job completion:

- **Polling strategy:** Adaptive intervals starting at 2s, increasing by 1.5x up to 10s max
- **Timeout:** Matches the configured job timeout (default 30 minutes)
- **Failure detection:** Max 5 consecutive API errors before giving up

The service checks `job.Status.Conditions` for:
- `JobComplete` with status `True` → success
- `JobFailed` with status `True` → failure

It also inspects pod counts (`Active`, `Succeeded`, `Failed`) as a fallback.

On failure, the service retrieves pod details and container logs (last 50 lines) for debugging.

### Step 8: File Cleanup (Conditional)

After successful job completion:

1. If `FILE_CLEANUP_ENABLED` is `true`:
   - Validate job status one more time via `GetJobStatus()` to confirm the job truly succeeded
   - Delete the CSV file from the NFS volume
2. If cleanup is disabled, the file is retained for manual inspection or retry

On job failure, the file is **never** deleted (to allow retrying without regeneration).

---

## Kubernetes RBAC Requirements

The allocation service's service account (`globeco-allocation-service`) needs the following RBAC permissions in the `globeco` namespace:

| Resource | Verbs |
|----------|-------|
| `jobs` (batch/v1) | create, get, list, delete |
| `pods` | get, list |
| `pods/log` | get |
| `serviceaccounts` | get |
| `persistentvolumeclaims` | get |

At startup (when `KUBERNETES_BATCH_ENABLED=true`), the service validates access by:
1. Listing jobs (connectivity test)
2. Listing pods (pod access test)
3. Getting the configured service account
4. Getting the configured PVC
5. Performing a dry-run job creation (permission test)

---

## Shared Storage Architecture

The CSV file handoff between the allocation service and the CLI batch Job relies on a shared NFS PersistentVolumeClaim:

```
┌─────────────────────────┐         ┌────────────────────────────┐
│  Allocation Service Pod │         │  Portfolio CLI Job Pod      │
│                         │         │                            │
│  Writes CSV to /data    │───NFS───│  Reads CSV from /data      │
│  (nfs-pvc mounted)      │   PVC   │  (nfs-pvc mounted)         │
└─────────────────────────┘         └────────────────────────────┘
```

Both the allocation service and the batch Job mount the same PVC (`nfs-pvc`) at `/data`.

---

## Configuration Reference

All configuration is via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `KUBERNETES_BATCH_ENABLED` | `true` | Enable Kubernetes batch job mode |
| `KUBERNETES_BATCH_NAMESPACE` | `globeco` | Namespace for batch Jobs |
| `KUBERNETES_BATCH_CLI_IMAGE` | `kasbench/globeco-portfolio-accounting-service-cli:latest` | Container image for CLI |
| `KUBERNETES_BATCH_JOB_TIMEOUT_SECONDS` | `1800` | Max job runtime (seconds) |
| `KUBERNETES_BATCH_JOB_RETRY_LIMIT` | `2` | Kubernetes backoff limit for pod restarts |
| `KUBERNETES_BATCH_SERVICE_ACCOUNT_NAME` | `globeco-allocation-service` | SA for batch Job pods |
| `KUBERNETES_BATCH_NFS_PVC_NAME` | `nfs-pvc` | PVC name for shared NFS storage |
| `FILE_CLEANUP_ENABLED` | `false` | Delete CSV after successful job |
| `OUTPUT_DIR` | `/data` | Directory for generated CSV files |

---

## Sequence Diagram

```
Client              Handler              Service              K8s API              NFS Volume
  │                    │                    │                    │                    │
  │  POST /send        │                    │                    │                    │
  │───────────────────>│                    │                    │                    │
  │                    │  Send()            │                    │                    │
  │                    │───────────────────>│                    │                    │
  │                    │                    │                    │                    │
  │                    │                    │  GetMaxStartTime   │                    │
  │                    │                    │──── (DB query) ────│                    │
  │                    │                    │                    │                    │
  │                    │                    │  Create BatchHistory                    │
  │                    │                    │──── (DB insert) ───│                    │
  │                    │                    │                    │                    │
  │                    │                    │  GetForBatch       │                    │
  │                    │                    │──── (DB query) ────│                    │
  │                    │                    │                    │                    │
  │                    │                    │  Generate CSV       │                    │
  │                    │                    │───────────────────────────────────────>│
  │                    │                    │                    │    (write file)    │
  │                    │                    │                    │                    │
  │                    │                    │  Create Job        │                    │
  │                    │                    │───────────────────>│                    │
  │                    │                    │                    │                    │
  │                    │                    │  Poll Job Status   │                    │
  │                    │                    │───────────────────>│                    │
  │                    │                    │  (repeat until     │                    │
  │                    │                    │   complete/fail)   │                    │
  │                    │                    │                    │                    │
  │                    │                    │                    │  Job Pod runs CLI  │
  │                    │                    │                    │───────────────────>│
  │                    │                    │                    │   (reads CSV)      │
  │                    │                    │                    │                    │
  │                    │                    │  (Optional) Delete CSV                  │
  │                    │                    │───────────────────────────────────────>│
  │                    │                    │                    │                    │
  │                    │  SendResponse      │                    │                    │
  │                    │<───────────────────│                    │                    │
  │  200 OK + JSON     │                    │                    │                    │
  │<───────────────────│                    │                    │                    │
```

---

## Error Handling Summary

| Scenario | HTTP Code | Behavior |
|----------|-----------|----------|
| Duplicate batch in progress | 409 | Immediate rejection |
| No executions to process | 200 | Returns `processedCount: 0` |
| File generation fails | 500 | Error returned, no Job created |
| Job submission fails (non-retryable) | 500 | Immediate failure after error analysis |
| Job submission fails (retryable) | 500 | Retries up to 5 times with backoff |
| Job timeout | 500 | Fails after configured deadline |
| Job pod failure | 500 | CSV file retained for retry |
| File cleanup fails | 200 | Warning logged, success still returned |
