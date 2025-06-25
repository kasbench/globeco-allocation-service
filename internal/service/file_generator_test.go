package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/kasbench/globeco-allocation-service/internal/domain"
)

func TestFileGeneratorService_GeneratePortfolioAccountingFile(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "test_file_generator")
	require.NoError(t, err)
	defer func() {
		err := os.RemoveAll(tempDir)
		require.NoError(t, err)
	}()

	logger := zap.NewNop()
	generator := NewFileGeneratorService(tempDir, logger)

	// Create test executions
	ctx := context.Background()
	tradeDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	portfolioID1 := "PORTFOLIO123456789012"
	portfolioID2 := "PORTFOLIO987654321098"

	executions := []domain.Execution{
		{
			ID:           1,
			PortfolioID:  &portfolioID1,
			SecurityID:   "SECURITY123456789012ABCD",
			TradeType:    "BUY",
			Quantity:     100.5,
			AveragePrice: 149.25,
			TradeDate:    tradeDate,
		},
		{
			ID:           2,
			PortfolioID:  &portfolioID2,
			SecurityID:   "SECURITY987654321098WXYZ",
			TradeType:    "SELL",
			Quantity:     50.0,
			AveragePrice: 200.75,
			TradeDate:    tradeDate,
		},
		{
			ID:           3,
			PortfolioID:  &portfolioID1,
			SecurityID:   "SECURITY555666777888MNOP",
			TradeType:    "BUY",
			Quantity:     25.25,
			AveragePrice: 75.50,
			TradeDate:    tradeDate,
		},
	}

	filename, err := generator.GeneratePortfolioAccountingFile(ctx, executions)

	assert.NoError(t, err)
	assert.NotEmpty(t, filename)
	assert.Contains(t, filename, "transactions_")
	assert.Contains(t, filename, ".csv")

	// Verify file exists
	fullPath := filepath.Join(tempDir, filename)
	assert.FileExists(t, fullPath)

	// Read file content and verify
	content, err := os.ReadFile(fullPath)
	require.NoError(t, err)

	expectedHeader := "portfolio_id,security_id,source_id,transaction_type,quantity,price,transaction_date\n"
	contentStr := string(content)
	assert.Contains(t, contentStr, expectedHeader)

	// Verify each execution is present in the file
	assert.Contains(t, contentStr, "PORTFOLIO123456789012")
	assert.Contains(t, contentStr, "PORTFOLIO987654321098")
	assert.Contains(t, contentStr, "SECURITY123456789012ABCD")
	assert.Contains(t, contentStr, "SECURITY987654321098WXYZ")
	assert.Contains(t, contentStr, "SECURITY555666777888MNOP")
	assert.Contains(t, contentStr, "AC1") // source_id for first execution
	assert.Contains(t, contentStr, "AC2") // source_id for second execution
	assert.Contains(t, contentStr, "AC3") // source_id for third execution
	assert.Contains(t, contentStr, "BUY")
	assert.Contains(t, contentStr, "SELL")
	assert.Contains(t, contentStr, "100.50000000")
	assert.Contains(t, contentStr, "149.25000000")
	assert.Contains(t, contentStr, "50.00000000")
	assert.Contains(t, contentStr, "200.75000000")
	assert.Contains(t, contentStr, "2024-01-15")

	// Verify line count (header + 3 executions)
	lines := strings.Split(strings.TrimSpace(contentStr), "\n")
	assert.Len(t, lines, 4)
}

func TestFileGeneratorService_GeneratePortfolioAccountingFile_EmptyExecutions(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "test_file_generator_empty")
	require.NoError(t, err)
	defer func() {
		err := os.RemoveAll(tempDir)
		require.NoError(t, err)
	}()

	logger := zap.NewNop()
	generator := NewFileGeneratorService(tempDir, logger)

	ctx := context.Background()
	executions := []domain.Execution{}

	filename, err := generator.GeneratePortfolioAccountingFile(ctx, executions)

	// The service returns an error for empty executions
	assert.Error(t, err)
	assert.Empty(t, filename)
	assert.Contains(t, err.Error(), "no executions to process")
}

func TestFileGeneratorService_GeneratePortfolioAccountingFile_WithCSVEscaping(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "test_file_generator_escape")
	require.NoError(t, err)
	defer func() {
		err := os.RemoveAll(tempDir)
		require.NoError(t, err)
	}()

	logger := zap.NewNop()
	generator := NewFileGeneratorService(tempDir, logger)

	// Create test execution with values that need CSV escaping
	ctx := context.Background()
	tradeDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	portfolioID := "PORTFOLIO,WITH,COMMAS"

	executions := []domain.Execution{
		{
			ID:           1,
			PortfolioID:  &portfolioID,
			SecurityID:   "SECURITY\"WITH\"QUOTES",
			TradeType:    "BUY",
			Quantity:     100.5,
			AveragePrice: 149.25,
			TradeDate:    tradeDate,
		},
	}

	filename, err := generator.GeneratePortfolioAccountingFile(ctx, executions)

	assert.NoError(t, err)

	// Read file content and verify CSV escaping
	fullPath := filepath.Join(tempDir, filename)
	content, err := os.ReadFile(fullPath)
	require.NoError(t, err)

	contentStr := string(content)
	// Values with commas should be quoted
	assert.Contains(t, contentStr, `"PORTFOLIO,WITH,COMMAS"`)
	// Values with quotes should be escaped
	assert.Contains(t, contentStr, `"SECURITY""WITH""QUOTES"`)
}

func TestFileGeneratorService_GeneratePortfolioAccountingFile_NonExistentDirectory(t *testing.T) {
	// Use a non-existent directory that can't be created (permission denied)
	nonExistentDir := "/root/non/existent/directory"
	logger := zap.NewNop()
	generator := NewFileGeneratorService(nonExistentDir, logger)

	ctx := context.Background()
	executions := []domain.Execution{
		{
			ID:           1,
			PortfolioID:  stringPtr("PORTFOLIO123456789012"),
			SecurityID:   "SECURITY123456789012ABCD",
			TradeType:    "BUY",
			Quantity:     100.5,
			AveragePrice: 149.25,
			TradeDate:    time.Now(),
		},
	}

	filename, err := generator.GeneratePortfolioAccountingFile(ctx, executions)

	assert.Error(t, err)
	assert.Empty(t, filename)
	// The error could be about creating the directory or the file
	assert.True(t, strings.Contains(err.Error(), "failed to create") || strings.Contains(err.Error(), "permission denied"))
}

func TestFileGeneratorService_CleanupFile(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "test_file_generator_cleanup")
	require.NoError(t, err)
	defer func() {
		err := os.RemoveAll(tempDir)
		require.NoError(t, err)
	}()

	logger := zap.NewNop()
	generator := NewFileGeneratorService(tempDir, logger)

	// Create a test file
	testFilename := "test_file.csv"
	testFilePath := filepath.Join(tempDir, testFilename)
	err = os.WriteFile(testFilePath, []byte("test content"), 0644)
	require.NoError(t, err)

	// Verify file exists before cleanup
	assert.FileExists(t, testFilePath)

	// Cleanup the file with cleanup enabled
	err = generator.CleanupFile(testFilename, true)

	assert.NoError(t, err)
	// Verify file no longer exists
	assert.NoFileExists(t, testFilePath)
}

func TestFileGeneratorService_CleanupFile_CleanupDisabled(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "test_file_generator_no_cleanup")
	require.NoError(t, err)
	defer func() {
		err := os.RemoveAll(tempDir)
		require.NoError(t, err)
	}()

	logger := zap.NewNop()
	generator := NewFileGeneratorService(tempDir, logger)

	// Create a test file
	testFilename := "test_file.csv"
	testFilePath := filepath.Join(tempDir, testFilename)
	err = os.WriteFile(testFilePath, []byte("test content"), 0644)
	require.NoError(t, err)

	// Verify file exists before cleanup
	assert.FileExists(t, testFilePath)

	// Attempt cleanup with cleanup disabled
	err = generator.CleanupFile(testFilename, false)

	assert.NoError(t, err)
	// Verify file still exists (cleanup was disabled)
	assert.FileExists(t, testFilePath)
}

func TestFileGeneratorService_CleanupFile_NonExistentFile(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "test_file_generator_cleanup_missing")
	require.NoError(t, err)
	defer func() {
		err := os.RemoveAll(tempDir)
		require.NoError(t, err)
	}()

	logger := zap.NewNop()
	generator := NewFileGeneratorService(tempDir, logger)

	// Try to cleanup a file that doesn't exist
	err = generator.CleanupFile("non_existent_file.csv", true)

	// Should return an error for missing files when cleanup is enabled
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to cleanup file")
}

func TestFileGeneratorService_FilenameGeneration(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "test_file_generator_filename")
	require.NoError(t, err)
	defer func() {
		err := os.RemoveAll(tempDir)
		require.NoError(t, err)
	}()

	logger := zap.NewNop()
	generator := NewFileGeneratorService(tempDir, logger)

	ctx := context.Background()
	executions := []domain.Execution{
		{
			ID:           1,
			PortfolioID:  stringPtr("PORTFOLIO123456789012"),
			SecurityID:   "SECURITY123456789012ABCD",
			TradeType:    "BUY",
			Quantity:     100.5,
			AveragePrice: 149.25,
			TradeDate:    time.Now(),
		},
	}

	// Generate multiple files and verify unique filenames
	filename1, err := generator.GeneratePortfolioAccountingFile(ctx, executions)
	assert.NoError(t, err)

	time.Sleep(1 * time.Second) // Ensure different timestamp (service uses seconds precision)

	filename2, err := generator.GeneratePortfolioAccountingFile(ctx, executions)
	assert.NoError(t, err)

	// Filenames should be different due to timestamps
	assert.NotEqual(t, filename1, filename2)
	assert.Contains(t, filename1, "transactions_")
	assert.Contains(t, filename2, "transactions_")
	assert.Contains(t, filename1, ".csv")
	assert.Contains(t, filename2, ".csv")
}

func TestFileGeneratorService_GetFilePath(t *testing.T) {
	tempDir := "/tmp/test"
	logger := zap.NewNop()
	generator := NewFileGeneratorService(tempDir, logger)

	filename := "test_file.csv"
	expectedPath := filepath.Join(tempDir, filename)

	actualPath := generator.GetFilePath(filename)

	assert.Equal(t, expectedPath, actualPath)
}

// Helper function for string pointer
func stringPtr(s string) *string {
	return &s
}
