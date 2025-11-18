#!/bin/bash
# Teardown KIND cluster used for kubefwd integration tests
# Usage: ./teardown-kind.sh

set -e

CLUSTER_NAME="${KIND_CLUSTER_NAME:-kubefwd-test}"

echo "========================================="
echo "Deleting KIND cluster: $CLUSTER_NAME"
echo "========================================="

# Delete cluster (|| true makes it idempotent)
if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
  kind delete cluster --name "$CLUSTER_NAME"
  echo "Cluster '$CLUSTER_NAME' deleted successfully"
else
  echo "Cluster '$CLUSTER_NAME' does not exist (already deleted)"
fi

echo ""
echo "========================================="
echo "Cleanup complete"
echo "========================================="
