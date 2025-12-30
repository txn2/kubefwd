package types

import (
	"time"

	"github.com/txn2/kubefwd/pkg/fwdmetrics"
	"github.com/txn2/kubefwd/pkg/fwdtui/events"
	"github.com/txn2/kubefwd/pkg/fwdtui/state"
)

// StateReader provides read-only access to forwarding state
type StateReader interface {
	// Service-level queries
	GetServices() []state.ServiceSnapshot
	GetService(key string) *state.ServiceSnapshot
	GetSummary() state.SummaryStats

	// Forward-level queries
	GetFiltered() []state.ForwardSnapshot
	GetForward(key string) *state.ForwardSnapshot

	// Logs
	GetLogs(count int) []state.LogEntry

	// Counts
	Count() int
	ServiceCount() int
}

// MetricsProvider provides bandwidth and connection metrics
type MetricsProvider interface {
	GetAllSnapshots() []fwdmetrics.ServiceSnapshot
	GetServiceSnapshot(key string) *fwdmetrics.ServiceSnapshot
	GetTotals() (bytesIn, bytesOut uint64, rateIn, rateOut float64)
	ServiceCount() int
	PortForwardCount() int
}

// ServiceController provides service lifecycle operations
type ServiceController interface {
	// Reconnect triggers reconnection for a service by key
	Reconnect(key string) error

	// ReconnectAll triggers reconnection for all errored services
	// Returns the number of services triggered
	ReconnectAll() int

	// Sync triggers pod sync for a service
	Sync(key string, force bool) error
}

// EventStreamer provides access to real-time events via channels
type EventStreamer interface {
	// Subscribe returns a channel that receives all events
	Subscribe() (<-chan events.Event, func())

	// SubscribeType returns a channel that receives events of a specific type
	SubscribeType(eventType events.EventType) (<-chan events.Event, func())
}

// MetricsSubscriber provides subscription to metrics updates
type MetricsSubscriber interface {
	// Subscribe returns a channel that receives metrics updates at the given interval
	Subscribe(interval time.Duration) (<-chan []fwdmetrics.ServiceSnapshot, func())
}

// ManagerInfo provides read-only access to manager configuration
type ManagerInfo interface {
	Version() string
	Uptime() time.Duration
	StartTime() time.Time
	Namespaces() []string
	Contexts() []string
	TUIEnabled() bool
}

// DiagnosticsProvider provides system and service diagnostics
type DiagnosticsProvider interface {
	// GetSummary returns system-wide diagnostic summary
	GetSummary() DiagnosticSummary

	// GetServiceDiagnostic returns diagnostics for a specific service
	GetServiceDiagnostic(key string) (*ServiceDiagnostic, error)

	// GetForwardDiagnostic returns diagnostics for a specific forward
	GetForwardDiagnostic(key string) (*ForwardDiagnostic, error)

	// GetNetworkStatus returns network interface status
	GetNetworkStatus() NetworkStatus

	// GetErrors returns recent errors across all services
	GetErrors(count int) []ErrorDetail
}

// NamespaceController manages namespace watcher lifecycle
type NamespaceController interface {
	// AddNamespace starts watching a namespace for services
	AddNamespace(ctx, namespace string, opts AddNamespaceOpts) (*NamespaceInfoResponse, error)

	// RemoveNamespace stops watching a namespace (removes all its services)
	RemoveNamespace(ctx, namespace string) error

	// ListNamespaces returns all watched namespaces
	ListNamespaces() []NamespaceInfoResponse

	// GetNamespace returns details for a specific watched namespace
	GetNamespace(ctx, namespace string) (*NamespaceInfoResponse, error)
}

// AddNamespaceOpts options for adding a namespace watcher
type AddNamespaceOpts struct {
	LabelSelector string
	FieldSelector string
}

// ServiceCRUD extends ServiceController with add/remove operations
type ServiceCRUD interface {
	ServiceController

	// AddService forwards a specific service
	AddService(req AddServiceRequest) (*AddServiceResponse, error)

	// RemoveService stops forwarding a service
	RemoveService(key string) error
}

// KubernetesDiscovery queries available Kubernetes resources
type KubernetesDiscovery interface {
	// ListNamespaces returns available namespaces in the cluster
	ListNamespaces(ctx string) ([]K8sNamespace, error)

	// ListServices returns available services in a namespace
	ListServices(ctx, namespace string) ([]K8sService, error)

	// ListContexts returns available Kubernetes contexts
	ListContexts() (*K8sContextsResponse, error)

	// GetService returns details for a specific service
	GetService(ctx, namespace, name string) (*K8sService, error)

	// GetPodLogs returns logs from a pod backing a forwarded service
	GetPodLogs(ctx, namespace, podName string, opts PodLogsOptions) (*PodLogsResponse, error)

	// ListPods returns pods in a namespace, optionally filtered by label selector
	ListPods(ctx, namespace string, opts ListPodsOptions) ([]K8sPod, error)

	// GetPod returns detailed information about a specific pod
	GetPod(ctx, namespace, podName string) (*K8sPodDetail, error)

	// GetEvents returns Kubernetes events for a resource
	GetEvents(ctx, namespace string, opts GetEventsOptions) ([]K8sEvent, error)

	// GetEndpoints returns endpoints for a service
	GetEndpoints(ctx, namespace, serviceName string) (*K8sEndpoints, error)
}

// PodLogsOptions configures pod log retrieval
type PodLogsOptions struct {
	Container  string // specific container (optional, defaults to first)
	TailLines  int    // number of lines from end (default 100)
	SinceTime  string // RFC3339 timestamp to start from (optional)
	Previous   bool   // get logs from previous container instance
	Timestamps bool   // include timestamps in output
}

// PodLogsResponse contains log output from a pod
type PodLogsResponse struct {
	PodName       string   `json:"podName"`
	Namespace     string   `json:"namespace"`
	Context       string   `json:"context"`
	ContainerName string   `json:"containerName"`
	Logs          []string `json:"logs"`
	LineCount     int      `json:"lineCount"`
	Truncated     bool     `json:"truncated"` // true if logs were truncated due to size
}

// ConnectionInfoProvider provides developer-focused connection information
type ConnectionInfoProvider interface {
	// GetConnectionInfo returns connection info for a forwarded service
	GetConnectionInfo(key string) (*ConnectionInfoResponse, error)

	// ListHostnames returns all hostnames added to /etc/hosts
	ListHostnames() (*HostnameListResponse, error)

	// FindServices searches for services matching criteria
	FindServices(query string, port int, namespace string) ([]ConnectionInfoResponse, error)
}
