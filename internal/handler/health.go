package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/kasbench/globeco-allocation-service/internal/domain"
	"github.com/kasbench/globeco-allocation-service/internal/repository"
)

// HealthHandler handles health check endpoints
type HealthHandler struct {
	db     *repository.DB
	logger *zap.Logger
}

// NewHealthHandler creates a new health handler
func NewHealthHandler(db *repository.DB, logger *zap.Logger) *HealthHandler {
	return &HealthHandler{
		db:     db,
		logger: logger,
	}
}

// Liveness handles the liveness probe endpoint
func (h *HealthHandler) Liveness(w http.ResponseWriter, r *http.Request) {
	response := domain.HealthResponse{
		Status:    "ok",
		Timestamp: time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Error("Failed to encode liveness response", zap.Error(err))
	}
}

// Readiness handles the readiness probe endpoint
func (h *HealthHandler) Readiness(w http.ResponseWriter, r *http.Request) {
	checks := make(map[string]string)
	status := "ok"
	statusCode := http.StatusOK

	// Check database connection
	if err := h.db.HealthCheck(); err != nil {
		checks["database"] = "unhealthy: " + err.Error()
		status = "error"
		statusCode = http.StatusServiceUnavailable
		h.logger.Error("Database health check failed", zap.Error(err))
	} else {
		checks["database"] = "healthy"
	}

	response := domain.HealthResponse{
		Status:    status,
		Timestamp: time.Now(),
		Checks:    checks,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Error("Failed to encode readiness response", zap.Error(err))
	}
}
