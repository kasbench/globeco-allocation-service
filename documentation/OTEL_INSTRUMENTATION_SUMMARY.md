# OpenTelemetry Instrumentation Summary for globeco-allocation-service

This document summarizes the comprehensive OpenTelemetry instrumentation implemented for the `globeco-allocation-service` following GlobeCo standards.

## Overview

The service has been fully instrumented with OpenTelemetry for:
- **Distributed Tracing** - All inbound and outbound API calls
- **Metrics Collection** - Business metrics, HTTP metrics, database metrics, and Go runtime metrics
- **Structured Logging** - Enhanced logging with trace context correlation

All telemetry data is sent to the OpenTelemetry Collector at `otel-collector-collector.monitoring.svc.cluster.local:4317` using gRPC OTLP protocol.

## Implementation Details

### 1. Configuration (GlobeCo Standards)

**Service Identity:**
- Service Name: `globeco-allocation-service`
- Service Version: `1.0.0`
- Service Namespace: `globeco`
- Endpoint: `otel-collector-collector.monitoring.svc.cluster.local:4317`

**Environment Variable Support:**
- `OTEL_EXPORTER_OTLP_ENDPOINT`
- `OTEL_SERVICE_NAME`
- `OTEL_SERVICE_VERSION`
- `OTEL_SERVICE_NAMESPACE`

### 2. Distributed Tracing

**Inbound API Tracing:**
- All HTTP requests are automatically traced using OpenTelemetry HTTP middleware
- Trace context is propagated through the request lifecycle
- Health check endpoints (`/healthz`, `/readyz`, `/metrics`) are filtered out to reduce noise

**Outbound API Tracing:**
- Trade Service API calls are fully instrumented with custom spans
- HTTP client uses OpenTelemetry transport for automatic instrumentation
- Retry attempts are tracked with individual spans

**Database Operation Tracing:**
- All database operations include custom spans with attributes:
  - `db.system`: "postgresql"
  - `db.operation`: INSERT/SELECT/UPDATE/DELETE
  - `db.table`: table name
  - Execution-specific attributes (IDs, trade types, etc.)

**Custom Span Attributes:**
- Service-specific business context
- Operation timing and status
- Error recording with proper status codes

### 3. Metrics Collection

**Go Runtime Metrics:**
- Goroutine count
- Memory heap allocation and system usage
- Stack memory usage
- Garbage collection runs and pause times

**HTTP Metrics:**
- Request count by method, path, and status
- Request duration histograms
- In-flight request gauge

**Database Metrics:**
- Operation count by operation type, table, and status
- Operation duration histograms
- Active connection count

**Trade Service Metrics:**
- API call count by method and status
- Call duration histograms
- Retry attempt counters

**Business Metrics:**
- Execution creation and processing counts
- Batch processing duration
- Portfolio file generation counts

### 4. Enhanced Logging

**Trace Context Integration:**
- All log entries include trace ID and span ID when available
- Correlation IDs are maintained throughout request lifecycle
- OpenTelemetry collector interactions are logged at INFO level

**Structured Logging:**
- JSON format for production
- Consistent field naming across all components
- Component-specific logging methods for different operations

## Key Files Modified/Created

### Core OpenTelemetry Setup
- `internal/observability/tracing.go` - Updated with GlobeCo-compliant OTEL setup
- `internal/observability/otel_metrics.go` - New comprehensive metrics manager
- `internal/config/config.go` - Added OTEL configuration options

### Middleware
- `internal/middleware/otel.go` - OpenTelemetry tracing middleware
- `internal/middleware/otel_metrics.go` - OpenTelemetry metrics middleware

### Service Layer Instrumentation
- `internal/service/trade_client.go` - Instrumented outbound API calls

### Repository Layer Instrumentation
- `internal/repository/execution.go` - Instrumented database operations

### Application Bootstrap
- `cmd/server/main.go` - Integrated OTEL manager and middleware

### Dependencies
- `go.mod` - Added OpenTelemetry dependencies following GlobeCo standards

## Verification Checklist

✅ **Dependencies** - All required OpenTelemetry packages added
✅ **Service Identity** - Proper service name, version, and namespace configured
✅ **OTLP Endpoint** - Using GlobeCo standard collector endpoint
✅ **Resource Attributes** - Service metadata properly set
✅ **Shutdown Handling** - Graceful shutdown of OTEL providers
✅ **HTTP Middleware** - All APIs instrumented for tracing
✅ **Outbound Calls** - Trade Service calls instrumented
✅ **Database Operations** - Repository operations instrumented
✅ **Go Runtime Metrics** - Standard Go metrics collected
✅ **Logging Integration** - Trace context in all log entries
✅ **Error Handling** - Proper error recording in spans

## Telemetry Data Flow

1. **Traces**: Application → OTLP gRPC → OpenTelemetry Collector → Jaeger
2. **Metrics**: Application → OTLP gRPC → OpenTelemetry Collector → Prometheus
3. **Logs**: Application → Structured JSON → Log aggregation system

## Debugging and Monitoring

**Log Messages for OTEL Operations:**
- "OpenTelemetry initialized successfully with GlobeCo standards"
- "Recorded [operation] metrics to OpenTelemetry collector"
- "Calling Trade Service with OpenTelemetry tracing"
- "Created execution with OpenTelemetry tracing"

**Trace Context in Logs:**
- All operations include `trace_id` and `span_id` fields
- Correlation IDs maintained across service boundaries

**Metrics Visibility:**
- All metrics are sent to the collector with proper labels
- Go runtime metrics provide system health visibility
- Business metrics track application-specific KPIs

## Environment Variables

```bash
# OpenTelemetry Configuration (optional overrides)
OTEL_EXPORTER_OTLP_ENDPOINT=otel-collector-collector.monitoring.svc.cluster.local:4317
OTEL_SERVICE_NAME=globeco-allocation-service
OTEL_SERVICE_VERSION=1.0.0
OTEL_SERVICE_NAMESPACE=globeco

# Application Configuration
OBSERVABILITY_OTEL_ENABLED=true
OBSERVABILITY_OTEL_ENDPOINT=otel-collector-collector.monitoring.svc.cluster.local:4317
OBSERVABILITY_OTEL_SERVICE_NAME=globeco-allocation-service
OBSERVABILITY_OTEL_SERVICE_VERSION=1.0.0
OBSERVABILITY_OTEL_SERVICE_NAMESPACE=globeco
```

## Performance Considerations

- **Sampling**: Currently set to sample all traces (100%) as per GlobeCo standards
- **Batching**: OTLP exporters use batching for efficient data transmission
- **Filtering**: Health check endpoints filtered from tracing to reduce noise
- **Async Export**: Metrics and traces are exported asynchronously to minimize latency impact

## Troubleshooting

**Common Issues:**
1. **Collector Unreachable**: Check network connectivity to `otel-collector-collector.monitoring.svc.cluster.local:4317`
2. **Missing Traces**: Verify OTEL middleware is properly configured in router
3. **Missing Metrics**: Check that OTEL metrics manager is initialized and middleware is active
4. **Log Correlation**: Ensure trace context is properly propagated through request lifecycle

**Debug Commands:**
```bash
# Check if service is sending data to collector
kubectl logs -n monitoring otel-collector-collector-xxx

# Verify service configuration
kubectl describe configmap -n default globeco-allocation-service-config

# Check service logs for OTEL messages
kubectl logs -n default globeco-allocation-service-xxx | grep -i otel
```

This implementation provides comprehensive observability for the globeco-allocation-service while maintaining consistency with GlobeCo standards and ensuring efficient telemetry data collection and transmission.