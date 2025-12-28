package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/txn2/kubefwd/pkg/fwdapi/types"
)

// DiagnosticsHandler handles diagnostic-related endpoints
type DiagnosticsHandler struct {
	diagnostics types.DiagnosticsProvider
}

// NewDiagnosticsHandler creates a new diagnostics handler
func NewDiagnosticsHandler(diagnostics types.DiagnosticsProvider) *DiagnosticsHandler {
	return &DiagnosticsHandler{
		diagnostics: diagnostics,
	}
}

// Summary returns system-wide diagnostics
// GET /v1/diagnostics
func (h *DiagnosticsHandler) Summary(c *gin.Context) {
	if h.diagnostics == nil {
		c.JSON(http.StatusServiceUnavailable, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "NOT_READY",
				Message: "Diagnostics provider not available",
			},
		})
		return
	}

	summary := h.diagnostics.GetSummary()

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data:    summary,
		Meta: &types.MetaInfo{
			Timestamp: time.Now(),
		},
	})
}

// ServiceDiagnostic returns diagnostics for a specific service
// GET /v1/diagnostics/services/:key
func (h *DiagnosticsHandler) ServiceDiagnostic(c *gin.Context) {
	if h.diagnostics == nil {
		c.JSON(http.StatusServiceUnavailable, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "NOT_READY",
				Message: "Diagnostics provider not available",
			},
		})
		return
	}

	key := c.Param("key")
	diag, err := h.diagnostics.GetServiceDiagnostic(key)

	if err != nil {
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
		Data:    diag,
		Meta: &types.MetaInfo{
			Timestamp: time.Now(),
		},
	})
}

// ForwardDiagnostic returns diagnostics for a specific forward
// GET /v1/diagnostics/forwards/:key
func (h *DiagnosticsHandler) ForwardDiagnostic(c *gin.Context) {
	if h.diagnostics == nil {
		c.JSON(http.StatusServiceUnavailable, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "NOT_READY",
				Message: "Diagnostics provider not available",
			},
		})
		return
	}

	key := c.Param("key")
	diag, err := h.diagnostics.GetForwardDiagnostic(key)

	if err != nil {
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
		Data:    diag,
		Meta: &types.MetaInfo{
			Timestamp: time.Now(),
		},
	})
}

// Network returns network diagnostics
// GET /v1/diagnostics/network
func (h *DiagnosticsHandler) Network(c *gin.Context) {
	if h.diagnostics == nil {
		c.JSON(http.StatusServiceUnavailable, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "NOT_READY",
				Message: "Diagnostics provider not available",
			},
		})
		return
	}

	network := h.diagnostics.GetNetworkStatus()

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data:    network,
		Meta: &types.MetaInfo{
			Timestamp: time.Now(),
		},
	})
}

// Errors returns recent errors
// GET /v1/diagnostics/errors?count=50
func (h *DiagnosticsHandler) Errors(c *gin.Context) {
	if h.diagnostics == nil {
		c.JSON(http.StatusServiceUnavailable, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "NOT_READY",
				Message: "Diagnostics provider not available",
			},
		})
		return
	}

	countStr := c.DefaultQuery("count", "50")
	count, err := strconv.Atoi(countStr)
	if err != nil || count < 1 {
		count = 50
	}
	if count > 500 {
		count = 500
	}

	errors := h.diagnostics.GetErrors(count)

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data:    errors,
		Meta: &types.MetaInfo{
			Count:     len(errors),
			Timestamp: time.Now(),
		},
	})
}
