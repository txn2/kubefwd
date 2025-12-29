package events

import (
	"time"

	"github.com/sirupsen/logrus"
)

// EventType represents the type of TUI event
type EventType int

const (
	ServiceAdded EventType = iota
	ServiceRemoved
	ServiceUpdated

	PodAdded
	PodRemoved
	PodStatusChanged

	BandwidthUpdate

	LogMessage

	ShutdownStarted
	ShutdownComplete

	NamespaceRemoved
)

// String returns a string representation of the event type
func (e EventType) String() string {
	switch e {
	case ServiceAdded:
		return "ServiceAdded"
	case ServiceRemoved:
		return "ServiceRemoved"
	case ServiceUpdated:
		return "ServiceUpdated"
	case PodAdded:
		return "PodAdded"
	case PodRemoved:
		return "PodRemoved"
	case PodStatusChanged:
		return "PodStatusChanged"
	case BandwidthUpdate:
		return "BandwidthUpdate"
	case LogMessage:
		return "LogMessage"
	case ShutdownStarted:
		return "ShutdownStarted"
	case ShutdownComplete:
		return "ShutdownComplete"
	case NamespaceRemoved:
		return "NamespaceRemoved"
	default:
		return "Unknown"
	}
}

// Event represents a TUI event with all relevant data
type Event struct {
	Type      EventType
	Timestamp time.Time

	// Service identification
	ServiceKey  string // "service.namespace.context" (display name for TUI)
	RegistryKey string // "realservice.namespace.context" (for registry lookup)
	Service     string
	Namespace   string
	Context     string

	// Pod identification
	PodKey        string // "podname"
	PodName       string
	ContainerName string // container that owns the forwarded port

	// Network info
	LocalIP   string
	LocalPort string
	PodPort   string
	Hostnames []string

	// Status
	Status string
	Error  error

	// Log info
	LogLevel   logrus.Level
	LogMessage string
	LogFields  map[string]interface{}

	// Bandwidth data
	BytesIn  uint64
	BytesOut uint64
	RateIn   float64
	RateOut  float64
}

// NewServiceEvent creates a new service-related event
func NewServiceEvent(eventType EventType, service, namespace, context string) Event {
	return Event{
		Type:       eventType,
		Timestamp:  time.Now(),
		ServiceKey: service + "." + namespace + "." + context,
		Service:    service,
		Namespace:  namespace,
		Context:    context,
	}
}

// NewPodEvent creates a new pod-related event
// registryKey is the actual service key used in the registry (service.namespace.context)
// which may differ from the ServiceKey for headless services where service includes pod name
func NewPodEvent(eventType EventType, service, namespace, context, podName, registryKey string) Event {
	return Event{
		Type:        eventType,
		Timestamp:   time.Now(),
		ServiceKey:  service + "." + namespace + "." + context,
		RegistryKey: registryKey,
		Service:     service,
		Namespace:   namespace,
		Context:     context,
		PodKey:      podName,
		PodName:     podName,
	}
}

// NewNamespaceRemovedEvent creates a new namespace removal event
// This event signals that all services in the given namespace/context should be removed
func NewNamespaceRemovedEvent(namespace, context string) Event {
	return Event{
		Type:      NamespaceRemoved,
		Timestamp: time.Now(),
		Namespace: namespace,
		Context:   context,
	}
}
