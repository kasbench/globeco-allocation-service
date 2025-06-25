package domain

import "time"

// ExecutionListResponse represents the paginated response for listing executions
type ExecutionListResponse struct {
	Executions []ExecutionDTO `json:"executions"`
	Pagination PaginationInfo `json:"pagination"`
}

// PaginationInfo represents pagination metadata
type PaginationInfo struct {
	TotalElements int  `json:"totalElements"`
	TotalPages    int  `json:"totalPages"`
	CurrentPage   int  `json:"currentPage"`
	PageSize      int  `json:"pageSize"`
	HasNext       bool `json:"hasNext"`
	HasPrevious   bool `json:"hasPrevious"`
}

// BatchCreateResponse represents the response for batch creation
type BatchCreateResponse struct {
	ProcessedCount int               `json:"processedCount"`
	SkippedCount   int               `json:"skippedCount"`
	ErrorCount     int               `json:"errorCount"`
	Results        []ExecutionResult `json:"results"`
}

// ExecutionResult represents the result of processing a single execution
type ExecutionResult struct {
	ExecutionServiceID int    `json:"executionServiceId"`
	Status             string `json:"status"` // "created", "skipped", "error"
	Error              string `json:"error,omitempty"`
	ExecutionID        *int   `json:"executionId,omitempty"`
}

// SendResponse represents the response for sending executions to Portfolio Accounting
type SendResponse struct {
	ProcessedCount int    `json:"processedCount"`
	FileName       string `json:"fileName"`
	Status         string `json:"status"`
	Message        string `json:"message"`
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status    string            `json:"status"`
	Timestamp time.Time         `json:"timestamp"`
	Checks    map[string]string `json:"checks,omitempty"`
}

// TradeServiceExecutionResponse represents the response from Trade Service
type TradeServiceExecutionResponse struct {
	Executions []TradeServiceExecution `json:"executions"`
	Pagination PaginationInfo          `json:"pagination"`
}

// TradeServiceExecution represents a single execution from Trade Service
type TradeServiceExecution struct {
	ID                 int                     `json:"id"`
	ExecutionTimestamp time.Time               `json:"executionTimestamp"`
	ExecutionStatus    TradeServiceStatus      `json:"executionStatus"`
	TradeType          TradeServiceTradeType   `json:"tradeType"`
	TradeOrder         TradeServiceTradeOrder  `json:"tradeOrder"`
	Destination        TradeServiceDestination `json:"destination"`
	QuantityOrdered    float64                 `json:"quantityOrdered"`
	QuantityPlaced     float64                 `json:"quantityPlaced"`
	QuantityFilled     float64                 `json:"quantityFilled"`
	LimitPrice         *float64                `json:"limitPrice"`
	ExecutionServiceID int                     `json:"executionServiceId"`
	Version            int                     `json:"version"`
}

// TradeServiceStatus represents execution status from Trade Service
type TradeServiceStatus struct {
	ID           int    `json:"id"`
	Abbreviation string `json:"abbreviation"`
	Description  string `json:"description"`
	Version      int    `json:"version"`
}

// TradeServiceTradeType represents trade type from Trade Service
type TradeServiceTradeType struct {
	ID           int    `json:"id"`
	Abbreviation string `json:"abbreviation"`
	Description  string `json:"description"`
	Version      int    `json:"version"`
}

// TradeServiceTradeOrder represents trade order from Trade Service
type TradeServiceTradeOrder struct {
	ID        int                   `json:"id"`
	OrderID   int                   `json:"orderId"`
	Portfolio TradeServicePortfolio `json:"portfolio"`
	Security  TradeServiceSecurity  `json:"security"`
}

// TradeServicePortfolio represents portfolio from Trade Service
type TradeServicePortfolio struct {
	PortfolioID string `json:"portfolioId"`
	Name        string `json:"name"`
}

// TradeServiceSecurity represents security from Trade Service
type TradeServiceSecurity struct {
	SecurityID string `json:"securityId"`
	Ticker     string `json:"ticker"`
}

// TradeServiceDestination represents destination from Trade Service
type TradeServiceDestination struct {
	ID           int    `json:"id"`
	Abbreviation string `json:"abbreviation"`
	Description  string `json:"description"`
	Version      int    `json:"version"`
}

// ErrorResponse represents a standardized API error response
type ErrorResponse struct {
	Message   string `json:"message"`
	Status    int    `json:"status"`
	Timestamp string `json:"timestamp"`
	Details   string `json:"details,omitempty"`
}

// GetCurrentTimestamp returns the current timestamp in RFC3339 format
func GetCurrentTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339)
}
