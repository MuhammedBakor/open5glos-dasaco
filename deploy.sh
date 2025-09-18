#!/bin/bash

# 5GLOS Deployment Script
# This script builds and deploys 5GLOS to Kubernetes

set -e

# Configuration
DOCKER_REGISTRY=${DOCKER_REGISTRY:-"localhost:5000"}
VERSION=${VERSION:-"latest"}
NAMESPACE=${NAMESPACE:-"free5gc"}
APP_NAME="5glos"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."
    
    if ! command -v docker &> /dev/null; then
        log_error "Docker is not installed"
        exit 1
    fi
    
    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl is not installed"
        exit 1
    fi
    
    if ! kubectl cluster-info &> /dev/null; then
        log_error "kubectl is not connected to a cluster"
        exit 1
    fi
    
    log_info "Prerequisites check passed"
}

# Build Docker image
build_image() {
    log_info "Building Docker image..."
    docker build -t ${DOCKER_REGISTRY}/${APP_NAME}:${VERSION} .
    log_info "Docker image built successfully"
}

# Push Docker image
push_image() {
    log_info "Pushing Docker image to registry..."
    docker push ${DOCKER_REGISTRY}/${APP_NAME}:${VERSION}
    log_info "Docker image pushed successfully"
}

# Deploy to Kubernetes
deploy_k8s() {
    log_info "Deploying to Kubernetes..."
    
    # Create namespace if it doesn't exist
    kubectl create namespace ${NAMESPACE} --dry-run=client -o yaml | kubectl apply -f -
    
    # Apply all manifests
    kubectl apply -f deployments/k8s/
    
    log_info "Waiting for deployment to be ready..."
    kubectl wait --for=condition=available --timeout=300s deployment/open5glos -n ${NAMESPACE}
    
    log_info "Deployment completed successfully"
}

# Show deployment status
show_status() {
    log_info "Deployment Status:"
    echo
    kubectl get pods -n ${NAMESPACE} -l nf=open5glos
    echo
    kubectl get svc -n ${NAMESPACE} open5glos
    echo
    
    # Get NodePort details
    NODE_PORT=$(kubectl get svc -n ${NAMESPACE} open5glos -o jsonpath='{.spec.ports[?(@.name=="metrics")].nodePort}')
    if [ ! -z "$NODE_PORT" ]; then
        log_info "Metrics endpoint will be available on any node at port: $NODE_PORT"
        log_info "Example: curl http://<NODE_IP>:$NODE_PORT/metrics"
    fi
}

# Show usage
usage() {
    echo "Usage: $0 [OPTIONS] COMMAND"
    echo
    echo "Commands:"
    echo "  build    - Build Docker image only"
    echo "  push     - Build and push Docker image"
    echo "  deploy   - Build, push and deploy to Kubernetes"
    echo "  status   - Show deployment status"
    echo "  logs     - Show pod logs"
    echo "  cleanup  - Remove deployment from Kubernetes"
    echo
    echo "Options:"
    echo "  -r REGISTRY  - Docker registry (default: localhost:5000)"
    echo "  -v VERSION   - Image version (default: latest)"
    echo "  -n NAMESPACE - Kubernetes namespace (default: free5gc)"
    echo
    echo "Environment variables:"
    echo "  DOCKER_REGISTRY - Docker registry"
    echo "  VERSION         - Image version"
    echo "  NAMESPACE       - Kubernetes namespace"
}

# Show logs
show_logs() {
    log_info "Showing pod logs..."
    kubectl logs -n ${NAMESPACE} deployment/open5glos --tail=50 -f
}

# Cleanup deployment
cleanup() {
    log_warn "Removing deployment from Kubernetes..."
    kubectl delete -f deployments/k8s/ --ignore-not-found=true
    log_info "Cleanup completed"
}

# Parse command line arguments
while getopts "r:v:n:h" opt; do
    case $opt in
        r) DOCKER_REGISTRY="$OPTARG" ;;
        v) VERSION="$OPTARG" ;;
        n) NAMESPACE="$OPTARG" ;;
        h) usage; exit 0 ;;
        *) usage; exit 1 ;;
    esac
done

shift $((OPTIND-1))

# Main execution
case "${1:-}" in
    build)
        check_prerequisites
        build_image
        ;;
    push)
        check_prerequisites
        build_image
        push_image
        ;;
    deploy)
        check_prerequisites
        build_image
        push_image
        deploy_k8s
        show_status
        ;;
    status)
        show_status
        ;;
    logs)
        show_logs
        ;;
    cleanup)
        cleanup
        ;;
    *)
        usage
        exit 1
        ;;
esac
