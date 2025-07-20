package middleware

import (
	"context"
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// OTELTracing returns OpenTelemetry HTTP middleware for tracing all APIs
func OTELTracing(serviceName string, logger *zap.Logger) func(next http.Handler) http.Handler {
	// Create OTEL HTTP handler with custom options
	return otelhttp.NewMiddleware(
		serviceName,
		otelhttp.WithMessageEvents(otelhttp.ReadEvents, otelhttp.WriteEvents),
		otelhttp.WithSpanNameFormatter(func(operation string, r *http.Request) string {
			return r.Method + " " + r.URL.Path
		}),
		otelhttp.WithFilter(func(r *http.Request) bool {
			// Skip health check endpoints from tracing to reduce noise
			path := r.URL.Path
			return path != "/healthz" && path != "/readyz" && path != "/metrics"
		}),
	)
}

// AddTraceAttributes adds custom attributes to the current span
func AddTraceAttributes(r *http.Request, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(r.Context())
	if span.IsRecording() {
		span.SetAttributes(attrs...)
	}
}

// StartSpan starts a new span with the given name and attributes
func StartSpan(r *http.Request, spanName string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	tracer := otel.Tracer("globeco-allocation-service")
	ctx, span := tracer.Start(r.Context(), spanName)
	if len(attrs) > 0 {
		span.SetAttributes(attrs...)
	}
	return ctx, span
}