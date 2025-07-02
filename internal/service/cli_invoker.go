package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"go.uber.org/zap"
)

// CLIInvokerService handles execution of Portfolio Accounting CLI commands
type CLIInvokerService struct {
	cliCommand string
	logger     *zap.Logger
	timeout    time.Duration
}

// NewCLIInvokerService creates a new CLI invoker service
func NewCLIInvokerService(cliCommand string, logger *zap.Logger) *CLIInvokerService {
	home, err := os.UserHomeDir()
	if err == nil && strings.Contains(cliCommand, "{home}") {
		cliCommand = strings.ReplaceAll(cliCommand, "{home}", home)
	}
	return &CLIInvokerService{
		cliCommand: cliCommand,
		logger:     logger,
		timeout:    5 * time.Minute, // Default timeout
	}
}

// SetTimeout configures the CLI execution timeout
func (s *CLIInvokerService) SetTimeout(timeout time.Duration) {
	s.timeout = timeout
}

// InvokePortfolioAccountingCLI executes the Portfolio Accounting CLI with the given file and output directory
func (s *CLIInvokerService) InvokePortfolioAccountingCLI(ctx context.Context, filename string, outputDir string) error {
	if s.cliCommand == "" {
		return fmt.Errorf("CLI command not configured")
	}

	// Replace placeholders in command
	command := strings.ReplaceAll(s.cliCommand, "{filename}", filename)
	command = strings.ReplaceAll(command, "{output_dir}", outputDir)

	s.logger.Info("Invoking Portfolio Accounting CLI",
		zap.String("command", command),
		zap.String("filename", filename),
		zap.String("outputDir", outputDir))

	// Create context with timeout
	cmdCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	// Parse and execute command
	if err := s.executeCommand(cmdCtx, command); err != nil {
		s.logger.Error("Portfolio Accounting CLI execution failed",
			zap.String("command", command),
			zap.Error(err))
		return fmt.Errorf("CLI execution failed: %w", err)
	}

	s.logger.Info("Portfolio Accounting CLI executed successfully",
		zap.String("filename", filename))

	return nil
}

// executeCommand parses and executes the CLI command
func (s *CLIInvokerService) executeCommand(ctx context.Context, command string) error {
	// Parse command into parts
	parts := s.parseCommand(command)
	if len(parts) == 0 {
		return fmt.Errorf("empty command")
	}

	var cmd *exec.Cmd

	// Handle different command types
	if strings.HasPrefix(command, "docker run") {
		// For Docker commands, use the full command as-is with shell
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	} else {
		// For other commands, use the parsed parts
		cmd = exec.CommandContext(ctx, parts[0], parts[1:]...)
	}

	// Capture output for logging
	output, err := cmd.CombinedOutput()

	if err != nil {
		s.logger.Error("Command execution failed",
			zap.String("command", command),
			zap.String("output", string(output)),
			zap.Error(err))
		return fmt.Errorf("command failed: %w, output: %s", err, string(output))
	}

	s.logger.Info("Command executed successfully",
		zap.String("command", command),
		zap.String("output", string(output)))

	return nil
}

// parseCommand splits a command string into executable parts
func (s *CLIInvokerService) parseCommand(command string) []string {
	// Simple command parsing - splits on spaces but respects quotes
	var parts []string
	var current strings.Builder
	inQuotes := false

	for i, char := range command {
		switch char {
		case '"':
			inQuotes = !inQuotes
		case ' ':
			if !inQuotes {
				if current.Len() > 0 {
					parts = append(parts, current.String())
					current.Reset()
				}
				continue
			}
			current.WriteRune(char)
		default:
			current.WriteRune(char)
		}

		// Handle end of string
		if i == len(command)-1 && current.Len() > 0 {
			parts = append(parts, current.String())
		}
	}

	return parts
}

// ValidateCommand checks if the CLI command is properly configured
func (s *CLIInvokerService) ValidateCommand() error {
	if s.cliCommand == "" {
		return fmt.Errorf("CLI command is not configured")
	}

	// Basic validation - check if it contains expected patterns
	if !strings.Contains(s.cliCommand, "globeco-portfolio-cli") &&
		!strings.Contains(s.cliCommand, "portfolio") {
		s.logger.Warn("CLI command may not be valid Portfolio Accounting CLI command",
			zap.String("command", s.cliCommand))
	}

	return nil
}

// GetCommand returns the configured CLI command (for testing/debugging)
func (s *CLIInvokerService) GetCommand() string {
	return s.cliCommand
}
