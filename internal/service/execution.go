package service

import (
	"context"
	"fmt"
	"time"

	"github.com/go-playground/validator/v10"
	"go.uber.org/zap"

	"github.com/kasbench/globeco-allocation-service/internal/config"
	"github.com/kasbench/globeco-allocation-service/internal/domain"
	"github.com/kasbench/globeco-allocation-service/internal/repository"
)

// ExecutionService handles business logic for executions
type ExecutionService struct {
	executionRepo    *repository.ExecutionRepository
	batchHistoryRepo *repository.BatchHistoryRepository
	tradeClient      *TradeServiceClient
	fileGenerator    *FileGeneratorService
	cliInvoker       *CLIInvokerService
	logger           *zap.Logger
	validator        *validator.Validate
	config           *config.Config
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

	return &ExecutionService{
		executionRepo:    executionRepo,
		batchHistoryRepo: batchHistoryRepo,
		tradeClient:      tradeClient,
		fileGenerator:    fileGenerator,
		cliInvoker:       cliInvoker,
		logger:           logger,
		validator:        validator.New(),
		config:           cfg,
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

// Send processes executions for Portfolio Accounting
func (s *ExecutionService) Send(ctx context.Context) (*domain.SendResponse, error) {
	s.logger.Info("Starting execution send process")

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
		}, nil
	}

	s.logger.Info("Retrieved executions for processing", zap.Int("count", len(executions)))

	// Step 4: Generate Portfolio Accounting file
	filename, err := s.fileGenerator.GeneratePortfolioAccountingFile(ctx, executions)
	if err != nil {
		return nil, fmt.Errorf("failed to generate file: %w", err)
	}

	// Step 5: Invoke Portfolio Accounting CLI
	if err := s.cliInvoker.InvokePortfolioAccountingCLI(ctx, filename); err != nil {
		s.logger.Error("CLI invocation failed", zap.Error(err))
		return &domain.SendResponse{
			ProcessedCount: len(executions),
			FileName:       filename,
			Status:         "error",
			Message:        fmt.Sprintf("CLI invocation failed: %v", err),
		}, fmt.Errorf("CLI invocation failed: %w", err)
	}

	// Step 6: Cleanup file if enabled
	if s.config.FileCleanupEnabled {
		if err := s.fileGenerator.CleanupFile(filename, true); err != nil {
			s.logger.Warn("File cleanup failed", zap.Error(err))
		}
	}

	s.logger.Info("Execution send process completed successfully",
		zap.Int("processed_count", len(executions)),
		zap.String("filename", filename))

	return &domain.SendResponse{
		ProcessedCount: len(executions),
		FileName:       filename,
		Status:         "success",
		Message:        "Portfolio Accounting CLI executed successfully",
	}, nil
}
