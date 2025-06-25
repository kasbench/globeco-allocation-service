package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// ContextKey is the type for context keys used in logging
type ContextKey string

const (
	// CorrelationIDKey is the context key for correlation ID
	CorrelationIDKey ContextKey = "correlation_id"
	// RequestIDKey is the context key for request ID
	RequestIDKey ContextKey = "request_id"
	// UserIDKey is the context key for user ID
	UserIDKey ContextKey = "user_id"
	// TraceIDKey is the context key for trace ID
	TraceIDKey ContextKey = "trace_id"
	// SpanIDKey is the context key for span ID
	SpanIDKey ContextKey = "span_id"
)

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level               string
	Format              string // "json" or "console"
	EnableCaller        bool
	EnableStacktrace    bool
	Development         bool
	DisableSampling     bool
	OutputPaths         []string
	ErrorOutputPaths    []string
	CorrelationIDHeader string
	InitialFields       map[string]interface{}
}

// StructuredLogger provides enhanced structured logging capabilities
type StructuredLogger struct {
	logger *zap.Logger
	sugar  *zap.SugaredLogger
	config LoggingConfig
}

// NewStructuredLogger creates a new structured logger
func NewStructuredLogger(config LoggingConfig) (*StructuredLogger, error) {
	// Set defaults
	if config.Level == "" {
		config.Level = "info"
	}
	if config.Format == "" {
		config.Format = "json"
	}
	if config.CorrelationIDHeader == "" {
		config.CorrelationIDHeader = "X-Correlation-ID"
	}
	if len(config.OutputPaths) == 0 {
		config.OutputPaths = []string{"stdout"}
	}
	if len(config.ErrorOutputPaths) == 0 {
		config.ErrorOutputPaths = []string{"stderr"}
	}

	// Create zap config
	var zapConfig zap.Config
	if config.Development {
		zapConfig = zap.NewDevelopmentConfig()
	} else {
		zapConfig = zap.NewProductionConfig()
	}

	// Set level
	level, err := zapcore.ParseLevel(config.Level)
	if err != nil {
		return nil, fmt.Errorf("invalid log level %s: %w", config.Level, err)
	}
	zapConfig.Level = zap.NewAtomicLevelAt(level)

	// Set format
	if config.Format == "console" {
		zapConfig.Encoding = "console"
		zapConfig.EncoderConfig = zap.NewDevelopmentEncoderConfig()
	} else {
		zapConfig.Encoding = "json"
		zapConfig.EncoderConfig = zap.NewProductionEncoderConfig()
	}

	// Configure time encoding
	zapConfig.EncoderConfig.TimeKey = "timestamp"
	zapConfig.EncoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder

	// Configure caller and stacktrace
	zapConfig.DisableCaller = !config.EnableCaller
	zapConfig.DisableStacktrace = !config.EnableStacktrace

	// Set output paths
	zapConfig.OutputPaths = config.OutputPaths
	zapConfig.ErrorOutputPaths = config.ErrorOutputPaths

	// Disable sampling if requested
	if config.DisableSampling {
		zapConfig.Sampling = nil
	}

	// Add initial fields
	if len(config.InitialFields) > 0 {
		fields := make([]zap.Field, 0, len(config.InitialFields))
		for key, value := range config.InitialFields {
			fields = append(fields, zap.Any(key, value))
		}
		zapConfig.InitialFields = map[string]interface{}{}
		for key, value := range config.InitialFields {
			zapConfig.InitialFields[key] = value
		}
	}

	// Build logger
	logger, err := zapConfig.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build logger: %w", err)
	}

	return &StructuredLogger{
		logger: logger,
		sugar:  logger.Sugar(),
		config: config,
	}, nil
}

// GenerateCorrelationID generates a new correlation ID
func GenerateCorrelationID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp-based ID
		return fmt.Sprintf("corr_%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}

// WithCorrelationID adds a correlation ID to the context
func WithCorrelationID(ctx context.Context, correlationID string) context.Context {
	return context.WithValue(ctx, CorrelationIDKey, correlationID)
}

// WithRequestID adds a request ID to the context
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

// WithUserID adds a user ID to the context
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, UserIDKey, userID)
}

// GetCorrelationID extracts the correlation ID from context
func GetCorrelationID(ctx context.Context) string {
	if id, ok := ctx.Value(CorrelationIDKey).(string); ok {
		return id
	}
	return ""
}

// GetRequestID extracts the request ID from context
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(RequestIDKey).(string); ok {
		return id
	}
	return ""
}

// WithContext returns a logger with context fields
func (l *StructuredLogger) WithContext(ctx context.Context) *zap.Logger {
	fields := make([]zap.Field, 0, 4)

	if correlationID := GetCorrelationID(ctx); correlationID != "" {
		fields = append(fields, zap.String("correlation_id", correlationID))
	}

	if requestID := GetRequestID(ctx); requestID != "" {
		fields = append(fields, zap.String("request_id", requestID))
	}

	if userID, ok := ctx.Value(UserIDKey).(string); ok && userID != "" {
		fields = append(fields, zap.String("user_id", userID))
	}

	if traceID, ok := ctx.Value(TraceIDKey).(string); ok && traceID != "" {
		fields = append(fields, zap.String("trace_id", traceID))
	}

	if spanID, ok := ctx.Value(SpanIDKey).(string); ok && spanID != "" {
		fields = append(fields, zap.String("span_id", spanID))
	}

	return l.logger.With(fields...)
}

// WithFields returns a logger with additional fields
func (l *StructuredLogger) WithFields(fields ...zap.Field) *zap.Logger {
	return l.logger.With(fields...)
}

// Logger returns the underlying zap logger
func (l *StructuredLogger) Logger() *zap.Logger {
	return l.logger
}

// Sugar returns the sugared logger
func (l *StructuredLogger) Sugar() *zap.SugaredLogger {
	return l.sugar
}

// Sync flushes any buffered log entries
func (l *StructuredLogger) Sync() error {
	return l.logger.Sync()
}

// CorrelationIDMiddleware is a middleware that adds correlation ID to requests
func (l *StructuredLogger) CorrelationIDMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get correlation ID from header or generate new one
			correlationID := r.Header.Get(l.config.CorrelationIDHeader)
			if correlationID == "" {
				correlationID = GenerateCorrelationID()
			}

			// Add to response header
			w.Header().Set(l.config.CorrelationIDHeader, correlationID)

			// Add to context
			ctx := WithCorrelationID(r.Context(), correlationID)
			r = r.WithContext(ctx)

			// Continue with next handler
			next.ServeHTTP(w, r)
		})
	}
}

// LogExecutionProcessing logs execution processing events with context
func (l *StructuredLogger) LogExecutionProcessing(ctx context.Context, level zapcore.Level, msg string, fields ...zap.Field) {
	logger := l.WithContext(ctx)

	// Add execution-specific fields
	contextFields := []zap.Field{
		zap.String("component", "execution_service"),
		zap.String("operation", "processing"),
	}

	allFields := append(contextFields, fields...)
	logger.Log(level, msg, allFields...)
}

// LogTradeServiceCall logs Trade Service API calls with context
func (l *StructuredLogger) LogTradeServiceCall(ctx context.Context, level zapcore.Level, msg string, fields ...zap.Field) {
	logger := l.WithContext(ctx)

	// Add trade service specific fields
	contextFields := []zap.Field{
		zap.String("component", "trade_service"),
		zap.String("operation", "api_call"),
	}

	allFields := append(contextFields, fields...)
	logger.Log(level, msg, allFields...)
}

// LogDatabaseOperation logs database operations with context
func (l *StructuredLogger) LogDatabaseOperation(ctx context.Context, level zapcore.Level, msg string, fields ...zap.Field) {
	logger := l.WithContext(ctx)

	// Add database specific fields
	contextFields := []zap.Field{
		zap.String("component", "database"),
		zap.String("operation", "query"),
	}

	allFields := append(contextFields, fields...)
	logger.Log(level, msg, allFields...)
}

// LogPortfolioOperation logs portfolio accounting operations with context
func (l *StructuredLogger) LogPortfolioOperation(ctx context.Context, level zapcore.Level, msg string, fields ...zap.Field) {
	logger := l.WithContext(ctx)

	// Add portfolio specific fields
	contextFields := []zap.Field{
		zap.String("component", "portfolio_service"),
		zap.String("operation", "processing"),
	}

	allFields := append(contextFields, fields...)
	logger.Log(level, msg, allFields...)
}

// LogHTTPRequest logs HTTP requests with standard fields
func (l *StructuredLogger) LogHTTPRequest(ctx context.Context, method, path string, statusCode int, duration time.Duration, fields ...zap.Field) {
	logger := l.WithContext(ctx)

	// Add HTTP specific fields
	contextFields := []zap.Field{
		zap.String("component", "http_server"),
		zap.String("operation", "request"),
		zap.String("method", method),
		zap.String("path", path),
		zap.Int("status_code", statusCode),
		zap.Duration("duration", duration),
	}

	allFields := append(contextFields, fields...)

	// Log at different levels based on status code
	var level zapcore.Level
	var msg string
	switch {
	case statusCode >= 500:
		level = zapcore.ErrorLevel
		msg = "HTTP request failed with server error"
	case statusCode >= 400:
		level = zapcore.WarnLevel
		msg = "HTTP request failed with client error"
	default:
		level = zapcore.InfoLevel
		msg = "HTTP request completed successfully"
	}

	logger.Log(level, msg, allFields...)
}
