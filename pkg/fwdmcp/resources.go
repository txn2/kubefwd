package fwdmcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerResources registers all MCP resources
func (s *Server) registerResources() {
	// kubefwd://services - Current service state
	s.mcpServer.AddResource(&mcp.Resource{
		URI:         "kubefwd://services",
		Name:        "Forwarded Services",
		Description: "Complete list of all Kubernetes services currently being forwarded by kubefwd",
		MIMEType:    "application/json",
	}, s.handleServicesResource)

	// kubefwd://forwards - All port forwards
	s.mcpServer.AddResource(&mcp.Resource{
		URI:         "kubefwd://forwards",
		Name:        "Port Forwards",
		Description: "All active port forwards with connection details, IPs, ports, and hostnames",
		MIMEType:    "application/json",
	}, s.handleForwardsResource)

	// kubefwd://metrics - Current metrics
	s.mcpServer.AddResource(&mcp.Resource{
		URI:         "kubefwd://metrics",
		Name:        "Traffic Metrics",
		Description: "Bandwidth statistics including bytes in/out and transfer rates",
		MIMEType:    "application/json",
	}, s.handleMetricsResource)

	// kubefwd://summary - Summary statistics
	s.mcpServer.AddResource(&mcp.Resource{
		URI:         "kubefwd://summary",
		Name:        "Summary Statistics",
		Description: "High-level summary of kubefwd state: counts, health status, uptime",
		MIMEType:    "application/json",
	}, s.handleSummaryResource)

	// kubefwd://errors - Current errors
	s.mcpServer.AddResource(&mcp.Resource{
		URI:         "kubefwd://errors",
		Name:        "Current Errors",
		Description: "All services and forwards currently in error state with error details",
		MIMEType:    "application/json",
	}, s.handleErrorsResource)

	// === Quick access resources ===

	// kubefwd://status - Quick health status
	s.mcpServer.AddResource(&mcp.Resource{
		URI:         "kubefwd://status",
		Name:        "Quick Status",
		Description: "Quick health status: ok/issues/error with message. Use for fast health checks.",
		MIMEType:    "application/json",
	}, s.handleStatusResource)

	// kubefwd://http-traffic - Recent HTTP traffic
	s.mcpServer.AddResource(&mcp.Resource{
		URI:         "kubefwd://http-traffic",
		Name:        "HTTP Traffic",
		Description: "Recent HTTP requests through all forwards. Shows method, path, status, duration.",
		MIMEType:    "application/json",
	}, s.handleHTTPTrafficResource)

	// kubefwd://contexts - Kubernetes contexts
	s.mcpServer.AddResource(&mcp.Resource{
		URI:         "kubefwd://contexts",
		Name:        "Kubernetes Contexts",
		Description: "Available Kubernetes contexts from kubeconfig with current context highlighted.",
		MIMEType:    "application/json",
	}, s.handleContextsResource)
}

// Resource handlers

func (s *Server) handleServicesResource(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	state := s.getState()
	if state == nil {
		return nil, fmt.Errorf("state reader not available")
	}

	services := state.GetServices()

	result := make([]map[string]interface{}, len(services))
	for i, svc := range services {
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

		result[i] = map[string]interface{}{
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
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal services: %w", err)
	}

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{{
			URI:      req.Params.URI,
			MIMEType: "application/json",
			Text:     string(data),
		}},
	}, nil
}

func (s *Server) handleForwardsResource(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	state := s.getState()
	if state == nil {
		return nil, fmt.Errorf("state reader not available")
	}

	forwards := state.GetFiltered()

	result := make([]map[string]interface{}, len(forwards))
	for i, fwd := range forwards {
		result[i] = map[string]interface{}{
			"key":           fwd.Key,
			"serviceKey":    fwd.ServiceKey,
			"serviceName":   fwd.ServiceName,
			"namespace":     fwd.Namespace,
			"context":       fwd.Context,
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
		}
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal forwards: %w", err)
	}

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{{
			URI:      req.Params.URI,
			MIMEType: "application/json",
			Text:     string(data),
		}},
	}, nil
}

func (s *Server) handleMetricsResource(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	metrics := s.getMetrics()
	state := s.getState()
	manager := s.getManager()

	if metrics == nil || state == nil {
		return nil, fmt.Errorf("metrics or state not available")
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
		"timestamp":      time.Now(),
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metrics: %w", err)
	}

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{{
			URI:      req.Params.URI,
			MIMEType: "application/json",
			Text:     string(data),
		}},
	}, nil
}

func (s *Server) handleSummaryResource(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	state := s.getState()
	manager := s.getManager()

	if state == nil {
		return nil, fmt.Errorf("state reader not available")
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
		"totalForwards":  summary.TotalForwards,
		"activeForwards": summary.ActiveForwards,
		"errorCount":     summary.ErrorCount,
		"totalBytesIn":   summary.TotalBytesIn,
		"totalBytesOut":  summary.TotalBytesOut,
		"timestamp":      time.Now(),
	}

	if manager != nil {
		result["uptime"] = manager.Uptime().Round(time.Second).String()
		result["startTime"] = manager.StartTime()
		result["namespaces"] = manager.Namespaces()
		result["contexts"] = manager.Contexts()
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal summary: %w", err)
	}

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{{
			URI:      req.Params.URI,
			MIMEType: "application/json",
			Text:     string(data),
		}},
	}, nil
}

func (s *Server) handleErrorsResource(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	state := s.getState()
	if state == nil {
		return nil, fmt.Errorf("state reader not available")
	}

	services := state.GetServices()

	var errors []map[string]interface{}
	for _, svc := range services {
		if svc.ErrorCount == 0 {
			continue
		}

		for _, fwd := range svc.PortForwards {
			if fwd.Error == "" {
				continue
			}

			errors = append(errors, map[string]interface{}{
				"serviceKey":  svc.Key,
				"serviceName": svc.ServiceName,
				"namespace":   svc.Namespace,
				"context":     svc.Context,
				"podName":     fwd.PodName,
				"localIP":     fwd.LocalIP,
				"localPort":   fwd.LocalPort,
				"error":       fwd.Error,
				"status":      fwd.Status.String(),
			})
		}
	}

	result := map[string]interface{}{
		"errorCount": len(errors),
		"errors":     errors,
		"timestamp":  time.Now(),
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal errors: %w", err)
	}

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{{
			URI:      req.Params.URI,
			MIMEType: "application/json",
			Text:     string(data),
		}},
	}, nil
}

// === Quick access resource handlers ===

func (s *Server) handleStatusResource(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	analysis := s.getAnalysisProvider()
	if analysis == nil {
		// Fallback to state-based status if analysis provider not available
		state := s.getState()
		if state == nil {
			return nil, fmt.Errorf("status not available")
		}

		summary := state.GetSummary()
		status := "ok"
		message := fmt.Sprintf("All %d services healthy", summary.ActiveServices)

		if summary.ErrorCount > 0 {
			if summary.ErrorCount >= summary.ActiveServices {
				status = "error"
				message = fmt.Sprintf("%d services with errors", summary.ErrorCount)
			} else {
				status = "issues"
				message = fmt.Sprintf("%d of %d services have issues", summary.ErrorCount, summary.TotalServices)
			}
		} else if summary.TotalServices == 0 {
			message = "No services currently forwarded"
		}

		result := map[string]interface{}{
			"status":     status,
			"message":    message,
			"errorCount": summary.ErrorCount,
			"timestamp":  time.Now(),
		}

		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal status: %w", err)
		}

		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{
				URI:      req.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			}},
		}, nil
	}

	status, err := analysis.GetQuickStatus()
	if err != nil {
		return nil, fmt.Errorf("failed to get status: %w", err)
	}

	result := map[string]interface{}{
		"status":     status.Status,
		"message":    status.Message,
		"errorCount": status.ErrorCount,
		"uptime":     status.Uptime,
		"timestamp":  time.Now(),
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal status: %w", err)
	}

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{{
			URI:      req.Params.URI,
			MIMEType: "application/json",
			Text:     string(data),
		}},
	}, nil
}

func (s *Server) handleHTTPTrafficResource(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	state := s.getState()
	metrics := s.getMetrics()

	if state == nil || metrics == nil {
		return nil, fmt.Errorf("state or metrics not available")
	}

	// Collect HTTP traffic from all services
	services := state.GetServices()
	var allTraffic []map[string]interface{}

	for _, svc := range services {
		snapshot := metrics.GetServiceSnapshot(svc.Key)
		if snapshot == nil {
			continue
		}

		for _, pf := range snapshot.PortForwards {
			for _, log := range pf.HTTPLogs {
				allTraffic = append(allTraffic, map[string]interface{}{
					"timestamp":  log.Timestamp,
					"service":    svc.ServiceName,
					"namespace":  svc.Namespace,
					"pod":        pf.PodName,
					"method":     log.Method,
					"path":       log.Path,
					"statusCode": log.StatusCode,
					"duration":   log.Duration.String(),
					"size":       log.Size,
				})
			}
		}
	}

	// Limit to last 100 entries
	if len(allTraffic) > 100 {
		allTraffic = allTraffic[len(allTraffic)-100:]
	}

	result := map[string]interface{}{
		"count":     len(allTraffic),
		"requests":  allTraffic,
		"timestamp": time.Now(),
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal HTTP traffic: %w", err)
	}

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{{
			URI:      req.Params.URI,
			MIMEType: "application/json",
			Text:     string(data),
		}},
	}, nil
}

func (s *Server) handleContextsResource(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	k8s := s.getK8sDiscovery()
	if k8s == nil {
		return nil, fmt.Errorf("kubernetes discovery not available")
	}

	resp, err := k8s.ListContexts()
	if err != nil {
		return nil, fmt.Errorf("failed to list contexts: %w", err)
	}

	result := map[string]interface{}{
		"currentContext": resp.CurrentContext,
		"contexts":       resp.Contexts,
		"count":          len(resp.Contexts),
		"timestamp":      time.Now(),
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal contexts: %w", err)
	}

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{{
			URI:      req.Params.URI,
			MIMEType: "application/json",
			Text:     string(data),
		}},
	}, nil
}
