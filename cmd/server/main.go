package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"

	"github.com/kasbench/globeco-allocation-service/internal/config"
	"github.com/kasbench/globeco-allocation-service/internal/handler"
	internalMiddleware "github.com/kasbench/globeco-allocation-service/internal/middleware"
	"github.com/kasbench/globeco-allocation-service/internal/repository"
	"github.com/kasbench/globeco-allocation-service/internal/service"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize logger
	logger, err := initLogger(cfg.LogLevel)
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Sync()

	logger.Info("Starting Allocation Service",
		zap.String("version", "1.0.0"),
		zap.Int("port", cfg.Port))

	// Initialize database connection
	db, err := repository.NewPostgresDB(cfg.Database)
	if err != nil {
		logger.Fatal("Failed to connect to database", zap.Error(err))
	}
	defer db.Close()

	// Initialize repositories
	executionRepo := repository.NewExecutionRepository(db, logger)
	batchHistoryRepo := repository.NewBatchHistoryRepository(db, logger)

	// Initialize services
	tradeServiceClient := service.NewTradeServiceClient(cfg.TradeServiceURL, logger)
	executionService := service.NewExecutionService(executionRepo, batchHistoryRepo, tradeServiceClient, logger)

	// Initialize handlers
	executionHandler := handler.NewExecutionHandler(executionService, logger)
	healthHandler := handler.NewHealthHandler(db, logger)

	// Setup router
	r := setupRouter(cfg, logger, executionHandler, healthHandler)

	// Setup HTTP server
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		logger.Info("HTTP server starting", zap.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatal("Server forced to shutdown", zap.Error(err))
	}

	logger.Info("Server exited")
}

func initLogger(level string) (*zap.Logger, error) {
	var cfg zap.Config
	if level == "debug" {
		cfg = zap.NewDevelopmentConfig()
	} else {
		cfg = zap.NewProductionConfig()
	}

	switch level {
	case "debug":
		cfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		cfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		cfg.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		cfg.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	default:
		cfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}

	return cfg.Build()
}

func setupRouter(cfg *config.Config, logger *zap.Logger, executionHandler *handler.ExecutionHandler, healthHandler *handler.HealthHandler) *chi.Mux {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(internalMiddleware.Logger(logger))
	r.Use(middleware.Recoverer)
	r.Use(internalMiddleware.CORS())
	if cfg.MetricsEnabled {
		r.Use(internalMiddleware.Metrics())
	}

	// Health check endpoints
	r.Get("/healthz", healthHandler.Liveness)
	r.Get("/readyz", healthHandler.Readiness)

	// Metrics endpoint
	if cfg.MetricsEnabled {
		r.Handle("/metrics", internalMiddleware.MetricsHandler())
	}

	// API routes
	r.Route("/api/v1", func(r chi.Router) {
		r.Route("/executions", func(r chi.Router) {
			r.Get("/", executionHandler.List)
			r.Post("/", executionHandler.CreateBatch)
			r.Get("/{id}", executionHandler.GetByID)
			r.Post("/send", executionHandler.Send)
		})
	})

	return r
}
