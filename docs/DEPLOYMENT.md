# GlobeCo Allocation Service - Deployment Guide

## Overview

This document provides comprehensive instructions for deploying the GlobeCo Allocation Service in various environments including local development, staging, and production.

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Local Development Deployment](#local-development-deployment)
3. [Kubernetes Deployment](#kubernetes-deployment)
4. [Production Deployment](#production-deployment)
5. [Environment Configuration](#environment-configuration)
6. [Monitoring and Observability](#monitoring-and-observability)
7. [Troubleshooting](#troubleshooting)
8. [Operational Procedures](#operational-procedures)

## Prerequisites

### Required Software

- **Docker**: 20.10+ with Docker Compose
- **Kubernetes**: 1.28+ cluster
- **kubectl**: 1.28+ configured for your cluster
- **Helm**: 3.12+ (optional, for advanced deployments)

### Required Permissions

- Docker registry access for image pushing/pulling
- Kubernetes cluster admin access (for initial setup)
- Service account with appropriate RBAC permissions

### External Dependencies

- PostgreSQL 17+ database instance
- GlobeCo Trade Service (v2 API)
- Portfolio Accounting CLI service
- Monitoring infrastructure (Prometheus, Grafana, Jaeger)

## Local Development Deployment

### Using Docker Compose

1. **Clone the repository and navigate to the project directory**
   ```bash
   git clone <repository-url>
   cd globeco-allocation-service
   ```

2. **Build and start all services**
   ```bash
   docker-compose up -d
   ```

3. **Verify deployment**
   ```bash
   # Check service health
   curl http://localhost:8089/healthz
   
   # Check all services are running
   docker-compose ps
   ```

4. **Access services**
   - Allocation Service: http://localhost:8089
   - PostgreSQL: localhost:5432
   - Prometheus: http://localhost:9090
   - Grafana: http://localhost:3000 (admin/admin123)
   - Jaeger UI: http://localhost:16686

### Environment Variables for Development

```bash
export LOG_LEVEL=debug
export DATABASE_SSL_MODE=disable
export TRADE_SERVICE_URL=http://trade-service:8082
export CLI_COMMAND="echo 'Mock CLI command executed'"
export FILE_CLEANUP_ENABLED=false
```

## Kubernetes Deployment

### Basic Deployment

1. **Create namespace (optional)**
   ```bash
   kubectl create namespace globeco-allocation
   kubectl config set-context --current --namespace=globeco-allocation
   ```

2. **Apply base configurations**
   ```bash
   # Apply in order
   kubectl apply -f k8s/secret.yaml
   kubectl apply -f k8s/configmap.yaml
   kubectl apply -f k8s/rbac.yaml
   kubectl apply -f k8s/pvc.yaml
   ```

3. **Deploy the application**
   ```bash
   kubectl apply -f k8s/deployment.yaml
   kubectl apply -f k8s/service.yaml
   kubectl apply -f k8s/ingress.yaml
   ```

4. **Verify deployment**
   ```bash
   # Check pod status
   kubectl get pods -l app=globeco-allocation-service
   
   # Check logs
   kubectl logs -f deployment/globeco-allocation-service
   
   # Check service health
   kubectl port-forward service/globeco-allocation-service 8089:8089
   curl http://localhost:8089/healthz
   ```

### Environment-Specific Deployments

#### Development Environment
```bash
# Use development configuration
kubectl apply -f k8s/configmap.yaml  # Contains dev-specific config
kubectl set env deployment/globeco-allocation-service --from=configmap/allocation-service-config-dev
```

#### Production Environment
```bash
# Use production configuration and higher replica count
kubectl apply -f k8s/configmap.yaml  # Contains prod-specific config
kubectl set env deployment/globeco-allocation-service --from=configmap/allocation-service-config-prod
kubectl scale deployment globeco-allocation-service --replicas=5
```

## Production Deployment

### Pre-deployment Checklist

- [ ] Database migration scripts tested
- [ ] External service connectivity verified
- [ ] SSL certificates configured
- [ ] Monitoring and alerting set up
- [ ] Backup procedures in place
- [ ] Rollback procedures documented
- [ ] Security scanning completed
- [ ] Performance testing completed

### Production Configuration

1. **Update secrets with production values**
   ```bash
   # Generate strong password
   PROD_PASSWORD=$(openssl rand -base64 32)
   
   # Update secret
   kubectl create secret generic allocation-service-secrets \
     --from-literal=database-password="$PROD_PASSWORD" \
     --dry-run=client -o yaml | kubectl apply -f -
   ```

2. **Configure TLS certificates**
   ```bash
   # If using cert-manager
   kubectl apply -f - <<EOF
   apiVersion: cert-manager.io/v1
   kind: Certificate
   metadata:
     name: allocation-service-tls
   spec:
     secretName: allocation-service-tls
     issuerRef:
       name: letsencrypt-prod
       kind: ClusterIssuer
     dnsNames:
     - allocation-service.globeco.com
     - api.globeco.com
   EOF
   ```

3. **Deploy with production settings**
   ```bash
   # Apply production manifests
   kubectl apply -k k8s/overlays/production
   ```

### Rolling Updates

```bash
# Update image version
kubectl set image deployment/globeco-allocation-service \
  allocation-service=globeco-allocation-service:1.0.1

# Monitor rollout
kubectl rollout status deployment/globeco-allocation-service

# Rollback if needed
kubectl rollout undo deployment/globeco-allocation-service
```

## Environment Configuration

### Development
- Single replica
- Debug logging
- Mock external services
- Minimal resource limits
- SSL disabled for database

### Staging
- 2 replicas
- Info logging
- Real external services
- Moderate resource limits
- SSL enabled

### Production
- 3+ replicas (auto-scaling recommended)
- Warn/Error logging only
- Production external services
- High resource limits
- Full SSL/TLS encryption
- Network policies enabled

## Monitoring and Observability

### Metrics Collection

```bash
# Verify Prometheus scraping
kubectl get servicemonitor allocation-service-metrics

# Check metrics endpoint
kubectl port-forward service/globeco-allocation-service 8089:8089
curl http://localhost:8089/metrics
```

### Log Aggregation

```bash
# View structured logs
kubectl logs -f deployment/globeco-allocation-service | jq '.'

# Filter by severity
kubectl logs deployment/globeco-allocation-service | jq 'select(.level=="ERROR")'
```

### Distributed Tracing

```bash
# Port forward to Jaeger
kubectl port-forward service/jaeger-query 16686:16686

# Access Jaeger UI at http://localhost:16686
```

### Health Checks

```bash
# Liveness probe
curl http://allocation-service.globeco.local/healthz

# Readiness probe
curl http://allocation-service.globeco.local/readyz

# Detailed health status
curl http://allocation-service.globeco.local/api/v1/health
```

## Troubleshooting

### Common Issues

#### Pod Startup Issues
```bash
# Check pod events
kubectl describe pod <pod-name>

# Check logs
kubectl logs <pod-name> --previous

# Check resource constraints
kubectl top pod <pod-name>
```

#### Database Connection Issues
```bash
# Verify database connectivity
kubectl run -it --rm debug --image=postgres:17-alpine --restart=Never -- \
  psql -h globeco-allocation-service-postgresql -U postgres -d postgres

# Check database credentials
kubectl get secret allocation-service-secrets -o yaml
```

#### Service Discovery Issues
```bash
# Check service endpoints
kubectl get endpoints globeco-allocation-service

# Test internal connectivity
kubectl run -it --rm debug --image=busybox --restart=Never -- \
  wget -qO- http://globeco-allocation-service:8089/healthz
```

#### Ingress Issues
```bash
# Check ingress status
kubectl describe ingress globeco-allocation-service-ingress

# Verify ingress controller
kubectl get pods -n ingress-nginx
```

### Debug Commands

```bash
# Enable debug logging
kubectl set env deployment/globeco-allocation-service LOG_LEVEL=debug

# Scale down for maintenance
kubectl scale deployment globeco-allocation-service --replicas=0

# Emergency pod access
kubectl exec -it deployment/globeco-allocation-service -- /bin/sh
```

## Operational Procedures

### Backup Procedures

1. **Database Backup**
   ```bash
   # Create database backup
   kubectl exec -it postgres-pod -- pg_dump -U postgres allocation_db > backup.sql
   ```

2. **Configuration Backup**
   ```bash
   # Backup all configurations
   kubectl get configmap,secret,deployment,service -o yaml > backup-config.yaml
   ```

### Disaster Recovery

1. **Service Recovery**
   ```bash
   # Restore from backup
   kubectl apply -f backup-config.yaml
   
   # Verify service health
   kubectl get pods -w
   ```

2. **Database Recovery**
   ```bash
   # Restore database
   kubectl exec -i postgres-pod -- psql -U postgres allocation_db < backup.sql
   ```

### Maintenance Windows

1. **Planned Maintenance**
   ```bash
   # Drain traffic
   kubectl scale deployment globeco-allocation-service --replicas=0
   
   # Perform maintenance
   # ...
   
   # Restore traffic
   kubectl scale deployment globeco-allocation-service --replicas=3
   ```

2. **Zero-Downtime Updates**
   ```bash
   # Rolling update
   kubectl set image deployment/globeco-allocation-service \
     allocation-service=globeco-allocation-service:new-version
   ```

### Performance Tuning

1. **Resource Optimization**
   ```bash
   # Update resource limits
   kubectl patch deployment globeco-allocation-service -p \
     '{"spec":{"template":{"spec":{"containers":[{"name":"allocation-service","resources":{"limits":{"memory":"2Gi","cpu":"2000m"}}}]}}}}'
   ```

2. **Auto-scaling**
   ```bash
   # Enable horizontal pod autoscaler
   kubectl autoscale deployment globeco-allocation-service --cpu-percent=70 --min=3 --max=10
   ```

### Security Maintenance

1. **Certificate Renewal**
   ```bash
   # Check certificate expiry
   kubectl describe certificate allocation-service-tls
   
   # Force renewal if needed
   kubectl annotate certificate allocation-service-tls cert-manager.io/issue-temporary-certificate="true"
   ```

2. **Security Updates**
   ```bash
   # Update base images
   docker build --no-cache -t globeco-allocation-service:security-update .
   kubectl set image deployment/globeco-allocation-service allocation-service=globeco-allocation-service:security-update
   ```

## Contact and Support

- **Development Team**: Noah Krieger (noah@kasbench.org)
- **Operations Team**: [Contact Information]
- **Emergency Escalation**: [Emergency Procedures]

For additional support, refer to the project's issue tracker or internal documentation system. 