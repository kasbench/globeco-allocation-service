# GlobeCo Allocation Service

The GlobeCo Allocation Service receives executed trades and generates input for the Portfolio Accounting Service. It is a Go microservice following clean architecture and domain-driven design principles, exposing a RESTful API for trade execution management and integration with downstream systems.

---

## Features
- Receive and store executed trades
- Batch creation of executions
- Send executions to Portfolio Accounting
- Health and readiness endpoints
- Structured logging, metrics, and tracing
- Containerized and Kubernetes-ready

---

## Architecture
- **Language:** Go 1.21+
- **Frameworks:** Chi (HTTP), sqlx (DB), zap (logging), viper (config)
- **Persistence:** PostgreSQL
- **Observability:** Prometheus, OpenTelemetry, zap
- **Project Structure:**
  - `cmd/` - Main entrypoint
  - `internal/` - Application code (domain, handler, service, repository, etc.)
  - `api/` - API definitions
  - `config/` - Configuration
  - `docs/`, `documentation/` - Documentation
  - `k8s/` - Kubernetes manifests
  - `monitoring/` - Prometheus/Grafana configs

---

## API Endpoints (v1)

| Method | Path                                | Description                                 |
|--------|-------------------------------------|---------------------------------------------|
| GET    | `/api/v1/executions`                | List executions (paginated)                 |
| GET    | `/api/v1/executions/{id}`           | Get execution by ID                         |
| POST   | `/api/v1/executions`                | Batch create executions                     |
| POST   | `/api/v1/executions/send`           | Send executions to Portfolio Accounting     |
| POST   | `/api/v1/executions/send/retry`     | Retry batch job execution for a file        |
| POST   | `/api/v1/executions/send/delete_last` | Delete last batch history record          |
| GET    | `/healthz`                          | Liveness probe                             |
| GET    | `/readyz`                           | Readiness probe                            |

See [`openapi.yaml`](openapi.yaml) for full schema and examples.

---

## Setup & Running Locally

### Prerequisites
- Go 1.21+
- Docker & Docker Compose
- PostgreSQL

### Quick Start
```sh
git clone <repo-url>
cd globeco-allocation-service
cp config/config.sample.yaml config/config.yaml
make build
make run
```

Or run with Docker Compose:
```sh
docker-compose up --build
```

### Configuration
- See `config/` and environment variables for all options.
- Main config file: `config.yaml` (can be overridden by env vars)

#### Kubernetes Batch Job Configuration
The service supports Kubernetes batch job execution mode for Portfolio Accounting CLI invocation. Configure the following settings:

```yaml
# Kubernetes batch job settings
kubernetes_batch_enabled: true          # Enable Kubernetes batch job mode
kubernetes_namespace: "globeco"          # Kubernetes namespace for jobs
cli_image: "globeco/portfolio-cli:latest" # CLI container image
job_timeout_seconds: 1800               # Job timeout (30 minutes)
job_retry_limit: 2                      # Number of retry attempts
service_account_name: "globeco-allocation-service" # Service account for jobs
```

Environment variable equivalents:
- `KUBERNETES_BATCH_ENABLED`
- `KUBERNETES_NAMESPACE`
- `CLI_IMAGE`
- `JOB_TIMEOUT_SECONDS`
- `JOB_RETRY_LIMIT`
- `SERVICE_ACCOUNT_NAME`

---

## Development
- Code in `internal/` follows clean architecture.
- Handlers in `internal/handler/`, business logic in `internal/service/`, DB in `internal/repository/`.
- Run tests:
  ```sh
  go test ./...
  ```
- Lint:
  ```sh
  golangci-lint run
  ```

---

## API Example

### Create Executions (Batch)
```http
POST /api/v1/executions
Content-Type: application/json

[
  {
    "executionServiceId": 123,
    "isOpen": false,
    "executionStatus": "FILLED",
    "tradeType": "BUY",
    "destination": "NYSE",
    "securityId": "12345678901234567890ABCD",
    "ticker": "AAPL",
    "quantity": 100.5,
    "limitPrice": 150.0,
    "receivedTimestamp": "2024-01-15T10:00:00Z",
    "sentTimestamp": "2024-01-15T10:01:00Z",
    "lastFillTimestamp": null,
    "quantityFilled": 100.5,
    "totalAmount": 15075.0,
    "averagePrice": 150.0
  }
]
```

**Response:**
```json
{
  "processedCount": 1,
  "skippedCount": 0,
  "errorCount": 0,
  "results": [
    {
      "executionServiceId": 123,
      "status": "created",
      "executionId": 1
    }
  ]
}
```

### List Executions
```http
GET /api/v1/executions?limit=50&offset=0
```
**Response:**
```json
{
  "executions": [
    {
      "id": 1,
      "executionServiceId": 123,
      "isOpen": false,
      "executionStatus": "FILLED",
      "tradeType": "BUY",
      "destination": "NYSE",
      "securityId": "12345678901234567890ABCD",
      "ticker": "AAPL",
      "quantity": 100.5,
      "limitPrice": 150.0,
      "receivedTimestamp": "2024-01-15T10:00:00Z",
      "sentTimestamp": "2024-01-15T10:01:00Z",
      "lastFillTimestamp": null,
      "quantityFilled": 100.5,
      "totalAmount": 15075.0,
      "averagePrice": 150.0,
      "version": 1
    }
  ],
  "pagination": {
    "totalElements": 1,
    "totalPages": 1,
    "currentPage": 0,
    "pageSize": 50,
    "hasNext": false,
    "hasPrevious": false
  }
}
```

### Send Executions to Portfolio Accounting
```http
POST /api/v1/executions/send
```
**Response (Kubernetes batch mode):**
```json
{
  "processed_count": 150,
  "filename": "executions_20240130_143022.csv",
  "status": "success",
  "message": "Batch job completed successfully",
  "job_name": "portfolio-cli-1706627422",
  "job_status": "succeeded",
  "execution_mode": "kubernetes"
}
```

**Response (Direct execution mode):**
```json
{
  "processed_count": 150,
  "filename": "executions_20240130_143022.csv",
  "status": "success",
  "message": "CLI execution completed successfully",
  "execution_mode": "direct"
}
```

### Retry Batch Job Execution
```http
POST /api/v1/executions/send/retry
Content-Type: application/json

{
  "filename": "executions_20240130_143022.csv"
}
```
**Response:**
```json
{
  "status": "success",
  "message": "Batch job completed successfully",
  "filename": "executions_20240130_143022.csv",
  "job_name": "portfolio-cli-retry-1706627422",
  "job_status": "succeeded",
  "execution_mode": "kubernetes"
}
```

**Error Response:**
```json
{
  "status": "error",
  "message": "File not found: executions_20240130_143022.csv"
}
```

### Delete Last Batch History
```http
POST /api/v1/executions/send/delete_last
```
**Response:**
```json
{
  "status": "success",
  "message": "Last batch history record deleted",
  "deleted_batch_id": 42
}
```

**Error Response:**
```json
{
  "status": "error",
  "message": "No batch history records found to delete"
}
```

### Health Check
```http
GET /healthz
```
**Response:**
```json
{
  "status": "ok",
  "timestamp": "2024-01-15T10:00:00Z"
}
```

---

## Observability
- **Logging:** Structured logs via zap
- **Metrics:** Prometheus endpoint (`/metrics`)
- **Tracing:** OpenTelemetry support

---

## Kubernetes RBAC Setup

When using Kubernetes batch job mode, the service requires proper RBAC configuration to create and monitor batch jobs.

### Required Permissions
The service needs the following Kubernetes permissions:
- Create, get, list, watch, and delete batch jobs
- Get, list, and watch pods
- Get pod logs

### RBAC Manifests
Apply the following manifests to set up proper permissions:

```bash
# Apply RBAC configuration
kubectl apply -f k8s/rbac.yaml
```

The RBAC configuration includes:

1. **ServiceAccount**: `globeco-allocation-service`
2. **Role**: `batch-job-manager` with required permissions
3. **RoleBinding**: Associates the service account with the role

### Manual RBAC Setup
If you need to create RBAC manually:

```yaml
# ServiceAccount
apiVersion: v1
kind: ServiceAccount
metadata:
  name: globeco-allocation-service
  namespace: globeco

---
# Role
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  namespace: globeco
  name: batch-job-manager
rules:
- apiGroups: ["batch"]
  resources: ["jobs"]
  verbs: ["create", "get", "list", "watch", "delete"]
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "list", "watch"]
- apiGroups: [""]
  resources: ["pods/log"]
  verbs: ["get"]

---
# RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: allocation-service-batch-jobs
  namespace: globeco
subjects:
- kind: ServiceAccount
  name: globeco-allocation-service
  namespace: globeco
roleRef:
  kind: Role
  name: batch-job-manager
  apiGroup: rbac.authorization.k8s.io
```

### Deployment Notes

#### Prerequisites for Kubernetes Batch Mode
1. **NFS Storage**: Ensure NFS PVC is available and accessible from both the service and batch job pods
2. **CLI Image**: Portfolio Accounting CLI container image must be available in the cluster
3. **Service Account**: RBAC configuration must be applied before deploying the service
4. **Configuration**: Enable Kubernetes batch mode in configuration

#### Deployment Steps
1. Apply RBAC configuration:
   ```bash
   kubectl apply -f k8s/rbac.yaml
   ```

2. Deploy NFS storage (if not already available):
   ```bash
   kubectl apply -f k8s/nfs-pv.yaml
   kubectl apply -f k8s/nfs-pvc.yaml
   ```

3. Deploy the service:
   ```bash
   kubectl apply -f k8s/deployment.yaml
   kubectl apply -f k8s/service-dev.yaml
   ```

4. Verify RBAC permissions:
   ```bash
   # Check if service account can create jobs
   kubectl auth can-i create jobs --as=system:serviceaccount:globeco:globeco-allocation-service -n globeco
   
   # Check if service account can get pods
   kubectl auth can-i get pods --as=system:serviceaccount:globeco:globeco-allocation-service -n globeco
   ```

#### Troubleshooting
- **Permission Denied**: Verify RBAC configuration is applied and service account is correctly referenced in deployment
- **Job Creation Fails**: Check resource quotas and limits in the namespace
- **NFS Mount Issues**: Verify NFS PVC is bound and accessible from job pods
- **CLI Image Pull Errors**: Ensure CLI container image is available and pull secrets are configured if needed

## Deployment
- Multi-stage Docker build (`Dockerfile`)
- Kubernetes manifests in `k8s/`
- See `docs/DEPLOYMENT.md` for details

---

## License
Apache 2.0

## API Documentation

- **Swagger UI:** Interactive docs available at [http://localhost:8089/swagger-ui/](http://localhost:8089/swagger-ui/)
- **OpenAPI Spec:** Download the OpenAPI YAML at [http://localhost:8089/openapi.yaml](http://localhost:8089/openapi.yaml)
