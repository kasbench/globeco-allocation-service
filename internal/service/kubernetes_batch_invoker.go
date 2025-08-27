package service

import (
	"context"
	"fmt"
	"strings"
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
		Args: []string{
			"process",
			"--file", fmt.Sprintf("/data/%s", config.Filename),
			"--output-dir", config.OutputDir,
			"--config", "/etc/config/config.yaml",
		},
		// Requirement 6.2: Proper volume mounts for NFS access
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "nfs-storage",
				MountPath: "/data",
				ReadOnly:  false, // CLI needs write access for output files
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
