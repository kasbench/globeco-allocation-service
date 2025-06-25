#!/bin/bash

# GlobeCo Allocation Service Deployment Script
# Author: Noah Krieger (noah@kasbench.org)
# Version: 1.0.0

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
NAMESPACE="${NAMESPACE:-default}"
ENVIRONMENT="${ENVIRONMENT:-development}"
IMAGE_TAG="${IMAGE_TAG:-latest}"
TIMEOUT="${TIMEOUT:-300}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Help function
show_help() {
    cat << EOF
GlobeCo Allocation Service Deployment Script

Usage: $0 [OPTIONS] COMMAND

COMMANDS:
    deploy          Deploy the service
    verify          Verify deployment
    rollback        Rollback to previous version
    clean           Clean up resources
    logs            Show service logs
    status          Show deployment status

OPTIONS:
    -e, --environment   Environment (development|staging|production) [default: development]
    -n, --namespace     Kubernetes namespace [default: default]
    -t, --tag          Image tag [default: latest]
    -h, --help         Show this help

EXAMPLES:
    $0 deploy                                    # Deploy to development
    $0 -e production -t v1.0.1 deploy          # Deploy v1.0.1 to production
    $0 verify                                   # Verify current deployment
    $0 rollback                                 # Rollback to previous version

EOF
}

# Parse command line arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            -e|--environment)
                ENVIRONMENT="$2"
                shift 2
                ;;
            -n|--namespace)
                NAMESPACE="$2"
                shift 2
                ;;
            -t|--tag)
                IMAGE_TAG="$2"
                shift 2
                ;;
            -h|--help)
                show_help
                exit 0
                ;;
            deploy|verify|rollback|clean|logs|status)
                COMMAND="$1"
                shift
                ;;
            *)
                log_error "Unknown option: $1"
                show_help
                exit 1
                ;;
        esac
    done

    if [[ -z "${COMMAND:-}" ]]; then
        log_error "No command specified"
        show_help
        exit 1
    fi
}

# Validation functions
validate_environment() {
    case $ENVIRONMENT in
        development|staging|production)
            log_info "Deploying to environment: $ENVIRONMENT"
            ;;
        *)
            log_error "Invalid environment: $ENVIRONMENT"
            log_error "Valid environments: development, staging, production"
            exit 1
            ;;
    esac
}

validate_dependencies() {
    local missing_deps=()

    # Check required tools
    for tool in kubectl docker; do
        if ! command -v "$tool" &> /dev/null; then
            missing_deps+=("$tool")
        fi
    done

    if [[ ${#missing_deps[@]} -gt 0 ]]; then
        log_error "Missing required dependencies: ${missing_deps[*]}"
        exit 1
    fi

    # Check kubectl connectivity
    if ! kubectl cluster-info &> /dev/null; then
        log_error "Cannot connect to Kubernetes cluster"
        exit 1
    fi

    log_success "All dependencies validated"
}

# Build and push Docker image
build_image() {
    log_info "Building Docker image..."
    
    cd "$PROJECT_ROOT"
    
    # Build image with appropriate target
    local docker_target="production"
    if [[ "$ENVIRONMENT" == "development" ]]; then
        docker_target="development"
    fi
    
    docker build \
        --target "$docker_target" \
        --tag "globeco-allocation-service:$IMAGE_TAG" \
        --tag "globeco-allocation-service:latest" \
        .
    
    log_success "Docker image built successfully"
}

# Deploy to Kubernetes
deploy_service() {
    log_info "Deploying to Kubernetes..."
    
    cd "$PROJECT_ROOT"
    
    # Create namespace if it doesn't exist
    kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -
    
    # Set namespace context
    kubectl config set-context --current --namespace="$NAMESPACE"
    
    # Apply manifests in order
    local manifests=(
        "k8s/secret.yaml"
        "k8s/configmap.yaml"
        "k8s/rbac.yaml"
        "k8s/pvc.yaml"
        "k8s/deployment.yaml"
        "k8s/service.yaml"
    )
    
    # Add ingress for non-development environments
    if [[ "$ENVIRONMENT" != "development" ]]; then
        manifests+=("k8s/ingress.yaml")
    fi
    
    for manifest in "${manifests[@]}"; do
        if [[ -f "$manifest" ]]; then
            log_info "Applying $manifest..."
            kubectl apply -f "$manifest"
        else
            log_warning "Manifest not found: $manifest"
        fi
    done
    
    # Update image tag
    kubectl set image deployment/globeco-allocation-service \
        allocation-service="globeco-allocation-service:$IMAGE_TAG"
    
    # Use environment-specific configuration
    case $ENVIRONMENT in
        development)
            kubectl set env deployment/globeco-allocation-service \
                --from=configmap/allocation-service-config-dev
            ;;
        production)
            kubectl set env deployment/globeco-allocation-service \
                --from=configmap/allocation-service-config-prod
            kubectl scale deployment globeco-allocation-service --replicas=3
            ;;
        *)
            kubectl set env deployment/globeco-allocation-service \
                --from=configmap/allocation-service-config
            ;;
    esac
    
    log_success "Deployment manifests applied"
}

# Wait for deployment to be ready
wait_for_deployment() {
    log_info "Waiting for deployment to be ready..."
    
    if kubectl rollout status deployment/globeco-allocation-service --timeout="${TIMEOUT}s"; then
        log_success "Deployment is ready"
    else
        log_error "Deployment failed to become ready within ${TIMEOUT} seconds"
        return 1
    fi
}

# Verify deployment
verify_deployment() {
    log_info "Verifying deployment..."
    
    # Check pod status
    local pods
    pods=$(kubectl get pods -l app=globeco-allocation-service -o jsonpath='{.items[*].metadata.name}')
    
    if [[ -z "$pods" ]]; then
        log_error "No pods found for allocation service"
        return 1
    fi
    
    log_info "Found pods: $pods"
    
    # Check pod health
    for pod in $pods; do
        local status
        status=$(kubectl get pod "$pod" -o jsonpath='{.status.phase}')
        
        if [[ "$status" != "Running" ]]; then
            log_error "Pod $pod is not running (status: $status)"
            kubectl describe pod "$pod"
            return 1
        fi
        
        log_success "Pod $pod is running"
    done
    
    # Check service health
    log_info "Checking service health..."
    
    # Port forward and test health endpoint
    kubectl port-forward service/globeco-allocation-service 8089:8089 &
    local port_forward_pid=$!
    
    # Wait for port forward to be ready
    sleep 5
    
    local health_check_failed=false
    if ! curl -f http://localhost:8089/healthz; then
        log_error "Health check failed"
        health_check_failed=true
    else
        log_success "Health check passed"
    fi
    
    # Clean up port forward
    kill $port_forward_pid 2>/dev/null || true
    
    if [[ "$health_check_failed" == "true" ]]; then
        return 1
    fi
    
    # Check metrics endpoint
    log_info "Checking metrics endpoint..."
    kubectl port-forward service/globeco-allocation-service 8089:8089 &
    port_forward_pid=$!
    sleep 5
    
    if curl -f http://localhost:8089/metrics > /dev/null; then
        log_success "Metrics endpoint is accessible"
    else
        log_warning "Metrics endpoint is not accessible"
    fi
    
    kill $port_forward_pid 2>/dev/null || true
    
    log_success "Deployment verification completed"
}

# Rollback deployment
rollback_deployment() {
    log_info "Rolling back deployment..."
    
    if kubectl rollout undo deployment/globeco-allocation-service; then
        log_success "Rollback initiated"
        wait_for_deployment
    else
        log_error "Rollback failed"
        return 1
    fi
}

# Clean up resources
clean_resources() {
    log_warning "Cleaning up resources..."
    
    read -p "Are you sure you want to delete all resources? (y/N): " -n 1 -r
    echo
    
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        kubectl delete -f k8s/ --ignore-not-found=true
        log_success "Resources cleaned up"
    else
        log_info "Clean up cancelled"
    fi
}

# Show service logs
show_logs() {
    log_info "Showing service logs..."
    kubectl logs -f deployment/globeco-allocation-service
}

# Show deployment status
show_status() {
    log_info "Deployment status:"
    echo
    
    kubectl get deployment globeco-allocation-service -o wide
    echo
    
    kubectl get pods -l app=globeco-allocation-service -o wide
    echo
    
    kubectl get service globeco-allocation-service -o wide
    echo
    
    if kubectl get ingress globeco-allocation-service-ingress &> /dev/null; then
        kubectl get ingress globeco-allocation-service-ingress -o wide
    fi
}

# Main execution
main() {
    parse_args "$@"
    validate_environment
    
    case $COMMAND in
        deploy)
            validate_dependencies
            build_image
            deploy_service
            wait_for_deployment
            verify_deployment
            ;;
        verify)
            verify_deployment
            ;;
        rollback)
            rollback_deployment
            ;;
        clean)
            clean_resources
            ;;
        logs)
            show_logs
            ;;
        status)
            show_status
            ;;
        *)
            log_error "Unknown command: $COMMAND"
            show_help
            exit 1
            ;;
    esac
}

# Execute main function with all arguments
main "$@" 