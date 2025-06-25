# GlobeCo Allocation Service Requirements

## Overview

The Allocation Service is a Go microservice that processes trade execution data and prepares portfolio accounting files. It serves as an intermediary between the Trade Service and the Portfolio Accounting Service within the GlobeCo benchmarking suite.

### Service Details
- **Name**: Allocation Service
- **Host**: globeco-allocation-service
- **Port**: 8089
- **Deployment**: Kubernetes 1.33
- **Author**: Noah Krieger (noah@kasbench.org)

## Technology Stack

| Component | Version | Notes |
|-----------|---------|-------|
| Go | 1.21+ | Latest stable version |
| PostgreSQL | 17 | Primary database |
| Chi Router | v5 | HTTP routing |
| sqlx | Latest | Database operations |
| Zap | Latest | Structured logging |
| Viper | Latest | Configuration management |
| Testify | Latest | Unit testing |

## Architecture

### Dependencies
| Service | Host | Port | Description | API Documentation |
|---------|------|------|-------------|-------------------|
| Portfolio Accounting Service CLI | globeco-portfolio-accounting-service-cli | N/A | CLI for portfolio processing | [CLI-Usage-Guide.md](CLI-Usage-Guide.md) |
| Trade Service | globeco-trade-service | 8082 | Trade execution data source | [trade-service-openapi.json](trade-service-openapi.json) |

### Database Configuration
- **Host**: globeco-allocation-service-postgresql:5432
- **Database**: postgres
- **Schema**: public
- **Owner**: postgres

## Data Model

### Database Schema

#### execution Table
| Column | Type | Constraints | Default | Description |
|--------|------|-------------|---------|-------------|
| id | serial | PRIMARY KEY, NOT NULL | | Auto-generated ID |
| execution_service_id | integer | NOT NULL, UNIQUE | | External execution service identifier |
| is_open | boolean | NOT NULL | true | Whether execution is still open |
| execution_status | varchar(20) | NOT NULL | | Status of execution (e.g., FILLED, PARTIAL) |
| trade_type | varchar(10) | NOT NULL | | BUY or SELL |
| destination | varchar(20) | NOT NULL | | Trading destination |
| trade_date | date | NOT NULL | | Date of trade |
| security_id | char(24) | NOT NULL | | Security identifier |
| ticker | varchar(20) | NOT NULL | | Security ticker symbol |
| portfolio_id | char(24) | | | Portfolio identifier |
| quantity | decimal(18,8) | NOT NULL | | Trade quantity |
| limit_price | decimal(18,8) | | | Limit price if applicable |
| received_timestamp | timestamptz | NOT NULL | | When execution was received |
| sent_timestamp | timestamptz | NOT NULL | | When execution was sent |
| last_fill_timestamp | timestamptz | | | Last fill timestamp |
| quantity_filled | decimal(18,8) | NOT NULL | 0 | Quantity filled |
| total_amount | decimal(18,8) | | 0 | Total trade amount |
| average_price | decimal(18,8) | NOT NULL | | Average execution price |
| ready_to_send_timestamp | timestamptz | | CURRENT_TIMESTAMP | When ready for portfolio accounting |
| version | integer | NOT NULL | 1 | Record version |

**Indexes:**
- `execution_execution_service_id_ndx` ON execution_service_id
- `execution_ready_to_send_timestamp_ndx` ON ready_to_send_timestamp

#### batch_history Table
| Column | Type | Constraints | Default | Description |
|--------|------|-------------|---------|-------------|
| id | serial | PRIMARY KEY, NOT NULL | | Auto-generated ID |
| start_time | timestamptz | NOT NULL, UNIQUE | CURRENT_TIMESTAMP | Batch start time |
| previous_start_time | timestamptz | NOT NULL, UNIQUE | | Previous batch start time |
| version | integer | NOT NULL | 1 | Record version |

**Indexes:**
- `batch_history_start_time_ndx` ON start_time (UNIQUE)
- `batch_history_previous_start_time_ndx` ON previous_start_time (UNIQUE)

### Data Transfer Objects

#### ExecutionDTO (Response)
```go
type ExecutionDTO struct {
    ID                   int       `json:"id"`
    ExecutionServiceID   int       `json:"executionServiceId"`
    IsOpen              bool      `json:"isOpen"`
    ExecutionStatus     string    `json:"executionStatus"`
    TradeType           string    `json:"tradeType"`
    Destination         string    `json:"destination"`
    SecurityID          string    `json:"securityId"`
    PortfolioID         *string   `json:"portfolioId"`
    Ticker              string    `json:"ticker"`
    Quantity            float64   `json:"quantity"`
    LimitPrice          *float64  `json:"limitPrice"`
    ReceivedTimestamp   time.Time `json:"receivedTimestamp"`
    SentTimestamp       time.Time `json:"sentTimestamp"`
    LastFillTimestamp   *time.Time `json:"lastFillTimestamp"`
    QuantityFilled      float64   `json:"quantityFilled"`
    TotalAmount         float64   `json:"totalAmount"`
    AveragePrice        float64   `json:"averagePrice"`
    Version             int       `json:"version"`
}
```

#### ExecutionPostDTO (Request)
```go
type ExecutionPostDTO struct {
    ExecutionServiceID   int       `json:"executionServiceId" validate:"required"`
    IsOpen              bool      `json:"isOpen"`
    ExecutionStatus     string    `json:"executionStatus" validate:"required"`
    TradeType           string    `json:"tradeType" validate:"required,oneof=BUY SELL"`
    Destination         string    `json:"destination" validate:"required"`
    SecurityID          string    `json:"securityId" validate:"required"`
    Ticker              string    `json:"ticker" validate:"required"`
    Quantity            float64   `json:"quantity" validate:"required,gt=0"`
    LimitPrice          *float64  `json:"limitPrice"`
    ReceivedTimestamp   time.Time `json:"receivedTimestamp" validate:"required"`
    SentTimestamp       time.Time `json:"sentTimestamp" validate:"required"`
    LastFillTimestamp   *time.Time `json:"lastFillTimestamp"`
    QuantityFilled      float64   `json:"quantityFilled" validate:"gte=0"`
    TotalAmount         float64   `json:"totalAmount" validate:"gte=0"`
    AveragePrice        float64   `json:"averagePrice" validate:"gt=0"`
}
```

## REST API Specification

### Endpoints

| Method | Path | Request Body | Response Body | Description |
|--------|------|--------------|---------------|-------------|
| GET | `/api/v1/executions` | - | `ExecutionListResponse` | List executions with pagination |
| GET | `/api/v1/executions/{id}` | - | `ExecutionDTO` | Get execution by ID |
| POST | `/api/v1/executions` | `[]ExecutionPostDTO` | `BatchCreateResponse` | Create executions (max 100) |
| POST | `/api/v1/executions/send` | - | `SendResponse` | Send executions to Portfolio Accounting |
| GET | `/healthz` | - | `HealthResponse` | Liveness probe |
| GET | `/readyz` | - | `ReadinessResponse` | Readiness probe |
| GET | `/metrics` | - | - | Prometheus metrics |

### Response Models

#### ExecutionListResponse
```go
type ExecutionListResponse struct {
    Executions []ExecutionDTO `json:"executions"`
    Pagination PaginationInfo `json:"pagination"`
}

type PaginationInfo struct {
    TotalElements int  `json:"totalElements"`
    TotalPages    int  `json:"totalPages"`
    CurrentPage   int  `json:"currentPage"`
    PageSize      int  `json:"pageSize"`
    HasNext       bool `json:"hasNext"`
    HasPrevious   bool `json:"hasPrevious"`
}
```

#### BatchCreateResponse
```go
type BatchCreateResponse struct {
    ProcessedCount int                    `json:"processedCount"`
    SkippedCount   int                    `json:"skippedCount"`
    ErrorCount     int                    `json:"errorCount"`
    Results        []ExecutionResult      `json:"results"`
}

type ExecutionResult struct {
    ExecutionServiceID int    `json:"executionServiceId"`
    Status            string `json:"status"` // "created", "skipped", "error"
    Error             string `json:"error,omitempty"`
    ExecutionID       *int   `json:"executionId,omitempty"`
}
```

## Business Logic

### POST /api/v1/executions Processing

1. **Validation**: Validate input DTOs (max 100 records)
2. **Filtering**: Skip records where `isOpen = true`
3. **Portfolio ID Resolution**: 
   - Call Trade Service: `GET /api/v2/executions?executionServiceId={id}`
   - Extract `portfolioId` from response
4. **Persistence**: Insert execution record with current timestamp for `ready_to_send_timestamp`
5. **Response**: Return batch processing results

### POST /api/v1/executions/send Processing

1. **Batch Control**: 
   - Get `max(start_time)` from `batch_history`
   - Insert new batch record with `previous_start_time`
   - Return 409 if duplicate batch detected
2. **Data Selection**: 
   - Select executions where `ready_to_send_timestamp >= previous_start_time AND < current_start_time`
3. **File Generation**: 
   - Format data according to Portfolio Accounting CLI specification
   - Write to configured shared directory (default: `/usr/local/share/files`)
4. **CLI Invocation**: 
   - Execute Portfolio Accounting CLI
   - Handle success/failure responses

### Portfolio Accounting File Format

| Field | Source | Description |
|-------|--------|-------------|
| portfolio_id | execution.portfolio_id | Portfolio identifier |
| security_id | execution.security_id | Security identifier |
| source_id | "AC" + execution.id | Allocation service source ID |
| transaction_type | execution.trade_type | BUY/SELL |
| quantity | execution.quantity | Trade quantity |
| price | execution.average_price | Average execution price |
| transaction_date | execution.trade_date | Trade date |

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | 8089 | HTTP server port |
| `DB_HOST` | globeco-allocation-service-postgresql | Database host |
| `DB_PORT` | 5432 | Database port |
| `DB_NAME` | postgres | Database name |
| `DB_USER` | postgres | Database user |
| `DB_PASSWORD` | | Database password |
| `TRADE_SERVICE_URL` | http://globeco-trade-service:8082 | Trade service URL |
| `OUTPUT_DIR` | /usr/local/share/files | Portfolio accounting output directory |
| `CLI_COMMAND` | | Portfolio accounting CLI command |
| `LOG_LEVEL` | info | Logging level |
| `METRICS_ENABLED` | true | Enable Prometheus metrics |
| `TRACING_ENABLED` | true | Enable OpenTelemetry tracing |

## Non-Functional Requirements

### Performance
- Support concurrent request processing
- Database connection pooling
- Efficient pagination for large datasets
- Background processing for CLI invocation

### Reliability
- Graceful error handling and recovery
- Circuit breaker for external service calls
- Retry logic with exponential backoff
- Transaction management for data consistency

### Observability
- Structured logging with correlation IDs
- Prometheus metrics for key operations
- OpenTelemetry distributed tracing
- Health check endpoints

### Security
- Input validation and sanitization
- CORS configuration
- Secure configuration management
- SQL injection prevention

### Deployment
- Docker containerization
- Kubernetes deployment manifests
- Multi-architecture support
- CI/CD pipeline integration

## Open Questions

1. **Error Handling**: What should happen if the Trade Service is unavailable during execution processing?  Answer: For now, fail the record with an appropriate message.  In a subsequent iteration, we will implement a retry process.

2. **Data Consistency**: Should there be a rollback mechanism if CLI invocation fails after file generation?  Answer: Not now.  In a subsequent iteration, we will implement a retry process.

3. **Concurrency**: How should concurrent `/executions/send` requests be handled beyond the current batch control mechanism?  Answer: For now, just the current batch control mechanism.  This will be tightened up in a future iteration.

4. **Retry Logic**: What are the specific retry policies for Trade Service calls and CLI invocation?  Answer: follow general best practices.  Retry up to three times with exponential backoff.  Make this configurable so we can adjust.

5. **Monitoring**: What specific metrics and alerts are needed for production monitoring?  Answer: Follow generally accepted practices for now.  We will refine later.

6. **Portfolio ID**: What should happen if the Trade Service doesn't return a portfolio ID for an execution?  Answer: Fail the record.  A potfolio ID is required for further processing.  We will tighten this control later.

7. **File Cleanup**: Should generated files be cleaned up after successful CLI processing?  For verification purposes, do not delete the files.  Add a configuration flag to delete files, which we will enable in the future.

8. **Time Zones**: Should all timestamps be stored/processed in UTC, or are there timezone considerations?  Answer: Store all dates in UTC.  However, trade_date should be determined based on the US Eastern Time Zone.

9. **Data Validation**: Are there additional business rules for execution data validation beyond basic type checking?  Answer: Not now.

10. **CLI Configuration**: What are the exact CLI command parameters and Docker/Kubernetes execution contexts? Answer:  For Docker, use the following:

  ```
  docker run --rm \
  -v /tmp/portfolio-files:/data \
  --network host \
  globeco-portfolio-cli:latest \
  process --file /data/your-transactions.csv --output-dir /data
  ```

For Kubernetes, see the [CLI-Usage-Guide.md](CLI-Usage-Guide.md)


## Execution Plan

### Phase 1: Project Setup ✅ COMPLETED
- [x] Initialize Go module and project structure
- [x] Set up Docker configuration
- [x] Configure CI/CD pipeline
- [x] Set up database migrations
- [x] Implement configuration management

### Phase 2: Core Infrastructure ✅ COMPLETED
- [x] Database layer implementation (sqlx, repository pattern)
- [x] HTTP server setup (Chi router)
- [x] Middleware implementation (logging, metrics, CORS)
- [x] Health check endpoints
- [x] Error handling framework

### Phase 3: Business Logic
- [ ] Execution model and repository
- [ ] Batch history model and repository
- [ ] Trade Service client implementation
- [ ] File generation logic
- [ ] CLI invocation mechanism

### Phase 4: API Implementation
- [ ] GET /api/v1/executions (with pagination)
- [ ] GET /api/v1/executions/{id}
- [ ] POST /api/v1/executions
- [ ] POST /api/v1/executions/send
- [ ] Input validation and error responses

### Phase 5: Observability
- [ ] Structured logging implementation
- [ ] Prometheus metrics integration
- [ ] OpenTelemetry tracing setup
- [ ] Monitoring dashboard configuration

### Phase 6: Testing
- [ ] Unit tests for all business logic
- [ ] Integration tests with testcontainers
- [ ] API endpoint testing
- [ ] Performance testing
- [ ] Error scenario testing

### Phase 7: Deployment
- [ ] Kubernetes manifests
- [ ] Docker image optimization
- [ ] Production configuration
- [ ] Documentation and runbooks
- [ ] Deployment verification

### Phase 8: Production Readiness
- [ ] Security review and hardening
- [ ] Performance optimization
- [ ] Monitoring and alerting setup
- [ ] Disaster recovery procedures
- [ ] Production deployment

