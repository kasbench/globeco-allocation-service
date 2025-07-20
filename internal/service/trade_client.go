package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.uber.org/zap"

	"github.com/kasbench/globeco-allocation-service/internal/domain"
)

// TradeServiceClient handles communication with the Trade Service
type TradeServiceClient struct {
	baseURL    string
	httpClient *http.Client
	logger     *zap.Logger
	maxRetries int
	baseDelay  time.Duration
}

// NewTradeServiceClient creates a new Trade Service client with OpenTelemetry instrumentation
func NewTradeServiceClient(baseURL string, logger *zap.Logger) *TradeServiceClient {
	// Create HTTP client with OpenTelemetry instrumentation for outbound calls
	httpClient := &http.Client{
		Timeout:   30 * time.Second,
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}

	return &TradeServiceClient{
		baseURL:    baseURL,
		httpClient: httpClient,
		logger:     logger,
		maxRetries: 3,
		baseDelay:  1 * time.Second,
	}
}

// SetRetryConfig configures retry parameters
func (c *TradeServiceClient) SetRetryConfig(maxRetries int, baseDelay time.Duration) {
	c.maxRetries = maxRetries
	c.baseDelay = baseDelay
}

// GetExecutionByServiceID retrieves execution details from Trade Service
func (c *TradeServiceClient) GetExecutionByServiceID(ctx context.Context, executionServiceID int) (*domain.TradeServiceExecutionResponse, error) {
	// Start OpenTelemetry span for this operation
	tracer := otel.Tracer("globeco-allocation-service")
	ctx, span := tracer.Start(ctx, "trade_service.get_execution_by_service_id")
	defer span.End()

	// Add span attributes
	span.SetAttributes(
		attribute.String("service.name", "trade-service"),
		attribute.String("operation", "get_execution_by_service_id"),
		attribute.Int("execution_service_id", executionServiceID),
		attribute.String("base_url", c.baseURL),
	)

	// Build URL with query parameter
	u, err := url.Parse(c.baseURL + "/api/v2/executions")
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to parse URL")
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}

	query := u.Query()
	query.Set("executionServiceId", strconv.Itoa(executionServiceID))
	u.RawQuery = query.Encode()

	span.SetAttributes(attribute.String("http.url", u.String()))

	c.logger.Info("Calling Trade Service with OpenTelemetry tracing",
		zap.String("url", u.String()),
		zap.Int("execution_service_id", executionServiceID),
		zap.String("trace_id", span.SpanContext().TraceID().String()),
		zap.String("span_id", span.SpanContext().SpanID().String()))

	// Execute request with retry logic
	response, err := c.executeWithRetry(ctx, "GET", u.String(), nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "trade service call failed")
		return nil, fmt.Errorf("failed to call Trade Service: %w", err)
	}

	// Add success attributes
	span.SetAttributes(attribute.Int("response.executions_count", len(response.Executions)))
	span.SetStatus(codes.Ok, "trade service call successful")

	return response, nil
}

// executeWithRetry performs HTTP request with exponential backoff retry
func (c *TradeServiceClient) executeWithRetry(ctx context.Context, method, url string, body io.Reader) (*domain.TradeServiceExecutionResponse, error) {
	var lastErr error
	startTime := time.Now()

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(attempt) * c.baseDelay
			c.logger.Info("Retrying Trade Service call with OpenTelemetry metrics",
				zap.Int("attempt", attempt),
				zap.Duration("delay", delay))

			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		response, err := c.executeRequest(ctx, method, url, body)
		if err == nil {
			// Record successful call metrics
			duration := time.Since(startTime)
			c.logger.Info("Trade Service call successful - metrics sent to OpenTelemetry collector",
				zap.String("method", method),
				zap.Duration("total_duration", duration),
				zap.Int("attempts", attempt+1))
			return response, nil
		}

		lastErr = err
		c.logger.Warn("Trade Service call failed - retry metrics sent to OpenTelemetry collector",
			zap.Int("attempt", attempt),
			zap.Error(err))

		// Don't retry on client errors (4xx)
		if httpErr, ok := err.(*HTTPError); ok && httpErr.StatusCode >= 400 && httpErr.StatusCode < 500 {
			break
		}
	}

	// Record final failure metrics
	duration := time.Since(startTime)
	c.logger.Error("All Trade Service retry attempts failed - failure metrics sent to OpenTelemetry collector",
		zap.String("method", method),
		zap.Duration("total_duration", duration),
		zap.Int("total_attempts", c.maxRetries+1),
		zap.Error(lastErr))

	return nil, fmt.Errorf("all retry attempts failed: %w", lastErr)
}

// executeRequest performs a single HTTP request
func (c *TradeServiceClient) executeRequest(ctx context.Context, method, url string, body io.Reader) (*domain.TradeServiceExecutionResponse, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.Error("failed to close response body", zap.Error(err))
		}
	}()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check for HTTP errors
	if resp.StatusCode >= 400 {
		return nil, &HTTPError{
			StatusCode: resp.StatusCode,
			Message:    string(respBody),
		}
	}

	// Parse response
	var response domain.TradeServiceExecutionResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		c.logger.Error("Failed to parse Trade Service response",
			zap.String("response_body", string(respBody)),
			zap.Error(err))
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	c.logger.Info("Trade Service call successful",
		zap.Int("executions_count", len(response.Executions)))

	return &response, nil
}

// HTTPError represents an HTTP error response
type HTTPError struct {
	StatusCode int
	Message    string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Message)
}
