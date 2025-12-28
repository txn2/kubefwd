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
}

// Prompt handlers

// === Developer-focused prompt handlers ===

func (s *Server) handleSetupLocalDevPrompt(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
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

func (s *Server) handleConnectionGuidePrompt(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
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

func (s *Server) handleForwardNamespacePrompt(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
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

func (s *Server) handleTroubleshootPrompt(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
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

func (s *Server) handleExplainStatusPrompt(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
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

func (s *Server) handleFixErrorsPrompt(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
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

func (s *Server) handleMonitorPrompt(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
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
