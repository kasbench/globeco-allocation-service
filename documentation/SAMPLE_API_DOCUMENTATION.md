# Globeco Execution Service API Documentation

## Overview

The Globeco Execution Service provides a RESTful API for managing trade executions with enhanced filtering, pagination, batch processing, and security integration capabilities.

**Base URL:** `http://localhost:8084/api/v1`  
**Version:** v1.3.0  
**Content-Type:** `application/json`

## Authentication

Currently, the API does not require authentication. This will be added in future versions.

## Endpoints

### 1. Get Executions with Filtering and Pagination

**GET** `/executions`

Retrieve executions with optional filtering, sorting, and pagination support.

#### Query Parameters

| Parameter | Type | Required | Default | Description | Example |
|-----------|------|----------|---------|-------------|---------|
| `offset` | Integer | No | 0 | Number of records to skip | `10` |
| `limit` | Integer | No | 50 | Maximum records to return (max: 100) | `25` |
| `executionStatus` | String | No | - | Filter by execution status | `NEW` |
| `tradeType` | String | No | - | Filter by trade type | `BUY` |
| `destination` | String | No | - | Filter by destination exchange | `NYSE` |
| `ticker` | String | No | - | Filter by ticker symbol | `AAPL` |
| `sortBy` | String | No | `id` | Comma-separated sort fields with optional minus prefix for descending | `receivedTimestamp,-id` |

#### Valid Sort Fields
- `id` - Execution ID
- `receivedTimestamp` - When execution was received
- `sentTimestamp` - When execution was sent
- `executionStatus` - Current status
- `tradeType` - BUY or SELL
- `destination` - Target exchange
- `quantity` - Order quantity
- `limitPrice` - Limit price
- `ticker` - Security ticker symbol

#### Response

**Status Code:** `200 OK`

```json
{
  "content": [
    {
      "id": 1,
      "executionStatus": "NEW",
      "tradeType": "BUY",
      "destination": "NYSE",
      "security": {
        "securityId": "SEC001",
        "ticker": "AAPL"
      },
      "quantity": 100.00,
      "limitPrice": 150.50,
      "receivedTimestamp": "2024-01-15T10:30:00Z",
      "sentTimestamp": "2024-01-15T10:30:05Z",
      "tradeServiceExecutionId": 12345,
      "quantityFilled": 0.00,
      "averagePrice": null,
      "version": 1
    }
  ],
  "pagination": {
    "offset": 0,
    "limit": 50,
    "totalElements": 150,
    "totalPages": 3,
    "currentPage": 0,
    "hasNext": true,
    "hasPrevious": false
  }
}
```

#### Example Requests

```bash
# Get first 10 executions
GET /api/v1/executions?limit=10

# Get NEW executions sorted by timestamp descending
GET /api/v1/executions?executionStatus=NEW&sortBy=-receivedTimestamp

# Get BUY orders for NYSE with pagination
GET /api/v1/executions?tradeType=BUY&destination=NYSE&offset=20&limit=10

# Complex filtering with multiple parameters
GET /api/v1/executions?executionStatus=FILLED&ticker=AAPL&sortBy=receivedTimestamp,id
```

### 2. Get Execution by ID

**GET** `/execution/{id}`

Retrieve a specific execution by its unique identifier.

#### Path Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | Integer | Yes | Execution ID |

#### Response

**Status Code:** `200 OK` - Execution found  
**Status Code:** `404 Not Found` - Execution not found

```json
{
  "id": 1,
  "executionStatus": "NEW",
  "tradeType": "BUY",
  "destination": "NYSE",
  "security": {
    "securityId": "SEC001",
    "ticker": "AAPL"
  },
  "quantity": 100.00,
  "limitPrice": 150.50,
  "receivedTimestamp": "2024-01-15T10:30:00Z",
  "sentTimestamp": "2024-01-15T10:30:05Z",
  "tradeServiceExecutionId": 12345,
  "quantityFilled": 0.00,
  "averagePrice": null,
  "version": 1
}
```

### 3. Create Single Execution

**POST** `/executions`

Create a new trade execution.

#### Request Body

```json
{
  "tradeType": "BUY",
  "destination": "NYSE",
  "securityId": "SEC001",
  "quantity": 100.00,
  "limitPrice": 150.50
}
```

#### Response

**Status Code:** `201 Created` - Execution created successfully  
**Status Code:** `400 Bad Request` - Invalid execution data

```json
{
  "id": 1,
  "executionStatus": "NEW",
  "tradeType": "BUY",
  "destination": "NYSE",
  "security": {
    "securityId": "SEC001",
    "ticker": "AAPL"
  },
  "quantity": 100.00,
  "limitPrice": 150.50,
  "receivedTimestamp": "2024-01-15T10:30:00Z",
  "sentTimestamp": null,
  "tradeServiceExecutionId": null,
  "quantityFilled": 0.00,
  "averagePrice": null,
  "version": 1
}
```

### 4. Create Batch Executions

**POST** `/executions/batch`

Create multiple executions in a single batch operation (up to 100 executions).

#### Request Body

```json
{
  "executions": [
    {
      "tradeType": "BUY",
      "destination": "NYSE",
      "securityId": "SEC001",
      "quantity": 100.00,
      "limitPrice": 150.50
    },
    {
      "tradeType": "SELL",
      "destination": "NASDAQ",
      "securityId": "SEC002",
      "quantity": 200.00,
      "limitPrice": 75.25
    }
  ]
}
```

#### Response

**Status Code:** `201 Created` - All executions created successfully  
**Status Code:** `207 Multi-Status` - Partial success (some executions failed)  
**Status Code:** `400 Bad Request` - All executions failed or validation errors

```json
{
  "status": "SUCCESS",
  "message": "All executions processed successfully",
  "totalRequested": 2,
  "successful": 2,
  "failed": 0,
  "results": [
    {
      "requestIndex": 0,
      "status": "SUCCESS",
      "message": "Execution created successfully",
      "execution": {
        "id": 1,
        "executionStatus": "NEW",
        "tradeType": "BUY",
        "destination": "NYSE",
        "security": {
          "securityId": "SEC001",
          "ticker": "AAPL"
        },
        "quantity": 100.00,
        "limitPrice": 150.50,
        "receivedTimestamp": "2024-01-15T10:30:00Z",
        "sentTimestamp": null,
        "tradeServiceExecutionId": null,
        "quantityFilled": 0.00,
        "averagePrice": null,
        "version": 1
      }
    },
    {
      "requestIndex": 1,
      "status": "SUCCESS",
      "message": "Execution created successfully",
      "execution": {
        "id": 2,
        "executionStatus": "NEW",
        "tradeType": "SELL",
        "destination": "NASDAQ",
        "security": {
          "securityId": "SEC002",
          "ticker": "GOOGL"
        },
        "quantity": 200.00,
        "limitPrice": 75.25,
        "receivedTimestamp": "2024-01-15T10:30:01Z",
        "sentTimestamp": null,
        "tradeServiceExecutionId": null,
        "quantityFilled": 0.00,
        "averagePrice": null,
        "version": 1
      }
    }
  ]
}
```

#### Batch Status Values

- `SUCCESS` - All executions processed successfully
- `PARTIAL_SUCCESS` - Some executions succeeded, some failed
- `FAILED` - All executions failed

### 5. Update Execution

**PUT** `/execution/{id}`

Update an execution's fill quantities and average price.

#### Path Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | Integer | Yes | Execution ID |

#### Request Body

```json
{
  "quantityFilled": 50.00,
  "averagePrice": 149.75,
  "version": 1
}
```

#### Response

**Status Code:** `200 OK` - Execution updated successfully  
**Status Code:** `404 Not Found` - Execution not found  
**Status Code:** `409 Conflict` - Version conflict (optimistic locking)

```json
{
  "id": 1,
  "executionStatus": "PARTIALLY_FILLED",
  "tradeType": "BUY",
  "destination": "NYSE",
  "security": {
    "securityId": "SEC001",
    "ticker": "AAPL"
  },
  "quantity": 100.00,
  "limitPrice": 150.50,
  "receivedTimestamp": "2024-01-15T10:30:00Z",
  "sentTimestamp": "2024-01-15T10:30:05Z",
  "tradeServiceExecutionId": 12345,
  "quantityFilled": 50.00,
  "averagePrice": 149.75,
  "version": 2
}
```

## Data Models

### ExecutionDTO

| Field | Type | Description |
|-------|------|-------------|
| `id` | Integer | Unique execution identifier |
| `executionStatus` | String | Current status (NEW, SENT, FILLED, PARTIALLY_FILLED, CANCELLED) |
| `tradeType` | String | Trade direction (BUY, SELL) |
| `destination` | String | Target exchange (NYSE, NASDAQ, LSE, etc.) |
| `security` | SecurityDTO | Security information |
| `quantity` | BigDecimal | Order quantity |
| `limitPrice` | BigDecimal | Limit price |
| `receivedTimestamp` | OffsetDateTime | When execution was received |
| `sentTimestamp` | OffsetDateTime | When execution was sent to trading platform |
| `tradeServiceExecutionId` | Integer | External trading platform execution ID |
| `quantityFilled` | BigDecimal | Quantity filled so far |
| `averagePrice` | BigDecimal | Average fill price |
| `version` | Integer | Version for optimistic locking |

### SecurityDTO

| Field | Type | Description |
|-------|------|-------------|
| `securityId` | String | Unique security identifier |
| `ticker` | String | Stock ticker symbol |

### PaginationDTO

| Field | Type | Description |
|-------|------|-------------|
| `offset` | Integer | Number of records skipped |
| `limit` | Integer | Maximum records per page |
| `totalElements` | Long | Total number of records |
| `totalPages` | Integer | Total number of pages |
| `currentPage` | Integer | Current page number (0-based) |
| `hasNext` | Boolean | Whether there is a next page |
| `hasPrevious` | Boolean | Whether there is a previous page |

### BatchExecutionRequestDTO

| Field | Type | Description |
|-------|------|-------------|
| `executions` | List<ExecutionPostDTO> | List of executions to create (max 100) |

### BatchExecutionResponseDTO

| Field | Type | Description |
|-------|------|-------------|
| `status` | String | Overall batch status |
| `message` | String | Status message |
| `totalRequested` | Integer | Total number of executions requested |
| `successful` | Integer | Number of successful executions |
| `failed` | Integer | Number of failed executions |
| `results` | List<ExecutionResultDTO> | Individual results for each execution |

### ExecutionResultDTO

| Field | Type | Description |
|-------|------|-------------|
| `requestIndex` | Integer | Index in the original request |
| `status` | String | Individual execution status (SUCCESS, FAILED) |
| `message` | String | Status message |
| `execution` | ExecutionDTO | Created execution (null if failed) |

## Error Handling

The API uses standard HTTP status codes and returns error information in JSON format:

```json
{
  "timestamp": "2024-01-15T10:30:00Z",
  "status": 400,
  "error": "Bad Request",
  "message": "Validation failed for field 'quantity': must be greater than 0",
  "path": "/api/v1/executions"
}
```

### Common Error Codes

- `400 Bad Request` - Invalid request data or validation errors
- `404 Not Found` - Resource not found
- `409 Conflict` - Version conflict during update
- `413 Payload Too Large` - Batch request exceeds 100 executions
- `500 Internal Server Error` - Server error

## Performance Characteristics

### Pagination Performance
- Optimized queries with database indexes
- Recommended page size: 50 records
- Maximum page size: 100 records

### Batch Processing Performance
- Maximum batch size: 100 executions
- Expected throughput: >10 executions/second
- Transactional processing with rollback on critical failures

### Filtering Performance
- Indexed fields for fast filtering: `executionStatus`, `tradeType`, `destination`, `securityId`
- Composite indexes for common filter combinations
- Sub-second response times for typical datasets

## Security Integration

The API integrates with the Security Service to provide ticker information:

- Security data is cached for 5 minutes to improve performance
- Cache hit ratio typically >80% in normal operation
- Automatic fallback to Security Service API on cache miss

## Monitoring and Health

### Performance Monitoring
- Query performance metrics collected automatically
- Slow query detection (>1 second threshold)
- Cache hit ratio monitoring

### Health Checks
- Database connectivity monitoring
- Security Service integration health
- Performance health indicators

## OpenAPI Specification

The complete OpenAPI 3.0 specification is available at:
- **Development:** `http://localhost:8084/swagger-ui.html`
- **API Docs:** `http://localhost:8084/v3/api-docs`

## Examples

### Complete Workflow Example

```bash
# 1. Create a batch of executions
curl -X POST http://localhost:8084/api/v1/executions/batch \
  -H "Content-Type: application/json" \
  -d '{
    "executions": [
      {
        "tradeType": "BUY",
        "destination": "NYSE",
        "securityId": "SEC001",
        "quantity": 100.00,
        "limitPrice": 150.50
      },
      {
        "tradeType": "SELL",
        "destination": "NASDAQ",
        "securityId": "SEC002",
        "quantity": 200.00,
        "limitPrice": 75.25
      }
    ]
  }'

# 2. Get executions with filtering
curl "http://localhost:8084/api/v1/executions?executionStatus=NEW&ticker=AAPL&limit=10"

# 3. Get specific execution
curl "http://localhost:8084/api/v1/execution/1"

# 4. Update execution with fill data
curl -X PUT http://localhost:8084/api/v1/execution/1 \
  -H "Content-Type: application/json" \
  -d '{
    "quantityFilled": 50.00,
    "averagePrice": 149.75,
    "version": 1
  }'
```

## Migration Guide

### Breaking Changes from v1.2.0

1. **ExecutionDTO Structure Change**
   - `securityId` field replaced with `security` object containing `securityId` and `ticker`
   - Update client code to access `execution.security.securityId` instead of `execution.securityId`

2. **New Pagination Response Format**
   - Responses now wrapped in `ExecutionPageDTO` with `content` and `pagination` fields
   - Update client code to access `response.content` for execution list

3. **Query Parameter Change: securityId → ticker**
   - **BREAKING CHANGE**: The `securityId` query parameter has been replaced with `ticker` for filtering
   - Update client code to use `ticker=AAPL` instead of `securityId=SEC001`
   - This change makes the API more user-friendly by allowing filtering by ticker symbols
   - Ticker filtering uses in-memory caching with 5-minute TTL for optimal performance

4. **Enhanced Filtering Parameters**
   - Multiple new query parameters available for filtering
   - Existing behavior preserved for non-breaking parameter changes

### Recommended Migration Steps

1. Update client DTOs to match new `ExecutionDTO` and `ExecutionPageDTO` structures
2. **Update filtering logic**: Replace `securityId` query parameter with `ticker` in all API calls
3. Test batch processing functionality in development environment
4. Update any hardcoded security ID references to use new security object structure
5. Validate pagination handling in client applications
6. Test ticker-based filtering to ensure expected results

For assistance with migration, contact the development team at dev@globeco.com. 