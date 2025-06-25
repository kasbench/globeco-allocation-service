package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

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
		r.logger.Error("Failed to create execution", zap.Error(err))
		return fmt.Errorf("failed to create execution: %w", err)
	}
	defer rows.Close()

	if rows.Next() {
		if err := rows.Scan(&execution.ID); err != nil {
			return fmt.Errorf("failed to scan execution ID: %w", err)
		}
	}

	r.logger.Info("Created execution", zap.Int("id", execution.ID), zap.Int("execution_service_id", execution.ExecutionServiceID))
	return nil
}

// GetByID retrieves an execution by ID
func (r *ExecutionRepository) GetByID(ctx context.Context, id int) (*domain.Execution, error) {
	var execution domain.Execution
	query := "SELECT * FROM execution WHERE id = $1"

	err := r.db.GetContext(ctx, &execution, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("execution not found: %d", id)
		}
		r.logger.Error("Failed to get execution by ID", zap.Int("id", id), zap.Error(err))
		return nil, fmt.Errorf("failed to get execution: %w", err)
	}

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
