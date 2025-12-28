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
}

// Resource handlers

func (s *Server) handleServicesResource(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	state := s.getState()
	if state == nil {
		return nil, fmt.Errorf("state reader not available")
	}

	services := state.GetServices()

	result := make([]map[string]interface{}, len(services))
	for i, svc := range services {
		status := "pending"
		if svc.ActiveCount > 0 && svc.ErrorCount == 0 {
			status = "active"
		} else if svc.ErrorCount > 0 && svc.ActiveCount == 0 {
			status = "error"
		} else if svc.ErrorCount > 0 && svc.ActiveCount > 0 {
			status = "partial"
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

func (s *Server) handleForwardsResource(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
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

func (s *Server) handleMetricsResource(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
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

func (s *Server) handleSummaryResource(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
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

func (s *Server) handleErrorsResource(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
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
