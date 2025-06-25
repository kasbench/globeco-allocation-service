package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/kasbench/globeco-allocation-service/internal/service"
)

// ExecutionHandler handles HTTP requests for executions
type ExecutionHandler struct {
	service *service.ExecutionService
	logger  *zap.Logger
}

// NewExecutionHandler creates a new execution handler
func NewExecutionHandler(service *service.ExecutionService, logger *zap.Logger) *ExecutionHandler {
	return &ExecutionHandler{
		service: service,
		logger:  logger,
	}
}

// List handles GET /api/v1/executions
func (h *ExecutionHandler) List(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement list endpoint
	// This will be implemented in Phase 4
	w.WriteHeader(http.StatusNotImplemented)
	json.NewEncoder(w).Encode(map[string]string{"message": "Not implemented yet"})
}

// GetByID handles GET /api/v1/executions/{id}
func (h *ExecutionHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement get by ID endpoint
	// This will be implemented in Phase 4
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid ID"})
		return
	}

	h.logger.Info("GetByID called", zap.Int("id", id))
	w.WriteHeader(http.StatusNotImplemented)
	json.NewEncoder(w).Encode(map[string]string{"message": "Not implemented yet"})
}

// CreateBatch handles POST /api/v1/executions
func (h *ExecutionHandler) CreateBatch(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement create batch endpoint
	// This will be implemented in Phase 4
	w.WriteHeader(http.StatusNotImplemented)
	json.NewEncoder(w).Encode(map[string]string{"message": "Not implemented yet"})
}

// Send handles POST /api/v1/executions/send
func (h *ExecutionHandler) Send(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement send endpoint
	// This will be implemented in Phase 4
	w.WriteHeader(http.StatusNotImplemented)
	json.NewEncoder(w).Encode(map[string]string{"message": "Not implemented yet"})
}
