package state

import (
	"time"
)

// ForwardStatus represents the status of a port forward
type ForwardStatus int

const (
	StatusPending ForwardStatus = iota
	StatusConnecting
	StatusActive
	StatusError
	StatusStopping
)

// String returns a string representation of the status
func (s ForwardStatus) String() string {
	switch s {
	case StatusPending:
		return "Pending"
	case StatusConnecting:
		return "Connecting"
	case StatusActive:
		return "Active"
	case StatusError:
		return "Error"
	case StatusStopping:
		return "Stopping"
	default:
		return "Unknown"
	}
}

// ForwardSnapshot represents an immutable snapshot of a port forward for TUI rendering
type ForwardSnapshot struct {
	// Identification
	Key         string // unique key: "service.namespace.context.podname.localport"
	ServiceKey  string // "service.namespace.context" (display name for TUI)
	RegistryKey string // "realservice.namespace.context" (for registry lookup)

	// Service info
	ServiceName string
	Namespace   string
	Context     string
	Headless    bool

	// Pod info
	PodName       string
	ContainerName string // container that owns the forwarded port

	// Network info
	LocalIP   string
	LocalPort string
	PodPort   string
	Hostnames []string

	// Status
	Status     ForwardStatus
	Error      string
	StartedAt  time.Time
	LastActive time.Time

	// Bandwidth metrics
	BytesIn    uint64
	BytesOut   uint64
	RateIn     float64 // bytes/sec instantaneous
	RateOut    float64 // bytes/sec instantaneous
	AvgRateIn  float64 // bytes/sec 10-second average
	AvgRateOut float64 // bytes/sec 10-second average
}

// PrimaryHostname returns the shortest hostname or service name if no hostnames
func (f *ForwardSnapshot) PrimaryHostname() string {
	if len(f.Hostnames) == 0 {
		return f.ServiceName
	}
	shortest := f.Hostnames[0]
	for _, h := range f.Hostnames[1:] {
		if len(h) < len(shortest) {
			shortest = h
		}
	}
	return shortest
}

// LocalAddress returns the local IP:Port string
func (f *ForwardSnapshot) LocalAddress() string {
	return f.LocalIP + ":" + f.LocalPort
}

// ServiceSnapshot aggregates all port forwards for a service
type ServiceSnapshot struct {
	// Identification
	Key         string // "service.namespace.context"
	ServiceName string
	Namespace   string
	Context     string
	Headless    bool

	// Aggregated metrics
	TotalBytesIn  uint64
	TotalBytesOut uint64
	TotalRateIn   float64
	TotalRateOut  float64
	ActiveCount   int
	ErrorCount    int

	// Port forwards
	PortForwards []ForwardSnapshot
}

// LogEntry represents a log message for TUI display
type LogEntry struct {
	Timestamp time.Time
	Level     string
	Message   string
	Color     string // tview color tag
}

// SummaryStats provides overall statistics for the status bar
type SummaryStats struct {
	TotalServices  int
	ActiveServices int
	TotalForwards  int
	ActiveForwards int
	ErrorCount     int
	TotalBytesIn   uint64
	TotalBytesOut  uint64
	TotalRateIn    float64
	TotalRateOut   float64
	LastUpdated    time.Time
}
