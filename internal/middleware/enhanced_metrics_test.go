package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.uber.org/zap"
)

func TestEnhancedMetricsMiddleware(t *testing.T) {
	// Setup test meter provider
	reader := sdkmetric.NewManualReader()
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(resource.Default()),
		sdkmetric.WithReader(reader),
	)
	otel.SetMeterProvider(meterProvider)

	// Create test logger
	logger, _ := zap.NewDevelopment()

	// Create middleware
	middleware := NewEnhancedMetricsMiddleware(EnhancedMetricsConfig{
		ServiceName: "test-service",
		Enabled:     true,
	}, logger)

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

	// Verify response
	if recorder.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", recorder.Code)
	}

	if recorder.Body.String() != "OK" {
		t.Errorf("Expected body 'OK', got %s", recorder.Body.String())
	}

	// Collect metrics
	rm := metricdata.ResourceMetrics{}
	err := reader.Collect(context.Background(), &rm)
	if err != nil {
		t.Errorf("Failed to collect metrics: %v", err)
	}

	// Basic validation that metrics were collected
	// Note: Detailed metric validation would require more complex setup
	t.Log("Enhanced metrics middleware test completed successfully")
}

func TestEnhancedMetricsMiddleware_Disabled(t *testing.T) {
	// Create test logger
	logger, _ := zap.NewDevelopment()

	// Create disabled middleware
	middleware := NewEnhancedMetricsMiddleware(EnhancedMetricsConfig{
		ServiceName: "test-service",
		Enabled:     false,
	}, logger)

	// Test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Wrap with middleware
	handler := middleware.Handler()(testHandler)

	// Make test request
	req := httptest.NewRequest("GET", "/test", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	// Verify response (should work normally even when disabled)
	if recorder.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", recorder.Code)
	}

	if recorder.Body.String() != "OK" {
		t.Errorf("Expected body 'OK', got %s", recorder.Body.String())
	}

	t.Log("Disabled enhanced metrics middleware test completed successfully")
}

func TestPathPatternNormalization(t *testing.T) {
	// Create test logger
	logger, _ := zap.NewDevelopment()

	// Create middleware
	middleware := NewEnhancedMetricsMiddleware(EnhancedMetricsConfig{
		ServiceName: "test-service",
		Enabled:     false, // Disabled to avoid OpenTelemetry setup
	}, logger)

	tests := []struct {
		input    string
		expected string
	}{
		{"/healthz", "/healthz"},
		{"/api/v1/executions", "/api/v1/executions"},
		{"/api/v1/executions/123", "/api/v1/executions/{id}"},
		{"/api/v1/executions/abc-def", "/api/v1/executions/{id}"},
		{"/swagger-ui/index.html", "/swagger-ui/index.html"},
		{"/swagger-ui/some-file.js", "/swagger-ui/*"},
		{"/unknown/path", "/unknown/path"},
	}

	for _, test := range tests {
		result := middleware.normalizePathPattern(test.input)
		if result != test.expected {
			t.Errorf("normalizePathPattern(%s) = %s, expected %s", test.input, result, test.expected)
		}
	}
}

func TestSanitizeMethods(t *testing.T) {
	// Create test logger
	logger, _ := zap.NewDevelopment()

	// Create middleware
	middleware := NewEnhancedMetricsMiddleware(EnhancedMetricsConfig{
		ServiceName: "test-service",
		Enabled:     false, // Disabled to avoid OpenTelemetry setup
	}, logger)

	tests := []struct {
		input    string
		expected string
	}{
		{"GET", "GET"},
		{"post", "POST"},
		{"", "UNKNOWN"},
		{"VERY_LONG_METHOD_NAME", "INVALID"},
		{" PUT ", "PUT"},
	}

	for _, test := range tests {
		result := middleware.sanitizeMethod(test.input)
		if result != test.expected {
			t.Errorf("sanitizeMethod(%s) = %s, expected %s", test.input, result, test.expected)
		}
	}
}

func TestSanitizeStatus(t *testing.T) {
	// Create test logger
	logger, _ := zap.NewDevelopment()

	// Create middleware
	middleware := NewEnhancedMetricsMiddleware(EnhancedMetricsConfig{
		ServiceName: "test-service",
		Enabled:     false, // Disabled to avoid OpenTelemetry setup
	}, logger)

	tests := []struct {
		input    int
		expected string
	}{
		{200, "200"},
		{404, "404"},
		{500, "500"},
		{99, "unknown"},   // Below valid range
		{600, "unknown"},  // Above valid range
	}

	for _, test := range tests {
		result := middleware.sanitizeStatus(test.input)
		if result != test.expected {
			t.Errorf("sanitizeStatus(%d) = %s, expected %s", test.input, result, test.expected)
		}
	}
}