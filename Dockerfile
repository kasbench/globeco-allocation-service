# Build stage
FROM golang:1.23-alpine AS builder

# Install git and ca-certificates for build dependencies
RUN apk add --no-cache git ca-certificates tzdata make

# Set working directory
WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build the application with optimizations
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -a -installsuffix cgo \
    -ldflags='-w -s -extldflags "-static"' \
    -o main ./cmd/server

# Test stage (optional, can be skipped in production builds)
FROM builder AS tester
RUN go test ./... -short

# Final stage - minimal production image
FROM scratch AS production

# Import ca-certificates from builder stage
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Import timezone data
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy the binary from builder stage
COPY --from=builder /app/main /main

# Copy migrations
COPY --from=builder /app/migrations /migrations

# Copy OpenAPI spec
COPY --from=builder /app/openapi.yaml /openapi.yaml

# Create directory structure for non-root user
# Note: Using scratch means we need to create these at runtime

# Expose port
EXPOSE 8089

# Add metadata labels
LABEL maintainer="noah@kasbench.org"
LABEL version="1.0.0"
LABEL description="GlobeCo Allocation Service"
LABEL org.opencontainers.image.source="https://github.com/kasbench/globeco-allocation-service"

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD ["/main", "--health-check"] || exit 1

# Run the application
ENTRYPOINT ["/main"]

# Development stage for local development
FROM alpine:3.19 AS development

# Install development tools
RUN apk add --no-cache ca-certificates tzdata curl wget

# Create non-root user
RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

WORKDIR /app

# Copy the binary from builder stage
COPY --from=builder /app/main .

# Copy migrations
COPY --from=builder /app/migrations /migrations

# Copy OpenAPI spec
COPY --from=builder /app/openapi.yaml /openapi.yaml

# Create the output directory with proper permissions
RUN mkdir -p /usr/local/share/files && \
    chown -R appuser:appgroup /usr/local/share/files && \
    chown -R appuser:appgroup /app

# Switch to non-root user
USER appuser

# Expose port
EXPOSE 8089

# Health check using wget
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8089/healthz || exit 1

# Run the application
CMD ["./main"] 