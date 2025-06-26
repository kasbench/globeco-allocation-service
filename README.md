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

| Method | Path                        | Description                                 |
|--------|-----------------------------|---------------------------------------------|
| GET    | `/api/v1/executions`        | List executions (paginated)                 |
| GET    | `/api/v1/executions/{id}`   | Get execution by ID                         |
| POST   | `/api/v1/executions`        | Batch create executions                     |
| POST   | `/api/v1/executions/send`   | Send executions to Portfolio Accounting     |
| GET    | `/healthz`                  | Liveness probe                             |
| GET    | `/readyz`                   | Readiness probe                            |

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

## Deployment
- Multi-stage Docker build (`Dockerfile`)
- Kubernetes manifests in `k8s/`
- See `docs/DEPLOYMENT.md` for details

---

## License
Apache 2.0

## API Documentation

- **Swagger UI:** Interactive docs available at [http://localhost:8080/swagger-ui/](http://localhost:8089/swagger-ui/)
- **OpenAPI Spec:** Download the OpenAPI YAML at [http://localhost:8089/openapi.yaml](http://localhost:8089/openapi.yaml)
