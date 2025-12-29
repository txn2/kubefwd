# MCP Integration

Don't ask about kubefwd. Ask about your work.

Need a database connection? Say "I need to connect to PostgreSQL in staging." Your AI handles the infrastructure. Want to test a deployment? Say "Test my new API and show me the logs." kubefwd becomes invisible.

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

```bash
sudo -E kubefwd  # Idle mode, API enabled
```

### 2. Configure Your AI

=== "Claude Code (Easy)"

    One command to add kubefwd:

    ```bash
    claude mcp add --transport stdio kubefwd -- kubefwd mcp
    ```

    Verify it's configured:

    ```bash
    claude mcp list
    ```

=== "Claude Code (Manual)"

    Add to `~/.claude.json`:

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

That's it. Start talking to your AI about your work.

---

## What Your AI Can Do

### Access Dependencies
- Connect to any cluster service by name
- Set up your local dev environment
- Discover available services and databases
- Switch between clusters and namespaces

### Test Your Deployments
- Forward and test newly deployed services
- Make HTTP requests and report results
- Monitor traffic and bandwidth
- Stream pod logs

### Troubleshoot
- Diagnose connection failures
- Auto-reconnect stale forwards
- Analyze HTTP traffic patterns
- Identify misconfigured services

---

## Architecture

```mermaid
flowchart LR
    A[AI Client<br/>Claude, Cursor, etc] <-->|stdio<br/>MCP protocol| B[kubefwd mcp<br/>bridge]
    B <-->|HTTP<br/>REST API| C[kubefwd<br/>with sudo]
```

The MCP bridge runs without sudo and communicates with kubefwd via REST API. Your AI spawns it automatically.

---

## Technical Reference

For custom integrations, kubefwd MCP provides:

??? note "23 Tools"

    | Category | Tools |
    |----------|-------|
    | Discovery | `list_k8s_namespaces`, `list_k8s_services`, `list_contexts` |
    | Namespace | `add_namespace`, `remove_namespace` |
    | Service | `add_service`, `remove_service`, `list_services`, `get_service`, `find_services` |
    | Connection | `get_connection_info`, `list_hostnames` |
    | Health | `get_health`, `get_quick_status`, `get_metrics`, `get_analysis`, `diagnose_errors` |
    | Traffic | `get_http_traffic`, `get_history`, `get_logs` |
    | Control | `reconnect_service`, `reconnect_all_errors`, `sync_service` |

??? note "8 Resources"

    | URI | Description |
    |-----|-------------|
    | `kubefwd://status` | Quick health check |
    | `kubefwd://services` | Forwarded services |
    | `kubefwd://forwards` | Port forward details |
    | `kubefwd://metrics` | Traffic metrics |
    | `kubefwd://summary` | Overall summary |
    | `kubefwd://errors` | Current errors |
    | `kubefwd://http-traffic` | HTTP requests |
    | `kubefwd://contexts` | Kubernetes contexts |

??? note "10 Prompts"

    | Prompt | Purpose |
    |--------|---------|
    | `setup_local_dev` | Environment setup guide |
    | `forward_namespace` | Forward all services |
    | `quick_connect` | Fast service connection |
    | `connection_guide` | Connection examples |
    | `troubleshoot` | Debugging workflow |
    | `debug_service` | Service-specific debug |
    | `fix_errors` | Error resolution |
    | `analyze_issues` | Issue analysis |
    | `explain_status` | Status explanation |
    | `monitor` | Monitoring guide |

---

## See Also

- [REST API](api-reference.md) - Direct API access for custom tooling
- [Getting Started](getting-started.md) - Installation and basic usage
