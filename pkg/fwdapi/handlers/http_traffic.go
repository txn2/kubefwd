package handlers

import (
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/txn2/kubefwd/pkg/fwdapi/types"
	"github.com/txn2/kubefwd/pkg/fwdmetrics"
)

// HTTPTrafficHandler handles HTTP traffic inspection endpoints
type HTTPTrafficHandler struct {
	metricsProvider types.MetricsProvider
	stateReader     types.StateReader
}

// NewHTTPTrafficHandler creates a new HTTP traffic handler
func NewHTTPTrafficHandler(metricsProvider types.MetricsProvider, stateReader types.StateReader) *HTTPTrafficHandler {
	return &HTTPTrafficHandler{
		metricsProvider: metricsProvider,
		stateReader:     stateReader,
	}
}

// ForwardHTTP returns HTTP logs for a specific forward
// GET /v1/forwards/:key/http?count=50
func (h *HTTPTrafficHandler) ForwardHTTP(c *gin.Context) {
	if h.metricsProvider == nil {
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
	countStr := c.DefaultQuery("count", "50")
	count, err := strconv.Atoi(countStr)
	if err != nil || count < 1 {
		count = 50
	}
	if count > 500 {
		count = 500
	}

	// Find the forward in state to get service key
	if h.stateReader == nil {
		c.JSON(http.StatusServiceUnavailable, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "NOT_READY",
				Message: "State reader not available",
			},
		})
		return
	}

	fwd := h.stateReader.GetForward(key)
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

	// Get metrics snapshot to find HTTP logs
	snapshot := h.metricsProvider.GetServiceSnapshot(fwd.ServiceKey)
	if snapshot == nil {
		c.JSON(http.StatusNotFound, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "NOT_FOUND",
				Message: "Metrics not found for service: " + fwd.ServiceKey,
			},
		})
		return
	}

	// Find the port forward metrics for this key
	var pfMetrics *fwdmetrics.PortForwardSnapshot
	for i := range snapshot.PortForwards {
		pf := &snapshot.PortForwards[i]
		pfKey := pf.PodName + "." + fwd.ServiceKey
		if pfKey == key || pf.PodName == fwd.PodName {
			pfMetrics = pf
			break
		}
	}

	if pfMetrics == nil {
		c.JSON(http.StatusNotFound, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "NOT_FOUND",
				Message: "Port forward metrics not found",
			},
		})
		return
	}

	// Build response
	logs := pfMetrics.HTTPLogs
	if len(logs) > count {
		logs = logs[len(logs)-count:]
	}

	response := buildHTTPTrafficResponse(key, fwd.PodName, fwd.LocalIP, fwd.LocalPort, logs)

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data:    response,
		Meta: &types.MetaInfo{
			Count:     len(response.Logs),
			Timestamp: time.Now(),
		},
	})
}

// ServiceHTTP returns HTTP logs for all forwards of a service
// GET /v1/services/:key/http?count=50
func (h *HTTPTrafficHandler) ServiceHTTP(c *gin.Context) {
	if h.metricsProvider == nil {
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
	countStr := c.DefaultQuery("count", "50")
	count, err := strconv.Atoi(countStr)
	if err != nil || count < 1 {
		count = 50
	}
	if count > 500 {
		count = 500
	}

	snapshot := h.metricsProvider.GetServiceSnapshot(key)
	if snapshot == nil {
		c.JSON(http.StatusNotFound, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "NOT_FOUND",
				Message: "Service not found: " + key,
			},
		})
		return
	}

	// Build response for all forwards
	var forwards []types.HTTPTrafficResponse
	var allLogs []fwdmetrics.HTTPLogEntry

	for _, pf := range snapshot.PortForwards {
		logs := pf.HTTPLogs
		if len(logs) > count {
			logs = logs[len(logs)-count:]
		}

		fwdKey := pf.PodName + "." + key
		fwdResponse := buildHTTPTrafficResponse(fwdKey, pf.PodName, pf.LocalIP, pf.LocalPort, logs)
		forwards = append(forwards, fwdResponse)

		allLogs = append(allLogs, pf.HTTPLogs...)
	}

	// Build aggregate summary
	summary := buildHTTPActivitySummary(allLogs)

	response := types.ServiceHTTPTrafficResponse{
		ServiceKey: key,
		Summary:    summary,
		Forwards:   forwards,
	}

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data:    response,
		Meta: &types.MetaInfo{
			Timestamp: time.Now(),
		},
	})
}

// buildHTTPTrafficResponse creates a traffic response from logs
func buildHTTPTrafficResponse(key, podName, localIP, localPort string, logs []fwdmetrics.HTTPLogEntry) types.HTTPTrafficResponse {
	summary := buildHTTPActivitySummary(logs)

	// Convert logs to response format
	responseLogs := make([]types.HTTPLogResponse, len(logs))
	for i, log := range logs {
		responseLogs[i] = types.HTTPLogResponse{
			Timestamp:  log.Timestamp,
			Method:     log.Method,
			Path:       log.Path,
			StatusCode: log.StatusCode,
			Duration:   log.Duration.String(),
			Size:       log.Size,
		}
	}

	return types.HTTPTrafficResponse{
		ForwardKey: key,
		PodName:    podName,
		LocalIP:    localIP,
		LocalPort:  localPort,
		Summary:    summary,
		Logs:       responseLogs,
	}
}

// buildHTTPActivitySummary creates a summary from logs
func buildHTTPActivitySummary(logs []fwdmetrics.HTTPLogEntry) types.HTTPActivitySummary {
	summary := types.HTTPActivitySummary{
		TotalRequests: len(logs),
		StatusCodes:   make(map[string]int),
	}

	if len(logs) == 0 {
		return summary
	}

	pathCounts := make(map[string]int)
	var lastRequest time.Time

	for _, log := range logs {
		// Track last request time
		if log.Timestamp.After(lastRequest) {
			lastRequest = log.Timestamp
		}

		// Count status codes by category
		switch {
		case log.StatusCode >= 200 && log.StatusCode < 300:
			summary.StatusCodes["2xx"]++
		case log.StatusCode >= 300 && log.StatusCode < 400:
			summary.StatusCodes["3xx"]++
		case log.StatusCode >= 400 && log.StatusCode < 500:
			summary.StatusCodes["4xx"]++
		case log.StatusCode >= 500:
			summary.StatusCodes["5xx"]++
		}

		// Count paths
		pathCounts[log.Path]++
	}

	summary.LastRequest = lastRequest

	// Get top paths (up to 10)
	type pathEntry struct {
		path  string
		count int
	}
	var paths []pathEntry
	for path, count := range pathCounts {
		paths = append(paths, pathEntry{path, count})
	}
	sort.Slice(paths, func(i, j int) bool {
		return paths[i].count > paths[j].count
	})

	maxPaths := 10
	if len(paths) < maxPaths {
		maxPaths = len(paths)
	}

	summary.TopPaths = make([]types.PathCount, maxPaths)
	for i := 0; i < maxPaths; i++ {
		summary.TopPaths[i] = types.PathCount{
			Path:  paths[i].path,
			Count: paths[i].count,
		}
	}

	return summary
}
