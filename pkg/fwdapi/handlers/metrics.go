package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/txn2/kubefwd/pkg/fwdapi/types"
	"github.com/txn2/kubefwd/pkg/fwdmetrics"
)

// MetricsHandler handles metrics-related endpoints
type MetricsHandler struct {
	state      types.StateReader
	metrics    types.MetricsProvider
	getManager func() types.ManagerInfo
}

// NewMetricsHandler creates a new metrics handler
func NewMetricsHandler(state types.StateReader, metrics types.MetricsProvider, getManager func() types.ManagerInfo) *MetricsHandler {
	return &MetricsHandler{
		state:      state,
		metrics:    metrics,
		getManager: getManager,
	}
}

// Summary returns overall metrics summary
func (h *MetricsHandler) Summary(c *gin.Context) {
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

	summary := h.state.GetSummary()

	uptime := ""
	if h.getManager != nil {
		if m := h.getManager(); m != nil {
			uptime = m.Uptime().Round(time.Second).String()
		}
	}

	// Get bandwidth from metrics provider (more accurate than state store)
	var totalBytesIn, totalBytesOut uint64
	var totalRateIn, totalRateOut float64
	if h.metrics != nil {
		totalBytesIn, totalBytesOut, totalRateIn, totalRateOut = h.metrics.GetTotals()
	}

	response := types.MetricsSummaryResponse{
		TotalServices:  summary.TotalServices,
		ActiveServices: summary.ActiveServices,
		TotalForwards:  summary.TotalForwards,
		ActiveForwards: summary.ActiveForwards,
		ErrorCount:     summary.ErrorCount,
		TotalBytesIn:   totalBytesIn,
		TotalBytesOut:  totalBytesOut,
		TotalRateIn:    totalRateIn,
		TotalRateOut:   totalRateOut,
		Uptime:         uptime,
		LastUpdated:    summary.LastUpdated,
	}

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data:    response,
		Meta: &types.MetaInfo{
			Timestamp: time.Now(),
		},
	})
}

// ByService returns metrics for all services
func (h *MetricsHandler) ByService(c *gin.Context) {
	if h.metrics == nil {
		c.JSON(http.StatusServiceUnavailable, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "NOT_READY",
				Message: "Metrics provider not available",
			},
		})
		return
	}

	snapshots := h.metrics.GetAllSnapshots()

	response := make([]types.ServiceMetricsResponse, len(snapshots))
	for i, svc := range snapshots {
		response[i] = types.ServiceMetricsResponse{
			Key:           svc.ServiceName + "." + svc.Namespace + "." + svc.Context,
			ServiceName:   svc.ServiceName,
			Namespace:     svc.Namespace,
			Context:       svc.Context,
			TotalBytesIn:  svc.TotalBytesIn,
			TotalBytesOut: svc.TotalBytesOut,
			RateIn:        svc.TotalRateIn,
			RateOut:       svc.TotalRateOut,
			PortForwards:  mapPortForwardMetrics(svc.PortForwards),
		}
	}

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data:    response,
		Meta: &types.MetaInfo{
			Count:     len(snapshots),
			Timestamp: time.Now(),
		},
	})
}

// ServiceDetail returns metrics for a single service
func (h *MetricsHandler) ServiceDetail(c *gin.Context) {
	if h.metrics == nil {
		c.JSON(http.StatusServiceUnavailable, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "NOT_READY",
				Message: "Metrics provider not available",
			},
		})
		return
	}

	key := c.Param("key")
	svc := h.metrics.GetServiceSnapshot(key)

	if svc == nil {
		c.JSON(http.StatusNotFound, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "NOT_FOUND",
				Message: "Service metrics not found: " + key,
			},
		})
		return
	}

	response := types.ServiceMetricsResponse{
		Key:           svc.ServiceName + "." + svc.Namespace + "." + svc.Context,
		ServiceName:   svc.ServiceName,
		Namespace:     svc.Namespace,
		Context:       svc.Context,
		TotalBytesIn:  svc.TotalBytesIn,
		TotalBytesOut: svc.TotalBytesOut,
		RateIn:        svc.TotalRateIn,
		RateOut:       svc.TotalRateOut,
		PortForwards:  mapPortForwardMetrics(svc.PortForwards),
	}

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data:    response,
		Meta: &types.MetaInfo{
			Timestamp: time.Now(),
		},
	})
}

// ServiceHistory returns rate history for a service (for graphing)
func (h *MetricsHandler) ServiceHistory(c *gin.Context) {
	if h.metrics == nil {
		c.JSON(http.StatusServiceUnavailable, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "NOT_READY",
				Message: "Metrics provider not available",
			},
		})
		return
	}

	key := c.Param("key")
	pointsStr := c.DefaultQuery("points", "60")
	points, err := strconv.Atoi(pointsStr)
	if err != nil || points < 1 {
		points = 60
	}
	if points > 300 {
		points = 300
	}

	svc := h.metrics.GetServiceSnapshot(key)

	if svc == nil {
		c.JSON(http.StatusNotFound, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "NOT_FOUND",
				Message: "Service metrics not found: " + key,
			},
		})
		return
	}

	// Collect history from all port forwards
	history := make([]types.RateSampleResponse, 0)
	for _, pf := range svc.PortForwards {
		for _, sample := range pf.History {
			history = append(history, types.RateSampleResponse{
				Timestamp: sample.Timestamp,
				BytesIn:   sample.BytesIn,
				BytesOut:  sample.BytesOut,
			})
		}
	}

	// Limit to requested points
	if len(history) > points {
		history = history[len(history)-points:]
	}

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data: gin.H{
			"key":     key,
			"history": history,
		},
		Meta: &types.MetaInfo{
			Count:     len(history),
			Timestamp: time.Now(),
		},
	})
}

// mapPortForwardMetrics converts fwdmetrics port forward snapshots to API response
func mapPortForwardMetrics(pfs []fwdmetrics.PortForwardSnapshot) []types.PortForwardMetrics {
	result := make([]types.PortForwardMetrics, len(pfs))
	for i, pf := range pfs {
		result[i] = types.PortForwardMetrics{
			PodName:        pf.PodName,
			LocalIP:        pf.LocalIP,
			LocalPort:      pf.LocalPort,
			PodPort:        pf.PodPort,
			BytesIn:        pf.BytesIn,
			BytesOut:       pf.BytesOut,
			RateIn:         pf.RateIn,
			RateOut:        pf.RateOut,
			AvgRateIn:      pf.AvgRateIn,
			AvgRateOut:     pf.AvgRateOut,
			ConnectedAt:    pf.ConnectedAt,
			LastActivityAt: pf.LastActivityAt,
		}
	}
	return result
}
