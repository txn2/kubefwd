package fwdmcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/txn2/kubefwd/pkg/fwdapi/types"
)

// Tool input types

type ListServicesInput struct {
	Namespace       string `json:"namespace,omitempty" jsonschema:"Filter by namespace"`
	Status          string `json:"status,omitempty" jsonschema:"Filter by status: active, error, partial, pending, or all"`
	IncludeForwards bool   `json:"include_forwards,omitempty" jsonschema:"Include detailed port forward information"`
}

type GetServiceInput struct {
	Key string `json:"key" jsonschema:"Service key in format 'servicename.namespace.context'"`
}

type ReconnectServiceInput struct {
	Key string `json:"key" jsonschema:"Service key to reconnect"`
}

type SyncServiceInput struct {
	Key   string `json:"key" jsonschema:"Service key to sync"`
	Force bool   `json:"force,omitempty" jsonschema:"Force sync even if debounce timer hasn't expired"`
}

type GetMetricsInput struct {
	Scope      string `json:"scope,omitempty" jsonschema:"Level of detail: summary, by_service, or service_detail"`
	ServiceKey string `json:"service_key,omitempty" jsonschema:"Service key for service_detail scope"`
}

type GetLogsInput struct {
	Count  int    `json:"count,omitempty" jsonschema:"Number of log entries to return (default: 50, max: 500)"`
	Level  string `json:"level,omitempty" jsonschema:"Filter by log level: debug, info, warn, error, or all"`
	Search string `json:"search,omitempty" jsonschema:"Search term to filter log messages"`
}

// === Developer-focused tool input types ===

type AddNamespaceInput struct {
	Namespace string `json:"namespace" jsonschema:"The Kubernetes namespace to forward"`
	Context   string `json:"context,omitempty" jsonschema:"Kubernetes context (default: current context)"`
	Selector  string `json:"selector,omitempty" jsonschema:"Label selector to filter services (e.g., 'app=myapp')"`
}

type RemoveNamespaceInput struct {
	Namespace string `json:"namespace" jsonschema:"The namespace to stop forwarding"`
	Context   string `json:"context,omitempty" jsonschema:"Kubernetes context (default: current context)"`
}

type AddServiceInput struct {
	Namespace   string   `json:"namespace" jsonschema:"The Kubernetes namespace"`
	ServiceName string   `json:"service_name" jsonschema:"Name of the service to forward"`
	Context     string   `json:"context,omitempty" jsonschema:"Kubernetes context (default: current context)"`
	Ports       []string `json:"ports,omitempty" jsonschema:"Specific ports to forward (optional)"`
}

type RemoveServiceInput struct {
	Key string `json:"key" jsonschema:"Service key (servicename.namespace.context) or just servicename if unambiguous"`
}

type GetConnectionInfoInput struct {
	ServiceName string `json:"service_name" jsonschema:"Service name to get connection info for"`
	Namespace   string `json:"namespace,omitempty" jsonschema:"Namespace (optional if service name is unambiguous)"`
	Context     string `json:"context,omitempty" jsonschema:"Kubernetes context (default: current context)"`
	Port        int    `json:"port,omitempty" jsonschema:"Specific port to get info for (optional)"`
}

type ListK8sNamespacesInput struct {
	Context string `json:"context,omitempty" jsonschema:"Kubernetes context (default: current context)"`
}

type ListK8sServicesInput struct {
	Namespace string `json:"namespace" jsonschema:"Kubernetes namespace to list services from"`
	Context   string `json:"context,omitempty" jsonschema:"Kubernetes context (default: current context)"`
}

type FindServicesInput struct {
	Query     string `json:"query,omitempty" jsonschema:"Search query to match service names"`
	Port      int    `json:"port,omitempty" jsonschema:"Filter by port number"`
	Namespace string `json:"namespace,omitempty" jsonschema:"Filter by namespace"`
}

// === HTTP traffic and history tool input types ===

type GetHTTPTrafficInput struct {
	ServiceKey string `json:"service_key,omitempty" jsonschema:"Service key to get HTTP traffic for (optional)"`
	ForwardKey string `json:"forward_key,omitempty" jsonschema:"Forward key to get HTTP traffic for (optional)"`
	Count      int    `json:"count,omitempty" jsonschema:"Number of log entries to return (default: 50, max: 500)"`
}

type GetHistoryInput struct {
	Type       string `json:"type" jsonschema:"Type of history: events, errors, or reconnections"`
	ServiceKey string `json:"service_key,omitempty" jsonschema:"Filter by service key (optional)"`
	EventType  string `json:"event_type,omitempty" jsonschema:"Filter events by type (e.g., ForwardError, ServiceAdded)"`
	Count      int    `json:"count,omitempty" jsonschema:"Number of entries to return (default varies by type)"`
}

// registerTools registers all MCP tools
func (s *Server) registerTools() {
	// === Developer-focused tools (primary use cases) ===

	// add_namespace - Forward all services in a namespace
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "add_namespace",
		Description: "Forward all services in a namespace to localhost. Adds /etc/hosts entries for each service. Returns list of discovered services with connection info.",
	}, s.handleAddNamespace)

	// remove_namespace - Stop all forwards in a namespace
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "remove_namespace",
		Description: "Stop all service forwards in a namespace. Removes /etc/hosts entries and cleans up network interfaces.",
	}, s.handleRemoveNamespace)

	// add_service - Forward a single service
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "add_service",
		Description: "Forward a single service to localhost. Returns local IP, hostnames (added to /etc/hosts), and mapped ports.",
	}, s.handleAddService)

	// remove_service - Stop forwarding a service
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "remove_service",
		Description: "Stop forwarding a service. Removes /etc/hosts entries and releases the allocated IP.",
	}, s.handleRemoveService)

	// get_connection_info - Get connection details
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_connection_info",
		Description: "Get connection details for a forwarded service: IP address, hostnames, ports, and environment variables. Use after add_service to get ready-to-use connection strings.",
	}, s.handleGetConnectionInfo)

	// list_k8s_namespaces - Discover namespaces
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_k8s_namespaces",
		Description: "List available Kubernetes namespaces with forwarding status. Use to discover what can be forwarded.",
	}, s.handleListK8sNamespaces)

	// list_k8s_services - Discover services
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_k8s_services",
		Description: "List services in a namespace with type, ports, and forwarding status. Use to discover what can be forwarded.",
	}, s.handleListK8sServices)

	// find_services - Search forwards
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "find_services",
		Description: "Search forwarded services by name pattern, port number, or namespace. Returns matching services with connection info.",
	}, s.handleFindServices)

	// list_hostnames - View /etc/hosts entries
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_hostnames",
		Description: "List all /etc/hosts entries created by kubefwd. Shows hostname-to-IP mappings for each forwarded service.",
	}, s.handleListHostnames)

	// === Service inspection and management tools ===

	// list_services - List forwarded services
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_services",
		Description: "List all forwarded services with status. Returns service names, namespaces, contexts, forward counts, and error states.",
	}, s.handleListServices)

	// get_service - Get service details
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_service",
		Description: "Get detailed service info: all port forwards, pods, hostnames, traffic bytes/rates, and error state.",
	}, s.handleGetService)

	// reconnect_service - Force reconnection
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "reconnect_service",
		Description: "Reconnect a service to its pods. Use when service is in error state. Forces immediate reconnection attempt.",
	}, s.handleReconnectService)

	// reconnect_all_errors - Batch reconnect
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "reconnect_all_errors",
		Description: "Reconnect all errored services at once. Returns count of services triggered.",
	}, s.handleReconnectAllErrors)

	// sync_service - Re-sync pods
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "sync_service",
		Description: "Re-sync pods for a service. Discovers pod changes and updates forwards. Use after deployments or pod restarts.",
	}, s.handleSyncService)

	// get_health - Get health status
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_health",
		Description: "Get kubefwd health: status (healthy/degraded/unhealthy), version, uptime, and service/error counts.",
	}, s.handleGetHealth)

	// get_metrics - Get traffic metrics
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_metrics",
		Description: "Get bandwidth metrics: total bytes in/out, transfer rates, per-service breakdown. Shows network activity.",
	}, s.handleGetMetrics)

	// diagnose_errors - Analyze errors
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "diagnose_errors",
		Description: "Diagnose current errors with root cause analysis. Returns error types, affected pods, and specific fix suggestions.",
	}, s.handleDiagnoseErrors)

	// get_logs - Access logs
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_logs",
		Description: "Get kubefwd log entries. Filter by level (debug/info/warn/error) or search text. Returns timestamped messages.",
	}, s.handleGetLogs)

	// === AI-optimized analysis tools ===

	// get_analysis - Full analysis with recommendations
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_analysis",
		Description: "Full analysis with issues, priorities, and recommended actions. Returns classified errors and specific tool calls to fix them. Best starting point for troubleshooting.",
	}, s.handleGetAnalysis)

	// get_quick_status - Fast health check
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_quick_status",
		Description: "Fast health check returning ok/issues/error with message. Use as first check before taking action.",
	}, s.handleGetQuickStatus)

	// get_http_traffic - View HTTP requests
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_http_traffic",
		Description: "View HTTP requests through a forward: method, path, status code, response time. Debug API calls and verify connectivity.",
	}, s.handleGetHTTPTraffic)

	// list_contexts - List clusters
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_contexts",
		Description: "List Kubernetes contexts from kubeconfig with current context marked. Use to discover clusters before forwarding.",
	}, s.handleListContexts)

	// get_history - Access history
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_history",
		Description: "Access event, error, or reconnection history. Filter by type and service key. Analyze patterns over time.",
	}, s.handleGetHistory)
}

// Tool handlers

func (s *Server) handleListServices(ctx context.Context, req *mcp.CallToolRequest, input ListServicesInput) (*mcp.CallToolResult, any, error) {
	state := s.getState()
	if state == nil {
		return nil, nil, fmt.Errorf("state reader not available")
	}

	services := state.GetServices()
	summary := state.GetSummary()

	// Apply filters
	var filtered []map[string]interface{}
	for _, svc := range services {
		// Calculate status
		status := "pending"
		if svc.ActiveCount > 0 && svc.ErrorCount == 0 {
			status = "active"
		} else if svc.ErrorCount > 0 && svc.ActiveCount == 0 {
			status = "error"
		} else if svc.ErrorCount > 0 && svc.ActiveCount > 0 {
			status = "partial"
		}

		// Apply namespace filter
		if input.Namespace != "" && svc.Namespace != input.Namespace {
			continue
		}

		// Apply status filter
		if input.Status != "" && input.Status != "all" && status != input.Status {
			continue
		}

		svcData := map[string]interface{}{
			"key":           svc.Key,
			"serviceName":   svc.ServiceName,
			"namespace":     svc.Namespace,
			"context":       svc.Context,
			"headless":      svc.Headless,
			"status":        status,
			"activeCount":   svc.ActiveCount,
			"errorCount":    svc.ErrorCount,
			"totalBytesIn":  svc.TotalBytesIn,
			"totalBytesOut": svc.TotalBytesOut,
		}

		if input.IncludeForwards {
			forwards := make([]map[string]interface{}, len(svc.PortForwards))
			for i, fwd := range svc.PortForwards {
				forwards[i] = map[string]interface{}{
					"podName":   fwd.PodName,
					"localIP":   fwd.LocalIP,
					"localPort": fwd.LocalPort,
					"podPort":   fwd.PodPort,
					"hostnames": fwd.Hostnames,
					"status":    fwd.Status.String(),
					"error":     fwd.Error,
				}
			}
			svcData["forwards"] = forwards
		}

		filtered = append(filtered, svcData)
	}

	result := map[string]interface{}{
		"services": filtered,
		"summary": map[string]interface{}{
			"totalServices":  summary.TotalServices,
			"activeServices": summary.ActiveServices,
			"totalForwards":  summary.TotalForwards,
			"activeForwards": summary.ActiveForwards,
			"errorCount":     summary.ErrorCount,
		},
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Found %d services (%d active, %d with errors)",
				len(filtered), summary.ActiveServices, summary.ErrorCount)},
		},
	}, result, nil
}

func (s *Server) handleGetService(ctx context.Context, req *mcp.CallToolRequest, input GetServiceInput) (*mcp.CallToolResult, any, error) {
	state := s.getState()
	if state == nil {
		return nil, nil, NewProviderUnavailableError("State reader", "start kubefwd with: sudo -E kubefwd")
	}

	svc := state.GetService(input.Key)
	if svc == nil {
		return nil, nil, NewServiceNotFoundError(input.Key)
	}

	// Calculate status
	status := "pending"
	if svc.ActiveCount > 0 && svc.ErrorCount == 0 {
		status = "active"
	} else if svc.ErrorCount > 0 && svc.ActiveCount == 0 {
		status = "error"
	} else if svc.ErrorCount > 0 && svc.ActiveCount > 0 {
		status = "partial"
	}

	forwards := make([]map[string]interface{}, len(svc.PortForwards))
	for i, fwd := range svc.PortForwards {
		forwards[i] = map[string]interface{}{
			"key":           fwd.Key,
			"podName":       fwd.PodName,
			"containerName": fwd.ContainerName,
			"localIP":       fwd.LocalIP,
			"localPort":     fwd.LocalPort,
			"podPort":       fwd.PodPort,
			"hostnames":     fwd.Hostnames,
			"status":        fwd.Status.String(),
			"error":         fwd.Error,
			"startedAt":     fwd.StartedAt,
			"lastActive":    fwd.LastActive,
			"bytesIn":       fwd.BytesIn,
			"bytesOut":      fwd.BytesOut,
			"rateIn":        fwd.RateIn,
			"rateOut":       fwd.RateOut,
		}
	}

	result := map[string]interface{}{
		"key":           svc.Key,
		"serviceName":   svc.ServiceName,
		"namespace":     svc.Namespace,
		"context":       svc.Context,
		"headless":      svc.Headless,
		"status":        status,
		"activeCount":   svc.ActiveCount,
		"errorCount":    svc.ErrorCount,
		"totalBytesIn":  svc.TotalBytesIn,
		"totalBytesOut": svc.TotalBytesOut,
		"forwards":      forwards,
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Service %s: %s (%d forwards, %d active, %d errors)",
				svc.ServiceName, status, len(svc.PortForwards), svc.ActiveCount, svc.ErrorCount)},
		},
	}, result, nil
}

func (s *Server) handleDiagnoseErrors(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error) {
	state := s.getState()
	if state == nil {
		return nil, nil, fmt.Errorf("state reader not available")
	}

	services := state.GetServices()
	logs := state.GetLogs(20)

	var errors []map[string]interface{}
	for _, svc := range services {
		if svc.ErrorCount == 0 {
			continue
		}

		for _, fwd := range svc.PortForwards {
			if fwd.Error == "" {
				continue
			}

			// Analyze error type and provide suggestion
			errorType := "unknown"
			suggestion := "Check pod status and logs"

			errLower := strings.ToLower(fwd.Error)
			if strings.Contains(errLower, "connection refused") {
				errorType = "connection_refused"
				suggestion = "Pod may not be ready or listening on the expected port. Check pod logs and readiness probes."
			} else if strings.Contains(errLower, "timeout") {
				errorType = "timeout"
				suggestion = "Connection timed out. Check network policies and pod availability."
			} else if strings.Contains(errLower, "not found") {
				errorType = "pod_not_found"
				suggestion = "Pod no longer exists. Trigger a sync to discover new pods."
			}

			errors = append(errors, map[string]interface{}{
				"serviceKey":   svc.Key,
				"serviceName":  svc.ServiceName,
				"namespace":    svc.Namespace,
				"podName":      fwd.PodName,
				"errorType":    errorType,
				"errorMessage": fwd.Error,
				"suggestion":   suggestion,
			})
		}
	}

	// Determine overall health
	health := "healthy"
	if len(errors) > 0 {
		health = "degraded"
		if len(errors) > 5 {
			health = "unhealthy"
		}
	}

	// Build recent logs
	recentLogs := make([]map[string]interface{}, len(logs))
	for i, log := range logs {
		recentLogs[i] = map[string]interface{}{
			"timestamp": log.Timestamp,
			"level":     log.Level,
			"message":   log.Message,
		}
	}

	result := map[string]interface{}{
		"errorCount":    len(errors),
		"errors":        errors,
		"recentLogs":    recentLogs,
		"overallHealth": health,
	}

	summary := fmt.Sprintf("Found %d errors. Overall health: %s", len(errors), health)
	if len(errors) > 0 {
		summary += fmt.Sprintf(". Most common issue: %s", errors[0]["errorType"])
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: summary},
		},
	}, result, nil
}

func (s *Server) handleReconnectService(ctx context.Context, req *mcp.CallToolRequest, input ReconnectServiceInput) (*mcp.CallToolResult, any, error) {
	controller := s.getController()
	if controller == nil {
		return nil, nil, NewProviderUnavailableError("Service controller", "start kubefwd with: sudo -E kubefwd")
	}

	if input.Key == "" {
		return nil, nil, NewInvalidInputError("key", "", "service key is required")
	}

	if err := controller.Reconnect(input.Key); err != nil {
		return nil, nil, ClassifyError(err, map[string]interface{}{"service": input.Key})
	}

	result := map[string]interface{}{
		"success": true,
		"service": input.Key,
		"message": "Reconnection triggered successfully",
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Reconnection triggered for service: %s", input.Key)},
		},
	}, result, nil
}

func (s *Server) handleReconnectAllErrors(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error) {
	controller := s.getController()
	if controller == nil {
		return nil, nil, NewProviderUnavailableError("Service controller", "start kubefwd with: sudo -E kubefwd")
	}

	count := controller.ReconnectAll()

	result := map[string]interface{}{
		"success":   true,
		"triggered": count,
		"message":   fmt.Sprintf("Reconnection triggered for %d services", count),
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Triggered reconnection for %d errored services", count)},
		},
	}, result, nil
}

func (s *Server) handleGetMetrics(ctx context.Context, req *mcp.CallToolRequest, input GetMetricsInput) (*mcp.CallToolResult, any, error) {
	metrics := s.getMetrics()
	state := s.getState()
	manager := s.getManager()

	if metrics == nil || state == nil {
		return nil, nil, fmt.Errorf("metrics or state not available")
	}

	bytesIn, bytesOut, rateIn, rateOut := metrics.GetTotals()
	summary := state.GetSummary()

	uptime := ""
	if manager != nil {
		uptime = manager.Uptime().Round(time.Second).String()
	}

	result := map[string]interface{}{
		"totalServices":  summary.TotalServices,
		"activeServices": summary.ActiveServices,
		"totalForwards":  summary.TotalForwards,
		"activeForwards": summary.ActiveForwards,
		"errorCount":     summary.ErrorCount,
		"totalBytesIn":   bytesIn,
		"totalBytesOut":  bytesOut,
		"totalRateIn":    rateIn,
		"totalRateOut":   rateOut,
		"uptime":         uptime,
	}

	// Add per-service breakdown if requested
	if input.Scope == "by_service" || input.Scope == "service_detail" {
		snapshots := metrics.GetAllSnapshots()
		services := make([]map[string]interface{}, len(snapshots))
		for i, svc := range snapshots {
			services[i] = map[string]interface{}{
				"key":           svc.ServiceName + "." + svc.Namespace + "." + svc.Context,
				"serviceName":   svc.ServiceName,
				"namespace":     svc.Namespace,
				"totalBytesIn":  svc.TotalBytesIn,
				"totalBytesOut": svc.TotalBytesOut,
				"rateIn":        svc.TotalRateIn,
				"rateOut":       svc.TotalRateOut,
			}
		}
		result["services"] = services
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Metrics: %d services, %d forwards, %.2f KB/s in, %.2f KB/s out",
				summary.TotalServices, summary.TotalForwards, rateIn/1024, rateOut/1024)},
		},
	}, result, nil
}

func (s *Server) handleGetLogs(ctx context.Context, req *mcp.CallToolRequest, input GetLogsInput) (*mcp.CallToolResult, any, error) {
	state := s.getState()
	if state == nil {
		return nil, nil, fmt.Errorf("state reader not available")
	}

	count := input.Count
	if count <= 0 {
		count = 50
	}
	if count > 500 {
		count = 500
	}

	logs := state.GetLogs(count)

	// Apply filters
	var filtered []map[string]interface{}
	for _, log := range logs {
		// Apply level filter
		if input.Level != "" && input.Level != "all" && strings.ToLower(log.Level) != input.Level {
			continue
		}

		// Apply search filter
		if input.Search != "" && !strings.Contains(strings.ToLower(log.Message), strings.ToLower(input.Search)) {
			continue
		}

		filtered = append(filtered, map[string]interface{}{
			"timestamp": log.Timestamp,
			"level":     log.Level,
			"message":   log.Message,
		})
	}

	result := map[string]interface{}{
		"logs":  filtered,
		"count": len(filtered),
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Retrieved %d log entries", len(filtered))},
		},
	}, result, nil
}

func (s *Server) handleGetHealth(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error) {
	state := s.getState()
	manager := s.getManager()

	if state == nil {
		return nil, nil, fmt.Errorf("state reader not available")
	}

	summary := state.GetSummary()

	// Determine health status
	status := "healthy"
	if summary.ErrorCount > 0 {
		status = "degraded"
		if summary.ErrorCount > summary.ActiveServices {
			status = "unhealthy"
		}
	}

	result := map[string]interface{}{
		"status":         status,
		"version":        s.version,
		"totalServices":  summary.TotalServices,
		"activeServices": summary.ActiveServices,
		"errorCount":     summary.ErrorCount,
	}

	if manager != nil {
		result["uptime"] = manager.Uptime().Round(time.Second).String()
		result["startTime"] = manager.StartTime()
		result["namespaces"] = manager.Namespaces()
		result["contexts"] = manager.Contexts()
		result["tuiEnabled"] = manager.TUIEnabled()
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("kubefwd %s: %s (%d services, %d errors)",
				s.version, status, summary.TotalServices, summary.ErrorCount)},
		},
	}, result, nil
}

func (s *Server) handleSyncService(ctx context.Context, req *mcp.CallToolRequest, input SyncServiceInput) (*mcp.CallToolResult, any, error) {
	controller := s.getController()
	if controller == nil {
		return nil, nil, NewProviderUnavailableError("Service controller", "start kubefwd with: sudo -E kubefwd")
	}

	if input.Key == "" {
		return nil, nil, NewInvalidInputError("key", "", "service key is required")
	}

	if err := controller.Sync(input.Key, input.Force); err != nil {
		return nil, nil, ClassifyError(err, map[string]interface{}{"service": input.Key})
	}

	result := map[string]interface{}{
		"success": true,
		"service": input.Key,
		"force":   input.Force,
		"message": "Pod sync triggered successfully",
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Pod sync triggered for service: %s (force=%v)", input.Key, input.Force)},
		},
	}, result, nil
}

// === Developer-focused tool handlers ===

func (s *Server) handleAddNamespace(ctx context.Context, req *mcp.CallToolRequest, input AddNamespaceInput) (*mcp.CallToolResult, any, error) {
	nsController := s.getNamespaceController()
	if nsController == nil {
		return nil, nil, NewProviderUnavailableError("Namespace controller", "start kubefwd with: sudo -E kubefwd")
	}

	// Validate input
	if input.Namespace == "" {
		return nil, nil, NewInvalidInputError("namespace", "", "namespace name is required")
	}

	k8sContext := input.Context
	if k8sContext == "" {
		k8sContext = s.getCurrentContext()
		if k8sContext == "" {
			return nil, nil, NewInvalidInputError("context", "", "could not determine current context; please specify context explicitly")
		}
	}

	opts := types.AddNamespaceOpts{
		LabelSelector: input.Selector,
	}

	info, err := nsController.AddNamespace(k8sContext, input.Namespace, opts)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to add namespace: %w", err)
	}

	result := map[string]interface{}{
		"success":      true,
		"key":          info.Key,
		"namespace":    info.Namespace,
		"context":      info.Context,
		"serviceCount": info.ServiceCount,
		"message":      fmt.Sprintf("Started forwarding namespace %s", input.Namespace),
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Started forwarding namespace %s. Discovered %d services.",
				input.Namespace, info.ServiceCount)},
		},
	}, result, nil
}

func (s *Server) handleRemoveNamespace(ctx context.Context, req *mcp.CallToolRequest, input RemoveNamespaceInput) (*mcp.CallToolResult, any, error) {
	nsController := s.getNamespaceController()
	if nsController == nil {
		return nil, nil, NewProviderUnavailableError("Namespace controller", "start kubefwd with: sudo -E kubefwd")
	}

	if input.Namespace == "" {
		return nil, nil, NewInvalidInputError("namespace", "", "namespace name is required")
	}

	k8sContext := input.Context
	if k8sContext == "" {
		k8sContext = s.getCurrentContext()
		if k8sContext == "" {
			return nil, nil, NewInvalidInputError("context", "", "could not determine current context; please specify context explicitly")
		}
	}

	if err := nsController.RemoveNamespace(k8sContext, input.Namespace); err != nil {
		return nil, nil, fmt.Errorf("failed to remove namespace: %w", err)
	}

	result := map[string]interface{}{
		"success":   true,
		"namespace": input.Namespace,
		"context":   k8sContext,
		"message":   "Namespace watcher stopped and services removed",
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Stopped forwarding namespace %s", input.Namespace)},
		},
	}, result, nil
}

func (s *Server) handleAddService(ctx context.Context, req *mcp.CallToolRequest, input AddServiceInput) (*mcp.CallToolResult, any, error) {
	svcCRUD := s.getServiceCRUD()
	if svcCRUD == nil {
		return nil, nil, NewProviderUnavailableError("Service CRUD controller", "start kubefwd with: sudo -E kubefwd")
	}

	// Validate required inputs
	if input.Namespace == "" {
		return nil, nil, NewInvalidInputError("namespace", "", "namespace name is required")
	}
	if input.ServiceName == "" {
		return nil, nil, NewInvalidInputError("service_name", "", "service name is required")
	}

	k8sContext := input.Context
	if k8sContext == "" {
		k8sContext = s.getCurrentContext()
		if k8sContext == "" {
			return nil, nil, NewInvalidInputError("context", "", "could not determine current context; please specify context explicitly")
		}
	}

	addReq := types.AddServiceRequest{
		Namespace:   input.Namespace,
		ServiceName: input.ServiceName,
		Context:     k8sContext,
		Ports:       input.Ports,
	}

	resp, err := svcCRUD.AddService(addReq)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to add service: %w", err)
	}

	result := map[string]interface{}{
		"success":     true,
		"key":         resp.Key,
		"serviceName": resp.ServiceName,
		"namespace":   resp.Namespace,
		"context":     resp.Context,
		"localIP":     resp.LocalIP,
		"hostnames":   resp.Hostnames,
		"ports":       resp.Ports,
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Forwarding service %s. Connect via %s or hostnames: %v",
				resp.ServiceName, resp.LocalIP, resp.Hostnames)},
		},
	}, result, nil
}

func (s *Server) handleRemoveService(ctx context.Context, req *mcp.CallToolRequest, input RemoveServiceInput) (*mcp.CallToolResult, any, error) {
	svcCRUD := s.getServiceCRUD()
	if svcCRUD == nil {
		return nil, nil, NewProviderUnavailableError("Service CRUD controller", "start kubefwd with: sudo -E kubefwd")
	}

	if input.Key == "" {
		return nil, nil, NewInvalidInputError("key", "", "service key is required")
	}

	if err := svcCRUD.RemoveService(input.Key); err != nil {
		return nil, nil, ClassifyError(err, map[string]interface{}{"service": input.Key})
	}

	result := map[string]interface{}{
		"success": true,
		"key":     input.Key,
		"message": "Service forwarding stopped",
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Stopped forwarding service: %s", input.Key)},
		},
	}, result, nil
}

func (s *Server) handleGetConnectionInfo(ctx context.Context, req *mcp.CallToolRequest, input GetConnectionInfoInput) (*mcp.CallToolResult, any, error) {
	connInfo := s.getConnectionInfo()
	if connInfo == nil {
		// Fallback to state reader if connection info provider not available
		state := s.getState()
		if state == nil {
			return nil, nil, fmt.Errorf("connection info not available")
		}

		// Search for the service
		services := state.GetServices()
		for _, svc := range services {
			if svc.ServiceName == input.ServiceName {
				if input.Namespace != "" && svc.Namespace != input.Namespace {
					continue
				}
				if input.Context != "" && svc.Context != input.Context {
					continue
				}

				// Build connection info from service state
				var ports []map[string]interface{}
				var hostnames []string
				var localIP string

				for _, fwd := range svc.PortForwards {
					if localIP == "" {
						localIP = fwd.LocalIP
					}
					hostnames = append(hostnames, fwd.Hostnames...)
					ports = append(ports, map[string]interface{}{
						"localPort":  fwd.LocalPort,
						"remotePort": fwd.PodPort,
					})
				}

				result := map[string]interface{}{
					"service":   svc.ServiceName,
					"namespace": svc.Namespace,
					"context":   svc.Context,
					"localIP":   localIP,
					"hostnames": unique(hostnames),
					"ports":     ports,
					"status":    "active",
				}

				return &mcp.CallToolResult{
					Content: []mcp.Content{
						&mcp.TextContent{Text: fmt.Sprintf("Service %s available at %s. Hostnames: %v",
							svc.ServiceName, localIP, unique(hostnames))},
					},
				}, result, nil
			}
		}

		return nil, nil, fmt.Errorf("service not found: %s", input.ServiceName)
	}

	// Build key from input: service.namespace.context
	key := input.ServiceName
	if input.Namespace != "" {
		key = input.ServiceName + "." + input.Namespace
		// Add context if specified, or use current context
		context := input.Context
		if context == "" {
			context = s.getCurrentContext()
		}
		if context != "" {
			key = key + "." + context
		}
	}

	info, err := connInfo.GetConnectionInfo(key)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get connection info: %w", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Service %s available at %s. Hostnames: %v. Ports: %v",
				info.Service, info.LocalIP, info.Hostnames, info.Ports)},
		},
	}, info, nil
}

func (s *Server) handleListK8sNamespaces(ctx context.Context, req *mcp.CallToolRequest, input ListK8sNamespacesInput) (*mcp.CallToolResult, any, error) {
	k8s := s.getK8sDiscovery()
	if k8s == nil {
		return nil, nil, NewProviderUnavailableError("Kubernetes discovery", "start kubefwd with: sudo -E kubefwd")
	}

	context := input.Context
	if context == "" {
		context = s.getCurrentContext()
		if context == "" {
			return nil, nil, NewInvalidInputError("context", "", "could not determine current context; please specify context explicitly")
		}
	}

	namespaces, err := k8s.ListNamespaces(context)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list namespaces: %w", err)
	}

	result := map[string]interface{}{
		"namespaces": namespaces,
		"count":      len(namespaces),
	}

	// Count forwarded
	forwarded := 0
	for _, ns := range namespaces {
		if ns.Forwarded {
			forwarded++
		}
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Found %d namespaces (%d currently forwarding)",
				len(namespaces), forwarded)},
		},
	}, result, nil
}

func (s *Server) handleListK8sServices(ctx context.Context, req *mcp.CallToolRequest, input ListK8sServicesInput) (*mcp.CallToolResult, any, error) {
	k8s := s.getK8sDiscovery()
	if k8s == nil {
		return nil, nil, NewProviderUnavailableError("Kubernetes discovery", "start kubefwd with: sudo -E kubefwd")
	}

	if input.Namespace == "" {
		return nil, nil, NewInvalidInputError("namespace", "", "namespace name is required")
	}

	context := input.Context
	if context == "" {
		context = s.getCurrentContext()
		if context == "" {
			return nil, nil, NewInvalidInputError("context", "", "could not determine current context; please specify context explicitly")
		}
	}

	services, err := k8s.ListServices(context, input.Namespace)
	if err != nil {
		return nil, nil, ClassifyError(err, map[string]interface{}{"namespace": input.Namespace, "context": context})
	}

	result := map[string]interface{}{
		"services":  services,
		"namespace": input.Namespace,
		"count":     len(services),
	}

	// Count forwarded
	forwarded := 0
	for _, svc := range services {
		if svc.Forwarded {
			forwarded++
		}
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Found %d services in namespace %s (%d currently forwarding)",
				len(services), input.Namespace, forwarded)},
		},
	}, result, nil
}

func (s *Server) handleFindServices(ctx context.Context, req *mcp.CallToolRequest, input FindServicesInput) (*mcp.CallToolResult, any, error) {
	state := s.getState()
	if state == nil {
		return nil, nil, fmt.Errorf("state reader not available")
	}

	services := state.GetServices()
	var matches []map[string]interface{}

	for _, svc := range services {
		// Apply filters
		if input.Namespace != "" && svc.Namespace != input.Namespace {
			continue
		}

		if input.Query != "" && !strings.Contains(strings.ToLower(svc.ServiceName), strings.ToLower(input.Query)) {
			continue
		}

		if input.Port > 0 {
			found := false
			for _, fwd := range svc.PortForwards {
				if fwd.LocalPort == fmt.Sprintf("%d", input.Port) || fwd.PodPort == fmt.Sprintf("%d", input.Port) {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		// Build connection info
		var localIP string
		var hostnames []string
		for _, fwd := range svc.PortForwards {
			if localIP == "" {
				localIP = fwd.LocalIP
			}
			hostnames = append(hostnames, fwd.Hostnames...)
		}

		matches = append(matches, map[string]interface{}{
			"service":   svc.ServiceName,
			"namespace": svc.Namespace,
			"context":   svc.Context,
			"localIP":   localIP,
			"hostnames": unique(hostnames),
		})
	}

	result := map[string]interface{}{
		"services": matches,
		"count":    len(matches),
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Found %d matching services", len(matches))},
		},
	}, result, nil
}

func (s *Server) handleListHostnames(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error) {
	state := s.getState()
	if state == nil {
		return nil, nil, fmt.Errorf("state reader not available")
	}

	// Collect all hostnames from forwards
	services := state.GetServices()
	var hostnames []map[string]interface{}

	for _, svc := range services {
		for _, fwd := range svc.PortForwards {
			for _, hostname := range fwd.Hostnames {
				hostnames = append(hostnames, map[string]interface{}{
					"hostname":  hostname,
					"ip":        fwd.LocalIP,
					"service":   svc.ServiceName,
					"namespace": svc.Namespace,
					"context":   svc.Context,
				})
			}
		}
	}

	result := map[string]interface{}{
		"hostnames": hostnames,
		"total":     len(hostnames),
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Found %d hostnames in /etc/hosts", len(hostnames))},
		},
	}, result, nil
}

// unique returns a slice with duplicate strings removed
func unique(s []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	return result
}

// === AI-optimized analysis tool handlers ===

func (s *Server) handleGetAnalysis(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error) {
	analysis := s.getAnalysisProvider()
	if analysis == nil {
		return nil, nil, NewProviderUnavailableError("Analysis provider", "start kubefwd with: sudo -E kubefwd")
	}

	resp, err := analysis.GetAnalysis()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get analysis: %w", err)
	}

	// Build summary message
	summary := fmt.Sprintf("Status: %s. %s", resp.Status, resp.Summary)
	if len(resp.Issues) > 0 {
		summary += fmt.Sprintf(" (%d issues found)", len(resp.Issues))
	}
	if len(resp.Recommendations) > 0 {
		summary += fmt.Sprintf(" %d recommendations available.", len(resp.Recommendations))
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: summary},
		},
	}, resp, nil
}

func (s *Server) handleGetQuickStatus(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error) {
	analysis := s.getAnalysisProvider()
	if analysis == nil {
		return nil, nil, NewProviderUnavailableError("Analysis provider", "start kubefwd with: sudo -E kubefwd")
	}

	resp, err := analysis.GetQuickStatus()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get status: %w", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Status: %s - %s", resp.Status, resp.Message)},
		},
	}, resp, nil
}

func (s *Server) handleGetHTTPTraffic(ctx context.Context, req *mcp.CallToolRequest, input GetHTTPTrafficInput) (*mcp.CallToolResult, any, error) {
	httpTraffic := s.getHTTPTrafficProvider()
	if httpTraffic == nil {
		return nil, nil, NewProviderUnavailableError("HTTP traffic provider", "start kubefwd with: sudo -E kubefwd")
	}

	count := input.Count
	if count <= 0 {
		count = 50
	}

	if input.ForwardKey != "" {
		// Get traffic for a specific forward
		resp, err := httpTraffic.GetForwardHTTP(input.ForwardKey, count)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get forward HTTP traffic: %w", err)
		}

		summary := fmt.Sprintf("HTTP traffic for forward %s: %d requests", input.ForwardKey, resp.Summary.TotalRequests)
		if resp.Summary.TotalRequests > 0 {
			summary += fmt.Sprintf(" (last request: %s)", resp.Summary.LastRequest.Format(time.RFC3339))
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: summary},
			},
		}, resp, nil
	}

	if input.ServiceKey != "" {
		// Get traffic for a service (all forwards)
		resp, err := httpTraffic.GetServiceHTTP(input.ServiceKey, count)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get service HTTP traffic: %w", err)
		}

		summary := fmt.Sprintf("HTTP traffic for service %s: %d requests across %d forwards",
			input.ServiceKey, resp.Summary.TotalRequests, len(resp.Forwards))

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: summary},
			},
		}, resp, nil
	}

	return nil, nil, fmt.Errorf("either service_key or forward_key must be provided")
}

func (s *Server) handleListContexts(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error) {
	k8s := s.getK8sDiscovery()
	if k8s == nil {
		return nil, nil, NewProviderUnavailableError("Kubernetes discovery", "start kubefwd with: sudo -E kubefwd")
	}

	resp, err := k8s.ListContexts()
	if err != nil {
		return nil, nil, ClassifyError(err, nil)
	}

	result := map[string]interface{}{
		"contexts": resp.Contexts,
		"current":  resp.CurrentContext,
		"count":    len(resp.Contexts),
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Found %d Kubernetes contexts. Current: %s",
				len(resp.Contexts), resp.CurrentContext)},
		},
	}, result, nil
}

func (s *Server) handleGetHistory(ctx context.Context, req *mcp.CallToolRequest, input GetHistoryInput) (*mcp.CallToolResult, any, error) {
	history := s.getHistoryProvider()
	if history == nil {
		return nil, nil, NewProviderUnavailableError("History provider", "start kubefwd with: sudo -E kubefwd")
	}

	historyType := strings.ToLower(input.Type)
	if historyType == "" {
		historyType = "events"
	}

	count := input.Count
	if count <= 0 {
		switch historyType {
		case "events":
			count = 100
		case "errors":
			count = 50
		case "reconnections":
			count = 50
		default:
			count = 50
		}
	}

	switch historyType {
	case "events":
		events, err := history.GetEvents(count, input.EventType)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get events: %w", err)
		}
		result := map[string]interface{}{
			"type":   "events",
			"events": events,
			"count":  len(events),
		}
		filterMsg := ""
		if input.EventType != "" {
			filterMsg = fmt.Sprintf(" (filtered by type: %s)", input.EventType)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Retrieved %d historical events%s", len(events), filterMsg)},
			},
		}, result, nil

	case "errors":
		errors, err := history.GetErrors(count)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get errors: %w", err)
		}
		result := map[string]interface{}{
			"type":   "errors",
			"errors": errors,
			"count":  len(errors),
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Retrieved %d historical errors", len(errors))},
			},
		}, result, nil

	case "reconnections", "reconnects":
		reconnects, err := history.GetReconnects(count, input.ServiceKey)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get reconnections: %w", err)
		}
		result := map[string]interface{}{
			"type":          "reconnections",
			"reconnections": reconnects,
			"count":         len(reconnects),
		}
		filterMsg := ""
		if input.ServiceKey != "" {
			filterMsg = fmt.Sprintf(" for service %s", input.ServiceKey)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Retrieved %d reconnection records%s", len(reconnects), filterMsg)},
			},
		}, result, nil

	default:
		return nil, nil, fmt.Errorf("invalid history type: %s (valid types: events, errors, reconnections)", input.Type)
	}
}
