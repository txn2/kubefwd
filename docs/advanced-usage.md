# Advanced Usage

This guide covers advanced kubefwd scenarios and configurations.

## Multiple Namespaces

Forward services from multiple namespaces simultaneously:

```bash
sudo -E kubefwd svc -n frontend,backend,database --tui
```

### Namespace Hostname Resolution

When forwarding from multiple namespaces, hostnames are qualified to avoid conflicts:

| Namespace Index | Hostname Pattern |
|-----------------|------------------|
| First namespace | `service-name` |
| Additional namespaces | `service-name.namespace` |

Example with `-n frontend,backend`:
- `api` (from frontend)
- `api.backend` (from backend)

### All Namespaces

Forward every service in the cluster:

```bash
sudo -E kubefwd svc -A --tui
```

**Caution**: This can forward hundreds of services. Use with label selectors to filter:

```bash
sudo -E kubefwd svc -A -l team=myteam --tui
```

## Multiple Clusters

Forward services from different Kubernetes clusters:

```bash
sudo -E kubefwd svc -n default -x production-cluster,staging-cluster --tui
```

### Cluster Hostname Resolution

Services from additional clusters get context-qualified names:

| Cluster Index | Hostname Pattern |
|---------------|------------------|
| First cluster | `service-name` |
| Additional clusters | `service-name.context-name` |

### Cross-Cluster Development

Forward production dependencies while developing locally:

```bash
sudo -E kubefwd svc -n production -x prod-cluster -l type=database --tui
# Or combine namespaces across clusters:
sudo -E kubefwd svc -n production,development -x prod-cluster,dev-cluster --tui
```

## Selectors

### Label Selectors

kubefwd uses the same selector syntax as `kubectl`:

```bash
# Equality-based
sudo -E kubefwd svc -n default -l app=nginx --tui
sudo -E kubefwd svc -n default -l app==nginx --tui  # Same as above
sudo -E kubefwd svc -n default -l app!=nginx --tui  # Exclude

# Set-based
sudo -E kubefwd svc -n default -l "app in (api, web, worker)" --tui
sudo -E kubefwd svc -n default -l "tier notin (frontend)" --tui
sudo -E kubefwd svc -n default -l "environment" --tui  # Has label
sudo -E kubefwd svc -n default -l "!canary" --tui      # Doesn't have label

# Multiple requirements (AND logic)
sudo -E kubefwd svc -n default -l app=api,version=v2,team=platform --tui
```

### Field Selectors

Filter by service metadata:

```bash
# Forward single service
sudo -E kubefwd svc -n default -f metadata.name=my-service --tui

# Exclude services
sudo -E kubefwd svc -n default -f metadata.name!=internal-only --tui
```

### Combining Selectors

Use both label and field selectors together:

```bash
sudo -E kubefwd svc -n default \
  -l app=api \
  -f metadata.name!=api-canary \
  --tui
```

## Port Mapping

Remap service ports to different local ports.

### Basic Mapping

```bash
# Service port 80 → local port 8080
sudo -E kubefwd svc -n default -m 80:8080 --tui
```

### Multiple Mappings

```bash
sudo -E kubefwd svc -n default \
  -m 80:8080 \
  -m 443:8443 \
  -m 3000:3000 \
  --tui
```

### Use Cases

1. **Privileged ports**: Map port 80 to 8080 when you can't bind to low ports
2. **Conflict avoidance**: When local services use the same ports
3. **Tool compatibility**: Some tools expect specific port numbers

## Headless Services

kubefwd handles headless services (ClusterIP: None) differently:

### Normal Services
- Forward to the first available pod
- Single hostname: `service-name`

### Headless Services
- Forward to ALL pods backing the service
- Multiple hostnames:
  - `service-name` → first pod
  - `pod-name.service-name` → specific pods

Example for a headless service with 3 pods:
```
my-headless           → pod-0
pod-0.my-headless     → pod-0
pod-1.my-headless     → pod-1
pod-2.my-headless     → pod-2
```

This is useful for:
- StatefulSets (e.g., database clusters)
- Services requiring pod-specific addressing

## Docker Integration

Run kubefwd inside Docker for isolated environments.

### Basic Docker Usage

```bash
docker run -it --rm --privileged \
  --name kubefwd \
  -v "$HOME/.kube:/root/.kube:ro" \
  txn2/kubefwd services -n my-namespace --tui
```

### Access Forwarded Services

From another container on the same network:

```bash
# Start kubefwd container
docker run -d --rm --privileged \
  --name kubefwd \
  --network my-network \
  -v "$HOME/.kube:/root/.kube:ro" \
  txn2/kubefwd services -n my-namespace

# Access services from another container
docker run --rm --network my-network \
  curlimages/curl curl http://kubefwd:8080
```

### Docker Compose Integration

```yaml
version: '3.8'

services:
  kubefwd:
    image: txn2/kubefwd
    privileged: true
    volumes:
      - ~/.kube:/root/.kube:ro
    command: services -n development
    networks:
      - app-network

  my-app:
    build: .
    depends_on:
      - kubefwd
    networks:
      - app-network
    # Access cluster services through kubefwd container

networks:
  app-network:
```

### Ubuntu vs Alpine Images

Two image variants are available:

- `txn2/kubefwd` or `txn2/kubefwd:latest` - Alpine-based (smaller)
- `txn2/kubefwd:ubuntu` - Ubuntu-based (more tools available)

## IP Reservation

### Command Line Reservations

```bash
sudo -E kubefwd svc -n default -r api.default:127.3.3.1,database.default:127.3.3.2 --tui
```

### Configuration File

Create `fwdconf.yml`:

```yaml
reservations:
  - service: api
    namespace: default
    ip: 127.3.3.1

  - service: database
    namespace: default
    ip: 127.3.3.2

  - service: cache
    namespace: production
    ip: 127.3.3.3
```

Use the config:

```bash
sudo -E kubefwd svc -n default,production -z fwdconf.yml --tui
```

### Why Reserve IPs?

1. **Consistent configuration**: Applications expecting specific IPs
2. **Firewall rules**: When local firewall rules reference specific IPs
3. **Testing**: Reproduce specific network conditions
4. **Documentation**: Known IPs for team documentation

## Custom Domain Suffix

Add a domain suffix to all generated hostnames:

```bash
sudo -E kubefwd svc -n default -d svc.cluster.local --tui
```

Hostnames become:
- `my-service.svc.cluster.local`
- `another-service.svc.cluster.local`

Both the short name (`my-service`) and full name (`my-service.svc.cluster.local`) will work.

## Custom Hosts File Path

For testing or containerized environments:

```bash
sudo -E kubefwd svc -n default --hosts-path /tmp/hosts --tui
```

## Scripting and Automation

### Non-Interactive Mode

For CI/CD or background processes, omit `--tui`:

```bash
sudo -E kubefwd svc -n testing -l app=test &
KUBEFWD_PID=$!

# Run your tests
npm test

# Cleanup
kill $KUBEFWD_PID
```

### Health Checking

Check if kubefwd is working:

```bash
# Start kubefwd
sudo -E kubefwd svc -n default &

# Wait for services
sleep 10

# Check a service
curl -sf http://my-service:8080/health || exit 1
```

### Timeout Configuration

For long-running test suites:

```bash
sudo -E kubefwd svc -n default \
  --timeout 3600 \
  --retry-interval 30s \
  --resync-interval 10m
```

## Integration Patterns

### With Tilt

Add to your `Tiltfile`:

```python
local_resource(
  'kubefwd',
  serve_cmd='sudo -E kubefwd svc -n default',
  deps=[]
)
```

### With Skaffold

In `skaffold.yaml`:

```yaml
portForward:
  # Let kubefwd handle port forwarding instead
  - resourceType: service
    resourceName: my-service
    port: 8080
    localPort: 8080
```

Then run kubefwd separately for bulk forwarding.

### With Telepresence

kubefwd can complement Telepresence for different use cases:
- Use Telepresence for intercepting traffic to your service
- Use kubefwd for accessing other cluster services
