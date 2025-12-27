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
