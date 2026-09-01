package service

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"strings"
	"time"

	"go.uber.org/zap"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/kasbench/globeco-allocation-service/internal/config"
)

// KubernetesBatchInvoker defines the interface for Kubernetes batch job operations
type KubernetesBatchInvoker interface {
	// InvokePortfolioAccountingCLI creates and submits a batch job for CLI execution
	InvokePortfolioAccountingCLI(ctx context.Context, filename string, outputDir string) error

	// RetryBatchJob retries failed batch jobs without regenerating files
	RetryBatchJob(ctx context.Context, filename string, outputDir string) error

	// ValidateKubernetesAccess validates RBAC permissions and connectivity
	ValidateKubernetesAccess() error

	// GetJobStatus retrieves the current status of a job
	GetJobStatus(ctx context.Context, jobName string) (*JobStatus, error)

	// CleanupCompletedJobs removes completed jobs older than specified duration
	CleanupCompletedJobs(ctx context.Context, olderThan time.Duration) error
}

// KubernetesBatchInvokerService implements the KubernetesBatchInvoker interface
type KubernetesBatchInvokerService struct {
	clientset   *kubernetes.Clientset
	config      *config.KubernetesBatchConfig
	logger      *zap.Logger
	timeout     time.Duration
	retryConfig *RetryConfig
}

// RetryConfig holds configuration for retry mechanisms
type RetryConfig struct {
	MaxRetries         int           `json:"max_retries"`
	BaseDelay          time.Duration `json:"base_delay"`
	MaxDelay           time.Duration `json:"max_delay"`
	BackoffMultiplier  float64       `json:"backoff_multiplier"`
	JitterEnabled      bool          `json:"jitter_enabled"`
	RetryableErrors    []string      `json:"retryable_errors"`
	NonRetryableErrors []string      `json:"non_retryable_errors"`
}

// ErrorType represents different types of Kubernetes errors
type ErrorType string

const (
	ErrorTypeTransient     ErrorType = "transient"
	ErrorTypePermission    ErrorType = "permission"
	ErrorTypeValidation    ErrorType = "validation"
	ErrorTypeConnectivity  ErrorType = "connectivity"
	ErrorTypeResourceLimit ErrorType = "resource_limit"
	ErrorTypeUnknown       ErrorType = "unknown"
)

// KubernetesError wraps Kubernetes API errors with additional context
type KubernetesError struct {
	Type        ErrorType
	OriginalErr error
	Message     string
	Retryable   bool
	JobName     string
	Namespace   string
	Operation   string
	Suggestions []string
}

// BatchJobConfig holds configuration for creating batch jobs
type BatchJobConfig struct {
	JobName        string
	Namespace      string
	Image          string
	Filename       string
	OutputDir      string
	ServiceAccount string
	Timeout        time.Duration
	RetryLimit     int32
	NFSVolumeClaim string
}

// JobStatus represents the status of a Kubernetes job
type JobStatus struct {
	JobName   string
	Status    string // "pending", "running", "succeeded", "failed"
	StartTime time.Time
	EndTime   *time.Time
	Message   string
	PodName   string
}

// Error implements the error interface for KubernetesError
func (e *KubernetesError) Error() string {
	return fmt.Sprintf("[%s] %s: %s", e.Type, e.Operation, e.Message)
}

// NewKubernetesBatchInvokerService creates a new Kubernetes batch invoker service
func NewKubernetesBatchInvokerService(cfg *config.KubernetesBatchConfig, logger *zap.Logger) (*KubernetesBatchInvokerService, error) {
	if cfg == nil {
		return nil, fmt.Errorf("kubernetes batch config is required")
	}

	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	// Create Kubernetes client configuration
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		logger.Error("Failed to create in-cluster Kubernetes config", zap.Error(err))
		return nil, fmt.Errorf("failed to create in-cluster config: %w", err)
	}

	// Create Kubernetes clientset
	clientset, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		logger.Error("Failed to create Kubernetes clientset", zap.Error(err))
		return nil, fmt.Errorf("failed to create Kubernetes clientset: %w", err)
	}

	// Set timeout from config or use default
	timeout := time.Duration(cfg.JobTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Minute // Default timeout
	}

	// Initialize retry configuration with defaults
	retryConfig := &RetryConfig{
		MaxRetries:        5,
		BaseDelay:         1 * time.Second,
		MaxDelay:          30 * time.Second,
		BackoffMultiplier: 2.0,
		JitterEnabled:     true,
		RetryableErrors: []string{
			"connection refused",
			"timeout",
			"temporary failure",
			"service unavailable",
			"too many requests",
			"internal server error",
			"bad gateway",
			"gateway timeout",
		},
		NonRetryableErrors: []string{
			"forbidden",
			"unauthorized",
			"invalid",
			"not found",
			"conflict",
			"bad request",
		},
	}

	service := &KubernetesBatchInvokerService{
		clientset:   clientset,
		config:      cfg,
		logger:      logger,
		timeout:     timeout,
		retryConfig: retryConfig,
	}

	logger.Info("Kubernetes batch invoker service initialized",
		zap.String("namespace", cfg.Namespace),
		zap.String("cli_image", cfg.CLIImage),
		zap.Duration("timeout", timeout),
		zap.Int("retry_limit", cfg.JobRetryLimit),
		zap.Int("max_retries", retryConfig.MaxRetries),
		zap.Duration("base_delay", retryConfig.BaseDelay))

	return service, nil
}

// InvokePortfolioAccountingCLI creates and submits a batch job for CLI execution
func (s *KubernetesBatchInvokerService) InvokePortfolioAccountingCLI(ctx context.Context, filename string, outputDir string) error {
	if filename == "" {
		return fmt.Errorf("filename is required")
	}

	if outputDir == "" {
		return fmt.Errorf("output directory is required")
	}

	s.logger.Info("Creating Kubernetes batch job for Portfolio Accounting CLI",
		zap.String("filename", filename),
		zap.String("outputDir", outputDir))

	// Generate unique job name with timestamp (Requirement 1.3: proper job naming)
	jobName := s.generateJobName("portfolio-cli")

	// Create batch job configuration
	jobConfig := &BatchJobConfig{
		JobName:        jobName,
		Namespace:      s.config.Namespace,
		Image:          s.config.CLIImage,
		Filename:       filename,
		OutputDir:      outputDir,
		ServiceAccount: s.config.ServiceAccountName,
		Timeout:        s.timeout,
		RetryLimit:     int32(s.config.JobRetryLimit),
		NFSVolumeClaim: s.config.NFSPVCName,
	}

	// Create and submit the job
	job, err := s.createBatchJob(jobConfig)
	if err != nil {
		s.logger.Error("Failed to create batch job manifest", zap.Error(err))
		return fmt.Errorf("failed to create batch job manifest: %w", err)
	}

	// Validate job before submission (Requirement 1.4: enhanced job submission)
	if err := s.validateJobSubmission(job); err != nil {
		s.logger.Error("Job validation failed",
			zap.String("job_name", jobName),
			zap.Error(err))
		return fmt.Errorf("job validation failed: %w", err)
	}

	// Submit job to Kubernetes API with retry logic (Requirement 1.4, 2.1)
	createdJob, err := s.submitJobWithRetry(ctx, job)
	if err != nil {
		s.logger.Error("Failed to submit batch job to Kubernetes",
			zap.String("job_name", jobName),
			zap.Error(err))
		return fmt.Errorf("failed to submit batch job: %w", err)
	}

	s.logger.Info("Batch job submitted successfully",
		zap.String("job_name", createdJob.Name),
		zap.String("namespace", createdJob.Namespace),
		zap.String("uid", string(createdJob.UID)))

	// Monitor job completion
	if err := s.monitorJobCompletion(ctx, jobName); err != nil {
		s.logger.Error("Batch job failed or timed out",
			zap.String("job_name", jobName),
			zap.Error(err))
		return fmt.Errorf("batch job failed: %w", err)
	}

	s.logger.Info("Batch job completed successfully",
		zap.String("job_name", jobName),
		zap.String("filename", filename))

	return nil
}

// RetryBatchJob retries failed batch jobs without regenerating files
func (s *KubernetesBatchInvokerService) RetryBatchJob(ctx context.Context, filename string, outputDir string) error {
	s.logger.Info("Retrying batch job for existing file",
		zap.String("filename", filename),
		zap.String("outputDir", outputDir))

	// Generate unique retry job name with timestamp
	jobName := s.generateJobName("portfolio-cli-retry")

	jobConfig := &BatchJobConfig{
		JobName:        jobName,
		Namespace:      s.config.Namespace,
		Image:          s.config.CLIImage,
		Filename:       filename,
		OutputDir:      outputDir,
		ServiceAccount: s.config.ServiceAccountName,
		Timeout:        s.timeout,
		RetryLimit:     int32(s.config.JobRetryLimit),
		NFSVolumeClaim: s.config.NFSPVCName,
	}

	// Create and submit the retry job
	job, err := s.createBatchJob(jobConfig)
	if err != nil {
		return fmt.Errorf("failed to create retry batch job manifest: %w", err)
	}

	// Validate retry job before submission
	if err := s.validateJobSubmission(job); err != nil {
		s.logger.Error("Retry job validation failed",
			zap.String("job_name", jobName),
			zap.Error(err))
		return fmt.Errorf("retry job validation failed: %w", err)
	}

	// Submit retry job with enhanced error handling
	createdJob, err := s.submitJobWithRetry(ctx, job)
	if err != nil {
		s.logger.Error("Failed to submit retry batch job to Kubernetes",
			zap.String("job_name", jobName),
			zap.Error(err))
		return fmt.Errorf("failed to submit retry batch job: %w", err)
	}

	s.logger.Info("Retry batch job submitted successfully",
		zap.String("job_name", createdJob.Name),
		zap.String("namespace", createdJob.Namespace),
		zap.String("uid", string(createdJob.UID)))

	// Monitor job completion
	if err := s.monitorJobCompletion(ctx, jobName); err != nil {
		return fmt.Errorf("retry batch job failed: %w", err)
	}

	s.logger.Info("Retry batch job completed successfully",
		zap.String("job_name", jobName),
		zap.String("filename", filename))

	return nil
}

// ValidateKubernetesAccess validates RBAC permissions and connectivity with enhanced error handling
// Implements comprehensive error handling for RBAC permission issues (Requirements 7.2, 7.3)
func (s *KubernetesBatchInvokerService) ValidateKubernetesAccess() error {
	return s.enhanceValidateKubernetesAccess()
}

// buildCLIArgs constructs the CLI arguments based on the job configuration
func buildCLIArgs(config *BatchJobConfig) []string {
	args := []string{
		"process",
		"--file", fmt.Sprintf("/data/%s", config.Filename),
		"--output-dir", config.OutputDir,
		"--config", "/etc/config/config.yaml",
	}

	return args
}

// createBatchJob creates a Kubernetes Job manifest for Portfolio Accounting CLI
// This method implements dynamic job manifest generation with proper volume mounts,
// job naming with timestamps, and comprehensive labeling as per requirements 1.3 and 6.2
func (s *KubernetesBatchInvokerService) createBatchJob(config *BatchJobConfig) (*batchv1.Job, error) {
	if config == nil {
		return nil, fmt.Errorf("batch job config is required")
	}

	// Validate required configuration fields
	if config.JobName == "" {
		return nil, fmt.Errorf("job name is required")
	}
	if config.Namespace == "" {
		return nil, fmt.Errorf("namespace is required")
	}
	if config.Image == "" {
		return nil, fmt.Errorf("container image is required")
	}
	if config.Filename == "" {
		return nil, fmt.Errorf("filename is required")
	}

	// Create comprehensive job labels with timestamp and proper identification
	// Requirement 1.3: Proper labeling for job identification and management
	labels := map[string]string{
		"app":                   "portfolio-accounting-cli",
		"component":             "batch-processor",
		"managed-by":            "globeco-allocation-service",
		"job-name":              config.JobName,
		"globeco.io/service":    "allocation-service",
		"globeco.io/job-type":   "portfolio-accounting",
		"globeco.io/created-by": "kubernetes-batch-invoker",
		"globeco.io/filename":   sanitizeLabelValue(config.Filename),
	}

	// Create container spec with enhanced configuration
	container := corev1.Container{
		Name:    "portfolio-cli",
		Image:   config.Image,
		Command: []string{"/usr/local/bin/cli"},
		Args:    buildCLIArgs(config),
		// Requirement 6.2: Proper volume mounts for NFS access
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "nfs-storage",
				MountPath: "/data",
				ReadOnly:  false, // Portfolio accounting service needs write access for output and error files
			},
			{
				Name:      "cli-config",
				MountPath: "/etc/config",
				ReadOnly:  true, // Config should be read-only
			},
		},
		Env: []corev1.EnvVar{
			{
				Name:  "GLOBECO_PA_SERVER_HOST",
				Value: "globeco-allocation-service",
			},
			{
				Name:  "GLOBECO_PA_SERVER_PORT",
				Value: "8089",
			},
			{
				Name:  "JOB_NAME",
				Value: config.JobName,
			},
			{
				Name:  "INPUT_FILENAME",
				Value: config.Filename,
			},
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("512Mi"),
				corev1.ResourceCPU:    resource.MustParse("250m"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("1Gi"),
				corev1.ResourceCPU:    resource.MustParse("500m"),
			},
		},
		// Add security context for better security posture
		SecurityContext: &corev1.SecurityContext{
			RunAsNonRoot:             boolPtr(true),
			RunAsUser:                int64Ptr(1000),
			AllowPrivilegeEscalation: boolPtr(false),
			ReadOnlyRootFilesystem:   boolPtr(true),
		},
	}

	// Create volumes with proper NFS and ConfigMap configuration
	// Requirement 6.2: Proper volume configuration for NFS access
	volumes := []corev1.Volume{
		{
			Name: "nfs-storage",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: config.NFSVolumeClaim,
				},
			},
		},
		{
			Name: "cli-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "portfolio-cli-config",
					},
					DefaultMode: int32Ptr(0444), // Read-only permissions
				},
			},
		},
	}

	// Create pod template spec with enhanced configuration
	podSpec := corev1.PodSpec{
		RestartPolicy:      corev1.RestartPolicyNever,
		ServiceAccountName: config.ServiceAccount,
		Containers:         []corev1.Container{container},
		Volumes:            volumes,
		// Add security context at pod level
		SecurityContext: &corev1.PodSecurityContext{
			FSGroup: int64Ptr(1000),
		},
		// Add node selector for better scheduling if needed
		NodeSelector: map[string]string{
			"kubernetes.io/os": "linux",
		},
	}

	// Create job spec with comprehensive configuration
	// Requirement 1.3: Proper job configuration with timeouts and retry limits
	jobSpec := batchv1.JobSpec{
		TTLSecondsAfterFinished: int32Ptr(3600), // Clean up after 1 hour
		BackoffLimit:            &config.RetryLimit,
		ActiveDeadlineSeconds:   int64Ptr(int64(config.Timeout.Seconds())),
		Completions:             int32Ptr(1), // Ensure exactly one successful completion
		Parallelism:             int32Ptr(1), // Run only one pod at a time
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: labels,
				Annotations: map[string]string{
					"globeco.io/created-at": time.Now().UTC().Format(time.RFC3339),
					"globeco.io/filename":   config.Filename,
					"globeco.io/output-dir": config.OutputDir,
				},
			},
			Spec: podSpec,
		},
	}

	// Create the job with comprehensive metadata
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.JobName,
			Namespace: config.Namespace,
			Labels:    labels,
			Annotations: map[string]string{
				"globeco.io/created-at":      time.Now().UTC().Format(time.RFC3339),
				"globeco.io/filename":        config.Filename,
				"globeco.io/output-dir":      config.OutputDir,
				"globeco.io/service-account": config.ServiceAccount,
				"globeco.io/nfs-pvc":         config.NFSVolumeClaim,
			},
		},
		Spec: jobSpec,
	}

	s.logger.Debug("Created batch job manifest",
		zap.String("job_name", config.JobName),
		zap.String("namespace", config.Namespace),
		zap.String("image", config.Image),
		zap.String("filename", config.Filename),
		zap.String("service_account", config.ServiceAccount),
		zap.String("nfs_pvc", config.NFSVolumeClaim))

	return job, nil
}

// monitorJobCompletion monitors a job until completion or timeout
// Implements comprehensive job status monitoring with polling mechanism (Requirement 1.4, 2.1, 2.2)
func (s *KubernetesBatchInvokerService) monitorJobCompletion(ctx context.Context, jobName string) error {
	s.logger.Info("Starting job monitoring with enhanced polling mechanism",
		zap.String("job_name", jobName),
		zap.Duration("timeout", s.timeout))

	// Create context with timeout for monitoring (Requirement 2.1: timeout handling).
	//
	// IMPORTANT: derive from context.Background() rather than the request-scoped ctx.
	// The incoming ctx is the HTTP request context, which is cancelled as soon as the
	// caller (or any proxy/load balancer in front of the service) closes the connection.
	// A typical proxy/client idle timeout (~120s) would otherwise cancel monitorCtx and
	// abandon a batch job that is still running, while misreporting it as an s.timeout
	// ("30m") timeout. Once the job is submitted, its lifecycle must not be tied to the
	// inbound request. We still observe the parent ctx separately below so we can log the
	// real cause, but we no longer let it kill the job monitoring.
	monitorCtx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()

	// Use adaptive polling intervals for better efficiency
	initialInterval := 2 * time.Second
	maxInterval := 10 * time.Second
	currentInterval := initialInterval

	ticker := time.NewTicker(currentInterval)
	defer ticker.Stop()

	startTime := time.Now()
	consecutiveErrors := 0
	maxConsecutiveErrors := 5

	for {
		select {
		case <-ctx.Done():
			// The request-scoped context was cancelled (e.g. the caller or an intermediate
			// proxy/load balancer closed the connection). The batch job itself is unaffected
			// and continues running in Kubernetes; we simply stop actively monitoring it here.
			// This is logged as a warning rather than a timeout so it isn't confused with an
			// actual s.timeout expiry.
			elapsed := time.Since(startTime)
			s.logger.Warn("Stopped monitoring job because the request context was cancelled; the Kubernetes job continues running",
				zap.String("job_name", jobName),
				zap.Duration("elapsed", elapsed),
				zap.Error(ctx.Err()))

			if finalJob, err := s.clientset.BatchV1().Jobs(s.config.Namespace).Get(context.Background(), jobName, metav1.GetOptions{}); err == nil {
				s.logJobStatusDetails(finalJob, "request-cancelled")
			}

			return fmt.Errorf("stopped monitoring job %s after %v: request context cancelled (%w); the Kubernetes job may still be running", jobName, elapsed, ctx.Err())

		case <-monitorCtx.Done():
			// Enhanced timeout handling with detailed error information (Requirement 2.1).
			// monitorCtx is derived from context.Background(), so reaching here means the
			// configured monitoring timeout (s.timeout) genuinely elapsed.
			elapsed := time.Since(startTime)
			s.logger.Error("Job monitoring timed out",
				zap.String("job_name", jobName),
				zap.Duration("elapsed", elapsed),
				zap.Duration("timeout", s.timeout))

			// Try to get final job status for debugging
			if finalJob, err := s.clientset.BatchV1().Jobs(s.config.Namespace).Get(context.Background(), jobName, metav1.GetOptions{}); err == nil {
				s.logJobStatusDetails(finalJob, "timeout")
			}

			return fmt.Errorf("job monitoring timed out after %v (elapsed: %v)", s.timeout, elapsed)

		case <-ticker.C:
			// Get current job status with error handling (Requirement 1.4: job status monitoring)
			job, err := s.clientset.BatchV1().Jobs(s.config.Namespace).Get(monitorCtx, jobName, metav1.GetOptions{})
			if err != nil {
				consecutiveErrors++
				s.logger.Error("Failed to get job status",
					zap.String("job_name", jobName),
					zap.Error(err),
					zap.Int("consecutive_errors", consecutiveErrors))

				// If we have too many consecutive errors, fail the monitoring
				if consecutiveErrors >= maxConsecutiveErrors {
					return fmt.Errorf("failed to monitor job after %d consecutive API errors: %w", maxConsecutiveErrors, err)
				}
				continue
			}

			// Reset error counter on successful API call
			consecutiveErrors = 0

			// Enhanced job completion detection (Requirement 2.2: job completion detection)
			jobStatus := s.analyzeJobStatus(job)

			switch jobStatus.Status {
			case "succeeded":
				s.logger.Info("Job completed successfully",
					zap.String("job_name", jobName),
					zap.Duration("elapsed", time.Since(startTime)),
					zap.String("message", jobStatus.Message))
				s.logJobStatusDetails(job, "success")
				return nil

			case "failed":
				s.logger.Error("Job failed",
					zap.String("job_name", jobName),
					zap.Duration("elapsed", time.Since(startTime)),
					zap.String("reason", jobStatus.Message))
				s.logJobStatusDetails(job, "failed")

				// Get pod logs for debugging if available
				s.logPodDetailsForDebugging(monitorCtx, jobName)

				return fmt.Errorf("job failed: %s", jobStatus.Message)

			case "running":
				// Job is actively running, log progress
				s.logger.Debug("Job is running",
					zap.String("job_name", jobName),
					zap.Duration("elapsed", time.Since(startTime)),
					zap.Int32("active_pods", job.Status.Active))

				// Adjust polling interval for running jobs (less frequent polling)
				if currentInterval < maxInterval {
					currentInterval = time.Duration(float64(currentInterval) * 1.5)
					if currentInterval > maxInterval {
						currentInterval = maxInterval
					}
					ticker.Reset(currentInterval)
				}

			case "pending":
				// Job is pending, check for stuck conditions
				elapsed := time.Since(startTime)
				if elapsed > 5*time.Minute {
					s.logger.Warn("Job has been pending for extended time",
						zap.String("job_name", jobName),
						zap.Duration("elapsed", elapsed))

					// Log pod details to help debug why job is stuck
					s.logPodDetailsForDebugging(monitorCtx, jobName)
				}

			default:
				// Unknown status, log for debugging
				s.logger.Debug("Job status update",
					zap.String("job_name", jobName),
					zap.String("status", jobStatus.Status),
					zap.Duration("elapsed", time.Since(startTime)),
					zap.Int32("active", job.Status.Active),
					zap.Int32("succeeded", job.Status.Succeeded),
					zap.Int32("failed", job.Status.Failed))
			}

			// Check for job deadline exceeded (additional timeout detection)
			if job.Status.StartTime != nil {
				jobRuntime := time.Since(job.Status.StartTime.Time)
				if jobRuntime > s.timeout {
					s.logger.Error("Job exceeded maximum runtime",
						zap.String("job_name", jobName),
						zap.Duration("runtime", jobRuntime),
						zap.Duration("max_timeout", s.timeout))
					return fmt.Errorf("job exceeded maximum runtime: %v", jobRuntime)
				}
			}
		}
	}
}

// analyzeJobStatus analyzes job status and returns structured status information
// Implements enhanced job completion detection (Requirement 2.2)
func (s *KubernetesBatchInvokerService) analyzeJobStatus(job *batchv1.Job) *JobStatus {
	status := &JobStatus{
		JobName:   job.Name,
		Status:    "unknown",
		StartTime: time.Now(),
		Message:   "Job status unknown",
	}

	// Set start time if available
	if job.Status.StartTime != nil {
		status.StartTime = job.Status.StartTime.Time
	}

	// Check job conditions for completion status
	for _, condition := range job.Status.Conditions {
		switch condition.Type {
		case batchv1.JobComplete:
			if condition.Status == corev1.ConditionTrue {
				status.Status = "succeeded"
				status.Message = fmt.Sprintf("Job completed successfully: %s", condition.Message)
				if condition.LastTransitionTime.Time.After(status.StartTime) {
					endTime := condition.LastTransitionTime.Time
					status.EndTime = &endTime
				}
				return status
			}

		case batchv1.JobFailed:
			if condition.Status == corev1.ConditionTrue {
				status.Status = "failed"
				status.Message = fmt.Sprintf("Job failed - %s: %s", condition.Reason, condition.Message)
				if condition.LastTransitionTime.Time.After(status.StartTime) {
					endTime := condition.LastTransitionTime.Time
					status.EndTime = &endTime
				}
				return status
			}
		}
	}

	// Analyze job status based on pod counts
	if job.Status.Active > 0 {
		status.Status = "running"
		status.Message = fmt.Sprintf("Job is running with %d active pod(s)", job.Status.Active)
	} else if job.Status.Succeeded > 0 {
		status.Status = "succeeded"
		status.Message = fmt.Sprintf("Job succeeded with %d completed pod(s)", job.Status.Succeeded)
	} else if job.Status.Failed > 0 {
		status.Status = "failed"
		status.Message = fmt.Sprintf("Job failed with %d failed pod(s)", job.Status.Failed)
	} else {
		status.Status = "pending"
		status.Message = "Job is pending - no active, succeeded, or failed pods"
	}

	return status
}

// logJobStatusDetails logs comprehensive job status information for debugging
// Supports enhanced error handling and status reporting (Requirement 2.1, 2.2)
func (s *KubernetesBatchInvokerService) logJobStatusDetails(job *batchv1.Job, context string) {
	s.logger.Info("Job status details",
		zap.String("context", context),
		zap.String("job_name", job.Name),
		zap.String("namespace", job.Namespace),
		zap.Int32("active_pods", job.Status.Active),
		zap.Int32("succeeded_pods", job.Status.Succeeded),
		zap.Int32("failed_pods", job.Status.Failed),
		zap.Any("start_time", job.Status.StartTime),
		zap.Any("completion_time", job.Status.CompletionTime),
		zap.Int("conditions_count", len(job.Status.Conditions)))

	// Log each condition for detailed debugging
	for i, condition := range job.Status.Conditions {
		s.logger.Debug("Job condition",
			zap.String("job_name", job.Name),
			zap.Int("condition_index", i),
			zap.String("type", string(condition.Type)),
			zap.String("status", string(condition.Status)),
			zap.String("reason", condition.Reason),
			zap.String("message", condition.Message),
			zap.Time("last_transition", condition.LastTransitionTime.Time))
	}
}

// logPodDetailsForDebugging retrieves and logs pod information for debugging failed or stuck jobs
// Supports enhanced error handling and debugging (Requirement 2.1)
func (s *KubernetesBatchInvokerService) logPodDetailsForDebugging(ctx context.Context, jobName string) {
	// List pods associated with this job
	labelSelector := fmt.Sprintf("job-name=%s", jobName)
	pods, err := s.clientset.CoreV1().Pods(s.config.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})

	if err != nil {
		s.logger.Error("Failed to list pods for job debugging",
			zap.String("job_name", jobName),
			zap.Error(err))
		return
	}

	s.logger.Info("Pod details for job debugging",
		zap.String("job_name", jobName),
		zap.Int("pod_count", len(pods.Items)))

	for _, pod := range pods.Items {
		s.logger.Debug("Pod status",
			zap.String("job_name", jobName),
			zap.String("pod_name", pod.Name),
			zap.String("phase", string(pod.Status.Phase)),
			zap.String("reason", pod.Status.Reason),
			zap.String("message", pod.Status.Message),
			zap.Any("start_time", pod.Status.StartTime))

		// Log container statuses
		for _, containerStatus := range pod.Status.ContainerStatuses {
			s.logger.Debug("Container status",
				zap.String("job_name", jobName),
				zap.String("pod_name", pod.Name),
				zap.String("container_name", containerStatus.Name),
				zap.Bool("ready", containerStatus.Ready),
				zap.Int32("restart_count", containerStatus.RestartCount))

			// Log container state details
			if containerStatus.State.Waiting != nil {
				s.logger.Debug("Container waiting",
					zap.String("job_name", jobName),
					zap.String("pod_name", pod.Name),
					zap.String("container_name", containerStatus.Name),
					zap.String("reason", containerStatus.State.Waiting.Reason),
					zap.String("message", containerStatus.State.Waiting.Message))
			}

			if containerStatus.State.Terminated != nil {
				s.logger.Debug("Container terminated",
					zap.String("job_name", jobName),
					zap.String("pod_name", pod.Name),
					zap.String("container_name", containerStatus.Name),
					zap.Int32("exit_code", containerStatus.State.Terminated.ExitCode),
					zap.String("reason", containerStatus.State.Terminated.Reason),
					zap.String("message", containerStatus.State.Terminated.Message))
			}
		}

		// Try to get recent pod logs for debugging (limit to avoid overwhelming logs)
		if pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded {
			s.getPodLogsForDebugging(ctx, pod.Name, jobName)
		}
	}
}

// getPodLogsForDebugging retrieves recent pod logs for debugging purposes
// Supports enhanced error handling and debugging (Requirement 2.1)
func (s *KubernetesBatchInvokerService) getPodLogsForDebugging(ctx context.Context, podName, jobName string) {
	// Get logs with reasonable limits to avoid overwhelming the system
	logOptions := &corev1.PodLogOptions{
		TailLines: int64Ptr(50), // Last 50 lines
		Container: "portfolio-cli",
	}

	logStream, err := s.clientset.CoreV1().Pods(s.config.Namespace).GetLogs(podName, logOptions).Stream(ctx)
	if err != nil {
		s.logger.Error("Failed to get pod logs for debugging",
			zap.String("job_name", jobName),
			zap.String("pod_name", podName),
			zap.Error(err))
		return
	}
	defer logStream.Close()

	// Read logs (with size limit to prevent memory issues)
	logBytes := make([]byte, 4096) // 4KB limit
	n, err := logStream.Read(logBytes)
	if err != nil && err.Error() != "EOF" {
		s.logger.Error("Failed to read pod logs",
			zap.String("job_name", jobName),
			zap.String("pod_name", podName),
			zap.Error(err))
		return
	}

	if n > 0 {
		logContent := string(logBytes[:n])
		s.logger.Debug("Pod logs for debugging",
			zap.String("job_name", jobName),
			zap.String("pod_name", podName),
			zap.String("logs", logContent))
	}
}

// submitJobWithRetry submits a job to Kubernetes API with comprehensive retry logic
// Implements enhanced job submission with exponential backoff and error handling (Requirements 7.1, 7.3)
func (s *KubernetesBatchInvokerService) submitJobWithRetry(ctx context.Context, job *batchv1.Job) (*batchv1.Job, error) {
	operation := "job_submission"

	s.logger.Info("Starting job submission with retry logic",
		zap.String("job_name", job.Name),
		zap.String("namespace", job.Namespace),
		zap.Int("max_retries", s.retryConfig.MaxRetries),
		zap.Duration("base_delay", s.retryConfig.BaseDelay))

	var lastErr error

	for attempt := 0; attempt <= s.retryConfig.MaxRetries; attempt++ {
		// Log attempt details
		s.logger.Debug("Attempting job submission",
			zap.String("job_name", job.Name),
			zap.Int("attempt", attempt+1),
			zap.Int("max_attempts", s.retryConfig.MaxRetries+1))

		// Attempt to create the job
		createdJob, err := s.clientset.BatchV1().Jobs(s.config.Namespace).Create(ctx, job, metav1.CreateOptions{})
		if err == nil {
			s.logger.Info("Job submitted successfully",
				zap.String("job_name", job.Name),
				zap.String("namespace", job.Namespace),
				zap.String("uid", string(createdJob.UID)),
				zap.Int("attempt", attempt+1))
			return createdJob, nil
		}

		// Analyze the error and determine if it's retryable
		kubernetesErr := s.analyzeKubernetesError(err, job.Name, job.Namespace, operation)
		lastErr = kubernetesErr

		// Log detailed error information
		s.logger.Error("Job submission attempt failed",
			zap.String("job_name", job.Name),
			zap.Int("attempt", attempt+1),
			zap.String("error_type", string(kubernetesErr.Type)),
			zap.Bool("retryable", kubernetesErr.Retryable),
			zap.Error(kubernetesErr.OriginalErr),
			zap.String("message", kubernetesErr.Message),
			zap.Strings("suggestions", kubernetesErr.Suggestions))

		// If error is not retryable, fail immediately
		if !kubernetesErr.Retryable {
			s.logger.Error("Job submission failed with non-retryable error",
				zap.String("job_name", job.Name),
				zap.String("error_type", string(kubernetesErr.Type)),
				zap.String("message", kubernetesErr.Message),
				zap.Strings("suggestions", kubernetesErr.Suggestions))
			return nil, kubernetesErr
		}

		// If this was the last attempt, don't wait
		if attempt >= s.retryConfig.MaxRetries {
			break
		}

		// Calculate delay with exponential backoff and jitter
		delay := s.calculateRetryDelay(attempt)

		s.logger.Warn("Job submission failed, retrying after delay",
			zap.String("job_name", job.Name),
			zap.Int("attempt", attempt+1),
			zap.Int("max_retries", s.retryConfig.MaxRetries),
			zap.Duration("delay", delay),
			zap.String("error_type", string(kubernetesErr.Type)))

		// Wait before retrying
		select {
		case <-ctx.Done():
			return nil, &KubernetesError{
				Type:        ErrorTypeTransient,
				OriginalErr: ctx.Err(),
				Message:     "job submission cancelled due to context timeout",
				Retryable:   false,
				JobName:     job.Name,
				Namespace:   job.Namespace,
				Operation:   operation,
			}
		case <-time.After(delay):
			// Continue to next attempt
		}
	}

	// All retries exhausted
	s.logger.Error("Job submission failed after all retry attempts",
		zap.String("job_name", job.Name),
		zap.Int("total_attempts", s.retryConfig.MaxRetries+1),
		zap.Error(lastErr))

	return nil, &KubernetesError{
		Type:        ErrorTypeTransient,
		OriginalErr: lastErr,
		Message:     fmt.Sprintf("job submission failed after %d attempts", s.retryConfig.MaxRetries+1),
		Retryable:   false,
		JobName:     job.Name,
		Namespace:   job.Namespace,
		Operation:   operation,
		Suggestions: []string{
			"Check Kubernetes cluster health",
			"Verify RBAC permissions",
			"Check resource quotas and limits",
			"Review job manifest for validation errors",
		},
	}
}

// validateJobSubmission validates job configuration before submission
// Implements enhanced job submission validation (Requirement 1.4)
func (s *KubernetesBatchInvokerService) validateJobSubmission(job *batchv1.Job) error {
	if job == nil {
		return fmt.Errorf("job is nil")
	}

	if job.Name == "" {
		return fmt.Errorf("job name is required")
	}

	if job.Namespace == "" {
		return fmt.Errorf("job namespace is required")
	}

	if len(job.Spec.Template.Spec.Containers) == 0 {
		return fmt.Errorf("job must have at least one container")
	}

	container := job.Spec.Template.Spec.Containers[0]
	if container.Image == "" {
		return fmt.Errorf("container image is required")
	}

	// Validate volume mounts
	hasNFSMount := false
	for _, mount := range container.VolumeMounts {
		if mount.Name == "nfs-storage" {
			hasNFSMount = true
			break
		}
	}
	if !hasNFSMount {
		return fmt.Errorf("job must have NFS storage volume mount")
	}

	// Validate volumes
	hasNFSVolume := false
	for _, volume := range job.Spec.Template.Spec.Volumes {
		if volume.Name == "nfs-storage" && volume.PersistentVolumeClaim != nil {
			hasNFSVolume = true
			break
		}
	}
	if !hasNFSVolume {
		return fmt.Errorf("job must have NFS PVC volume")
	}

	s.logger.Debug("Job validation passed",
		zap.String("job_name", job.Name),
		zap.String("namespace", job.Namespace),
		zap.String("image", container.Image))

	return nil
}

// Helper functions for creating Kubernetes pointers
func int32Ptr(i int32) *int32 {
	return &i
}

func int64Ptr(i int64) *int64 {
	return &i
}

func boolPtr(b bool) *bool {
	return &b
}

// sanitizeLabelValue ensures label values conform to Kubernetes requirements
// Labels must be 63 characters or less and match regex [a-z0-9A-Z]([a-z0-9A-Z\-_.]*[a-z0-9A-Z])?
func sanitizeLabelValue(value string) string {
	if len(value) == 0 {
		return "unknown"
	}

	// Replace invalid characters with hyphens
	sanitized := ""
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			sanitized += string(r)
		} else {
			sanitized += "-"
		}
	}

	// Ensure it starts and ends with alphanumeric
	if len(sanitized) > 0 && !isAlphanumeric(rune(sanitized[0])) {
		sanitized = "x" + sanitized
	}
	if len(sanitized) > 0 && !isAlphanumeric(rune(sanitized[len(sanitized)-1])) {
		sanitized = sanitized + "x"
	}

	// Truncate to 63 characters
	if len(sanitized) > 63 {
		sanitized = sanitized[:63]
		// Ensure it still ends with alphanumeric after truncation
		if !isAlphanumeric(rune(sanitized[len(sanitized)-1])) {
			sanitized = sanitized[:62] + "x"
		}
	}

	return sanitized
}

// isAlphanumeric checks if a rune is alphanumeric
func isAlphanumeric(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

// GetJobStatus retrieves the current status of a job with enhanced error handling
// Implements enhanced job status monitoring with comprehensive error handling (Requirements 2.1, 7.2)
func (s *KubernetesBatchInvokerService) GetJobStatus(ctx context.Context, jobName string) (*JobStatus, error) {
	if jobName == "" {
		return nil, &KubernetesError{
			Type:      ErrorTypeValidation,
			Message:   "job name is required",
			Retryable: false,
			Operation: "get_job_status",
		}
	}

	var job *batchv1.Job
	var err error

	// Get job status with retry logic
	err = s.retryWithExponentialBackoff(ctx, "get_job_status", func() error {
		var getErr error
		job, getErr = s.clientset.BatchV1().Jobs(s.config.Namespace).Get(ctx, jobName, metav1.GetOptions{})
		return getErr
	})

	if err != nil {
		kubernetesErr := s.analyzeKubernetesError(err, jobName, s.config.Namespace, "get_job_status")
		s.logger.Error("Failed to get job status",
			zap.String("job_name", jobName),
			zap.String("error_type", string(kubernetesErr.Type)),
			zap.Error(err))
		return nil, kubernetesErr
	}

	// Analyze job status with enhanced error detection
	jobStatus := s.analyzeJobStatusWithEnhancedErrorHandling(job)

	s.logger.Debug("Retrieved job status",
		zap.String("job_name", jobName),
		zap.String("status", jobStatus.Status),
		zap.String("message", jobStatus.Message))

	return jobStatus, nil
}

// CleanupCompletedJobs removes completed jobs older than specified duration with enhanced error handling
// Implements comprehensive error handling for cleanup operations (Requirement 7.2)
func (s *KubernetesBatchInvokerService) CleanupCompletedJobs(ctx context.Context, olderThan time.Duration) error {
	s.logger.Info("Starting cleanup of completed jobs",
		zap.Duration("older_than", olderThan),
		zap.String("namespace", s.config.Namespace))

	var jobs *batchv1.JobList
	var err error

	// List jobs with retry logic
	err = s.retryWithExponentialBackoff(ctx, "list_jobs_for_cleanup", func() error {
		var listErr error
		jobs, listErr = s.clientset.BatchV1().Jobs(s.config.Namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app=portfolio-accounting-cli",
		})
		return listErr
	})

	if err != nil {
		kubernetesErr := s.analyzeKubernetesError(err, "", s.config.Namespace, "cleanup_list_jobs")
		s.logger.Error("Failed to list jobs for cleanup",
			zap.String("namespace", s.config.Namespace),
			zap.Error(err))
		return kubernetesErr
	}

	cutoffTime := time.Now().Add(-olderThan)
	deletedCount := 0
	errorCount := 0

	for _, job := range jobs.Items {
		// Check if job is completed and old enough
		if !s.isJobCompletedAndOld(&job, cutoffTime) {
			continue
		}

		// Delete job with retry logic
		err = s.retryWithExponentialBackoff(ctx, "delete_job", func() error {
			return s.clientset.BatchV1().Jobs(s.config.Namespace).Delete(ctx, job.Name, metav1.DeleteOptions{})
		})

		if err != nil {
			errorCount++
			kubernetesErr := s.analyzeKubernetesError(err, job.Name, s.config.Namespace, "cleanup_delete_job")
			s.logger.Error("Failed to delete completed job",
				zap.String("job_name", job.Name),
				zap.String("error_type", string(kubernetesErr.Type)),
				zap.Error(err))

			// Continue with other jobs even if one fails
			continue
		}

		deletedCount++
		s.logger.Debug("Deleted completed job",
			zap.String("job_name", job.Name),
			zap.Time("completion_time", job.Status.CompletionTime.Time))
	}

	s.logger.Info("Job cleanup completed",
		zap.Int("deleted_count", deletedCount),
		zap.Int("error_count", errorCount),
		zap.Duration("older_than", olderThan))

	if errorCount > 0 {
		return &KubernetesError{
			Type:      ErrorTypeTransient,
			Message:   fmt.Sprintf("cleanup completed with %d errors out of %d jobs", errorCount, deletedCount+errorCount),
			Retryable: true,
			Operation: "cleanup_jobs",
			Suggestions: []string{
				"Check RBAC permissions for job deletion",
				"Verify jobs are not protected by finalizers",
				"Review error logs for specific failure reasons",
			},
		}
	}

	return nil
}

// shouldCleanupJob determines if a job should be cleaned up based on completion status and age
func (s *KubernetesBatchInvokerService) shouldCleanupJob(job *batchv1.Job, cutoffTime time.Time) bool {
	// Only cleanup completed jobs (succeeded or failed)
	isCompleted := false
	var completionTime *time.Time

	// Check if job has completion time
	if job.Status.CompletionTime != nil {
		completionTime = &job.Status.CompletionTime.Time
		isCompleted = true
	} else {
		// Check conditions for completion
		for _, condition := range job.Status.Conditions {
			if (condition.Type == batchv1.JobComplete || condition.Type == batchv1.JobFailed) &&
				condition.Status == corev1.ConditionTrue {
				isCompleted = true
				completionTime = &condition.LastTransitionTime.Time
				break
			}
		}
	}

	// Only cleanup if job is completed and older than cutoff time
	if isCompleted && completionTime != nil && completionTime.Before(cutoffTime) {
		return true
	}

	return false
}

// generateJobName creates a unique job name with timestamp
// Requirement 1.3: Add job naming with timestamps and proper labeling
func (s *KubernetesBatchInvokerService) generateJobName(prefix string) string {
	timestamp := time.Now().UTC()

	// Use both Unix timestamp and formatted time for better uniqueness and readability
	unixTime := timestamp.Unix()
	formattedTime := timestamp.Format("20060102-150405")

	// Create job name: prefix-YYYYMMDD-HHMMSS-unixtime
	jobName := fmt.Sprintf("%s-%s-%d", prefix, formattedTime, unixTime)

	// Ensure the job name is valid for Kubernetes (DNS-1123 subdomain)
	// Must be lowercase, max 63 chars, start/end with alphanumeric
	jobName = strings.ToLower(jobName)
	if len(jobName) > 63 {
		// Truncate but keep the timestamp for uniqueness
		maxPrefixLen := 63 - len(fmt.Sprintf("-%s-%d", formattedTime, unixTime))
		if maxPrefixLen > 0 {
			jobName = fmt.Sprintf("%s-%s-%d", prefix[:maxPrefixLen], formattedTime, unixTime)
		} else {
			// Fallback to just timestamp if prefix is too long
			jobName = fmt.Sprintf("%s-%d", formattedTime, unixTime)
		}
		jobName = strings.ToLower(jobName)
	}

	return jobName
}

// JobTemplate represents a reusable job template structure
// Requirement 1.3: Create job template structure and configuration
type JobTemplate struct {
	APIVersion string          `yaml:"apiVersion"`
	Kind       string          `yaml:"kind"`
	Metadata   JobMetadata     `yaml:"metadata"`
	Spec       JobTemplateSpec `yaml:"spec"`
}

type JobMetadata struct {
	Name        string            `yaml:"name,omitempty"`
	Namespace   string            `yaml:"namespace,omitempty"`
	Labels      map[string]string `yaml:"labels"`
	Annotations map[string]string `yaml:"annotations"`
}

type JobTemplateSpec struct {
	TTLSecondsAfterFinished int32           `yaml:"ttlSecondsAfterFinished"`
	BackoffLimit            int32           `yaml:"backoffLimit"`
	ActiveDeadlineSeconds   int64           `yaml:"activeDeadlineSeconds"`
	Completions             int32           `yaml:"completions"`
	Parallelism             int32           `yaml:"parallelism"`
	Template                PodTemplateSpec `yaml:"template"`
}

type PodTemplateSpec struct {
	Metadata PodMetadata `yaml:"metadata"`
	Spec     PodSpec     `yaml:"spec"`
}

type PodMetadata struct {
	Labels      map[string]string `yaml:"labels"`
	Annotations map[string]string `yaml:"annotations,omitempty"`
}

type PodSpec struct {
	RestartPolicy      string              `yaml:"restartPolicy"`
	ServiceAccountName string              `yaml:"serviceAccountName,omitempty"`
	Containers         []ContainerSpec     `yaml:"containers"`
	Volumes            []Volume            `yaml:"volumes"`
	SecurityContext    *PodSecurityContext `yaml:"securityContext,omitempty"`
	NodeSelector       map[string]string   `yaml:"nodeSelector,omitempty"`
}

type ContainerSpec struct {
	Name            string               `yaml:"name"`
	Image           string               `yaml:"image,omitempty"`
	Command         []string             `yaml:"command,omitempty"`
	Args            []string             `yaml:"args,omitempty"`
	VolumeMounts    []VolumeMount        `yaml:"volumeMounts,omitempty"`
	Env             []EnvVar             `yaml:"env,omitempty"`
	Resources       ResourceRequirements `yaml:"resources,omitempty"`
	SecurityContext SecurityContext      `yaml:"securityContext,omitempty"`
}

type VolumeMount struct {
	Name      string `yaml:"name"`
	MountPath string `yaml:"mountPath"`
	ReadOnly  bool   `yaml:"readOnly,omitempty"`
}

type EnvVar struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

type ResourceRequirements struct {
	Requests ResourceList `yaml:"requests,omitempty"`
	Limits   ResourceList `yaml:"limits,omitempty"`
}

type ResourceList struct {
	Memory string `yaml:"memory,omitempty"`
	CPU    string `yaml:"cpu,omitempty"`
}

type SecurityContext struct {
	RunAsNonRoot             bool  `yaml:"runAsNonRoot,omitempty"`
	RunAsUser                int64 `yaml:"runAsUser,omitempty"`
	AllowPrivilegeEscalation bool  `yaml:"allowPrivilegeEscalation,omitempty"`
	ReadOnlyRootFilesystem   bool  `yaml:"readOnlyRootFilesystem,omitempty"`
}

type Volume struct {
	Name                  string                 `yaml:"name"`
	PersistentVolumeClaim *PVCVolumeSource       `yaml:"persistentVolumeClaim,omitempty"`
	ConfigMap             *ConfigMapVolumeSource `yaml:"configMap,omitempty"`
}

type PVCVolumeSource struct {
	ClaimName string `yaml:"claimName"`
}

type ConfigMapVolumeSource struct {
	Name        string `yaml:"name"`
	DefaultMode int32  `yaml:"defaultMode,omitempty"`
}

type PodSecurityContext struct {
	FSGroup int64 `yaml:"fsGroup,omitempty"`
}

// createJobTemplate creates a reusable job template structure
// Requirement 1.3: Create job template structure and configuration
func (s *KubernetesBatchInvokerService) createJobTemplate() *JobTemplate {
	return &JobTemplate{
		APIVersion: "batch/v1",
		Kind:       "Job",
		Metadata: JobMetadata{
			Labels: map[string]string{
				"app":                   "portfolio-accounting-cli",
				"component":             "batch-processor",
				"managed-by":            "globeco-allocation-service",
				"globeco.io/service":    "allocation-service",
				"globeco.io/job-type":   "portfolio-accounting",
				"globeco.io/created-by": "kubernetes-batch-invoker",
			},
			Annotations: map[string]string{
				"globeco.io/created-at": time.Now().UTC().Format(time.RFC3339),
			},
		},
		Spec: JobTemplateSpec{
			TTLSecondsAfterFinished: 3600, // Clean up after 1 hour
			BackoffLimit:            2,    // Default retry limit
			ActiveDeadlineSeconds:   1800, // 30 minute default timeout
			Completions:             1,    // Ensure exactly one successful completion
			Parallelism:             1,    // Run only one pod at a time
			Template: PodTemplateSpec{
				Metadata: PodMetadata{
					Labels: map[string]string{
						"app":       "portfolio-accounting-cli",
						"component": "batch-processor",
					},
				},
				Spec: PodSpec{
					RestartPolicy: "Never",
					Containers: []ContainerSpec{
						{
							Name:    "portfolio-cli",
							Command: []string{"/usr/local/bin/cli"},
							VolumeMounts: []VolumeMount{
								{
									Name:      "nfs-storage",
									MountPath: "/data",
									ReadOnly:  false,
								},
								{
									Name:      "cli-config",
									MountPath: "/etc/config",
									ReadOnly:  true,
								},
							},
							Env: []EnvVar{
								{
									Name:  "GLOBECO_PA_SERVER_HOST",
									Value: "globeco-allocation-service",
								},
								{
									Name:  "GLOBECO_PA_SERVER_PORT",
									Value: "8089",
								},
							},
							Resources: ResourceRequirements{
								Requests: ResourceList{
									Memory: "512Mi",
									CPU:    "250m",
								},
								Limits: ResourceList{
									Memory: "1Gi",
									CPU:    "500m",
								},
							},
							SecurityContext: SecurityContext{
								RunAsNonRoot:             true,
								RunAsUser:                1000,
								AllowPrivilegeEscalation: false,
								ReadOnlyRootFilesystem:   true,
							},
						},
					},
					Volumes: []Volume{
						{
							Name: "nfs-storage",
							PersistentVolumeClaim: &PVCVolumeSource{
								ClaimName: "", // Will be set dynamically
							},
						},
						{
							Name: "cli-config",
							ConfigMap: &ConfigMapVolumeSource{
								Name:        "portfolio-cli-config",
								DefaultMode: 0444,
							},
						},
					},
					SecurityContext: &PodSecurityContext{
						FSGroup: 1000,
					},
					NodeSelector: map[string]string{
						"kubernetes.io/os": "linux",
					},
				},
			},
		},
	}
}

// applyJobTemplateConfig applies configuration to a job template
// Requirement 1.3: Dynamic job manifest generation with proper volume mounts
func (s *KubernetesBatchInvokerService) applyJobTemplateConfig(template *JobTemplate, config *BatchJobConfig) error {
	if template == nil || config == nil {
		return fmt.Errorf("template and config are required")
	}

	// Apply job metadata
	template.Metadata.Name = config.JobName
	template.Metadata.Namespace = config.Namespace
	template.Metadata.Labels["job-name"] = config.JobName
	template.Metadata.Labels["globeco.io/filename"] = sanitizeLabelValue(config.Filename)
	template.Metadata.Annotations["globeco.io/filename"] = config.Filename
	template.Metadata.Annotations["globeco.io/output-dir"] = config.OutputDir

	// Apply job spec configuration
	template.Spec.BackoffLimit = config.RetryLimit
	template.Spec.ActiveDeadlineSeconds = int64(config.Timeout.Seconds())

	// Apply pod template configuration
	template.Spec.Template.Metadata.Labels["job-name"] = config.JobName
	template.Spec.Template.Metadata.Annotations = map[string]string{
		"globeco.io/created-at": time.Now().UTC().Format(time.RFC3339),
		"globeco.io/filename":   config.Filename,
		"globeco.io/output-dir": config.OutputDir,
	}

	// Apply pod spec configuration
	template.Spec.Template.Spec.ServiceAccountName = config.ServiceAccount

	// Configure container
	if len(template.Spec.Template.Spec.Containers) > 0 {
		container := &template.Spec.Template.Spec.Containers[0]
		container.Image = config.Image
		container.Args = []string{
			"process",
			"--file", fmt.Sprintf("/data/%s", config.Filename),
			"--output-dir", config.OutputDir,
			"--config", "/etc/config/config.yaml",
		}

		// Add job-specific environment variables
		container.Env = append(container.Env, []EnvVar{
			{
				Name:  "JOB_NAME",
				Value: config.JobName,
			},
			{
				Name:  "INPUT_FILENAME",
				Value: config.Filename,
			},
		}...)
	}

	// Configure NFS volume
	for i := range template.Spec.Template.Spec.Volumes {
		volume := &template.Spec.Template.Spec.Volumes[i]
		if volume.Name == "nfs-storage" && volume.PersistentVolumeClaim != nil {
			volume.PersistentVolumeClaim.ClaimName = config.NFSVolumeClaim
		}
	}

	return nil
}

// calculateRetryDelay calculates the delay for the next retry attempt using exponential backoff with jitter
// Implements exponential backoff for transient failures (Requirement 7.1)
func (s *KubernetesBatchInvokerService) calculateRetryDelay(attempt int) time.Duration {
	// Calculate exponential backoff: baseDelay * (backoffMultiplier ^ attempt)
	delay := float64(s.retryConfig.BaseDelay) * math.Pow(s.retryConfig.BackoffMultiplier, float64(attempt))

	// Cap the delay at maxDelay
	if delay > float64(s.retryConfig.MaxDelay) {
		delay = float64(s.retryConfig.MaxDelay)
	}

	// Add jitter to prevent thundering herd problem
	if s.retryConfig.JitterEnabled {
		// Add random jitter up to 25% of the delay
		jitter := delay * 0.25 * rand.Float64()
		delay += jitter
	}

	finalDelay := time.Duration(delay)

	s.logger.Debug("Calculated retry delay",
		zap.Int("attempt", attempt),
		zap.Duration("base_delay", s.retryConfig.BaseDelay),
		zap.Float64("multiplier", s.retryConfig.BackoffMultiplier),
		zap.Duration("max_delay", s.retryConfig.MaxDelay),
		zap.Duration("calculated_delay", finalDelay),
		zap.Bool("jitter_enabled", s.retryConfig.JitterEnabled))

	return finalDelay
}

// analyzeKubernetesError analyzes Kubernetes API errors and categorizes them for retry logic
// Implements comprehensive error handling and categorization (Requirements 7.2, 7.3)
func (s *KubernetesBatchInvokerService) analyzeKubernetesError(err error, jobName, namespace, operation string) *KubernetesError {
	if err == nil {
		return nil
	}

	kubernetesErr := &KubernetesError{
		OriginalErr: err,
		JobName:     jobName,
		Namespace:   namespace,
		Operation:   operation,
		Type:        ErrorTypeUnknown,
		Retryable:   false,
		Suggestions: []string{},
	}

	errorMsg := strings.ToLower(err.Error())

	// Analyze Kubernetes API errors
	if statusErr, ok := err.(*errors.StatusError); ok {
		status := statusErr.ErrStatus

		switch status.Code {
		case 400: // Bad Request
			kubernetesErr.Type = ErrorTypeValidation
			kubernetesErr.Retryable = false
			kubernetesErr.Message = fmt.Sprintf("validation error: %s", status.Message)
			kubernetesErr.Suggestions = []string{
				"Check job manifest for syntax errors",
				"Verify all required fields are present",
				"Validate resource requests and limits",
				"Check container image name and tag",
			}

		case 401: // Unauthorized
			kubernetesErr.Type = ErrorTypePermission
			kubernetesErr.Retryable = false
			kubernetesErr.Message = fmt.Sprintf("authentication failed: %s", status.Message)
			kubernetesErr.Suggestions = []string{
				"Check service account configuration",
				"Verify authentication credentials",
				"Ensure service account token is valid",
			}

		case 403: // Forbidden
			kubernetesErr.Type = ErrorTypePermission
			kubernetesErr.Retryable = false
			kubernetesErr.Message = fmt.Sprintf("insufficient permissions: %s", status.Message)
			kubernetesErr.Suggestions = []string{
				"Check RBAC permissions for service account",
				"Verify Role and RoleBinding configuration",
				"Ensure service account has 'create' permission for jobs",
				"Check namespace access permissions",
			}

		case 404: // Not Found
			kubernetesErr.Type = ErrorTypeValidation
			kubernetesErr.Retryable = false
			kubernetesErr.Message = fmt.Sprintf("resource not found: %s", status.Message)
			kubernetesErr.Suggestions = []string{
				"Verify namespace exists",
				"Check service account exists",
				"Verify PVC exists and is accessible",
				"Check ConfigMap references",
			}

		case 409: // Conflict
			kubernetesErr.Type = ErrorTypeValidation
			kubernetesErr.Retryable = false
			kubernetesErr.Message = fmt.Sprintf("resource conflict: %s", status.Message)
			kubernetesErr.Suggestions = []string{
				"Job with same name may already exist",
				"Use unique job names with timestamps",
				"Check for resource naming conflicts",
			}

		case 422: // Unprocessable Entity
			kubernetesErr.Type = ErrorTypeValidation
			kubernetesErr.Retryable = false
			kubernetesErr.Message = fmt.Sprintf("validation failed: %s", status.Message)
			kubernetesErr.Suggestions = []string{
				"Check job specification for validation errors",
				"Verify resource quotas and limits",
				"Check container image pull policy",
				"Validate volume mount configurations",
			}

		case 429: // Too Many Requests
			kubernetesErr.Type = ErrorTypeResourceLimit
			kubernetesErr.Retryable = true
			kubernetesErr.Message = fmt.Sprintf("rate limited: %s", status.Message)
			kubernetesErr.Suggestions = []string{
				"Reduce request frequency",
				"Implement request throttling",
				"Check API server rate limits",
			}

		case 500: // Internal Server Error
			kubernetesErr.Type = ErrorTypeTransient
			kubernetesErr.Retryable = true
			kubernetesErr.Message = fmt.Sprintf("server error: %s", status.Message)
			kubernetesErr.Suggestions = []string{
				"Check Kubernetes API server health",
				"Verify cluster stability",
				"Check etcd health",
			}

		case 502, 503, 504: // Bad Gateway, Service Unavailable, Gateway Timeout
			kubernetesErr.Type = ErrorTypeConnectivity
			kubernetesErr.Retryable = true
			kubernetesErr.Message = fmt.Sprintf("connectivity issue: %s", status.Message)
			kubernetesErr.Suggestions = []string{
				"Check network connectivity to API server",
				"Verify API server availability",
				"Check load balancer health",
			}

		default:
			kubernetesErr.Type = ErrorTypeUnknown
			kubernetesErr.Retryable = true // Default to retryable for unknown status codes
			kubernetesErr.Message = fmt.Sprintf("unknown status code %d: %s", status.Code, status.Message)
		}
	} else {
		// Analyze error message for common patterns
		switch {
		case s.containsAnyString(errorMsg, s.retryConfig.NonRetryableErrors):
			kubernetesErr.Type = ErrorTypeValidation
			kubernetesErr.Retryable = false
			kubernetesErr.Message = fmt.Sprintf("non-retryable error: %s", err.Error())

		case s.containsAnyString(errorMsg, s.retryConfig.RetryableErrors):
			kubernetesErr.Type = ErrorTypeTransient
			kubernetesErr.Retryable = true
			kubernetesErr.Message = fmt.Sprintf("transient error: %s", err.Error())

		case strings.Contains(errorMsg, "connection refused") || strings.Contains(errorMsg, "no route to host"):
			kubernetesErr.Type = ErrorTypeConnectivity
			kubernetesErr.Retryable = true
			kubernetesErr.Message = fmt.Sprintf("connectivity error: %s", err.Error())
			kubernetesErr.Suggestions = []string{
				"Check network connectivity",
				"Verify Kubernetes API server endpoint",
				"Check firewall rules",
			}

		case strings.Contains(errorMsg, "timeout") || strings.Contains(errorMsg, "deadline exceeded"):
			kubernetesErr.Type = ErrorTypeTransient
			kubernetesErr.Retryable = true
			kubernetesErr.Message = fmt.Sprintf("timeout error: %s", err.Error())
			kubernetesErr.Suggestions = []string{
				"Increase timeout values",
				"Check API server performance",
				"Verify network latency",
			}

		case strings.Contains(errorMsg, "quota") || strings.Contains(errorMsg, "limit"):
			kubernetesErr.Type = ErrorTypeResourceLimit
			kubernetesErr.Retryable = false
			kubernetesErr.Message = fmt.Sprintf("resource limit error: %s", err.Error())
			kubernetesErr.Suggestions = []string{
				"Check resource quotas in namespace",
				"Verify CPU and memory limits",
				"Check storage quotas",
				"Review resource requests in job spec",
			}

		default:
			kubernetesErr.Type = ErrorTypeUnknown
			kubernetesErr.Retryable = true // Default to retryable for safety
			kubernetesErr.Message = fmt.Sprintf("unknown error: %s", err.Error())
			kubernetesErr.Suggestions = []string{
				"Check Kubernetes cluster health",
				"Review error logs for more details",
				"Verify service configuration",
			}
		}
	}

	// Log detailed error analysis
	s.logger.Debug("Kubernetes error analysis completed",
		zap.String("job_name", jobName),
		zap.String("operation", operation),
		zap.String("error_type", string(kubernetesErr.Type)),
		zap.Bool("retryable", kubernetesErr.Retryable),
		zap.String("message", kubernetesErr.Message),
		zap.Strings("suggestions", kubernetesErr.Suggestions))

	return kubernetesErr
}

// containsAnyString checks if the text contains any of the provided strings
func (s *KubernetesBatchInvokerService) containsAnyString(text string, patterns []string) bool {
	for _, pattern := range patterns {
		if strings.Contains(text, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

// retryWithExponentialBackoff executes a function with exponential backoff retry logic
// Generic retry mechanism for various Kubernetes operations (Requirement 7.1)
func (s *KubernetesBatchInvokerService) retryWithExponentialBackoff(ctx context.Context, operation string, fn func() error) error {
	var lastErr error

	for attempt := 0; attempt <= s.retryConfig.MaxRetries; attempt++ {
		err := fn()
		if err == nil {
			if attempt > 0 {
				s.logger.Info("Operation succeeded after retry",
					zap.String("operation", operation),
					zap.Int("attempt", attempt+1))
			}
			return nil
		}

		lastErr = err

		// Analyze error to determine if retryable
		kubernetesErr := s.analyzeKubernetesError(err, "", "", operation)

		if !kubernetesErr.Retryable {
			s.logger.Error("Operation failed with non-retryable error",
				zap.String("operation", operation),
				zap.String("error_type", string(kubernetesErr.Type)),
				zap.Error(err))
			return kubernetesErr
		}

		// If this was the last attempt, don't wait
		if attempt >= s.retryConfig.MaxRetries {
			break
		}

		// Calculate delay and wait
		delay := s.calculateRetryDelay(attempt)

		s.logger.Warn("Operation failed, retrying",
			zap.String("operation", operation),
			zap.Int("attempt", attempt+1),
			zap.Duration("delay", delay),
			zap.Error(err))

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			// Continue to next attempt
		}
	}

	return fmt.Errorf("operation failed after %d attempts: %w", s.retryConfig.MaxRetries+1, lastErr)
}

// enhanceValidateKubernetesAccess enhances the existing validation with retry logic and comprehensive error handling
// Implements enhanced RBAC permission validation and connectivity testing (Requirements 7.2, 7.3)
func (s *KubernetesBatchInvokerService) enhanceValidateKubernetesAccess() error {
	s.logger.Info("Starting enhanced Kubernetes access validation with retry logic")

	// Test basic connectivity with retry
	err := s.retryWithExponentialBackoff(context.Background(), "list_jobs", func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err := s.clientset.BatchV1().Jobs(s.config.Namespace).List(ctx, metav1.ListOptions{Limit: 1})
		return err
	})
	if err != nil {
		return &KubernetesError{
			Type:        ErrorTypeConnectivity,
			OriginalErr: err,
			Message:     "failed to validate basic Kubernetes connectivity",
			Retryable:   false,
			Namespace:   s.config.Namespace,
			Operation:   "connectivity_test",
			Suggestions: []string{
				"Check Kubernetes API server endpoint",
				"Verify network connectivity",
				"Check service account token",
				"Verify namespace exists",
			},
		}
	}

	// Test pod access with retry
	err = s.retryWithExponentialBackoff(context.Background(), "list_pods", func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err := s.clientset.CoreV1().Pods(s.config.Namespace).List(ctx, metav1.ListOptions{Limit: 1})
		return err
	})
	if err != nil {
		return &KubernetesError{
			Type:        ErrorTypePermission,
			OriginalErr: err,
			Message:     "failed to validate pod access permissions",
			Retryable:   false,
			Namespace:   s.config.Namespace,
			Operation:   "pod_access_test",
			Suggestions: []string{
				"Check RBAC permissions for pods",
				"Verify Role includes 'get', 'list' permissions for pods",
				"Check RoleBinding configuration",
			},
		}
	}

	// Validate service account with retry
	if s.config.ServiceAccountName != "" {
		err = s.retryWithExponentialBackoff(context.Background(), "get_service_account", func() error {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			_, err := s.clientset.CoreV1().ServiceAccounts(s.config.Namespace).Get(ctx, s.config.ServiceAccountName, metav1.GetOptions{})
			return err
		})
		if err != nil {
			return &KubernetesError{
				Type:        ErrorTypeValidation,
				OriginalErr: err,
				Message:     fmt.Sprintf("service account '%s' validation failed", s.config.ServiceAccountName),
				Retryable:   false,
				Namespace:   s.config.Namespace,
				Operation:   "service_account_validation",
				Suggestions: []string{
					fmt.Sprintf("Create service account '%s' in namespace '%s'", s.config.ServiceAccountName, s.config.Namespace),
					"Check service account name in configuration",
					"Verify namespace is correct",
				},
			}
		}
	}

	// Validate PVC with retry
	if s.config.NFSPVCName != "" {
		err = s.retryWithExponentialBackoff(context.Background(), "get_pvc", func() error {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			_, err := s.clientset.CoreV1().PersistentVolumeClaims(s.config.Namespace).Get(ctx, s.config.NFSPVCName, metav1.GetOptions{})
			return err
		})
		if err != nil {
			return &KubernetesError{
				Type:        ErrorTypeValidation,
				OriginalErr: err,
				Message:     fmt.Sprintf("PVC '%s' validation failed", s.config.NFSPVCName),
				Retryable:   false,
				Namespace:   s.config.Namespace,
				Operation:   "pvc_validation",
				Suggestions: []string{
					fmt.Sprintf("Create PVC '%s' in namespace '%s'", s.config.NFSPVCName, s.config.Namespace),
					"Check PVC name in configuration",
					"Verify PVC is bound to a persistent volume",
					"Check storage class configuration",
				},
			}
		}
	}

	// Test job creation permissions (dry-run)
	err = s.testJobCreationPermissions()
	if err != nil {
		return err
	}

	s.logger.Info("Enhanced Kubernetes access validation completed successfully",
		zap.String("namespace", s.config.Namespace),
		zap.String("service_account", s.config.ServiceAccountName),
		zap.String("pvc_name", s.config.NFSPVCName))

	return nil
}

// testJobCreationPermissions tests job creation permissions using a dry-run
// Implements RBAC permission validation for job creation (Requirement 7.2)
func (s *KubernetesBatchInvokerService) testJobCreationPermissions() error {
	s.logger.Debug("Testing job creation permissions with dry-run")

	// Create a minimal test job for dry-run validation
	testJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-permissions-job",
			Namespace: s.config.Namespace,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:    "test",
							Image:   "busybox:latest",
							Command: []string{"echo", "test"},
						},
					},
				},
			},
		},
	}

	// Perform dry-run to test permissions without actually creating the job
	err := s.retryWithExponentialBackoff(context.Background(), "test_job_creation", func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err := s.clientset.BatchV1().Jobs(s.config.Namespace).Create(ctx, testJob, metav1.CreateOptions{
			DryRun: []string{metav1.DryRunAll},
		})
		return err
	})

	if err != nil {
		kubernetesErr := s.analyzeKubernetesError(err, "test-permissions-job", s.config.Namespace, "job_creation_test")

		// Enhance error message for permission issues
		if kubernetesErr.Type == ErrorTypePermission {
			kubernetesErr.Suggestions = append(kubernetesErr.Suggestions,
				"Ensure service account has 'create' permission for jobs",
				"Check if Role includes 'jobs' resource with 'create' verb",
				"Verify RoleBinding associates service account with correct Role")
		}

		return kubernetesErr
	}

	s.logger.Debug("Job creation permissions validated successfully")
	return nil
}

// analyzeJobStatusWithEnhancedErrorHandling provides enhanced job status analysis with detailed error information
// Implements detailed error messages for debugging failed jobs (Requirement 7.2)
func (s *KubernetesBatchInvokerService) analyzeJobStatusWithEnhancedErrorHandling(job *batchv1.Job) *JobStatus {
	status := &JobStatus{
		JobName:   job.Name,
		Status:    "unknown",
		StartTime: time.Now(),
		Message:   "Job status unknown",
	}

	// Set start time if available
	if job.Status.StartTime != nil {
		status.StartTime = job.Status.StartTime.Time
	}

	// Enhanced condition analysis with detailed error information
	for _, condition := range job.Status.Conditions {
		switch condition.Type {
		case batchv1.JobComplete:
			if condition.Status == corev1.ConditionTrue {
				status.Status = "succeeded"
				status.Message = s.buildSuccessMessage(job, condition)
				if condition.LastTransitionTime.Time.After(status.StartTime) {
					endTime := condition.LastTransitionTime.Time
					status.EndTime = &endTime
				}
				return status
			}

		case batchv1.JobFailed:
			if condition.Status == corev1.ConditionTrue {
				status.Status = "failed"
				status.Message = s.buildFailureMessage(job, condition)
				if condition.LastTransitionTime.Time.After(status.StartTime) {
					endTime := condition.LastTransitionTime.Time
					status.EndTime = &endTime
				}
				return status
			}
		}
	}

	// Enhanced status analysis based on pod counts and conditions
	if job.Status.Active > 0 {
		status.Status = "running"
		status.Message = s.buildRunningMessage(job)
	} else if job.Status.Succeeded > 0 {
		status.Status = "succeeded"
		status.Message = s.buildSuccessMessage(job, batchv1.JobCondition{})
	} else if job.Status.Failed > 0 {
		status.Status = "failed"
		status.Message = s.buildFailureMessage(job, batchv1.JobCondition{})
	} else {
		status.Status = "pending"
		status.Message = s.buildPendingMessage(job)
	}

	return status
}

// buildSuccessMessage creates detailed success message with job statistics
func (s *KubernetesBatchInvokerService) buildSuccessMessage(job *batchv1.Job, condition batchv1.JobCondition) string {
	var duration string
	if job.Status.StartTime != nil && job.Status.CompletionTime != nil {
		duration = job.Status.CompletionTime.Sub(job.Status.StartTime.Time).String()
	}

	if condition.Message != "" {
		return fmt.Sprintf("Job completed successfully in %s: %s (succeeded: %d, failed: %d)",
			duration, condition.Message, job.Status.Succeeded, job.Status.Failed)
	}

	return fmt.Sprintf("Job completed successfully in %s (succeeded: %d pods)",
		duration, job.Status.Succeeded)
}

// buildFailureMessage creates detailed failure message with debugging information
func (s *KubernetesBatchInvokerService) buildFailureMessage(job *batchv1.Job, condition batchv1.JobCondition) string {
	var duration string
	if job.Status.StartTime != nil {
		duration = time.Since(job.Status.StartTime.Time).String()
	}

	failureReasons := []string{}

	// Add condition information if available
	if condition.Reason != "" {
		failureReasons = append(failureReasons, fmt.Sprintf("reason: %s", condition.Reason))
	}
	if condition.Message != "" {
		failureReasons = append(failureReasons, fmt.Sprintf("message: %s", condition.Message))
	}

	// Add pod statistics
	failureReasons = append(failureReasons, fmt.Sprintf("failed pods: %d", job.Status.Failed))
	if job.Status.Succeeded > 0 {
		failureReasons = append(failureReasons, fmt.Sprintf("succeeded pods: %d", job.Status.Succeeded))
	}

	// Check for specific failure patterns
	if job.Spec.BackoffLimit != nil && job.Status.Failed >= *job.Spec.BackoffLimit {
		failureReasons = append(failureReasons, "backoff limit reached")
	}

	if job.Spec.ActiveDeadlineSeconds != nil && duration != "" {
		failureReasons = append(failureReasons, fmt.Sprintf("runtime: %s", duration))
	}

	return fmt.Sprintf("Job failed after %s - %s", duration, strings.Join(failureReasons, ", "))
}

// buildRunningMessage creates detailed running message with progress information
func (s *KubernetesBatchInvokerService) buildRunningMessage(job *batchv1.Job) string {
	var duration string
	if job.Status.StartTime != nil {
		duration = time.Since(job.Status.StartTime.Time).String()
	}

	details := []string{
		fmt.Sprintf("active pods: %d", job.Status.Active),
	}

	if job.Status.Succeeded > 0 {
		details = append(details, fmt.Sprintf("succeeded: %d", job.Status.Succeeded))
	}
	if job.Status.Failed > 0 {
		details = append(details, fmt.Sprintf("failed: %d", job.Status.Failed))
	}

	return fmt.Sprintf("Job running for %s (%s)", duration, strings.Join(details, ", "))
}

// buildPendingMessage creates detailed pending message with diagnostic information
func (s *KubernetesBatchInvokerService) buildPendingMessage(job *batchv1.Job) string {
	var duration string
	if job.Status.StartTime != nil {
		duration = time.Since(job.Status.StartTime.Time).String()
	} else {
		duration = time.Since(job.CreationTimestamp.Time).String()
	}

	reasons := []string{"no active, succeeded, or failed pods"}

	// Check for common pending reasons
	if job.Status.StartTime == nil {
		reasons = append(reasons, "job not started")
	}

	// Check conditions for more specific reasons
	for _, condition := range job.Status.Conditions {
		if condition.Status == corev1.ConditionFalse && condition.Reason != "" {
			reasons = append(reasons, fmt.Sprintf("%s: %s", condition.Reason, condition.Message))
		}
	}

	return fmt.Sprintf("Job pending for %s - %s", duration, strings.Join(reasons, ", "))
}

// isJobCompletedAndOld checks if a job is completed and older than the cutoff time
func (s *KubernetesBatchInvokerService) isJobCompletedAndOld(job *batchv1.Job, cutoffTime time.Time) bool {
	// Check if job is completed
	isCompleted := false
	var completionTime time.Time

	for _, condition := range job.Status.Conditions {
		if condition.Type == batchv1.JobComplete && condition.Status == corev1.ConditionTrue {
			isCompleted = true
			completionTime = condition.LastTransitionTime.Time
			break
		}
		if condition.Type == batchv1.JobFailed && condition.Status == corev1.ConditionTrue {
			isCompleted = true
			completionTime = condition.LastTransitionTime.Time
			break
		}
	}

	// If no condition found, check status completion time
	if !isCompleted && job.Status.CompletionTime != nil {
		isCompleted = true
		completionTime = job.Status.CompletionTime.Time
	}

	// Job must be completed and older than cutoff time
	return isCompleted && completionTime.Before(cutoffTime)
}
