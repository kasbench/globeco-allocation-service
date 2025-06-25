package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kasbench/globeco-allocation-service/internal/domain"
)

// BatchHistoryRepository handles database operations for batch history
type BatchHistoryRepository struct {
	db     *DB
	logger *zap.Logger
}

// NewBatchHistoryRepository creates a new batch history repository
func NewBatchHistoryRepository(db *DB, logger *zap.Logger) *BatchHistoryRepository {
	return &BatchHistoryRepository{
		db:     db,
		logger: logger,
	}
}

// GetMaxStartTime retrieves the maximum start time from batch history
func (r *BatchHistoryRepository) GetMaxStartTime(ctx context.Context) (time.Time, error) {
	var maxTime sql.NullTime
	query := "SELECT MAX(start_time) FROM batch_history"

	err := r.db.GetContext(ctx, &maxTime, query)
	if err != nil {
		r.logger.Error("Failed to get max start time", zap.Error(err))
		return time.Time{}, fmt.Errorf("failed to get max start time: %w", err)
	}

	// If no records exist, return zero time
	if !maxTime.Valid {
		return time.Time{}, nil
	}

	return maxTime.Time, nil
}

// Create inserts a new batch history record
func (r *BatchHistoryRepository) Create(ctx context.Context, batchHistory *domain.BatchHistory) error {
	query := `
		INSERT INTO batch_history (start_time, previous_start_time, version) 
		VALUES (:start_time, :previous_start_time, :version) 
		RETURNING id`

	rows, err := r.db.NamedQueryContext(ctx, query, batchHistory)
	if err != nil {
		r.logger.Error("Failed to create batch history", zap.Error(err))
		return fmt.Errorf("failed to create batch history: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			r.logger.Error("failed to close rows", zap.Error(err))
		}
	}()

	if rows.Next() {
		if err := rows.Scan(&batchHistory.ID); err != nil {
			return fmt.Errorf("failed to scan batch history ID: %w", err)
		}
	}

	r.logger.Info("Created batch history",
		zap.Int("id", batchHistory.ID),
		zap.Time("start_time", batchHistory.StartTime),
		zap.Time("previous_start_time", batchHistory.PreviousStartTime))

	return nil
}

// GetByID retrieves a batch history record by ID
func (r *BatchHistoryRepository) GetByID(ctx context.Context, id int) (*domain.BatchHistory, error) {
	var batchHistory domain.BatchHistory
	query := "SELECT * FROM batch_history WHERE id = $1"

	err := r.db.GetContext(ctx, &batchHistory, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("batch history not found: %d", id)
		}
		r.logger.Error("Failed to get batch history by ID", zap.Int("id", id), zap.Error(err))
		return nil, fmt.Errorf("failed to get batch history: %w", err)
	}

	return &batchHistory, nil
}

// List retrieves batch history records with pagination
func (r *BatchHistoryRepository) List(ctx context.Context, limit, offset int) ([]domain.BatchHistory, int, error) {
	var batches []domain.BatchHistory
	var totalCount int

	// Get total count
	countQuery := "SELECT COUNT(*) FROM batch_history"
	if err := r.db.GetContext(ctx, &totalCount, countQuery); err != nil {
		r.logger.Error("Failed to get batch history count", zap.Error(err))
		return nil, 0, fmt.Errorf("failed to get batch history count: %w", err)
	}

	// Get batch history with pagination
	query := "SELECT * FROM batch_history ORDER BY start_time DESC LIMIT $1 OFFSET $2"
	if err := r.db.SelectContext(ctx, &batches, query, limit, offset); err != nil {
		r.logger.Error("Failed to list batch history", zap.Error(err))
		return nil, 0, fmt.Errorf("failed to list batch history: %w", err)
	}

	return batches, totalCount, nil
}

// GetLatest retrieves the most recent batch history record
func (r *BatchHistoryRepository) GetLatest(ctx context.Context) (*domain.BatchHistory, error) {
	var batchHistory domain.BatchHistory
	query := "SELECT * FROM batch_history ORDER BY start_time DESC LIMIT 1"

	err := r.db.GetContext(ctx, &batchHistory, query)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no batch history found")
		}
		r.logger.Error("Failed to get latest batch history", zap.Error(err))
		return nil, fmt.Errorf("failed to get latest batch history: %w", err)
	}

	return &batchHistory, nil
}

// Update updates a batch history record
func (r *BatchHistoryRepository) Update(ctx context.Context, batchHistory *domain.BatchHistory) error {
	query := `
		UPDATE batch_history SET
			start_time = :start_time,
			previous_start_time = :previous_start_time,
			version = :version + 1
		WHERE id = :id AND version = :version`

	result, err := r.db.NamedExecContext(ctx, query, batchHistory)
	if err != nil {
		r.logger.Error("Failed to update batch history", zap.Int("id", batchHistory.ID), zap.Error(err))
		return fmt.Errorf("failed to update batch history: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("batch history not found or version conflict: %d", batchHistory.ID)
	}

	batchHistory.Version++
	r.logger.Info("Updated batch history", zap.Int("id", batchHistory.ID), zap.Int("version", batchHistory.Version))
	return nil
}

// Delete removes a batch history record
func (r *BatchHistoryRepository) Delete(ctx context.Context, id int) error {
	query := "DELETE FROM batch_history WHERE id = $1"
	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		r.logger.Error("Failed to delete batch history", zap.Int("id", id), zap.Error(err))
		return fmt.Errorf("failed to delete batch history: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("batch history not found: %d", id)
	}

	r.logger.Info("Deleted batch history", zap.Int("id", id))
	return nil
}
