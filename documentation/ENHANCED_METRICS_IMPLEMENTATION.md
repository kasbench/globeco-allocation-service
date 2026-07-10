# Enhanced HTTP Metrics Implementation

## Overview

This implementation adds enhanced HTTP metrics that produce the `http_request_duration` metric in **milliseconds** instead of seconds, following the GlobeCo suite standards and the detailed implementation guide.

## Key Changes

### 1. Enhanced Metrics Middleware (`internal/middleware/metrics.go`)

- **Added `EnhancedMetricsMiddleware`**: A new OpenTelemetry-based middleware that follows the implementation guide
- **Duration in Milliseconds**: Records `http_request_duration_milliseconds` with proper unit conversion
- **Proper Bucket Boundaries**: Uses millisecond-appropriate buckets: `[5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000]`
- **Path Pattern Normalization**: Converts dynamic paths to route patterns (e.g., `/api/v1/executions/123` → `/api/v1/executions/{id}`)
- **Thread-Safe Implementation**: Uses proper synchronization for concurrent requests
- **Graceful Degradation**: Continues to work even when OpenTelemetry is unavailable
- **Error Handling**: Comprehensive error handling that doesn't block request processing

### 2. Updated OpenTelemetry Metrics Manager (`internal/observability/otel_metrics.go`)

- **Changed metric name**: `http_request_duration_seconds` → `http_request_duration_milliseconds`
- **Updated unit**: Added `metric.WithUnit("ms")` for proper unit specification
- **Duration conversion**: Now converts nanoseconds to milliseconds using `float64(duration.Nanoseconds()) / 1e6`
- **Updated bucket boundaries**: Changed from seconds-based to milliseconds-based buckets

### 3. Configuration Updates (`internal/config/config.go`)

Added new configuration options for enhanced metrics:
- `enhanced_metrics_enabled`: Enable/disable enhanced metrics (default: true)
- `enhanced_metrics_max_path_pattern_cache`: Maximum cached path patterns (default: 1000)
- `enhanced_metrics_max_path_length`: Maximum path length to prevent cardinality explosion (default: 100)
- `enhanced_metrics_enable_failsafe_logging`: Enable detailed error logging (default: false)

### 4. Main Server Integration (`cmd/server/main.go`)

- **Conditional middleware**: Uses enhanced metrics when both `enhanced_metrics_enabled` and `otel_enabled` are true
- **Backward compatibility**: Falls back to legacy OTEL metrics when enhanced metrics are disabled
- **Proper logger integration**: Uses structured logger for enhanced metrics

## Metrics Produced

### Enhanced Metrics (when enabled)
- `http_requests_total_enhanced`: Counter for total HTTP requests
- `http_request_duration_milliseconds`: **Histogram for request duration in milliseconds** (main goal)
- `http_requests_in_flight_enhanced`: Gauge for concurrent requests

### Legacy Metrics (still available)
- `http_requests_total`: Prometheus counter (backward compatibility)
- `http_request_duration`: Prometheus histogram in seconds (backward compatibility)
- `http_requests_in_flight`: Prometheus gauge (backward compatibility)

## Key Features

### 1. Millisecond Duration Recording
```go
// CRITICAL: Calculate duration in milliseconds
duration := float64(time.Since(start).Nanoseconds()) / 1e6
```

### 2. Proper Bucket Boundaries
```go
metric.WithExplicitBucketBoundaries(5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000)
```

### 3. Path Pattern Normalization
- `/api/v1/executions/123` → `/api/v1/executions/{id}`
- `/swagger-ui/some-file.js` → `/swagger-ui/*`
- Static paths remain unchanged

### 4. Thread-Safe Caching
- Path patterns are cached for performance
- Uses `sync.RWMutex` for thread safety
- Configurable cache size limits

### 5. Error Handling
- Graceful degradation when OpenTelemetry is unavailable
- Timeout protection for metric recording
- Rate-limited error logging

## Configuration

### Environment Variables
```bash
# Enable enhanced metrics (default: true)
OBSERVABILITY_ENHANCED_METRICS_ENABLED=true

# Cache configuration
OBSERVABILITY_ENHANCED_METRICS_MAX_PATH_PATTERN_CACHE=1000
OBSERVABILITY_ENHANCED_METRICS_MAX_PATH_LENGTH=100

# Debug logging (default: false)
OBSERVABILITY_ENHANCED_METRICS_ENABLE_FAILSAFE_LOGGING=false
```

### Kubernetes ConfigMap
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: allocation-service-config
data:
  OBSERVABILITY_ENHANCED_METRICS_ENABLED: "true"
  OBSERVABILITY_ENHANCED_METRICS_MAX_PATH_PATTERN_CACHE: "1000"
  OBSERVABILITY_ENHANCED_METRICS_MAX_PATH_LENGTH: "100"
```

## Testing

### Unit Tests
- `TestEnhancedMetricsMiddleware`: Basic functionality test
- `TestEnhancedMetricsMiddleware_Disabled`: Disabled state test
- `TestPathPatternNormalization`: Path pattern conversion test
- `TestSanitizeMethods`: HTTP method sanitization test
- `TestSanitizeStatus`: HTTP status code sanitization test

### Running Tests
```bash
go test ./internal/middleware -v
```

## Deployment

1. **Build**: `go build -o server cmd/server/main.go`
2. **Deploy**: Use existing Kubernetes deployment
3. **Verify**: Check OpenTelemetry Collector for `http_request_duration_milliseconds` metric
4. **Monitor**: Ensure Prometheus shows the new metric with millisecond values

## Backward Compatibility

- Legacy Prometheus metrics are still available
- Legacy OpenTelemetry metrics are available when enhanced metrics are disabled
- No breaking changes to existing functionality
- Configuration is additive (new fields with sensible defaults)

## Troubleshooting

### Metrics Not Appearing
1. Check `OBSERVABILITY_ENHANCED_METRICS_ENABLED=true`
2. Check `OBSERVABILITY_OTEL_ENABLED=true`
3. Verify OpenTelemetry Collector connectivity
4. Check application logs for initialization errors

### Incorrect Duration Values
1. Verify the metric name is `http_request_duration_milliseconds`
2. Check that values are in milliseconds (not seconds)
3. Confirm bucket boundaries are appropriate for millisecond values

### High Memory Usage
1. Monitor path pattern cache size
2. Adjust `OBSERVABILITY_ENHANCED_METRICS_MAX_PATH_PATTERN_CACHE`
3. Check for high cardinality in path labels

## Next Steps for Kubernetes Testing

The implementation is ready for Kubernetes testing. When you deploy:

1. **Verify the metric name**: Look for `http_request_duration_milliseconds` in the OpenTelemetry Collector
2. **Check the unit**: Values should be in milliseconds (e.g., 50.5 for 50.5ms, not 0.0505 for seconds)
3. **Validate buckets**: Histogram buckets should be appropriate for millisecond values
4. **Monitor performance**: Ensure the enhanced middleware doesn't significantly impact request latency

The enhanced metrics middleware will automatically detect if OpenTelemetry is available and gracefully degrade if not, ensuring the service continues to function normally in all environments.