package fwdmcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerPrompts registers all MCP prompt templates
func (s *Server) registerPrompts() {
	// === Developer-focused prompts (primary use cases) ===

	// setup_local_dev - Help set up local development environment
	s.mcpServer.AddPrompt(&mcp.Prompt{
		Name:        "setup_local_dev",
		Description: "Help set up local development environment with forwarded Kubernetes services",
		Arguments: []*mcp.PromptArgument{
			{
				Name:        "namespace",
				Description: "Kubernetes namespace to forward services from",
				Required:    false,
			},
			{
				Name:        "context",
				Description: "Kubernetes context to use (optional)",
				Required:    false,
			},
		},
	}, s.handleSetupLocalDevPrompt)

	// connection_guide - How to connect to a service
	s.mcpServer.AddPrompt(&mcp.Prompt{
		Name:        "connection_guide",
		Description: "Get connection information and code examples for a forwarded service",
		Arguments: []*mcp.PromptArgument{
			{
				Name:        "service_name",
				Description: "Name of the service to connect to",
				Required:    true,
			},
			{
				Name:        "language",
				Description: "Programming language for code examples (e.g., 'go', 'python', 'node')",
				Required:    false,
			},
		},
	}, s.handleConnectionGuidePrompt)

	// forward_namespace - Forward all services in a namespace
	s.mcpServer.AddPrompt(&mcp.Prompt{
		Name:        "forward_namespace",
		Description: "Forward all services from a Kubernetes namespace for local development",
		Arguments: []*mcp.PromptArgument{
			{
				Name:        "namespace",
				Description: "Kubernetes namespace to forward",
				Required:    true,
			},
			{
				Name:        "selector",
				Description: "Label selector to filter services (e.g., 'app=api')",
				Required:    false,
			},
		},
	}, s.handleForwardNamespacePrompt)

	// === Utility prompts ===

	// explain_status - Non-technical status explanation
	s.mcpServer.AddPrompt(&mcp.Prompt{
		Name:        "explain_status",
		Description: "Provide a clear, non-technical explanation of kubefwd's current state",
		Arguments:   []*mcp.PromptArgument{},
	}, s.handleExplainStatusPrompt)

	// troubleshoot - Systematic debugging guide
	s.mcpServer.AddPrompt(&mcp.Prompt{
		Name:        "troubleshoot",
		Description: "A systematic guide for troubleshooting kubefwd connectivity issues",
		Arguments: []*mcp.PromptArgument{
			{
				Name:        "service_name",
				Description: "Name of the service having issues (optional)",
				Required:    false,
			},
			{
				Name:        "symptom",
				Description: "Description of the problem (e.g., 'connection refused', 'timeout')",
				Required:    false,
			},
		},
	}, s.handleTroubleshootPrompt)

	// fix_errors - Step-by-step error resolution
	s.mcpServer.AddPrompt(&mcp.Prompt{
		Name:        "fix_errors",
		Description: "Guide for fixing all current errors in kubefwd",
		Arguments: []*mcp.PromptArgument{
			{
				Name:        "auto_reconnect",
				Description: "Automatically trigger reconnections (true/false)",
				Required:    false,
			},
		},
	}, s.handleFixErrorsPrompt)

	// monitor - Monitoring and observability guide
	s.mcpServer.AddPrompt(&mcp.Prompt{
		Name:        "monitor",
		Description: "Guide for monitoring kubefwd traffic and health",
		Arguments:   []*mcp.PromptArgument{},
	}, s.handleMonitorPrompt)

	// === Debugging and quick access prompts ===

	// debug_service - Systematic debugging for a specific service
	s.mcpServer.AddPrompt(&mcp.Prompt{
		Name:        "debug_service",
		Description: "Systematic debugging workflow for a specific service with HTTP traffic inspection",
		Arguments: []*mcp.PromptArgument{
			{
				Name:        "service_name",
				Description: "Name of the service to debug",
				Required:    true,
			},
			{
				Name:        "symptom",
				Description: "What's going wrong (e.g., 'no response', '500 errors', 'timeout')",
				Required:    false,
			},
		},
	}, s.handleDebugServicePrompt)

	// quick_connect - Fastest path to connect to a service
	s.mcpServer.AddPrompt(&mcp.Prompt{
		Name:        "quick_connect",
		Description: "Fastest path from 'I need to connect to X' to a working connection",
		Arguments: []*mcp.PromptArgument{
			{
				Name:        "service_name",
				Description: "Name of the service to connect to",
				Required:    true,
			},
			{
				Name:        "namespace",
				Description: "Namespace containing the service (optional, will search if not specified)",
				Required:    false,
			},
		},
	}, s.handleQuickConnectPrompt)

	// analyze_issues - AI-guided comprehensive issue resolution
	s.mcpServer.AddPrompt(&mcp.Prompt{
		Name:        "analyze_issues",
		Description: "Comprehensive analysis of all kubefwd issues with prioritized resolution steps",
		Arguments:   []*mcp.PromptArgument{},
	}, s.handleAnalyzeIssuesPrompt)
}

// Prompt handlers

// === Developer-focused prompt handlers ===

func (s *Server) handleSetupLocalDevPrompt(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	namespace := ""
	k8sContext := ""

	if req.Params.Arguments != nil {
		if v, ok := req.Params.Arguments["namespace"]; ok {
			namespace = v
		}
		if v, ok := req.Params.Arguments["context"]; ok {
			k8sContext = v
		}
	}

	content := `You are helping a developer set up their local development environment with kubefwd.

`
	if namespace != "" {
		content += fmt.Sprintf("The user wants to work with namespace: %s\n", namespace)
	}
	if k8sContext != "" {
		content += fmt.Sprintf("Using Kubernetes context: %s\n", k8sContext)
	}

	content += `
## Your Goal
Help the user forward Kubernetes services to their local machine so they can develop against real cluster services.

## Steps to Follow

1. **Discover Available Services**
   - Use 'list_k8s_namespaces' to show available namespaces
   - Use 'list_k8s_services' with the target namespace to show available services
   - Explain what services are available and their ports

2. **Start Forwarding**
   - Use 'add_namespace' to forward all services in the namespace, OR
   - Use 'add_service' to forward specific services the user needs
   - Confirm successful forwarding with 'list_services'

3. **Provide Connection Information**
   - Use 'get_connection_info' for each forwarded service
   - Show the user:
     - Local IP address (e.g., 127.1.2.3)
     - Hostnames (e.g., postgres, postgres.default, postgres.default.svc)
     - Available ports
   - Provide environment variable examples:
     ` + "```bash" + `
     export DATABASE_HOST=postgres
     export DATABASE_PORT=5432
     ` + "```" + `

4. **Verify Setup**
   - Use 'get_health' to confirm everything is working
   - Mention how to check if services are accessible (e.g., curl, psql, redis-cli)

5. **Best Practices**
   - Recommend using service hostnames (e.g., 'postgres') for easier configuration
   - Suggest adding environment variables to their .env file
   - Explain that services will auto-reconnect if pods restart

Be friendly and helpful. Remember the user may not be familiar with Kubernetes internals.`

	return &mcp.GetPromptResult{
		Description: "Set up local development environment with kubefwd",
		Messages: []*mcp.PromptMessage{
			{
				Role:    "user",
				Content: &mcp.TextContent{Text: content},
			},
		},
	}, nil
}

func (s *Server) handleConnectionGuidePrompt(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	serviceName := ""
	language := ""

	if req.Params.Arguments != nil {
		if v, ok := req.Params.Arguments["service_name"]; ok {
			serviceName = v
		}
		if v, ok := req.Params.Arguments["language"]; ok {
			language = v
		}
	}

	content := fmt.Sprintf(`You are helping a developer connect to a forwarded Kubernetes service.

**Target Service**: %s
`, serviceName)

	if language != "" {
		content += fmt.Sprintf("**Preferred Language**: %s\n", language)
	}

	content += `
## Your Goal
Provide clear, actionable connection information and code examples.

## Steps

1. **Get Connection Details**
   - Use 'get_connection_info' for the service
   - If not found, use 'find_services' to search by name
   - If not forwarded, suggest using 'add_service' first

2. **Provide Connection Information**
   Present the connection details clearly:
   - **Host**: Service hostname (e.g., postgres, redis)
   - **IP**: Local IP address (e.g., 127.1.2.3)
   - **Port**: Service port(s)
   - **Connection String**: Ready-to-use connection string

3. **Code Examples**
   Provide code examples appropriate to the service type:

   **For Databases (PostgreSQL, MySQL, MongoDB):**
   - Connection string format
   - Driver initialization code
   - Environment variable setup

   **For Redis/Cache:**
   - Client connection code
   - Cluster configuration if applicable

   **For HTTP APIs:**
   - Base URL
   - Example HTTP request
   - Health check endpoint

4. **Environment Variables**
   Suggest environment variables to add to .env:
   ` + "```bash" + `
   SERVICE_HOST=<hostname>
   SERVICE_PORT=<port>
   SERVICE_URL=<connection_string>
   ` + "```" + `

5. **Testing Connection**
   Provide a simple command to test the connection:
   - For databases: simple query
   - For Redis: PING command
   - For HTTP: curl command

Be specific to the service type and programming language requested.`

	return &mcp.GetPromptResult{
		Description: fmt.Sprintf("Connection guide for %s", serviceName),
		Messages: []*mcp.PromptMessage{
			{
				Role:    "user",
				Content: &mcp.TextContent{Text: content},
			},
		},
	}, nil
}

func (s *Server) handleForwardNamespacePrompt(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	namespace := ""
	selector := ""

	if req.Params.Arguments != nil {
		if v, ok := req.Params.Arguments["namespace"]; ok {
			namespace = v
		}
		if v, ok := req.Params.Arguments["selector"]; ok {
			selector = v
		}
	}

	content := fmt.Sprintf(`You are helping a developer forward all services from a Kubernetes namespace.

**Target Namespace**: %s
`, namespace)

	if selector != "" {
		content += fmt.Sprintf("**Label Selector**: %s\n", selector)
	}

	content += `
## Your Goal
Forward the requested namespace and provide a summary of available services.

## Steps

1. **Preview Services**
   - Use 'list_k8s_services' with namespace='` + namespace + `' to show what will be forwarded
   - List each service with its type and ports
   - Warn about any services without pod selectors (won't be forwarded)

2. **Start Forwarding**
   - Use 'add_namespace' with the specified namespace
   - If a selector was provided, include it to filter services
   - Wait for forwarding to start

3. **Confirm Success**
   - Use 'list_services' to verify services are forwarding
   - Report any errors or services that failed to forward

4. **Provide Connection Summary**
   Create a table or list of all forwarded services:
   | Service | Hostname | IP | Port |
   |---------|----------|----|----|
   | postgres | postgres.` + namespace + ` | 127.1.x.x | 5432 |
   | redis | redis.` + namespace + ` | 127.1.x.x | 6379 |
   ...

5. **Quick Start Commands**
   Provide ready-to-use commands for common services:
   ` + "```bash" + `
   # PostgreSQL
   psql -h postgres -U postgres

   # Redis
   redis-cli -h redis

   # HTTP API
   curl http://api.` + namespace + `/health
   ` + "```" + `

6. **Next Steps**
   - Explain how to use 'get_connection_info' for specific services
   - Mention that services will auto-reconnect if pods restart
   - Suggest using 'list_hostnames' to see all DNS entries

Be concise and focus on actionable information the developer needs.`

	return &mcp.GetPromptResult{
		Description: fmt.Sprintf("Forward all services from namespace %s", namespace),
		Messages: []*mcp.PromptMessage{
			{
				Role:    "user",
				Content: &mcp.TextContent{Text: content},
			},
		},
	}, nil
}

// === Utility prompt handlers ===

func (s *Server) handleTroubleshootPrompt(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	serviceName := ""
	symptom := ""

	if req.Params.Arguments != nil {
		if v, ok := req.Params.Arguments["service_name"]; ok {
			serviceName = v
		}
		if v, ok := req.Params.Arguments["symptom"]; ok {
			symptom = v
		}
	}

	content := `You are helping troubleshoot a kubefwd connectivity issue.

`
	if serviceName != "" {
		content += fmt.Sprintf("The user is having issues with service: %s\n", serviceName)
	}
	if symptom != "" {
		content += fmt.Sprintf("Reported symptom: %s\n", symptom)
	}

	content += `
Follow this systematic approach:

1. **Check Service Status**
   - Use the 'list_services' tool to see all services and their status
   - Look for the affected service's status (active/error/partial)
   - Note any error counts

2. **Get Detailed Info**
   - Use 'get_service' with the service key to see port forward details
   - Check if the pod name is valid and the connection is established
   - Review the hostnames registered for DNS resolution

3. **Diagnose Errors**
   - Use 'diagnose_errors' to get a comprehensive view of all errors
   - Look at recent logs for error patterns
   - Identify the error type (connection_refused, timeout, pod_not_found)

4. **Common Issues and Solutions**
   - **"connection refused"**: Pod may not be ready, check if pod is running and listening
   - **"timeout"**: Network policy may be blocking, check pod labels and network policies
   - **"no such host"**: DNS issue, verify /etc/hosts was updated correctly
   - **"pod not found"**: Pod was deleted, trigger a sync to discover new pods

5. **Recovery Actions**
   - Use 'reconnect_service' to trigger a reconnection attempt
   - Use 'sync_service' to refresh pod information from Kubernetes
   - For persistent issues, check kubectl logs for the target pod

Always explain what you find and suggest concrete next steps.`

	return &mcp.GetPromptResult{
		Description: "Systematic troubleshooting guide for kubefwd",
		Messages: []*mcp.PromptMessage{
			{
				Role:    "user",
				Content: &mcp.TextContent{Text: content},
			},
		},
	}, nil
}

func (s *Server) handleExplainStatusPrompt(_ context.Context, _ *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	content := `You are explaining the current state of kubefwd to someone who may not be familiar with Kubernetes or port forwarding.

First, use 'get_health' and 'list_services' to understand the current state.

Then, provide a clear explanation:

1. **What kubefwd is doing**
   - Explain that kubefwd creates network bridges between your local machine and Kubernetes services
   - Each service gets its own IP address so you can access them by name

2. **Current Status Summary**
   - How many services are being forwarded
   - Overall health status (are things working or are there problems?)
   - Total network traffic flowing through

3. **If there are issues**
   - Explain problems in simple terms
   - Avoid technical jargon where possible
   - Suggest what actions might help

Use analogies if helpful. For example:
- "kubefwd is like a bridge between your computer and the Kubernetes cluster"
- "Each service is like a phone line connecting you to a specific application"
- "An 'active' status means the connection is working, 'error' means there's a problem"
- "Bytes transferred shows how much data has flowed through the connection"

Keep your explanation friendly and accessible to non-developers.`

	return &mcp.GetPromptResult{
		Description: "Non-technical explanation of kubefwd status",
		Messages: []*mcp.PromptMessage{
			{
				Role:    "user",
				Content: &mcp.TextContent{Text: content},
			},
		},
	}, nil
}

func (s *Server) handleFixErrorsPrompt(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	autoReconnect := false
	if req.Params.Arguments != nil {
		if v, ok := req.Params.Arguments["auto_reconnect"]; ok && v == "true" {
			autoReconnect = true
		}
	}

	content := `You are helping to fix all current errors in kubefwd.

Follow this step-by-step approach:

1. **Assess the Situation**
   - Use 'diagnose_errors' to get a complete list of current errors
   - Categorize errors by type (connection issues, pod issues, network issues)

2. **Prioritize Issues**
   - Critical: Services that are completely unavailable
   - High: Services with partial availability
   - Medium: Services with intermittent issues

3. **Fix Each Issue**
   For each error, follow the appropriate remediation:

   **Connection Refused Errors:**
   - Check if the pod is running and ready
   - Verify the service port configuration
   - Use 'sync_service' to refresh pod information
   - Use 'reconnect_service' to re-establish the connection

   **Timeout Errors:**
   - Check for network policies blocking traffic
   - Verify the pod is responsive
   - Use 'reconnect_service' with patience for retry

   **Pod Not Found Errors:**
   - Use 'sync_service' with force=true to rediscover pods
   - The service selector may have changed

4. **Verify Fixes**
   - After each fix attempt, use 'get_service' to check status
   - Confirm the forward is now 'active'

`
	if autoReconnect {
		content += `5. **Auto-Reconnect Mode**
   Since auto_reconnect is enabled:
   - Use 'reconnect_all_errors' to trigger bulk reconnection
   - Wait a few seconds for connections to establish
   - Check status with 'diagnose_errors' again
`
	} else {
		content += `5. **Manual Reconnect**
   For each errored service:
   - Use 'reconnect_service' with the specific service key
   - Wait for the connection attempt
   - Verify with 'get_service'
`
	}

	content += `
6. **Report Results**
   - Summarize what was fixed
   - List any remaining issues that need manual intervention
   - Provide recommendations for preventing future errors`

	return &mcp.GetPromptResult{
		Description: "Step-by-step guide to fix kubefwd errors",
		Messages: []*mcp.PromptMessage{
			{
				Role:    "user",
				Content: &mcp.TextContent{Text: content},
			},
		},
	}, nil
}

func (s *Server) handleMonitorPrompt(_ context.Context, _ *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	content := `You are helping to monitor kubefwd traffic and health.

Use these tools to provide monitoring insights:

1. **Health Check**
   - Use 'get_health' to get overall status
   - Report the health status (healthy/degraded/unhealthy)
   - Note the uptime and version

2. **Service Overview**
   - Use 'list_services' to see all services
   - Count services by status (active/error/partial)
   - Identify any services with high error counts

3. **Traffic Analysis**
   - Use 'get_metrics' with scope='by_service' for detailed breakdown
   - Identify high-traffic services
   - Note any unusual traffic patterns
   - Report rates in human-readable format (KB/s, MB/s)

4. **Error Monitoring**
   - Use 'diagnose_errors' to check for current issues
   - Report any errors with their suggested fixes
   - Track error patterns over time

5. **Recommendations**
   Based on your analysis, provide:
   - Services that may need attention
   - Traffic optimization suggestions
   - Health improvement recommendations

Format your response as a monitoring dashboard summary, easy to scan quickly.`

	return &mcp.GetPromptResult{
		Description: "Monitoring and observability guide for kubefwd",
		Messages: []*mcp.PromptMessage{
			{
				Role:    "user",
				Content: &mcp.TextContent{Text: content},
			},
		},
	}, nil
}

// === Debugging and quick access prompt handlers ===

func (s *Server) handleDebugServicePrompt(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	serviceName := ""
	symptom := ""

	if req.Params.Arguments != nil {
		if v, ok := req.Params.Arguments["service_name"]; ok {
			serviceName = v
		}
		if v, ok := req.Params.Arguments["symptom"]; ok {
			symptom = v
		}
	}

	content := fmt.Sprintf(`You are performing a systematic debug of a Kubernetes service through kubefwd.

**Target Service**: %s
`, serviceName)

	if symptom != "" {
		content += fmt.Sprintf("**Reported Symptom**: %s\n", symptom)
	}

	content += `
## Debugging Workflow

### Step 1: Check Service Status
1. Use 'find_services' with query='` + serviceName + `' to find the service
2. If found, use 'get_service' with the full key to get detailed status
3. If not found, the service may not be forwarded - check with 'list_k8s_services'

**What to look for:**
- Is the service in 'active', 'error', or 'partial' status?
- Are there any error messages?
- What pods are backing the service?

### Step 2: Analyze HTTP Traffic (if applicable)
1. Use 'get_http_traffic' with service_key to inspect recent requests
2. Look for patterns:
   - Are requests reaching the service?
   - What status codes are being returned?
   - Are there timeout issues?

**What to look for:**
- 4xx errors (client-side issues, wrong paths)
- 5xx errors (server-side issues, service unhealthy)
- No traffic at all (DNS/routing issue)

### Step 3: Check Diagnostics
1. Use 'diagnose_errors' to get error analysis
2. For specific service issues, the error type helps identify the cause:
   - **connection_refused**: Pod not ready or wrong port
   - **timeout**: Network policy or pod unresponsive
   - **pod_not_found**: Pod deleted, needs sync

### Step 4: Examine History (for recurring issues)
1. Use 'get_history' with type='errors' to see past errors
2. Use 'get_history' with type='reconnections' and service_key to see reconnection attempts
3. Look for patterns - is this a recurring issue?

### Step 5: Take Action
Based on findings:
- **If pod not ready**: Wait for pod to be ready, use 'sync_service'
- **If connection issues**: Use 'reconnect_service' to retry
- **If wrong port/config**: Service configuration may need review
- **If no pods**: Use 'sync_service' with force=true

### Step 6: Verify Fix
1. After taking action, use 'get_service' to confirm status is 'active'
2. Use 'get_http_traffic' to confirm traffic is flowing
3. Report the resolution

Be methodical and report findings at each step. Provide actionable recommendations.`

	return &mcp.GetPromptResult{
		Description: fmt.Sprintf("Debug service: %s", serviceName),
		Messages: []*mcp.PromptMessage{
			{
				Role:    "user",
				Content: &mcp.TextContent{Text: content},
			},
		},
	}, nil
}

func (s *Server) handleQuickConnectPrompt(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	serviceName := ""
	namespace := ""

	if req.Params.Arguments != nil {
		if v, ok := req.Params.Arguments["service_name"]; ok {
			serviceName = v
		}
		if v, ok := req.Params.Arguments["namespace"]; ok {
			namespace = v
		}
	}

	content := fmt.Sprintf(`You are helping the user connect to a Kubernetes service as fast as possible.

**Target Service**: %s
`, serviceName)

	if namespace != "" {
		content += fmt.Sprintf("**Namespace**: %s\n", namespace)
	}

	content += `
## Quick Connect Workflow

### Step 1: Find the Service (< 5 seconds)
`
	if namespace != "" {
		content += fmt.Sprintf(`Use 'list_k8s_services' with namespace='%s' to find the service.
`, namespace)
	} else {
		content += `Use 'find_services' with query='` + serviceName + `' to search forwarded services.
If not found, use 'list_k8s_namespaces' then 'list_k8s_services' to find it.
`
	}

	content += `
### Step 2: Forward if Needed
If the service is not already forwarded:
- Use 'add_service' with namespace and service_name
- Wait for forwarding to start
- Confirm with 'get_service'

### Step 3: Get Connection Info (the deliverable)
Use 'get_connection_info' with service_name='` + serviceName + `' and provide:

**Ready-to-use connection details:**
` + "```" + `
Host:       <service-hostname>
IP:         <local-ip>
Port:       <port>
` + "```" + `

**For databases, provide connection string:**
` + "```bash" + `
# PostgreSQL
postgresql://user:pass@` + serviceName + `:5432/dbname

# MySQL
mysql://user:pass@` + serviceName + `:3306/dbname

# MongoDB
mongodb://` + serviceName + `:27017

# Redis
redis://` + serviceName + `:6379
` + "```" + `

**For HTTP services, provide:**
` + "```bash" + `
# Base URL
http://` + serviceName + `:<port>

# Quick test
curl http://` + serviceName + `/health
` + "```" + `

**Environment variables:**
` + "```bash" + `
export ` + serviceName + `_HOST=` + serviceName + `
export ` + serviceName + `_PORT=<port>
` + "```" + `

### Goal
The user should be able to copy-paste something and immediately connect. Be concise but complete.`

	return &mcp.GetPromptResult{
		Description: fmt.Sprintf("Quick connect to: %s", serviceName),
		Messages: []*mcp.PromptMessage{
			{
				Role:    "user",
				Content: &mcp.TextContent{Text: content},
			},
		},
	}, nil
}

func (s *Server) handleAnalyzeIssuesPrompt(_ context.Context, _ *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	content := `You are performing a comprehensive analysis of all kubefwd issues with prioritized resolution.

## Analysis Workflow

### Step 1: Get Full Analysis
Use 'get_analysis' to get AI-optimized analysis including:
- Overall status (healthy/degraded/unhealthy)
- All detected issues with severity
- Recommendations from the system
- Suggested actions with API endpoints

### Step 2: Quick Status Check
Use 'get_quick_status' to verify current state:
- Is it ok, issues, or error?
- How many errors?
- What's the uptime?

### Step 3: Prioritize Issues
From the analysis, organize issues by priority:

**🔴 CRITICAL (Fix First)**
- Services completely unavailable
- Multiple pods failing
- Core infrastructure services affected

**🟠 HIGH (Fix Soon)**
- Partial service availability
- Connection timeouts
- Recent failures that may cascade

**🟡 MEDIUM (Schedule Fix)**
- Intermittent issues
- Performance degradation
- Non-critical services affected

**🟢 LOW (Monitor)**
- Cosmetic issues
- Services with auto-recovery
- Already resolving

### Step 4: Resolution Plan
For each issue, provide:
1. **What's wrong**: Clear description
2. **Why it matters**: Impact on users/services
3. **How to fix**: Specific tool call or action
4. **Verification**: How to confirm it's fixed

### Step 5: Execute Fixes
For automated fixes:
- Use 'reconnect_service' for connection issues
- Use 'sync_service' for stale pod data
- Use 'reconnect_all_errors' for bulk recovery

### Step 6: Check History for Patterns
Use 'get_history' with type='errors' to identify:
- Recurring issues that need permanent fixes
- Services that fail frequently
- Time patterns (e.g., during deployments)

### Step 7: Report Summary
Provide a summary:
- Issues found: X
- Issues resolved: Y
- Manual intervention needed: Z
- Recommendations for prevention

Be thorough but actionable. Focus on getting the user's environment healthy.`

	return &mcp.GetPromptResult{
		Description: "Comprehensive issue analysis and resolution",
		Messages: []*mcp.PromptMessage{
			{
				Role:    "user",
				Content: &mcp.TextContent{Text: content},
			},
		},
	}, nil
}
