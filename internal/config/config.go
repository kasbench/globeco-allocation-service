package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config holds all configuration for the application
type Config struct {
	Port               int      `mapstructure:"port"`
	LogLevel           string   `mapstructure:"log_level"`
	MetricsEnabled     bool     `mapstructure:"metrics_enabled"`
	TracingEnabled     bool     `mapstructure:"tracing_enabled"`
	Database           Database `mapstructure:"database"`
	TradeServiceURL    string   `mapstructure:"trade_service_url"`
	OutputDir          string   `mapstructure:"output_dir"`
	CLICommand         string   `mapstructure:"cli_command"`
	RetryMaxAttempts   int      `mapstructure:"retry_max_attempts"`
	RetryBaseDelay     int      `mapstructure:"retry_base_delay_ms"`
	FileCleanupEnabled bool     `mapstructure:"file_cleanup_enabled"`

	// Observability configuration
	Observability ObservabilityConfig `mapstructure:"observability"`
}

// Database holds database configuration
type Database struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Name     string `mapstructure:"name"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	SSLMode  string `mapstructure:"ssl_mode"`
}

// ObservabilityConfig holds observability configuration
type ObservabilityConfig struct {
	// Tracing configuration
	TracingEnabled       bool              `mapstructure:"tracing_enabled"`
	TracingOTLPEndpoint  string            `mapstructure:"tracing_otlp_endpoint"`
	TracingSamplingRatio float64           `mapstructure:"tracing_sampling_ratio"`
	TracingHeaders       map[string]string `mapstructure:"tracing_headers"`

	// Enhanced logging configuration
	LogFormat            string `mapstructure:"log_format"`
	LogEnableCaller      bool   `mapstructure:"log_enable_caller"`
	LogEnableStacktrace  bool   `mapstructure:"log_enable_stacktrace"`
	LogDevelopment       bool   `mapstructure:"log_development"`
	LogDisableSampling   bool   `mapstructure:"log_disable_sampling"`
	LogCorrelationHeader string `mapstructure:"log_correlation_header"`

	// Metrics configuration
	MetricsEnabled       bool   `mapstructure:"metrics_enabled"`
	MetricsPath          string `mapstructure:"metrics_path"`
	MetricsListenAddress string `mapstructure:"metrics_listen_address"`
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Read from environment variables
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("port", 8089)
	v.SetDefault("log_level", "info")
	v.SetDefault("metrics_enabled", true)
	v.SetDefault("tracing_enabled", true)

	// Database defaults
	v.SetDefault("database.host", "globeco-allocation-service-postgresql")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.name", "postgres")
	v.SetDefault("database.user", "postgres")
	v.SetDefault("database.password", "")
	v.SetDefault("database.ssl_mode", "disable")

	// External service defaults
	v.SetDefault("trade_service_url", "http://globeco-trade-service:8082")
	v.SetDefault("output_dir", "/usr/local/share/files")
	v.SetDefault("cli_command", "")

	// Retry configuration defaults
	v.SetDefault("retry_max_attempts", 3)
	v.SetDefault("retry_base_delay_ms", 1000)

	// File management defaults
	v.SetDefault("file_cleanup_enabled", false)

	// Observability defaults
	v.SetDefault("observability.tracing_enabled", true)
	v.SetDefault("observability.tracing_otlp_endpoint", "")
	v.SetDefault("observability.tracing_sampling_ratio", 1.0)
	v.SetDefault("observability.tracing_headers", map[string]string{})

	v.SetDefault("observability.log_format", "json")
	v.SetDefault("observability.log_enable_caller", true)
	v.SetDefault("observability.log_enable_stacktrace", true)
	v.SetDefault("observability.log_development", false)
	v.SetDefault("observability.log_disable_sampling", false)
	v.SetDefault("observability.log_correlation_header", "X-Correlation-ID")

	v.SetDefault("observability.metrics_enabled", true)
	v.SetDefault("observability.metrics_path", "/metrics")
	v.SetDefault("observability.metrics_listen_address", "")
}

// DatabaseConnectionString returns the PostgreSQL connection string
func (d Database) ConnectionString() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.Name, d.SSLMode)
}
