package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/txn2/kubefwd/pkg/fwdapi/types"
	"github.com/txn2/kubefwd/pkg/fwdtui/state"
)

// AnalyzeHandler handles AI-optimized analysis endpoints
type AnalyzeHandler struct {
	stateReader    types.StateReader
	diagnostics    types.DiagnosticsProvider
	getManagerInfo func() types.ManagerInfo
}

// NewAnalyzeHandler creates a new analyze handler
func NewAnalyzeHandler(stateReader types.StateReader, diagnostics types.DiagnosticsProvider, getManagerInfo func() types.ManagerInfo) *AnalyzeHandler {
	return &AnalyzeHandler{
		stateReader:    stateReader,
		diagnostics:    diagnostics,
		getManagerInfo: getManagerInfo,
	}
}

// StatusResponse is a quick AI-friendly status
type StatusResponse struct {
	Status     string `json:"status"`     // "ok", "issues", "error"
	Message    string `json:"message"`    // Human-readable summary
	ErrorCount int    `json:"errorCount"` // Number of current errors
	Uptime     string `json:"uptime,omitempty"`
}

// Status returns a quick status for AI consumption
// GET /v1/status
func (h *AnalyzeHandler) Status(c *gin.Context) {
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

	summary := h.stateReader.GetSummary()

	status := "ok"
	var message string

	switch {
	case summary.ErrorCount == 0 && summary.ActiveServices > 0:
		message = fmt.Sprintf("All %d services healthy, %d active forwards",
			summary.ActiveServices, summary.ActiveForwards)
	case summary.ErrorCount > 0 && summary.ActiveServices > summary.ErrorCount:
		status = "issues"
		message = fmt.Sprintf("%d of %d services have issues, %d errors",
			summary.ErrorCount, summary.TotalServices, summary.ErrorCount)
	case summary.ErrorCount > 0:
		status = "error"
		message = fmt.Sprintf("%d services with errors, only %d active",
			summary.ErrorCount, summary.ActiveServices)
	default:
		message = "No services currently forwarded"
	}

	uptime := ""
	if h.getManagerInfo != nil {
		if mgr := h.getManagerInfo(); mgr != nil {
			uptime = mgr.Uptime().Round(time.Second).String()
		}
	}

	response := StatusResponse{
		Status:     status,
		Message:    message,
		ErrorCount: summary.ErrorCount,
		Uptime:     uptime,
	}

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data:    response,
		Meta: &types.MetaInfo{
			Timestamp: time.Now(),
		},
	})
}

// AnalysisResponse provides full analysis for AI
type AnalysisResponse struct {
	Status           string             `json:"status"`
	Summary          string             `json:"summary"`
	Issues           []Issue            `json:"issues,omitempty"`
	Recommendations  []Recommendation   `json:"recommendations,omitempty"`
	SuggestedActions []ActionSuggestion `json:"suggestedActions,omitempty"`
	Stats            AnalysisStats      `json:"stats"`
}

// Issue represents a detected problem
type Issue struct {
	Severity   string `json:"severity"`  // "critical", "high", "medium", "low"
	Component  string `json:"component"` // "service", "forward", "network"
	ServiceKey string `json:"serviceKey,omitempty"`
	PodName    string `json:"podName,omitempty"`
	Message    string `json:"message"`
	ErrorType  string `json:"errorType,omitempty"`
}

// Recommendation provides actionable advice
type Recommendation struct {
	Priority string `json:"priority"` // "high", "medium", "low"
	Category string `json:"category"` // "performance", "reliability", "configuration"
	Message  string `json:"message"`
}

// ActionSuggestion provides API actions to fix issues
type ActionSuggestion struct {
	Action   string `json:"action"` // "reconnect", "sync", "reconnect_all"
	Target   string `json:"target"` // service key or "all"
	Reason   string `json:"reason"`
	Endpoint string `json:"endpoint"` // POST /v1/services/:key/reconnect
	Method   string `json:"method"`   // POST
}

// AnalysisStats provides statistics
type AnalysisStats struct {
	TotalServices   int    `json:"totalServices"`
	ActiveServices  int    `json:"activeServices"`
	ErroredServices int    `json:"erroredServices"`
	TotalForwards   int    `json:"totalForwards"`
	ActiveForwards  int    `json:"activeForwards"`
	TotalBytesIn    uint64 `json:"totalBytesIn"`
	TotalBytesOut   uint64 `json:"totalBytesOut"`
	Uptime          string `json:"uptime,omitempty"`
}

// errorClassification holds error type and severity
type errorClassification struct {
	errorType string
	severity  string
}

// classifyError classifies an error message into type and severity
func classifyError(errMsg string) errorClassification {
	errLower := strings.ToLower(errMsg)
	switch {
	case strings.Contains(errLower, "connection refused"):
		return errorClassification{"connection_refused", "high"}
	case strings.Contains(errLower, "timeout"):
		return errorClassification{"timeout", "high"}
	case strings.Contains(errLower, "not found"):
		return errorClassification{"pod_not_found", "critical"}
	case strings.Contains(errLower, "broken pipe"):
		return errorClassification{"broken_pipe", "high"}
	default:
		return errorClassification{"unknown", "high"}
	}
}

// buildIssues collects issues from services
func (h *AnalyzeHandler) buildIssues(services []state.ServiceSnapshot) ([]Issue, []string, map[string]int) {
	var issues []Issue
	var erroredServiceKeys []string
	errorTypes := make(map[string]int)

	for _, svc := range services {
		if svc.ErrorCount == 0 {
			continue
		}
		erroredServiceKeys = append(erroredServiceKeys, svc.Key)
		for _, fwd := range svc.PortForwards {
			if fwd.Error == "" {
				continue
			}
			class := classifyError(fwd.Error)
			errorTypes[class.errorType]++
			issues = append(issues, Issue{
				Severity:   class.severity,
				Component:  "forward",
				ServiceKey: svc.Key,
				PodName:    fwd.PodName,
				Message:    fwd.Error,
				ErrorType:  class.errorType,
			})
		}
	}
	return issues, erroredServiceKeys, errorTypes
}

// buildRecommendations generates recommendations based on error analysis
func buildRecommendations(erroredCount int, errorTypes map[string]int, noTraffic bool) []Recommendation {
	var recs []Recommendation

	if erroredCount > 3 {
		recs = append(recs, Recommendation{
			Priority: "high",
			Category: "reliability",
			Message:  fmt.Sprintf("Multiple services (%d) have errors. Consider using reconnect_all to attempt bulk recovery.", erroredCount),
		})
	}
	if errorTypes["pod_not_found"] > 0 {
		recs = append(recs, Recommendation{
			Priority: "high",
			Category: "reliability",
			Message:  "Some pods are missing. Use sync to rediscover pods or check if deployments are healthy.",
		})
	}
	if errorTypes["connection_refused"] > 0 {
		recs = append(recs, Recommendation{
			Priority: "medium",
			Category: "reliability",
			Message:  "Connection refused errors indicate pods may not be ready. Check readiness probes and pod logs.",
		})
	}
	if errorTypes["timeout"] > 0 {
		recs = append(recs, Recommendation{
			Priority: "medium",
			Category: "configuration",
			Message:  "Timeout errors may indicate network policies blocking traffic. Review network policies.",
		})
	}
	if noTraffic {
		recs = append(recs, Recommendation{
			Priority: "low",
			Category: "performance",
			Message:  "No traffic detected. Verify applications are sending requests through forwarded services.",
		})
	}
	return recs
}

// buildActions generates suggested actions based on issues
func buildActions(erroredServiceKeys []string, issues []Issue) []ActionSuggestion {
	var actions []ActionSuggestion

	if len(erroredServiceKeys) > 5 {
		actions = append(actions, ActionSuggestion{
			Action:   "reconnect_all",
			Target:   "all",
			Reason:   fmt.Sprintf("Bulk reconnect %d errored services", len(erroredServiceKeys)),
			Endpoint: "/v1/services/reconnect",
			Method:   "POST",
		})
	} else {
		for _, key := range erroredServiceKeys {
			actions = append(actions, ActionSuggestion{
				Action:   "reconnect",
				Target:   key,
				Reason:   "Service has errors, attempt reconnection",
				Endpoint: fmt.Sprintf("/v1/services/%s/reconnect", key),
				Method:   "POST",
			})
		}
	}

	for _, issue := range issues {
		if issue.ErrorType == "pod_not_found" {
			actions = append(actions, ActionSuggestion{
				Action:   "sync",
				Target:   issue.ServiceKey,
				Reason:   "Pod not found, sync to rediscover pods",
				Endpoint: fmt.Sprintf("/v1/services/%s/sync", issue.ServiceKey),
				Method:   "POST",
			})
			break
		}
	}

	if len(actions) > 10 {
		return actions[:10]
	}
	return actions
}

// determineStatus returns the overall health status
func determineStatus(summary state.SummaryStats) string {
	if summary.ErrorCount == 0 {
		return "healthy"
	}
	if summary.ErrorCount > summary.ActiveServices {
		return "unhealthy"
	}
	return "degraded"
}

// buildSummaryMessage creates a human-readable summary
func buildSummaryMessage(summary state.SummaryStats) string {
	var parts []string
	if summary.ActiveServices > 0 {
		parts = append(parts, fmt.Sprintf("%d active services", summary.ActiveServices))
	}
	if summary.ErrorCount > 0 {
		parts = append(parts, fmt.Sprintf("%d errors", summary.ErrorCount))
	}
	if len(parts) == 0 {
		return "No services currently forwarded"
	}
	return strings.Join(parts, ", ")
}

// getUptime retrieves the uptime string from manager info
func (h *AnalyzeHandler) getUptime() string {
	if h.getManagerInfo == nil {
		return ""
	}
	if mgr := h.getManagerInfo(); mgr != nil {
		return mgr.Uptime().Round(time.Second).String()
	}
	return ""
}

// Analyze returns full analysis for AI consumption
// GET /v1/analyze
func (h *AnalyzeHandler) Analyze(c *gin.Context) {
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

	summary := h.stateReader.GetSummary()
	services := h.stateReader.GetServices()

	issues, erroredServiceKeys, errorTypes := h.buildIssues(services)
	noTraffic := summary.TotalBytesIn == 0 && summary.TotalBytesOut == 0 && summary.ActiveForwards > 0

	response := AnalysisResponse{
		Status:           determineStatus(summary),
		Summary:          buildSummaryMessage(summary),
		Issues:           issues,
		Recommendations:  buildRecommendations(len(erroredServiceKeys), errorTypes, noTraffic),
		SuggestedActions: buildActions(erroredServiceKeys, issues),
		Stats: AnalysisStats{
			TotalServices:   summary.TotalServices,
			ActiveServices:  summary.ActiveServices,
			ErroredServices: len(erroredServiceKeys),
			TotalForwards:   summary.TotalForwards,
			ActiveForwards:  summary.ActiveForwards,
			TotalBytesIn:    summary.TotalBytesIn,
			TotalBytesOut:   summary.TotalBytesOut,
			Uptime:          h.getUptime(),
		},
	}

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data:    response,
		Meta: &types.MetaInfo{
			Timestamp: time.Now(),
		},
	})
}
