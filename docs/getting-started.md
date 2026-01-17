# Getting Started

This guide covers installation and your first steps with kubefwd. By the end, you'll have cluster services accessible locally by name.

## Prerequisites

- **kubectl configured** with access to a Kubernetes cluster
- **Root/sudo access** (required for `/etc/hosts` and network interface modifications)

kubefwd reads your kubeconfig (`~/.kube/config` or `KUBECONFIG` environment variable) and connects directly to the Kubernetes API. It does not invoke kubectl.

---

## Installation

=== "macOS"

    Using [Homebrew](https://brew.sh/):

    ```bash
    brew install kubefwd
    ```

    To upgrade:

    ```bash
    brew update && brew upgrade kubefwd
    ```

=== "Windows"

    Using [winget](https://learn.microsoft.com/en-us/windows/package-manager/winget/):

    ```powershell
    winget install txn2.kubefwd
    ```

    Or using [Scoop](https://scoop.sh/):

    ```powershell
    scoop install kubefwd
    ```

    To upgrade:

    ```powershell
    winget upgrade txn2.kubefwd
    # or
    scoop update kubefwd
    ```

    !!! note "Run as Administrator"
        kubefwd requires elevated privileges on Windows to modify the hosts file. Right-click your terminal and select "Run as Administrator".

=== "Linux (Debian/Ubuntu)"

    Download the `.deb` package from [GitHub Releases](https://github.com/txn2/kubefwd/releases/latest):

    ```bash
    # Install
    sudo dpkg -i kubefwd_*_amd64.deb

    # Upgrade (same command)
    sudo dpkg -i kubefwd_*_amd64.deb
    ```

=== "Linux (RHEL/Fedora)"

    Download the `.rpm` package from [GitHub Releases](https://github.com/txn2/kubefwd/releases/latest):

    ```bash
    # Install
    sudo rpm -i kubefwd_*_amd64.rpm

    # Upgrade
    sudo rpm -U kubefwd_*_amd64.rpm
    ```

=== "Linux (Manual)"

    Download and extract the tarball from [GitHub Releases](https://github.com/txn2/kubefwd/releases/latest):

    ```bash
    tar -xzf kubefwd_Linux_amd64.tar.gz
    sudo mv kubefwd /usr/local/bin/
    sudo chmod +x /usr/local/bin/kubefwd
    ```

=== "Docker"

    Run kubefwd in a privileged container:

    ```bash
    docker run -it --rm --privileged \
        -v "$HOME/.kube:/root/.kube:ro" \
        txn2/kubefwd --tui
    ```

    The container needs `--privileged` to modify network interfaces and the hosts file.

Verify installation:

```bash
kubefwd version
```

---

## Operating Modes

kubefwd supports two operating modes depending on how you want to work.

### Idle Mode (Recommended)

Start kubefwd without specifying namespaces. Add services interactively via TUI or programmatically via API.

```bash
sudo -E kubefwd --tui
```

In Idle Mode:

- **REST API** is enabled by default
- **Auto-reconnect** is enabled by default
- Use the TUI to browse namespaces, select services, and forward them
- Or use the API to add services dynamically

This is the recommended mode for interactive development. You can explore what's available in your cluster and forward exactly what you need.

### Classic Mode

Specify namespaces upfront. All services in those namespaces are forwarded immediately.

```bash
sudo -E kubefwd svc -n default --tui
```

Or without TUI for scripting:

```bash
sudo -E kubefwd svc -n default
```

Classic mode is backwards-compatible with all previous kubefwd versions. Add `--tui`, `--api`, or `-a` (auto-reconnect) as needed.

---

## Your First Forward

### Interactive (Recommended)

1. **Start kubefwd in Idle Mode:**

    ```bash
    sudo -E kubefwd --tui
    ```

2. **Browse namespaces:** Press `f` to open the service browser, then select a namespace.

3. **Forward services:** Select individual services or press `a` to forward all services in the namespace.

4. **Use your services:** They're now accessible by name:

    ```bash
    curl http://my-api:8080/health
    psql -h postgres -U admin -d mydb
    redis-cli -h redis ping
    ```

5. **Quit:** Press `q` to exit. kubefwd cleans up automatically.

### Quick Start (Classic Mode)

Forward all services in a namespace immediately:

```bash
sudo -E kubefwd svc -n my-namespace --tui
```

You'll see the TUI with all services from `my-namespace` being forwarded.

---

## What Happens Under the Hood

When kubefwd forwards a service:

1. **IP Allocation**: Each service gets a unique loopback IP (e.g., `127.1.27.1`, `127.1.27.2`)

2. **Hosts File Update**: Service names are added to `/etc/hosts`:
   ```
   127.1.27.1    my-api
   127.1.27.2    postgres
   127.1.27.3    redis
   ```

3. **Port Forwarding**: kubefwd creates port forwards to pods backing each service

4. **Ready to Use**: Your applications can connect using service names, exactly like in-cluster

```
Your App → postgres:5432
         ↓
/etc/hosts: postgres = 127.1.27.2
         ↓
kubefwd listening on 127.1.27.2:5432
         ↓
Kubernetes API → Pod in cluster
```

### Unique IPs Solve Port Conflicts

Unlike `kubectl port-forward` which binds to a single port, kubefwd assigns each service its own IP address. This means:

- Multiple databases on port 5432? No problem.
- Multiple web services on port 80? Works fine.
- Every service uses its real port, just like in-cluster.

### Cleanup

When kubefwd exits (via `q` or `Ctrl+C`):

- `/etc/hosts` entries are removed
- Network interface aliases are cleaned up
- All port forward connections are closed

Your original hosts file is backed up to `~/hosts.original` for safety.

---

## Why sudo -E?

kubefwd requires root privileges for:

- Modifying `/etc/hosts`
- Creating loopback IP aliases on the network interface
- Binding to ports below 1024

The `-E` flag preserves your environment variables, especially `KUBECONFIG`:

```bash
# Correct - preserves KUBECONFIG
sudo -E kubefwd --tui

# Wrong - may fail to find cluster config
sudo kubefwd --tui
```

---

## Enabling the REST API

The REST API allows programmatic control of kubefwd and enables AI assistant integration via MCP.

**Idle Mode** (API enabled by default):
```bash
sudo -E kubefwd
sudo -E kubefwd --tui
```

**Classic Mode** (add `--api` flag):
```bash
sudo -E kubefwd svc -n default --api
```

The API is available at:

- `http://kubefwd.internal/api` - REST endpoints
- `http://kubefwd.internal/docs` - Interactive API documentation

See [REST API Reference](api-reference.md) for endpoints and [MCP Integration](mcp-integration.md) for AI assistant setup.

---

## Common Commands

```bash
# Idle mode with TUI (recommended for interactive use)
sudo -E kubefwd --tui

# Forward specific namespace with TUI
sudo -E kubefwd svc -n my-namespace --tui

# Forward multiple namespaces
sudo -E kubefwd svc -n frontend,backend,data --tui

# Forward with label selector
sudo -E kubefwd svc -n default -l app=myapp --tui

# Classic mode (no TUI, for scripts)
sudo -E kubefwd svc -n default

# Enable verbose logging
sudo -E kubefwd svc -n default -v
```

---

## Next Steps

- **[User Guide](user-guide.md)** - Master the interactive interface
- **[Configuration](configuration.md)** - All command-line options
- **[Advanced Usage](advanced-usage.md)** - Multi-namespace, selectors, port mapping
- **[MCP Integration](mcp-integration.md)** - AI assistant setup
