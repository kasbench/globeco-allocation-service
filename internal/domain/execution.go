package domain

import (
	"time"
)

// Execution represents a trade execution record
type Execution struct {
	ID                   int        `json:"id" db:"id"`
	ExecutionServiceID   int        `json:"executionServiceId" db:"execution_service_id"`
	IsOpen               bool       `json:"isOpen" db:"is_open"`
	ExecutionStatus      string     `json:"executionStatus" db:"execution_status"`
	TradeType            string     `json:"tradeType" db:"trade_type"`
	Destination          string     `json:"destination" db:"destination"`
	TradeDate            time.Time  `json:"tradeDate" db:"trade_date"`
	SecurityID           string     `json:"securityId" db:"security_id"`
	Ticker               string     `json:"ticker" db:"ticker"`
	PortfolioID          *string    `json:"portfolioId" db:"portfolio_id"`
	Quantity             float64    `json:"quantity" db:"quantity"`
	LimitPrice           *float64   `json:"limitPrice" db:"limit_price"`
	ReceivedTimestamp    time.Time  `json:"receivedTimestamp" db:"received_timestamp"`
	SentTimestamp        time.Time  `json:"sentTimestamp" db:"sent_timestamp"`
	LastFillTimestamp    *time.Time `json:"lastFillTimestamp" db:"last_fill_timestamp"`
	QuantityFilled       float64    `json:"quantityFilled" db:"quantity_filled"`
	TotalAmount          float64    `json:"totalAmount" db:"total_amount"`
	AveragePrice         float64    `json:"averagePrice" db:"average_price"`
	ReadyToSendTimestamp time.Time  `json:"readyToSendTimestamp" db:"ready_to_send_timestamp"`
	Version              int        `json:"version" db:"version"`
}

// BatchHistory represents a batch processing history record
type BatchHistory struct {
	ID                int       `json:"id" db:"id"`
	StartTime         time.Time `json:"startTime" db:"start_time"`
	PreviousStartTime time.Time `json:"previousStartTime" db:"previous_start_time"`
	Version           int       `json:"version" db:"version"`
}

// ExecutionDTO represents the response DTO for execution
type ExecutionDTO struct {
	ID                 int        `json:"id"`
	ExecutionServiceID int        `json:"executionServiceId"`
	IsOpen             bool       `json:"isOpen"`
	ExecutionStatus    string     `json:"executionStatus"`
	TradeType          string     `json:"tradeType"`
	Destination        string     `json:"destination"`
	SecurityID         string     `json:"securityId"`
	PortfolioID        *string    `json:"portfolioId"`
	Ticker             string     `json:"ticker"`
	Quantity           float64    `json:"quantity"`
	LimitPrice         *float64   `json:"limitPrice"`
	ReceivedTimestamp  time.Time  `json:"receivedTimestamp"`
	SentTimestamp      time.Time  `json:"sentTimestamp"`
	LastFillTimestamp  *time.Time `json:"lastFillTimestamp"`
	QuantityFilled     float64    `json:"quantityFilled"`
	TotalAmount        float64    `json:"totalAmount"`
	AveragePrice       float64    `json:"averagePrice"`
	Version            int        `json:"version"`
}

// ExecutionPostDTO represents the request DTO for creating executions
type ExecutionPostDTO struct {
	ExecutionServiceID int        `json:"executionServiceId" validate:"required"`
	IsOpen             bool       `json:"isOpen"`
	ExecutionStatus    string     `json:"executionStatus" validate:"required"`
	TradeType          string     `json:"tradeType" validate:"required,oneof=BUY SELL"`
	Destination        string     `json:"destination" validate:"required"`
	SecurityID         string     `json:"securityId" validate:"required"`
	Ticker             string     `json:"ticker" validate:"required"`
	Quantity           float64    `json:"quantity" validate:"required,gt=0"`
	LimitPrice         *float64   `json:"limitPrice"`
	ReceivedTimestamp  time.Time  `json:"receivedTimestamp" validate:"required"`
	SentTimestamp      time.Time  `json:"sentTimestamp" validate:"required"`
	LastFillTimestamp  *time.Time `json:"lastFillTimestamp"`
	QuantityFilled     float64    `json:"quantityFilled" validate:"gte=0"`
	TotalAmount        float64    `json:"totalAmount" validate:"gte=0"`
	AveragePrice       float64    `json:"averagePrice" validate:"gt=0"`
}

// ToDTO converts an Execution domain model to ExecutionDTO
func (e *Execution) ToDTO() ExecutionDTO {
	return ExecutionDTO{
		ID:                 e.ID,
		ExecutionServiceID: e.ExecutionServiceID,
		IsOpen:             e.IsOpen,
		ExecutionStatus:    e.ExecutionStatus,
		TradeType:          e.TradeType,
		Destination:        e.Destination,
		SecurityID:         e.SecurityID,
		PortfolioID:        e.PortfolioID,
		Ticker:             e.Ticker,
		Quantity:           e.Quantity,
		LimitPrice:         e.LimitPrice,
		ReceivedTimestamp:  e.ReceivedTimestamp,
		SentTimestamp:      e.SentTimestamp,
		LastFillTimestamp:  e.LastFillTimestamp,
		QuantityFilled:     e.QuantityFilled,
		TotalAmount:        e.TotalAmount,
		AveragePrice:       e.AveragePrice,
		Version:            e.Version,
	}
}

// ToExecution converts an ExecutionPostDTO to Execution domain model
func (dto *ExecutionPostDTO) ToExecution() Execution {
	now := time.Now()

	// Calculate trade date in US Eastern Time
	loc, _ := time.LoadLocation("America/New_York")
	tradeDate := dto.SentTimestamp.In(loc).Truncate(24 * time.Hour)

	return Execution{
		ExecutionServiceID:   dto.ExecutionServiceID,
		IsOpen:               dto.IsOpen,
		ExecutionStatus:      dto.ExecutionStatus,
		TradeType:            dto.TradeType,
		Destination:          dto.Destination,
		TradeDate:            tradeDate,
		SecurityID:           dto.SecurityID,
		Ticker:               dto.Ticker,
		PortfolioID:          nil, // Will be set by business logic
		Quantity:             dto.Quantity,
		LimitPrice:           dto.LimitPrice,
		ReceivedTimestamp:    dto.ReceivedTimestamp,
		SentTimestamp:        dto.SentTimestamp,
		LastFillTimestamp:    dto.LastFillTimestamp,
		QuantityFilled:       dto.QuantityFilled,
		TotalAmount:          dto.TotalAmount,
		AveragePrice:         dto.AveragePrice,
		ReadyToSendTimestamp: now,
		Version:              1,
	}
}
