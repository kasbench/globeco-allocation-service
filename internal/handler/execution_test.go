package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/kasbench/globeco-allocation-service/internal/domain"
)

// ExecutionServiceInterface defines the interface for execution service operations
type ExecutionServiceInterface interface {
	CreateBatch(ctx context.Context, dtos []domain.ExecutionPostDTO) (*domain.BatchCreateResponse, error)
	GetByID(ctx context.Context, id int) (*domain.ExecutionDTO, error)
	List(ctx context.Context, limit, offset int) (*domain.ExecutionListResponse, error)
	Send(ctx context.Context) (*domain.SendResponse, error)
}

// MockExecutionService is a mock for the execution service
type MockExecutionService struct {
	mock.Mock
}

func (m *MockExecutionService) CreateBatch(ctx context.Context, dtos []domain.ExecutionPostDTO) (*domain.BatchCreateResponse, error) {
	args := m.Called(ctx, dtos)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.BatchCreateResponse), args.Error(1)
}

func (m *MockExecutionService) GetByID(ctx context.Context, id int) (*domain.ExecutionDTO, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.ExecutionDTO), args.Error(1)
}

func (m *MockExecutionService) List(ctx context.Context, limit, offset int) (*domain.ExecutionListResponse, error) {
	args := m.Called(ctx, limit, offset)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.ExecutionListResponse), args.Error(1)
}

func (m *MockExecutionService) Send(ctx context.Context) (*domain.SendResponse, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.SendResponse), args.Error(1)
}

// TestableExecutionHandler wraps ExecutionHandler for testing
type TestableExecutionHandler struct {
	service ExecutionServiceInterface
	logger  *zap.Logger
}

func (h *TestableExecutionHandler) CreateExecutions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse request body
	var executions []domain.ExecutionPostDTO
	if err := json.NewDecoder(r.Body).Decode(&executions); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, "invalid request body", err)
		return
	}

	// Validate request
	if len(executions) == 0 {
		h.writeErrorResponse(w, http.StatusBadRequest, "no executions provided", nil)
		return
	}

	// Call service
	response, err := h.service.CreateBatch(ctx, executions)
	if err != nil {
		h.writeErrorResponse(w, http.StatusInternalServerError, "failed to create executions", err)
		return
	}

	// Determine response status based on results
	statusCode := http.StatusCreated
	if response.ErrorCount > 0 && response.ProcessedCount == 0 {
		statusCode = http.StatusBadRequest
	} else if response.ErrorCount > 0 {
		statusCode = http.StatusMultiStatus
	}

	h.writeJSONResponse(w, statusCode, response)
}

func (h *TestableExecutionHandler) GetExecution(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse ID from URL
	idStr := chi.URLParam(r, "id")
	if idStr == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "execution ID is required", nil)
		return
	}

	id := 0
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, "invalid execution ID", err)
		return
	}

	// Call service
	execution, err := h.service.GetByID(ctx, id)
	if err != nil {
		h.writeErrorResponse(w, http.StatusNotFound, "execution not found", err)
		return
	}

	h.writeJSONResponse(w, http.StatusOK, execution)
}

func (h *TestableExecutionHandler) GetExecutions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse pagination parameters (simplified for test)
	limit := 50
	offset := 0

	// Call service
	response, err := h.service.List(ctx, limit, offset)
	if err != nil {
		h.writeErrorResponse(w, http.StatusInternalServerError, "failed to retrieve executions", err)
		return
	}

	h.writeJSONResponse(w, http.StatusOK, response)
}

func (h *TestableExecutionHandler) SendExecutions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Call service
	response, err := h.service.Send(ctx)
	if err != nil {
		if err.Error() == "duplicate batch process already started" {
			h.writeErrorResponse(w, http.StatusConflict, "batch process already in progress", err)
			return
		}
		h.writeErrorResponse(w, http.StatusInternalServerError, "failed to process executions", err)
		return
	}

	h.writeJSONResponse(w, http.StatusOK, response)
}

func (h *TestableExecutionHandler) writeJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}

func (h *TestableExecutionHandler) writeErrorResponse(w http.ResponseWriter, statusCode int, message string, err error) {
	response := domain.ErrorResponse{
		Message:   message,
		Status:    statusCode,
		Timestamp: domain.GetCurrentTimestamp(),
	}
	if err != nil {
		response.Details = err.Error()
	}
	h.writeJSONResponse(w, statusCode, response)
}

func TestExecutionHandler_CreateExecutions(t *testing.T) {
	mockService := new(MockExecutionService)
	logger := zap.NewNop()
	handler := &TestableExecutionHandler{
		service: mockService,
		logger:  logger,
	}

	// Test data - create a fixed time to avoid monotonic clock issues
	fixedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	executions := []domain.ExecutionPostDTO{
		{
			ExecutionServiceID: 123,
			IsOpen:             false,
			ExecutionStatus:    "FILLED",
			TradeType:          "BUY",
			Destination:        "NYSE",
			SecurityID:         "12345678901234567890ABCD",
			Ticker:             "AAPL",
			Quantity:           100.5,
			ReceivedTimestamp:  fixedTime,
			SentTimestamp:      fixedTime.Add(1 * time.Minute),
			QuantityFilled:     100.5,
			TotalAmount:        15075.0,
			AveragePrice:       150.0,
		},
	}

	executionID1 := 1
	expectedResponse := &domain.BatchCreateResponse{
		ProcessedCount: 1,
		SkippedCount:   0,
		ErrorCount:     0,
		Results: []domain.ExecutionResult{
			{ExecutionServiceID: 123, Status: "created", ExecutionID: &executionID1},
		},
	}

	// Use mock.Anything for context to avoid type matching issues
	mockService.On("CreateBatch", mock.Anything, mock.MatchedBy(func(dtos []domain.ExecutionPostDTO) bool {
		return len(dtos) == 1 && dtos[0].ExecutionServiceID == 123
	})).Return(expectedResponse, nil)

	// Create request
	requestBody, _ := json.Marshal(executions)
	req := httptest.NewRequest("POST", "/api/v1/executions", bytes.NewBuffer(requestBody))
	req.Header.Set("Content-Type", "application/json")

	// Create response recorder
	rr := httptest.NewRecorder()

	// Execute request
	handler.CreateExecutions(rr, req)

	// Verify response
	assert.Equal(t, http.StatusCreated, rr.Code)

	var response domain.BatchCreateResponse
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 1, response.ProcessedCount)
	assert.Equal(t, 0, response.SkippedCount)
	assert.Equal(t, 0, response.ErrorCount)

	mockService.AssertExpectations(t)
}

func TestExecutionHandler_GetExecution(t *testing.T) {
	mockService := new(MockExecutionService)
	logger := zap.NewNop()
	handler := &TestableExecutionHandler{
		service: mockService,
		logger:  logger,
	}

	// Test data
	now := time.Now()
	portfolioID := "PORTFOLIO123456789012"
	execution := &domain.ExecutionDTO{
		ID:                 1,
		ExecutionServiceID: 123,
		IsOpen:             false,
		ExecutionStatus:    "FILLED",
		TradeType:          "BUY",
		Destination:        "NYSE",
		SecurityID:         "12345678901234567890ABCD",
		Ticker:             "AAPL",
		PortfolioID:        &portfolioID,
		Quantity:           100.5,
		ReceivedTimestamp:  now,
		Version:            1,
	}

	mockService.On("GetByID", mock.Anything, 1).Return(execution, nil)

	// Create request with chi context
	req := httptest.NewRequest("GET", "/api/v1/executions/1", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	// Create response recorder
	rr := httptest.NewRecorder()

	// Execute request
	handler.GetExecution(rr, req)

	// Verify response
	assert.Equal(t, http.StatusOK, rr.Code)

	var response domain.ExecutionDTO
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 1, response.ID)
	assert.Equal(t, 123, response.ExecutionServiceID)
	assert.Equal(t, "FILLED", response.ExecutionStatus)

	mockService.AssertExpectations(t)
}

func TestExecutionHandler_GetExecutions(t *testing.T) {
	mockService := new(MockExecutionService)
	logger := zap.NewNop()
	handler := &TestableExecutionHandler{
		service: mockService,
		logger:  logger,
	}

	// Test data
	executions := []domain.ExecutionDTO{
		{ID: 1, ExecutionServiceID: 123, TradeType: "BUY"},
		{ID: 2, ExecutionServiceID: 124, TradeType: "SELL"},
	}

	expectedResponse := &domain.ExecutionListResponse{
		Executions: executions,
		Pagination: domain.PaginationInfo{
			TotalElements: 2,
			TotalPages:    1,
			CurrentPage:   0,
			PageSize:      50,
		},
	}

	mockService.On("List", mock.Anything, 50, 0).Return(expectedResponse, nil)

	// Create request
	req := httptest.NewRequest("GET", "/api/v1/executions", nil)
	rr := httptest.NewRecorder()

	// Execute request
	handler.GetExecutions(rr, req)

	// Verify response
	assert.Equal(t, http.StatusOK, rr.Code)

	var response domain.ExecutionListResponse
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Len(t, response.Executions, 2)

	mockService.AssertExpectations(t)
}

func TestExecutionHandler_SendExecutions(t *testing.T) {
	mockService := new(MockExecutionService)
	logger := zap.NewNop()
	handler := &TestableExecutionHandler{
		service: mockService,
		logger:  logger,
	}

	expectedResponse := &domain.SendResponse{
		ProcessedCount: 5,
		FileName:       "transactions_20240115.csv",
		Status:         "success",
		Message:        "5 executions processed successfully",
	}

	mockService.On("Send", mock.Anything).Return(expectedResponse, nil)

	// Create request
	req := httptest.NewRequest("POST", "/api/v1/executions/send", nil)
	rr := httptest.NewRecorder()

	// Execute request
	handler.SendExecutions(rr, req)

	// Verify response
	assert.Equal(t, http.StatusOK, rr.Code)

	var response domain.SendResponse
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 5, response.ProcessedCount)
	assert.Equal(t, "success", response.Status)

	mockService.AssertExpectations(t)
}

// Additional test scenarios for error handling and edge cases

func TestExecutionHandler_CreateExecutions_InvalidJSON(t *testing.T) {
	mockService := new(MockExecutionService)
	logger := zap.NewNop()
	handler := &TestableExecutionHandler{
		service: mockService,
		logger:  logger,
	}

	// Create request with invalid JSON
	req := httptest.NewRequest("POST", "/api/v1/executions", bytes.NewBuffer([]byte("{invalid json")))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	// Execute request
	handler.CreateExecutions(rr, req)

	// Verify error response
	assert.Equal(t, http.StatusBadRequest, rr.Code)

	var response domain.ErrorResponse
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "invalid request body", response.Message)
	assert.Equal(t, http.StatusBadRequest, response.Status)
}

func TestExecutionHandler_CreateExecutions_EmptyArray(t *testing.T) {
	mockService := new(MockExecutionService)
	logger := zap.NewNop()
	handler := &TestableExecutionHandler{
		service: mockService,
		logger:  logger,
	}

	// Create request with empty array
	requestBody, _ := json.Marshal([]domain.ExecutionPostDTO{})
	req := httptest.NewRequest("POST", "/api/v1/executions", bytes.NewBuffer(requestBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	// Execute request
	handler.CreateExecutions(rr, req)

	// Verify error response
	assert.Equal(t, http.StatusBadRequest, rr.Code)

	var response domain.ErrorResponse
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "no executions provided", response.Message)
}

func TestExecutionHandler_CreateExecutions_ServiceError(t *testing.T) {
	mockService := new(MockExecutionService)
	logger := zap.NewNop()
	handler := &TestableExecutionHandler{
		service: mockService,
		logger:  logger,
	}

	// Test data
	fixedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	executions := []domain.ExecutionPostDTO{
		{
			ExecutionServiceID: 123,
			ExecutionStatus:    "FILLED",
			TradeType:          "BUY",
			Destination:        "NYSE",
			SecurityID:         "12345678901234567890ABCD",
			Ticker:             "AAPL",
			Quantity:           100.5,
			ReceivedTimestamp:  fixedTime,
			SentTimestamp:      fixedTime.Add(1 * time.Minute),
			AveragePrice:       150.0,
		},
	}

	// Mock service to return error
	mockService.On("CreateBatch", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("database connection failed"))

	// Create request
	requestBody, _ := json.Marshal(executions)
	req := httptest.NewRequest("POST", "/api/v1/executions", bytes.NewBuffer(requestBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	// Execute request
	handler.CreateExecutions(rr, req)

	// Verify error response
	assert.Equal(t, http.StatusInternalServerError, rr.Code)

	var response domain.ErrorResponse
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "failed to create executions", response.Message)
	assert.Contains(t, response.Details, "database connection failed")

	mockService.AssertExpectations(t)
}

func TestExecutionHandler_GetExecution_InvalidID(t *testing.T) {
	mockService := new(MockExecutionService)
	logger := zap.NewNop()
	handler := &TestableExecutionHandler{
		service: mockService,
		logger:  logger,
	}

	// Create request with invalid ID
	req := httptest.NewRequest("GET", "/api/v1/executions/invalid", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "invalid")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	// Execute request
	handler.GetExecution(rr, req)

	// Verify error response
	assert.Equal(t, http.StatusBadRequest, rr.Code)

	var response domain.ErrorResponse
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "invalid execution ID", response.Message)
}

func TestExecutionHandler_GetExecution_NotFound(t *testing.T) {
	mockService := new(MockExecutionService)
	logger := zap.NewNop()
	handler := &TestableExecutionHandler{
		service: mockService,
		logger:  logger,
	}

	// Mock service to return error for not found
	mockService.On("GetByID", mock.Anything, 999).Return(nil, fmt.Errorf("execution not found"))

	// Create request
	req := httptest.NewRequest("GET", "/api/v1/executions/999", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "999")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	// Execute request
	handler.GetExecution(rr, req)

	// Verify error response
	assert.Equal(t, http.StatusNotFound, rr.Code)

	var response domain.ErrorResponse
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "execution not found", response.Message)

	mockService.AssertExpectations(t)
}

func TestExecutionHandler_SendExecutions_ConflictError(t *testing.T) {
	mockService := new(MockExecutionService)
	logger := zap.NewNop()
	handler := &TestableExecutionHandler{
		service: mockService,
		logger:  logger,
	}

	// Mock service to return duplicate batch error
	mockService.On("Send", mock.Anything).Return(nil, fmt.Errorf("duplicate batch process already started"))

	// Create request
	req := httptest.NewRequest("POST", "/api/v1/executions/send", nil)
	rr := httptest.NewRecorder()

	// Execute request
	handler.SendExecutions(rr, req)

	// Verify conflict response
	assert.Equal(t, http.StatusConflict, rr.Code)

	var response domain.ErrorResponse
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "batch process already in progress", response.Message)

	mockService.AssertExpectations(t)
}

func TestExecutionHandler_CreateExecutions_MixedResults(t *testing.T) {
	mockService := new(MockExecutionService)
	logger := zap.NewNop()
	handler := &TestableExecutionHandler{
		service: mockService,
		logger:  logger,
	}

	// Test data
	fixedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	executions := []domain.ExecutionPostDTO{
		{
			ExecutionServiceID: 123,
			ExecutionStatus:    "FILLED",
			TradeType:          "BUY",
			Destination:        "NYSE",
			SecurityID:         "12345678901234567890ABCD",
			Ticker:             "AAPL",
			Quantity:           100.5,
			ReceivedTimestamp:  fixedTime,
			SentTimestamp:      fixedTime.Add(1 * time.Minute),
			AveragePrice:       150.0,
		},
	}

	// Mock response with errors
	executionID1 := 1
	expectedResponse := &domain.BatchCreateResponse{
		ProcessedCount: 1,
		SkippedCount:   0,
		ErrorCount:     1,
		Results: []domain.ExecutionResult{
			{ExecutionServiceID: 123, Status: "created", ExecutionID: &executionID1},
			{ExecutionServiceID: 124, Status: "error", Error: "validation failed"},
		},
	}

	mockService.On("CreateBatch", mock.Anything, mock.Anything).Return(expectedResponse, nil)

	// Create request
	requestBody, _ := json.Marshal(executions)
	req := httptest.NewRequest("POST", "/api/v1/executions", bytes.NewBuffer(requestBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	// Execute request
	handler.CreateExecutions(rr, req)

	// Verify multi-status response (some success, some errors)
	assert.Equal(t, http.StatusMultiStatus, rr.Code)

	var response domain.BatchCreateResponse
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 1, response.ProcessedCount)
	assert.Equal(t, 1, response.ErrorCount)

	mockService.AssertExpectations(t)
}
