#!/bin/bash
# Deploy test services to KIND cluster for kubefwd integration tests
# Usage: ./deploy-test-services.sh [manifest-directory]

set -e

MANIFEST_DIR="${1:-test/manifests}"

echo "========================================="
echo "Deploying test services"
echo "Manifest directory: $MANIFEST_DIR"
echo "========================================="

# Check if manifest directory exists
if [ ! -d "$MANIFEST_DIR" ]; then
  echo "ERROR: Manifest directory not found: $MANIFEST_DIR"
  exit 1
fi

# Check if there are any YAML files
if ! ls "$MANIFEST_DIR"/*.yaml >/dev/null 2>&1 && ! ls "$MANIFEST_DIR"/*.yml >/dev/null 2>&1; then
  echo "WARNING: No YAML files found in $MANIFEST_DIR"
  exit 0
fi

# Apply all manifests
echo ""
echo "Applying manifests..."
kubectl apply -f "$MANIFEST_DIR/" 2>&1 | grep -v "Warning: would violate PodSecurity" || true

echo ""
echo "Waiting for deployments to be ready (timeout: 120s)..."
kubectl wait --for=condition=available --timeout=120s deployment --all --all-namespaces 2>/dev/null || true

echo ""
echo "Waiting for pods to be ready (timeout: 120s)..."
kubectl wait --for=condition=ready --timeout=120s pods --all --all-namespaces 2>/dev/null || true

echo ""
echo "========================================="
echo "Deployment status"
echo "========================================="
kubectl get pods --all-namespaces

echo ""
echo "========================================="
echo "Services"
echo "========================================="
kubectl get services --all-namespaces

echo ""
echo "Deployment complete"
