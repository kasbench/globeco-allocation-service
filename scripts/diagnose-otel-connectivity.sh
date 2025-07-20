#!/bin/bash

# OpenTelemetry Connectivity Diagnostic Script
# Run this script to diagnose OTEL connectivity issues

set -e

echo "🔧 OpenTelemetry Connectivity Diagnostics"
echo "========================================"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

print_info() {
    echo -e "${BLUE}ℹ️  $1${NC}"
}

print_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

print_error() {
    echo -e "${RED}❌ $1${NC}"
}

echo
print_info "1. Checking OpenTelemetry Collector status..."

# Check if OTEL Collector is running
if kubectl get pods -n monitoring | grep -q otel-collector; then
    print_success "OTEL Collector pods found in monitoring namespace"
    kubectl get pods -n monitoring | grep otel-collector
else
    print_error "OTEL Collector pods not found in monitoring namespace"
    print_info "Checking other namespaces..."
    kubectl get pods --all-namespaces | grep otel-collector || print_warning "No OTEL Collector found in any namespace"
fi

echo
print_info "2. Checking OTEL Collector service..."

# Check OTEL Collector service
if kubectl get svc -n monitoring | grep -q otel-collector; then
    print_success "OTEL Collector service found"
    kubectl get svc -n monitoring | grep otel-collector
    
    # Get service details
    COLLECTOR_IP=$(kubectl get svc -n monitoring otel-collector-collector -o jsonpath='{.spec.clusterIP}' 2>/dev/null || echo "N/A")
    print_info "Collector ClusterIP: $COLLECTOR_IP"
else
    print_error "OTEL Collector service not found in monitoring namespace"
fi

echo
print_info "3. Checking allocation service status..."

# Check allocation service
if kubectl get pods | grep -q globeco-allocation-service; then
    print_success "Allocation service pods found"
    kubectl get pods | grep globeco-allocation-service
    
    # Get pod name for further testing
    POD_NAME=$(kubectl get pods -l app=globeco-allocation-service -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
    if [ -n "$POD_NAME" ]; then
        print_info "Using pod: $POD_NAME"
    fi
else
    print_error "Allocation service pods not found"
    exit 1
fi

echo
print_info "4. Checking environment variables in allocation service..."

if [ -n "$POD_NAME" ]; then
    print_info "OTEL-related environment variables:"
    kubectl exec $POD_NAME -- env | grep -i otel || print_warning "No OTEL environment variables found"
    
    echo
    print_info "Observability-related environment variables:"
    kubectl exec $POD_NAME -- env | grep -i observability || print_warning "No observability environment variables found"
else
    print_warning "Cannot check environment variables - no pod available"
fi

echo
print_info "5. Testing network connectivity..."

if [ -n "$POD_NAME" ]; then
    print_info "Testing DNS resolution..."
    if kubectl exec $POD_NAME -- nslookup otel-collector-collector.monitoring.svc.cluster.local > /dev/null 2>&1; then
        print_success "DNS resolution successful"
        kubectl exec $POD_NAME -- nslookup otel-collector-collector.monitoring.svc.cluster.local | head -5
    else
        print_error "DNS resolution failed"
    fi
    
    echo
    print_info "Testing port connectivity..."
    if kubectl exec $POD_NAME -- timeout 5 sh -c 'echo > /dev/tcp/otel-collector-collector.monitoring.svc.cluster.local/4317' 2>/dev/null; then
        print_success "Port 4317 (gRPC) is reachable"
    else
        print_error "Port 4317 (gRPC) is not reachable"
    fi
    
    if kubectl exec $POD_NAME -- timeout 5 sh -c 'echo > /dev/tcp/otel-collector-collector.monitoring.svc.cluster.local/4318' 2>/dev/null; then
        print_success "Port 4318 (HTTP) is reachable"
    else
        print_warning "Port 4318 (HTTP) is not reachable"
    fi
else
    print_warning "Cannot test connectivity - no pod available"
fi

echo
print_info "6. Checking service logs for OTEL messages..."

if [ -n "$POD_NAME" ]; then
    print_info "Recent OTEL-related log messages:"
    kubectl logs $POD_NAME --tail=50 | grep -i otel || print_warning "No OTEL messages found in recent logs"
    
    echo
    print_info "Checking for TLS/connection errors:"
    kubectl logs $POD_NAME --tail=100 | grep -i "tls\|handshake\|connection.*error" || print_success "No TLS/connection errors found"
else
    print_warning "Cannot check logs - no pod available"
fi

echo
print_info "7. Checking ConfigMap configuration..."

if kubectl get configmap allocation-service-config-dev > /dev/null 2>&1; then
    print_success "ConfigMap found"
    print_info "OTEL configuration in ConfigMap:"
    kubectl get configmap allocation-service-config-dev -o yaml | grep -A 20 -B 5 -i otel || print_warning "No OTEL configuration found in ConfigMap"
else
    print_error "ConfigMap 'allocation-service-config-dev' not found"
fi

echo
print_info "8. Suggested fixes based on diagnostics..."

echo
print_info "If you see TLS handshake errors, try these fixes:"
echo "1. Ensure OTEL_EXPORTER_OTLP_INSECURE=true is set"
echo "2. Verify the collector endpoint doesn't include 'https://' prefix"
echo "3. Check that the collector is configured for insecure connections"
echo

print_info "If connectivity fails, try these fixes:"
echo "1. Check if the monitoring namespace exists: kubectl get ns monitoring"
echo "2. Verify network policies allow traffic between namespaces"
echo "3. Try using the collector's ClusterIP directly instead of DNS name"
echo

print_info "To apply the updated configuration:"
echo "kubectl apply -f k8s/configmap-dev.yaml"
echo "kubectl rollout restart deployment/globeco-allocation-service"
echo

print_info "To monitor the fix:"
echo "kubectl logs -f deployment/globeco-allocation-service | grep -i otel"
echo

print_success "Diagnostics complete!"