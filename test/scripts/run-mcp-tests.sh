#!/bin/bash
#
# run-mcp-tests.sh - Run MCP live integration tests
#
# This script:
#   1. Deploys the demo-microservices.yaml manifest
#   2. Waits for pods to be ready
#   3. Starts kubefwd with --api flag
#   4. Runs the live MCP tests
#   5. Cleans up
#
# Usage:
#   ./test/scripts/run-mcp-tests.sh
#
# Requirements:
#   - kubectl configured with cluster access
#   - sudo access (for kubefwd)
#   - Go toolchain installed
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
MANIFEST_PATH="$PROJECT_ROOT/test/manifests/demo-microservices.yaml"
KUBEFWD_BINARY="$PROJECT_ROOT/kubefwd"
KUBEFWD_PID=""
API_URL="http://localhost:8080"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

cleanup() {
    log_info "Cleaning up..."

    # Stop kubefwd if running
    if [ -n "$KUBEFWD_PID" ] && kill -0 "$KUBEFWD_PID" 2>/dev/null; then
        log_info "Stopping kubefwd (PID: $KUBEFWD_PID)..."
        sudo kill "$KUBEFWD_PID" 2>/dev/null || true
        sleep 2
    fi

    # Optionally clean up manifests (commented out by default to preserve state)
    # log_info "Removing demo-microservices..."
    # kubectl delete -f "$MANIFEST_PATH" --ignore-not-found=true

    log_info "Cleanup complete"
}

trap cleanup EXIT

check_prerequisites() {
    log_info "Checking prerequisites..."

    # Check kubectl
    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl not found. Please install kubectl."
        exit 1
    fi

    # Check cluster access
    if ! kubectl cluster-info &> /dev/null; then
        log_error "Cannot connect to Kubernetes cluster. Please check your kubeconfig."
        exit 1
    fi

    # Check kubefwd binary
    if [ ! -f "$KUBEFWD_BINARY" ]; then
        log_info "kubefwd binary not found, building..."
        (cd "$PROJECT_ROOT" && go build -o kubefwd ./cmd/kubefwd/kubefwd.go)
    fi

    # Check manifest
    if [ ! -f "$MANIFEST_PATH" ]; then
        log_error "Demo manifest not found: $MANIFEST_PATH"
        exit 1
    fi

    log_success "Prerequisites check passed"
}

deploy_manifests() {
    log_info "Deploying demo-microservices..."

    kubectl apply -f "$MANIFEST_PATH"

    log_info "Waiting for pods to be ready in kft1..."
    kubectl wait --for=condition=ready pod -l purpose=kubefwd-testing -n kft1 --timeout=120s || {
        log_warn "Some pods in kft1 may not be ready yet"
    }

    log_info "Waiting for pods to be ready in kft2..."
    kubectl wait --for=condition=ready pod -l purpose=kubefwd-testing -n kft2 --timeout=120s || {
        log_warn "Some pods in kft2 may not be ready yet"
    }

    # Show pod status
    log_info "Pod status:"
    kubectl get pods -n kft1 --no-headers | head -5
    echo "  ..."
    kubectl get pods -n kft2 --no-headers | head -5
    echo "  ..."

    log_success "Demo manifests deployed"
}

start_kubefwd() {
    log_info "Starting kubefwd with --api flag..."

    # Kill any existing kubefwd
    sudo pkill -f "kubefwd svc" 2>/dev/null || true
    sleep 1

    # Start kubefwd in background
    sudo -E "$KUBEFWD_BINARY" svc -n kft1 --api &
    KUBEFWD_PID=$!

    log_info "kubefwd started (PID: $KUBEFWD_PID)"

    # Wait for API to be ready
    log_info "Waiting for API to be ready..."
    local retries=30
    while [ $retries -gt 0 ]; do
        if curl -s "$API_URL/v1/health" > /dev/null 2>&1; then
            log_success "API is ready"
            return 0
        fi
        retries=$((retries - 1))
        sleep 1
    done

    log_error "API did not become ready in time"
    exit 1
}

wait_for_services() {
    log_info "Waiting for services to be forwarded..."

    local retries=30
    while [ $retries -gt 0 ]; do
        local count=$(curl -s "$API_URL/v1/services" | jq -r '.data.summary.totalServices // 0')
        if [ "$count" -ge 20 ]; then
            log_success "$count services are being forwarded"
            return 0
        fi
        log_info "Currently forwarding $count services, waiting..."
        retries=$((retries - 1))
        sleep 2
    done

    log_warn "Not all services may be ready, proceeding anyway"
}

run_tests() {
    log_info "Running live MCP tests..."

    cd "$PROJECT_ROOT"

    # Run tests with verbose output
    KUBEFWD_API_URL="$API_URL" go test ./test/mcp/... -v -tags=live -timeout=5m

    local exit_code=$?
    if [ $exit_code -eq 0 ]; then
        log_success "All tests passed!"
    else
        log_error "Some tests failed (exit code: $exit_code)"
    fi

    return $exit_code
}

run_quick_test() {
    log_info "Running quick connectivity test..."

    # Just check if we can reach a service
    if curl -s "http://api-gateway.kft1/" > /dev/null 2>&1; then
        log_success "HTTP connectivity to api-gateway.kft1 confirmed"
    else
        log_warn "Could not reach api-gateway.kft1 - tests may fail"
    fi
}

main() {
    echo ""
    echo "========================================"
    echo "  kubefwd MCP Live Integration Tests"
    echo "========================================"
    echo ""

    check_prerequisites
    deploy_manifests
    start_kubefwd
    wait_for_services
    run_quick_test
    run_tests

    echo ""
    log_success "MCP test run complete!"
    echo ""
}

# Allow running specific steps
case "${1:-all}" in
    deploy)
        check_prerequisites
        deploy_manifests
        ;;
    start)
        check_prerequisites
        start_kubefwd
        wait_for_services
        ;;
    test)
        run_tests
        ;;
    all)
        main
        ;;
    *)
        echo "Usage: $0 [deploy|start|test|all]"
        exit 1
        ;;
esac
