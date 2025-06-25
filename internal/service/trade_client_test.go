package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/kasbench/globeco-allocation-service/internal/domain"
)

func TestTradeServiceClient_GetExecutionByServiceID(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	tradeServiceURL := "http://globeco-trade-service:8082"
	logger := zap.NewNop()
	client := NewTradeServiceClient(tradeServiceURL, logger)

	// Mock response data
	tradeServiceResponse := domain.TradeServiceExecutionResponse{
		Executions: []domain.TradeServiceExecution{
			{
				ID:                 1,
				ExecutionTimestamp: time.Now(),
				ExecutionStatus: domain.TradeServiceStatus{
					ID:           1,
					Abbreviation: "FILLED",
					Description:  "Filled",
					Version:      1,
				},
				TradeType: domain.TradeServiceTradeType{
					ID:           1,
					Abbreviation: "BUY",
					Description:  "Buy",
					Version:      1,
				},
				TradeOrder: domain.TradeServiceTradeOrder{
					ID:      1,
					OrderID: 123,
					Portfolio: domain.TradeServicePortfolio{
						PortfolioID: "PORTFOLIO123456789012",
						Name:        "Test Portfolio",
					},
					Security: domain.TradeServiceSecurity{
						SecurityID: "12345678901234567890ABCD",
						Ticker:     "AAPL",
					},
				},
				Destination: domain.TradeServiceDestination{
					ID:           1,
					Abbreviation: "NYSE",
					Description:  "New York Stock Exchange",
					Version:      1,
				},
				QuantityOrdered:    100.5,
				QuantityPlaced:     100.5,
				QuantityFilled:     100.5,
				LimitPrice:         nil,
				ExecutionServiceID: 123,
				Version:            1,
			},
		},
		Pagination: domain.PaginationInfo{
			TotalElements: 1,
			TotalPages:    1,
			CurrentPage:   0,
			PageSize:      50,
			HasNext:       false,
			HasPrevious:   false,
		},
	}

	responseBody, _ := json.Marshal(tradeServiceResponse)

	httpmock.RegisterResponder(
		"GET",
		"http://globeco-trade-service:8082/api/v2/executions",
		httpmock.NewStringResponder(200, string(responseBody)))

	ctx := context.Background()
	response, err := client.GetExecutionByServiceID(ctx, 123)

	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Len(t, response.Executions, 1)

	execution := response.Executions[0]
	assert.Equal(t, 1, execution.ID)
	assert.Equal(t, 123, execution.ExecutionServiceID)
	assert.Equal(t, "PORTFOLIO123456789012", execution.TradeOrder.Portfolio.PortfolioID)
	assert.Equal(t, "AAPL", execution.TradeOrder.Security.Ticker)
	assert.Equal(t, "FILLED", execution.ExecutionStatus.Abbreviation)
	assert.Equal(t, "BUY", execution.TradeType.Abbreviation)
	assert.Equal(t, "NYSE", execution.Destination.Abbreviation)
	assert.Equal(t, 100.5, execution.QuantityFilled)

	// Verify the request was made correctly
	assert.Equal(t, 1, httpmock.GetTotalCallCount())
	info := httpmock.GetCallCountInfo()
	assert.Contains(t, info, "GET http://globeco-trade-service:8082/api/v2/executions")
}

func TestTradeServiceClient_GetExecutionByServiceID_NotFound(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	tradeServiceURL := "http://globeco-trade-service:8082"
	logger := zap.NewNop()
	client := NewTradeServiceClient(tradeServiceURL, logger)

	// Mock empty response
	tradeServiceResponse := domain.TradeServiceExecutionResponse{
		Executions: []domain.TradeServiceExecution{},
		Pagination: domain.PaginationInfo{
			TotalElements: 0,
			TotalPages:    0,
			CurrentPage:   0,
			PageSize:      50,
			HasNext:       false,
			HasPrevious:   false,
		},
	}

	responseBody, _ := json.Marshal(tradeServiceResponse)

	httpmock.RegisterResponder(
		"GET",
		"http://globeco-trade-service:8082/api/v2/executions",
		httpmock.NewStringResponder(200, string(responseBody)))

	ctx := context.Background()
	response, err := client.GetExecutionByServiceID(ctx, 999)

	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Len(t, response.Executions, 0)
	assert.Equal(t, 0, response.Pagination.TotalElements)
}

func TestTradeServiceClient_GetExecutionByServiceID_HTTPError(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	tradeServiceURL := "http://globeco-trade-service:8082"
	logger := zap.NewNop()
	client := NewTradeServiceClient(tradeServiceURL, logger)

	httpmock.RegisterResponder(
		"GET",
		"http://globeco-trade-service:8082/api/v2/executions",
		httpmock.NewStringResponder(500, "Internal Server Error"))

	ctx := context.Background()
	response, err := client.GetExecutionByServiceID(ctx, 123)

	assert.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "all retry attempts failed")
}

func TestTradeServiceClient_GetExecutionByServiceID_InvalidJSON(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	tradeServiceURL := "http://globeco-trade-service:8082"
	logger := zap.NewNop()
	client := NewTradeServiceClient(tradeServiceURL, logger)

	httpmock.RegisterResponder(
		"GET",
		"http://globeco-trade-service:8082/api/v2/executions",
		httpmock.NewStringResponder(200, "invalid json"))

	ctx := context.Background()
	response, err := client.GetExecutionByServiceID(ctx, 123)

	assert.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "failed to parse response")
}

func TestTradeServiceClient_GetExecutionByServiceID_Timeout(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	tradeServiceURL := "http://globeco-trade-service:8082"
	logger := zap.NewNop()
	client := NewTradeServiceClient(tradeServiceURL, logger)

	// Register a responder that will timeout
	httpmock.RegisterResponder(
		"GET",
		"http://globeco-trade-service:8082/api/v2/executions",
		func(req *http.Request) (*http.Response, error) {
			time.Sleep(35 * time.Second) // Longer than the 30s timeout
			return httpmock.NewStringResponse(200, "{}"), nil
		})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	response, err := client.GetExecutionByServiceID(ctx, 123)

	assert.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "context deadline exceeded")
}

func TestTradeServiceClient_GetExecutionByServiceID_Retry(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	tradeServiceURL := "http://globeco-trade-service:8082"
	logger := zap.NewNop()
	client := NewTradeServiceClient(tradeServiceURL, logger)

	callCount := 0
	httpmock.RegisterResponder(
		"GET",
		"http://globeco-trade-service:8082/api/v2/executions",
		func(req *http.Request) (*http.Response, error) {
			callCount++
			if callCount < 3 {
				// Return 500 for first two attempts
				return httpmock.NewStringResponse(500, "Internal Server Error"), nil
			}
			// Return success on third attempt
			tradeServiceResponse := domain.TradeServiceExecutionResponse{
				Executions: []domain.TradeServiceExecution{
					{
						ID:                 1,
						ExecutionServiceID: 123,
						TradeOrder: domain.TradeServiceTradeOrder{
							Portfolio: domain.TradeServicePortfolio{
								PortfolioID: "PORTFOLIO123456789012",
								Name:        "Test Portfolio",
							},
						},
					},
				},
			}
			responseBody, _ := json.Marshal(tradeServiceResponse)
			return httpmock.NewStringResponse(200, string(responseBody)), nil
		})

	ctx := context.Background()
	response, err := client.GetExecutionByServiceID(ctx, 123)

	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Len(t, response.Executions, 1)
	assert.Equal(t, 1, response.Executions[0].ID)
	assert.Equal(t, 3, callCount) // Should have been called 3 times
}

func TestTradeServiceClient_GetExecutionByServiceID_RetryExhausted(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	tradeServiceURL := "http://globeco-trade-service:8082"
	logger := zap.NewNop()
	client := NewTradeServiceClient(tradeServiceURL, logger)

	// Always return 500
	httpmock.RegisterResponder(
		"GET",
		"http://globeco-trade-service:8082/api/v2/executions",
		httpmock.NewStringResponder(500, "Internal Server Error"))

	ctx := context.Background()
	response, err := client.GetExecutionByServiceID(ctx, 123)

	assert.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "all retry attempts failed")

	// Should have been called 4 times (initial + 3 retries)
	assert.Equal(t, 4, httpmock.GetTotalCallCount())
}
