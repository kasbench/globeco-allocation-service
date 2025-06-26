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
	"github.com/kasbench/globeco-allocation-service/internal/observability"
	"github.com/kasbench/globeco-allocation-service/internal/repository"
	"github.com/kasbench/globeco-allocation-service/internal/service"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize enhanced structured logger
	structuredLogger, err := initStructuredLogger(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize structured logger: %v", err)
	}
	defer func() {
		if err := structuredLogger.Sync(); err != nil {
			log.Printf("Failed to sync logger: %v", err)
		}
	}()

	logger := structuredLogger.Logger()
	logger.Info("Starting Allocation Service",
		zap.String("version", "1.0.0"),
		zap.Int("port", cfg.Port))

	// Initialize tracing
	tracingManager, err := observability.NewTracingManager(observability.TracingConfig{
		Enabled:        cfg.Observability.TracingEnabled,
		OTLPEndpoint:   cfg.Observability.TracingOTLPEndpoint,
		SamplingRatio:  cfg.Observability.TracingSamplingRatio,
		TracingHeaders: cfg.Observability.TracingHeaders,
	}, logger)
	if err != nil {
		logger.Fatal("Failed to initialize tracing", zap.Error(err))
	}

	// Initialize business metrics
	businessMetrics := observability.NewBusinessMetrics(logger)

	// Initialize database connection
	db, err := repository.NewPostgresDB(cfg.Database)
	if err != nil {
		logger.Fatal("Failed to connect to database", zap.Error(err))
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Error("Failed to close database", zap.Error(err))
		}
	}()

	// Initialize repositories
	executionRepo := repository.NewExecutionRepository(db, logger)
	batchHistoryRepo := repository.NewBatchHistoryRepository(db, logger)

	// Initialize services with metrics integration
	tradeClient := service.NewTradeServiceClient(cfg.TradeServiceURL, logger)
	tradeClient.SetRetryConfig(cfg.RetryMaxAttempts, time.Duration(cfg.RetryBaseDelay)*time.Millisecond)

	executionService := service.NewExecutionService(
		executionRepo,
		batchHistoryRepo,
		tradeClient,
		logger,
		cfg,
	)

	// Initialize handlers with structured logging
	executionHandler := handler.NewExecutionHandler(executionService, logger)
	healthHandler := handler.NewHealthHandler(db, logger)

	// Setup router with observability middleware
	r := setupRouterWithObservability(cfg, structuredLogger, businessMetrics, executionHandler, healthHandler)

	// Serve OpenAPI spec (YAML)
	r.Get("/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		http.ServeFile(w, r, "openapi.yaml")
	})

	// Serve Swagger UI
	r.Get("/swagger-ui/*", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/swagger-ui/" || r.URL.Path == "/swagger-ui" {
			http.Redirect(w, r, "/swagger-ui/index.html", http.StatusFound)
			return
		}
		if r.URL.Path == "/swagger-ui/index.html" {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Swagger UI</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.17.12/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5.17.12/swagger-ui-bundle.js"></script>
  <script>
    window.onload = function() {
      window.ui = SwaggerUIBundle({
        url: window.location.protocol + '//' + window.location.hostname + ':8089/openapi.yaml',
        dom_id: '#swagger-ui',
      });
    };
  </script>
</body>
</html>`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

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

	// Shutdown tracing
	if tracingManager != nil {
		if err := tracingManager.Shutdown(ctx); err != nil {
			logger.Error("Failed to shutdown tracing", zap.Error(err))
		}
	}

	// Shutdown HTTP server
	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatal("Server forced to shutdown", zap.Error(err))
	}

	logger.Info("Server exited")
}

func initStructuredLogger(cfg *config.Config) (*observability.StructuredLogger, error) {
	loggingConfig := observability.LoggingConfig{
		Level:               cfg.LogLevel,
		Format:              cfg.Observability.LogFormat,
		EnableCaller:        cfg.Observability.LogEnableCaller,
		EnableStacktrace:    cfg.Observability.LogEnableStacktrace,
		Development:         cfg.Observability.LogDevelopment,
		DisableSampling:     cfg.Observability.LogDisableSampling,
		CorrelationIDHeader: cfg.Observability.LogCorrelationHeader,
		InitialFields: map[string]interface{}{
			"service":     "globeco-allocation-service",
			"version":     "1.0.0",
			"environment": "production",
		},
	}

	return observability.NewStructuredLogger(loggingConfig)
}

func setupRouterWithObservability(
	cfg *config.Config,
	structuredLogger *observability.StructuredLogger,
	metrics *observability.BusinessMetrics,
	executionHandler *handler.ExecutionHandler,
	healthHandler *handler.HealthHandler,
) *chi.Mux {
	r := chi.NewRouter()

	// Core middleware
	r.Use(middleware.RequestID)
	r.Use(structuredLogger.CorrelationIDMiddleware())
	r.Use(internalMiddleware.Logger(structuredLogger.Logger()))
	r.Use(middleware.Recoverer)
	r.Use(internalMiddleware.CORS())

	// Metrics middleware
	if cfg.Observability.MetricsEnabled {
		r.Use(internalMiddleware.Metrics())
	}

	// Health check endpoints
	r.Get("/healthz", healthHandler.Liveness)
	r.Get("/readyz", healthHandler.Readiness)

	// Metrics endpoint
	if cfg.Observability.MetricsEnabled {
		metricsPath := cfg.Observability.MetricsPath
		if metricsPath == "" {
			metricsPath = "/metrics"
		}
		r.Handle(metricsPath, internalMiddleware.MetricsHandler())
	}

	// API routes
	r.Route("/api/v1", func(r chi.Router) {
		r.Route("/executions", func(r chi.Router) {
			r.Get("/", executionHandler.GetExecutions)
			r.Post("/", executionHandler.CreateExecutions)
			r.Get("/{id}", executionHandler.GetExecution)
			r.Post("/send", executionHandler.SendExecutions)
		})
	})

	return r
}
