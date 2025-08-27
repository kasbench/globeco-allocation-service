package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/kasbench/globeco-allocation-service/internal/domain"
	"github.com/kasbench/globeco-allocation-service/internal/service"
)

// ExecutionHandler handles HTTP requests for executions
type ExecutionHandler struct {
	executionService *service.ExecutionService
	logger           *zap.Logger
}

// NewExecutionHandler creates a new execution handler
func NewExecutionHandler(executionService *service.ExecutionService, logger *zap.Logger) *ExecutionHandler {
	return &ExecutionHandler{
		executionService: executionService,
		logger:           logger,
	}
}

// GetExecutions handles GET /api/v1/executions
func (h *ExecutionHandler) GetExecutions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse query parameters
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	// Set defaults
	limit := 50
	offset := 0

	// Parse limit
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err != nil {
			h.writeErrorResponse(w, http.StatusBadRequest, "invalid limit parameter", err)
			return
		} else {
			limit = parsedLimit
		}
	}

	// Parse offset
	if offsetStr != "" {
		if parsedOffset, err := strconv.Atoi(offsetStr); err != nil {
			h.writeErrorResponse(w, http.StatusBadRequest, "invalid offset parameter", err)
			return
		} else {
			offset = parsedOffset
		}
	}

	// Validate parameters
	if limit < 1 || limit > 1000 {
		h.writeErrorResponse(w, http.StatusBadRequest, "limit must be between 1 and 1000", nil)
		return
	}

	if offset < 0 {
		h.writeErrorResponse(w, http.StatusBadRequest, "offset must be non-negative", nil)
		return
	}

	h.logger.Info("Fetching executions",
		zap.Int("limit", limit),
		zap.Int("offset", offset))

	// Call service
	response, err := h.executionService.List(ctx, limit, offset)
	if err != nil {
		h.logger.Error("Failed to list executions", zap.Error(err))
		h.writeErrorResponse(w, http.StatusInternalServerError, "failed to retrieve executions", err)
		return
	}

	h.writeJSONResponse(w, http.StatusOK, response)
}

// GetExecution handles GET /api/v1/executions/{id}
func (h *ExecutionHandler) GetExecution(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse ID from URL
	idStr := chi.URLParam(r, "id")
	if idStr == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "execution ID is required", nil)
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, "invalid execution ID", err)
		return
	}

	h.logger.Info("Fetching execution by ID", zap.Int("id", id))

	// Call service
	execution, err := h.executionService.GetByID(ctx, id)
	if err != nil {
		if strings.Contains(err.Error(), "execution not found") {
			h.writeErrorResponse(w, http.StatusNotFound, "execution not found", err)
			return
		}
		h.logger.Error("Failed to get execution", zap.Int("id", id), zap.Error(err))
		h.writeErrorResponse(w, http.StatusInternalServerError, "failed to retrieve execution", err)
		return
	}

	h.writeJSONResponse(w, http.StatusOK, execution)
}

// CreateExecutions handles POST /api/v1/executions
func (h *ExecutionHandler) CreateExecutions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse request body
	var executions []domain.ExecutionPostDTO
	if err := json.NewDecoder(r.Body).Decode(&executions); err != nil {
		h.logger.Error("Failed to decode request body", zap.Error(err))
		h.writeErrorResponse(w, http.StatusBadRequest, "invalid request body", err)
		return
	}

	// Validate request
	if len(executions) == 0 {
		h.writeErrorResponse(w, http.StatusBadRequest, "no executions provided", nil)
		return
	}

	if len(executions) > 100 {
		h.writeErrorResponse(w, http.StatusBadRequest, "batch size exceeds maximum of 100 executions", nil)
		return
	}

	h.logger.Info("Creating execution batch", zap.Int("batch_size", len(executions)))

	// Call service
	response, err := h.executionService.CreateBatch(ctx, executions)
	if err != nil {
		h.logger.Error("Failed to create executions", zap.Error(err))
		h.writeErrorResponse(w, http.StatusInternalServerError, "failed to create executions", err)
		return
	}

	// Determine response status based on results
	statusCode := http.StatusCreated
	if response.ErrorCount > 0 && response.ProcessedCount == 0 {
		// All requests failed
		statusCode = http.StatusBadRequest
	} else if response.ErrorCount > 0 {
		// Mixed results
		statusCode = http.StatusMultiStatus
	}

	h.writeJSONResponse(w, statusCode, response)
}

// SendExecutions handles POST /api/v1/executions/send
func (h *ExecutionHandler) SendExecutions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	h.logger.Info("Sending executions to Portfolio Accounting")

	// Call service
	response, err := h.executionService.Send(ctx)
	if err != nil {
		// Check for specific error types
		if err.Error() == "duplicate batch process already started" {
			h.writeErrorResponse(w, http.StatusConflict, "batch process already in progress", err)
			return
		}

		h.logger.Error("Failed to send executions", zap.Error(err))
		h.writeErrorResponse(w, http.StatusInternalServerError, "failed to process executions", err)
		return
	}

	// Determine status code based on response
	statusCode := http.StatusOK
	if response.Status == "error" {
		statusCode = http.StatusInternalServerError
	}

	h.writeJSONResponse(w, statusCode, response)
}

// RetryExecutionRequest represents the request body for retry endpoint
type RetryExecutionRequest struct {
	Filename string `json:"filename" validate:"required"`
}

// RetryExecution handles POST /api/v1/executions/send/retry (Requirements 3.3, 3.4)
func (h *ExecutionHandler) RetryExecution(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	h.logger.Info("Processing retry execution request")

	// Parse and validate request body (Requirement 3.3: request validation)
	var req RetryExecutionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("Failed to decode retry request body", zap.Error(err))
		h.writeErrorResponse(w, http.StatusBadRequest, "invalid request body", err)
		return
	}

	// Validate filename parameter (Requirement 3.3: filename parameter validation)
	if req.Filename == "" {
		h.logger.Error("Filename parameter is required for retry operation")
		h.writeErrorResponse(w, http.StatusBadRequest, "filename parameter is required", nil)
		return
	}

	h.logger.Info("Retrying execution for file",
		zap.String("filename", req.Filename))

	// Call service to retry execution
	response, err := h.executionService.RetryExecution(ctx, req.Filename)
	if err != nil {
		// Check for specific error types
		if strings.Contains(err.Error(), "file does not exist") {
			h.logger.Error("File not found for retry operation",
				zap.String("filename", req.Filename),
				zap.Error(err))
			h.writeErrorResponse(w, http.StatusNotFound, "file not found for retry operation", err)
			return
		}

		if strings.Contains(err.Error(), "failed to check file existence") {
			h.logger.Error("Failed to validate file existence for retry",
				zap.String("filename", req.Filename),
				zap.Error(err))
			h.writeErrorResponse(w, http.StatusInternalServerError, "failed to validate file for retry", err)
			return
		}

		// General retry failure (Requirement 3.4: 500 error for retry failure)
		h.logger.Error("Failed to retry execution",
			zap.String("filename", req.Filename),
			zap.Error(err))
		h.writeErrorResponse(w, http.StatusInternalServerError, "failed to retry execution", err)
		return
	}

	// Determine status code based on response (Requirement 3.3, 3.4: proper HTTP status codes)
	statusCode := http.StatusOK
	if response.Status == "error" {
		statusCode = http.StatusInternalServerError
	}

	h.logger.Info("Retry execution completed",
		zap.String("filename", req.Filename),
		zap.String("status", response.Status),
		zap.Int("status_code", statusCode))

	h.writeJSONResponse(w, statusCode, response)
}

// DeleteLastBatchHistoryResponse represents the response for delete_last endpoint
type DeleteLastBatchHistoryResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// DeleteLastBatchHistory handles POST /api/v1/executions/send/delete_last (Requirements 4.2, 4.3)
func (h *ExecutionHandler) DeleteLastBatchHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	h.logger.Info("Processing delete last batch history request")

	// Call service to delete last batch history record
	err := h.executionService.DeleteLastBatchHistory(ctx)
	if err != nil {
		// Check for specific error types and provide appropriate HTTP status codes (Requirement 4.2, 4.3)
		if strings.Contains(err.Error(), "no batch history records exist") {
			h.logger.Error("No batch history records exist to delete", zap.Error(err))
			h.writeErrorResponse(w, http.StatusNotFound, "no batch history records exist to delete", err)
			return
		}

		if strings.Contains(err.Error(), "validation failed") {
			h.logger.Error("Batch history deletion validation failed", zap.Error(err))
			h.writeErrorResponse(w, http.StatusConflict, "validation failed - record is not the latest", err)
			return
		}

		if strings.Contains(err.Error(), "was already deleted or does not exist") {
			h.logger.Error("Batch history record was already deleted", zap.Error(err))
			h.writeErrorResponse(w, http.StatusNotFound, "batch history record was already deleted", err)
			return
		}

		// General deletion failure (Requirement 4.3: 500 error for deletion failure)
		h.logger.Error("Failed to delete last batch history record", zap.Error(err))
		h.writeErrorResponse(w, http.StatusInternalServerError, "failed to delete last batch history record", err)
		return
	}

	// Success response (Requirement 4.2: 200 success code)
	response := DeleteLastBatchHistoryResponse{
		Status:  "success",
		Message: "Last batch history record deleted successfully",
	}

	h.logger.Info("Last batch history record deleted successfully")

	h.writeJSONResponse(w, http.StatusOK, response)
}

// writeJSONResponse writes a JSON response with the given status code
func (h *ExecutionHandler) writeJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("Failed to encode JSON response", zap.Error(err))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// writeErrorResponse writes a standardized error response
func (h *ExecutionHandler) writeErrorResponse(w http.ResponseWriter, statusCode int, message string, err error) {
	errorResponse := domain.ErrorResponse{
		Message:   message,
		Status:    statusCode,
		Timestamp: domain.GetCurrentTimestamp(),
	}

	// Add error details for debugging (but not in production)
	if err != nil {
		h.logger.Error("API Error",
			zap.String("message", message),
			zap.Int("status", statusCode),
			zap.Error(err))

		// Only include error details in development
		errorResponse.Details = err.Error()
	}

	h.writeJSONResponse(w, statusCode, errorResponse)
}
