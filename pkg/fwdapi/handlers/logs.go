package handlers

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/txn2/kubefwd/pkg/fwdapi/types"
)

// LogsHandler handles log-related endpoints
type LogsHandler struct {
	state        types.StateReader
	getLogBuffer func() types.LogBufferProvider
}

// NewLogsHandler creates a new logs handler
func NewLogsHandler(state types.StateReader, getLogBuffer func() types.LogBufferProvider) *LogsHandler {
	return &LogsHandler{
		state:        state,
		getLogBuffer: getLogBuffer,
	}
}

// Recent returns recent log entries
func (h *LogsHandler) Recent(c *gin.Context) {
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

	countStr := c.DefaultQuery("count", "100")
	count, err := strconv.Atoi(countStr)
	if err != nil || count < 1 {
		count = 100
	}
	if count > 1000 {
		count = 1000
	}

	logs := h.state.GetLogs(count)

	response := types.LogsResponse{
		Logs: make([]types.LogEntryResponse, len(logs)),
	}

	for i, log := range logs {
		response.Logs[i] = types.LogEntryResponse{
			Timestamp: log.Timestamp,
			Level:     log.Level,
			Message:   log.Message,
		}
	}

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data:    response,
		Meta: &types.MetaInfo{
			Count:     len(logs),
			Timestamp: time.Now(),
		},
	})
}

// Stream provides Server-Sent Events for real-time log updates
func (h *LogsHandler) Stream(c *gin.Context) {
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

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // Disable nginx buffering

	// Track last seen log count for incremental updates
	lastCount := 0

	// Send keepalive every 30 seconds, check for new logs every second
	ticker := time.NewTicker(1 * time.Second)
	keepalive := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	defer keepalive.Stop()

	c.Stream(func(w io.Writer) bool {
		select {
		case <-ticker.C:
			// Check for new logs
			logs := h.state.GetLogs(100)
			if len(logs) > lastCount {
				// Send new logs
				for i := lastCount; i < len(logs); i++ {
					log := logs[i]
					data := fmt.Sprintf(`{"timestamp":%q,"level":%q,"message":%q}`,
						log.Timestamp.Format(time.RFC3339),
						log.Level,
						log.Message)
					_, _ = fmt.Fprintf(w, "event: log\n")
					_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
				}
				lastCount = len(logs)
			}
			return true

		case <-keepalive.C:
			_, _ = fmt.Fprintf(w, ": keepalive\n\n")
			return true

		case <-c.Request.Context().Done():
			return false
		}
	})
}

// escapeJSON escapes special characters for JSON strings
func escapeJSON(s string) string {
	result := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			result = append(result, '\\', '"')
		case '\\':
			result = append(result, '\\', '\\')
		case '\n':
			result = append(result, '\\', 'n')
		case '\r':
			result = append(result, '\\', 'r')
		case '\t':
			result = append(result, '\\', 't')
		default:
			result = append(result, s[i])
		}
	}
	return string(result)
}

// System returns kubefwd system logs from the ring buffer
// GET /v1/logs/system?count=100&level=error
func (h *LogsHandler) System(c *gin.Context) {
	var buffer types.LogBufferProvider
	if h.getLogBuffer != nil {
		buffer = h.getLogBuffer()
	}
	if buffer == nil {
		c.JSON(http.StatusServiceUnavailable, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "log_buffer_unavailable",
				Message: "Log buffer not initialized",
			},
		})
		return
	}

	// Parse count parameter (default 100, max 1000)
	countStr := c.DefaultQuery("count", "100")
	count, err := strconv.Atoi(countStr)
	if err != nil || count < 1 {
		count = 100
	}
	if count > 1000 {
		count = 1000
	}

	// Get log level filter
	levelFilter := c.Query("level")

	// Get entries
	entries := buffer.GetLast(count)

	// Filter by level if specified and convert to response format
	var result []types.LogEntryResponse
	for _, entry := range entries {
		if levelFilter != "" && entry.Level != levelFilter {
			continue
		}
		result = append(result, types.LogEntryResponse{
			Timestamp: entry.Timestamp,
			Level:     entry.Level,
			Message:   entry.Message,
		})
	}

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data: gin.H{
			"logs":       result,
			"totalCount": buffer.Count(),
		},
		Meta: &types.MetaInfo{
			Count:     len(result),
			Timestamp: time.Now(),
		},
	})
}

// ClearSystem clears the system log buffer
// DELETE /v1/logs/system
func (h *LogsHandler) ClearSystem(c *gin.Context) {
	var buffer types.LogBufferProvider
	if h.getLogBuffer != nil {
		buffer = h.getLogBuffer()
	}
	if buffer == nil {
		c.JSON(http.StatusServiceUnavailable, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "log_buffer_unavailable",
				Message: "Log buffer not initialized",
			},
		})
		return
	}

	buffer.Clear()

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data: gin.H{
			"cleared": true,
			"message": "System log buffer cleared",
		},
		Meta: &types.MetaInfo{
			Timestamp: time.Now(),
		},
	})
}
