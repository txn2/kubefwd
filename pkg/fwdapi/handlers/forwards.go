package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/txn2/kubefwd/pkg/fwdapi/types"
)

// ForwardsHandler handles port forward-related endpoints
type ForwardsHandler struct {
	state types.StateReader
}

// NewForwardsHandler creates a new forwards handler
func NewForwardsHandler(state types.StateReader) *ForwardsHandler {
	return &ForwardsHandler{
		state: state,
	}
}

// List returns all port forwards
func (h *ForwardsHandler) List(c *gin.Context) {
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

	forwards := h.state.GetFiltered()
	summary := h.state.GetSummary()

	response := types.ForwardListResponse{
		Forwards: make([]types.ForwardResponse, len(forwards)),
		Summary:  mapSummary(summary),
	}

	for i, fwd := range forwards {
		response.Forwards[i] = mapForwardSnapshot(fwd)
	}

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data:    response,
		Meta: &types.MetaInfo{
			Count:     len(forwards),
			Timestamp: time.Now(),
		},
	})
}

// Get returns a single forward by key
func (h *ForwardsHandler) Get(c *gin.Context) {
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
	fwd := h.state.GetForward(key)

	if fwd == nil {
		c.JSON(http.StatusNotFound, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "NOT_FOUND",
				Message: "Forward not found: " + key,
			},
		})
		return
	}

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data:    mapForwardSnapshot(*fwd),
		Meta: &types.MetaInfo{
			Timestamp: time.Now(),
		},
	})
}
