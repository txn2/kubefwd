# MCP Integration

## What is MCP?

The **Model Context Protocol (MCP)** is an open standard that lets AI assistants connect to external tools and services. Instead of just answering questions, your AI can actually *do things*: forward ports, query databases, run commands, and interact with APIs.

kubefwd's MCP integration gives your AI assistant full access to Kubernetes port forwarding capabilities. Your AI can discover services, establish connections, monitor traffic, stream logs, and troubleshoot issues—all through natural conversation.

---

## Platform Support

kubefwd MCP uses **stdio transport**, which means it runs locally on your machine and communicates with AI clients through standard input/output. This works seamlessly with desktop applications but has implications for cloud-based AI services.

| Platform | Transport | kubefwd Support | Notes |
|----------|-----------|-----------------|-------|
| **Claude Code** | stdio | :white_check_mark: Full | Native support, recommended |
| **Claude Desktop** | stdio | :white_check_mark: Full | One-click `.mcpb` install |
| **Cursor** | stdio | :white_check_mark: Full | Native support |
| **Windsurf** | stdio | :white_check_mark: Full | Native support |
| **VS Code Copilot** | stdio | :white_check_mark: Full | GA in VS Code 1.102+ |
| **Zed** | stdio | :white_check_mark: Full | Via settings or extensions |
| **Cline** | stdio | :white_check_mark: Full | VS Code extension |
| **JetBrains IDEs** | stdio | :white_check_mark: Full | IntelliJ 2025.1+, PyCharm, WebStorm, etc. |
| **Gemini CLI** | stdio | :white_check_mark: Full | Google's CLI supports MCP |
| **ChatGPT** | HTTP/SSE only | :material-tunnel: Tunnel required | See [ChatGPT Setup](#chatgpt-setup) |

!!! info "About ChatGPT"
    ChatGPT supports MCP but **only via remote servers** using HTTP/SSE transport. It cannot connect to localhost. To use kubefwd with ChatGPT, you must expose the kubefwd API through a tunnel service like [ngrok](https://ngrok.com). See [ChatGPT Setup](#chatgpt-setup) for details.

---

## Why MCP?

Don't ask about kubefwd. Ask about your work.

Need a database connection? Say "I need to connect to PostgreSQL in staging." Your AI handles the infrastructure. Want to test a deployment? Say "Test my new API and show me the logs." kubefwd becomes invisible.

Traditional workflow:
```
You: How do I forward postgres from kft1?
AI: Run sudo -E kubefwd svc -n kft1 -l app=postgres
You: *runs command*
You: What's the connection string?
AI: psql -h postgres -U user -d dbname
```

With MCP:
```
You: I need the staging database
AI: Connected. Your connection: psql -h postgres -U admin -d mydb
```

The AI understands context, remembers what you're working on, and handles the mechanics.

---

## Architecture

kubefwd MCP uses a two-process architecture that separates privilege levels for security:

```mermaid
flowchart TB
    subgraph client["AI Client"]
        cc["Claude Code<br/>Cursor, Windsurf, etc."]
    end

    subgraph mcp["kubefwd mcp<br/><small>(no sudo required)</small>"]
        bridge["MCP Bridge"]
    end

    subgraph kubefwd["kubefwd<br/><small>(with sudo)</small>"]
        fwd["Port Forwarding Engine"]
    end

    subgraph system["System Resources"]
        hosts["/etc/hosts"]
        loopback["Loopback IPs<br/>127.x.x.x"]
        k8s["Kubernetes API"]
    end

    cc <-->|"stdio<br/>MCP Protocol"| bridge
    bridge <-->|"REST API<br/>localhost:8080"| fwd
    fwd --> hosts
    fwd --> loopback
    fwd <--> k8s
```

### Process Separation

1. **kubefwd** (elevated privileges)
    - Manages actual port forwarding via Kubernetes API
    - Creates loopback IP aliases (127.x.x.x) on the network interface
    - Modifies `/etc/hosts` with service hostnames
    - Exposes REST API on `http://localhost:8080` (configurable)
    - Requires `sudo` for network and hosts file access

2. **kubefwd mcp** (standard user)
    - Runs as MCP server via stdio transport
    - Communicates with kubefwd via REST API
    - Spawned automatically by AI clients
    - No elevated privileges required

This separation means your AI client never needs root access—it communicates through the unprivileged MCP bridge, which in turn talks to the privileged kubefwd process.

---

## What It Looks Like

### Accessing Cluster Services

> **You:** "I need to connect to PostgreSQL in kft1 on the dev cluster"
>
> **AI:** "Connected. Use: `psql -h postgres -U admin -d mydb`"

Behind the scenes, the AI discovered the service, forwarded it, and got your connection info. You just asked for what you needed.

### Setting Up Your Environment

> **You:** "I'm starting work on the checkout service, set up my environment"
>
> **AI:** "Your environment is ready. These services are available locally:
>
> - cart-service:8080
> - payment-service:443
> - inventory-service:8080
> - user-db:5432
>
> Add to your .env:
> ```
> CART_URL=http://cart-service:8080
> PAYMENT_URL=https://payment-service:443
> ```"

### Real Workflow: Building a Notification Service

Here's a realistic development scenario showing MCP capabilities:

> **You:** "I'm building a notification service that needs to send emails and store records in the database. What services are available in the dev namespace?"
>
> **AI:** *queries cluster* "I found these services in dev:
>
> - postgres:5432 (database)
> - smtp:2525 (mail server)
> - redis:6379 (cache)
> - api-gateway:8080
>
> Want me to forward postgres and smtp for your notification service?"
>
> **You:** "Yes, forward those"
>
> **AI:** *forwards services* "Done. Here's your configuration:
>
> ```python
> # config.py
> DATABASE_URL = 'postgresql://dev:dev123@postgres:5432/notifications'
> SMTP_HOST = 'smtp'
> SMTP_PORT = 2525
> ```
>
> These hostnames work exactly like they would in-cluster."

The AI understands the development context, discovers available infrastructure, and provides ready-to-use configuration.

### Testing Your Deployment

> **You:** "I just deployed my payments API to staging. Test it and show me the traffic."
>
> **AI:** "I forwarded payments-api from staging. Results:
>
> - GET /health returned 200 OK (45ms)
> - Service receiving 12 req/sec
> - No errors in last 5 minutes
> - Pod logs: 'Server started on :8080'"

### Debugging Issues

> **You:** "My API calls to auth-service are failing with connection refused"
>
> **AI:** "The auth-service pod restarted and the forward was stale. I've reconnected it. Try again."

Or:

> **You:** "My new user-service deployment isn't working"
>
> **AI:** "I tested user-service:
>
> - /health returns 503 Service Unavailable
> - Pod logs: 'ERROR: Cannot connect to database'
>
> The postgres service isn't in this namespace. Did you mean to deploy to staging where postgres exists?"

### Service Discovery

> **You:** "What databases are available in the dev cluster?"
>
> **AI:** "I found:
>
> - PostgreSQL in default and staging namespaces
> - MySQL in the legacy namespace
> - Redis in default and production
>
> Which do you need?"

---

## Setup

### 1. Start kubefwd

kubefwd must be running with API enabled for MCP to communicate with it:

```bash
# Start in idle mode with API (waits for MCP commands)
sudo -E kubefwd

# Or start with TUI to monitor AI-driven activity
sudo -E kubefwd --tui

# Or pre-forward a namespace and enable API
sudo -E kubefwd svc -n default --api
```

The `--tui` flag is recommended as it lets you visually monitor what your AI is doing with your forwards.

!!! tip "The `-E` flag"
    Always use `sudo -E` to preserve your `KUBECONFIG` environment variable. Without it, kubefwd won't find your cluster configuration.

### 2. Configure Your AI Client

=== "Claude Code (Recommended)"

    One command to add kubefwd:

    ```bash
    claude mcp add --transport stdio kubefwd -- kubefwd mcp
    ```

    Verify it's configured:

    ```bash
    claude mcp list
    ```

    That's it. Claude Code will automatically spawn `kubefwd mcp` when needed.

=== "Claude Desktop (One-Click)"

    Download the `.mcpb` bundle for your platform from [GitHub Releases](https://github.com/txn2/kubefwd/releases):

    - **macOS (Apple Silicon):** `kubefwd-VERSION-darwin-arm64.mcpb`
    - **macOS (Intel):** `kubefwd-VERSION-darwin-amd64.mcpb`
    - **Windows:** `kubefwd-VERSION-windows-amd64.mcpb`

    Double-click the downloaded file to install kubefwd as a Claude Desktop extension.

=== "Cursor"

    Add to `~/.cursor/mcp.json` (global) or `.cursor/mcp.json` (project):

    ```json
    {
      "mcpServers": {
        "kubefwd": {
          "command": "kubefwd",
          "args": ["mcp"]
        }
      }
    }
    ```

=== "Windsurf"

    Open Windsurf, click the hammer (MCP) icon in Cascade sidebar, then Configure:

    ```json
    {
      "mcpServers": {
        "kubefwd": {
          "command": "kubefwd",
          "args": ["mcp"]
        }
      }
    }
    ```

=== "VS Code Copilot"

    Add to your VS Code settings or `.vscode/mcp.json`:

    ```json
    {
      "mcpServers": {
        "kubefwd": {
          "command": "kubefwd",
          "args": ["mcp"]
        }
      }
    }
    ```

    Requires VS Code 1.102+ with AI Assistant enabled.

=== "Zed"

    Add to your Zed `settings.json`:

    ```json
    {
      "context_servers": {
        "kubefwd": {
          "source": "custom",
          "command": "kubefwd",
          "args": ["mcp"]
        }
      }
    }
    ```

=== "JetBrains IDEs"

    IntelliJ IDEA 2025.1+, PyCharm, WebStorm, and other JetBrains IDEs support MCP:

    1. Go to **Settings > Tools > AI Assistant > Model Context Protocol (MCP)**
    2. Add a new server configuration:
        - **Name:** kubefwd
        - **Command:** kubefwd
        - **Arguments:** mcp

    See [JetBrains MCP documentation](https://www.jetbrains.com/help/idea/mcp-server.html) for details.

=== "Cline"

    In Cline settings (VS Code extension), add MCP server:

    ```json
    {
      "mcpServers": {
        "kubefwd": {
          "command": "kubefwd",
          "args": ["mcp"]
        }
      }
    }
    ```

=== "Gemini CLI"

    Add to your Gemini CLI MCP configuration:

    ```json
    {
      "mcpServers": {
        "kubefwd": {
          "command": "kubefwd",
          "args": ["mcp"]
        }
      }
    }
    ```

    See [Gemini CLI documentation](https://github.com/google-gemini/gemini-cli) for setup details.

### ChatGPT Setup

ChatGPT only supports remote MCP servers via HTTP/SSE transport. To use kubefwd with ChatGPT, you need to expose the kubefwd REST API through a tunnel:

1. **Start kubefwd with API enabled:**
   ```bash
   sudo -E kubefwd --api
   ```

2. **Create a tunnel using ngrok:**
   ```bash
   ngrok http 8080
   ```

3. **In ChatGPT settings:**
    - Go to Settings > Connectors > Advanced > Developer Mode
    - Add a custom MCP connector with your ngrok URL
    - Configure authentication if needed

!!! warning "Security Consideration"
    Exposing kubefwd via a public tunnel gives external access to your Kubernetes port forwards. Only do this in development environments and consider using ngrok's authentication features.

---

## What Your AI Can Do

### Service Forwarding

- **Forward entire namespaces** - Forward all services in a namespace to localhost with automatic `/etc/hosts` entries
- **Forward individual services** - Forward specific services with custom port mappings
- **Remove forwards** - Stop forwarding namespaces or individual services
- **Multi-context support** - Forward services from multiple clusters simultaneously

### Discovery & Connection

- **List Kubernetes contexts** - See available clusters from your kubeconfig
- **List namespaces** - Discover what namespaces are available and which are forwarded
- **List services** - See services in a namespace with their types, ports, and selectors
- **Get connection info** - Get ready-to-use connection strings, IPs, hostnames, and environment variables
- **Find services** - Search forwarded services by name pattern, port, or namespace

### Pod Debugging

- **List pods** - See pods with status, ready state, restarts, and age; filter by labels or service
- **Get pod details** - Inspect containers, conditions, resources, and recent events
- **Get pod logs** - Stream logs from containers with tail, timestamps, and time filtering
- **Get Kubernetes events** - View scheduling, pulling, starting events for diagnosing failures
- **Get service endpoints** - See which pods back a service and their ready state

### Monitoring & Metrics

- **Quick status** - Fast health check of kubefwd (healthy/degraded/unhealthy)
- **View logs** - Filter by level (debug, info, warn, error) or search terms
- **Get metrics** - Bandwidth stats, bytes in/out, transfer rates, per-service breakdown
- **HTTP traffic** - View requests flowing through forwards (method, path, status code, response time)
- **Event history** - Track events, errors, and reconnections over time

### Troubleshooting

- **Diagnose errors** - Root cause analysis with specific fix suggestions
- **Get analysis** - Full issue classification with priorities and recommended actions
- **Reconnect services** - Force reconnection for errored or stale services
- **Sync services** - Re-discover pods after deployments or pod restarts

---

## Technical Reference

For custom integrations and advanced usage, kubefwd MCP provides a comprehensive set of tools, resources, and prompts.

### Tools (28 total)

| Category | Tools | Description |
|----------|-------|-------------|
| **Discovery** | `list_k8s_namespaces`, `list_k8s_services`, `list_contexts` | Query available Kubernetes resources |
| **Namespace Ops** | `add_namespace`, `remove_namespace` | Forward/stop entire namespaces |
| **Service Ops** | `add_service`, `remove_service`, `list_services`, `get_service`, `find_services` | Individual service management |
| **Connection** | `get_connection_info`, `list_hostnames` | Get connection details and host mappings |
| **Pods** | `list_pods`, `get_pod`, `get_pod_logs` | Pod inspection and log streaming |
| **Kubernetes** | `get_events`, `get_endpoints` | Cluster events and endpoint details |
| **Health** | `get_health`, `get_quick_status`, `get_metrics`, `get_analysis`, `diagnose_errors` | System health and diagnostics |
| **Traffic** | `get_http_traffic`, `get_history`, `get_logs` | Traffic monitoring and history |
| **Control** | `reconnect_service`, `reconnect_all_errors`, `sync_service` | Forward management |

### Resources (8 total)

Resources provide read-only access to kubefwd state:

| URI | Description |
|-----|-------------|
| `kubefwd://status` | Quick health check (ok/degraded/error) |
| `kubefwd://services` | List of all forwarded services |
| `kubefwd://forwards` | Detailed port forward information |
| `kubefwd://metrics` | Traffic metrics and bandwidth stats |
| `kubefwd://summary` | Overall system summary |
| `kubefwd://errors` | Current error conditions |
| `kubefwd://http-traffic` | Recent HTTP requests through forwards |
| `kubefwd://contexts` | Available Kubernetes contexts |

### Prompts (10 total)

Prompts provide guided workflows for common tasks:

| Prompt | Purpose |
|--------|---------|
| `setup_local_dev` | Complete environment setup guide |
| `forward_namespace` | Forward all services in a namespace |
| `quick_connect` | Fast connection to a single service |
| `connection_guide` | Connection string examples for various tools |
| `troubleshoot` | General debugging workflow |
| `debug_service` | Deep dive into a specific service |
| `fix_errors` | Resolve current error conditions |
| `analyze_issues` | Comprehensive issue analysis |
| `explain_status` | Explain current kubefwd state |
| `monitor` | Set up monitoring for forwards |

---

## Best Practices

### Running kubefwd for MCP

**Recommended: TUI Mode**
```bash
sudo -E kubefwd --tui
```
The TUI provides visibility into what your AI is doing—you can see services being forwarded, traffic flowing, and any errors that occur.

**Alternative: Background with Logging**
```bash
sudo -E kubefwd -v 2>&1 | tee kubefwd.log &
```
Run in verbose mode with logs captured for debugging.

### Security Considerations

1. **Local only by default** - kubefwd API binds to localhost. Your forwards are only accessible on your machine.

2. **Privilege separation** - The MCP bridge runs without sudo. Only kubefwd itself needs elevated privileges.

3. **No credential exposure** - kubefwd uses your existing kubeconfig. No cluster credentials are passed through MCP.

4. **Network isolation** - Each forwarded service gets its own loopback IP (127.x.x.x), preventing port conflicts and providing isolation.

### Multi-Cluster Workflows

kubefwd supports forwarding from multiple clusters simultaneously:

> **You:** "Forward the payment service from both dev and staging so I can compare behavior"
>
> **AI:** "Done. Both are accessible:
>
> - dev: payment-service.dev-context (127.1.27.1:8080)
> - staging: payment-service.staging-context (127.1.27.2:8080)
>
> The hostnames include the context to differentiate them."

---

## Troubleshooting

### MCP Not Connecting

1. **Is kubefwd running?**
   ```bash
   curl http://localhost:8080/api/health
   ```
   Should return `{"status":"ok"}` or similar.

2. **Is kubefwd mcp in your PATH?**
   ```bash
   which kubefwd
   ```

3. **Check AI client logs** - Most clients have MCP debugging options.

### "Permission denied" Errors

kubefwd requires sudo for network operations:
```bash
# Wrong
kubefwd svc -n default

# Right
sudo -E kubefwd svc -n default
```

### Forwards Not Working

1. **Check kubefwd status:**
   Ask your AI: "What's the kubefwd status?"

2. **Verify hosts file:**
   ```bash
   grep 127.1 /etc/hosts
   ```

3. **Test connectivity:**
   ```bash
   curl http://service-name:port/health
   ```

---

## See Also

- [REST API Reference](api-reference.md) - Direct API access for custom tooling
- [Getting Started](getting-started.md) - Installation and basic usage
- [User Guide](user-guide.md) - Interactive terminal interface
