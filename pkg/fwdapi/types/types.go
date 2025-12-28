package types

import "time"

// Response is the standard API response wrapper
type Response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *ErrorInfo  `json:"error,omitempty"`
	Meta    *MetaInfo   `json:"meta,omitempty"`
}

// ErrorInfo provides error details
type ErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// MetaInfo provides response metadata
type MetaInfo struct {
	Count     int       `json:"count,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// === Service Types ===

// ServiceResponse represents a service in API responses
type ServiceResponse struct {
	Key           string            `json:"key"`
	ServiceName   string            `json:"serviceName"`
	Namespace     string            `json:"namespace"`
	Context       string            `json:"context"`
	Headless      bool              `json:"headless"`
	Status        string            `json:"status"` // "active", "error", "partial", "pending"
	ActiveCount   int               `json:"activeCount"`
	ErrorCount    int               `json:"errorCount"`
	TotalBytesIn  uint64            `json:"totalBytesIn"`
	TotalBytesOut uint64            `json:"totalBytesOut"`
	RateIn        float64           `json:"rateIn"`
	RateOut       float64           `json:"rateOut"`
	Forwards      []ForwardResponse `json:"forwards,omitempty"`
}

// ServiceListResponse contains a list of services with summary
type ServiceListResponse struct {
	Services []ServiceResponse `json:"services"`
	Summary  SummaryResponse   `json:"summary"`
}

// === Forward Types ===

// ForwardResponse represents a port forward in API responses
type ForwardResponse struct {
	Key           string    `json:"key"`
	ServiceKey    string    `json:"serviceKey"`
	ServiceName   string    `json:"serviceName"`
	Namespace     string    `json:"namespace"`
	Context       string    `json:"context"`
	Headless      bool      `json:"headless"`
	PodName       string    `json:"podName"`
	ContainerName string    `json:"containerName,omitempty"`
	LocalIP       string    `json:"localIP"`
	LocalPort     string    `json:"localPort"`
	PodPort       string    `json:"podPort"`
	Hostnames     []string  `json:"hostnames"`
	Status        string    `json:"status"`
	Error         string    `json:"error,omitempty"`
	StartedAt     time.Time `json:"startedAt"`
	LastActive    time.Time `json:"lastActive"`
	BytesIn       uint64    `json:"bytesIn"`
	BytesOut      uint64    `json:"bytesOut"`
	RateIn        float64   `json:"rateIn"`
	RateOut       float64   `json:"rateOut"`
	AvgRateIn     float64   `json:"avgRateIn"`
	AvgRateOut    float64   `json:"avgRateOut"`
}

// ForwardListResponse contains a list of forwards
type ForwardListResponse struct {
	Forwards []ForwardResponse `json:"forwards"`
	Summary  SummaryResponse   `json:"summary"`
}

// === Metrics Types ===

// MetricsSummaryResponse provides overall metrics
type MetricsSummaryResponse struct {
	TotalServices  int       `json:"totalServices"`
	ActiveServices int       `json:"activeServices"`
	TotalForwards  int       `json:"totalForwards"`
	ActiveForwards int       `json:"activeForwards"`
	ErrorCount     int       `json:"errorCount"`
	TotalBytesIn   uint64    `json:"totalBytesIn"`
	TotalBytesOut  uint64    `json:"totalBytesOut"`
	TotalRateIn    float64   `json:"totalRateIn"`
	TotalRateOut   float64   `json:"totalRateOut"`
	Uptime         string    `json:"uptime"`
	LastUpdated    time.Time `json:"lastUpdated"`
}

// ServiceMetricsResponse provides metrics for a single service
type ServiceMetricsResponse struct {
	Key           string               `json:"key"`
	ServiceName   string               `json:"serviceName"`
	Namespace     string               `json:"namespace"`
	Context       string               `json:"context"`
	TotalBytesIn  uint64               `json:"totalBytesIn"`
	TotalBytesOut uint64               `json:"totalBytesOut"`
	RateIn        float64              `json:"rateIn"`
	RateOut       float64              `json:"rateOut"`
	PortForwards  []PortForwardMetrics `json:"portForwards"`
	History       []RateSampleResponse `json:"history,omitempty"`
}

// PortForwardMetrics provides metrics for a single port forward
type PortForwardMetrics struct {
	PodName        string            `json:"podName"`
	LocalIP        string            `json:"localIP"`
	LocalPort      string            `json:"localPort"`
	PodPort        string            `json:"podPort"`
	BytesIn        uint64            `json:"bytesIn"`
	BytesOut       uint64            `json:"bytesOut"`
	RateIn         float64           `json:"rateIn"`
	RateOut        float64           `json:"rateOut"`
	AvgRateIn      float64           `json:"avgRateIn"`
	AvgRateOut     float64           `json:"avgRateOut"`
	ConnectedAt    time.Time         `json:"connectedAt"`
	LastActivityAt time.Time         `json:"lastActivityAt"`
	HTTPLogs       []HTTPLogResponse `json:"httpLogs,omitempty"`
}

// RateSampleResponse represents a point-in-time rate measurement
type RateSampleResponse struct {
	Timestamp time.Time `json:"timestamp"`
	BytesIn   uint64    `json:"bytesIn"`
	BytesOut  uint64    `json:"bytesOut"`
}

// HTTPLogResponse represents an HTTP request/response log entry
type HTTPLogResponse struct {
	Timestamp  time.Time `json:"timestamp"`
	Method     string    `json:"method"`
	Path       string    `json:"path"`
	StatusCode int       `json:"statusCode"`
	Duration   string    `json:"duration"`
	Size       int64     `json:"size"`
}

// === Log Types ===

// LogEntryResponse represents a log entry in API responses
type LogEntryResponse struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
}

// LogsResponse contains a list of log entries
type LogsResponse struct {
	Logs []LogEntryResponse `json:"logs"`
}

// === Query Parameters ===

// ListParams provides common query parameters for list endpoints
type ListParams struct {
	Limit     int    `form:"limit"`     // max items to return (default: 100)
	Offset    int    `form:"offset"`    // pagination offset
	Status    string `form:"status"`    // filter by status: active|error|partial|pending
	Namespace string `form:"namespace"` // filter by namespace
	Context   string `form:"context"`   // filter by context
	Search    string `form:"search"`    // text search in service/pod names
}

// PagedResponse wraps list responses with pagination info
type PagedResponse struct {
	Items      interface{} `json:"items"`
	Pagination Pagination  `json:"pagination"`
}

// Pagination provides pagination metadata
type Pagination struct {
	Total   int  `json:"total"`
	Limit   int  `json:"limit"`
	Offset  int  `json:"offset"`
	HasMore bool `json:"hasMore"`
}

// === Summary Types ===

// SummaryResponse provides overall summary statistics
type SummaryResponse struct {
	TotalServices  int     `json:"totalServices"`
	ActiveServices int     `json:"activeServices"`
	TotalForwards  int     `json:"totalForwards"`
	ActiveForwards int     `json:"activeForwards"`
	ErrorCount     int     `json:"errorCount"`
	TotalBytesIn   uint64  `json:"totalBytesIn"`
	TotalBytesOut  uint64  `json:"totalBytesOut"`
	RateIn         float64 `json:"rateIn"`
	RateOut        float64 `json:"rateOut"`
}

// === Health Types ===

// HealthResponse provides health status
type HealthResponse struct {
	Status    string    `json:"status"` // "healthy", "degraded", "unhealthy"
	Version   string    `json:"version"`
	Uptime    string    `json:"uptime"`
	Timestamp time.Time `json:"timestamp"`
}

// InfoResponse provides detailed runtime information
type InfoResponse struct {
	Version    string    `json:"version"`
	GoVersion  string    `json:"goVersion"`
	Platform   string    `json:"platform"`
	StartTime  time.Time `json:"startTime"`
	Uptime     string    `json:"uptime"`
	Namespaces []string  `json:"namespaces,omitempty"`
	Contexts   []string  `json:"contexts,omitempty"`
	TUIEnabled bool      `json:"tuiEnabled"`
	APIEnabled bool      `json:"apiEnabled"`
}

// === Event Types (for SSE) ===

// EventResponse represents an event for SSE streaming
type EventResponse struct {
	Type      string                 `json:"type"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
}

// === Action Response Types ===

// ReconnectResponse provides reconnection action results
type ReconnectResponse struct {
	Triggered int      `json:"triggered"`
	Services  []string `json:"services,omitempty"`
}

// SyncResponse provides sync action results
type SyncResponse struct {
	Service string `json:"service"`
	Force   bool   `json:"force"`
}

// === HTTP Traffic Types ===

// HTTPTrafficResponse provides HTTP traffic logs for a forward
type HTTPTrafficResponse struct {
	ForwardKey string              `json:"forwardKey"`
	PodName    string              `json:"podName"`
	LocalIP    string              `json:"localIP"`
	LocalPort  string              `json:"localPort"`
	Summary    HTTPActivitySummary `json:"summary"`
	Logs       []HTTPLogResponse   `json:"logs"`
}

// HTTPActivitySummary provides summary of HTTP activity
type HTTPActivitySummary struct {
	TotalRequests int            `json:"totalRequests"`
	LastRequest   time.Time      `json:"lastRequest,omitempty"`
	StatusCodes   map[string]int `json:"statusCodes"` // "2xx", "3xx", "4xx", "5xx"
	TopPaths      []PathCount    `json:"topPaths,omitempty"`
}

// PathCount provides request count for a path
type PathCount struct {
	Path  string `json:"path"`
	Count int    `json:"count"`
}

// ServiceHTTPTrafficResponse provides HTTP traffic for all forwards of a service
type ServiceHTTPTrafficResponse struct {
	ServiceKey string                `json:"serviceKey"`
	Summary    HTTPActivitySummary   `json:"summary"`
	Forwards   []HTTPTrafficResponse `json:"forwards"`
}
