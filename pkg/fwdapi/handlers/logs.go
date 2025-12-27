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
	state types.StateReader
}

// NewLogsHandler creates a new logs handler
func NewLogsHandler(state types.StateReader) *LogsHandler {
	return &LogsHandler{
		state: state,
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
					data := fmt.Sprintf(`{"timestamp":"%s","level":"%s","message":"%s"}`,
						log.Timestamp.Format(time.RFC3339),
						log.Level,
						escapeJSON(log.Message))
					fmt.Fprintf(w, "event: log\n")
					fmt.Fprintf(w, "data: %s\n\n", data)
				}
				lastCount = len(logs)
			}
			return true

		case <-keepalive.C:
			fmt.Fprintf(w, ": keepalive\n\n")
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
