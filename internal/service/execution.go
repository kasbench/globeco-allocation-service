package service

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/go-playground/validator/v10"
	"go.uber.org/zap"

	"github.com/kasbench/globeco-allocation-service/internal/config"
	"github.com/kasbench/globeco-allocation-service/internal/domain"
	"github.com/kasbench/globeco-allocation-service/internal/repository"
)

// ExecutionService handles business logic for executions
type ExecutionService struct {
	executionRepo          *repository.ExecutionRepository
	batchHistoryRepo       *repository.BatchHistoryRepository
	tradeClient            *TradeServiceClient
	fileGenerator          *FileGeneratorService
	cliInvoker             *CLIInvokerService
	kubernetesBatchInvoker KubernetesBatchInvoker
	logger                 *zap.Logger
	validator              *validator.Validate
	config                 *config.Config
}

// NewExecutionService creates a new execution service
func NewExecutionService(
	executionRepo *repository.ExecutionRepository,
	batchHistoryRepo *repository.BatchHistoryRepository,
	tradeClient *TradeServiceClient,
	logger *zap.Logger,
	cfg *config.Config,
) *ExecutionService {
	fileGenerator := NewFileGeneratorService(cfg.OutputDir, logger)
	cliInvoker := NewCLIInvokerService(cfg.CLICommand, logger)

	// Initialize Kubernetes batch invoker if enabled
	var kubernetesBatchInvoker KubernetesBatchInvoker
	if cfg.KubernetesBatch.Enabled {
		invoker, err := NewKubernetesBatchInvokerService(&cfg.KubernetesBatch, logger)
		if err != nil {
			logger.Error("Failed to initialize Kubernetes batch invoker", zap.Error(err))
			// Continue without Kubernetes support - will fall back to direct execution
		} else {
			kubernetesBatchInvoker = invoker
			logger.Info("Kubernetes batch invoker initialized successfully")
		}
	}

	return &ExecutionService{
		executionRepo:          executionRepo,
		batchHistoryRepo:       batchHistoryRepo,
		tradeClient:            tradeClient,
		fileGenerator:          fileGenerator,
		cliInvoker:             cliInvoker,
		kubernetesBatchInvoker: kubernetesBatchInvoker,
		logger:                 logger,
		validator:              validator.New(),
		config:                 cfg,
	}
}

// CreateBatch processes a batch of execution requests
func (s *ExecutionService) CreateBatch(ctx context.Context, executions []domain.ExecutionPostDTO) (*domain.BatchCreateResponse, error) {
	if len(executions) == 0 {
		return nil, fmt.Errorf("no executions provided")
	}

	if len(executions) > 100 {
		return nil, fmt.Errorf("batch size exceeds maximum of 100 executions")
	}

	s.logger.Info("Processing execution batch", zap.Int("batch_size", len(executions)))

	response := &domain.BatchCreateResponse{
		Results: make([]domain.ExecutionResult, 0, len(executions)),
	}

	for _, executionDTO := range executions {
		result := s.processExecution(ctx, executionDTO)
		response.Results = append(response.Results, result)

		switch result.Status {
		case "created":
			response.ProcessedCount++
		case "skipped":
			response.SkippedCount++
		case "error":
			response.ErrorCount++
		}
	}

	s.logger.Info("Batch processing completed",
		zap.Int("processed", response.ProcessedCount),
		zap.Int("skipped", response.SkippedCount),
		zap.Int("errors", response.ErrorCount))

	return response, nil
}

// processExecution processes a single execution DTO
func (s *ExecutionService) processExecution(ctx context.Context, executionDTO domain.ExecutionPostDTO) domain.ExecutionResult {
	result := domain.ExecutionResult{
		ExecutionServiceID: executionDTO.ExecutionServiceID,
	}

	// Validate input
	if err := s.validator.Struct(executionDTO); err != nil {
		result.Status = "error"
		result.Error = fmt.Sprintf("validation failed: %v", err)
		return result
	}

	// Skip open executions
	if executionDTO.IsOpen {
		result.Status = "skipped"
		result.Error = "execution is still open"
		s.logger.Debug("Skipping open execution", zap.Int("execution_service_id", executionDTO.ExecutionServiceID))
		return result
	}

	// Check if execution already exists
	existing, err := s.executionRepo.GetByExecutionServiceID(ctx, executionDTO.ExecutionServiceID)
	if err == nil && existing != nil {
		result.Status = "skipped"
		result.Error = "execution already exists"
		result.ExecutionID = &existing.ID
		s.logger.Debug("Execution already exists", zap.Int("execution_service_id", executionDTO.ExecutionServiceID))
		return result
	}

	// Get portfolio ID from Trade Service
	portfolioID, err := s.getPortfolioIDFromTradeService(ctx, executionDTO.ExecutionServiceID)
	if err != nil {
		result.Status = "error"
		result.Error = fmt.Sprintf("failed to get portfolio ID: %v", err)
		return result
	}

	// Convert DTO to domain model
	execution := s.dtoToExecution(executionDTO, portfolioID)

	// Save execution
	if err := s.executionRepo.Create(ctx, execution); err != nil {
		result.Status = "error"
		result.Error = fmt.Sprintf("failed to create execution: %v", err)
		return result
	}

	result.Status = "created"
	result.ExecutionID = &execution.ID
	s.logger.Info("Execution created successfully",
		zap.Int("id", execution.ID),
		zap.Int("execution_service_id", execution.ExecutionServiceID))

	return result
}

// getPortfolioIDFromTradeService retrieves portfolio ID from Trade Service
func (s *ExecutionService) getPortfolioIDFromTradeService(ctx context.Context, executionServiceID int) (string, error) {
	response, err := s.tradeClient.GetExecutionByServiceID(ctx, executionServiceID)
	if err != nil {
		return "", fmt.Errorf("trade service call failed: %w", err)
	}

	if len(response.Executions) == 0 {
		return "", fmt.Errorf("no execution found in trade service for ID %d", executionServiceID)
	}

	execution := response.Executions[0]
	portfolioID := execution.TradeOrder.Portfolio.PortfolioID

	if portfolioID == "" {
		return "", fmt.Errorf("portfolio ID is empty for execution service ID %d", executionServiceID)
	}

	return portfolioID, nil
}

// dtoToExecution converts ExecutionPostDTO to Execution domain model
func (s *ExecutionService) dtoToExecution(dto domain.ExecutionPostDTO, portfolioID string) *domain.Execution {
	now := time.Now()

	// Determine trade date based on US Eastern Time
	easternLoc, _ := time.LoadLocation("America/New_York")
	tradeDate := dto.SentTimestamp.In(easternLoc).Truncate(24 * time.Hour)

	return &domain.Execution{
		ExecutionServiceID:   dto.ExecutionServiceID,
		IsOpen:               false, // We only process closed executions
		ExecutionStatus:      dto.ExecutionStatus,
		TradeType:            dto.TradeType,
		Destination:          dto.Destination,
		TradeDate:            tradeDate,
		SecurityID:           dto.SecurityID,
		Ticker:               dto.Ticker,
		PortfolioID:          &portfolioID,
		Quantity:             dto.Quantity,
		LimitPrice:           dto.LimitPrice,
		ReceivedTimestamp:    dto.ReceivedTimestamp.UTC(),
		SentTimestamp:        dto.SentTimestamp.UTC(),
		LastFillTimestamp:    dto.LastFillTimestamp,
		QuantityFilled:       dto.QuantityFilled,
		TotalAmount:          dto.TotalAmount,
		AveragePrice:         dto.AveragePrice,
		ReadyToSendTimestamp: now.UTC(),
		Version:              1,
	}
}

// GetByID retrieves an execution by ID
func (s *ExecutionService) GetByID(ctx context.Context, id int) (*domain.ExecutionDTO, error) {
	execution, err := s.executionRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get execution: %w", err)
	}

	dto := execution.ToDTO()
	return &dto, nil
}

// List retrieves executions with pagination
func (s *ExecutionService) List(ctx context.Context, limit, offset int) (*domain.ExecutionListResponse, error) {
	// Set default and maximum limits
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}

	executions, totalCount, err := s.executionRepo.List(ctx, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list executions: %w", err)
	}

	// Convert to DTOs
	executionDTOs := make([]domain.ExecutionDTO, len(executions))
	for i, execution := range executions {
		executionDTOs[i] = execution.ToDTO()
	}

	// Calculate pagination info
	totalPages := (totalCount + limit - 1) / limit
	currentPage := offset / limit

	response := &domain.ExecutionListResponse{
		Executions: executionDTOs,
		Pagination: domain.PaginationInfo{
			TotalElements: totalCount,
			TotalPages:    totalPages,
			CurrentPage:   currentPage,
			PageSize:      limit,
			HasNext:       offset+limit < totalCount,
			HasPrevious:   offset > 0,
		},
	}

	return response, nil
}

// Send processes executions for Portfolio Accounting with configuration-based execution mode selection
func (s *ExecutionService) Send(ctx context.Context) (*domain.SendResponse, error) {
	// Determine execution mode based on configuration (Requirement 1.1, 1.2)
	if s.config.KubernetesBatch.Enabled && s.kubernetesBatchInvoker != nil {
		s.logger.Info("Using Kubernetes batch job execution mode")
		return s.SendWithKubernetes(ctx)
	}

	s.logger.Info("Using direct CLI execution mode")
	return s.sendWithDirectCLI(ctx)
}

// SendWithKubernetes processes executions using Kubernetes batch jobs (Requirement 1.1, 1.2)
func (s *ExecutionService) SendWithKubernetes(ctx context.Context) (*domain.SendResponse, error) {
	s.logger.Info("Starting execution send process with Kubernetes batch jobs")

	// Step 1: Get max start time from batch history
	previousStartTime, err := s.batchHistoryRepo.GetMaxStartTime(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get max start time: %w", err)
	}

	// Step 2: Create new batch history record
	currentTime := time.Now().UTC()
	batchHistory := &domain.BatchHistory{
		StartTime:         currentTime,
		PreviousStartTime: previousStartTime,
		Version:           1,
	}

	if err := s.batchHistoryRepo.Create(ctx, batchHistory); err != nil {
		// Check if this is a uniqueness constraint violation (duplicate batch)
		if err.Error() == "duplicate batch detected" {
			return nil, fmt.Errorf("duplicate batch process already started")
		}
		return nil, fmt.Errorf("failed to create batch history: %w", err)
	}

	s.logger.Info("Batch history created",
		zap.Int("batch_id", batchHistory.ID),
		zap.Time("start_time", currentTime),
		zap.Time("previous_start_time", previousStartTime))

	// Step 3: Get executions for this batch
	executions, err := s.executionRepo.GetForBatch(ctx, previousStartTime, currentTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get executions for batch: %w", err)
	}

	if len(executions) == 0 {
		s.logger.Info("No executions to process")
		return &domain.SendResponse{
			ProcessedCount: 0,
			FileName:       "",
			Status:         "success",
			Message:        "No executions to process",
			ExecutionMode:  "kubernetes",
		}, nil
	}

	s.logger.Info("Retrieved executions for processing", zap.Int("count", len(executions)))

	// Step 4: Generate Portfolio Accounting file
	filename, err := s.fileGenerator.GeneratePortfolioAccountingFile(ctx, executions)
	if err != nil {
		return nil, fmt.Errorf("failed to generate file: %w", err)
	}

	// Step 5: Invoke Portfolio Accounting CLI using Kubernetes batch job
	jobName := fmt.Sprintf("portfolio-cli-%d", time.Now().Unix())
	if err := s.kubernetesBatchInvoker.InvokePortfolioAccountingCLI(ctx, filename, s.config.OutputDir); err != nil {
		s.logger.Error("Kubernetes batch job invocation failed", zap.Error(err))

		// Don't cleanup file on failure (Requirement 2.4)
		return &domain.SendResponse{
			ProcessedCount: len(executions),
			FileName:       filename,
			Status:         "error",
			Message:        fmt.Sprintf("Kubernetes batch job failed: %v", err),
			JobName:        &jobName,
			JobStatus:      stringPtr("failed"),
			ExecutionMode:  "kubernetes",
		}, fmt.Errorf("Kubernetes batch job failed: %w", err)
	}

	// Step 6: Cleanup file only after successful job completion (Requirement 2.3, 2.4)
	if s.config.FileCleanupEnabled {
		// Validate job status before file deletion (Requirement 2.3)
		if err := s.ValidateJobStatusForCleanup(ctx, jobName); err != nil {
			s.logger.Error("Job status validation failed, skipping file cleanup",
				zap.String("job_name", jobName),
				zap.String("filename", filename),
				zap.Error(err))
		} else {
			// Only cleanup if job status validation passes
			if err := s.cleanupFileAfterSuccessfulJob(filename); err != nil {
				s.logger.Warn("File cleanup failed after successful job", zap.Error(err))
			}
		}
	}

	s.logger.Info("Execution send process completed successfully with Kubernetes batch job",
		zap.Int("processed_count", len(executions)),
		zap.String("filename", filename),
		zap.String("job_name", jobName))

	return &domain.SendResponse{
		ProcessedCount: len(executions),
		FileName:       filename,
		Status:         "success",
		Message:        "Kubernetes batch job executed successfully",
		JobName:        &jobName,
		JobStatus:      stringPtr("succeeded"),
		ExecutionMode:  "kubernetes",
	}, nil
}

// sendWithDirectCLI processes executions using direct CLI invocation (legacy mode)
func (s *ExecutionService) sendWithDirectCLI(ctx context.Context) (*domain.SendResponse, error) {
	s.logger.Info("Starting execution send process with direct CLI")

	// Step 1: Get max start time from batch history
	previousStartTime, err := s.batchHistoryRepo.GetMaxStartTime(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get max start time: %w", err)
	}

	// Step 2: Create new batch history record
	currentTime := time.Now().UTC()
	batchHistory := &domain.BatchHistory{
		StartTime:         currentTime,
		PreviousStartTime: previousStartTime,
		Version:           1,
	}

	if err := s.batchHistoryRepo.Create(ctx, batchHistory); err != nil {
		// Check if this is a uniqueness constraint violation (duplicate batch)
		if err.Error() == "duplicate batch detected" {
			return nil, fmt.Errorf("duplicate batch process already started")
		}
		return nil, fmt.Errorf("failed to create batch history: %w", err)
	}

	s.logger.Info("Batch history created",
		zap.Int("batch_id", batchHistory.ID),
		zap.Time("start_time", currentTime),
		zap.Time("previous_start_time", previousStartTime))

	// Step 3: Get executions for this batch
	executions, err := s.executionRepo.GetForBatch(ctx, previousStartTime, currentTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get executions for batch: %w", err)
	}

	if len(executions) == 0 {
		s.logger.Info("No executions to process")
		return &domain.SendResponse{
			ProcessedCount: 0,
			FileName:       "",
			Status:         "success",
			Message:        "No executions to process",
			ExecutionMode:  "direct",
		}, nil
	}

	s.logger.Info("Retrieved executions for processing", zap.Int("count", len(executions)))

	// Step 4: Generate Portfolio Accounting file
	filename, err := s.fileGenerator.GeneratePortfolioAccountingFile(ctx, executions)
	if err != nil {
		return nil, fmt.Errorf("failed to generate file: %w", err)
	}

	// Step 5: Invoke Portfolio Accounting CLI directly
	if err := s.cliInvoker.InvokePortfolioAccountingCLI(ctx, filename, s.config.OutputDir); err != nil {
		s.logger.Error("CLI invocation failed", zap.Error(err))
		return &domain.SendResponse{
			ProcessedCount: len(executions),
			FileName:       filename,
			Status:         "error",
			Message:        fmt.Sprintf("CLI invocation failed: %v", err),
			ExecutionMode:  "direct",
		}, fmt.Errorf("CLI invocation failed: %w", err)
	}

	// Step 6: Cleanup file if enabled (legacy behavior)
	if s.config.FileCleanupEnabled {
		if err := s.fileGenerator.CleanupFile(filename, true); err != nil {
			s.logger.Warn("File cleanup failed", zap.Error(err))
		}
	}

	s.logger.Info("Execution send process completed successfully with direct CLI",
		zap.Int("processed_count", len(executions)),
		zap.String("filename", filename))

	return &domain.SendResponse{
		ProcessedCount: len(executions),
		FileName:       filename,
		Status:         "success",
		Message:        "Portfolio Accounting CLI executed successfully",
		ExecutionMode:  "direct",
	}, nil
}

// cleanupFileAfterSuccessfulJob implements file cleanup logic for successful Kubernetes jobs (Requirement 2.3, 2.4)
func (s *ExecutionService) cleanupFileAfterSuccessfulJob(filename string) error {
	s.logger.Info("Cleaning up file after successful job completion", zap.String("filename", filename))

	// Verify file exists before attempting cleanup
	filePath := s.fileGenerator.GetFilePath(filename)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		s.logger.Warn("File does not exist, skipping cleanup",
			zap.String("filename", filename),
			zap.String("filepath", filePath))
		return nil
	}

	// Attempt file cleanup with enhanced error handling (Requirement 2.4)
	if err := s.fileGenerator.CleanupFile(filename, true); err != nil {
		// Implement proper error handling for cleanup failures (Requirement 2.4)
		s.logger.Error("Failed to cleanup file after successful job",
			zap.String("filename", filename),
			zap.String("filepath", filePath),
			zap.Error(err))

		// Check if it's a permission error or other specific error types
		if os.IsPermission(err) {
			return fmt.Errorf("file cleanup failed due to permission error: %w", err)
		}

		return fmt.Errorf("file cleanup failed: %w", err)
	}

	s.logger.Info("File cleanup completed successfully",
		zap.String("filename", filename),
		zap.String("filepath", filePath))
	return nil
}

// ValidateJobStatusForCleanup validates job status before allowing file cleanup (Requirement 2.3)
func (s *ExecutionService) ValidateJobStatusForCleanup(ctx context.Context, jobName string) error {
	if s.kubernetesBatchInvoker == nil {
		return fmt.Errorf("kubernetes batch invoker not available")
	}

	s.logger.Debug("Validating job status before file cleanup", zap.String("job_name", jobName))

	jobStatus, err := s.kubernetesBatchInvoker.GetJobStatus(ctx, jobName)
	if err != nil {
		s.logger.Error("Failed to get job status for cleanup validation",
			zap.String("job_name", jobName),
			zap.Error(err))
		return fmt.Errorf("failed to get job status: %w", err)
	}

	if jobStatus.Status != "succeeded" {
		s.logger.Warn("Job status validation failed - job not successful",
			zap.String("job_name", jobName),
			zap.String("status", jobStatus.Status),
			zap.String("message", jobStatus.Message))
		return fmt.Errorf("job status is %s, cannot cleanup file", jobStatus.Status)
	}

	s.logger.Debug("Job status validation passed - job succeeded",
		zap.String("job_name", jobName),
		zap.String("status", jobStatus.Status))

	return nil
}

// stringPtr returns a pointer to the given string value
func stringPtr(s string) *string {
	return &s
}
