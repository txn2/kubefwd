# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

kubefwd is a popular command-line utility for bulk Kubernetes port forwarding, featured in "essential k8s developer tools" lists. It forwards multiple services from one or more namespaces, adding corresponding entries to `/etc/hosts` for local development.

**Key Differentiator from `kubectl port-forward`**: Each service gets its own unique loopback IP address (127.x.x.x), allowing multiple services to use the same port simultaneously (e.g., multiple databases on port 3306, or multiple web services on port 80). This mirrors how services work in the cluster, enabling truly cluster-like local development.

The tool automatically monitors service and pod lifecycle events, starting/stopping port forwards as services are created, deleted, or pods are rescheduled.

## Integration and Usage Patterns

kubefwd is commonly integrated into development workflows:

- **With Tilt**: Blog posts document using kubefwd with Tilt for automated local development setups
- **Development Environment Setup**: Teams use kubefwd to mirror production-like service topologies locally
- **Microservices Development**: Allows developers to run one service locally while accessing dependencies in the cluster via service names
- **Database Access**: Popular for forwarding multiple databases (MySQL, PostgreSQL, MongoDB) on their native ports without port conflicts

The typical workflow: `sudo -E kubefwd svc -n <namespace> --tui` runs in a dedicated terminal while developers work in their IDE, accessing cluster services as if running in-cluster.

## TUI (Terminal User Interface)

kubefwd includes an interactive TUI built with [Bubble Tea](https://github.com/charmbracelet/bubbletea), enabled with `--tui`:

```bash
sudo -E kubefwd svc -n default --tui
```

### TUI Features
- **Real-time service monitoring**: See all forwarded services with connection status
- **Traffic metrics**: Bytes in/out with sparkline graphs
- **Pod log streaming**: View container logs directly in the TUI
- **HTTP activity detection**: Monitor HTTP requests/responses flowing through forwards
- **Auto-reconnect**: Enabled by default in TUI mode, with exponential backoff (1s → 5min max)
- **Keyboard-driven**: Full keyboard navigation (j/k, Enter, Tab, /, ?, q)

### TUI Architecture

The TUI follows the Model-View-Update (MVU) pattern:

- **Event Bus** (`pkg/fwdtui/events`): Decoupled pub/sub communication between components
- **State Store** (`pkg/fwdtui/state`): Centralized, thread-safe state management for all forwards
- **Metrics Registry** (`pkg/fwdtui/metrics`): Traffic counters with atomic operations
- **Components** (`pkg/fwdtui/components`): UI models for services list, logs, detail view

### TUI Keyboard Shortcuts
- `j/k` or arrows: Navigate
- `Enter`: Open detail view
- `Tab`: Switch panels/tabs
- `/`: Filter services
- `r`: Reconnect errored services
- `b`: Toggle bandwidth columns
- `?`: Help overlay
- `q`: Quit

## Git Policy

**All commits require explicit human approval.** Do not run `git commit` unless the user explicitly asks you to commit. Always show the diff or summarize changes and wait for approval first.

## Build and Development Commands

### Building
```bash
# Build the project (uses goreleaser)
go build -o kubefwd ./cmd/kubefwd/kubefwd.go

# Build with version information
go build -ldflags "-X main.Version=dev" -o kubefwd ./cmd/kubefwd/kubefwd.go
```

### Testing
```bash
# Run all tests
go test ./...

# Run tests for a specific package
go test ./pkg/fwdport

# Run tests with verbose output
go test -v ./pkg/fwdport
```

### Running Locally
```bash
# Requires root/sudo for network interface management and /etc/hosts modification
sudo ./kubefwd svc -n <namespace>

# Use -E flag to preserve environment (especially KUBECONFIG)
sudo -E ./kubefwd svc -n <namespace>

# With interactive TUI (recommended for development)
sudo -E ./kubefwd svc -n <namespace> --tui
```

### Dependencies
```bash
# Download dependencies
go mod download

# Update dependencies
go mod tidy
```

### Debugging and Development

```bash
# Enable verbose logging
sudo -E ./kubefwd svc -n <namespace> -v

# Test with a single service using field selector
sudo -E ./kubefwd svc -n <namespace> -f metadata.name=<service-name>

# Test with label selector
sudo -E ./kubefwd svc -n <namespace> -l app=myapp

# Use IP reservation config for reproducible testing
sudo -E ./kubefwd svc -n <namespace> -z example.fwdconf.yml
```

The verbose flag (`-v`) enables debug-level logging (logrus.DebugLevel), which shows:
- Service registry operations
- Pod sync events
- IP allocation details
- Port forwarding lifecycle events

### Demo Environment

A comprehensive demo manifest is available for testing:

```bash
# Deploy 60 services across 2 namespaces
kubectl apply -f test/manifests/demo-microservices.yaml

# Forward all demo services with TUI
sudo -E ./kubefwd svc -n kft1,kft2 --tui

# Cleanup
kubectl delete -f test/manifests/demo-microservices.yaml
```

## Architecture

### Core Components Flow

1. **Entry Point** (`cmd/kubefwd/kubefwd.go`): CLI entry, delegates to services command
2. **Services Command** (`cmd/kubefwd/services/services.go`): Main orchestration logic
   - Validates cluster connectivity and RBAC permissions
   - Creates Kubernetes informers to watch Service events (Add/Delete/Update)
   - Spawns namespace watchers for each namespace×context combination
3. **Service Registry** (`pkg/fwdsvcregistry`): Central registry of all forwarded services
   - Thread-safe map of active ServiceFWD instances
   - Handles lifecycle (add, remove, shutdown all)
4. **Service Forwarding** (`pkg/fwdservice`): Per-service forwarding logic
   - Queries pods backing each service
   - Manages pod selection (single pod for normal services, all pods for headless)
   - Debounces pod sync operations to avoid hammering k8s API
   - Maintains map of active port forwards per pod
5. **Port Forwarding** (`pkg/fwdport`): Individual pod port forwarding
   - Creates SPDY connection to k8s API server
   - Manages port forward lifecycle with watch for pod deletion
   - Updates `/etc/hosts` file with service hostnames
6. **Network/IP Management** (`pkg/fwdnet`, `pkg/fwdIp`): Loopback interface management
   - Allocates unique 127.x.x.x IPs for each service
   - Manages IP aliases on loopback interface (macOS: lo0, Linux: lo)
   - Supports IP reservation via config file or CLI flags

### Key Architectural Patterns

**Event-Driven Architecture**: Uses Kubernetes informers to react to service/pod lifecycle events in real-time.

**Headless vs Normal Services**:
- Normal services forward the first available pod using the service name
- Headless services (ClusterIP: None) forward all pods, with first pod as service name and rest as pod-name.service-name

**Debouncing**: Uses 5-second debouncer for pod sync operations to handle rapid pod changes (e.g., rolling deployments) without excessive k8s API calls.

**Thread Safety**: Extensive use of mutexes for:
- Per-namespace IP allocation (`NamespaceIPLock`)
- Service registry access
- Hosts file modifications

**Shutdown Orchestration**: Clean shutdown cascade:
1. User signal → stopListenCh
2. Namespace watchers stop
3. Service registry shutdown
4. Individual pod port forwards stop
5. Hosts file cleanup and network alias removal

### Package Responsibilities

- `fwdservice`: Service-level forwarding orchestration and pod management
- `fwdport`: Individual pod port forwarding via k8s API
- `fwdsvcregistry`: Global service registry and lifecycle management
- `fwdnet`: Network interface management (IP aliasing)
- `fwdIp`: IP allocation logic and reservation handling
- `fwdhost`: Hosts file backup management
- `fwdcfg`: Kubernetes client configuration
- `fwdpub`: Publisher interface for output
- `fwdtui`: Terminal User Interface (Bubble Tea)
  - `fwdtui/components`: UI view models (services list, logs, detail, help, status bar)
  - `fwdtui/events`: Event bus with panic recovery for decoupled pub/sub
  - `fwdtui/state`: Thread-safe state store with RWMutex protection
  - `fwdtui/metrics`: Traffic metrics with atomic counters and rate calculation
  - `fwdtui/portforward`: Port forward wrapper that emits metrics events
- `utils`: Root permission checks (OS-specific)

## Important Implementation Details

### Root Permissions and Security

kubefwd requires superuser privileges for:
- Adding IP aliases to loopback interface
- Binding to low-numbered ports
- Modifying `/etc/hosts`

Always run with `sudo -E` to preserve environment variables (KUBECONFIG).

**Security Considerations**: kubefwd is considered a "powerful but potentially dangerous tool" because it:
- Modifies system-level network configuration
- Changes `/etc/hosts` which affects DNS resolution system-wide
- Requires root access which can impact system stability if misused
- Opens network connections to cluster services

The tool backs up the original hosts file to `~/hosts.original` before making modifications, but improper shutdown can leave the system in an inconsistent state.

### Hostname Generation
Hostnames follow pattern based on cluster and namespace indices:
- Namespace 0, Cluster 0: `service-name`
- Namespace >0: `service-name.namespace`
- Cluster >0: `service-name.context` or `service-name.namespace.context`

For headless services, pods get: `pod-name.service-name[.namespace][.context]`

### IP Allocation
Base IP is 127.1.x.x with incremental allocation. Can be customized via:
- `--fwd-conf` YAML config file (see `example.fwdconf.yml`)
- `--reserve` CLI flag for individual reservations

### Port Mapping
Supports port remapping with `-m` flag (e.g., `-m 80:8080` maps service port 80 to local 8080).

### UDP Limitation
UDP port forwarding is not supported (Kubernetes API limitation: kubernetes/kubernetes#47862).

## Code Organization

```
cmd/kubefwd/           # CLI entry point
  kubefwd.go          # Main entry, root command
  services/
    services.go       # Services subcommand, namespace watchers
pkg/
  fwdservice/         # Service forwarding logic
  fwdport/            # Pod port forwarding
  fwdsvcregistry/     # Service registry
  fwdnet/             # Network interface management
  fwdIp/              # IP allocation
  fwdhost/            # Hosts file operations
  fwdcfg/             # K8s config
  fwdpub/             # Publisher
  fwdtui/             # Terminal User Interface
    components/       # UI models (services, logs, detail, help)
    events/           # Event bus for decoupled communication
    state/            # Centralized state store
    metrics/          # Traffic metrics with atomic counters
    portforward/      # Port forward wrapper with metrics
  utils/              # Utilities (root check)
test/
  integration/        # Integration tests (require KIND cluster)
  manifests/          # Test Kubernetes manifests
  scripts/            # Test setup/teardown scripts
```

## Contribution Policy

**Public Contributions**: The project welcomes pull requests from the community for:
- Bug fixes
- Tests
- Documentation
- Stability and compatibility enhancements

**Feature Development**: New features are developed by select contributors only. Public feature PRs are discouraged due to past experience with contributions that introduced drastic codebase changes, were difficult to review and test, and added significant technical debt. This policy is documented in the README.

**Internal Development**: Maintainers and trusted contributors are welcome to develop new features. If you're working on this codebase with direct access, feature development is encouraged.

## Release Process

Uses GoReleaser (`.goreleaser.yml`):
- Builds for Linux, macOS, Windows (multiple architectures)
- Creates Docker images (Alpine and Ubuntu variants)
- Publishes to GitHub releases
- Updates Homebrew tap (txn2/homebrew-tap)
- Generates RPM, DEB, APK packages

The version is set via ldflags during build: `-ldflags "-X main.Version={{.Version}}"`

## Testing

### Unit Tests

Test coverage has improved significantly:

```bash
# Run all unit tests
go test ./...

# Run with coverage
go test -cover ./...

# Run specific package tests
go test -v ./pkg/fwdtui/events/...     # Event bus tests (100% coverage)
go test -v ./pkg/fwdtui/components/... # UI component tests
go test -v ./pkg/fwdtui/state/...      # State store tests
go test -v ./pkg/fwdport/...           # Port forwarding tests
```

### Integration Tests

Located in `test/integration/`, these require a KIND cluster:

```bash
# Setup KIND cluster
./test/scripts/setup-kind.sh
./test/scripts/deploy-test-services.sh test/manifests

# Run integration tests (requires sudo)
sudo -E go test -tags=integration -v ./test/integration/...

# Teardown
./test/scripts/teardown-kind.sh
```

Integration tests cover: basic forwarding, headless services, auto-reconnect, multi-namespace, port mapping, and cleanup verification.

### Areas for Additional Testing
- Hosts file race condition scenarios (Issue #74)
- Cleanup/restore logic for error conditions (Issue #5)
- Pod state filtering for evicted/completed pods (Issues #34, #114)

## Areas for Improvement

Based on known issues, these areas would benefit from development:

### Auto-Reconnect Enhancement
Auto-reconnect is now implemented (`-a` flag, enabled by default in TUI mode) with exponential backoff. However, edge cases may still exist:
- Very rapid pod cycling during rolling deployments
- Network partitions vs pod deletion detection
- Reconnection priority when multiple pods are available

### Hosts File Synchronization (Issues #74, #79)
The `HostFileWithLock` mutex in `pkg/fwdport` may not be sufficient for all race conditions. Consider:
- Batching hosts file updates
- Implementing retry logic for failed updates
- Adding verification that all services are actually written

### Graceful Shutdown (Issue #5)
Enhance the shutdown cascade to guarantee hosts file restoration even on signal interruption. Consider using `defer` more extensively or implementing a signal handler with guaranteed cleanup.

### Pod State Filtering (Issues #34, #114)
The pod filtering in `fwdservice.GetPodsForService()` should be more defensive:
- Handle additional pod phases gracefully
- Add better error handling for watch events with non-pod objects
- Consider ignoring evicted pods earlier in the pipeline

### Services Without Selectors (Issue #35)
Could potentially support endpoint-based services by querying Endpoints resources directly instead of relying on pod selectors.

## Known Limitations and Active Issues

Based on community feedback and GitHub issues, developers should be aware of:

### Architectural Limitations

**Services Without Selectors** (Issue #35): kubefwd does not support ClusterIP services without selectors (services backed by manually created Endpoints). The code assumes services have pod selectors and will skip services with empty selector strings.

**Auto-Reconnect Available** (Issue #21 resolved): Auto-reconnect is now implemented with the `-a` flag (enabled by default in TUI mode). When pods restart, kubefwd automatically re-establishes port forwards with exponential backoff (1s → 5min max).

**UDP Protocol Not Supported**: Kubernetes port-forwarding API limitation (kubernetes/kubernetes#47862).

### Race Conditions and Edge Cases

**Hosts File Race Condition** (Issue #74): When forwarding multiple services simultaneously, especially on Windows with Docker Desktop, race conditions can occur during hosts file updates, leading to DNS resolution errors.

**Hosts File Not Fully Restored** (Issue #5): On certain error conditions or unclean shutdowns (Ctrl-C timing), the original `/etc/hosts` may not be fully restored. Always check `~/hosts.original` backup.

**Incomplete Hosts File Updates** (Issue #79): Some services may be assigned IPs and be accessible but not appear in the hosts file. This appears related to timing in the hosts file update logic.

### Pod State Handling

**Evicted/Completed Pods** (Issues #34, #114): Starting from v1.11.0, interface conversion errors occur with evicted pods or completed jobs. The code in `pkg/fwdservice` filters for `PodPending` and `PodRunning` states, but watch events for other states can cause errors.

**Pod Status Assumptions**: The code assumes pods are either Pending, Running, Succeeded, Failed, or Unknown. Edge cases with custom pod conditions may not be handled gracefully.

## Common Issues and Troubleshooting

**Permission Errors**: Always use `sudo -E` to preserve KUBECONFIG.

**IP/Port Conflicts**: Use IP reservations (`-r` or `-z`) to avoid conflicts.

**Stale /etc/hosts Entries**: Original hosts file backed up to `~/hosts.original`. Check this file if DNS resolution is broken after kubefwd exits.

**Pod Watch Failures**: Check RBAC permissions (list/get/watch pods, get services).

**Connection Refused (localhost:8080)**: This usually means kubectl is not properly configured or KUBECONFIG was not preserved (missing `-E` flag with sudo).
