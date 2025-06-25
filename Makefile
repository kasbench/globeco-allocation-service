# GlobeCo Allocation Service Makefile
# Author: Noah Krieger (noah@kasbench.org)

# Configuration
APP_NAME := globeco-allocation-service
VERSION := 1.0.0
DOCKER_REGISTRY := docker.io
DOCKER_IMAGE := $(DOCKER_REGISTRY)/$(APP_NAME)
NAMESPACE := default
ENVIRONMENT := development

# Docker build configurations
DOCKER_BUILD_TARGET := production
DOCKER_BUILD_ARGS := --no-cache

# Go configurations
GO_VERSION := 1.21
GOFLAGS := -v
GOLINT_MIN_CONFIDENCE := 0.8

# Test configurations
TEST_TIMEOUT := 10m
COVERAGE_THRESHOLD := 80

# Kubernetes configurations
KUBECTL_TIMEOUT := 300s

# Color output
BLUE := \033[34m
GREEN := \033[32m
YELLOW := \033[33m
RED := \033[31m
NC := \033[0m

.PHONY: help
help: ## Show help message
	@echo "$(BLUE)GlobeCo Allocation Service - Available Commands$(NC)"
	@echo ""
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make \033[36m<target>\033[0m\n\nTargets:\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

# Development targets
.PHONY: setup
setup: ## Set up development environment
	@echo "$(BLUE)Setting up development environment...$(NC)"
	go mod download
	go mod verify
	@echo "$(GREEN)Development environment ready$(NC)"

.PHONY: deps
deps: ## Install dependencies
	@echo "$(BLUE)Installing dependencies...$(NC)"
	go mod tidy
	go mod download
	@echo "$(GREEN)Dependencies installed$(NC)"

.PHONY: deps-update
deps-update: ## Update dependencies
	@echo "$(BLUE)Updating dependencies...$(NC)"
	go get -u ./...
	go mod tidy
	@echo "$(GREEN)Dependencies updated$(NC)"

# Build targets
.PHONY: build
build: ## Build the application
	@echo "$(BLUE)Building application...$(NC)"
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-ldflags='-w -s -X main.version=$(VERSION)' \
		-o bin/$(APP_NAME) \
		./cmd/server
	@echo "$(GREEN)Build completed$(NC)"

.PHONY: build-dev
build-dev: ## Build for development
	@echo "$(BLUE)Building for development...$(NC)"
	go build -o bin/$(APP_NAME)-dev ./cmd/server
	@echo "$(GREEN)Development build completed$(NC)"

.PHONY: clean
clean: ## Clean build artifacts
	@echo "$(BLUE)Cleaning build artifacts...$(NC)"
	rm -rf bin/
	rm -rf coverage.out
	rm -rf coverage.html
	go clean -cache
	@echo "$(GREEN)Clean completed$(NC)"

# Testing targets
.PHONY: test
test: ## Run tests
	@echo "$(BLUE)Running tests...$(NC)"
	go test $(GOFLAGS) -timeout $(TEST_TIMEOUT) ./...
	@echo "$(GREEN)Tests completed$(NC)"

.PHONY: test-short
test-short: ## Run short tests only
	@echo "$(BLUE)Running short tests...$(NC)"
	go test $(GOFLAGS) -short -timeout 30s ./...
	@echo "$(GREEN)Short tests completed$(NC)"

.PHONY: test-coverage
test-coverage: ## Run tests with coverage
	@echo "$(BLUE)Running tests with coverage...$(NC)"
	go test $(GOFLAGS) -timeout $(TEST_TIMEOUT) -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	go tool cover -func=coverage.out
	@echo "$(GREEN)Coverage report generated: coverage.html$(NC)"

.PHONY: test-integration
test-integration: ## Run integration tests
	@echo "$(BLUE)Running integration tests...$(NC)"
	go test $(GOFLAGS) -tags=integration -timeout $(TEST_TIMEOUT) ./...
	@echo "$(GREEN)Integration tests completed$(NC)"

.PHONY: benchmark
benchmark: ## Run benchmarks
	@echo "$(BLUE)Running benchmarks...$(NC)"
	go test -bench=. -benchmem ./...
	@echo "$(GREEN)Benchmarks completed$(NC)"

# Code quality targets
.PHONY: fmt
fmt: ## Format code
	@echo "$(BLUE)Formatting code...$(NC)"
	go fmt ./...
	@echo "$(GREEN)Code formatted$(NC)"

.PHONY: vet
vet: ## Run go vet
	@echo "$(BLUE)Running go vet...$(NC)"
	go vet ./...
	@echo "$(GREEN)Vet completed$(NC)"

.PHONY: lint
lint: ## Run linter
	@echo "$(BLUE)Running linter...$(NC)"
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "$(YELLOW)golangci-lint not found, skipping...$(NC)"; \
	fi
	@echo "$(GREEN)Linting completed$(NC)"

.PHONY: security
security: ## Run security scan
	@echo "$(BLUE)Running security scan...$(NC)"
	@if command -v gosec >/dev/null 2>&1; then \
		gosec ./...; \
	else \
		echo "$(YELLOW)gosec not found, skipping security scan$(NC)"; \
	fi
	@echo "$(GREEN)Security scan completed$(NC)"

.PHONY: check
check: fmt vet lint security test ## Run all checks
	@echo "$(GREEN)All checks completed$(NC)"

# Docker targets
.PHONY: docker-build
docker-build: ## Build Docker image
	@echo "$(BLUE)Building Docker image...$(NC)"
	docker build \
		--target $(DOCKER_BUILD_TARGET) \
		--tag $(APP_NAME):$(VERSION) \
		--tag $(APP_NAME):latest \
		$(DOCKER_BUILD_ARGS) \
		.
	@echo "$(GREEN)Docker image built: $(APP_NAME):$(VERSION)$(NC)"

.PHONY: docker-build-dev
docker-build-dev: ## Build development Docker image
	@echo "$(BLUE)Building development Docker image...$(NC)"
	docker build \
		--target development \
		--tag $(APP_NAME):dev \
		.
	@echo "$(GREEN)Development Docker image built$(NC)"

.PHONY: docker-push
docker-push: docker-build ## Push Docker image to registry
	@echo "$(BLUE)Pushing Docker image...$(NC)"
	docker tag $(APP_NAME):$(VERSION) $(DOCKER_IMAGE):$(VERSION)
	docker tag $(APP_NAME):latest $(DOCKER_IMAGE):latest
	docker push $(DOCKER_IMAGE):$(VERSION)
	docker push $(DOCKER_IMAGE):latest
	@echo "$(GREEN)Docker image pushed$(NC)"

.PHONY: docker-run
docker-run: ## Run Docker image locally
	@echo "$(BLUE)Running Docker image locally...$(NC)"
	docker run --rm -p 8089:8089 \
		-e LOG_LEVEL=debug \
		-e DATABASE_HOST=host.docker.internal \
		$(APP_NAME):$(VERSION)

.PHONY: docker-compose-up
docker-compose-up: ## Start services with Docker Compose
	@echo "$(BLUE)Starting services with Docker Compose...$(NC)"
	docker-compose up -d
	@echo "$(GREEN)Services started$(NC)"

.PHONY: docker-compose-down
docker-compose-down: ## Stop services with Docker Compose
	@echo "$(BLUE)Stopping services with Docker Compose...$(NC)"
	docker-compose down
	@echo "$(GREEN)Services stopped$(NC)"

.PHONY: docker-compose-logs
docker-compose-logs: ## Show Docker Compose logs
	docker-compose logs -f

# Kubernetes targets
.PHONY: k8s-deploy
k8s-deploy: ## Deploy to Kubernetes
	@echo "$(BLUE)Deploying to Kubernetes...$(NC)"
	./scripts/deploy.sh -e $(ENVIRONMENT) deploy
	@echo "$(GREEN)Deployment completed$(NC)"

.PHONY: k8s-deploy-dev
k8s-deploy-dev: ## Deploy to development environment
	@echo "$(BLUE)Deploying to development...$(NC)"
	./scripts/deploy.sh -e development deploy
	@echo "$(GREEN)Development deployment completed$(NC)"

.PHONY: k8s-deploy-prod
k8s-deploy-prod: ## Deploy to production environment
	@echo "$(BLUE)Deploying to production...$(NC)"
	./scripts/deploy.sh -e production -t $(VERSION) deploy
	@echo "$(GREEN)Production deployment completed$(NC)"

.PHONY: k8s-verify
k8s-verify: ## Verify Kubernetes deployment
	@echo "$(BLUE)Verifying deployment...$(NC)"
	./scripts/deploy.sh verify
	@echo "$(GREEN)Verification completed$(NC)"

.PHONY: k8s-rollback
k8s-rollback: ## Rollback Kubernetes deployment
	@echo "$(BLUE)Rolling back deployment...$(NC)"
	./scripts/deploy.sh rollback
	@echo "$(GREEN)Rollback completed$(NC)"

.PHONY: k8s-logs
k8s-logs: ## Show Kubernetes logs
	./scripts/deploy.sh logs

.PHONY: k8s-status
k8s-status: ## Show Kubernetes deployment status
	./scripts/deploy.sh status

.PHONY: k8s-clean
k8s-clean: ## Clean Kubernetes resources
	@echo "$(BLUE)Cleaning Kubernetes resources...$(NC)"
	./scripts/deploy.sh clean
	@echo "$(GREEN)Resources cleaned$(NC)"

# Development workflow targets
.PHONY: dev
dev: docker-compose-up ## Start development environment
	@echo "$(GREEN)Development environment started$(NC)"
	@echo "$(BLUE)Services available at:$(NC)"
	@echo "  - Allocation Service: http://localhost:8089"
	@echo "  - PostgreSQL: localhost:5432"
	@echo "  - Prometheus: http://localhost:9090"
	@echo "  - Grafana: http://localhost:3000"
	@echo "  - Jaeger: http://localhost:16686"

.PHONY: dev-stop
dev-stop: docker-compose-down ## Stop development environment

.PHONY: dev-restart
dev-restart: dev-stop dev ## Restart development environment

.PHONY: dev-logs
dev-logs: docker-compose-logs ## Show development logs

# Database targets
.PHONY: db-migrate
db-migrate: ## Run database migrations
	@echo "$(BLUE)Running database migrations...$(NC)"
	@if command -v migrate >/dev/null 2>&1; then \
		migrate -path migrations -database "postgres://postgres:postgres123@localhost:5432/allocation_db?sslmode=disable" up; \
	else \
		echo "$(YELLOW)migrate tool not found$(NC)"; \
	fi
	@echo "$(GREEN)Migrations completed$(NC)"

.PHONY: db-reset
db-reset: ## Reset database
	@echo "$(BLUE)Resetting database...$(NC)"
	docker-compose exec postgres psql -U postgres -d allocation_db -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
	$(MAKE) db-migrate
	@echo "$(GREEN)Database reset$(NC)"

# Monitoring targets
.PHONY: metrics
metrics: ## Show application metrics
	@curl -s http://localhost:8089/metrics | grep allocation_

.PHONY: health
health: ## Check application health
	@curl -s http://localhost:8089/healthz | jq '.'

# Release targets
.PHONY: release
release: check docker-build docker-push ## Create a release
	@echo "$(GREEN)Release $(VERSION) created$(NC)"

.PHONY: tag
tag: ## Create git tag
	@echo "$(BLUE)Creating git tag v$(VERSION)...$(NC)"
	git tag -a v$(VERSION) -m "Release version $(VERSION)"
	git push origin v$(VERSION)
	@echo "$(GREEN)Tag v$(VERSION) created$(NC)"

# Utility targets
.PHONY: version
version: ## Show version information
	@echo "$(BLUE)Application Version:$(NC) $(VERSION)"
	@echo "$(BLUE)Go Version:$(NC) $(shell go version)"
	@echo "$(BLUE)Docker Version:$(NC) $(shell docker --version 2>/dev/null || echo 'Not installed')"
	@echo "$(BLUE)Kubectl Version:$(NC) $(shell kubectl version --client --short 2>/dev/null || echo 'Not installed')"

.PHONY: env
env: ## Show environment variables
	@echo "$(BLUE)Environment Variables:$(NC)"
	@echo "APP_NAME: $(APP_NAME)"
	@echo "VERSION: $(VERSION)"
	@echo "DOCKER_REGISTRY: $(DOCKER_REGISTRY)"
	@echo "NAMESPACE: $(NAMESPACE)"
	@echo "ENVIRONMENT: $(ENVIRONMENT)"

# Default target
.DEFAULT_GOAL := help 