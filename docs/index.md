# kubefwd Documentation

> Bulk Kubernetes port forwarding with an interactive TUI, unique IPs per service, and automatic reconnection.

![kubefwd TUI](images/tui-110-main-active.png)

## What is kubefwd?

kubefwd is a command-line tool that bulk-forwards Kubernetes services to your local machine. Unlike `kubectl port-forward`, kubefwd:

- Forwards **all services** in a namespace with one command
- Assigns **unique IP addresses** (127.x.x.x) to each service
- Updates `/etc/hosts` so you can use **service names** directly
- Provides an **interactive TUI** with real-time metrics
- **Auto-reconnects** when pods restart

## Quick Start

```bash
# Install
brew install txn2/tap/kubefwd

# Forward all services with the TUI
sudo -E kubefwd svc -n my-namespace --tui
```

Access services by name:

```bash
curl http://api-service:8080
mysql -h database -P 3306
```

## Documentation

### Getting Started

- [Installation & Quick Start](getting-started.md) - Get up and running

### Using kubefwd

- [TUI Guide](tui-guide.md) - Interactive interface, keyboard shortcuts
- [Configuration](configuration.md) - All command line flags and options
- [Advanced Usage](advanced-usage.md) - Multi-namespace, multi-cluster, Docker

### Reference

- [Troubleshooting](troubleshooting.md) - Common issues and solutions
- [Architecture](architecture.md) - How kubefwd works under the hood
- [Comparison](comparison.md) - kubefwd vs Telepresence, mirrord, Gefyra

## Key Features

### Interactive TUI

The Terminal User Interface provides:
- Real-time service status monitoring
- Traffic metrics with sparkline graphs
- Pod log streaming
- Keyboard-driven navigation

Press `?` for help, `q` to quit.

### Unique IP per Service

Each service gets its own loopback IP (127.1.27.x), eliminating port conflicts:

```
127.1.27.1    api-service
127.1.27.2    database
127.1.27.3    cache
```

Multiple services can use the same port (e.g., several databases on 3306).

### Automatic Reconnection

With `--tui` or `-a`, kubefwd automatically reconnects when:
- Pods are restarted
- Connections drop
- Services are updated

Uses exponential backoff (1s → 5min max).

### Headless Service Support

For headless services (ClusterIP: None), kubefwd forwards all pods:
- `service-name` → first pod
- `pod-name.service-name` → specific pods

Essential for StatefulSets and database clusters.

## Comparison with kubectl

| Feature | kubectl port-forward | kubefwd |
|---------|---------------------|---------|
| Services per command | One | All in namespace |
| IP allocation | localhost only | Unique IP per service |
| Port conflicts | Manual management | None |
| Service name resolution | Not supported | Automatic |
| Auto-reconnect | No | Yes |
| Monitoring | None | TUI with metrics |

## Installation Options

| Platform | Command |
|----------|---------|
| macOS | `brew install txn2/tap/kubefwd` |
| Linux | Download from [releases](https://github.com/txn2/kubefwd/releases) |
| Windows | `scoop install kubefwd` |
| Docker | `docker run txn2/kubefwd ...` |

See [Getting Started](getting-started.md) for details.

## Requirements

- kubectl configured with cluster access
- Root/sudo access (required for /etc/hosts and network interfaces)

## Source Code

kubefwd is open source under the Apache 2.0 license.

- GitHub: [https://github.com/txn2/kubefwd](https://github.com/txn2/kubefwd)
- Issues: [https://github.com/txn2/kubefwd/issues](https://github.com/txn2/kubefwd/issues)
