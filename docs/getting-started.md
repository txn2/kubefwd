# Getting Started

This guide will help you install kubefwd and start forwarding Kubernetes services to your local machine.

## Prerequisites

Before using kubefwd, ensure you have:

- **kubectl configured** with access to a Kubernetes cluster
- **Root/sudo access** (required for modifying `/etc/hosts` and creating network interfaces)

Note: kubefwd reads your kubeconfig file (`~/.kube/config` or `KUBECONFIG` env var) and connects directly to the Kubernetes API—it does not invoke kubectl.

## Installation

### macOS

Using [Homebrew](https://brew.sh/):

```bash
brew install txn2/tap/kubefwd
```

To upgrade:

```bash
brew upgrade kubefwd
```

### Linux

Download pre-built packages from the [releases page](https://github.com/txn2/kubefwd/releases):

**Debian/Ubuntu (.deb):**
```bash
# Download the latest .deb from releases
sudo dpkg -i kubefwd_*.deb
```

**RHEL/CentOS/Fedora (.rpm):**
```bash
# Download the latest .rpm from releases
sudo rpm -i kubefwd_*.rpm
```

**Any Linux (tar.gz):**
```bash
# Download and extract
tar -xzf kubefwd_*.tar.gz
sudo mv kubefwd /usr/local/bin/
```

### Windows

Download the Windows executable from [GitHub Releases](https://github.com/txn2/kubefwd/releases/latest):

1. Download `kubefwd_Windows_x86_64.zip`
2. Extract `kubefwd.exe`
3. Add to your PATH or run directly

Run as Administrator (required for hosts file access).

### Docker

Run kubefwd in a privileged container:

```bash
docker run -it --rm --privileged --name kubefwd \
    -v "$HOME/.kube:/root/.kube" \
    txn2/kubefwd services -n my-namespace --tui
```

## Your First Forward

### Interactive Mode (Recommended)

Launch kubefwd with the TUI for real-time monitoring:

```bash
sudo -E kubefwd svc -n default --tui
```

You'll see an interactive interface showing:
- All services being forwarded
- Connection status and traffic metrics
- Real-time logs

Press `?` for help, `q` to quit.

### Classic Mode

For scripting or simpler output:

```bash
sudo -E kubefwd svc -n default
```

Press `Ctrl+C` to stop forwarding.

## What Happens When You Run kubefwd

1. **Service Discovery**: kubefwd queries your cluster for services in the specified namespace(s)

2. **IP Allocation**: Each service gets a unique loopback IP address (e.g., `127.1.27.1`, `127.1.27.2`)

3. **Hosts File Update**: Service names are added to `/etc/hosts` pointing to their assigned IPs:
   ```
   127.1.27.1    my-service
   127.1.27.2    another-service
   ```

4. **Port Forwarding**: Connections to each IP:port are forwarded to pods backing the service

5. **Ready to Use**: Access services by name just like in-cluster:
   ```bash
   curl http://my-service:8080
   mysql -h mysql-service -P 3306
   ```

## Important Notes

### Why sudo -E?

kubefwd requires root privileges to:
- Modify `/etc/hosts`
- Create network interface aliases
- Bind to ports below 1024

The `-E` flag preserves your environment variables, especially `KUBECONFIG`:

```bash
# Correct - preserves KUBECONFIG
sudo -E kubefwd svc -n default --tui

# Wrong - may fail to find cluster config
sudo kubefwd svc -n default --tui
```

### Cleanup

When kubefwd exits (via `q` or `Ctrl+C`), it automatically:
- Removes `/etc/hosts` entries it added
- Removes network interface aliases
- Closes all port forward connections

Your original `/etc/hosts` is backed up to `~/hosts.original`.

## Next Steps

- [TUI Guide](tui-guide.md) - Learn the interactive interface
- [Configuration](configuration.md) - All command line options
- [Advanced Usage](advanced-usage.md) - Multi-namespace, selectors, port mapping
