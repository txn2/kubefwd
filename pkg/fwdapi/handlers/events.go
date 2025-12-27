package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/txn2/kubefwd/pkg/fwdapi/types"
	"github.com/txn2/kubefwd/pkg/fwdtui/events"
)

// EventsHandler handles event streaming endpoints
type EventsHandler struct {
	streamer types.EventStreamer
}

// NewEventsHandler creates a new events handler
func NewEventsHandler(streamer types.EventStreamer) *EventsHandler {
	return &EventsHandler{
		streamer: streamer,
	}
}

// Stream provides Server-Sent Events for real-time event updates
func (h *EventsHandler) Stream(c *gin.Context) {
	if h.streamer == nil {
		c.JSON(http.StatusServiceUnavailable, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "NOT_READY",
				Message: "Event streamer not available",
			},
		})
		return
	}

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // Disable nginx buffering

	// Get optional event type filter
	eventTypeFilter := c.Query("type")

	// Subscribe to events
	var eventCh <-chan events.Event
	var cancel func()

	if eventTypeFilter != "" {
		eventType := parseEventType(eventTypeFilter)
		eventCh, cancel = h.streamer.SubscribeType(eventType)
	} else {
		eventCh, cancel = h.streamer.Subscribe()
	}
	defer cancel()

	// Send keepalive every 30 seconds
	keepalive := time.NewTicker(30 * time.Second)
	defer keepalive.Stop()

	c.Stream(func(w io.Writer) bool {
		select {
		case event, ok := <-eventCh:
			if !ok {
				return false
			}
			data := mapEventToResponse(event)
			jsonData, err := json.Marshal(data)
			if err != nil {
				return true // Skip malformed events
			}
			fmt.Fprintf(w, "event: %s\n", event.Type.String())
			fmt.Fprintf(w, "data: %s\n\n", jsonData)
			return true

		case <-keepalive.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			return true

		case <-c.Request.Context().Done():
			return false
		}
	})
}

// mapEventToResponse converts an event to an API response
func mapEventToResponse(e events.Event) types.EventResponse {
	data := map[string]interface{}{
		"serviceKey": e.ServiceKey,
		"service":    e.Service,
		"namespace":  e.Namespace,
		"context":    e.Context,
	}

	if e.RegistryKey != "" {
		data["registryKey"] = e.RegistryKey
	}
	if e.PodName != "" {
		data["podName"] = e.PodName
	}
	if e.ContainerName != "" {
		data["containerName"] = e.ContainerName
	}
	if e.LocalIP != "" {
		data["localIP"] = e.LocalIP
	}
	if e.LocalPort != "" {
		data["localPort"] = e.LocalPort
	}
	if e.PodPort != "" {
		data["podPort"] = e.PodPort
	}
	if len(e.Hostnames) > 0 {
		data["hostnames"] = e.Hostnames
	}
	if e.Status != "" {
		data["status"] = e.Status
	}
	if e.Error != nil {
		data["error"] = e.Error.Error()
	}
	if e.BytesIn > 0 || e.BytesOut > 0 {
		data["bytesIn"] = e.BytesIn
		data["bytesOut"] = e.BytesOut
	}
	if e.RateIn > 0 || e.RateOut > 0 {
		data["rateIn"] = e.RateIn
		data["rateOut"] = e.RateOut
	}

	return types.EventResponse{
		Type:      e.Type.String(),
		Timestamp: e.Timestamp,
		Data:      data,
	}
}

// parseEventType converts a string to an EventType
func parseEventType(s string) events.EventType {
	switch s {
	case "ServiceAdded":
		return events.ServiceAdded
	case "ServiceRemoved":
		return events.ServiceRemoved
	case "ServiceUpdated":
		return events.ServiceUpdated
	case "PodAdded":
		return events.PodAdded
	case "PodRemoved":
		return events.PodRemoved
	case "PodStatusChanged":
		return events.PodStatusChanged
	case "BandwidthUpdate":
		return events.BandwidthUpdate
	case "LogMessage":
		return events.LogMessage
	case "ShutdownStarted":
		return events.ShutdownStarted
	case "ShutdownComplete":
		return events.ShutdownComplete
	default:
		// Return PodStatusChanged as default for unknown types
		return events.PodStatusChanged
	}
}
