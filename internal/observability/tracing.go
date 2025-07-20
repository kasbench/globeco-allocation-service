package observability

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// OTELConfig holds OpenTelemetry configuration following GlobeCo standards
type OTELConfig struct {
	Enabled         bool
	Endpoint        string
	ServiceName     string
	ServiceVersion  string
	ServiceNamespace string
}

// OTELManager manages OpenTelemetry setup for both traces and metrics
type OTELManager struct {
	tracerProvider *trace.TracerProvider
	meterProvider  *metric.MeterProvider
	logger         *zap.Logger
	config         OTELConfig
}

// NewOTELManager creates a new OpenTelemetry manager following GlobeCo standards
func NewOTELManager(config OTELConfig, logger *zap.Logger) (*OTELManager, error) {
	if !config.Enabled {
		logger.Info("OpenTelemetry is disabled")
		return &OTELManager{
			logger: logger,
			config: config,
		}, nil
	}

	// Allow environment variable overrides for 12-factor compliance
	logger.Info("Checking OTEL environment variables for overrides")
	
	if envEndpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); envEndpoint != "" {
		config.Endpoint = envEndpoint
		logger.Info("Using OTEL endpoint from environment", zap.String("endpoint", envEndpoint))
	}
	if envServiceName := os.Getenv("OTEL_SERVICE_NAME"); envServiceName != "" {
		config.ServiceName = envServiceName
		logger.Info("Using service name from environment", zap.String("service_name", envServiceName))
	}
	if envServiceVersion := os.Getenv("OTEL_SERVICE_VERSION"); envServiceVersion != "" {
		config.ServiceVersion = envServiceVersion
		logger.Info("Using service version from environment", zap.String("service_version", envServiceVersion))
	}
	if envServiceNamespace := os.Getenv("OTEL_SERVICE_NAMESPACE"); envServiceNamespace != "" {
		config.ServiceNamespace = envServiceNamespace
		logger.Info("Using service namespace from environment", zap.String("service_namespace", envServiceNamespace))
	}
	
	// Log the final configuration
	logger.Info("Final OTEL configuration",
		zap.String("endpoint", config.Endpoint),
		zap.String("service_name", config.ServiceName),
		zap.String("service_version", config.ServiceVersion),
		zap.String("service_namespace", config.ServiceNamespace),
		zap.Bool("enabled", config.Enabled))

	ctx := context.Background()

	// Create resource with service information (GlobeCo standard)
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(config.ServiceName),
			semconv.ServiceVersionKey.String(config.ServiceVersion),
			semconv.ServiceNamespaceKey.String(config.ServiceNamespace),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	logger.Info("Setting up OpenTelemetry with GlobeCo standards",
		zap.String("service_name", config.ServiceName),
		zap.String("service_version", config.ServiceVersion),
		zap.String("service_namespace", config.ServiceNamespace),
		zap.String("endpoint", config.Endpoint))

	// Setup traces exporter (gRPC, insecure as per GlobeCo standards)
	logger.Info("Creating OTLP trace exporter with insecure connection", 
		zap.String("endpoint", config.Endpoint),
		zap.Bool("insecure", true))
	
	traceExp, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(config.Endpoint),
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
	)
	if err != nil {
		logger.Error("Failed to create OTLP trace exporter", zap.Error(err))
		return nil, fmt.Errorf("failed to create OTLP trace exporter: %w", err)
	}

	logger.Info("OTLP trace exporter created successfully", zap.String("endpoint", config.Endpoint))

	// Create tracer provider
	tracerProvider := trace.NewTracerProvider(
		trace.WithBatcher(traceExp),
		trace.WithResource(res),
		trace.WithSampler(trace.AlwaysSample()), // Sample all traces as per GlobeCo standards
	)
	otel.SetTracerProvider(tracerProvider)

	// Setup metrics exporter (gRPC, insecure as per GlobeCo standards)
	logger.Info("Creating OTLP metric exporter with insecure connection", 
		zap.String("endpoint", config.Endpoint),
		zap.Bool("insecure", true))
	
	metricExp, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(config.Endpoint),
		otlpmetricgrpc.WithInsecure(),
		otlpmetricgrpc.WithDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
	)
	if err != nil {
		logger.Error("Failed to create OTLP metric exporter", zap.Error(err))
		return nil, fmt.Errorf("failed to create OTLP metric exporter: %w", err)
	}

	logger.Info("OTLP metric exporter created successfully", zap.String("endpoint", config.Endpoint))

	// Create meter provider
	meterProvider := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(metricExp)),
		metric.WithResource(res),
	)
	otel.SetMeterProvider(meterProvider)

	// Set global propagator for distributed tracing
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	logger.Info("OpenTelemetry initialized successfully with GlobeCo standards",
		zap.String("service_name", config.ServiceName),
		zap.String("service_version", config.ServiceVersion),
		zap.String("service_namespace", config.ServiceNamespace),
		zap.String("endpoint", config.Endpoint))

	return &OTELManager{
		tracerProvider: tracerProvider,
		meterProvider:  meterProvider,
		logger:         logger,
		config:         config,
	}, nil
}

// Shutdown gracefully shuts down both tracer and meter providers
func (om *OTELManager) Shutdown(ctx context.Context) error {
	if om.tracerProvider == nil && om.meterProvider == nil {
		return nil
	}

	om.logger.Info("Shutting down OpenTelemetry providers")

	var err1, err2 error
	if om.tracerProvider != nil {
		om.logger.Info("Shutting down tracer provider")
		err1 = om.tracerProvider.Shutdown(ctx)
		if err1 != nil {
			om.logger.Error("Failed to shutdown tracer provider", zap.Error(err1))
		} else {
			om.logger.Info("Tracer provider shut down successfully")
		}
	}

	if om.meterProvider != nil {
		om.logger.Info("Shutting down meter provider")
		err2 = om.meterProvider.Shutdown(ctx)
		if err2 != nil {
			om.logger.Error("Failed to shutdown meter provider", zap.Error(err2))
		} else {
			om.logger.Info("Meter provider shut down successfully")
		}
	}

	if err1 != nil {
		return fmt.Errorf("failed to shutdown tracer provider: %w", err1)
	}
	if err2 != nil {
		return fmt.Errorf("failed to shutdown meter provider: %w", err2)
	}

	om.logger.Info("OpenTelemetry providers shut down successfully")
	return nil
}

// ForceFlush forces all pending traces and metrics to be exported
func (om *OTELManager) ForceFlush(ctx context.Context) error {
	var err1, err2 error
	if om.tracerProvider != nil {
		err1 = om.tracerProvider.ForceFlush(ctx)
	}
	if om.meterProvider != nil {
		err2 = om.meterProvider.ForceFlush(ctx)
	}

	if err1 != nil {
		return fmt.Errorf("failed to flush tracer provider: %w", err1)
	}
	if err2 != nil {
		return fmt.Errorf("failed to flush meter provider: %w", err2)
	}

	return nil
}

// IsEnabled returns whether OpenTelemetry is enabled
func (om *OTELManager) IsEnabled() bool {
	return om.config.Enabled && (om.tracerProvider != nil || om.meterProvider != nil)
}

// Legacy TracingConfig for backward compatibility
type TracingConfig struct {
	Enabled        bool
	OTLPEndpoint   string
	SamplingRatio  float64
	TracingHeaders map[string]string
}

// Legacy TracingManager for backward compatibility
type TracingManager struct {
	otelManager *OTELManager
	logger      *zap.Logger
	config      TracingConfig
}

// NewTracingManager creates a new tracing manager (legacy compatibility)
func NewTracingManager(config TracingConfig, logger *zap.Logger) (*TracingManager, error) {
	otelConfig := OTELConfig{
		Enabled:          config.Enabled,
		Endpoint:         config.OTLPEndpoint,
		ServiceName:      "globeco-allocation-service",
		ServiceVersion:   "1.0.0",
		ServiceNamespace: "globeco",
	}

	otelManager, err := NewOTELManager(otelConfig, logger)
	if err != nil {
		return nil, err
	}

	return &TracingManager{
		otelManager: otelManager,
		logger:      logger,
		config:      config,
	}, nil
}

// Shutdown gracefully shuts down the tracing provider (legacy compatibility)
func (tm *TracingManager) Shutdown(ctx context.Context) error {
	if tm.otelManager != nil {
		return tm.otelManager.Shutdown(ctx)
	}
	return nil
}

// ForceFlush forces all pending traces to be exported (legacy compatibility)
func (tm *TracingManager) ForceFlush(ctx context.Context) error {
	if tm.otelManager != nil {
		return tm.otelManager.ForceFlush(ctx)
	}
	return nil
}

// IsEnabled returns whether tracing is enabled (legacy compatibility)
func (tm *TracingManager) IsEnabled() bool {
	return tm.otelManager != nil && tm.otelManager.IsEnabled()
}
