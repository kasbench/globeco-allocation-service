package observability

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

// BusinessMetrics holds all business-related metrics
type BusinessMetrics struct {
	// Execution processing metrics
	ExecutionsBatchProcessed *prometheus.CounterVec
	ExecutionsCreated        *prometheus.CounterVec
	ExecutionsSkipped        *prometheus.CounterVec
	ExecutionsErrored        *prometheus.CounterVec
	ExecutionProcessingTime  *prometheus.HistogramVec

	// Portfolio Accounting metrics
	PortfolioFileGenerated     *prometheus.CounterVec
	PortfolioCLIInvocations    *prometheus.CounterVec
	PortfolioCLIProcessingTime *prometheus.HistogramVec
	PortfolioRecordsProcessed  *prometheus.CounterVec

	// Trade Service metrics
	TradeServiceCalls   *prometheus.CounterVec
	TradeServiceLatency *prometheus.HistogramVec
	TradeServiceRetries *prometheus.CounterVec
	TradeServiceErrors  *prometheus.CounterVec

	// Database metrics
	DatabaseOperations       *prometheus.CounterVec
	DatabaseLatency          *prometheus.HistogramVec
	DatabaseConnections      prometheus.Gauge
	DatabaseConnectionErrors *prometheus.CounterVec

	// Batch processing metrics
	BatchHistoryCreated *prometheus.CounterVec
	BatchProcessingTime *prometheus.HistogramVec
	BatchSize           *prometheus.HistogramVec
	BatchConflicts      *prometheus.CounterVec

	// File operations metrics
	FileOperations        *prometheus.CounterVec
	FileSize              *prometheus.HistogramVec
	FileCleanupOperations *prometheus.CounterVec

	logger *zap.Logger
}

// NewBusinessMetrics creates a new business metrics instance
func NewBusinessMetrics(logger *zap.Logger) *BusinessMetrics {
	return &BusinessMetrics{
		// Execution processing metrics
		ExecutionsBatchProcessed: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "allocations_executions_batch_processed_total",
				Help: "Total number of execution batches processed",
			},
			[]string{"status"},
		),
		ExecutionsCreated: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "allocations_executions_created_total",
				Help: "Total number of executions created",
			},
			[]string{"trade_type", "destination"},
		),
		ExecutionsSkipped: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "allocations_executions_skipped_total",
				Help: "Total number of executions skipped",
			},
			[]string{"reason"},
		),
		ExecutionsErrored: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "allocations_executions_errored_total",
				Help: "Total number of executions that failed",
			},
			[]string{"error_type"},
		),
		ExecutionProcessingTime: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "allocations_execution_processing_duration_seconds",
				Help:    "Time spent processing executions",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"operation"},
		),

		// Portfolio Accounting metrics
		PortfolioFileGenerated: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "allocations_portfolio_files_generated_total",
				Help: "Total number of portfolio accounting files generated",
			},
			[]string{"status"},
		),
		PortfolioCLIInvocations: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "allocations_portfolio_cli_invocations_total",
				Help: "Total number of Portfolio Accounting CLI invocations",
			},
			[]string{"status"},
		),
		PortfolioCLIProcessingTime: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "allocations_portfolio_cli_processing_duration_seconds",
				Help:    "Time spent processing Portfolio Accounting CLI",
				Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 120, 300},
			},
			[]string{"command_type"},
		),
		PortfolioRecordsProcessed: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "allocations_portfolio_records_processed_total",
				Help: "Total number of records processed for portfolio accounting",
			},
			[]string{"status"},
		),

		// Trade Service metrics
		TradeServiceCalls: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "allocations_trade_service_calls_total",
				Help: "Total number of Trade Service API calls",
			},
			[]string{"method", "status"},
		),
		TradeServiceLatency: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "allocations_trade_service_latency_seconds",
				Help:    "Latency of Trade Service API calls",
				Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
			},
			[]string{"method"},
		),
		TradeServiceRetries: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "allocations_trade_service_retries_total",
				Help: "Total number of Trade Service API retries",
			},
			[]string{"method", "attempt"},
		),
		TradeServiceErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "allocations_trade_service_errors_total",
				Help: "Total number of Trade Service API errors",
			},
			[]string{"method", "error_type"},
		),

		// Database metrics
		DatabaseOperations: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "allocations_database_operations_total",
				Help: "Total number of database operations",
			},
			[]string{"operation", "table", "status"},
		),
		DatabaseLatency: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "allocations_database_operation_duration_seconds",
				Help:    "Time spent on database operations",
				Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
			},
			[]string{"operation", "table"},
		),
		DatabaseConnections: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "allocations_database_connections_active",
				Help: "Number of active database connections",
			},
		),
		DatabaseConnectionErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "allocations_database_connection_errors_total",
				Help: "Total number of database connection errors",
			},
			[]string{"error_type"},
		),

		// Batch processing metrics
		BatchHistoryCreated: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "allocations_batch_history_created_total",
				Help: "Total number of batch history records created",
			},
			[]string{"status"},
		),
		BatchProcessingTime: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "allocations_batch_processing_duration_seconds",
				Help:    "Time spent processing batches",
				Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 120, 300},
			},
			[]string{"operation"},
		),
		BatchSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "allocations_batch_size",
				Help:    "Size of processed batches",
				Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000},
			},
			[]string{"operation"},
		),
		BatchConflicts: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "allocations_batch_conflicts_total",
				Help: "Total number of batch processing conflicts",
			},
			[]string{"conflict_type"},
		),

		// File operations metrics
		FileOperations: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "allocations_file_operations_total",
				Help: "Total number of file operations",
			},
			[]string{"operation", "status"},
		),
		FileSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "allocations_file_size_bytes",
				Help:    "Size of generated files",
				Buckets: []float64{1024, 10240, 102400, 1048576, 10485760, 104857600}, // 1KB to 100MB
			},
			[]string{"file_type"},
		),
		FileCleanupOperations: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "allocations_file_cleanup_operations_total",
				Help: "Total number of file cleanup operations",
			},
			[]string{"status"},
		),

		logger: logger,
	}
}

// Helper methods for recording metrics

// RecordExecutionBatch records execution batch processing metrics
func (m *BusinessMetrics) RecordExecutionBatch(status string, batchSize int, duration time.Duration) {
	m.ExecutionsBatchProcessed.WithLabelValues(status).Inc()
	m.BatchSize.WithLabelValues("execution_batch").Observe(float64(batchSize))
	m.BatchProcessingTime.WithLabelValues("execution_batch").Observe(duration.Seconds())
}

// RecordExecutionCreated records execution creation metrics
func (m *BusinessMetrics) RecordExecutionCreated(tradeType, destination string) {
	m.ExecutionsCreated.WithLabelValues(tradeType, destination).Inc()
}

// RecordExecutionSkipped records execution skipping metrics
func (m *BusinessMetrics) RecordExecutionSkipped(reason string) {
	m.ExecutionsSkipped.WithLabelValues(reason).Inc()
}

// RecordExecutionError records execution error metrics
func (m *BusinessMetrics) RecordExecutionError(errorType string) {
	m.ExecutionsErrored.WithLabelValues(errorType).Inc()
}

// RecordTradeServiceCall records Trade Service API call metrics
func (m *BusinessMetrics) RecordTradeServiceCall(method, status string, duration time.Duration) {
	m.TradeServiceCalls.WithLabelValues(method, status).Inc()
	m.TradeServiceLatency.WithLabelValues(method).Observe(duration.Seconds())
}

// RecordTradeServiceRetry records Trade Service retry metrics
func (m *BusinessMetrics) RecordTradeServiceRetry(method string, attempt int) {
	m.TradeServiceRetries.WithLabelValues(method, strconv.Itoa(attempt)).Inc()
}

// RecordTradeServiceError records Trade Service error metrics
func (m *BusinessMetrics) RecordTradeServiceError(method, errorType string) {
	m.TradeServiceErrors.WithLabelValues(method, errorType).Inc()
}

// RecordDatabaseOperation records database operation metrics
func (m *BusinessMetrics) RecordDatabaseOperation(operation, table, status string, duration time.Duration) {
	m.DatabaseOperations.WithLabelValues(operation, table, status).Inc()
	m.DatabaseLatency.WithLabelValues(operation, table).Observe(duration.Seconds())
}

// RecordDatabaseConnections records active database connections
func (m *BusinessMetrics) RecordDatabaseConnections(count int) {
	m.DatabaseConnections.Set(float64(count))
}

// RecordPortfolioFileGenerated records portfolio file generation metrics
func (m *BusinessMetrics) RecordPortfolioFileGenerated(status string, fileSize int64) {
	m.PortfolioFileGenerated.WithLabelValues(status).Inc()
	m.FileSize.WithLabelValues("portfolio").Observe(float64(fileSize))
}

// RecordPortfolioCLIInvocation records Portfolio CLI invocation metrics
func (m *BusinessMetrics) RecordPortfolioCLIInvocation(status string, duration time.Duration, recordCount int) {
	m.PortfolioCLIInvocations.WithLabelValues(status).Inc()
	m.PortfolioCLIProcessingTime.WithLabelValues("cli").Observe(duration.Seconds())
	m.PortfolioRecordsProcessed.WithLabelValues(status).Add(float64(recordCount))
}

// RecordBatchHistory records batch history creation metrics
func (m *BusinessMetrics) RecordBatchHistory(status string) {
	m.BatchHistoryCreated.WithLabelValues(status).Inc()
}

// RecordBatchConflict records batch conflict metrics
func (m *BusinessMetrics) RecordBatchConflict(conflictType string) {
	m.BatchConflicts.WithLabelValues(conflictType).Inc()
}

// RecordFileOperation records file operation metrics
func (m *BusinessMetrics) RecordFileOperation(operation, status string) {
	m.FileOperations.WithLabelValues(operation, status).Inc()
}

// RecordFileCleanup records file cleanup metrics
func (m *BusinessMetrics) RecordFileCleanup(status string) {
	m.FileCleanupOperations.WithLabelValues(status).Inc()
}
