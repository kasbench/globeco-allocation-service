package observability

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.uber.org/zap"
)

const (
	serviceName    = "globeco-allocation-service"
	serviceVersion = "1.0.0"
)

// TracingConfig holds tracing configuration
type TracingConfig struct {
	Enabled        bool
	OTLPEndpoint   string
	SamplingRatio  float64
	TracingHeaders map[string]string
}

// TracingManager manages OpenTelemetry tracing
type TracingManager struct {
	provider *trace.TracerProvider
	logger   *zap.Logger
	config   TracingConfig
}

// NewTracingManager creates a new tracing manager
func NewTracingManager(config TracingConfig, logger *zap.Logger) (*TracingManager, error) {
	if !config.Enabled {
		logger.Info("Tracing is disabled")
		return &TracingManager{
			logger: logger,
			config: config,
		}, nil
	}

	// Create resource with service information
	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
			semconv.ServiceVersionKey.String(serviceVersion),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Set default configuration values
	if config.SamplingRatio == 0 {
		config.SamplingRatio = 1.0 // Sample all traces by default
	}

	// Create OTLP HTTP exporter (optional)
	var exporter trace.SpanExporter
	if config.OTLPEndpoint != "" {
		opts := []otlptracehttp.Option{
			otlptracehttp.WithEndpoint(config.OTLPEndpoint),
			otlptracehttp.WithInsecure(),
		}

		// Add custom headers if provided
		if len(config.TracingHeaders) > 0 {
			opts = append(opts, otlptracehttp.WithHeaders(config.TracingHeaders))
		}

		otlpExporter, err := otlptracehttp.New(context.Background(), opts...)
		if err != nil {
			logger.Warn("Failed to create OTLP exporter, using stdout", zap.Error(err))
			exporter = nil
		} else {
			exporter = otlpExporter
		}
	}

	// Create trace provider
	var tracerOpts []trace.TracerProviderOption
	tracerOpts = append(tracerOpts, trace.WithResource(res))
	tracerOpts = append(tracerOpts, trace.WithSampler(trace.TraceIDRatioBased(config.SamplingRatio)))

	if exporter != nil {
		tracerOpts = append(tracerOpts, trace.WithBatcher(exporter))
	}

	provider := trace.NewTracerProvider(tracerOpts...)

	// Set global tracer provider
	otel.SetTracerProvider(provider)

	// Set global propagator
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	logger.Info("OpenTelemetry tracing initialized",
		zap.String("service_name", serviceName),
		zap.String("service_version", serviceVersion),
		zap.String("otlp_endpoint", config.OTLPEndpoint),
		zap.Float64("sampling_ratio", config.SamplingRatio))

	return &TracingManager{
		provider: provider,
		logger:   logger,
		config:   config,
	}, nil
}

// Shutdown gracefully shuts down the tracing provider
func (tm *TracingManager) Shutdown(ctx context.Context) error {
	if tm.provider == nil {
		return nil
	}

	tm.logger.Info("Shutting down tracing provider")
	if err := tm.provider.Shutdown(ctx); err != nil {
		tm.logger.Error("Failed to shutdown tracing provider", zap.Error(err))
		return fmt.Errorf("failed to shutdown tracing provider: %w", err)
	}

	tm.logger.Info("Tracing provider shut down successfully")
	return nil
}

// ForceFlush forces all pending traces to be exported
func (tm *TracingManager) ForceFlush(ctx context.Context) error {
	if tm.provider == nil {
		return nil
	}

	return tm.provider.ForceFlush(ctx)
}

// IsEnabled returns whether tracing is enabled
func (tm *TracingManager) IsEnabled() bool {
	return tm.config.Enabled && tm.provider != nil
}
