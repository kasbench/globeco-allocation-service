# GlobeCo Portfolio Accounting Service - CLI Usage Guide

## Overview
This guide provides comprehensive instructions for using the GlobeCo Portfolio Accounting Service CLI tool. The CLI is designed for processing financial transaction files and interacting with the portfolio accounting service.

## File Format Specification

### CSV Transaction File Format

The CLI processes CSV files containing financial transactions. The file must have the following structure:

#### Required Headers (in any order)
```csv
portfolio_id,security_id,source_id,transaction_type,quantity,price,transaction_date
```

#### Column Specifications

| Column | Type | Required | Length/Format | Description |
|--------|------|----------|---------------|-------------|
| `portfolio_id` | String | ✅ Yes | Exactly 24 chars | Portfolio identifier (alphanumeric) |
| `security_id` | String | ❌ No | Exactly 24 chars or empty | Security identifier (empty for cash transactions) |
| `source_id` | String | ✅ Yes | Max 50 chars | Source system identifier |
| `transaction_type` | String | ✅ Yes | 3-5 chars | Transaction type code |
| `quantity` | Decimal | ✅ Yes | Decimal number | Transaction quantity (can be negative) |
| `price` | Decimal | ✅ Yes | Decimal number > 0 | Transaction price (1.0 for cash) |
| `transaction_date` | String | ✅ Yes | YYYYMMDD | Transaction date |

#### Valid Transaction Types

| Type | Description | Security ID | Balance Impact |
|------|-------------|-------------|----------------|
| `BUY` | Buy securities | Required | +Long Units, -Cash |
| `SELL` | Sell securities | Required | -Long Units, +Cash |
| `SHORT` | Short sell | Required | +Short Units, +Cash |
| `COVER` | Cover short | Required | -Short Units, -Cash |
| `DEP` | Cash deposit | Must be empty | +Cash |
| `WD` | Cash withdrawal | Must be empty | -Cash |
| `IN` | Securities transfer in | Required | +Long Units |
| `OUT` | Securities transfer out | Required | -Long Units |

#### Example CSV File
```csv
portfolio_id,security_id,source_id,transaction_type,quantity,price,transaction_date
PORTFOLIO123456789012345,SECURITY1234567890123456,EXT_SYS_001,BUY,100.00,50.25,20240130
PORTFOLIO123456789012345,,CASH_DEP_001,DEP,1000.00,1.0,20240130
PORTFOLIO987654321098765,SECURITY9876543210987654,EXT_SYS_002,SELL,50.00,52.75,20240131
```

#### Business Rules
1. **Cash transactions** (`DEP`, `WD`) must have empty `security_id` and `price = 1.0`
2. **Security transactions** must have valid `security_id` (24 characters)
3. All IDs must be alphanumeric
4. Quantity can be positive or negative
5. Price must be positive (except cash transactions use 1.0)

## CLI Commands

### Available Commands

#### 1. Process Command
Processes transaction files by validating, sorting, and submitting to the service.

```bash
portfolio-cli process [flags]
```

**Flags:**
- `-f, --file` (required): Transaction file to process
- `-o, --output-dir`: Output directory for result files (default: ".")
- `--batch-size`: Batch size for processing transactions (default: 1000)
- `--workers`: Number of concurrent workers (default: 1)
- `--timeout`: Timeout for processing operations (default: 5m)
- `--skip-sort`: Skip sorting step (assumes file is already sorted)
- `--force`: Force processing even with validation warnings

#### 2. Validate Command
Validates transaction files without processing them.

```bash
portfolio-cli validate [flags]
```

**Flags:**
- `-f, --file` (required): Transaction file to validate
- `--strict`: Strict mode - treat warnings as errors

#### 3. Status Command
Checks service status and health.

```bash
portfolio-cli status [flags]
```

**Flags:**
- `--url`: Service URL (default from config)
- `--timeout`: Request timeout (default: 10s)
- `-v, --verbose`: Verbose output

#### 4. Version Command
Prints version information.

```bash
portfolio-cli version
```

### Global Flags
- `-c, --config`: Config file path
- `-v, --verbose`: Enable verbose output
- `--dry-run`: Perform dry run without making changes
- `--log-level`: Log level (debug, info, warn, error)
- `--log-format`: Log format (json, console)

## Deployment Scenarios

### Scenario 1: CLI Running in Docker on Same Host

This scenario involves running the CLI in a Docker container on the same machine as the caller.

#### Prerequisites
- Docker installed and running
- Access to transaction CSV files on the host filesystem
- Network connectivity to the portfolio accounting service

#### Setup

1. **Build the CLI Docker Image:**
```bash
# From the project root directory
docker build --target cli -t globeco-portfolio-cli:latest .
```

2. **Create a Shared Volume Directory:**
```bash
# Create directory for file exchange
mkdir -p /tmp/portfolio-files
chmod 755 /tmp/portfolio-files
```

#### Usage Examples

**Basic File Processing:**
```bash
# Copy your CSV file to the shared directory
cp your-transactions.csv /tmp/portfolio-files/

# Run CLI container with volume mount
docker run --rm \
  -v /tmp/portfolio-files:/data \
  --network host \
  globeco-portfolio-cli:latest \
  process --file /data/your-transactions.csv --output-dir /data
```

**File Validation:**
```bash
docker run --rm \
  -v /tmp/portfolio-files:/data \
  globeco-portfolio-cli:latest \
  validate --file /data/your-transactions.csv --strict
```

**Service Status Check:**
```bash
docker run --rm \
  --network host \
  globeco-portfolio-cli:latest \
  status --url http://localhost:8087 --verbose
```

**Advanced Processing with Custom Settings:**
```bash
docker run --rm \
  -v /tmp/portfolio-files:/data \
  -v /path/to/config.yaml:/etc/globeco/config.yaml:ro \
  --network host \
  globeco-portfolio-cli:latest \
  process \
    --file /data/large-transactions.csv \
    --output-dir /data \
    --batch-size 500 \
    --workers 4 \
    --timeout 60s \
    --config /etc/globeco/config.yaml
```

#### Docker Compose Integration

If using Docker Compose, add the CLI service:

```yaml
services:
  portfolio-accounting-cli:
    build:
      context: .
      dockerfile: Dockerfile
      target: cli
    container_name: globeco-portfolio-cli
    volumes:
      - ./data:/data
      - ./config.yaml:/etc/globeco/config.yaml:ro
    environment:
      GLOBECO_PA_SERVER_HOST: portfolio-accounting-service
      GLOBECO_PA_SERVER_PORT: 8087
    networks:
      - globeco-network
    profiles:
      - tools
```

**Usage with Docker Compose:**
```bash
# Process file using compose
docker-compose run --rm portfolio-accounting-cli \
  process --file /data/transactions.csv

# Check service status
docker-compose run --rm portfolio-accounting-cli \
  status --verbose
```

### Scenario 2: CLI and Caller Running in Kubernetes

This scenario involves deploying both the CLI and the caller in a Kubernetes cluster.

#### Prerequisites
- Kubernetes cluster access with kubectl configured
- Persistent storage for file sharing
- Service networking configured

#### Deployment Resources

**1. ConfigMap for CLI Configuration:**
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: portfolio-cli-config
  namespace: globeco
data:
  config.yaml: |
    server:
      host: portfolio-accounting-service
      port: 8087
    logging:
      level: info
      format: json
    processing:
      batch_size: 1000
      worker_count: 4
      timeout: 300s
```

**2. PersistentVolumeClaim for File Storage:**
```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: portfolio-files-pvc
  namespace: globeco
spec:
  accessModes:
    - ReadWriteMany
  resources:
    requests:
      storage: 10Gi
  storageClassName: nfs  # Use appropriate storage class
```

**3. CLI Job Template:**
```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: portfolio-cli-process
  namespace: globeco
spec:
  template:
    spec:
      restartPolicy: Never
      containers:
      - name: portfolio-cli
        image: globeco-portfolio-cli:latest
        command: ["/usr/local/bin/cli"]
        args: 
          - "process"
          - "--file"
          - "/data/transactions.csv"
          - "--output-dir"
          - "/data"
          - "--config"
          - "/etc/config/config.yaml"
          - "--verbose"
        volumeMounts:
        - name: config-volume
          mountPath: /etc/config
        - name: data-volume
          mountPath: /data
        env:
        - name: GLOBECO_PA_SERVER_HOST
          value: "portfolio-accounting-service"
        - name: GLOBECO_PA_SERVER_PORT
          value: "8087"
      volumes:
      - name: config-volume
        configMap:
          name: portfolio-cli-config
      - name: data-volume
        persistentVolumeClaim:
          claimName: portfolio-files-pvc
```

**4. CLI CronJob for Scheduled Processing:**
```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: portfolio-cli-scheduled
  namespace: globeco
spec:
  schedule: "0 2 * * *"  # Daily at 2 AM
  jobTemplate:
    spec:
      template:
        spec:
          restartPolicy: OnFailure
          containers:
          - name: portfolio-cli
            image: globeco-portfolio-cli:latest
            command: ["/usr/local/bin/cli"]
            args: 
              - "process"
              - "--file"
              - "/data/daily-transactions.csv"
              - "--config"
              - "/etc/config/config.yaml"
            volumeMounts:
            - name: config-volume
              mountPath: /etc/config
            - name: data-volume
              mountPath: /data
          volumes:
          - name: config-volume
            configMap:
              name: portfolio-cli-config
          - name: data-volume
            persistentVolumeClaim:
              claimName: portfolio-files-pvc
```

#### Usage Examples in Kubernetes

**1. File Upload and Processing:**
```bash
# Upload file to shared storage (using a helper pod)
kubectl run file-uploader --rm -i --tty \
  --image=busybox \
  --overrides='
{
  "spec": {
    "containers": [{
      "name": "file-uploader",
      "image": "busybox",
      "stdin": true,
      "tty": true,
      "volumeMounts": [{
        "name": "data-volume",
        "mountPath": "/data"
      }]
    }],
    "volumes": [{
      "name": "data-volume",
      "persistentVolumeClaim": {
        "claimName": "portfolio-files-pvc"
      }
    }]
  }
}' -- sh

# Inside the pod, copy your file
# cp /path/to/local/file /data/

# Process the file
kubectl create job --from=cronjob/portfolio-cli-scheduled manual-process-$(date +%s)
```

**2. File Validation:**
```bash
kubectl run portfolio-cli-validate --rm -i --tty \
  --image=globeco-portfolio-cli:latest \
  --overrides='
{
  "spec": {
    "containers": [{
      "name": "portfolio-cli",
      "image": "globeco-portfolio-cli:latest",
      "stdin": true,
      "tty": true,
      "command": ["/usr/local/bin/cli"],
      "args": ["validate", "--file", "/data/transactions.csv", "--strict"],
      "volumeMounts": [{
        "name": "data-volume",
        "mountPath": "/data"
      }, {
        "name": "config-volume",
        "mountPath": "/etc/config"
      }]
    }],
    "volumes": [{
      "name": "data-volume",
      "persistentVolumeClaim": {
        "claimName": "portfolio-files-pvc"
      }
    }, {
      "name": "config-volume",
      "configMap": {
        "name": "portfolio-cli-config"
      }
    }]
  }
}'
```

**3. Service Status Check:**
```bash
kubectl run portfolio-cli-status --rm -i --tty \
  --image=globeco-portfolio-cli:latest \
  -- status --verbose
```

**4. Batch Processing with Job:**
```bash
# Create job from template
envsubst < cli-job-template.yaml | kubectl apply -f -

# Monitor job progress
kubectl logs -f job/portfolio-cli-process

# Check job status
kubectl get jobs
kubectl describe job portfolio-cli-process
```

#### File Management in Kubernetes

**Upload files using kubectl cp:**
```bash
# Copy file to running pod
kubectl cp ./transactions.csv \
  file-manager-pod:/data/transactions.csv

# Or use a dedicated file manager deployment
kubectl run file-manager --image=busybox \
  --command -- sleep 3600
kubectl cp ./transactions.csv \
  file-manager:/tmp/transactions.csv
```

**Access files from other pods:**
```bash
# Create a pod to access processed files
kubectl run file-reader --rm -i --tty \
  --image=busybox \
  --overrides='
{
  "spec": {
    "containers": [{
      "name": "file-reader",
      "image": "busybox",
      "stdin": true,
      "tty": true,
      "volumeMounts": [{
        "name": "data-volume",
        "mountPath": "/data"
      }]
    }],
    "volumes": [{
      "name": "data-volume",
      "persistentVolumeClaim": {
        "claimName": "portfolio-files-pvc"
      }
    }]
  }
}' -- sh

# Inside pod: ls /data/ to see processed files
```

## Error Handling and Troubleshooting

### Common Issues

**1. File Format Errors:**
- Ensure CSV headers match exactly
- Check for special characters in data
- Verify portfolio_id and security_id lengths (24 chars)
- Validate transaction_date format (YYYYMMDD)

**2. Service Connectivity:**
- Verify service URL and port
- Check network connectivity
- Ensure service is running and healthy

**3. Docker Issues:**
- Verify volume mounts are correct
- Check file permissions
- Ensure network connectivity between containers

**4. Kubernetes Issues:**
- Verify PVC is bound and accessible
- Check pod logs: `kubectl logs pod-name`
- Ensure ConfigMaps and Secrets are mounted
- Verify network policies allow communication

### Monitoring and Logging

**View CLI logs:**
```bash
# Docker
docker logs container-name

# Kubernetes
kubectl logs job/portfolio-cli-process
kubectl logs -f deployment/portfolio-cli
```

**Check processing results:**
- Output files are written to the specified output directory
- Error files contain failed transactions with error messages
- Success files contain processed transaction confirmations

## Configuration Reference

### Environment Variables

All configuration can be provided via environment variables with the `GLOBECO_PA_` prefix:

```bash
# Service connection
GLOBECO_PA_SERVER_HOST=localhost
GLOBECO_PA_SERVER_PORT=8087

# Processing settings
GLOBECO_PA_PROCESSING_BATCH_SIZE=1000
GLOBECO_PA_PROCESSING_WORKERS=4
GLOBECO_PA_PROCESSING_TIMEOUT=300s

# Logging
GLOBECO_PA_LOGGING_LEVEL=info
GLOBECO_PA_LOGGING_FORMAT=json
```

### Configuration File Template

```yaml
server:
  host: "localhost"
  port: 8087
  
processing:
  batch_size: 1000
  worker_count: 4
  timeout: 300s
  
logging:
  level: "info"
  format: "json"
  
output:
  directory: "/data"
  generate_stats: true
```

This guide provides comprehensive coverage of the CLI usage patterns for both Docker and Kubernetes environments. Adjust the specific values, image names, and configurations according to your deployment requirements. 