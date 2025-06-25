package domain

import (
	"testing"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/assert"
)

func TestExecutionPostDTO_Validation(t *testing.T) {
	validator := validator.New()

	tests := []struct {
		name    string
		dto     ExecutionPostDTO
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid DTO",
			dto: ExecutionPostDTO{
				ExecutionServiceID: 123,
				IsOpen:             false,
				ExecutionStatus:    "FILLED",
				TradeType:          "BUY",
				Destination:        "NYSE",
				SecurityID:         "12345678901234567890ABCD",
				Ticker:             "AAPL",
				Quantity:           100.5,
				LimitPrice:         nil,
				ReceivedTimestamp:  time.Now(),
				SentTimestamp:      time.Now(),
				QuantityFilled:     100.5,
				TotalAmount:        15000.0,
				AveragePrice:       149.25,
			},
			wantErr: false,
		},
		{
			name: "missing required ExecutionServiceID",
			dto: ExecutionPostDTO{
				ExecutionStatus:   "FILLED",
				TradeType:         "BUY",
				Destination:       "NYSE",
				SecurityID:        "12345678901234567890ABCD",
				Ticker:            "AAPL",
				Quantity:          100.5,
				ReceivedTimestamp: time.Now(),
				SentTimestamp:     time.Now(),
				QuantityFilled:    100.5,
				TotalAmount:       15000.0,
				AveragePrice:      149.25,
			},
			wantErr: true,
			errMsg:  "ExecutionServiceID",
		},
		{
			name: "invalid TradeType",
			dto: ExecutionPostDTO{
				ExecutionServiceID: 123,
				ExecutionStatus:    "FILLED",
				TradeType:          "INVALID",
				Destination:        "NYSE",
				SecurityID:         "12345678901234567890ABCD",
				Ticker:             "AAPL",
				Quantity:           100.5,
				ReceivedTimestamp:  time.Now(),
				SentTimestamp:      time.Now(),
				QuantityFilled:     100.5,
				TotalAmount:        15000.0,
				AveragePrice:       149.25,
			},
			wantErr: true,
			errMsg:  "TradeType",
		},
		{
			name: "zero quantity",
			dto: ExecutionPostDTO{
				ExecutionServiceID: 123,
				ExecutionStatus:    "FILLED",
				TradeType:          "BUY",
				Destination:        "NYSE",
				SecurityID:         "12345678901234567890ABCD",
				Ticker:             "AAPL",
				Quantity:           0,
				ReceivedTimestamp:  time.Now(),
				SentTimestamp:      time.Now(),
				QuantityFilled:     0,
				TotalAmount:        0,
				AveragePrice:       149.25,
			},
			wantErr: true,
			errMsg:  "Quantity",
		},
		{
			name: "negative average price",
			dto: ExecutionPostDTO{
				ExecutionServiceID: 123,
				ExecutionStatus:    "FILLED",
				TradeType:          "BUY",
				Destination:        "NYSE",
				SecurityID:         "12345678901234567890ABCD",
				Ticker:             "AAPL",
				Quantity:           100.5,
				ReceivedTimestamp:  time.Now(),
				SentTimestamp:      time.Now(),
				QuantityFilled:     100.5,
				TotalAmount:        15000.0,
				AveragePrice:       -149.25,
			},
			wantErr: true,
			errMsg:  "AveragePrice",
		},
		{
			name: "negative quantity filled",
			dto: ExecutionPostDTO{
				ExecutionServiceID: 123,
				ExecutionStatus:    "FILLED",
				TradeType:          "BUY",
				Destination:        "NYSE",
				SecurityID:         "12345678901234567890ABCD",
				Ticker:             "AAPL",
				Quantity:           100.5,
				ReceivedTimestamp:  time.Now(),
				SentTimestamp:      time.Now(),
				QuantityFilled:     -100.5,
				TotalAmount:        15000.0,
				AveragePrice:       149.25,
			},
			wantErr: true,
			errMsg:  "QuantityFilled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Struct(tt.dto)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestExecution_ToDTO(t *testing.T) {
	now := time.Now()
	fillTime := now.Add(1 * time.Hour)
	portfolioID := "PORTFOLIO123456789012"
	limitPrice := 150.0

	execution := Execution{
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
		LimitPrice:           &limitPrice,
		ReceivedTimestamp:    now,
		SentTimestamp:        now.Add(30 * time.Second),
		LastFillTimestamp:    &fillTime,
		QuantityFilled:       100.5,
		TotalAmount:          15000.0,
		AveragePrice:         149.25,
		ReadyToSendTimestamp: now,
		Version:              1,
	}

	dto := execution.ToDTO()

	assert.Equal(t, execution.ID, dto.ID)
	assert.Equal(t, execution.ExecutionServiceID, dto.ExecutionServiceID)
	assert.Equal(t, execution.IsOpen, dto.IsOpen)
	assert.Equal(t, execution.ExecutionStatus, dto.ExecutionStatus)
	assert.Equal(t, execution.TradeType, dto.TradeType)
	assert.Equal(t, execution.Destination, dto.Destination)
	assert.Equal(t, execution.SecurityID, dto.SecurityID)
	assert.Equal(t, execution.PortfolioID, dto.PortfolioID)
	assert.Equal(t, execution.Ticker, dto.Ticker)
	assert.Equal(t, execution.Quantity, dto.Quantity)
	assert.Equal(t, execution.LimitPrice, dto.LimitPrice)
	assert.Equal(t, execution.ReceivedTimestamp, dto.ReceivedTimestamp)
	assert.Equal(t, execution.SentTimestamp, dto.SentTimestamp)
	assert.Equal(t, execution.LastFillTimestamp, dto.LastFillTimestamp)
	assert.Equal(t, execution.QuantityFilled, dto.QuantityFilled)
	assert.Equal(t, execution.TotalAmount, dto.TotalAmount)
	assert.Equal(t, execution.AveragePrice, dto.AveragePrice)
	assert.Equal(t, execution.Version, dto.Version)
}

func TestExecutionPostDTO_ToExecution(t *testing.T) {
	now := time.Now()
	fillTime := now.Add(1 * time.Hour)
	limitPrice := 150.0

	dto := ExecutionPostDTO{
		ExecutionServiceID: 123,
		IsOpen:             false,
		ExecutionStatus:    "FILLED",
		TradeType:          "BUY",
		Destination:        "NYSE",
		SecurityID:         "12345678901234567890ABCD",
		Ticker:             "AAPL",
		Quantity:           100.5,
		LimitPrice:         &limitPrice,
		ReceivedTimestamp:  now,
		SentTimestamp:      now.Add(30 * time.Second),
		LastFillTimestamp:  &fillTime,
		QuantityFilled:     100.5,
		TotalAmount:        15000.0,
		AveragePrice:       149.25,
	}

	execution := dto.ToExecution()

	assert.Equal(t, dto.ExecutionServiceID, execution.ExecutionServiceID)
	assert.Equal(t, dto.IsOpen, execution.IsOpen)
	assert.Equal(t, dto.ExecutionStatus, execution.ExecutionStatus)
	assert.Equal(t, dto.TradeType, execution.TradeType)
	assert.Equal(t, dto.Destination, execution.Destination)
	assert.Equal(t, dto.SecurityID, execution.SecurityID)
	assert.Equal(t, dto.Ticker, execution.Ticker)
	assert.Equal(t, dto.Quantity, execution.Quantity)
	assert.Equal(t, dto.LimitPrice, execution.LimitPrice)
	assert.Equal(t, dto.ReceivedTimestamp, execution.ReceivedTimestamp)
	assert.Equal(t, dto.SentTimestamp, execution.SentTimestamp)
	assert.Equal(t, dto.LastFillTimestamp, execution.LastFillTimestamp)
	assert.Equal(t, dto.QuantityFilled, execution.QuantityFilled)
	assert.Equal(t, dto.TotalAmount, execution.TotalAmount)
	assert.Equal(t, dto.AveragePrice, execution.AveragePrice)
	assert.Equal(t, 1, execution.Version)
	assert.Nil(t, execution.PortfolioID)             // Should be nil initially
	assert.NotNil(t, execution.ReadyToSendTimestamp) // Should be set to current time
}

func TestBatchCreateResponse_CalculateTotals(t *testing.T) {
	results := []ExecutionResult{
		{ExecutionServiceID: 1, Status: "created"},
		{ExecutionServiceID: 2, Status: "created"},
		{ExecutionServiceID: 3, Status: "skipped"},
		{ExecutionServiceID: 4, Status: "error", Error: "validation failed"},
		{ExecutionServiceID: 5, Status: "created"},
	}

	response := BatchCreateResponse{Results: results}
	response.CalculateTotals()

	assert.Equal(t, 3, response.ProcessedCount)
	assert.Equal(t, 1, response.SkippedCount)
	assert.Equal(t, 1, response.ErrorCount)
}

func TestPaginationInfo_CalculatePages(t *testing.T) {
	tests := []struct {
		name            string
		totalElements   int
		pageSize        int
		currentPage     int
		expectedPages   int
		expectedHasNext bool
		expectedHasPrev bool
	}{
		{
			name:            "first page with more pages",
			totalElements:   100,
			pageSize:        20,
			currentPage:     0,
			expectedPages:   5,
			expectedHasNext: true,
			expectedHasPrev: false,
		},
		{
			name:            "middle page",
			totalElements:   100,
			pageSize:        20,
			currentPage:     2,
			expectedPages:   5,
			expectedHasNext: true,
			expectedHasPrev: true,
		},
		{
			name:            "last page",
			totalElements:   100,
			pageSize:        20,
			currentPage:     4,
			expectedPages:   5,
			expectedHasNext: false,
			expectedHasPrev: true,
		},
		{
			name:            "single page",
			totalElements:   10,
			pageSize:        20,
			currentPage:     0,
			expectedPages:   1,
			expectedHasNext: false,
			expectedHasPrev: false,
		},
		{
			name:            "exact division",
			totalElements:   100,
			pageSize:        25,
			currentPage:     3,
			expectedPages:   4,
			expectedHasNext: false,
			expectedHasPrev: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pagination := PaginationInfo{
				TotalElements: tt.totalElements,
				PageSize:      tt.pageSize,
				CurrentPage:   tt.currentPage,
			}

			// Calculate derived fields
			if tt.totalElements == 0 {
				pagination.TotalPages = 0
			} else {
				pagination.TotalPages = (tt.totalElements + tt.pageSize - 1) / tt.pageSize
			}
			pagination.HasNext = tt.currentPage < pagination.TotalPages-1
			pagination.HasPrevious = tt.currentPage > 0

			assert.Equal(t, tt.expectedPages, pagination.TotalPages)
			assert.Equal(t, tt.expectedHasNext, pagination.HasNext)
			assert.Equal(t, tt.expectedHasPrev, pagination.HasPrevious)
		})
	}
}
