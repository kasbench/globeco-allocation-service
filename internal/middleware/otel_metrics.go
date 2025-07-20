package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/kasbench/globeco-allocation-service/internal/observability"
)

// OTELMetrics returns middleware that records OpenTelemetry metrics for HTTP requests
func OTELMetrics(otelMetrics *observability.OTELMetricsManager) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ctx := r.Context()

			// Record request start
			otelMetrics.RecordHTTPRequestStart(ctx)
			defer otelMetrics.RecordHTTPRequestEnd(ctx)

			// Create a wrapped response writer to capture status code
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			// Process request
			next.ServeHTTP(ww, r)

			// Record metrics
			duration := time.Since(start)
			method := r.Method
			path := r.URL.Path
			status := strconv.Itoa(ww.Status())

			otelMetrics.RecordHTTPRequest(ctx, method, path, status, duration)
		})
	}
}