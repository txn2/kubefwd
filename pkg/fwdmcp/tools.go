package fwdmcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/txn2/kubefwd/pkg/fwdapi/types"
	"github.com/txn2/kubefwd/pkg/fwdtui/state"
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

type GetPodLogsInput struct {
	Namespace  string `json:"namespace" jsonschema:"The Kubernetes namespace"`
	PodName    string `json:"pod_name" jsonschema:"Name of the pod to get logs from"`
	Context    string `json:"context,omitempty" jsonschema:"Kubernetes context (default: current context)"`
	Container  string `json:"container,omitempty" jsonschema:"Container name (default: first container)"`
	TailLines  int    `json:"tail_lines,omitempty" jsonschema:"Number of lines from end (default: 100, max: 1000)"`
	SinceTime  string `json:"since_time,omitempty" jsonschema:"RFC3339 timestamp to start from (e.g., 2024-01-15T10:30:00Z)"`
	Previous   bool   `json:"previous,omitempty" jsonschema:"Get logs from previous container instance"`
	Timestamps bool   `json:"timestamps,omitempty" jsonschema:"Include timestamps in log output"`
}

type ListPodsInput struct {
	Namespace     string `json:"namespace" jsonschema:"The Kubernetes namespace"`
	Context       string `json:"context,omitempty" jsonschema:"Kubernetes context (default: current context)"`
	LabelSelector string `json:"label_selector,omitempty" jsonschema:"Label selector (e.g., 'app=nginx,version=v1')"`
	ServiceName   string `json:"service_name,omitempty" jsonschema:"Filter to pods backing this service"`
}

type GetPodInput struct {
	Namespace string `json:"namespace" jsonschema:"The Kubernetes namespace"`
	PodName   string `json:"pod_name" jsonschema:"Name of the pod"`
	Context   string `json:"context,omitempty" jsonschema:"Kubernetes context (default: current context)"`
}

type GetEventsInput struct {
	Namespace    string `json:"namespace" jsonschema:"The Kubernetes namespace"`
	Context      string `json:"context,omitempty" jsonschema:"Kubernetes context (default: current context)"`
	ResourceKind string `json:"resource_kind,omitempty" jsonschema:"Filter by resource kind (Pod, Service, Deployment, etc.)"`
	ResourceName string `json:"resource_name,omitempty" jsonschema:"Filter by resource name"`
	Limit        int    `json:"limit,omitempty" jsonschema:"Max events to return (default: 50)"`
}

type GetEndpointsInput struct {
	Namespace   string `json:"namespace" jsonschema:"The Kubernetes namespace"`
	ServiceName string `json:"service_name" jsonschema:"Name of the service"`
	Context     string `json:"context,omitempty" jsonschema:"Kubernetes context (default: current context)"`
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
		Description: "Forward a single service to localhost. Returns local IP, hostnames (added to /etc/hosts), and mapped ports. Requires namespace and service_name.",
	}, s.handleAddService)

	// remove_service - Stop forwarding a service
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "remove_service",
		Description: "Stop forwarding a service. Removes /etc/hosts entries and releases the allocated IP. Requires key (e.g., 'servicename.namespace.context').",
	}, s.handleRemoveService)

	// get_connection_info - Get connection details
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_connection_info",
		Description: "Get connection details for a forwarded service: IP address, hostnames, ports, and environment variables. Use after add_service to get ready-to-use connection strings. Requires service_name.",
	}, s.handleGetConnectionInfo)

	// list_k8s_namespaces - Discover namespaces
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_k8s_namespaces",
		Description: "List available Kubernetes namespaces with forwarding status. Use to discover what can be forwarded.",
	}, s.handleListK8sNamespaces)

	// list_k8s_services - Discover services
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_k8s_services",
		Description: "List services in a namespace with type, ports, and forwarding status. Use to discover what can be forwarded. Requires namespace.",
	}, s.handleListK8sServices)

	// get_pod_logs - Get logs from a pod
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_pod_logs",
		Description: "Get logs from a Kubernetes pod. Useful for debugging services. Returns recent log lines from the pod's container. Requires namespace and pod_name. Get pod names from list_pods or get_service output.",
	}, s.handleGetPodLogs)

	// list_pods - List pods in a namespace
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_pods",
		Description: "List Kubernetes pods with status, ready state, restarts, and age. Filter by label selector or service name. Essential for finding which pods back a service. Requires namespace.",
	}, s.handleListPods)

	// get_pod - Get detailed pod info
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_pod",
		Description: "Get detailed pod information including containers, status, conditions, resources, and events. Use to diagnose pod issues. Requires namespace and pod_name.",
	}, s.handleGetPod)

	// get_events - Get Kubernetes events
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_events",
		Description: "Get Kubernetes events for debugging. Shows scheduling, pulling, starting, killing events. Filter by resource kind (Pod, Service) and name. Critical for diagnosing startup failures.",
	}, s.handleGetEvents)

	// get_endpoints - Get service endpoints
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_endpoints",
		Description: "Get endpoints for a Kubernetes service. Shows which pods are backing the service and their ready state. Useful for debugging service-to-pod routing.",
	}, s.handleGetEndpoints)

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
		Description: "Get detailed service info: all port forwards, pods, hostnames, traffic bytes/rates, and error state. Requires key (e.g., 'servicename.namespace.context').",
	}, s.handleGetService)

	// reconnect_service - Force reconnection
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "reconnect_service",
		Description: "Reconnect a service to its pods. Use when service is in error state. Forces immediate reconnection attempt. Requires key (e.g., 'servicename.namespace.context').",
	}, s.handleReconnectService)

	// reconnect_all_errors - Batch reconnect
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "reconnect_all_errors",
		Description: "Reconnect all errored services at once. Returns count of services triggered.",
	}, s.handleReconnectAllErrors)

	// sync_service - Re-sync pods
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "sync_service",
		Description: "Re-sync pods for a service. Discovers pod changes and updates forwards. Use after deployments or pod restarts. Requires key (e.g., 'servicename.namespace.context').",
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
		Description: "View HTTP requests through a forward: method, path, status code, response time. Debug API calls and verify connectivity. Requires service_key (e.g., 'myservice.namespace.context') or forward_key.",
	}, s.handleGetHTTPTraffic)

	// list_contexts - List clusters
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_contexts",
		Description: "List Kubernetes contexts from kubeconfig with current context marked. Use to discover clusters before forwarding.",
	}, s.handleListContexts)

	// get_history - Access history
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_history",
		Description: "Access event, error, or reconnection history. Filter by type and service key. Analyze patterns over time. Requires type ('events', 'errors', or 'reconnections').",
	}, s.handleGetHistory)
}

// Tool handlers

// calculateMCPServiceStatus determines the status string for a service based on active/error counts
func calculateMCPServiceStatus(activeCount, errorCount int) string {
	switch {
	case activeCount > 0 && errorCount == 0:
		return "active"
	case errorCount > 0 && activeCount == 0:
		return "error"
	case errorCount > 0 && activeCount > 0:
		return "partial"
	default:
		return "pending"
	}
}

func (s *Server) handleListServices(_ context.Context, _ *mcp.CallToolRequest, input ListServicesInput) (*mcp.CallToolResult, any, error) {
	stateReader := s.getState()
	if stateReader == nil {
		return nil, nil, NewProviderUnavailableError("State reader", "start kubefwd with: sudo -E kubefwd")
	}

	services := stateReader.GetServices()
	summary := stateReader.GetSummary()

	// Apply filters
	var filtered []map[string]interface{}
	for _, svc := range services {
		status := calculateMCPServiceStatus(svc.ActiveCount, svc.ErrorCount)

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

	// Return nil CallToolResult to let SDK auto-populate Content with JSON
	// This ensures clients receive the full structured data, not just a summary
	return nil, result, nil
}

func (s *Server) handleGetService(_ context.Context, _ *mcp.CallToolRequest, input GetServiceInput) (*mcp.CallToolResult, any, error) {
	stateReader := s.getState()
	if stateReader == nil {
		return nil, nil, NewProviderUnavailableError("State reader", "start kubefwd with: sudo -E kubefwd")
	}

	svc := stateReader.GetService(input.Key)
	if svc == nil {
		return nil, nil, NewServiceNotFoundError(input.Key)
	}

	// Calculate status
	var status string
	switch {
	case svc.ActiveCount > 0 && svc.ErrorCount == 0:
		status = "active"
	case svc.ErrorCount > 0 && svc.ActiveCount == 0:
		status = "error"
	case svc.ErrorCount > 0 && svc.ActiveCount > 0:
		status = "partial"
	default:
		status = "pending"
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

	// Return nil to let SDK auto-populate Content with full JSON data
	return nil, result, nil
}

func (s *Server) handleDiagnoseErrors(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	stateReader := s.getState()
	if stateReader == nil {
		return nil, nil, NewProviderUnavailableError("State reader", "start kubefwd with: sudo -E kubefwd")
	}

	services := stateReader.GetServices()
	logs := stateReader.GetLogs(20)

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
			switch {
			case strings.Contains(errLower, "connection refused"):
				errorType = "connection_refused"
				suggestion = "Pod may not be ready or listening on the expected port. Check pod logs and readiness probes."
			case strings.Contains(errLower, "timeout"):
				errorType = "timeout"
				suggestion = "Connection timed out. Check network policies and pod availability."
			case strings.Contains(errLower, "not found"):
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

	// Return nil to let SDK auto-populate Content with full JSON data
	return nil, result, nil
}

func (s *Server) handleReconnectService(_ context.Context, _ *mcp.CallToolRequest, input ReconnectServiceInput) (*mcp.CallToolResult, any, error) {
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

	// Return nil to let SDK auto-populate Content with full JSON data
	return nil, result, nil
}

func (s *Server) handleReconnectAllErrors(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
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

	// Return nil to let SDK auto-populate Content with full JSON data
	return nil, result, nil
}

func buildServiceKey(serviceName, namespace, ctx string) string {
	return serviceName + "." + namespace + "." + ctx
}

func (s *Server) handleGetMetrics(_ context.Context, _ *mcp.CallToolRequest, input GetMetricsInput) (*mcp.CallToolResult, any, error) {
	metrics := s.getMetrics()
	stateReader := s.getState()
	manager := s.getManager()

	if metrics == nil || stateReader == nil {
		return nil, nil, NewProviderUnavailableError("Metrics provider", "start kubefwd with: sudo -E kubefwd")
	}

	bytesIn, bytesOut, rateIn, rateOut := metrics.GetTotals()
	summary := stateReader.GetSummary()

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
				"key":           buildServiceKey(svc.ServiceName, svc.Namespace, svc.Context),
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

	// Return nil to let SDK auto-populate Content with full JSON data
	return nil, result, nil
}

func (s *Server) handleGetLogs(_ context.Context, _ *mcp.CallToolRequest, input GetLogsInput) (*mcp.CallToolResult, any, error) {
	stateReader := s.getState()
	if stateReader == nil {
		return nil, nil, NewProviderUnavailableError("State reader", "start kubefwd with: sudo -E kubefwd")
	}

	count := input.Count
	if count <= 0 {
		count = 50
	}
	if count > 500 {
		count = 500
	}

	logs := stateReader.GetLogs(count)

	// Apply filters
	var filtered []map[string]interface{}
	for _, log := range logs {
		// Apply level filter
		if input.Level != "" && input.Level != "all" && !strings.EqualFold(log.Level, input.Level) {
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

	// Return nil to let SDK auto-populate Content with full JSON data
	return nil, result, nil
}

func (s *Server) handleGetHealth(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	stateReader := s.getState()
	manager := s.getManager()

	if stateReader == nil {
		return nil, nil, NewProviderUnavailableError("State reader", "start kubefwd with: sudo -E kubefwd")
	}

	summary := stateReader.GetSummary()

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

	// Return nil to let SDK auto-populate Content with full JSON data
	return nil, result, nil
}

func (s *Server) handleSyncService(_ context.Context, _ *mcp.CallToolRequest, input SyncServiceInput) (*mcp.CallToolResult, any, error) {
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

	// Return nil to let SDK auto-populate Content with full JSON data
	return nil, result, nil
}

// === Developer-focused tool handlers ===

func (s *Server) handleAddNamespace(_ context.Context, _ *mcp.CallToolRequest, input AddNamespaceInput) (*mcp.CallToolResult, any, error) {
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

	// Query the K8s API directly for the service count.
	// This is immediate - no need to wait for the informer.
	serviceCount := 0
	if k8s := s.getK8sDiscovery(); k8s != nil {
		if services, err := k8s.ListServices(k8sContext, input.Namespace); err == nil {
			serviceCount = len(services)
		}
	}

	result := map[string]interface{}{
		"success":      true,
		"key":          info.Key,
		"namespace":    info.Namespace,
		"context":      info.Context,
		"serviceCount": serviceCount,
		"message":      fmt.Sprintf("Started forwarding namespace %s", input.Namespace),
	}

	// Return nil to let SDK auto-populate Content with full JSON data
	return nil, result, nil
}

func (s *Server) handleRemoveNamespace(_ context.Context, _ *mcp.CallToolRequest, input RemoveNamespaceInput) (*mcp.CallToolResult, any, error) {
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

	// Return nil to let SDK auto-populate Content with full JSON data
	return nil, result, nil
}

func (s *Server) handleAddService(_ context.Context, _ *mcp.CallToolRequest, input AddServiceInput) (*mcp.CallToolResult, any, error) {
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

	// Return nil to let SDK auto-populate Content with full JSON data
	return nil, result, nil
}

func (s *Server) handleRemoveService(_ context.Context, _ *mcp.CallToolRequest, input RemoveServiceInput) (*mcp.CallToolResult, any, error) {
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

	// Return nil to let SDK auto-populate Content with full JSON data
	return nil, result, nil
}

func buildConnectionKey(serviceName, namespace, k8sContext string) string {
	key := serviceName + "." + namespace
	if k8sContext != "" {
		key = key + "." + k8sContext
	}
	return key
}

// buildConnectionInfoFromService creates a connection info map from service state
func buildConnectionInfoFromService(svc *state.ServiceSnapshot) map[string]interface{} {
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

	return map[string]interface{}{
		"service":   svc.ServiceName,
		"namespace": svc.Namespace,
		"context":   svc.Context,
		"localIP":   localIP,
		"hostnames": unique(hostnames),
		"ports":     ports,
		"status":    "active",
	}
}

// findServiceInState searches for a service in the state store
func findServiceInState(stateReader types.StateReader, serviceName, namespace, ctx string) (*state.ServiceSnapshot, error) {
	services := stateReader.GetServices()
	for i := range services {
		svc := &services[i]
		if svc.ServiceName != serviceName {
			continue
		}
		if namespace != "" && svc.Namespace != namespace {
			continue
		}
		if ctx != "" && svc.Context != ctx {
			continue
		}
		return svc, nil
	}
	return nil, fmt.Errorf("service not found: %s", serviceName)
}

// searchServicesByName finds services matching the name and filters to exact matches
func searchServicesByName(connInfo types.ConnectionInfoProvider, serviceName string, port int) ([]types.ConnectionInfoResponse, error) {
	results, err := connInfo.FindServices(serviceName, port, "")
	if err != nil {
		return nil, fmt.Errorf("failed to search for service: %w", err)
	}

	var exactMatches []types.ConnectionInfoResponse
	for _, r := range results {
		if r.Service == serviceName {
			exactMatches = append(exactMatches, r)
		}
	}
	return exactMatches, nil
}

func (s *Server) handleGetConnectionInfo(_ context.Context, _ *mcp.CallToolRequest, input GetConnectionInfoInput) (*mcp.CallToolResult, any, error) {
	connInfo := s.getConnectionInfo()
	if connInfo == nil {
		stateReader := s.getState()
		if stateReader == nil {
			return nil, nil, NewProviderUnavailableError("Connection info provider", "start kubefwd with: sudo -E kubefwd")
		}
		svc, err := findServiceInState(stateReader, input.ServiceName, input.Namespace, input.Context)
		if err != nil {
			return nil, nil, err
		}
		return nil, buildConnectionInfoFromService(svc), nil
	}

	if input.Namespace == "" {
		matches, err := searchServicesByName(connInfo, input.ServiceName, input.Port)
		if err != nil {
			return nil, nil, err
		}
		if len(matches) == 0 {
			return nil, nil, fmt.Errorf("service not found: %s", input.ServiceName)
		}
		if len(matches) == 1 {
			return nil, &matches[0], nil
		}
		var namespaces []string
		for _, r := range matches {
			namespaces = append(namespaces, r.Namespace)
		}
		return nil, nil, fmt.Errorf("multiple services found with name '%s' in namespaces: %v. Please specify namespace", input.ServiceName, namespaces)
	}

	k8sContext := input.Context
	if k8sContext == "" {
		k8sContext = s.getCurrentContext()
	}
	key := buildConnectionKey(input.ServiceName, input.Namespace, k8sContext)

	info, err := connInfo.GetConnectionInfo(key)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get connection info: %w", err)
	}
	return nil, info, nil
}

func (s *Server) handleListK8sNamespaces(_ context.Context, _ *mcp.CallToolRequest, input ListK8sNamespacesInput) (*mcp.CallToolResult, any, error) {
	k8s := s.getK8sDiscovery()
	if k8s == nil {
		return nil, nil, NewProviderUnavailableError("Kubernetes discovery", "start kubefwd with: sudo -E kubefwd")
	}

	k8sContext := input.Context
	if k8sContext == "" {
		k8sContext = s.getCurrentContext()
		if k8sContext == "" {
			return nil, nil, NewInvalidInputError("context", "", "could not determine current context; please specify context explicitly")
		}
	}

	namespaces, err := k8s.ListNamespaces(k8sContext)
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

	// Return nil to let SDK auto-populate Content with full JSON data
	return nil, result, nil
}

func (s *Server) handleListK8sServices(_ context.Context, _ *mcp.CallToolRequest, input ListK8sServicesInput) (*mcp.CallToolResult, any, error) {
	k8s := s.getK8sDiscovery()
	if k8s == nil {
		return nil, nil, NewProviderUnavailableError("Kubernetes discovery", "start kubefwd with: sudo -E kubefwd")
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

	services, err := k8s.ListServices(k8sContext, input.Namespace)
	if err != nil {
		return nil, nil, ClassifyError(err, map[string]interface{}{"namespace": input.Namespace, "context": k8sContext})
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

	// Return nil to let SDK auto-populate Content with full JSON data
	return nil, result, nil
}

func (s *Server) handleGetPodLogs(_ context.Context, _ *mcp.CallToolRequest, input GetPodLogsInput) (*mcp.CallToolResult, any, error) {
	k8s := s.getK8sDiscovery()
	if k8s == nil {
		return nil, nil, NewProviderUnavailableError("Kubernetes discovery", "start kubefwd with: sudo -E kubefwd")
	}

	if input.Namespace == "" {
		return nil, nil, NewInvalidInputError("namespace", "", "namespace is required")
	}

	if input.PodName == "" {
		return nil, nil, NewInvalidInputError("pod_name", "", "pod_name is required")
	}

	k8sContext := input.Context
	if k8sContext == "" {
		k8sContext = s.getCurrentContext()
		if k8sContext == "" {
			return nil, nil, NewInvalidInputError("context", "", "could not determine current context; please specify context explicitly")
		}
	}

	opts := types.PodLogsOptions{
		Container:  input.Container,
		TailLines:  input.TailLines,
		SinceTime:  input.SinceTime,
		Previous:   input.Previous,
		Timestamps: input.Timestamps,
	}

	logs, err := k8s.GetPodLogs(k8sContext, input.Namespace, input.PodName, opts)
	if err != nil {
		return nil, nil, ClassifyError(err, map[string]interface{}{
			"namespace": input.Namespace,
			"pod":       input.PodName,
			"context":   k8sContext,
		})
	}

	result := map[string]interface{}{
		"podName":       logs.PodName,
		"namespace":     logs.Namespace,
		"context":       logs.Context,
		"containerName": logs.ContainerName,
		"logs":          logs.Logs,
		"lineCount":     logs.LineCount,
		"truncated":     logs.Truncated,
	}

	// Return nil to let SDK auto-populate Content with full JSON data
	return nil, result, nil
}

func (s *Server) handleListPods(_ context.Context, _ *mcp.CallToolRequest, input ListPodsInput) (*mcp.CallToolResult, any, error) {
	k8s := s.getK8sDiscovery()
	if k8s == nil {
		return nil, nil, NewProviderUnavailableError("Kubernetes discovery", "start kubefwd with: sudo -E kubefwd")
	}

	if input.Namespace == "" {
		return nil, nil, NewInvalidInputError("namespace", "", "namespace is required")
	}

	k8sContext := input.Context
	if k8sContext == "" {
		k8sContext = s.getCurrentContext()
		if k8sContext == "" {
			return nil, nil, NewInvalidInputError("context", "", "could not determine current context; please specify context explicitly")
		}
	}

	opts := types.ListPodsOptions{
		LabelSelector: input.LabelSelector,
		ServiceName:   input.ServiceName,
	}

	pods, err := k8s.ListPods(k8sContext, input.Namespace, opts)
	if err != nil {
		return nil, nil, ClassifyError(err, map[string]interface{}{
			"namespace": input.Namespace,
			"context":   k8sContext,
		})
	}

	result := map[string]interface{}{
		"pods":      pods,
		"namespace": input.Namespace,
		"count":     len(pods),
	}

	return nil, result, nil
}

func (s *Server) handleGetPod(_ context.Context, _ *mcp.CallToolRequest, input GetPodInput) (*mcp.CallToolResult, any, error) {
	k8s := s.getK8sDiscovery()
	if k8s == nil {
		return nil, nil, NewProviderUnavailableError("Kubernetes discovery", "start kubefwd with: sudo -E kubefwd")
	}

	if input.Namespace == "" {
		return nil, nil, NewInvalidInputError("namespace", "", "namespace is required")
	}

	if input.PodName == "" {
		return nil, nil, NewInvalidInputError("pod_name", "", "pod_name is required")
	}

	k8sContext := input.Context
	if k8sContext == "" {
		k8sContext = s.getCurrentContext()
		if k8sContext == "" {
			return nil, nil, NewInvalidInputError("context", "", "could not determine current context; please specify context explicitly")
		}
	}

	pod, err := k8s.GetPod(k8sContext, input.Namespace, input.PodName)
	if err != nil {
		return nil, nil, ClassifyError(err, map[string]interface{}{
			"namespace": input.Namespace,
			"pod":       input.PodName,
			"context":   k8sContext,
		})
	}

	return nil, pod, nil
}

func (s *Server) handleGetEvents(_ context.Context, _ *mcp.CallToolRequest, input GetEventsInput) (*mcp.CallToolResult, any, error) {
	k8s := s.getK8sDiscovery()
	if k8s == nil {
		return nil, nil, NewProviderUnavailableError("Kubernetes discovery", "start kubefwd with: sudo -E kubefwd")
	}

	if input.Namespace == "" {
		return nil, nil, NewInvalidInputError("namespace", "", "namespace is required")
	}

	k8sContext := input.Context
	if k8sContext == "" {
		k8sContext = s.getCurrentContext()
		if k8sContext == "" {
			return nil, nil, NewInvalidInputError("context", "", "could not determine current context; please specify context explicitly")
		}
	}

	opts := types.GetEventsOptions{
		ResourceKind: input.ResourceKind,
		ResourceName: input.ResourceName,
		Limit:        input.Limit,
	}

	events, err := k8s.GetEvents(k8sContext, input.Namespace, opts)
	if err != nil {
		return nil, nil, ClassifyError(err, map[string]interface{}{
			"namespace": input.Namespace,
			"context":   k8sContext,
		})
	}

	result := map[string]interface{}{
		"events":    events,
		"namespace": input.Namespace,
		"count":     len(events),
	}

	return nil, result, nil
}

func (s *Server) handleGetEndpoints(_ context.Context, _ *mcp.CallToolRequest, input GetEndpointsInput) (*mcp.CallToolResult, any, error) {
	k8s := s.getK8sDiscovery()
	if k8s == nil {
		return nil, nil, NewProviderUnavailableError("Kubernetes discovery", "start kubefwd with: sudo -E kubefwd")
	}

	if input.Namespace == "" {
		return nil, nil, NewInvalidInputError("namespace", "", "namespace is required")
	}

	if input.ServiceName == "" {
		return nil, nil, NewInvalidInputError("service_name", "", "service_name is required")
	}

	k8sContext := input.Context
	if k8sContext == "" {
		k8sContext = s.getCurrentContext()
		if k8sContext == "" {
			return nil, nil, NewInvalidInputError("context", "", "could not determine current context; please specify context explicitly")
		}
	}

	endpoints, err := k8s.GetEndpoints(k8sContext, input.Namespace, input.ServiceName)
	if err != nil {
		return nil, nil, ClassifyError(err, map[string]interface{}{
			"namespace": input.Namespace,
			"service":   input.ServiceName,
			"context":   k8sContext,
		})
	}

	return nil, endpoints, nil
}

func (s *Server) handleFindServices(_ context.Context, _ *mcp.CallToolRequest, input FindServicesInput) (*mcp.CallToolResult, any, error) {
	stateReader := s.getState()
	if stateReader == nil {
		return nil, nil, NewProviderUnavailableError("State reader", "start kubefwd with: sudo -E kubefwd")
	}

	services := stateReader.GetServices()
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
			portStr := fmt.Sprintf("%d", input.Port)
			found := false
			for _, fwd := range svc.PortForwards {
				if fwd.LocalPort == portStr || fwd.PodPort == portStr {
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

	// Return nil to let SDK auto-populate Content with full JSON data
	return nil, result, nil
}

func (s *Server) handleListHostnames(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	stateReader := s.getState()
	if stateReader == nil {
		return nil, nil, NewProviderUnavailableError("State reader", "start kubefwd with: sudo -E kubefwd")
	}

	// Collect all hostnames from forwards
	services := stateReader.GetServices()
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

	// Return nil to let SDK auto-populate Content with full JSON data
	return nil, result, nil
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

func (s *Server) handleGetAnalysis(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	analysis := s.getAnalysisProvider()
	if analysis == nil {
		return nil, nil, NewProviderUnavailableError("Analysis provider", "start kubefwd with: sudo -E kubefwd")
	}

	resp, err := analysis.GetAnalysis()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get analysis: %w", err)
	}

	// Return nil to let SDK auto-populate Content with full JSON data
	return nil, resp, nil
}

func (s *Server) handleGetQuickStatus(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	analysis := s.getAnalysisProvider()
	if analysis == nil {
		return nil, nil, NewProviderUnavailableError("Analysis provider", "start kubefwd with: sudo -E kubefwd")
	}

	resp, err := analysis.GetQuickStatus()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get status: %w", err)
	}

	// Return nil to let SDK auto-populate Content with full JSON data
	return nil, resp, nil
}

func (s *Server) handleGetHTTPTraffic(_ context.Context, _ *mcp.CallToolRequest, input GetHTTPTrafficInput) (*mcp.CallToolResult, any, error) {
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

		// Return nil to let SDK auto-populate Content with full JSON data
		return nil, resp, nil
	}

	if input.ServiceKey != "" {
		// Get traffic for a service (all forwards)
		resp, err := httpTraffic.GetServiceHTTP(input.ServiceKey, count)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get service HTTP traffic: %w", err)
		}

		// Return nil to let SDK auto-populate Content with full JSON data
		return nil, resp, nil
	}

	return nil, nil, fmt.Errorf("either service_key or forward_key must be provided")
}

func (s *Server) handleListContexts(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
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

	// Return nil to let SDK auto-populate Content with full JSON data
	return nil, result, nil
}

func (s *Server) handleGetHistory(_ context.Context, _ *mcp.CallToolRequest, input GetHistoryInput) (*mcp.CallToolResult, any, error) {
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
		// Return nil to let SDK auto-populate Content with full JSON data
		return nil, result, nil

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
		// Return nil to let SDK auto-populate Content with full JSON data
		return nil, result, nil

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
		// Return nil to let SDK auto-populate Content with full JSON data
		return nil, result, nil

	default:
		return nil, nil, fmt.Errorf("invalid history type: %s (valid types: events, errors, reconnections)", input.Type)
	}
}
