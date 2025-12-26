# kubefwd Integration Tests

This directory contains end-to-end integration tests for kubefwd. These tests run kubefwd against a real Kubernetes cluster (KIND) and verify actual port forwarding, /etc/hosts modifications, and network interface management.

## Overview

**Test Count:** 28 integration tests across 7 test files
**Test Framework:** Go testing with build tags
**Cluster:** KIND (Kubernetes IN Docker)
**Requirements:** Docker, KIND, kubectl, sudo

## Directory Structure

```
test/
├── integration/                    # Integration test files
│   ├── helpers.go                 # Shared test utilities
│   ├── container_helpers.go       # Container-based test utilities
│   ├── forwarding_test.go         # Basic forwarding (5 tests)
│   ├── headless_test.go           # Headless services (4 tests)
│   ├── autoreconnect_test.go      # Auto-reconnect (4 tests)
│   ├── multinamespace_test.go     # Multi-namespace (3 tests)
│   ├── portmapping_test.go        # Port mapping (3 tests)
│   ├── cleanup_test.go            # Cleanup verification (4 tests)
│   └── container_forwarding_test.go # Container-based tests (5 tests)
├── scripts/                        # Cluster management scripts
│   ├── setup-kind.sh              # Create KIND cluster
│   ├── teardown-kind.sh           # Delete KIND cluster
│   ├── deploy-test-services.sh    # Deploy test workloads
│   ├── wait-for-ready.sh          # Wait for pods ready
│   ├── verify-cleanup.sh          # Verify cleanup complete
│   └── run-container-tests.sh     # Run tests in Docker container
├── manifests/                      # Kubernetes test manifests
│   ├── demo-microservices.yaml    # Comprehensive demo (30 pods, 60 services)
│   └── simple-service.yaml        # Simple nginx service
└── README.md                       # This file
```

## Prerequisites

### Required Tools

1. **Docker**: KIND runs Kubernetes in Docker containers
   ```bash
   docker --version  # Verify Docker is installed
   ```

2. **KIND (Kubernetes IN Docker)**: v0.20.0+
   ```bash
   # Install KIND
   go install sigs.k8s.io/kind@latest

   # Verify installation
   kind version
   ```

3. **kubectl**: v1.27.3+
   ```bash
   # macOS
   brew install kubectl

   # Verify installation
   kubectl version --client
   ```

4. **Go**: 1.21+ (already required for kubefwd development)
   ```bash
   go version
   ```

5. **sudo**: Required for kubefwd (network interfaces, /etc/hosts)
   - macOS/Linux: Built-in

### Build kubefwd Binary

Before running tests, build the kubefwd binary:

```bash
cd /path/to/kubefwd
go build -o kubefwd ./cmd/kubefwd/kubefwd.go
```

The integration tests will look for `../../kubefwd` relative to the test files.

## Quick Start

### Option 1: Container-Based Tests (NO SUDO REQUIRED!) ⭐

**Recommended for most users** - Tests run inside Docker containers with root access.

```bash
# One command - no sudo needed!
./test/scripts/run-container-tests.sh

# Run specific test
./test/scripts/run-container-tests.sh ForwardSingleService
```

Container-based tests run kubefwd with root privileges inside a Docker container, eliminating the need for sudo on your host machine.

### Option 2: Traditional Tests (Requires Sudo)

**For debugging host-specific issues** - Tests run directly on your machine.

```bash
# 1. Setup KIND Cluster
./test/scripts/setup-kind.sh
./test/scripts/deploy-test-services.sh test/manifests

# 2. Build kubefwd
go build -o kubefwd ./cmd/kubefwd/kubefwd.go

# 3. Run tests with sudo
sudo -E go test -tags=integration -v ./test/integration/...

# Run specific test
sudo -E go test -tags=integration -v ./test/integration/ -run TestForwardSingleService

# Run with race detector
sudo -E go test -race -tags=integration -v ./test/integration/...
```

### 3. Cleanup

```bash
# Verify kubefwd cleaned up properly
./test/scripts/verify-cleanup.sh

# Delete KIND cluster when done
./test/scripts/teardown-kind.sh
```

## Running Tests

### Container-Based Tests (No Sudo) ⭐ NEW

```bash
# From project root - no sudo required!
./test/scripts/run-container-tests.sh

# Or use go test directly
go test -tags=integration -v ./test/integration/ -run TestContainer
```

### Traditional Tests (Requires Sudo)

```bash
# From project root
sudo -E go test -tags=integration -v ./test/integration/...
```

### Specific Test Suites

```bash
# Basic forwarding tests
sudo -E go test -tags=integration -v ./test/integration/ -run TestForward

# Headless service tests
sudo -E go test -tags=integration -v ./test/integration/ -run TestHeadless

# Auto-reconnect tests
sudo -E go test -tags=integration -v ./test/integration/ -run TestPod

# Multi-namespace tests
sudo -E go test -tags=integration -v ./test/integration/ -run TestNamespace

# Port mapping tests
sudo -E go test -tags=integration -v ./test/integration/ -run TestPortMapping

# Cleanup tests (important!)
sudo -E go test -tags=integration -v ./test/integration/ -run TestCleanup
```

### Individual Tests

```bash
# Single test
sudo -E go test -tags=integration -v ./test/integration/ -run TestForwardSingleService

# With race detector
sudo -E go test -race -tags=integration -v ./test/integration/ -run TestForwardSingleService
```

## Important Notes

### Sudo Requirement

Integration tests **require sudo** because kubefwd needs root privileges for:
- Modifying `/etc/hosts`
- Adding IP aliases to loopback interface (lo0 on macOS, lo on Linux)
- Binding to low-numbered ports

**Always use `-E` flag with sudo** to preserve environment variables (especially KUBECONFIG):

```bash
sudo -E go test -tags=integration ./test/integration/...
```

### Build Tags

Tests use the `// +build integration` tag to separate them from unit tests:

- **Unit tests**: `go test ./...` (fast, no dependencies)
- **Integration tests**: `go test -tags=integration ./test/integration/...` (slow, requires cluster)

This ensures integration tests don't run accidentally during normal development.

### Test Duration

Integration tests are slower than unit tests:
- **Single test**: 10-30 seconds
- **Full suite**: 5-10 minutes
- **With race detector**: 10-15 minutes

Plan accordingly and consider running specific test suites during development.

## Test Scenarios

### 1. Basic Forwarding (forwarding_test.go)

- `TestForwardSingleService`: Forward one service, verify HTTP connectivity
- `TestForwardMultipleServices`: Forward 3-5 services simultaneously
- `TestForwardWithCustomDomain`: Test domain suffix functionality (-d flag)
- `TestForwardMultipleNamespaces`: Forward from 2+ namespaces
- `TestServiceAccessibility`: Verify consistent HTTP accessibility

### 2. Headless Services (headless_test.go)

- `TestForwardHeadlessService`: Forward headless service (ClusterIP: None)
- `TestHeadlessAllPodsForwarded`: Verify all pods accessible
- `TestHeadlessPodHostnames`: Verify pod-name.service-name hostnames
- `TestHeadlessStatefulSet`: Test StatefulSet with headless service

### 3. Auto-Reconnect (autoreconnect_test.go)

- `TestPodDeletionReconnect`: Delete pod, verify reconnection to new pod
- `TestDeploymentScale`: Scale deployment, verify pod selection updates
- `TestRollingUpdate`: Perform rolling update, verify seamless transition
- `TestPodRestartReconnect`: Restart pod, verify reconnection

### 4. Multi-Namespace (multinamespace_test.go)

- `TestMultiNamespaceForwarding`: Forward from kf-a and kf-b namespaces
- `TestNamespaceIsolation`: Verify different IPs per namespace
- `TestNamespaceHostnames`: Verify hostname patterns (service.namespace)

### 5. Port Mapping (portmapping_test.go)

- `TestPortMapping`: Use `-m 80:8080` flag, verify mapping
- `TestMultiplePortMappings`: Multiple port mappings for one service
- `TestPortMappingConflicts`: Verify error handling

### 6. Cleanup (cleanup_test.go)

**CRITICAL TESTS** - Verify kubefwd doesn't pollute the system:

- `TestGracefulShutdown`: Send SIGTERM, verify clean shutdown
- `TestHostsFileRestore`: Verify /etc/hosts restored to original
- `TestNetworkAliasRemoval`: Verify 127.x.x.x IPs removed from loopback
- `TestNoLeakedProcesses`: Verify no kubefwd processes remain

### 7. Container-Based Tests (container_forwarding_test.go)

**No sudo required** - Tests run inside Docker containers:

- `TestContainerForwardSingleService`: Forward one service in container
- `TestContainerForwardMultipleServices`: Forward multiple services
- `TestContainerHeadlessService`: Headless service in container
- `TestContainerCleanup`: Verify container cleanup
- `TestContainerAllForwardingTests`: Comprehensive container test suite

## Troubleshooting

### KIND Cluster Not Found

```bash
Error: KIND cluster not available

Solution:
./test/scripts/setup-kind.sh
```

### Pods Not Ready

```bash
Error: pods not ready

Solution:
kubectl get pods --all-namespaces
./test/scripts/wait-for-ready.sh <namespace>

# If needed, delete and recreate cluster
./test/scripts/teardown-kind.sh
./test/scripts/setup-kind.sh
./test/scripts/deploy-test-services.sh test/manifests
```

### Permission Denied

```bash
Error: permission denied

Solution:
Use sudo -E to run tests:
sudo -E go test -tags=integration ./test/integration/...
```

### Tests Skip with "Test requires root/sudo privileges"

This is expected if running without sudo. Integration tests require root.

```bash
# Don't run like this:
go test -tags=integration ./test/integration/...  # Will skip tests

# Run like this:
sudo -E go test -tags=integration ./test/integration/...
```

### kubefwd Processes Remain After Tests

```bash
# Check for leaked processes
./test/scripts/verify-cleanup.sh

# Manual cleanup if needed
sudo pkill -f kubefwd

# Check /etc/hosts
sudo cat /etc/hosts | grep -E "(127\.1\.|kubefwd)"

# Restore from backup if needed
sudo cp ~/hosts.original /etc/hosts

# Remove IP aliases (macOS)
sudo ifconfig lo0 | grep 127.1. | awk '{print $2}' | xargs -I {} sudo ifconfig lo0 -alias {}

# Remove IP aliases (Linux)
sudo ip addr show lo | grep 127.1. | awk '{print $2}' | xargs -I {} sudo ip addr del {} dev lo
```

### Connection Refused Errors

```bash
Error: connection refused when trying to connect to forwarded service

Troubleshooting:
1. Verify pod is running:
   kubectl get pods -n <namespace>

2. Verify service exists:
   kubectl get svc -n <namespace>

3. Check kubefwd logs in test output

4. Verify /etc/hosts entry:
   cat /etc/hosts | grep <service-name>

5. Increase wait time in test (may need longer for slow systems)
```

### Auto-Reconnect Tests Failing

Note: Auto-reconnect functionality may require manual intervention in some kubefwd versions. These tests document expected behavior and may fail if auto-reconnect isn't fully implemented. Check test output for "Note: Auto-reconnect may require manual intervention" messages.

## Environment Variables

### KUBEFWD_BIN

Override the kubefwd binary location:

```bash
export KUBEFWD_BIN=/path/to/kubefwd
sudo -E go test -tags=integration ./test/integration/...
```

Default: `../../kubefwd` (relative to test files)

### SKIP_SUDO_TESTS

Skip tests that require sudo (not recommended, tests won't run):

```bash
SKIP_SUDO_TESTS=1 go test -tags=integration ./test/integration/...
```

### KIND_CLUSTER_NAME

Use a custom KIND cluster name:

```bash
export KIND_CLUSTER_NAME=my-test-cluster
./test/scripts/setup-kind.sh
```

Default: `kubefwd-test`

### KIND_K8S_VERSION

Use a specific Kubernetes version:

```bash
export KIND_K8S_VERSION=v1.28.0
./test/scripts/setup-kind.sh
```

Default: `v1.27.3`

## Best Practices

### 1. Clean State Between Test Runs

Always verify cleanup between test runs:

```bash
./test/scripts/verify-cleanup.sh
```

### 2. Run Cleanup Tests Frequently

The cleanup tests are critical for ensuring kubefwd doesn't pollute your system:

```bash
sudo -E go test -tags=integration -v ./test/integration/ -run TestCleanup
```

### 3. Use Race Detector

Always run with race detector before committing:

```bash
sudo -E go test -race -tags=integration ./test/integration/...
```

### 4. Monitor System State

During test development, monitor:

```bash
# Watch /etc/hosts
watch -n 1 "cat /etc/hosts | tail -20"

# Watch loopback aliases (macOS)
watch -n 1 "ifconfig lo0 | grep 127.1"

# Watch loopback aliases (Linux)
watch -n 1 "ip addr show lo | grep 127.1"

# Watch kubefwd processes
watch -n 1 "pgrep -af kubefwd"
```

### 5. Incremental Development

When adding new tests:
1. Start with a simple test
2. Run it multiple times to verify reliability
3. Check cleanup thoroughly
4. Add more complex scenarios

## Writing New Tests

### Template

```go
// +build integration

package integration

import (
    "testing"
    "time"
)

func TestMyNewFeature(t *testing.T) {
    requiresSudo(t)
    requiresKindCluster(t)

    // Start kubefwd
    cmd := startKubefwd(t, "svc", "-n", "test-simple", "-v")
    defer stopKubefwd(t, cmd)

    // Wait for forwarding
    time.Sleep(10 * time.Second)

    // Your test logic here

    // Verify hosts entry
    if !verifyHostsEntry(t, "nginx") {
        t.Error("Service not found in /etc/hosts")
    }

    // Test HTTP connectivity
    resp, err := httpGet("http://nginx:80/", 5, 2*time.Second)
    if err != nil {
        t.Fatalf("Failed to connect: %v", err)
    }
    defer resp.Body.Close()

    // Assertions
    if resp.StatusCode != 200 {
        t.Errorf("Expected 200, got %d", resp.StatusCode)
    }
}
```

### Guidelines

1. Always use `requiresSudo(t)` and `requiresKindCluster(t)`
2. Always defer `stopKubefwd(t, cmd)`
3. Use generous timeouts (services take time to forward)
4. Check /etc/hosts entries before testing connectivity
5. Use retry logic for HTTP requests (`httpGet` helper)
6. Log progress with `t.Log()` for debugging
7. Clean up resources in `defer` statements

## CI/CD Integration

Currently, integration tests are designed for local execution. CI/CD integration (GitHub Actions) will be added in Phase 5.

Expected CI workflow:
1. Setup GitHub Actions runner
2. Install Docker, KIND, kubectl
3. Create KIND cluster
4. Deploy test services
5. Build kubefwd
6. Run integration tests with sudo
7. Verify cleanup
8. Teardown cluster

## Support

For issues with integration tests:
1. Check this README
2. Review test output logs
3. Check /etc/hosts and network aliases
4. Verify KIND cluster is healthy: `kubectl get nodes`
5. Report issues at: https://github.com/txn2/kubefwd/issues

## Summary

- **28 integration tests** covering all major kubefwd features
- **Container-based option** - no sudo required on host
- **Traditional option** - requires sudo for host-level testing
- **Uses KIND** - real Kubernetes cluster in Docker
- **Build tags** - won't run with normal `go test`
- **Cleanup critical** - verify system state after tests

Happy testing!
