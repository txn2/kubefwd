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

// registerTools registers all MCP tools
func (s *Server) registerTools() {
	// === Developer-focused tools (primary use cases) ===

	// add_namespace - Start forwarding all services in a namespace
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "add_namespace",
		Description: "Start forwarding all services in a Kubernetes namespace. Example: 'Forward all services from the staging namespace'. Returns the list of services discovered and their connection info.",
	}, s.handleAddNamespace)

	// remove_namespace - Stop forwarding a namespace
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "remove_namespace",
		Description: "Stop forwarding all services in a namespace and clean up resources. Use when done working with a namespace.",
	}, s.handleRemoveNamespace)

	// add_service - Forward a specific service
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "add_service",
		Description: "Forward a specific Kubernetes service. Example: 'Forward the postgres service from the staging namespace'. Returns connection info including local IP, hostnames, and ports.",
	}, s.handleAddService)

	// remove_service - Stop forwarding a service
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "remove_service",
		Description: "Stop forwarding a specific service. Use when done with a particular service.",
	}, s.handleRemoveService)

	// get_connection_info - Get connection details for a service
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_connection_info",
		Description: "Get local connection information for a forwarded service. Returns IP address, hostnames, ports, and suggested environment variables. Example: 'How do I connect to the postgres service?'",
	}, s.handleGetConnectionInfo)

	// list_k8s_namespaces - List available namespaces
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_k8s_namespaces",
		Description: "List all available Kubernetes namespaces in the cluster. Shows which namespaces are currently being forwarded.",
	}, s.handleListK8sNamespaces)

	// list_k8s_services - List available services in a namespace
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_k8s_services",
		Description: "List all available Kubernetes services in a namespace. Shows service types, ports, and whether each is currently being forwarded.",
	}, s.handleListK8sServices)

	// find_services - Search forwarded services
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "find_services",
		Description: "Search for forwarded services by name, port, or namespace. Example: 'Find all services on port 5432' or 'Find database services'.",
	}, s.handleFindServices)

	// list_hostnames - List all DNS entries
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_hostnames",
		Description: "List all hostnames that have been added to /etc/hosts for forwarded services. Useful for debugging DNS resolution.",
	}, s.handleListHostnames)

	// === Existing utility tools ===

	// list_services - List all forwarded services
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_services",
		Description: "List all Kubernetes services currently being forwarded by kubefwd. Returns service names, namespaces, contexts, status (active/error/partial), and summary statistics.",
	}, s.handleListServices)

	// get_service - Get detailed service information
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_service",
		Description: "Get detailed information about a specific forwarded service, including all port forwards, pod information, hostnames, and traffic metrics.",
	}, s.handleGetService)

	// reconnect_service - Trigger reconnection
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "reconnect_service",
		Description: "Trigger a reconnection attempt for a specific service. Use this when a service is in error state or needs to refresh its connection to a pod.",
	}, s.handleReconnectService)

	// reconnect_all_errors - Reconnect all errored services
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "reconnect_all_errors",
		Description: "Trigger reconnection for all services currently in error state. Returns the number of services triggered.",
	}, s.handleReconnectAllErrors)

	// sync_service - Force pod sync
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "sync_service",
		Description: "Force kubefwd to re-sync pods for a service. Useful after pod changes or when the service seems stale.",
	}, s.handleSyncService)

	// get_health - Get health status
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_health",
		Description: "Get the overall health status of kubefwd, including version information, runtime details, and summary of forwarding state.",
	}, s.handleGetHealth)

	// get_metrics - Get bandwidth metrics
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_metrics",
		Description: "Get bandwidth metrics and traffic statistics for kubefwd. Can return overall summary or per-service breakdown.",
	}, s.handleGetMetrics)

	// diagnose_errors - AI-optimized error diagnosis
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "diagnose_errors",
		Description: "Get a comprehensive diagnosis of all current errors in kubefwd, with context and suggested remediation steps.",
	}, s.handleDiagnoseErrors)

	// get_logs - Get recent logs (kept but lower priority)
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_logs",
		Description: "Get recent kubefwd log entries. Supports filtering by level and keyword search.",
	}, s.handleGetLogs)
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
		return nil, nil, fmt.Errorf("state reader not available")
	}

	svc := state.GetService(input.Key)
	if svc == nil {
		return nil, nil, fmt.Errorf("service not found: %s", input.Key)
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
		return nil, nil, fmt.Errorf("service controller not available")
	}

	if err := controller.Reconnect(input.Key); err != nil {
		return nil, nil, fmt.Errorf("reconnect failed: %w", err)
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
		return nil, nil, fmt.Errorf("service controller not available")
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
		return nil, nil, fmt.Errorf("service controller not available")
	}

	if err := controller.Sync(input.Key, input.Force); err != nil {
		return nil, nil, fmt.Errorf("sync failed: %w", err)
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
		return nil, nil, fmt.Errorf("namespace controller not available - make sure kubefwd is running with --api flag")
	}

	context := input.Context
	if context == "" {
		context = "default"
	}

	opts := types.AddNamespaceOpts{
		LabelSelector: input.Selector,
	}

	info, err := nsController.AddNamespace(context, input.Namespace, opts)
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
		return nil, nil, fmt.Errorf("namespace controller not available - make sure kubefwd is running with --api flag")
	}

	context := input.Context
	if context == "" {
		context = "default"
	}

	if err := nsController.RemoveNamespace(context, input.Namespace); err != nil {
		return nil, nil, fmt.Errorf("failed to remove namespace: %w", err)
	}

	result := map[string]interface{}{
		"success":   true,
		"namespace": input.Namespace,
		"context":   context,
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
		return nil, nil, fmt.Errorf("service CRUD controller not available - make sure kubefwd is running with --api flag")
	}

	context := input.Context
	if context == "" {
		context = "default"
	}

	addReq := types.AddServiceRequest{
		Namespace:   input.Namespace,
		ServiceName: input.ServiceName,
		Context:     context,
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
		return nil, nil, fmt.Errorf("service CRUD controller not available - make sure kubefwd is running with --api flag")
	}

	if err := svcCRUD.RemoveService(input.Key); err != nil {
		return nil, nil, fmt.Errorf("failed to remove service: %w", err)
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

	// Build key from input
	key := input.ServiceName
	if input.Namespace != "" {
		key = input.ServiceName + "." + input.Namespace
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
		return nil, nil, fmt.Errorf("kubernetes discovery not available - make sure kubefwd is running with --api flag")
	}

	context := input.Context
	if context == "" {
		context = "default"
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
		return nil, nil, fmt.Errorf("kubernetes discovery not available - make sure kubefwd is running with --api flag")
	}

	context := input.Context
	if context == "" {
		context = "default"
	}

	services, err := k8s.ListServices(context, input.Namespace)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list services: %w", err)
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
