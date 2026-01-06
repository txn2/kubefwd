package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/txn2/kubefwd/pkg/fwdapi/types"
	"github.com/txn2/kubefwd/pkg/fwdtui/state"
)

// ForwardsHandler handles port forward-related endpoints
type ForwardsHandler struct {
	state types.StateReader
}

// NewForwardsHandler creates a new forwards handler
func NewForwardsHandler(stateReader types.StateReader) *ForwardsHandler {
	return &ForwardsHandler{
		state: stateReader,
	}
}

// List returns all port forwards
// Supports query parameters: limit, offset, status, namespace, context, search
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

	// Parse query parameters
	var params types.ListParams
	if err := c.ShouldBindQuery(&params); err != nil {
		// Ignore binding errors, use defaults
		params = types.ListParams{}
	}

	// Set defaults
	if params.Limit <= 0 || params.Limit > 1000 {
		params.Limit = 100
	}

	forwards := h.state.GetFiltered()
	summary := h.state.GetSummary()

	// Apply filters
	filtered := filterForwards(forwards, params)

	// Apply pagination
	start := params.Offset
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + params.Limit
	if end > len(filtered) {
		end = len(filtered)
	}
	paged := filtered[start:end]

	response := types.ForwardListResponse{
		Forwards: make([]types.ForwardResponse, len(paged)),
		Summary:  mapSummary(summary),
	}

	for i, fwd := range paged {
		response.Forwards[i] = mapForwardSnapshot(fwd)
	}

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data:    response,
		Meta: &types.MetaInfo{
			Count:     len(paged),
			Timestamp: time.Now(),
		},
	})
}

// forwardMatchesSearch checks if a forward matches the search term (case-insensitive)
func forwardMatchesSearch(fwd *state.ForwardSnapshot, search string) bool {
	searchLower := strings.ToLower(search)
	return strings.Contains(strings.ToLower(fwd.ServiceName), searchLower) ||
		strings.Contains(strings.ToLower(fwd.PodName), searchLower) ||
		strings.Contains(strings.ToLower(fwd.Namespace), searchLower) ||
		strings.Contains(strings.ToLower(fwd.Context), searchLower)
}

// forwardMatchesFilters checks if a forward matches all filter criteria
func forwardMatchesFilters(fwd *state.ForwardSnapshot, params types.ListParams) bool {
	if params.Status != "" && strings.ToLower(fwd.Status.String()) != params.Status {
		return false
	}
	if params.Namespace != "" && fwd.Namespace != params.Namespace {
		return false
	}
	if params.Context != "" && fwd.Context != params.Context {
		return false
	}
	if params.Search != "" && !forwardMatchesSearch(fwd, params.Search) {
		return false
	}
	return true
}

// filterForwards applies query parameter filters to forwards
func filterForwards(forwards []state.ForwardSnapshot, params types.ListParams) []state.ForwardSnapshot {
	if params.Status == "" && params.Namespace == "" && params.Context == "" && params.Search == "" {
		return forwards
	}

	result := make([]state.ForwardSnapshot, 0, len(forwards))
	for i := range forwards {
		if forwardMatchesFilters(&forwards[i], params) {
			result = append(result, forwards[i])
		}
	}
	return result
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
