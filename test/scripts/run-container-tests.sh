#!/bin/bash
# Run integration tests using testcontainers (no sudo required on host!)
# This script runs kubefwd inside a privileged Docker container

set -e

CLUSTER_NAME="${KIND_CLUSTER_NAME:-kubefwd-test}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "========================================="
echo "Container-Based Integration Tests"
echo "========================================="
echo "Project: kubefwd"
echo "Cluster: $CLUSTER_NAME"
echo "Mode: Testcontainers (no sudo required!)"
echo "========================================="
echo ""

# Check Docker is available
if ! docker info > /dev/null 2>&1; then
    echo "ERROR: Docker is not running or not accessible"
    echo "Please start Docker Desktop or Docker daemon"
    exit 1
fi
echo "✓ Docker is running"

# Check if KIND cluster exists
if ! kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
    echo ""
    echo "KIND cluster '$CLUSTER_NAME' not found"
    echo "Creating cluster..."
    "$SCRIPT_DIR/setup-kind.sh"
    echo ""
fi
echo "✓ KIND cluster '$CLUSTER_NAME' exists"

# Check if test services are deployed
if ! kubectl get namespace kf-a > /dev/null 2>&1; then
    echo ""
    echo "Test services not deployed"
    echo "Deploying test services..."
    "$SCRIPT_DIR/deploy-test-services.sh" "$PROJECT_ROOT/test/manifests"
    echo ""
fi
echo "✓ Test services deployed"

# Ensure KIND network exists
if ! docker network inspect kind > /dev/null 2>&1; then
    echo ""
    echo "Creating KIND Docker network..."
    docker network create kind || true
fi
echo "✓ KIND Docker network exists"

echo ""
echo "========================================="
echo "Running Container-Based Tests"
echo "========================================="
echo ""
echo "NOTE: Tests run inside a privileged Docker container"
echo "      No sudo required on your host machine!"
echo ""

# Build kubefwd first (optional, container will build it too)
echo "Building kubefwd binary..."
cd "$PROJECT_ROOT"
go build -o kubefwd ./cmd/kubefwd/kubefwd.go
echo "✓ kubefwd binary built"

echo ""
echo "Starting testcontainer-based tests..."
echo ""

# Run container-based tests
# These tests will create privileged containers automatically
if [ -n "$1" ]; then
    # Run specific test
    TEST_NAME="$1"
    echo "Running specific test: $TEST_NAME"
    go test -tags=integration -v ./test/integration/ -run "TestContainer${TEST_NAME}"
else
    # Run all container-based tests
    echo "Running all container-based tests..."
    go test -tags=integration -v ./test/integration/ -run TestContainer
fi

EXIT_CODE=$?

echo ""
echo "========================================="
if [ $EXIT_CODE -eq 0 ]; then
    echo "✓ All tests passed!"
    echo "========================================="
else
    echo "✗ Tests failed"
    echo "========================================="
    echo ""
    echo "Troubleshooting:"
    echo "  - Check Docker is running: docker info"
    echo "  - Check KIND cluster: kubectl get nodes"
    echo "  - Check test services: kubectl get pods -A"
    echo "  - View container logs for details"
fi

echo ""
echo "Cleanup:"
echo "  - KIND cluster: ./test/scripts/teardown-kind.sh"
echo "  - Docker network: docker network rm kind"
echo ""

exit $EXIT_CODE
