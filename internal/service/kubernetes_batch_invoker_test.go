package service

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kasbench/globeco-allocation-service/internal/config"
)

func TestAnalyzeJobStatus(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := &config.KubernetesBatchConfig{
		Namespace:          "test",
		CLIImage:           "test-image",
		JobTimeoutSeconds:  1800,
		JobRetryLimit:      2,
		ServiceAccountName: "test-sa",
		NFSPVCName:         "test-pvc",
	}

	service := &KubernetesBatchInvokerService{
		config:  cfg,
		logger:  logger,
		timeout: 30 * time.Minute,
	}

	tests := []struct {
		name           string
		job            *batchv1.Job
		expectedStatus string
	}{
		{
			name: "job succeeded",
			job: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "test-job"},
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobComplete,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			expectedStatus: "succeeded",
		},
		{
			name: "job failed",
			job: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "test-job"},
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobFailed,
							Status: corev1.ConditionTrue,
							Reason: "BackoffLimitExceeded",
						},
					},
				},
			},
			expectedStatus: "failed",
		},
		{
			name: "job running",
			job: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "test-job"},
				Status: batchv1.JobStatus{
					Active: 1,
				},
			},
			expectedStatus: "running",
		},
		{
			name: "job pending",
			job: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "test-job"},
				Status:     batchv1.JobStatus{},
			},
			expectedStatus: "pending",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := service.analyzeJobStatus(tt.job)
			assert.Equal(t, tt.expectedStatus, status.Status)
			assert.Equal(t, tt.job.Name, status.JobName)
		})
	}
}

func TestValidateJobSubmission(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := &config.KubernetesBatchConfig{
		Namespace:          "test",
		CLIImage:           "test-image",
		JobTimeoutSeconds:  1800,
		JobRetryLimit:      2,
		ServiceAccountName: "test-sa",
		NFSPVCName:         "test-pvc",
	}

	service := &KubernetesBatchInvokerService{
		config:  cfg,
		logger:  logger,
		timeout: 30 * time.Minute,
	}

	tests := []struct {
		name      string
		job       *batchv1.Job
		expectErr bool
	}{
		{
			name:      "nil job",
			job:       nil,
			expectErr: true,
		},
		{
			name: "missing job name",
			job: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test"},
			},
			expectErr: true,
		},
		{
			name: "missing namespace",
			job: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "test-job"},
			},
			expectErr: true,
		},
		{
			name: "no containers",
			job: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "test",
				},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{},
						},
					},
				},
			},
			expectErr: true,
		},
		{
			name: "valid job",
			job: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "test",
				},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "test-container",
									Image: "test-image",
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "nfs-storage",
											MountPath: "/data",
										},
									},
								},
							},
							Volumes: []corev1.Volume{
								{
									Name: "nfs-storage",
									VolumeSource: corev1.VolumeSource{
										PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
											ClaimName: "test-pvc",
										},
									},
								},
							},
						},
					},
				},
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.validateJobSubmission(tt.job)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSanitizeLabelValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "unknown",
		},
		{
			name:     "valid string",
			input:    "test-file.csv",
			expected: "test-file.csv",
		},
		{
			name:     "string with invalid characters",
			input:    "test@file#.csv",
			expected: "test-file-.csv",
		},
		{
			name:     "string starting with invalid character",
			input:    "@test-file.csv",
			expected: "x-test-file.csv",
		},
		{
			name:     "string ending with invalid character",
			input:    "test-file.csv@",
			expected: "test-file.csv-x",
		},
		{
			name:     "very long string",
			input:    "this-is-a-very-long-filename-that-exceeds-the-kubernetes-label-limit-of-63-characters.csv",
			expected: "this-is-a-very-long-filename-that-exceeds-the-kubernetes-labelx",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeLabelValue(tt.input)
			assert.Equal(t, tt.expected, result)
			assert.LessOrEqual(t, len(result), 63, "Label value should not exceed 63 characters")
		})
	}
}

func TestGenerateJobName(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := &config.KubernetesBatchConfig{
		Namespace:          "test",
		CLIImage:           "test-image",
		JobTimeoutSeconds:  1800,
		JobRetryLimit:      2,
		ServiceAccountName: "test-sa",
		NFSPVCName:         "test-pvc",
	}

	service := &KubernetesBatchInvokerService{
		config:  cfg,
		logger:  logger,
		timeout: 30 * time.Minute,
	}

	tests := []struct {
		name   string
		prefix string
	}{
		{
			name:   "standard prefix",
			prefix: "portfolio-cli",
		},
		{
			name:   "retry prefix",
			prefix: "portfolio-cli-retry",
		},
		{
			name:   "very long prefix",
			prefix: "this-is-a-very-long-prefix-that-might-cause-issues-with-kubernetes-naming-limits",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jobName := service.generateJobName(tt.prefix)

			// Verify job name properties
			assert.NotEmpty(t, jobName)
			assert.LessOrEqual(t, len(jobName), 63, "Job name should not exceed 63 characters")
			assert.Equal(t, jobName, strings.ToLower(jobName), "Job name should be lowercase")

			// Verify uniqueness by generating multiple names with a delay to ensure different Unix timestamp
			time.Sleep(1001 * time.Millisecond) // Ensure different Unix timestamp
			jobName2 := service.generateJobName(tt.prefix)
			assert.NotEqual(t, jobName, jobName2, "Job names should be unique")
		})
	}
}

func TestShouldCleanupJob(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := &config.KubernetesBatchConfig{
		Namespace:          "test",
		CLIImage:           "test-image",
		JobTimeoutSeconds:  1800,
		JobRetryLimit:      2,
		ServiceAccountName: "test-sa",
		NFSPVCName:         "test-pvc",
	}

	service := &KubernetesBatchInvokerService{
		config:  cfg,
		logger:  logger,
		timeout: 30 * time.Minute,
	}

	now := time.Now()
	cutoffTime := now.Add(-1 * time.Hour)
	oldTime := now.Add(-2 * time.Hour)
	recentTime := now.Add(-30 * time.Minute)

	tests := []struct {
		name          string
		job           *batchv1.Job
		cutoffTime    time.Time
		shouldCleanup bool
	}{
		{
			name: "completed job older than cutoff",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					CompletionTime: &metav1.Time{Time: oldTime},
				},
			},
			cutoffTime:    cutoffTime,
			shouldCleanup: true,
		},
		{
			name: "completed job newer than cutoff",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					CompletionTime: &metav1.Time{Time: recentTime},
				},
			},
			cutoffTime:    cutoffTime,
			shouldCleanup: false,
		},
		{
			name: "running job",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Active: 1,
				},
			},
			cutoffTime:    cutoffTime,
			shouldCleanup: false,
		},
		{
			name: "failed job older than cutoff",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{
							Type:               batchv1.JobFailed,
							Status:             corev1.ConditionTrue,
							LastTransitionTime: metav1.Time{Time: oldTime},
						},
					},
				},
			},
			cutoffTime:    cutoffTime,
			shouldCleanup: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.shouldCleanupJob(tt.job, tt.cutoffTime)
			assert.Equal(t, tt.shouldCleanup, result)
		})
	}
}
