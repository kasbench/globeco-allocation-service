package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/kasbench/globeco-allocation-service/internal/domain"
)

// Test error constants
var (
	ErrExecutionNotFound  = errors.New("execution not found")
	ErrDuplicateExecution = errors.New("duplicate execution")
)

func TestExecutionRepository_Create(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		err := db.Close()
		require.NoError(t, err)
	}()

	sqlxDB := sqlx.NewDb(db, "postgres")
	dbWrapper := &DB{DB: sqlxDB, logger: zap.NewNop()}
	repo := NewExecutionRepository(dbWrapper, zap.NewNop())

	ctx := context.Background()
	now := time.Now()
	execution := &domain.Execution{
		ExecutionServiceID:   123,
		IsOpen:               false,
		ExecutionStatus:      "FILLED",
		TradeType:            "BUY",
		Destination:          "NYSE",
		TradeDate:            time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		SecurityID:           "12345678901234567890ABCD",
		Ticker:               "AAPL",
		PortfolioID:          nil,
		Quantity:             100.5,
		LimitPrice:           nil,
		ReceivedTimestamp:    now,
		SentTimestamp:        now.Add(30 * time.Second),
		LastFillTimestamp:    nil,
		QuantityFilled:       100.5,
		TotalAmount:          15000.0,
		AveragePrice:         149.25,
		ReadyToSendTimestamp: now,
		Version:              1,
	}

	mock.ExpectQuery(`INSERT INTO execution`).
		WithArgs(
			execution.ExecutionServiceID,
			execution.IsOpen,
			execution.ExecutionStatus,
			execution.TradeType,
			execution.Destination,
			execution.TradeDate,
			execution.SecurityID,
			execution.Ticker,
			execution.PortfolioID,
			execution.Quantity,
			execution.LimitPrice,
			execution.ReceivedTimestamp,
			execution.SentTimestamp,
			execution.LastFillTimestamp,
			execution.QuantityFilled,
			execution.TotalAmount,
			execution.AveragePrice,
			execution.ReadyToSendTimestamp,
			execution.Version,
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	err = repo.Create(ctx, execution)

	assert.NoError(t, err)
	assert.Equal(t, 1, execution.ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestExecutionRepository_Create_Error(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		err := db.Close()
		require.NoError(t, err)
	}()

	sqlxDB := sqlx.NewDb(db, "postgres")
	dbWrapper := &DB{DB: sqlxDB, logger: zap.NewNop()}
	repo := NewExecutionRepository(dbWrapper, zap.NewNop())

	ctx := context.Background()
	now := time.Now()
	execution := &domain.Execution{
		ExecutionServiceID:   123,
		IsOpen:               false,
		ExecutionStatus:      "FILLED",
		TradeType:            "BUY",
		Destination:          "NYSE",
		TradeDate:            time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		SecurityID:           "12345678901234567890ABCD",
		Ticker:               "AAPL",
		PortfolioID:          nil,
		Quantity:             100.5,
		LimitPrice:           nil,
		ReceivedTimestamp:    now,
		SentTimestamp:        now.Add(30 * time.Second),
		LastFillTimestamp:    nil,
		QuantityFilled:       100.5,
		TotalAmount:          15000.0,
		AveragePrice:         149.25,
		ReadyToSendTimestamp: now,
		Version:              1,
	}

	mock.ExpectQuery(`INSERT INTO execution`).
		WillReturnError(errors.New("database error"))

	err = repo.Create(ctx, execution)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create execution")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestExecutionRepository_GetByID(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		err := db.Close()
		require.NoError(t, err)
	}()

	sqlxDB := sqlx.NewDb(db, "postgres")
	dbWrapper := &DB{DB: sqlxDB, logger: zap.NewNop()}
	repo := NewExecutionRepository(dbWrapper, zap.NewNop())

	ctx := context.Background()
	now := time.Now()
	portfolioID := "PORTFOLIO123456789012"
	limitPrice := 150.0

	rows := sqlmock.NewRows([]string{
		"id", "execution_service_id", "is_open", "execution_status", "trade_type",
		"destination", "trade_date", "security_id", "ticker", "portfolio_id",
		"quantity", "limit_price", "received_timestamp", "sent_timestamp",
		"last_fill_timestamp", "quantity_filled", "total_amount", "average_price",
		"ready_to_send_timestamp", "version",
	}).AddRow(
		1, 123, false, "FILLED", "BUY",
		"NYSE", time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), "12345678901234567890ABCD", "AAPL", portfolioID,
		100.5, limitPrice, now, now.Add(30*time.Second),
		now.Add(1*time.Hour), 100.5, 15000.0, 149.25,
		now, 1,
	)

	mock.ExpectQuery(`SELECT \* FROM execution WHERE id = \$1`).
		WithArgs(1).
		WillReturnRows(rows)

	execution, err := repo.GetByID(ctx, 1)

	assert.NoError(t, err)
	assert.NotNil(t, execution)
	assert.Equal(t, 1, execution.ID)
	assert.Equal(t, 123, execution.ExecutionServiceID)
	assert.Equal(t, "FILLED", execution.ExecutionStatus)
	assert.Equal(t, "BUY", execution.TradeType)
	assert.Equal(t, "NYSE", execution.Destination)
	assert.Equal(t, "AAPL", execution.Ticker)
	assert.Equal(t, &portfolioID, execution.PortfolioID)
	assert.Equal(t, 100.5, execution.Quantity)
	assert.Equal(t, &limitPrice, execution.LimitPrice)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestExecutionRepository_GetByID_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		err := db.Close()
		require.NoError(t, err)
	}()

	sqlxDB := sqlx.NewDb(db, "postgres")
	dbWrapper := &DB{DB: sqlxDB, logger: zap.NewNop()}
	repo := NewExecutionRepository(dbWrapper, zap.NewNop())

	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM execution WHERE id = \$1`).
		WithArgs(999).
		WillReturnError(sql.ErrNoRows)

	execution, err := repo.GetByID(ctx, 999)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "execution not found")
	assert.Nil(t, execution)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestExecutionRepository_List(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		err := db.Close()
		require.NoError(t, err)
	}()

	sqlxDB := sqlx.NewDb(db, "postgres")
	dbWrapper := &DB{DB: sqlxDB, logger: zap.NewNop()}
	repo := NewExecutionRepository(dbWrapper, zap.NewNop())

	ctx := context.Background()
	now := time.Now()

	// Mock count query
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM execution`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	// Mock data query
	rows := sqlmock.NewRows([]string{
		"id", "execution_service_id", "is_open", "execution_status", "trade_type",
		"destination", "trade_date", "security_id", "ticker", "portfolio_id",
		"quantity", "limit_price", "received_timestamp", "sent_timestamp",
		"last_fill_timestamp", "quantity_filled", "total_amount", "average_price",
		"ready_to_send_timestamp", "version",
	}).
		AddRow(
			1, 123, false, "FILLED", "BUY",
			"NYSE", time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), "12345678901234567890ABCD", "AAPL", nil,
			100.5, nil, now, now.Add(30*time.Second),
			nil, 100.5, 15000.0, 149.25,
			now, 1,
		).
		AddRow(
			2, 124, false, "FILLED", "SELL",
			"NASDAQ", time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), "ABCDEFGHIJKLMNOPQRSTUVWX", "MSFT", nil,
			50.0, nil, now, now.Add(45*time.Second),
			nil, 50.0, 10000.0, 200.0,
			now, 1,
		)

	mock.ExpectQuery(`SELECT \* FROM execution ORDER BY id DESC LIMIT \$1 OFFSET \$2`).
		WithArgs(50, 0).
		WillReturnRows(rows)

	executions, totalCount, err := repo.List(ctx, 50, 0)

	assert.NoError(t, err)
	assert.Len(t, executions, 2)
	assert.Equal(t, 2, totalCount)

	// Verify first execution
	assert.Equal(t, 1, executions[0].ID)
	assert.Equal(t, 123, executions[0].ExecutionServiceID)
	assert.Equal(t, "BUY", executions[0].TradeType)

	// Verify second execution
	assert.Equal(t, 2, executions[1].ID)
	assert.Equal(t, 124, executions[1].ExecutionServiceID)
	assert.Equal(t, "SELL", executions[1].TradeType)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestExecutionRepository_Update(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		err := db.Close()
		require.NoError(t, err)
	}()

	sqlxDB := sqlx.NewDb(db, "postgres")
	dbWrapper := &DB{DB: sqlxDB, logger: zap.NewNop()}
	repo := NewExecutionRepository(dbWrapper, zap.NewNop())

	ctx := context.Background()
	now := time.Now()
	portfolioID := "PORTFOLIO123456789012"
	execution := &domain.Execution{
		ID:                   1,
		ExecutionServiceID:   123,
		IsOpen:               false,
		ExecutionStatus:      "FILLED",
		TradeType:            "BUY",
		Destination:          "NYSE",
		TradeDate:            time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		SecurityID:           "12345678901234567890ABCD",
		Ticker:               "AAPL",
		PortfolioID:          &portfolioID,
		Quantity:             100.5,
		LimitPrice:           nil,
		ReceivedTimestamp:    now,
		SentTimestamp:        now.Add(30 * time.Second),
		LastFillTimestamp:    nil,
		QuantityFilled:       100.5,
		TotalAmount:          15000.0,
		AveragePrice:         149.25,
		ReadyToSendTimestamp: now,
		Version:              1,
	}

	mock.ExpectExec(`UPDATE execution SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = repo.Update(ctx, execution)

	assert.NoError(t, err)
	assert.Equal(t, 2, execution.Version) // Version should be incremented
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestExecutionRepository_Update_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		err := db.Close()
		require.NoError(t, err)
	}()

	sqlxDB := sqlx.NewDb(db, "postgres")
	dbWrapper := &DB{DB: sqlxDB, logger: zap.NewNop()}
	repo := NewExecutionRepository(dbWrapper, zap.NewNop())

	ctx := context.Background()
	now := time.Now()
	execution := &domain.Execution{
		ID:                   999,
		ExecutionServiceID:   123,
		IsOpen:               false,
		ExecutionStatus:      "FILLED",
		TradeType:            "BUY",
		Destination:          "NYSE",
		TradeDate:            time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		SecurityID:           "12345678901234567890ABCD",
		Ticker:               "AAPL",
		PortfolioID:          nil,
		Quantity:             100.5,
		LimitPrice:           nil,
		ReceivedTimestamp:    now,
		SentTimestamp:        now.Add(30 * time.Second),
		LastFillTimestamp:    nil,
		QuantityFilled:       100.5,
		TotalAmount:          15000.0,
		AveragePrice:         149.25,
		ReadyToSendTimestamp: now,
		Version:              1,
	}

	mock.ExpectExec(`UPDATE execution SET`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = repo.Update(ctx, execution)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "execution not found or version conflict")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestExecutionRepository_GetForBatch(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		err := db.Close()
		require.NoError(t, err)
	}()

	sqlxDB := sqlx.NewDb(db, "postgres")
	dbWrapper := &DB{DB: sqlxDB, logger: zap.NewNop()}
	repo := NewExecutionRepository(dbWrapper, zap.NewNop())

	ctx := context.Background()
	now := time.Now()
	startTime := now.Add(-1 * time.Hour)
	endTime := now

	rows := sqlmock.NewRows([]string{
		"id", "execution_service_id", "is_open", "execution_status", "trade_type",
		"destination", "trade_date", "security_id", "ticker", "portfolio_id",
		"quantity", "limit_price", "received_timestamp", "sent_timestamp",
		"last_fill_timestamp", "quantity_filled", "total_amount", "average_price",
		"ready_to_send_timestamp", "version",
	}).AddRow(
		1, 123, false, "FILLED", "BUY",
		"NYSE", time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), "12345678901234567890ABCD", "AAPL", "PORTFOLIO123456789012",
		100.5, 150.0, now, now.Add(30*time.Second),
		now.Add(1*time.Hour), 100.5, 15000.0, 149.25,
		now.Add(-30*time.Minute), 1,
	)

	mock.ExpectQuery(`SELECT \* FROM execution WHERE ready_to_send_timestamp >= \$1 AND ready_to_send_timestamp < \$2 ORDER BY ready_to_send_timestamp ASC`).
		WithArgs(startTime, endTime).
		WillReturnRows(rows)

	executions, err := repo.GetForBatch(ctx, startTime, endTime)

	assert.NoError(t, err)
	assert.Len(t, executions, 1)
	assert.Equal(t, 1, executions[0].ID)
	assert.Equal(t, 123, executions[0].ExecutionServiceID)
	assert.NotNil(t, executions[0].PortfolioID)
	assert.NoError(t, mock.ExpectationsWereMet())
}
