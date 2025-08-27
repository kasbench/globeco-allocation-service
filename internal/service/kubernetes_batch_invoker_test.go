package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/kasbench/globeco-allocation-service/internal/config"
)

func TestNewKubernetesBatchInvokerService(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name        string
		config      *config.KubernetesBatchConfig
		logger      *zap.Logger
		expectError bool
		errorMsg    string
	}{
		{
			name:        "nil config should return error",
			config:      nil,
			logger:      logger,
			expectError: true,
			errorMsg:    "kubernetes batch config is required",
		},
		{
			name: "nil logger should return error",
			config: &config.KubernetesBatchConfig{
				Enabled:            true,
				Namespace:          "test",
				CLIImage:           "test-image",
				JobTimeoutSeconds:  1800,
				JobRetryLimit:      2,
				ServiceAccountName: "test-sa",
				NFSPVCName:         "test-pvc",
			},
			logger:      nil,
			expectError: true,
			errorMsg:    "logger is required",
		},
		{
			name: "valid config should create service (will fail on k8s client creation outside cluster)",
			config: &config.KubernetesBatchConfig{
				Enabled:            true,
				Namespace:          "test",
				CLIImage:           "test-image",
				JobTimeoutSeconds:  1800,
				JobRetryLimit:      2,
				ServiceAccountName: "test-sa",
				NFSPVCName:         "test-pvc",
			},
			logger:      logger,
			expectError: true, // Expected to fail outside k8s cluster
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, err := NewKubernetesBatchInvokerService(tt.config, tt.logger)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, service)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, service)
				assert.Equal(t, tt.config, service.config)
				assert.Equal(t, tt.logger, service.logger)
			}
		})
	}
}

func TestBatchJobConfig(t *testing.T) {
	config := &BatchJobConfig{
		JobName:        "test-job",
		Namespace:      "test-namespace",
		Image:          "test-image",
		Filename:       "test-file.csv",
		OutputDir:      "/data",
		ServiceAccount: "test-sa",
		RetryLimit:     2,
		NFSVolumeClaim: "test-pvc",
	}

	assert.Equal(t, "test-job", config.JobName)
	assert.Equal(t, "test-namespace", config.Namespace)
	assert.Equal(t, "test-image", config.Image)
	assert.Equal(t, "test-file.csv", config.Filename)
	assert.Equal(t, "/data", config.OutputDir)
	assert.Equal(t, "test-sa", config.ServiceAccount)
	assert.Equal(t, int32(2), config.RetryLimit)
	assert.Equal(t, "test-pvc", config.NFSVolumeClaim)
}

func TestJobStatus(t *testing.T) {
	status := &JobStatus{
		JobName: "test-job",
		Status:  "running",
		Message: "Job is running",
		PodName: "test-pod",
	}

	assert.Equal(t, "test-job", status.JobName)
	assert.Equal(t, "running", status.Status)
	assert.Equal(t, "Job is running", status.Message)
	assert.Equal(t, "test-pod", status.PodName)
}

func TestKubernetesBatchInvokerInterface(t *testing.T) {
	// Test that our service implements the interface
	var _ KubernetesBatchInvoker = (*KubernetesBatchInvokerService)(nil)
}

func TestHelperFunctions(t *testing.T) {
	t.Run("int32Ptr", func(t *testing.T) {
		val := int32(42)
		ptr := int32Ptr(val)
		require.NotNil(t, ptr)
		assert.Equal(t, val, *ptr)
	})

	t.Run("int64Ptr", func(t *testing.T) {
		val := int64(42)
		ptr := int64Ptr(val)
		require.NotNil(t, ptr)
		assert.Equal(t, val, *ptr)
	})
}
