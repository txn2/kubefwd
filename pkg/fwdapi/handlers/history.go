package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/txn2/kubefwd/pkg/fwdapi/history"
	"github.com/txn2/kubefwd/pkg/fwdapi/types"
)

// HistoryHandler handles history-related endpoints
type HistoryHandler struct {
	store *history.Store
}

// NewHistoryHandler creates a new history handler
func NewHistoryHandler() *HistoryHandler {
	return &HistoryHandler{
		store: history.GetStore(),
	}
}

// Events returns historical events
// GET /v1/history/events?count=100&type=ForwardError
func (h *HistoryHandler) Events(c *gin.Context) {
	countStr := c.DefaultQuery("count", "100")
	count, err := strconv.Atoi(countStr)
	if err != nil || count < 1 {
		count = 100
	}
	if count > 1000 {
		count = 1000
	}

	eventType := history.EventType(c.Query("type"))

	events := h.store.GetEvents(count, eventType)

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data:    events,
		Meta: &types.MetaInfo{
			Count:     len(events),
			Timestamp: time.Now(),
		},
	})
}

// Errors returns historical errors
// GET /v1/history/errors?count=50
func (h *HistoryHandler) Errors(c *gin.Context) {
	countStr := c.DefaultQuery("count", "50")
	count, err := strconv.Atoi(countStr)
	if err != nil || count < 1 {
		count = 50
	}
	if count > 500 {
		count = 500
	}

	errors := h.store.GetErrors(count)

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data:    errors,
		Meta: &types.MetaInfo{
			Count:     len(errors),
			Timestamp: time.Now(),
		},
	})
}

// Reconnections returns reconnection history for a service
// GET /v1/services/:key/history/reconnections?count=20
func (h *HistoryHandler) Reconnections(c *gin.Context) {
	serviceKey := c.Param("key")

	countStr := c.DefaultQuery("count", "20")
	count, err := strconv.Atoi(countStr)
	if err != nil || count < 1 {
		count = 20
	}
	if count > 200 {
		count = 200
	}

	reconnects := h.store.GetReconnects(count, serviceKey)

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data:    reconnects,
		Meta: &types.MetaInfo{
			Count:     len(reconnects),
			Timestamp: time.Now(),
		},
	})
}

// AllReconnections returns all reconnection history
// GET /v1/history/reconnections?count=50
func (h *HistoryHandler) AllReconnections(c *gin.Context) {
	countStr := c.DefaultQuery("count", "50")
	count, err := strconv.Atoi(countStr)
	if err != nil || count < 1 {
		count = 50
	}
	if count > 200 {
		count = 200
	}

	reconnects := h.store.GetReconnects(count, "")

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data:    reconnects,
		Meta: &types.MetaInfo{
			Count:     len(reconnects),
			Timestamp: time.Now(),
		},
	})
}

// Stats returns history storage statistics
// GET /v1/history/stats
func (h *HistoryHandler) Stats(c *gin.Context) {
	stats := h.store.GetStats()

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data:    stats,
		Meta: &types.MetaInfo{
			Timestamp: time.Now(),
		},
	})
}
