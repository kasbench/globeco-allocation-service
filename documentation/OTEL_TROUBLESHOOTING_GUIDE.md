# OpenTelemetry Troubleshooting Guide for globeco-allocation-service

This guide helps troubleshoot common OpenTelemetry issues in the globeco-allocation-service.

## Common Error: TLS Handshake Failed

**Error Message:**
```
failed to upload metrics: exporter export timeout: rpc error: code = Unavailable desc = connection error: desc = "transport: authentication handshake failed: tls: first record does not look like a TLS handshake"
```

**Root Cause:** The service is trying to establish a TLS connection to the OpenTelemetry Collector, but the collector is configured for insecure (non-TLS) connections.

### Solution Steps:

#### 1. Verify OpenTelemetry Collector Configuration

Check if the OTEL Collector is running and configured for insecure connections:

```bash
# Check if OTEL Collector is running
kubectl get pods -n monitoring | grep otel-collector

# Check OTEL Collector service
kubectl get svc -n monitoring | grep otel-collector

# Check OTEL Collector configuration
kubectl describe configmap -n monitoring otel-collector-config
```

The collector should have receivers configured like this:
```yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318
```

#### 2. Verify Network Connectivity

Test if the service can reach the OTEL Collector:

```bash
# From within the allocation service pod
kubectl exec -it deployment/globeco-allocation-service -- /bin/sh

# Test connectivity to OTEL Collector
nc -zv otel-collector-collector.monitoring.svc.cluster.local 4317
# or
telnet otel-collector-collector.monitoring.svc.cluster.local 4317
```

#### 3. Check DNS Resolution

Verify that the service can resolve the OTEL Collector hostname:

```bash
# From within the allocation service pod
kubectl exec -it deployment/globeco-allocation-service -- nslookup otel-collector-collector.monitoring.svc.cluster.local
```

#### 4. Verify Environment Variables

Check that all OTEL environment variables are properly set:

```bash
# Check environment variables in the running pod
kubectl exec -it deployment/globeco-allocation-service -- env | grep -i otel
```

Expected variables:
- `OBSERVABILITY_OTEL_ENABLED=true`
- `OBSERVABILITY_OTEL_ENDPOINT=otel-collector-collector.monitoring.svc.cluster.local:4317`
- `OTEL_EXPORTER_OTLP_INSECURE=true`
- `OTEL_EXPORTER_OTLP_PROTOCOL=grpc`

#### 5. Check Service Logs

Look for OTEL initialization messages:

```bash
# Check service logs for OTEL messages
kubectl logs deployment/globeco-allocation-service | grep -i otel

# Look for specific initialization messages
kubectl logs deployment/globeco-allocation-service | grep "OpenTelemetry initialized successfully"
kubectl logs deployment/globeco-allocation-service | grep "OTLP.*exporter created successfully"
```

Expected log messages:
- "Setting up OpenTelemetry with GlobeCo standards"
- "Creating OTLP trace exporter with insecure connection"
- "Creating OTLP metric exporter with insecure connection"
- "OTLP trace exporter created successfully"
- "OTLP metric exporter created successfully"
- "OpenTelemetry initialized successfully with GlobeCo standards"

## Alternative Solutions

### Option 1: Use HTTP Instead of gRPC

If gRPC continues to have issues, modify the configuration to use HTTP:

```yaml
# In configmap-dev.yaml
OBSERVABILITY_OTEL_ENDPOINT: "http://otel-collector-collector.monitoring.svc.cluster.local:4318"
OTEL_EXPORTER_OTLP_ENDPOINT: "http://otel-collector-collector.monitoring.svc.cluster.local:4318"
OTEL_EXPORTER_OTLP_PROTOCOL: "http/protobuf"
```

### Option 2: Use Different Collector Endpoint

Try using the service IP directly instead of DNS name:

```bash
# Get the collector service IP
kubectl get svc -n monitoring otel-collector-collector -o jsonpath='{.spec.clusterIP}'

# Use the IP in configuration
OBSERVABILITY_OTEL_ENDPOINT: "<COLLECTOR_IP>:4317"
```

### Option 3: Enable Debug Logging

Add more verbose logging to troubleshoot:

```yaml
# In configmap-dev.yaml
LOG_LEVEL: "debug"
OTEL_LOG_LEVEL: "debug"
OBSERVABILITY_LOG_DEVELOPMENT: "true"
```

## Verification Commands

### 1. Test OTEL Collector Directly

```bash
# Install grpcurl in a test pod
kubectl run grpc-test --rm -it --image=fullstorydev/grpcurl -- /bin/sh

# Test OTEL Collector gRPC endpoint
grpcurl -plaintext otel-collector-collector.monitoring.svc.cluster.local:4317 list
```

### 2. Check Metrics Endpoint

```bash
# Check if metrics are being collected locally
kubectl exec -it deployment/globeco-allocation-service -- curl http://localhost:8089/metrics
```

### 3. Monitor OTEL Collector Logs

```bash
# Watch OTEL Collector logs for incoming data
kubectl logs -f -n monitoring deployment/otel-collector-collector
```

## Network Policies

If using network policies, ensure the allocation service can reach the OTEL Collector:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-otel-traffic
  namespace: default
spec:
  podSelector:
    matchLabels:
      app: globeco-allocation-service
  policyTypes:
  - Egress
  egress:
  - to:
    - namespaceSelector:
        matchLabels:
          name: monitoring
    ports:
    - protocol: TCP
      port: 4317
    - protocol: TCP
      port: 4318
```

## Quick Fix Commands

### Restart the Service
```bash
kubectl rollout restart deployment/globeco-allocation-service
```

### Update ConfigMap and Restart
```bash
kubectl apply -f k8s/configmap-dev.yaml
kubectl rollout restart deployment/globeco-allocation-service
```

### Check Service Status
```bash
kubectl get pods -l app=globeco-allocation-service
kubectl describe pod -l app=globeco-allocation-service
```

## Expected Behavior After Fix

1. **Service Logs Should Show:**
   - "OpenTelemetry initialized successfully with GlobeCo standards"
   - No TLS handshake errors
   - Successful metric and trace exports

2. **OTEL Collector Logs Should Show:**
   - Incoming OTLP data from globeco-allocation-service
   - Successful forwarding to Jaeger and Prometheus

3. **Jaeger UI Should Show:**
   - Traces from globeco-allocation-service
   - Spans for HTTP requests, database operations, and Trade Service calls

4. **Prometheus Should Show:**
   - Metrics from globeco-allocation-service
   - Go runtime metrics, HTTP metrics, database metrics

## Contact Information

If issues persist, check:
1. OTEL Collector configuration in the monitoring namespace
2. Network connectivity between namespaces
3. Kubernetes DNS resolution
4. Service mesh configuration (if applicable)

The key is ensuring the OTEL Collector is configured for insecure connections and the service can reach it over the network.