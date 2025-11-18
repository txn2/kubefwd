#!/bin/bash
# Setup KIND cluster for kubefwd integration tests
# Usage: ./setup-kind.sh

set -e

CLUSTER_NAME="${KIND_CLUSTER_NAME:-kubefwd-test}"
K8S_VERSION="${KIND_K8S_VERSION:-v1.27.3}"

echo "========================================="
echo "Creating KIND cluster: $CLUSTER_NAME"
echo "Kubernetes version: $K8S_VERSION"
echo "========================================="

# Check if cluster already exists
if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
  echo "Cluster '$CLUSTER_NAME' already exists"
  kubectl cluster-info --context "kind-$CLUSTER_NAME"
  echo "Using existing cluster"
  exit 0
fi

# Create the cluster
kind create cluster \
  --name "$CLUSTER_NAME" \
  --image "kindest/node:$K8S_VERSION" \
  --wait 60s

# Verify cluster is ready
kubectl cluster-info --context "kind-$CLUSTER_NAME"

echo ""
echo "========================================="
echo "KIND cluster ready: $CLUSTER_NAME"
echo "========================================="
echo "Context: kind-$CLUSTER_NAME"
echo ""
echo "Next steps:"
echo "  1. Deploy test services: ./deploy-test-services.sh test/manifests"
echo "  2. Run integration tests: sudo -E go test -tags=integration -v ./test/integration/..."
echo ""
