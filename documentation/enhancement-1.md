# Enhancement 1

The purpose of this enhancement is to change the way it invokes the Portfolio Accounting CLI.  

- Please refer to the [CLI Usage Guide](CLI-Usage-Guide.md) for more information on the CLI
- Currently, the CLI is invoked as a step in processing a POST `/api/v1/executions/send` request.  Currently, the first step is to produce a file and save it to a NAS share.  Please see the [deployment manifest](../k8s/deployment.yaml) for details on how the NFS share is mounted.  Once the file is produced, this service currently uses os/exec to invoke a command (see [cli_invoker.go](../internal/service/cli_invoker.go)).
- This enhancement will add an option to use the Kubernetes Go Client Library (client-go) to dynamically generate and submit a Kubernetes batch job.  The requirements for the batch job are documented in the [CLI Usage Guide](CLI-Usage-Guide.md).  Please note that the batch job must be able to access the file produced by this service, so the batch job must mount the same NAS drive as this service (it can be in read-only mode).
- The service should monitor the batch job for completion and note its status.  If the batch job fails, the service should return a 500 error code.  If the batch job succeeds, the service should return a 200 success code.
- There is currently an option to delete the file produced by this service.  If this option is selected, the file should only be deleted if the batch job is successful.
- There should be a new API endpoint `/api/v1/executions/send/retry` that accepts a POST request with a body containing a filename.  This API should retry generating and submitting the batch job to invoke the CLI for this file.  It should not regenerate the file.  The new endpoint should return a 200 success code if the batch job is successful, and a 500 error code if the batch job fails.
- There should be a new API endpoint `/api/v1/executions/send/delete_last` that accepts a POST request.  This API should delete the last record inserted in the `batch_history` table by this service.  This will allow us to rerun the batch including generating a file.  It should return a 200 success code if the row was deleted, and a 500 error code it was not deleted.
- The new API endpoint should be documented in the README.md file and the OpenAPI specification.
- Please generate the Kubernetes manifests for any service accounts/role/role bindings necessary for this service to use client-go.
- Integration testing will necessarily take place in Kubernetes.  I will do the integration testing.  When developing the plan, please stop when integration testing is required and ask me to do it.  Do not waste time trying to create elaborate integration tests that don't prove that something will work in Kubernetes.  Important: Please remember this when developing the plan.  
- We are using Kubernetes 1.33.  The namespace for everything we develop is `globeco`.
- Please make sure that the service account used by the service has the necessary permissions to access the NAS share and to create batch jobs in the namespace.
- Since it is likely that it will take several attempts to get the batch job to run sucessfully while we are developing this functionality, the retry logic will be important.  Please generate the retry logic early in the plan.
