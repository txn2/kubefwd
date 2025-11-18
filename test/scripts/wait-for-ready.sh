#!/bin/bash
# Wait for pods in a specific namespace to be ready
# Usage: ./wait-for-ready.sh [namespace] [timeout-seconds]

set -e

NAMESPACE="${1:-default}"
TIMEOUT="${2:-120}"

echo "========================================="
echo "Waiting for pods in namespace: $NAMESPACE"
echo "Timeout: ${TIMEOUT}s"
echo "========================================="

echo ""
echo "Current pod status:"
kubectl get pods -n "$NAMESPACE" 2>/dev/null || echo "No pods found in namespace '$NAMESPACE'"

echo ""
echo "Waiting for pods to be ready..."
kubectl wait --for=condition=ready \
  --timeout="${TIMEOUT}s" \
  pods --all \
  -n "$NAMESPACE" 2>/dev/null || {
    echo "WARNING: Some pods may not be ready within timeout"
  }

echo ""
echo "========================================="
echo "Final pod status in namespace: $NAMESPACE"
echo "========================================="
kubectl get pods -n "$NAMESPACE"

echo ""
echo "Pods ready in namespace: $NAMESPACE"
