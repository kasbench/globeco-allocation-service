package middleware

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
)

// Legacy Prometheus metrics (kept for backward compatibility)
var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "endpoint", "status"},
	)

	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration",
			Help:    "Duration of HTTP requests",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "endpoint", "status"},
	)

	httpRequestsInFlight = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "http_requests_in_flight",
			Help: "Number of HTTP requests currently being processed",
		},
	)
)

func init() {
	prometheus.MustRegister(httpRequestsTotal)
	prometheus.MustRegister(httpRequestDuration)
	prometheus.MustRegister(httpRequestsInFlight)
}

// EnhancedMetricsConfig holds configuration for enhanced metrics middleware
type EnhancedMetricsConfig struct {
	ServiceName           string
	Enabled               bool
	MaxPathPatternCache   int  // Maximum number of path patterns to cache
	MaxPathLength         int  // Maximum path length to prevent cardinality explosion
	EnableFailsafeLogging bool // Enable detailed error logging for debugging
}

// EnhancedMetricsMiddleware provides OpenTelemetry-based HTTP metrics collection
type EnhancedMetricsMiddleware struct {
	// OpenTelemetry metrics - CRITICAL: These exact metric types are required
	httpRequestsTotal    metric.Int64Counter      // Counter for total requests
	httpRequestDuration  metric.Float64Histogram  // Histogram for request duration
	httpRequestsInFlight metric.Int64UpDownCounter // Gauge for concurrent requests

	// Configuration
	serviceName           string
	meter                 metric.Meter
	logger                *zap.Logger
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

// NewEnhancedMetricsMiddleware creates a new enhanced metrics middleware
func NewEnhancedMetricsMiddleware(config EnhancedMetricsConfig, logger *zap.Logger) *EnhancedMetricsMiddleware {
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
		logger:                logger,
		initializationFailed:  false,
	}

	if !config.Enabled {
		logger.Info("Enhanced metrics middleware is disabled")
		return middleware
	}

	// CRITICAL: Use existing meter provider
	meterProvider := otel.GetMeterProvider()
	if meterProvider == nil {
		logger.Warn("OpenTelemetry meter provider is not available, disabling enhanced metrics")
		middleware.enabled = false
		middleware.initializationFailed = true
		return middleware
	}

	// CRITICAL: Use service's package path for instrumentation scope
	middleware.meter = meterProvider.Meter(
		"github.com/kasbench/globeco-allocation-service/middleware",
		metric.WithInstrumentationVersion("1.0.0"),
	)

	// Initialize metrics with comprehensive error handling
	if err := middleware.initializeMetrics(); err != nil {
		logger.Error("Failed to initialize enhanced metrics, disabling middleware", zap.Error(err))
		middleware.enabled = false
		middleware.initializationFailed = true
		return middleware
	}

	logger.Info("Enhanced metrics middleware initialized successfully")
	return middleware
}

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
						m.logger.Error("Panic recovered in enhanced metrics middleware", zap.Any("panic", recovered))
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
		"/healthz":                "/healthz",
		"/readyz":                 "/readyz",
		"/metrics":                "/metrics",
		"/openapi.yaml":           "/openapi.yaml",
		"/swagger-ui/":            "/swagger-ui/",
		"/swagger-ui/index.html":  "/swagger-ui/index.html",
		"/api/v1/executions":      "/api/v1/executions",
		"/api/v1/executions/send": "/api/v1/executions/send",
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
		{
			regex:   regexp.MustCompile(`^/api/v1/executions/[^/]+$`),
			pattern: "/api/v1/executions/{id}",
		},
		{
			regex:   regexp.MustCompile(`^/swagger-ui/.*$`),
			pattern: "/swagger-ui/*",
		},
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
		m.logger.Error(message, zap.String("metric", metricName), zap.Error(err))
	} else {
		m.logger.Warn(message, zap.String("metric", metricName), zap.Error(err))
	}
}

// Legacy Prometheus middleware (kept for backward compatibility)
// Metrics returns a middleware that records Prometheus metrics
func Metrics() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			httpRequestsInFlight.Inc()
			defer httpRequestsInFlight.Dec()

			// Create a wrapped response writer to capture status code
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			// Process request
			next.ServeHTTP(ww, r)

			// Record metrics
			duration := time.Since(start).Seconds()
			method := r.Method
			endpoint := r.URL.Path
			status := strconv.Itoa(ww.Status())

			httpRequestsTotal.WithLabelValues(method, endpoint, status).Inc()
			httpRequestDuration.WithLabelValues(method, endpoint, status).Observe(duration)
		})
	}
}

// MetricsHandler returns a handler for the /metrics endpoint
func MetricsHandler() http.Handler {
	return promhttp.Handler()
}
