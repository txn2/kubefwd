package handlers

import (
	"net/http"
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
func NewServicesHandler(state types.StateReader, controller types.ServiceController) *ServicesHandler {
	return &ServicesHandler{
		state:      state,
		controller: controller,
	}
}

// List returns all services with their forwards
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

	services := h.state.GetServices()
	summary := h.state.GetSummary()

	response := types.ServiceListResponse{
		Services: make([]types.ServiceResponse, len(services)),
		Summary:  mapSummary(summary),
	}

	for i, svc := range services {
		response.Services[i] = mapServiceSnapshot(svc)
	}

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data:    response,
		Meta: &types.MetaInfo{
			Count:     len(services),
			Timestamp: time.Now(),
		},
	})
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
	status := "pending"
	if svc.ActiveCount > 0 && svc.ErrorCount == 0 {
		status = "active"
	} else if svc.ErrorCount > 0 && svc.ActiveCount == 0 {
		status = "error"
	} else if svc.ErrorCount > 0 && svc.ActiveCount > 0 {
		status = "partial"
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
