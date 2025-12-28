package fwdmcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerPrompts registers all MCP prompt templates
func (s *Server) registerPrompts() {
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

	// explain_status - Non-technical status explanation
	s.mcpServer.AddPrompt(&mcp.Prompt{
		Name:        "explain_status",
		Description: "Provide a clear, non-technical explanation of kubefwd's current state",
		Arguments:   []*mcp.PromptArgument{},
	}, s.handleExplainStatusPrompt)

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
