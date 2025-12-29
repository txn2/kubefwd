package handlers

import (
	"net/http"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/txn2/kubefwd/pkg/fwdapi/types"
)

// HealthHandler handles health and info endpoints
type HealthHandler struct {
	version    string
	startTime  time.Time
	getManager func() types.ManagerInfo
}

// NewHealthHandler creates a new health handler
func NewHealthHandler(version string, startTime time.Time, getManager func() types.ManagerInfo) *HealthHandler {
	return &HealthHandler{
		version:    version,
		startTime:  startTime,
		getManager: getManager,
	}
}

// Root provides API welcome/discovery
func (h *HealthHandler) Root(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"name":    "kubefwd API",
		"version": h.version,
		"docs":    "https://github.com/txn2/kubefwd",
		"endpoints": gin.H{
			"health":   "/health",
			"info":     "/info",
			"services": "/v1/services",
			"forwards": "/v1/forwards",
			"metrics":  "/v1/metrics",
			"logs":     "/v1/logs",
			"events":   "/v1/events (SSE)",
		},
	})
}

// Health returns health status
func (h *HealthHandler) Health(c *gin.Context) {
	uptime := time.Since(h.startTime)

	c.JSON(http.StatusOK, types.HealthResponse{
		Status:    "healthy",
		Version:   h.version,
		Uptime:    uptime.Round(time.Second).String(),
		Timestamp: time.Now(),
	})
}

// Info returns detailed runtime information
func (h *HealthHandler) Info(c *gin.Context) {
	uptime := time.Since(h.startTime)

	response := types.InfoResponse{
		Version:    h.version,
		GoVersion:  runtime.Version(),
		Platform:   runtime.GOOS + "/" + runtime.GOARCH,
		StartTime:  h.startTime,
		Uptime:     uptime.Round(time.Second).String(),
		APIEnabled: true,
	}

	// Get additional info from manager if available
	if h.getManager != nil {
		if m := h.getManager(); m != nil {
			response.Namespaces = m.Namespaces()
			response.Contexts = m.Contexts()
			response.TUIEnabled = m.TUIEnabled()
		}
	}

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data:    response,
		Meta: &types.MetaInfo{
			Timestamp: time.Now(),
		},
	})
}
