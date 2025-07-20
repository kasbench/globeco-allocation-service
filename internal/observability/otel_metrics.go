package observability

import (
	"context"
	"runtime"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
)

// OTELMetricsManager manages OpenTelemetry metrics including Go runtime metrics
type OTELMetricsManager struct {
	meter  metric.Meter
	logger *zap.Logger

	// Go runtime metrics
	goGoroutines      metric.Int64ObservableGauge
	goMemoryHeapAlloc metric.Int64ObservableGauge
	goMemoryHeapSys   metric.Int64ObservableGauge
	goMemoryStackSys  metric.Int64ObservableGauge
	goGCCount         metric.Int64ObservableCounter
	goGCPauseTime     metric.Float64ObservableGauge

	// HTTP metrics
	httpRequestsTotal    metric.Int64Counter
	httpRequestDuration  metric.Float64Histogram
	httpRequestsInFlight metric.Int64UpDownCounter

	// Database metrics
	dbOperationsTotal    metric.Int64Counter
	dbOperationDuration  metric.Float64Histogram
	dbConnectionsActive  metric.Int64UpDownCounter

	// Trade Service metrics
	tradeServiceCallsTotal    metric.Int64Counter
	tradeServiceCallDuration  metric.Float64Histogram
	tradeServiceRetries       metric.Int64Counter

	// Business metrics
	executionsCreated     metric.Int64Counter
	executionsProcessed   metric.Int64Counter
	batchProcessingTime   metric.Float64Histogram
	portfolioFilesGenerated metric.Int64Counter
}

// NewOTELMetricsManager creates a new OpenTelemetry metrics manager
func NewOTELMetricsManager(logger *zap.Logger) (*OTELMetricsManager, error) {
	meter := otel.Meter("globeco-allocation-service")

	manager := &OTELMetricsManager{
		meter:  meter,
		logger: logger,
	}

	if err := manager.initializeMetrics(); err != nil {
		return nil, err
	}

	logger.Info("OpenTelemetry metrics manager initialized with Go runtime metrics")
	return manager, nil
}

// initializeMetrics creates all the metric instruments
func (m *OTELMetricsManager) initializeMetrics() error {
	var err error

	// Go runtime metrics
	m.goGoroutines, err = m.meter.Int64ObservableGauge(
		"go_goroutines",
		metric.WithDescription("Number of goroutines that currently exist"),
	)
	if err != nil {
		return err
	}

	m.goMemoryHeapAlloc, err = m.meter.Int64ObservableGauge(
		"go_memory_heap_alloc_bytes",
		metric.WithDescription("Number of heap bytes allocated and still in use"),
	)
	if err != nil {
		return err
	}

	m.goMemoryHeapSys, err = m.meter.Int64ObservableGauge(
		"go_memory_heap_sys_bytes",
		metric.WithDescription("Number of heap bytes obtained from system"),
	)
	if err != nil {
		return err
	}

	m.goMemoryStackSys, err = m.meter.Int64ObservableGauge(
		"go_memory_stack_sys_bytes",
		metric.WithDescription("Number of stack bytes obtained from system"),
	)
	if err != nil {
		return err
	}

	m.goGCCount, err = m.meter.Int64ObservableCounter(
		"go_gc_runs_total",
		metric.WithDescription("Total number of GC runs"),
	)
	if err != nil {
		return err
	}

	m.goGCPauseTime, err = m.meter.Float64ObservableGauge(
		"go_gc_pause_seconds",
		metric.WithDescription("Time spent in GC pause"),
	)
	if err != nil {
		return err
	}

	// HTTP metrics
	m.httpRequestsTotal, err = m.meter.Int64Counter(
		"http_requests_total",
		metric.WithDescription("Total number of HTTP requests"),
	)
	if err != nil {
		return err
	}

	m.httpRequestDuration, err = m.meter.Float64Histogram(
		"http_request_duration_seconds",
		metric.WithDescription("Duration of HTTP requests"),
		metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10),
	)
	if err != nil {
		return err
	}

	m.httpRequestsInFlight, err = m.meter.Int64UpDownCounter(
		"http_requests_in_flight",
		metric.WithDescription("Number of HTTP requests currently being processed"),
	)
	if err != nil {
		return err
	}

	// Database metrics
	m.dbOperationsTotal, err = m.meter.Int64Counter(
		"db_operations_total",
		metric.WithDescription("Total number of database operations"),
	)
	if err != nil {
		return err
	}

	m.dbOperationDuration, err = m.meter.Float64Histogram(
		"db_operation_duration_seconds",
		metric.WithDescription("Duration of database operations"),
		metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5),
	)
	if err != nil {
		return err
	}

	m.dbConnectionsActive, err = m.meter.Int64UpDownCounter(
		"db_connections_active",
		metric.WithDescription("Number of active database connections"),
	)
	if err != nil {
		return err
	}

	// Trade Service metrics
	m.tradeServiceCallsTotal, err = m.meter.Int64Counter(
		"trade_service_calls_total",
		metric.WithDescription("Total number of Trade Service API calls"),
	)
	if err != nil {
		return err
	}

	m.tradeServiceCallDuration, err = m.meter.Float64Histogram(
		"trade_service_call_duration_seconds",
		metric.WithDescription("Duration of Trade Service API calls"),
		metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10),
	)
	if err != nil {
		return err
	}

	m.tradeServiceRetries, err = m.meter.Int64Counter(
		"trade_service_retries_total",
		metric.WithDescription("Total number of Trade Service API retries"),
	)
	if err != nil {
		return err
	}

	// Business metrics
	m.executionsCreated, err = m.meter.Int64Counter(
		"executions_created_total",
		metric.WithDescription("Total number of executions created"),
	)
	if err != nil {
		return err
	}

	m.executionsProcessed, err = m.meter.Int64Counter(
		"executions_processed_total",
		metric.WithDescription("Total number of executions processed"),
	)
	if err != nil {
		return err
	}

	m.batchProcessingTime, err = m.meter.Float64Histogram(
		"batch_processing_duration_seconds",
		metric.WithDescription("Duration of batch processing operations"),
		metric.WithExplicitBucketBoundaries(0.1, 0.5, 1, 2, 5, 10, 30, 60, 120, 300),
	)
	if err != nil {
		return err
	}

	m.portfolioFilesGenerated, err = m.meter.Int64Counter(
		"portfolio_files_generated_total",
		metric.WithDescription("Total number of portfolio files generated"),
	)
	if err != nil {
		return err
	}

	// Register callback for Go runtime metrics
	_, err = m.meter.RegisterCallback(
		m.collectGoRuntimeMetrics,
		m.goGoroutines,
		m.goMemoryHeapAlloc,
		m.goMemoryHeapSys,
		m.goMemoryStackSys,
		m.goGCCount,
		m.goGCPauseTime,
	)
	if err != nil {
		return err
	}

	return nil
}

// collectGoRuntimeMetrics collects Go runtime metrics
func (m *OTELMetricsManager) collectGoRuntimeMetrics(ctx context.Context, observer metric.Observer) error {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	observer.ObserveInt64(m.goGoroutines, int64(runtime.NumGoroutine()))
	observer.ObserveInt64(m.goMemoryHeapAlloc, int64(memStats.HeapAlloc))
	observer.ObserveInt64(m.goMemoryHeapSys, int64(memStats.HeapSys))
	observer.ObserveInt64(m.goMemoryStackSys, int64(memStats.StackSys))
	observer.ObserveInt64(m.goGCCount, int64(memStats.NumGC))
	observer.ObserveFloat64(m.goGCPauseTime, float64(memStats.PauseNs[(memStats.NumGC+255)%256])/1e9)

	return nil
}

// RecordHTTPRequest records HTTP request metrics
func (m *OTELMetricsManager) RecordHTTPRequest(ctx context.Context, method, path, status string, duration time.Duration) {
	m.httpRequestsTotal.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("method", method),
			attribute.String("path", path),
			attribute.String("status", status),
		))

	m.httpRequestDuration.Record(ctx, duration.Seconds(),
		metric.WithAttributes(
			attribute.String("method", method),
			attribute.String("path", path),
			attribute.String("status", status),
		))

	m.logger.Info("Recorded HTTP request metrics to OpenTelemetry collector",
		zap.String("method", method),
		zap.String("path", path),
		zap.String("status", status),
		zap.Duration("duration", duration))
}

// RecordHTTPRequestStart records the start of an HTTP request
func (m *OTELMetricsManager) RecordHTTPRequestStart(ctx context.Context) {
	m.httpRequestsInFlight.Add(ctx, 1)
}

// RecordHTTPRequestEnd records the end of an HTTP request
func (m *OTELMetricsManager) RecordHTTPRequestEnd(ctx context.Context) {
	m.httpRequestsInFlight.Add(ctx, -1)
}

// RecordDatabaseOperation records database operation metrics
func (m *OTELMetricsManager) RecordDatabaseOperation(ctx context.Context, operation, table, status string, duration time.Duration) {
	m.dbOperationsTotal.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("operation", operation),
			attribute.String("table", table),
			attribute.String("status", status),
		))

	m.dbOperationDuration.Record(ctx, duration.Seconds(),
		metric.WithAttributes(
			attribute.String("operation", operation),
			attribute.String("table", table),
		))

	m.logger.Info("Recorded database operation metrics to OpenTelemetry collector",
		zap.String("operation", operation),
		zap.String("table", table),
		zap.String("status", status),
		zap.Duration("duration", duration))
}

// RecordTradeServiceCall records Trade Service API call metrics
func (m *OTELMetricsManager) RecordTradeServiceCall(ctx context.Context, method, status string, duration time.Duration) {
	m.tradeServiceCallsTotal.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("method", method),
			attribute.String("status", status),
		))

	m.tradeServiceCallDuration.Record(ctx, duration.Seconds(),
		metric.WithAttributes(
			attribute.String("method", method),
		))

	m.logger.Info("Recorded Trade Service call metrics to OpenTelemetry collector",
		zap.String("method", method),
		zap.String("status", status),
		zap.Duration("duration", duration))
}

// RecordTradeServiceRetry records Trade Service retry metrics
func (m *OTELMetricsManager) RecordTradeServiceRetry(ctx context.Context, method string, attempt int) {
	m.tradeServiceRetries.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("method", method),
			attribute.Int("attempt", attempt),
		))

	m.logger.Info("Recorded Trade Service retry metrics to OpenTelemetry collector",
		zap.String("method", method),
		zap.Int("attempt", attempt))
}

// RecordExecutionCreated records execution creation metrics
func (m *OTELMetricsManager) RecordExecutionCreated(ctx context.Context, tradeType, destination string) {
	m.executionsCreated.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("trade_type", tradeType),
			attribute.String("destination", destination),
		))

	m.logger.Info("Recorded execution creation metrics to OpenTelemetry collector",
		zap.String("trade_type", tradeType),
		zap.String("destination", destination))
}

// RecordExecutionProcessed records execution processing metrics
func (m *OTELMetricsManager) RecordExecutionProcessed(ctx context.Context, status string, count int) {
	m.executionsProcessed.Add(ctx, int64(count),
		metric.WithAttributes(
			attribute.String("status", status),
		))

	m.logger.Info("Recorded execution processing metrics to OpenTelemetry collector",
		zap.String("status", status),
		zap.Int("count", count))
}

// RecordBatchProcessing records batch processing metrics
func (m *OTELMetricsManager) RecordBatchProcessing(ctx context.Context, operation string, duration time.Duration, batchSize int) {
	m.batchProcessingTime.Record(ctx, duration.Seconds(),
		metric.WithAttributes(
			attribute.String("operation", operation),
		))

	m.logger.Info("Recorded batch processing metrics to OpenTelemetry collector",
		zap.String("operation", operation),
		zap.Duration("duration", duration),
		zap.Int("batch_size", batchSize))
}

// RecordPortfolioFileGenerated records portfolio file generation metrics
func (m *OTELMetricsManager) RecordPortfolioFileGenerated(ctx context.Context, status string) {
	m.portfolioFilesGenerated.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("status", status),
		))

	m.logger.Info("Recorded portfolio file generation metrics to OpenTelemetry collector",
		zap.String("status", status))
}