package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.uber.org/zap"

	"github.com/kasbench/globeco-allocation-service/internal/domain"
)

// ExecutionRepository handles database operations for executions
type ExecutionRepository struct {
	db     *DB
	logger *zap.Logger
}

// NewExecutionRepository creates a new execution repository
func NewExecutionRepository(db *DB, logger *zap.Logger) *ExecutionRepository {
	return &ExecutionRepository{
		db:     db,
		logger: logger,
	}
}

// Create inserts a new execution record
func (r *ExecutionRepository) Create(ctx context.Context, execution *domain.Execution) error {
	// Start OpenTelemetry span for database operation
	tracer := otel.Tracer("globeco-allocation-service")
	ctx, span := tracer.Start(ctx, "db.execution.create")
	defer span.End()

	// Add span attributes
	span.SetAttributes(
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", "INSERT"),
		attribute.String("db.table", "execution"),
		attribute.Int("execution_service_id", execution.ExecutionServiceID),
		attribute.String("trade_type", execution.TradeType),
		attribute.String("destination", execution.Destination),
	)

	query := `
		INSERT INTO execution (
			execution_service_id, is_open, execution_status, trade_type, destination,
			trade_date, security_id, ticker, portfolio_id, quantity, limit_price,
			received_timestamp, sent_timestamp, last_fill_timestamp, quantity_filled,
			total_amount, average_price, ready_to_send_timestamp, version
		) VALUES (
			:execution_service_id, :is_open, :execution_status, :trade_type, :destination,
			:trade_date, :security_id, :ticker, :portfolio_id, :quantity, :limit_price,
			:received_timestamp, :sent_timestamp, :last_fill_timestamp, :quantity_filled,
			:total_amount, :average_price, :ready_to_send_timestamp, :version
		) RETURNING id`

	rows, err := r.db.NamedQueryContext(ctx, query, execution)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "database insert failed")
		r.logger.Error("Failed to create execution with OpenTelemetry tracing", 
			zap.Error(err),
			zap.String("trace_id", span.SpanContext().TraceID().String()),
			zap.String("span_id", span.SpanContext().SpanID().String()))
		return fmt.Errorf("failed to create execution: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			r.logger.Error("failed to close rows", zap.Error(err))
		}
	}()

	if rows.Next() {
		if err := rows.Scan(&execution.ID); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "failed to scan execution ID")
			return fmt.Errorf("failed to scan execution ID: %w", err)
		}
	}

	// Add success attributes
	span.SetAttributes(attribute.Int("execution.id", execution.ID))
	span.SetStatus(codes.Ok, "execution created successfully")

	r.logger.Info("Created execution with OpenTelemetry tracing", 
		zap.Int("id", execution.ID), 
		zap.Int("execution_service_id", execution.ExecutionServiceID),
		zap.String("trace_id", span.SpanContext().TraceID().String()),
		zap.String("span_id", span.SpanContext().SpanID().String()))
	return nil
}

// GetByID retrieves an execution by ID
func (r *ExecutionRepository) GetByID(ctx context.Context, id int) (*domain.Execution, error) {
	// Start OpenTelemetry span for database operation
	tracer := otel.Tracer("globeco-allocation-service")
	ctx, span := tracer.Start(ctx, "db.execution.get_by_id")
	defer span.End()

	// Add span attributes
	span.SetAttributes(
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", "SELECT"),
		attribute.String("db.table", "execution"),
		attribute.Int("execution.id", id),
	)

	var execution domain.Execution
	query := "SELECT * FROM execution WHERE id = $1"

	err := r.db.GetContext(ctx, &execution, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			span.SetStatus(codes.Ok, "execution not found")
			span.SetAttributes(attribute.Bool("found", false))
			return nil, fmt.Errorf("execution not found: %d", id)
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "database select failed")
		r.logger.Error("Failed to get execution by ID with OpenTelemetry tracing", 
			zap.Int("id", id), 
			zap.Error(err),
			zap.String("trace_id", span.SpanContext().TraceID().String()))
		return nil, fmt.Errorf("failed to get execution: %w", err)
	}

	// Add success attributes
	span.SetAttributes(
		attribute.Bool("found", true),
		attribute.Int("execution_service_id", execution.ExecutionServiceID),
		attribute.String("trade_type", execution.TradeType),
	)
	span.SetStatus(codes.Ok, "execution retrieved successfully")

	return &execution, nil
}

// GetByExecutionServiceID retrieves an execution by execution service ID
func (r *ExecutionRepository) GetByExecutionServiceID(ctx context.Context, executionServiceID int) (*domain.Execution, error) {
	var execution domain.Execution
	query := "SELECT * FROM execution WHERE execution_service_id = $1"

	err := r.db.GetContext(ctx, &execution, query, executionServiceID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("execution not found for service ID: %d", executionServiceID)
		}
		r.logger.Error("Failed to get execution by service ID", zap.Int("execution_service_id", executionServiceID), zap.Error(err))
		return nil, fmt.Errorf("failed to get execution: %w", err)
	}

	return &execution, nil
}

// List retrieves executions with pagination
func (r *ExecutionRepository) List(ctx context.Context, limit, offset int) ([]domain.Execution, int, error) {
	var executions []domain.Execution
	var totalCount int

	// Get total count
	countQuery := "SELECT COUNT(*) FROM execution"
	if err := r.db.GetContext(ctx, &totalCount, countQuery); err != nil {
		r.logger.Error("Failed to get execution count", zap.Error(err))
		return nil, 0, fmt.Errorf("failed to get execution count: %w", err)
	}

	// Get executions with pagination
	query := "SELECT * FROM execution ORDER BY id DESC LIMIT $1 OFFSET $2"
	if err := r.db.SelectContext(ctx, &executions, query, limit, offset); err != nil {
		r.logger.Error("Failed to list executions", zap.Error(err))
		return nil, 0, fmt.Errorf("failed to list executions: %w", err)
	}

	return executions, totalCount, nil
}

// GetForBatch retrieves executions ready for batch processing
func (r *ExecutionRepository) GetForBatch(ctx context.Context, startTime, endTime time.Time) ([]domain.Execution, error) {
	var executions []domain.Execution
	query := `
		SELECT * FROM execution 
		WHERE ready_to_send_timestamp >= $1 
		AND ready_to_send_timestamp < $2
		ORDER BY ready_to_send_timestamp ASC`

	if err := r.db.SelectContext(ctx, &executions, query, startTime, endTime); err != nil {
		r.logger.Error("Failed to get executions for batch",
			zap.Time("start_time", startTime),
			zap.Time("end_time", endTime),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get executions for batch: %w", err)
	}

	r.logger.Info("Retrieved executions for batch",
		zap.Int("count", len(executions)),
		zap.Time("start_time", startTime),
		zap.Time("end_time", endTime))

	return executions, nil
}

// Update updates an execution record
func (r *ExecutionRepository) Update(ctx context.Context, execution *domain.Execution) error {
	query := `
		UPDATE execution SET
			is_open = :is_open,
			execution_status = :execution_status,
			trade_type = :trade_type,
			destination = :destination,
			trade_date = :trade_date,
			security_id = :security_id,
			ticker = :ticker,
			portfolio_id = :portfolio_id,
			quantity = :quantity,
			limit_price = :limit_price,
			received_timestamp = :received_timestamp,
			sent_timestamp = :sent_timestamp,
			last_fill_timestamp = :last_fill_timestamp,
			quantity_filled = :quantity_filled,
			total_amount = :total_amount,
			average_price = :average_price,
			ready_to_send_timestamp = :ready_to_send_timestamp,
			version = :version + 1
		WHERE id = :id AND version = :version`

	result, err := r.db.NamedExecContext(ctx, query, execution)
	if err != nil {
		r.logger.Error("Failed to update execution", zap.Int("id", execution.ID), zap.Error(err))
		return fmt.Errorf("failed to update execution: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("execution not found or version conflict: %d", execution.ID)
	}

	execution.Version++
	r.logger.Info("Updated execution", zap.Int("id", execution.ID), zap.Int("version", execution.Version))
	return nil
}

// Delete removes an execution record
func (r *ExecutionRepository) Delete(ctx context.Context, id int) error {
	query := "DELETE FROM execution WHERE id = $1"
	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		r.logger.Error("Failed to delete execution", zap.Int("id", id), zap.Error(err))
		return fmt.Errorf("failed to delete execution: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("execution not found: %d", id)
	}

	r.logger.Info("Deleted execution", zap.Int("id", id))
	return nil
}
