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

// === CRUD Request/Response Types ===

// AddNamespaceRequest is the request to start watching a namespace
type AddNamespaceRequest struct {
	Namespace string `json:"namespace" binding:"required"`
	Context   string `json:"context,omitempty"`  // default: current context
	Selector  string `json:"selector,omitempty"` // label selector to filter services
}

// AddNamespaceResponse is the response after starting to watch a namespace
type AddNamespaceResponse struct {
	Key       string   `json:"key"` // "namespace.context"
	Namespace string   `json:"namespace"`
	Context   string   `json:"context"`
	Services  []string `json:"services"` // services discovered
}

// NamespaceListResponse contains a list of watched namespaces
type NamespaceListResponse struct {
	Namespaces []NamespaceInfoResponse `json:"namespaces"`
}

// NamespaceInfoResponse provides information about a watched namespace
type NamespaceInfoResponse struct {
	Key           string `json:"key"`
	Namespace     string `json:"namespace"`
	Context       string `json:"context"`
	ServiceCount  int    `json:"serviceCount"`
	ActiveCount   int    `json:"activeCount"`
	ErrorCount    int    `json:"errorCount"`
	Running       bool   `json:"running"`
	LabelSelector string `json:"labelSelector,omitempty"`
	FieldSelector string `json:"fieldSelector,omitempty"`
}

// AddServiceRequest is the request to forward a specific service
type AddServiceRequest struct {
	Namespace   string   `json:"namespace" binding:"required"`
	ServiceName string   `json:"serviceName" binding:"required"`
	Context     string   `json:"context,omitempty"` // default: current context
	Ports       []string `json:"ports,omitempty"`   // specific ports to forward (optional)
	LocalIP     string   `json:"localIP,omitempty"` // reserve specific IP (optional)
}

// AddServiceResponse is the response after forwarding a service
type AddServiceResponse struct {
	Key         string        `json:"key"`
	ServiceName string        `json:"serviceName"`
	Namespace   string        `json:"namespace"`
	Context     string        `json:"context"`
	LocalIP     string        `json:"localIP"`
	Hostnames   []string      `json:"hostnames"`
	Ports       []PortMapping `json:"ports"`
}

// PortMapping represents a port mapping for a forwarded service
type PortMapping struct {
	LocalPort  string `json:"localPort"`
	RemotePort string `json:"remotePort"`
	Protocol   string `json:"protocol,omitempty"`
}

// RemoveResponse is a generic response for remove operations
type RemoveResponse struct {
	Removed bool   `json:"removed"`
	Key     string `json:"key"`
	Message string `json:"message,omitempty"`
}

// === Kubernetes Discovery Types ===

// K8sNamespacesResponse contains a list of available K8s namespaces
type K8sNamespacesResponse struct {
	Namespaces []K8sNamespace `json:"namespaces"`
}

// K8sNamespace represents a Kubernetes namespace
type K8sNamespace struct {
	Name      string `json:"name"`
	Status    string `json:"status"`    // Active, Terminating
	Forwarded bool   `json:"forwarded"` // true if currently being watched
}

// K8sServicesResponse contains a list of available K8s services
type K8sServicesResponse struct {
	Services []K8sService `json:"services"`
}

// K8sService represents a Kubernetes service available for forwarding
type K8sService struct {
	Name       string            `json:"name"`
	Namespace  string            `json:"namespace"`
	Type       string            `json:"type"` // ClusterIP, NodePort, LoadBalancer, ExternalName
	ClusterIP  string            `json:"clusterIP"`
	Ports      []K8sServicePort  `json:"ports"`
	Selector   map[string]string `json:"selector,omitempty"`
	Forwarded  bool              `json:"forwarded"`            // true if currently being forwarded
	ForwardKey string            `json:"forwardKey,omitempty"` // key in registry if forwarded
}

// K8sServicePort represents a port on a Kubernetes service
type K8sServicePort struct {
	Name       string `json:"name,omitempty"`
	Port       int32  `json:"port"`
	TargetPort string `json:"targetPort"`
	Protocol   string `json:"protocol"` // TCP, UDP
}

// K8sContextsResponse contains a list of available Kubernetes contexts
type K8sContextsResponse struct {
	Contexts       []K8sContext `json:"contexts"`
	CurrentContext string       `json:"currentContext"`
}

// K8sContext represents a Kubernetes context
type K8sContext struct {
	Name      string `json:"name"`
	Cluster   string `json:"cluster"`
	User      string `json:"user,omitempty"`
	Namespace string `json:"namespace,omitempty"` // default namespace for this context
	Active    bool   `json:"active"`              // true if this is the current context
}

// === Pod Types ===

// ListPodsOptions configures pod listing
type ListPodsOptions struct {
	LabelSelector string // e.g., "app=nginx,version=v1"
	FieldSelector string // e.g., "status.phase=Running"
	ServiceName   string // filter to pods backing this service
}

// K8sPod represents a Kubernetes pod (summary view)
type K8sPod struct {
	Name          string            `json:"name"`
	Namespace     string            `json:"namespace"`
	Phase         string            `json:"phase"`    // Pending, Running, Succeeded, Failed, Unknown
	Status        string            `json:"status"`   // Human-readable status
	Ready         string            `json:"ready"`    // e.g., "1/1", "0/2"
	Restarts      int32             `json:"restarts"` // total restart count across containers
	Age           string            `json:"age"`      // human-readable age
	IP            string            `json:"ip,omitempty"`
	Node          string            `json:"node,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	Containers    []string          `json:"containers"` // container names
	StartTime     *time.Time        `json:"startTime,omitempty"`
	ServiceName   string            `json:"serviceName,omitempty"`   // if backing a service
	IsForwarded   bool              `json:"isForwarded"`             // true if currently forwarded
	ForwardedPort string            `json:"forwardedPort,omitempty"` // local port if forwarded
}

// K8sPodDetail represents detailed pod information
type K8sPodDetail struct {
	Name        string             `json:"name"`
	Namespace   string             `json:"namespace"`
	Context     string             `json:"context"`
	Phase       string             `json:"phase"`
	Status      string             `json:"status"` // detailed status message
	Message     string             `json:"message,omitempty"`
	Reason      string             `json:"reason,omitempty"`
	IP          string             `json:"ip,omitempty"`
	HostIP      string             `json:"hostIP,omitempty"`
	Node        string             `json:"node,omitempty"`
	StartTime   *time.Time         `json:"startTime,omitempty"`
	Labels      map[string]string  `json:"labels,omitempty"`
	Annotations map[string]string  `json:"annotations,omitempty"`
	Containers  []K8sContainerInfo `json:"containers"`
	Conditions  []K8sPodCondition  `json:"conditions,omitempty"`
	Volumes     []string           `json:"volumes,omitempty"` // volume names
	QoSClass    string             `json:"qosClass,omitempty"`
	IsForwarded bool               `json:"isForwarded"`
	ForwardKey  string             `json:"forwardKey,omitempty"`
}

// K8sContainerInfo represents container information within a pod
type K8sContainerInfo struct {
	Name         string              `json:"name"`
	Image        string              `json:"image"`
	Ready        bool                `json:"ready"`
	Started      bool                `json:"started"`
	RestartCount int32               `json:"restartCount"`
	State        string              `json:"state"` // Running, Waiting, Terminated
	StateReason  string              `json:"stateReason,omitempty"`
	StateMessage string              `json:"stateMessage,omitempty"`
	LastState    string              `json:"lastState,omitempty"` // previous state if any
	Ports        []K8sContainerPort  `json:"ports,omitempty"`
	Resources    *K8sResourceRequire `json:"resources,omitempty"`
}

// K8sContainerPort represents a container port
type K8sContainerPort struct {
	Name          string `json:"name,omitempty"`
	ContainerPort int32  `json:"containerPort"`
	Protocol      string `json:"protocol"`
}

// K8sResourceRequire represents resource requests/limits
type K8sResourceRequire struct {
	CPURequest    string `json:"cpuRequest,omitempty"`
	CPULimit      string `json:"cpuLimit,omitempty"`
	MemoryRequest string `json:"memoryRequest,omitempty"`
	MemoryLimit   string `json:"memoryLimit,omitempty"`
}

// K8sPodCondition represents a pod condition
type K8sPodCondition struct {
	Type    string `json:"type"` // PodScheduled, Ready, Initialized, ContainersReady
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// === Event Types ===

// GetEventsOptions configures event retrieval
type GetEventsOptions struct {
	ResourceKind string // Pod, Service, Deployment, etc.
	ResourceName string // name of the resource
	Limit        int    // max events to return (default 50)
}

// K8sEvent represents a Kubernetes event
type K8sEvent struct {
	Type           string    `json:"type"`   // Normal, Warning
	Reason         string    `json:"reason"` // e.g., Scheduled, Pulled, Created, Started, Killing
	Message        string    `json:"message"`
	Count          int32     `json:"count"`
	FirstTimestamp time.Time `json:"firstTimestamp"`
	LastTimestamp  time.Time `json:"lastTimestamp"`
	Source         string    `json:"source"` // component that generated the event
	ObjectKind     string    `json:"objectKind,omitempty"`
	ObjectName     string    `json:"objectName,omitempty"`
}

// === Endpoint Types ===

// K8sEndpoints represents endpoints for a service
type K8sEndpoints struct {
	Name      string              `json:"name"`
	Namespace string              `json:"namespace"`
	Subsets   []K8sEndpointSubset `json:"subsets"`
}

// K8sEndpointSubset represents a subset of endpoints
type K8sEndpointSubset struct {
	Addresses         []K8sEndpointAddress `json:"addresses,omitempty"`
	NotReadyAddresses []K8sEndpointAddress `json:"notReadyAddresses,omitempty"`
	Ports             []K8sEndpointPort    `json:"ports,omitempty"`
}

// K8sEndpointAddress represents an endpoint address
type K8sEndpointAddress struct {
	IP       string `json:"ip"`
	Hostname string `json:"hostname,omitempty"`
	NodeName string `json:"nodeName,omitempty"`
	PodName  string `json:"podName,omitempty"` // target ref
}

// K8sEndpointPort represents an endpoint port
type K8sEndpointPort struct {
	Name     string `json:"name,omitempty"`
	Port     int32  `json:"port"`
	Protocol string `json:"protocol"`
}

// === Connection Info Types (for developer-focused MCP) ===

// ConnectionInfoResponse provides connection information for a forwarded service
type ConnectionInfoResponse struct {
	Service          string            `json:"service"`
	Namespace        string            `json:"namespace"`
	Context          string            `json:"context"`
	LocalIP          string            `json:"localIP"`
	Hostnames        []string          `json:"hostnames"`
	Ports            []PortInfo        `json:"ports"`
	ConnectionString string            `json:"connectionString,omitempty"` // e.g., postgresql://127.1.2.3:5432
	EnvVars          map[string]string `json:"envVars,omitempty"`          // suggested environment variables
	Status           string            `json:"status"`
}

// PortInfo provides detailed port information
type PortInfo struct {
	LocalPort  int    `json:"localPort"`
	RemotePort int    `json:"remotePort"`
	Protocol   string `json:"protocol"`
	Name       string `json:"name,omitempty"`
}

// HostnameListResponse provides a list of all hostnames added to /etc/hosts
type HostnameListResponse struct {
	Hostnames []HostnameEntry `json:"hostnames"`
	Total     int             `json:"total"`
}

// HostnameEntry represents a hostname entry in /etc/hosts
type HostnameEntry struct {
	Hostname  string `json:"hostname"`
	IP        string `json:"ip"`
	Service   string `json:"service"`
	Namespace string `json:"namespace"`
	Context   string `json:"context"`
}

// === Log Buffer Types ===

// LogBufferEntry represents a single entry in the system log buffer
type LogBufferEntry struct {
	Timestamp time.Time         `json:"timestamp"`
	Level     string            `json:"level"`
	Message   string            `json:"message"`
	Fields    map[string]string `json:"fields,omitempty"`
}

// LogBufferProvider provides access to the system log buffer
type LogBufferProvider interface {
	// GetLast returns the last n entries (most recent first)
	GetLast(n int) []LogBufferEntry
	// Count returns the total number of entries in the buffer
	Count() int
	// Clear removes all entries from the buffer
	Clear()
}
