package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/txn2/kubefwd/pkg/fwdapi/types"
	"github.com/txn2/kubefwd/pkg/fwdtui/state"
)

// ServicesHandler handles service-related endpoints
type ServicesHandler struct {
	state      types.StateReader
	controller types.ServiceController
}

// NewServicesHandler creates a new services handler
func NewServicesHandler(stateReader types.StateReader, controller types.ServiceController) *ServicesHandler {
	return &ServicesHandler{
		state:      stateReader,
		controller: controller,
	}
}

// List returns all services with their forwards
// Supports query parameters: limit, offset, status, namespace, context, search
func (h *ServicesHandler) List(c *gin.Context) {
	if h.state == nil {
		c.JSON(http.StatusServiceUnavailable, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "NOT_READY",
				Message: "State store not available",
			},
		})
		return
	}

	// Parse query parameters
	var params types.ListParams
	if err := c.ShouldBindQuery(&params); err != nil {
		// Ignore binding errors, use defaults
		params = types.ListParams{}
	}

	// Set defaults
	if params.Limit <= 0 || params.Limit > 1000 {
		params.Limit = 100
	}

	services := h.state.GetServices()
	summary := h.state.GetSummary()

	// Apply filters
	filtered := filterServices(services, params)

	// Apply pagination
	start := params.Offset
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + params.Limit
	if end > len(filtered) {
		end = len(filtered)
	}
	paged := filtered[start:end]

	response := types.ServiceListResponse{
		Services: make([]types.ServiceResponse, len(paged)),
		Summary:  mapSummary(summary),
	}

	for i, svc := range paged {
		response.Services[i] = mapServiceSnapshot(svc)
	}

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data:    response,
		Meta: &types.MetaInfo{
			Count:     len(paged),
			Timestamp: time.Now(),
		},
	})
}

// calculateServiceStatus determines the status string for a service
func calculateServiceStatus(svc *state.ServiceSnapshot) string {
	switch {
	case svc.ActiveCount > 0 && svc.ErrorCount == 0:
		return "active"
	case svc.ErrorCount > 0 && svc.ActiveCount == 0:
		return "error"
	case svc.ErrorCount > 0 && svc.ActiveCount > 0:
		return "partial"
	default:
		return "pending"
	}
}

// matchesSearch checks if a service matches the search term (case-insensitive)
func matchesSearch(svc *state.ServiceSnapshot, search string) bool {
	searchLower := strings.ToLower(search)
	return strings.Contains(strings.ToLower(svc.ServiceName), searchLower) ||
		strings.Contains(strings.ToLower(svc.Namespace), searchLower) ||
		strings.Contains(strings.ToLower(svc.Context), searchLower)
}

// serviceMatchesFilters checks if a service matches all filter criteria
func serviceMatchesFilters(svc *state.ServiceSnapshot, params types.ListParams) bool {
	if params.Status != "" && calculateServiceStatus(svc) != params.Status {
		return false
	}
	if params.Namespace != "" && svc.Namespace != params.Namespace {
		return false
	}
	if params.Context != "" && svc.Context != params.Context {
		return false
	}
	if params.Search != "" && !matchesSearch(svc, params.Search) {
		return false
	}
	return true
}

// filterServices applies query parameter filters to services
func filterServices(services []state.ServiceSnapshot, params types.ListParams) []state.ServiceSnapshot {
	if params.Status == "" && params.Namespace == "" && params.Context == "" && params.Search == "" {
		return services
	}

	result := make([]state.ServiceSnapshot, 0, len(services))
	for i := range services {
		if serviceMatchesFilters(&services[i], params) {
			result = append(result, services[i])
		}
	}
	return result
}

// Get returns a single service by key
func (h *ServicesHandler) Get(c *gin.Context) {
	if h.state == nil {
		c.JSON(http.StatusServiceUnavailable, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "NOT_READY",
				Message: "State store not available",
			},
		})
		return
	}

	key := c.Param("key")
	svc := h.state.GetService(key)

	if svc == nil {
		c.JSON(http.StatusNotFound, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "NOT_FOUND",
				Message: "Service not found: " + key,
			},
		})
		return
	}

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data:    mapServiceSnapshot(*svc),
		Meta: &types.MetaInfo{
			Timestamp: time.Now(),
		},
	})
}

// Reconnect triggers reconnection for a single service
func (h *ServicesHandler) Reconnect(c *gin.Context) {
	if h.controller == nil {
		c.JSON(http.StatusServiceUnavailable, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "NOT_READY",
				Message: "Service controller not available",
			},
		})
		return
	}

	key := c.Param("key")

	if err := h.controller.Reconnect(key); err != nil {
		c.JSON(http.StatusNotFound, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "NOT_FOUND",
				Message: err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data: types.ReconnectResponse{
			Triggered: 1,
			Services:  []string{key},
		},
		Meta: &types.MetaInfo{
			Timestamp: time.Now(),
		},
	})
}

// ReconnectAll triggers reconnection for all errored services
func (h *ServicesHandler) ReconnectAll(c *gin.Context) {
	if h.controller == nil {
		c.JSON(http.StatusServiceUnavailable, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "NOT_READY",
				Message: "Service controller not available",
			},
		})
		return
	}

	count := h.controller.ReconnectAll()

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data: types.ReconnectResponse{
			Triggered: count,
		},
		Meta: &types.MetaInfo{
			Timestamp: time.Now(),
		},
	})
}

// Sync triggers pod sync for a service
func (h *ServicesHandler) Sync(c *gin.Context) {
	if h.controller == nil {
		c.JSON(http.StatusServiceUnavailable, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "NOT_READY",
				Message: "Service controller not available",
			},
		})
		return
	}

	key := c.Param("key")
	force := c.Query("force") == "true"

	if err := h.controller.Sync(key, force); err != nil {
		c.JSON(http.StatusNotFound, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "NOT_FOUND",
				Message: err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data: types.SyncResponse{
			Service: key,
			Force:   force,
		},
		Meta: &types.MetaInfo{
			Timestamp: time.Now(),
		},
	})
}

// mapServiceSnapshot converts a state.ServiceSnapshot to an API response
func mapServiceSnapshot(svc state.ServiceSnapshot) types.ServiceResponse {
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

	forwards := make([]types.ForwardResponse, len(svc.PortForwards))
	for i, fwd := range svc.PortForwards {
		forwards[i] = mapForwardSnapshot(fwd)
	}

	return types.ServiceResponse{
		Key:           svc.Key,
		ServiceName:   svc.ServiceName,
		Namespace:     svc.Namespace,
		Context:       svc.Context,
		Headless:      svc.Headless,
		Status:        status,
		ActiveCount:   svc.ActiveCount,
		ErrorCount:    svc.ErrorCount,
		TotalBytesIn:  svc.TotalBytesIn,
		TotalBytesOut: svc.TotalBytesOut,
		RateIn:        svc.TotalRateIn,
		RateOut:       svc.TotalRateOut,
		Forwards:      forwards,
	}
}

// mapForwardSnapshot converts a state.ForwardSnapshot to an API response
func mapForwardSnapshot(fwd state.ForwardSnapshot) types.ForwardResponse {
	return types.ForwardResponse{
		Key:           fwd.Key,
		ServiceKey:    fwd.ServiceKey,
		ServiceName:   fwd.ServiceName,
		Namespace:     fwd.Namespace,
		Context:       fwd.Context,
		Headless:      fwd.Headless,
		PodName:       fwd.PodName,
		ContainerName: fwd.ContainerName,
		LocalIP:       fwd.LocalIP,
		LocalPort:     fwd.LocalPort,
		PodPort:       fwd.PodPort,
		Hostnames:     fwd.Hostnames,
		Status:        fwd.Status.String(),
		Error:         fwd.Error,
		StartedAt:     fwd.StartedAt,
		LastActive:    fwd.LastActive,
		BytesIn:       fwd.BytesIn,
		BytesOut:      fwd.BytesOut,
		RateIn:        fwd.RateIn,
		RateOut:       fwd.RateOut,
		AvgRateIn:     fwd.AvgRateIn,
		AvgRateOut:    fwd.AvgRateOut,
	}
}

// mapSummary converts state.SummaryStats to an API response
func mapSummary(summary state.SummaryStats) types.SummaryResponse {
	return types.SummaryResponse{
		TotalServices:  summary.TotalServices,
		ActiveServices: summary.ActiveServices,
		TotalForwards:  summary.TotalForwards,
		ActiveForwards: summary.ActiveForwards,
		ErrorCount:     summary.ErrorCount,
		TotalBytesIn:   summary.TotalBytesIn,
		TotalBytesOut:  summary.TotalBytesOut,
		RateIn:         summary.TotalRateIn,
		RateOut:        summary.TotalRateOut,
	}
}
