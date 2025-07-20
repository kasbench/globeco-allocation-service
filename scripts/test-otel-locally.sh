#!/bin/bash

# Local OpenTelemetry Testing Script
# This script demonstrates how to test OTEL instrumentation locally

set -e

echo "🧪 Testing OpenTelemetry Instrumentation Locally"
echo "================================================"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

print_info() {
    echo -e "${BLUE}ℹ️  $1${NC}"
}

print_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

echo
print_info "Setting up environment variables for local testing..."

# Set environment variables for local testing
export OBSERVABILITY_OTEL_ENABLED=true
export OBSERVABILITY_OTEL_SERVICE_NAME=globeco-allocation-service
export OBSERVABILITY_OTEL_SERVICE_VERSION=1.0.0
export OBSERVABILITY_OTEL_SERVICE_NAMESPACE=globeco
export OBSERVABILITY_OTEL_ENDPOINT=localhost:4317

# Database configuration for local testing
export DATABASE_HOST=localhost
export DATABASE_PORT=5432
export DATABASE_NAME=postgres
export DATABASE_USER=postgres
export DATABASE_PASSWORD=password
export DATABASE_SSL_MODE=disable

# Service configuration
export PORT=8089
export LOG_LEVEL=info
export TRADE_SERVICE_URL=http://localhost:8082

print_success "Environment variables configured"

echo
print_info "Building the service..."
if go build -o ./bin/globeco-allocation-service ./cmd/server; then
    print_success "Service built successfully"
else
    echo -e "${RED}❌ Build failed${NC}"
    exit 1
fi

echo
print_warning "Prerequisites for local testing:"
echo "1. PostgreSQL running on localhost:5432"
echo "2. OpenTelemetry Collector running on localhost:4317 (optional)"
echo "3. Trade Service running on localhost:8082 (optional)"
echo

print_info "Starting the service with OpenTelemetry instrumentation..."
echo "Service will start on http://localhost:8089"
echo
print_info "Expected OTEL log messages:"
echo "- 'OpenTelemetry initialized successfully with GlobeCo standards'"
echo "- 'OTLP trace exporter created successfully'"
echo "- 'OTLP metric exporter created successfully'"
echo
print_info "To test the instrumentation:"
echo "1. Make HTTP requests to the service endpoints"
echo "2. Check logs for trace IDs and span IDs"
echo "3. Verify metrics are being collected"
echo
print_info "Example API calls to test:"
echo "curl http://localhost:8089/healthz"
echo "curl http://localhost:8089/api/v1/executions"
echo "curl -X POST http://localhost:8089/api/v1/executions -H 'Content-Type: application/json' -d '{}'"
echo
print_warning "Press Ctrl+C to stop the service"
echo

# Start the service
./bin/globeco-allocation-service