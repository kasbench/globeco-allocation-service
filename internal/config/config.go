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

	// Kubernetes batch job configuration
	KubernetesBatch KubernetesBatchConfig `mapstructure:"kubernetes_batch"`
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
	// OpenTelemetry configuration
	OTELEnabled          bool   `mapstructure:"otel_enabled"`
	OTELEndpoint         string `mapstructure:"otel_endpoint"`
	OTELServiceName      string `mapstructure:"otel_service_name"`
	OTELServiceVersion   string `mapstructure:"otel_service_version"`
	OTELServiceNamespace string `mapstructure:"otel_service_namespace"`

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

	// Enhanced metrics configuration
	EnhancedMetricsEnabled               bool `mapstructure:"enhanced_metrics_enabled"`
	EnhancedMetricsMaxPathPatternCache   int  `mapstructure:"enhanced_metrics_max_path_pattern_cache"`
	EnhancedMetricsMaxPathLength         int  `mapstructure:"enhanced_metrics_max_path_length"`
	EnhancedMetricsEnableFailsafeLogging bool `mapstructure:"enhanced_metrics_enable_failsafe_logging"`
}

// KubernetesBatchConfig holds Kubernetes batch job configuration
type KubernetesBatchConfig struct {
	Enabled            bool   `mapstructure:"enabled"`
	Namespace          string `mapstructure:"namespace"`
	CLIImage           string `mapstructure:"cli_image"`
	JobTimeoutSeconds  int    `mapstructure:"job_timeout_seconds"`
	JobRetryLimit      int    `mapstructure:"job_retry_limit"`
	ServiceAccountName string `mapstructure:"service_account_name"`
	NFSPVCName         string `mapstructure:"nfs_pvc_name"`
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
	v.SetDefault("output_dir", "/data")
	// Use {home} as a placeholder for the user's home directory; replace at runtime.
	v.SetDefault("cli_command", "docker run --rm -v {home}/docker_data:/data --network my-network kasbench/globeco-portfolio-accounting-service-cli:latest process --file /data/{filename} --output-dir /data")

	// "$HOME/docker_data:/data"

	// Retry configuration defaults
	v.SetDefault("retry_max_attempts", 3)
	v.SetDefault("retry_base_delay_ms", 1000)

	// File management defaults
	v.SetDefault("file_cleanup_enabled", false)

	// OpenTelemetry defaults (GlobeCo standards)
	v.SetDefault("observability.otel_enabled", true)
	v.SetDefault("observability.otel_endpoint", "otel-collector-collector.monitoring.svc.cluster.local:4317")
	v.SetDefault("observability.otel_service_name", "globeco-allocation-service")
	v.SetDefault("observability.otel_service_version", "1.0.0")
	v.SetDefault("observability.otel_service_namespace", "globeco")

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

	// Enhanced metrics defaults
	v.SetDefault("observability.enhanced_metrics_enabled", true)
	v.SetDefault("observability.enhanced_metrics_max_path_pattern_cache", 1000)
	v.SetDefault("observability.enhanced_metrics_max_path_length", 100)
	v.SetDefault("observability.enhanced_metrics_enable_failsafe_logging", false)

	// Kubernetes batch job defaults
	v.SetDefault("kubernetes_batch.enabled", true)
	v.SetDefault("kubernetes_batch.namespace", "globeco")
	v.SetDefault("kubernetes_batch.cli_image", "kasbench/globeco-portfolio-accounting-service-cli:latest")
	v.SetDefault("kubernetes_batch.job_timeout_seconds", 1800) // 30 minutes
	v.SetDefault("kubernetes_batch.job_retry_limit", 2)
	v.SetDefault("kubernetes_batch.service_account_name", "globeco-allocation-service")
	v.SetDefault("kubernetes_batch.nfs_pvc_name", "nfs-pvc")
}

// DatabaseConnectionString returns the PostgreSQL connection string
func (d Database) ConnectionString() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.Name, d.SSLMode)
}
