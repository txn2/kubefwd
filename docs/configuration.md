# Configuration

Complete reference for kubefwd command line flags and configuration options.

## Command Line Flags

### Essential Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--namespace` | `-n` | Namespace(s) to forward services from. Can be specified multiple times. |
| `--tui` | | Enable interactive terminal user interface |
| `--selector` | `-l` | Label selector to filter services |
| `--field-selector` | `-f` | Field selector to filter services |
| `--all-namespaces` | `-A` | Forward services from all namespaces |

### TUI and Behavior Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--tui` | | false | Enable interactive TUI mode |
| `--auto-reconnect` | `-a` | true (TUI) | Automatically reconnect lost port forwards |
| `--resync-interval` | | 5m | Interval for forced service resync |
| `--retry-interval` | | 10s | Retry interval when no pods found |
| `--timeout` | `-t` | 300 | Timeout in seconds for port forwarding |
| `--verbose` | `-v` | false | Enable debug logging |

### Network Configuration

| Flag | Short | Description |
|------|-------|-------------|
| `--mapping` | `-m` | Port mapping (e.g., `-m 80:8080` maps service port 80 to local 8080) |
| `--reserve` | `-r` | Reserve specific IP for service (e.g., `-r svc.ns:127.3.3.1`) |
| `--fwd-conf` | `-z` | Path to IP reservation config file |
| `--domain` | `-d` | Append domain suffix to hostnames |
| `--hosts-path` | | Custom path for hosts file (default: `/etc/hosts`) |

### Kubernetes Configuration

| Flag | Short | Description |
|------|-------|-------------|
| `--kubeconfig` | `-c` | Path to kubeconfig file |
| `--context` | `-x` | Kubernetes context(s) to use. Can be specified multiple times. |

### Maintenance Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--refresh-backup` | `-b` | Create fresh hosts backup before modifying |
| `--purge-stale-ips` | `-p` | Remove stale kubefwd entries from hosts file |

## Flag Details

### Namespace Selection (-n)

Forward services from one or more namespaces:

```bash
# Single namespace
sudo -E kubefwd svc -n default --tui

# Multiple namespaces (comma-separated)
sudo -E kubefwd svc -n default,staging,production --tui

# All namespaces
sudo -E kubefwd svc -A --tui
```

### Label Selectors (-l)

Filter services by Kubernetes labels:

```bash
# Exact match
sudo -E kubefwd svc -n default -l app=api --tui

# Multiple labels (AND)
sudo -E kubefwd svc -n default -l app=api,tier=backend --tui

# Set-based selectors
sudo -E kubefwd svc -n default -l "app in (api, web)" --tui
sudo -E kubefwd svc -n default -l "tier notin (frontend)" --tui

# Exclusion
sudo -E kubefwd svc -n default -l app!=debug --tui
```

### Field Selectors (-f)

Filter services by metadata fields:

```bash
# Forward single service by name
sudo -E kubefwd svc -n default -f metadata.name=my-service --tui

# Exclude a service
sudo -E kubefwd svc -n default -f metadata.name!=unwanted-service --tui
```

### Port Mapping (-m)

Remap service ports to different local ports:

```bash
# Map port 80 to local 8080
sudo -E kubefwd svc -n default -m 80:8080 --tui

# Multiple mappings
sudo -E kubefwd svc -n default -m 80:8080 -m 443:8443 --tui
```

Use case: When a service uses port 80 but you can't bind to low ports, or to avoid conflicts.

### IP Reservation (-r)

Reserve specific IPs for services:

```bash
# Reserve IP for a service
sudo -E kubefwd svc -n default -r my-service.default:127.3.3.1 --tui

# Multiple reservations
sudo -E kubefwd svc -n default \
  -r api.default:127.3.3.1 \
  -r db.default:127.3.3.2 \
  --tui
```

Format: `service-name.namespace:ip-address`

### Domain Suffix (-d)

Append a domain to all generated hostnames:

```bash
sudo -E kubefwd svc -n default -d cluster.local --tui
```

Services will be accessible as `my-service.cluster.local` in addition to `my-service`.

### Auto-Reconnect (-a)

Control automatic reconnection behavior:

```bash
# Explicitly enable (default in TUI mode)
sudo -E kubefwd svc -n default --tui -a

# Disable auto-reconnect
sudo -E kubefwd svc -n default --tui -a=false
```

When enabled, lost connections retry with exponential backoff (1s → 5min max).

## IP Reservation Config File

For complex setups, use a YAML configuration file with `-z`:

```bash
sudo -E kubefwd svc -n default -z config.yml --tui
```

### Config File Format

```yaml
# config.yml
reservations:
  # Reserve specific IPs for services
  - service: api-gateway
    namespace: default
    ip: 127.3.3.1

  - service: database
    namespace: default
    ip: 127.3.3.2

  - service: cache
    namespace: default
    ip: 127.3.3.3

# Optional: base IP for auto-allocation
baseIP: 127.1.27.1
```

See `example.fwdconf.yml` in the repository for a complete example.

## Environment Variables

### KUBECONFIG

kubefwd uses the standard `KUBECONFIG` environment variable:

```bash
export KUBECONFIG=~/.kube/config
sudo -E kubefwd svc -n default --tui
```

The `-E` flag with sudo preserves this variable.

### Multiple Kubeconfigs

```bash
export KUBECONFIG=~/.kube/config:~/.kube/other-config
sudo -E kubefwd svc -n default -x other-context --tui
```

## Default IP Allocation

When no reservations are specified, kubefwd allocates IPs starting from `127.1.27.1`:

```
127.[1+cluster].[27+namespace].[service_count]
```

| Octet | Base | Incremented By |
|-------|------|----------------|
| 1st | 127 | (fixed) |
| 2nd | 1 | Cluster index |
| 3rd | 27 | Namespace index |
| 4th | 1 | Service count |

This scheme supports:
- Up to 255 services per namespace
- Up to 229 namespaces per cluster (27+228=255)
- Up to 255 clusters

## Examples

### Development Setup

```bash
# Forward all services in dev namespace with TUI
sudo -E kubefwd svc -n development --tui
```

### Production Debugging

```bash
# Forward specific services from production
sudo -E kubefwd svc -n production \
  -l app=api \
  -f metadata.name!=api-canary \
  --tui
```

### Multi-Cluster

```bash
# Forward from multiple clusters (comma-separated)
sudo -E kubefwd svc -n default -x cluster-us-east,cluster-us-west --tui
```

### CI/CD Integration

```bash
# Non-interactive mode for scripts
sudo -E kubefwd svc -n testing \
  -l component=test \
  --auto-reconnect \
  --timeout 600
```
