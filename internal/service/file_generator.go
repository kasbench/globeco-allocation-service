package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kasbench/globeco-allocation-service/internal/domain"
)

// FileGeneratorService handles file generation for Portfolio Accounting CLI
type FileGeneratorService struct {
	outputDir string
	logger    *zap.Logger
}

// NewFileGeneratorService creates a new file generator service
func NewFileGeneratorService(outputDir string, logger *zap.Logger) *FileGeneratorService {
	return &FileGeneratorService{
		outputDir: outputDir,
		logger:    logger,
	}
}

// GeneratePortfolioAccountingFile creates a CSV file in the Portfolio Accounting CLI format
func (s *FileGeneratorService) GeneratePortfolioAccountingFile(ctx context.Context, executions []domain.Execution) (string, error) {
	if len(executions) == 0 {
		return "", fmt.Errorf("no executions to process")
	}

	// Generate filename with timestamp
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("transactions_%s.csv", timestamp)
	filepath := filepath.Join(s.outputDir, filename)

	s.logger.Info("Generating Portfolio Accounting file",
		zap.String("filename", filename),
		zap.String("filepath", filepath),
		zap.Int("execution_count", len(executions)))

	// Ensure output directory exists
	if err := os.MkdirAll(s.outputDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	// Create file
	file, err := os.Create(filepath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Write CSV header
	header := "portfolio_id,security_id,source_id,transaction_type,quantity,price,transaction_date\n"
	if _, err := file.WriteString(header); err != nil {
		return "", fmt.Errorf("failed to write header: %w", err)
	}

	// Convert executions to CSV format
	for _, execution := range executions {
		line := s.executionToCSVLine(execution)
		if _, err := file.WriteString(line); err != nil {
			return "", fmt.Errorf("failed to write execution line: %w", err)
		}
	}

	s.logger.Info("Portfolio Accounting file generated successfully",
		zap.String("filename", filename),
		zap.Int("records_written", len(executions)))

	return filename, nil
}

// executionToCSVLine converts an execution to a CSV line according to the Portfolio Accounting format
func (s *FileGeneratorService) executionToCSVLine(execution domain.Execution) string {
	// Extract portfolio_id (should not be null at this point)
	portfolioID := ""
	if execution.PortfolioID != nil {
		portfolioID = *execution.PortfolioID
	}

	// Generate source_id as "AC" + execution.id
	sourceID := fmt.Sprintf("AC%d", execution.ID)

	// Format trade date as YYYY-MM-DD
	tradeDate := execution.TradeDate.Format("2006-01-02")

	// Build CSV line
	fields := []string{
		portfolioID,
		execution.SecurityID,
		sourceID,
		execution.TradeType,
		fmt.Sprintf("%.8f", execution.Quantity),
		fmt.Sprintf("%.8f", execution.AveragePrice),
		tradeDate,
	}

	// Escape fields that might contain commas or quotes
	for i, field := range fields {
		if strings.Contains(field, ",") || strings.Contains(field, "\"") || strings.Contains(field, "\n") {
			fields[i] = "\"" + strings.ReplaceAll(field, "\"", "\"\"") + "\""
		}
	}

	return strings.Join(fields, ",") + "\n"
}

// CleanupFile removes a file if cleanup is enabled
func (s *FileGeneratorService) CleanupFile(filename string, cleanupEnabled bool) error {
	if !cleanupEnabled {
		s.logger.Info("File cleanup disabled, keeping file", zap.String("filename", filename))
		return nil
	}

	filepath := filepath.Join(s.outputDir, filename)
	if err := os.Remove(filepath); err != nil {
		s.logger.Error("Failed to cleanup file", zap.String("filepath", filepath), zap.Error(err))
		return fmt.Errorf("failed to cleanup file: %w", err)
	}

	s.logger.Info("File cleaned up successfully", zap.String("filepath", filepath))
	return nil
}

// GetFilePath returns the full path for a given filename
func (s *FileGeneratorService) GetFilePath(filename string) string {
	return filepath.Join(s.outputDir, filename)
}
