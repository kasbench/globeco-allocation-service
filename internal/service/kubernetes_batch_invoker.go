package service

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
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
}

// KubernetesBatchInvokerService implements the KubernetesBatchInvoker interface
type KubernetesBatchInvokerService struct {
	clientset *kubernetes.Clientset
	config    *config.KubernetesBatchConfig
	logger    *zap.Logger
	timeout   time.Duration
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

	service := &KubernetesBatchInvokerService{
		clientset: clientset,
		config:    cfg,
		logger:    logger,
		timeout:   timeout,
	}

	logger.Info("Kubernetes batch invoker service initialized",
		zap.String("namespace", cfg.Namespace),
		zap.String("cli_image", cfg.CLIImage),
		zap.Duration("timeout", timeout),
		zap.Int("retry_limit", cfg.JobRetryLimit))

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

	// Generate unique job name with timestamp
	timestamp := time.Now().Unix()
	jobName := fmt.Sprintf("portfolio-cli-%d", timestamp)

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

	// Submit job to Kubernetes API
	createdJob, err := s.clientset.BatchV1().Jobs(s.config.Namespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		s.logger.Error("Failed to submit batch job to Kubernetes",
			zap.String("job_name", jobName),
			zap.Error(err))
		return fmt.Errorf("failed to submit batch job: %w", err)
	}

	s.logger.Info("Batch job submitted successfully",
		zap.String("job_name", createdJob.Name),
		zap.String("namespace", createdJob.Namespace))

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

	// Use the same logic as InvokePortfolioAccountingCLI but with retry prefix
	timestamp := time.Now().Unix()
	jobName := fmt.Sprintf("portfolio-cli-retry-%d", timestamp)

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

	createdJob, err := s.clientset.BatchV1().Jobs(s.config.Namespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		s.logger.Error("Failed to submit retry batch job to Kubernetes",
			zap.String("job_name", jobName),
			zap.Error(err))
		return fmt.Errorf("failed to submit retry batch job: %w", err)
	}

	s.logger.Info("Retry batch job submitted successfully",
		zap.String("job_name", createdJob.Name))

	// Monitor job completion
	if err := s.monitorJobCompletion(ctx, jobName); err != nil {
		return fmt.Errorf("retry batch job failed: %w", err)
	}

	s.logger.Info("Retry batch job completed successfully",
		zap.String("job_name", jobName),
		zap.String("filename", filename))

	return nil
}

// ValidateKubernetesAccess validates RBAC permissions and connectivity
func (s *KubernetesBatchInvokerService) ValidateKubernetesAccess() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	s.logger.Info("Validating Kubernetes access and RBAC permissions")

	// Test basic connectivity by listing jobs in the namespace
	_, err := s.clientset.BatchV1().Jobs(s.config.Namespace).List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		s.logger.Error("Failed to validate Kubernetes access - cannot list jobs",
			zap.String("namespace", s.config.Namespace),
			zap.Error(err))
		return fmt.Errorf("kubernetes access validation failed: %w", err)
	}

	// Test pod access (needed for monitoring)
	_, err = s.clientset.CoreV1().Pods(s.config.Namespace).List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		s.logger.Error("Failed to validate pod access",
			zap.String("namespace", s.config.Namespace),
			zap.Error(err))
		return fmt.Errorf("pod access validation failed: %w", err)
	}

	// Validate service account exists
	if s.config.ServiceAccountName != "" {
		_, err = s.clientset.CoreV1().ServiceAccounts(s.config.Namespace).Get(ctx, s.config.ServiceAccountName, metav1.GetOptions{})
		if err != nil {
			s.logger.Error("Failed to validate service account",
				zap.String("service_account", s.config.ServiceAccountName),
				zap.String("namespace", s.config.Namespace),
				zap.Error(err))
			return fmt.Errorf("service account validation failed: %w", err)
		}
	}

	// Validate PVC exists
	if s.config.NFSPVCName != "" {
		_, err = s.clientset.CoreV1().PersistentVolumeClaims(s.config.Namespace).Get(ctx, s.config.NFSPVCName, metav1.GetOptions{})
		if err != nil {
			s.logger.Error("Failed to validate PVC",
				zap.String("pvc_name", s.config.NFSPVCName),
				zap.String("namespace", s.config.Namespace),
				zap.Error(err))
			return fmt.Errorf("PVC validation failed: %w", err)
		}
	}

	s.logger.Info("Kubernetes access validation successful",
		zap.String("namespace", s.config.Namespace),
		zap.String("service_account", s.config.ServiceAccountName),
		zap.String("pvc_name", s.config.NFSPVCName))

	return nil
}

// createBatchJob creates a Kubernetes Job manifest for Portfolio Accounting CLI
func (s *KubernetesBatchInvokerService) createBatchJob(config *BatchJobConfig) (*batchv1.Job, error) {
	// Create job labels
	labels := map[string]string{
		"app":        "portfolio-accounting-cli",
		"component":  "batch-processor",
		"managed-by": "globeco-allocation-service",
		"job-name":   config.JobName,
	}

	// Create container spec
	container := corev1.Container{
		Name:    "portfolio-cli",
		Image:   config.Image,
		Command: []string{"/usr/local/bin/cli"},
		Args: []string{
			"process",
			"--file", fmt.Sprintf("/data/%s", config.Filename),
			"--output-dir", "/data",
			"--config", "/etc/config/config.yaml",
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "nfs-storage",
				MountPath: "/data",
				ReadOnly:  false,
			},
			{
				Name:      "cli-config",
				MountPath: "/etc/config",
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
	}

	// Create volumes
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
				},
			},
		},
	}

	// Create pod template spec
	podSpec := corev1.PodSpec{
		RestartPolicy:      corev1.RestartPolicyNever,
		ServiceAccountName: config.ServiceAccount,
		Containers:         []corev1.Container{container},
		Volumes:            volumes,
	}

	// Create job spec
	jobSpec := batchv1.JobSpec{
		TTLSecondsAfterFinished: int32Ptr(3600), // Clean up after 1 hour
		BackoffLimit:            &config.RetryLimit,
		ActiveDeadlineSeconds:   int64Ptr(int64(config.Timeout.Seconds())),
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: labels,
			},
			Spec: podSpec,
		},
	}

	// Create the job
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.JobName,
			Namespace: config.Namespace,
			Labels:    labels,
		},
		Spec: jobSpec,
	}

	return job, nil
}

// monitorJobCompletion monitors a job until completion or timeout
func (s *KubernetesBatchInvokerService) monitorJobCompletion(ctx context.Context, jobName string) error {
	s.logger.Info("Monitoring job completion", zap.String("job_name", jobName))

	// Create context with timeout
	monitorCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-monitorCtx.Done():
			return fmt.Errorf("job monitoring timed out after %v", s.timeout)
		case <-ticker.C:
			job, err := s.clientset.BatchV1().Jobs(s.config.Namespace).Get(monitorCtx, jobName, metav1.GetOptions{})
			if err != nil {
				s.logger.Error("Failed to get job status", zap.String("job_name", jobName), zap.Error(err))
				continue
			}

			// Check job conditions
			for _, condition := range job.Status.Conditions {
				switch condition.Type {
				case batchv1.JobComplete:
					if condition.Status == corev1.ConditionTrue {
						s.logger.Info("Job completed successfully",
							zap.String("job_name", jobName),
							zap.String("message", condition.Message))
						return nil
					}
				case batchv1.JobFailed:
					if condition.Status == corev1.ConditionTrue {
						s.logger.Error("Job failed",
							zap.String("job_name", jobName),
							zap.String("reason", condition.Reason),
							zap.String("message", condition.Message))
						return fmt.Errorf("job failed: %s - %s", condition.Reason, condition.Message)
					}
				}
			}

			// Log current status
			s.logger.Debug("Job status update",
				zap.String("job_name", jobName),
				zap.Int32("active", job.Status.Active),
				zap.Int32("succeeded", job.Status.Succeeded),
				zap.Int32("failed", job.Status.Failed))
		}
	}
}

// Helper functions for creating Kubernetes pointers
func int32Ptr(i int32) *int32 {
	return &i
}

func int64Ptr(i int64) *int64 {
	return &i
}
