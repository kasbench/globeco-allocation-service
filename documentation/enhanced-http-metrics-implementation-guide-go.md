# Enhanced HTTP Metrics Implementation Guide for Go Microservices

## Overview

This guide provides step-by-step instructions for implementing enhanced HTTP metrics in Go microservices using OpenTelemetry. The implementation successfully produces `http_request_duration` in milliseconds and provides comprehensive HTTP request monitoring.

## Key Success Factors

Based on the successful implementation in the GlobeCo portfolio accounting service, the following factors are critical:

1. **Correct Duration Unit Conversion**: Convert nanoseconds to milliseconds using `float64(time.Since(start).Nanoseconds()) / 1e6`
2. **Proper OpenTelemetry Integration**: Use existing meter provider with `otel.GetMeterProvider()`
3. **Comprehensive Error Handling**: Implement graceful degradation when metrics fail
4. **Thread-Safe Implementation**: Use proper synchronization for concurrent requests
5. **Path Pattern Normalization**: Convert dynamic paths to route patterns for consistent labeling

## Prerequisites

- Go 1.19 or later
- OpenTelemetry Go SDK already integrated
- HTTP router (Chi, Gorilla Mux, or similar)
- Existing logger implementation

## Required Dependencies

```go
import (
    "context"
    "fmt"
    "net/http"
    "regexp"
    "strconv"
    "strings"
    "sync"
    "time"

    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/metric"
)
```

## Step 1: Configuration Structure

Create a configuration structure for the enhanced metrics middleware:

```go
// EnhancedMetricsConfig holds configuration for enhanced metrics middleware
type EnhancedMetricsConfig struct {
    ServiceName           string
    Enabled               bool
    MaxPathPatternCache   int  // Maximum number of path patterns to cache
    MaxPathLength         int  // Maximum path length to prevent cardinality explosion
    EnableFailsafeLogging bool // Enable detailed error logging for debugging
}
```

## Step 2: Core Middleware Structure

Implement the main middleware structure with all required components:

```go
// EnhancedMetricsMiddleware provides OpenTelemetry-based HTTP metrics collection
type EnhancedMetricsMiddleware struct {
    // OpenTelemetry metrics - CRITICAL: These exact metric types are required
    httpRequestsTotal    metric.Int64Counter      // Counter for total requests
    httpRequestDuration  metric.Float64Histogram  // Histogram for request duration
    httpRequestsInFlight metric.Int64UpDownCounter // Gauge for concurrent requests

    // Configuration
    serviceName           string
    meter                 metric.Meter
    logger                logger.Logger // Your logger interface
    enabled               bool
    maxPathPatternCache   int
    maxPathLength         int
    enableFailsafeLogging bool

    // Path pattern cache for performance with thread safety
    pathPatterns map[string]string
    cacheMutex   sync.RWMutex

    // Error tracking for graceful degradation
    initializationFailed bool
    errorCount           int64
    lastErrorTime        time.Time
    errorMutex           sync.RWMutex
}
```

## Step 3: Constructor with Error Handling

**CRITICAL**: This constructor pattern ensures proper initialization and graceful degradation:

```go
// NewEnhancedMetricsMiddleware creates a new enhanced metrics middleware
func NewEnhancedMetricsMiddleware(config EnhancedMetricsConfig) *EnhancedMetricsMiddleware {
    // Set default values for configuration
    if config.MaxPathPatternCache <= 0 {
        config.MaxPathPatternCache = 1000
    }
    if config.MaxPathLength <= 0 {
        config.MaxPathLength = 100
    }

    middleware := &EnhancedMetricsMiddleware{
        serviceName:           config.ServiceName,
        enabled:               config.Enabled,
        maxPathPatternCache:   config.MaxPathPatternCache,
        maxPathLength:         config.MaxPathLength,
        enableFailsafeLogging: config.EnableFailsafeLogging,
        pathPatterns:          make(map[string]string),
        logger:                logger.GetGlobal(), // Use your logger
        initializationFailed:  false,
    }

    if !config.Enabled {
        middleware.logger.Info("Enhanced metrics middleware is disabled")
        return middleware
    }

    // CRITICAL: Use existing meter provider
    meterProvider := otel.GetMeterProvider()
    if meterProvider == nil {
        middleware.logger.Warn("OpenTelemetry meter provider is not available, disabling enhanced metrics")
        middleware.enabled = false
        middleware.initializationFailed = true
        return middleware
    }

    // CRITICAL: Use your service's package path for instrumentation scope
    middleware.meter = meterProvider.Meter(
        "github.com/your-org/your-service/middleware", // Update this path
        metric.WithInstrumentationVersion("1.0.0"),
    )

    // Initialize metrics with comprehensive error handling
    if err := middleware.initializeMetrics(); err != nil {
        middleware.logger.Error("Failed to initialize enhanced metrics, disabling middleware", err)
        middleware.enabled = false
        middleware.initializationFailed = true
        return middleware
    }

    middleware.logger.Info("Enhanced metrics middleware initialized successfully")
    return middleware
}
```

## Step 4: Metrics Initialization

**CRITICAL**: This is where many implementations fail. Pay attention to the exact metric names, units, and bucket boundaries:

```go
// initializeMetrics creates and registers all OpenTelemetry metrics
func (m *EnhancedMetricsMiddleware) initializeMetrics() error {
    if m.meter == nil {
        return fmt.Errorf("meter is nil, cannot initialize metrics")
    }

    var err error
    var initErrors []string

    // HTTP Requests Total Counter
    m.httpRequestsTotal, err = m.meter.Int64Counter(
        "http_requests_total_enhanced", // Use unique name to avoid conflicts
        metric.WithDescription("Total number of HTTP requests (enhanced metrics)"),
        metric.WithUnit("{request}"),
    )
    if err != nil {
        initErrors = append(initErrors, fmt.Sprintf("failed to create counter: %v", err))
    }

    // HTTP Request Duration Histogram - CRITICAL: Exact buckets and unit
    m.httpRequestDuration, err = m.meter.Float64Histogram(
        "http_request_duration_milliseconds",
        metric.WithDescription("Duration of HTTP requests in milliseconds"),
        metric.WithUnit("ms"), // CRITICAL: Use "ms" for milliseconds
        // CRITICAL: These exact bucket boundaries in milliseconds
        metric.WithExplicitBucketBoundaries(5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000),
    )
    if err != nil {
        initErrors = append(initErrors, fmt.Sprintf("failed to create histogram: %v", err))
    }

    // HTTP Requests In Flight Gauge
    m.httpRequestsInFlight, err = m.meter.Int64UpDownCounter(
        "http_requests_in_flight_enhanced",
        metric.WithDescription("Number of HTTP requests currently being processed"),
        metric.WithUnit("{request}"),
    )
    if err != nil {
        initErrors = append(initErrors, fmt.Sprintf("failed to create gauge: %v", err))
    }

    // Return combined error if any metrics failed
    if len(initErrors) > 0 {
        return fmt.Errorf("metric initialization failed: %v", strings.Join(initErrors, "; "))
    }

    return nil
}
```

## Step 5: Response Writer Wrapper

Create a response writer wrapper to capture status codes:

```go
// enhancedMetricsResponseWriter wraps http.ResponseWriter to capture status codes
type enhancedMetricsResponseWriter struct {
    http.ResponseWriter
    statusCode int
    written    bool
}

// WriteHeader captures the status code
func (w *enhancedMetricsResponseWriter) WriteHeader(code int) {
    if !w.written {
        w.statusCode = code
        w.written = true
    }
    w.ResponseWriter.WriteHeader(code)
}

// Write ensures WriteHeader is called with default status if not already called
func (w *enhancedMetricsResponseWriter) Write(b []byte) (int, error) {
    if !w.written {
        w.WriteHeader(http.StatusOK)
    }
    return w.ResponseWriter.Write(b)
}
```

## Step 6: Main Handler Implementation

**CRITICAL**: This is the core logic that records metrics correctly:

```go
// Handler returns a middleware handler function for enhanced metrics collection
func (m *EnhancedMetricsMiddleware) Handler() func(http.Handler) http.Handler {
    if !m.enabled {
        // Return a no-op middleware if disabled
        return func(next http.Handler) http.Handler {
            return next
        }
    }

    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Safety check for nil metrics
            if m.httpRequestsInFlight == nil || m.httpRequestsTotal == nil || m.httpRequestDuration == nil {
                next.ServeHTTP(w, r)
                return
            }

            // CRITICAL: Start timing immediately
            start := time.Now()

            // Increment in-flight requests gauge
            m.recordMetricSafely(func() error {
                m.httpRequestsInFlight.Add(r.Context(), 1)
                return nil
            }, "http_requests_in_flight_increment")

            // CRITICAL: Ensure we decrement the gauge when the request completes
            defer func() {
                ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
                defer cancel()
                m.recordMetricSafely(func() error {
                    m.httpRequestsInFlight.Add(ctx, -1)
                    return nil
                }, "http_requests_in_flight_decrement")
            }()

            // Extract path pattern for consistent labeling
            pathPattern := m.extractPathPatternSafely(r.URL.Path)

            // Create enhanced metrics response writer to capture status code
            enhancedWriter := &enhancedMetricsResponseWriter{
                ResponseWriter: w,
                statusCode:     http.StatusOK, // Default to 200
            }

            // Call next handler with panic recovery
            func() {
                defer func() {
                    if recovered := recover(); recovered != nil {
                        m.logger.Error("Panic recovered in enhanced metrics middleware", recovered)
                        panic(recovered) // Re-panic to let the application handle it
                    }
                }()
                next.ServeHTTP(enhancedWriter, r)
            }()

            // CRITICAL: Calculate duration in milliseconds
            duration := float64(time.Since(start).Nanoseconds()) / 1e6

            // Prepare labels with validation
            method := m.sanitizeMethod(r.Method)
            status := m.sanitizeStatus(enhancedWriter.statusCode)

            // Create attributes for metrics
            attrs := []attribute.KeyValue{
                attribute.String("method", method),
                attribute.String("path", pathPattern),
                attribute.String("status", status),
            }

            // Record counter metric
            m.recordMetricSafely(func() error {
                m.httpRequestsTotal.Add(r.Context(), 1, metric.WithAttributes(attrs...))
                return nil
            }, "http_requests_total")

            // CRITICAL: Record histogram metric in milliseconds
            m.recordMetricSafely(func() error {
                // Validate duration to prevent extreme values
                if duration < 0 || duration > 300000 { // 5 minutes max
                    return fmt.Errorf("invalid duration: %f ms", duration)
                }
                m.httpRequestDuration.Record(r.Context(), duration, metric.WithAttributes(attrs...))
                return nil
            }, "http_request_duration_milliseconds")
        })
    }
}
```

## Step 7: Safe Metric Recording

Implement safe metric recording to prevent request blocking:

```go
// recordMetricSafely wraps metric recording with error handling
func (m *EnhancedMetricsMiddleware) recordMetricSafely(recordFunc func() error, metricName string) {
    defer func() {
        if recovered := recover(); recovered != nil {
            m.logMetricError("Panic recovered during metric recording", metricName, 
                fmt.Errorf("panic: %v", recovered))
        }
    }()

    // Use a timeout context to prevent hanging
    ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
    defer cancel()

    // Create a channel to handle the metric recording with timeout
    done := make(chan error, 1)
    go func() {
        defer func() {
            if recovered := recover(); recovered != nil {
                done <- fmt.Errorf("panic in metric recording goroutine: %v", recovered)
            }
        }()
        done <- recordFunc()
    }()

    select {
    case err := <-done:
        if err != nil {
            m.logMetricError("Failed to record metric", metricName, err)
        }
    case <-ctx.Done():
        m.logMetricError("Metric recording timed out", metricName, ctx.Err())
    }
}
```

## Step 8: Path Pattern Extraction

Implement path pattern extraction for consistent labeling:

```go
// extractPathPatternSafely normalizes URL paths to route patterns
func (m *EnhancedMetricsMiddleware) extractPathPatternSafely(path string) string {
    // Validate path length to prevent cardinality explosion
    if len(path) > m.maxPathLength {
        return "/path_too_long"
    }

    // Check cache first for performance
    m.cacheMutex.RLock()
    if pattern, exists := m.pathPatterns[path]; exists {
        m.cacheMutex.RUnlock()
        return pattern
    }
    m.cacheMutex.RUnlock()

    // Normalize the path pattern
    pattern := m.normalizePathPattern(path)

    // Cache the result with size limit protection
    m.cacheMutex.Lock()
    defer m.cacheMutex.Unlock()

    if len(m.pathPatterns) >= m.maxPathPatternCache {
        return pattern // Don't cache if limit reached
    }

    m.pathPatterns[path] = pattern
    return pattern
}

// normalizePathPattern converts actual paths to route patterns
func (m *EnhancedMetricsMiddleware) normalizePathPattern(path string) string {
    // Static path mappings for exact matches
    staticPaths := map[string]string{
        "/health":              "/health",
        "/health/live":         "/health/live",
        "/health/ready":        "/health/ready",
        "/metrics":             "/metrics",
        "/api/v1/transactions": "/api/v1/transactions",
        "/api/v1/balances":     "/api/v1/balances",
        // Add your service's static paths here
    }

    // Check for exact static matches first
    if pattern, exists := staticPaths[path]; exists {
        return pattern
    }

    // Dynamic path pattern matching using regex
    patterns := []struct {
        regex   *regexp.Regexp
        pattern string
    }{
        // Add your service's dynamic patterns here
        {
            regex:   regexp.MustCompile(`^/api/v1/transaction/[^/]+$`),
            pattern: "/api/v1/transaction/{id}",
        },
        {
            regex:   regexp.MustCompile(`^/api/v1/balance/[^/]+$`),
            pattern: "/api/v1/balance/{id}",
        },
        // Add more patterns as needed
    }

    // Check dynamic patterns
    for _, p := range patterns {
        if p.regex.MatchString(path) {
            return p.pattern
        }
    }

    // For unknown paths, return the path itself but limit length
    if len(path) > 100 {
        return "/unknown_long_path"
    }

    return path
}
```

## Step 9: Utility Functions

Implement sanitization functions:

```go
// sanitizeMethod ensures HTTP method is valid and uppercase
func (m *EnhancedMetricsMiddleware) sanitizeMethod(method string) string {
    method = strings.ToUpper(strings.TrimSpace(method))
    if method == "" {
        return "UNKNOWN"
    }
    if len(method) > 10 {
        return "INVALID"
    }
    return method
}

// sanitizeStatus ensures HTTP status code is valid
func (m *EnhancedMetricsMiddleware) sanitizeStatus(statusCode int) string {
    if statusCode < 100 || statusCode > 599 {
        return "unknown"
    }
    return strconv.Itoa(statusCode)
}

// logMetricError handles error logging with rate limiting
func (m *EnhancedMetricsMiddleware) logMetricError(message, metricName string, err error) {
    m.errorMutex.Lock()
    defer m.errorMutex.Unlock()

    m.errorCount++
    now := time.Now()

    // Rate limit error logging (max 1 error log per second)
    if now.Sub(m.lastErrorTime) < time.Second {
        return
    }
    m.lastErrorTime = now

    if m.enableFailsafeLogging {
        m.logger.Error(message, err) // Use your logger's error method
    } else {
        m.logger.Warn(message, err) // Use your logger's warn method
    }
}
```

## Step 10: Integration into Router

Integrate the middleware into your HTTP router:

```go
// In your router setup function (e.g., SetupRouter)
func SetupRouter() http.Handler {
    r := chi.NewRouter() // or your preferred router

    // Create enhanced metrics middleware
    enhancedMetrics := NewEnhancedMetricsMiddleware(EnhancedMetricsConfig{
        ServiceName:           "your-service-name", // Update this
        Enabled:               true,
        MaxPathPatternCache:   1000,
        MaxPathLength:         100,
        EnableFailsafeLogging: false, // Set to true for debugging
    })

    // CRITICAL: Add middleware early in the chain
    r.Use(enhancedMetrics.Handler())
    
    // Add your other middleware
    r.Use(middleware.Logger)
    r.Use(middleware.Recoverer)
    
    // Add your routes
    r.Get("/health", healthHandler)
    r.Route("/api/v1", func(r chi.Router) {
        r.Get("/transactions", getTransactions)
        r.Post("/transactions", createTransaction)
        // Add your routes
    })

    return r
}
```

## Step 11: Testing Implementation

Create comprehensive tests to validate the implementation:

```go
func TestEnhancedMetricsMiddleware(t *testing.T) {
    // Setup test meter provider
    reader := metric.NewManualReader()
    meterProvider := metric.NewMeterProvider(
        metric.WithResource(resource.Default()),
        metric.WithReader(reader),
    )
    otel.SetMeterProvider(meterProvider)

    // Create middleware
    middleware := NewEnhancedMetricsMiddleware(EnhancedMetricsConfig{
        ServiceName: "test-service",
        Enabled:     true,
    })

    // Test handler
    testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        time.Sleep(10 * time.Millisecond) // Simulate processing time
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("OK"))
    })

    // Wrap with middleware
    handler := middleware.Handler()(testHandler)

    // Make test request
    req := httptest.NewRequest("GET", "/test", nil)
    recorder := httptest.NewRecorder()
    handler.ServeHTTP(recorder, req)

    // Collect metrics
    rm := metricdata.ResourceMetrics{}
    err := reader.Collect(context.Background(), &rm)
    require.NoError(t, err)

    // Validate metrics exist and have correct values
    // Add your specific validation logic here
}
```

## Common Pitfalls and Solutions

### 1. Duration Unit Conversion
**Problem**: Recording duration in nanoseconds instead of milliseconds
**Solution**: Always use `float64(time.Since(start).Nanoseconds()) / 1e6`

### 2. Metric Initialization Failure
**Problem**: Service fails to start when OpenTelemetry is not available
**Solution**: Implement graceful degradation with proper error handling

### 3. High Cardinality Labels
**Problem**: Too many unique path values causing memory issues
**Solution**: Implement path pattern normalization and caching limits

### 4. Gauge Accuracy
**Problem**: In-flight gauge becomes inaccurate due to panics or errors
**Solution**: Use defer statements and proper error handling

### 5. Thread Safety
**Problem**: Race conditions in path pattern caching
**Solution**: Use sync.RWMutex for cache access

## Validation Checklist

- [ ] Duration is recorded in milliseconds (not nanoseconds or seconds)
- [ ] All three metrics are properly initialized
- [ ] Graceful degradation when OpenTelemetry is unavailable
- [ ] Path patterns are normalized (e.g., `/api/users/{id}`)
- [ ] Labels contain correct values (method, path, status)
- [ ] In-flight gauge increments and decrements correctly
- [ ] Error handling doesn't block request processing
- [ ] Thread-safe implementation for concurrent requests
- [ ] Proper metric names and descriptions
- [ ] Histogram buckets are in milliseconds

## Deployment Considerations

1. **Feature Flags**: Implement configuration to enable/disable metrics
2. **Performance Impact**: Monitor middleware overhead in production
3. **Memory Usage**: Watch for metric cardinality explosion
4. **Error Monitoring**: Set up alerts for metric recording failures
5. **Backward Compatibility**: Ensure existing metrics continue to work

## Troubleshooting

### Metrics Not Appearing
1. Check OpenTelemetry meter provider initialization
2. Verify metric names don't conflict with existing metrics
3. Ensure middleware is properly registered in router
4. Check for initialization errors in logs

### Incorrect Duration Values
1. Verify unit conversion: nanoseconds to milliseconds
2. Check histogram bucket boundaries
3. Validate timing logic in middleware

### High Memory Usage
1. Check path pattern cache size limits
2. Monitor unique label combinations
3. Implement cardinality limits for dynamic paths

This implementation guide provides a complete, production-ready solution for enhanced HTTP metrics in Go microservices. The key to success is following the exact patterns shown, especially for duration conversion and error handling.