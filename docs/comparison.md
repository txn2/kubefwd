# kubefwd vs Alternatives

Understanding when to use kubefwd versus other local Kubernetes development tools.

## The Essential Difference

**kubefwd solves one problem exceptionally well**: accessing cluster services from your local machine using their actual service names.

Most other tools in this space focus on **bidirectional traffic interception**: routing cluster traffic to your local machine so you can intercept requests destined for a deployed service. kubefwd takes the opposite, simpler approach: it makes cluster services available locally without modifying anything in the cluster.

| Approach | Direction | Use Case |
|----------|-----------|----------|
| **kubefwd** | Local → Cluster | "I want to develop locally and access cluster services" |
| **Telepresence/mirrord** | Cluster → Local | "I want to intercept cluster traffic on my machine" |

### System-Wide Access

Because kubefwd updates `/etc/hosts`, **any application** on your machine can access cluster services, not just your code:

- **Web browsers**: Navigate to `http://grafana:3000` or `http://kibana:5601`
- **Database clients**: Connect DBeaver, DataGrip, pgAdmin, or TablePlus to `postgres:5432`
- **CLI tools**: `curl http://api:8080`, `redis-cli -h redis`, `psql -h postgres`
- **IDEs**: Use built-in database explorers, REST clients, GraphQL playgrounds
- **Any other tool**: Postman, Insomnia, mongosh, mysql, etc.

This is a significant advantage over process-injection tools where only the injected process sees cluster services.

## Quick Comparison

| Feature | kubefwd | Telepresence | mirrord | Gefyra |
|---------|---------|--------------|---------|--------|
| **Access cluster services locally** | Yes | Yes | Yes | Yes |
| **Intercept cluster traffic** | No | Yes | Yes | Yes |
| **Cluster components required** | No | Yes (Traffic Manager) | Yes (Operator) | Yes (Operator) |
| **Service name resolution** | /etc/hosts | DNS proxy | Process injection | Docker networking |
| **Multi-service forwarding** | All in namespace | One at a time | Per-process | Per-container |
| **Root/sudo required** | Yes | Optional | No | No |
| **Setup complexity** | Minimal | Moderate | Low | Moderate |
| **Open source** | Apache 2.0 | Proprietary core | MIT | Apache 2.0 |

## Detailed Comparisons

### kubefwd vs Telepresence

[Telepresence](https://www.telepresence.io/) is Ambassador Labs' tool for intercepting traffic from a Kubernetes cluster.

**Telepresence strengths:**
- Intercept live cluster traffic on your local machine
- Personal intercepts with preview URLs (paid feature)
- Sophisticated traffic routing and filtering
- Good for debugging production-like traffic

**kubefwd strengths:**
- No cluster components to install
- Bulk forward all services with one command
- Unique IP per service (no port conflicts)
- Simpler mental model (one-way access)
- Fully open source

**Choose Telepresence when:**
- You need to intercept requests destined for a deployed service
- You want to test with real production traffic patterns
- You're debugging issues that only appear in-cluster

**Choose kubefwd when:**
- You just want local access to cluster services
- You're developing a new service that calls existing services
- You want minimal setup with no cluster changes
- You're forwarding many services simultaneously
- You want to browse cluster web UIs, use database clients, or access services from any local application

### kubefwd vs mirrord

[mirrord](https://mirrord.dev/) takes a unique approach: it injects into your local process to intercept and redirect network calls.

**mirrord strengths:**
- No root/sudo required
- Process-level isolation (doesn't affect other local processes)
- Can mirror or steal traffic from pods
- Supports file system access from cluster
- Lightweight agent pods

**kubefwd strengths:**
- System-wide access (all local processes see cluster services)
- No process injection complexity
- Works with any language/runtime
- Bulk forwarding of entire namespaces
- Unique IPs eliminate port conflicts

**Choose mirrord when:**
- You can't use sudo/root
- You want process-level isolation
- You need file system access from cluster pods
- You're intercepting traffic to a specific service

**Choose kubefwd when:**
- Multiple local processes need cluster access
- You're running IDEs, CLI tools, and apps that all need cluster services
- You want system-wide `/etc/hosts` entries
- You prefer simplicity over features

**System-wide vs Process-level Access:**

With kubefwd's system-wide approach, *any* application can access cluster services:

```bash
# Your web browser can access cluster UIs
open http://grafana:3000
open http://kibana:5601

# Database clients (DBeaver, DataGrip, pgAdmin) connect directly
# Just use: host=postgres port=5432

# Any CLI tool works
curl http://api-service:8080/health
redis-cli -h redis -p 6379
psql -h postgres -U admin
mongo --host mongodb:27017

# Your IDE's database explorer, REST client, etc.
```

With process-injection tools like mirrord, you must launch each application through the tool:
```bash
mirrord exec -- curl http://api-service:8080  # Only this curl sees cluster
mirrord exec -- your-app                        # Only this process sees cluster
# Your browser, database client, other terminals don't see cluster services
```

### kubefwd vs Gefyra

[Gefyra](https://gefyra.dev/) focuses on Docker-based development workflows.

**Gefyra strengths:**
- Docker-native approach
- Can bridge local containers into cluster network
- Good for Docker Compose workflows
- Supports traffic interception

**kubefwd strengths:**
- Works without Docker
- Native process development (no containers needed)
- Simpler setup for non-containerized workflows
- One command to forward everything

**Choose Gefyra when:**
- Your development workflow is Docker-based
- You want to run local containers as if they're in the cluster
- You're using Docker Compose for local development

**Choose kubefwd when:**
- You're developing without Docker locally
- You want to run your code natively (faster iteration)
- You prefer not to containerize during development
- You want GUI database clients (DBeaver, DataGrip) and browsers to access cluster services directly

### kubefwd vs kubectl port-forward

The built-in `kubectl port-forward` is the baseline for comparison.

| Feature | kubectl port-forward | kubefwd |
|---------|---------------------|---------|
| Services per command | One | All in namespace |
| IP allocation | localhost only | Unique 127.x.x.x per service |
| Port conflicts | Manual management | None (unique IPs) |
| Service name resolution | Not supported | Automatic (/etc/hosts) |
| Auto-reconnect | No | Yes |
| Multiple ports same number | No | Yes |
| Interactive monitoring | No | TUI with metrics |

**Use kubefwd when:**
- You need more than 2-3 services
- Services use the same port (databases, web servers)
- You want service names to work (`curl http://api-service`)
- You want auto-reconnect on pod restarts

### kubefwd vs krelay

[krelay](https://github.com/knight42/krelay) extends kubectl port-forward with additional features.

**krelay strengths:**
- Forwards to ClusterIP directly (not just pods)
- Supports services without selectors
- Works with ExternalName services

**kubefwd strengths:**
- Bulk forwarding (all services at once)
- Unique IPs per service
- Service name resolution via /etc/hosts
- Interactive TUI

**Choose krelay when:**
- You need to forward to a specific ClusterIP
- You have services without pod selectors

**Choose kubefwd when:**
- You want bulk forwarding
- You want service names to resolve

## When to Use kubefwd

kubefwd is ideal when:

1. **You're developing a service that depends on cluster services**
   - Your local API needs `auth-service:443`, `database:5432`, `cache:6379`
   - You want to use the same connection strings locally as in production

2. **You want minimal friction**
   - One command: `sudo -E kubefwd svc -n dev --tui`
   - No cluster components to install or manage
   - No Docker required

3. **You have many services to access**
   - Forward 10, 20, or 100 services with one command
   - Each gets a unique IP (no port conflicts)

4. **You want to keep it simple**
   - One-way access: local → cluster
   - No traffic interception complexity
   - No process injection magic

## When NOT to Use kubefwd

Consider alternatives when:

1. **You need to intercept cluster traffic**
   - Testing how your service handles real requests
   - Debugging production issues with live traffic
   - → Use Telepresence or mirrord

2. **You can't use sudo/root**
   - Restricted development environments
   - → Use mirrord (process injection, no root)

3. **Your workflow is Docker-native**
   - Local development in containers
   - Docker Compose environments
   - → Consider Gefyra

4. **You need to forward UDP**
   - Kubernetes port-forwarding doesn't support UDP
   - → No good options; consider running locally

## Summary

kubefwd occupies a specific niche in the local Kubernetes development ecosystem: **simple, bulk access to cluster services with service name resolution**. It doesn't try to intercept traffic or inject into processes. It just makes cluster services accessible locally, exactly as they appear in-cluster.

For developers who primarily need to connect their local code to cluster dependencies (databases, APIs, caches), kubefwd provides the simplest path with the least overhead.

---

*See also: [Getting Started](getting-started.md) | [User Guide](user-guide.md) | [Architecture](architecture.md)*
