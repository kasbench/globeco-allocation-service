# Requirements Document

## Introduction

This enhancement transforms the Portfolio Accounting CLI invocation mechanism from direct os/exec calls to Kubernetes batch jobs. The service currently processes POST `/api/v1/executions/send` requests by generating files on a NAS share and invoking CLI commands directly. This enhancement will add the capability to dynamically generate and submit Kubernetes batch jobs for CLI execution, providing better scalability, monitoring, and retry capabilities in a Kubernetes environment.

## Requirements

### Requirement 1

**User Story:** As a service operator, I want the system to use Kubernetes batch jobs instead of direct CLI execution, so that I can leverage Kubernetes' job management and monitoring capabilities.

#### Acceptance Criteria

1. WHEN a POST request is made to `/api/v1/executions/send` THEN the system SHALL provide an option to use Kubernetes batch jobs instead of direct CLI execution
2. WHEN using batch job mode THEN the system SHALL use the Kubernetes Go Client Library (client-go) to dynamically generate and submit batch jobs
3. WHEN creating a batch job THEN the system SHALL ensure the job can access the same NAS drive as the service in read-only mode
4. WHEN the batch job is submitted THEN the system SHALL monitor the job for completion and capture its status

### Requirement 2

**User Story:** As a service operator, I want proper error handling and status reporting for batch jobs, so that I can identify and respond to failures appropriately.

#### Acceptance Criteria

1. WHEN a batch job fails THEN the system SHALL return a 500 error code
2. WHEN a batch job succeeds THEN the system SHALL return a 200 success code
3. WHEN file deletion is enabled AND the batch job succeeds THEN the system SHALL delete the generated file
4. WHEN file deletion is enabled AND the batch job fails THEN the system SHALL NOT delete the generated file

### Requirement 3

**User Story:** As a service operator, I want a retry endpoint for failed batch jobs, so that I can reprocess files without regenerating them.

#### Acceptance Criteria

1. WHEN a POST request is made to `/api/v1/executions/send/retry` with a filename THEN the system SHALL retry generating and submitting the batch job for that file
2. WHEN using the retry endpoint THEN the system SHALL NOT regenerate the file
3. WHEN the retry batch job succeeds THEN the system SHALL return a 200 success code
4. WHEN the retry batch job fails THEN the system SHALL return a 500 error code

### Requirement 4

**User Story:** As a service operator, I want to delete the last batch history record, so that I can rerun the entire batch process including file generation.

#### Acceptance Criteria

1. WHEN a POST request is made to `/api/v1/executions/send/delete_last` THEN the system SHALL delete the last record inserted in the `batch_history` table by this service
2. WHEN the row is successfully deleted THEN the system SHALL return a 200 success code
3. WHEN the row deletion fails THEN the system SHALL return a 500 error code

### Requirement 5

**User Story:** As a developer, I want proper API documentation for the new endpoints, so that I can understand how to use them.

#### Acceptance Criteria

1. WHEN new endpoints are created THEN they SHALL be documented in the README.md file
2. WHEN new endpoints are created THEN they SHALL be documented in the OpenAPI specification
3. WHEN documentation is updated THEN it SHALL include request/response examples and error codes

### Requirement 6

**User Story:** As a service operator, I want proper Kubernetes RBAC configuration, so that the service has the necessary permissions to create and monitor batch jobs.

#### Acceptance Criteria

1. WHEN deploying the service THEN the system SHALL include Kubernetes manifests for service accounts, roles, and role bindings
2. WHEN the service account is created THEN it SHALL have permissions to create batch jobs in the `globeco` namespace
3. WHEN the service account is created THEN it SHALL have permissions to access the NAS share
4. WHEN the service account is created THEN it SHALL have permissions to monitor job status and logs

### Requirement 7

**User Story:** As a developer, I want early retry logic implementation, so that I can iterate quickly during development when batch jobs may fail frequently.

#### Acceptance Criteria

1. WHEN implementing the feature THEN retry logic SHALL be developed early in the implementation plan
2. WHEN a batch job fails THEN the system SHALL provide detailed error information for debugging
3. WHEN retry logic is implemented THEN it SHALL handle common Kubernetes job failure scenarios
4. WHEN integration testing is required THEN the implementation SHALL stop and request manual testing in Kubernetes environment