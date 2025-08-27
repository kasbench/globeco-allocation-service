# Implementation Plan

- [x] 1. Set up Kubernetes client integration and configuration
  - Add Kubernetes Go client library dependency to go.mod
  - Create configuration structure for Kubernetes batch job settings
  - Implement configuration loading for new Kubernetes-related settings
  - _Requirements: 1.2, 6.1_

- [ ] 2. Implement core Kubernetes batch invoker service
  - [x] 2.1 Create KubernetesBatchInvoker interface and basic structure
    - Define interface for Kubernetes batch job operations
    - Create basic service structure with Kubernetes client integration
    - Implement constructor with client initialization and validation
    - _Requirements: 1.1, 1.3_

  - [x] 2.2 Implement batch job manifest generation
    - Create job template structure and configuration
    - Implement dynamic job manifest generation with proper volume mounts
    - Add job naming with timestamps and proper labeling
    - _Requirements: 1.3, 6.2_

  - [x] 2.3 Implement job submission and monitoring logic
    - Code job creation and submission to Kubernetes API
    - Implement job status monitoring with polling mechanism
    - Add timeout handling and job completion detection
    - _Requirements: 1.4, 2.1, 2.2_

- [x] 3. Create retry logic and error handling
  - [x] 3.1 Implement retry mechanism for batch jobs
    - Create retry logic for failed Kubernetes API calls
    - Implement exponential backoff for transient failures
    - Add detailed error logging and status reporting
    - _Requirements: 7.1, 7.3_

  - [x] 3.2 Implement comprehensive error handling
    - Add specific error handling for RBAC permission issues
    - Implement fallback mechanisms for Kubernetes connectivity problems
    - Create detailed error messages for debugging failed jobs
    - _Requirements: 2.1, 2.2, 7.2_

- [x] 4. Enhance execution service for dual-mode operation
  - [x] 4.1 Modify execution service to support both execution modes
    - Update ExecutionService to integrate KubernetesBatchInvoker
    - Implement configuration-based selection between direct and Kubernetes execution
    - Add new SendWithKubernetes method for Kubernetes batch job flow
    - _Requirements: 1.1, 1.2_

  - [x] 4.2 Implement file cleanup logic for successful jobs
    - Modify file cleanup to only delete files after successful job completion
    - Add job status validation before file deletion
    - Implement proper error handling for cleanup failures
    - _Requirements: 2.3, 2.4_

- [ ] 5. Implement retry endpoint functionality
  - [ ] 5.1 Create retry service method
    - Implement RetryExecution method in ExecutionService
    - Add file existence validation for retry operations
    - Create batch job submission for existing files without regeneration
    - _Requirements: 3.1, 3.2_

  - [ ] 5.2 Create retry API endpoint handler
    - Implement POST /api/v1/executions/send/retry endpoint handler
    - Add request validation for filename parameter
    - Implement proper HTTP status code responses for success/failure
    - _Requirements: 3.3, 3.4_

- [ ] 6. Implement batch history deletion functionality
  - [ ] 6.1 Create batch history deletion service method
    - Implement DeleteLastBatchHistory method in ExecutionService
    - Add validation to ensure only the last record is deleted
    - Implement proper error handling for deletion failures
    - _Requirements: 4.1, 4.3_

  - [ ] 6.2 Create batch history deletion API endpoint
    - Implement POST /api/v1/executions/send/delete_last endpoint handler
    - Add proper HTTP status code responses for success/failure scenarios
    - Implement validation and error handling for deletion operations
    - _Requirements: 4.2, 4.3_

- [ ] 7. Create Kubernetes RBAC manifests
  - Create ServiceAccount manifest for globeco-allocation-service
  - Create Role manifest with batch job management permissions
  - Create RoleBinding to associate service account with role
  - _Requirements: 6.1, 6.2, 6.3_

- [ ] 8. Update API documentation
  - [ ] 8.1 Update OpenAPI specification
    - Add new retry endpoint to OpenAPI specification with request/response schemas
    - Add new delete_last endpoint to OpenAPI specification
    - Update existing send endpoint documentation to include new response fields
    - _Requirements: 5.1, 5.2_

  - [ ] 8.2 Update README.md documentation
    - Document new API endpoints with usage examples
    - Add configuration documentation for Kubernetes batch job settings
    - Include RBAC setup instructions and deployment notes
    - _Requirements: 5.3_

- [ ] 9. Integration testing preparation
  - [ ] 9.1 Create test configuration and setup scripts
    - Create test configuration files for Kubernetes batch job testing
    - Write setup scripts for RBAC configuration deployment
    - Create test data files for integration testing scenarios
    - _Requirements: 7.4_

  - [ ] 9.2 Implement basic validation and connectivity tests
    - Create validation functions to test Kubernetes API connectivity
    - Implement RBAC permission validation tests
    - Add NFS mount accessibility validation from job pods
    - _Requirements: 6.4, 7.4_

- [ ] 10. Stop for manual integration testing
  - Integration testing must be performed manually in Kubernetes environment
  - Test basic job creation, execution, and monitoring
  - Validate new API endpoints functionality
  - Test error scenarios and retry mechanisms
  - _Requirements: 7.4_