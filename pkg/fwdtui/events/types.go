package events

import (
	"time"

	"github.com/sirupsen/logrus"
)

// EventType represents the type of TUI event
type EventType int

const (
	// Service lifecycle events
	ServiceAdded EventType = iota
	ServiceRemoved
	ServiceUpdated

	// Pod lifecycle events
	PodAdded
	PodRemoved
	PodStatusChanged

	// Metrics events
	BandwidthUpdate

	// Log events
	LogMessage

	// Application lifecycle events
	ShutdownStarted
	ShutdownComplete
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
	default:
		return "Unknown"
	}
}

// Event represents a TUI event with all relevant data
type Event struct {
	Type      EventType
	Timestamp time.Time

	// Service identification
	ServiceKey string // "service.namespace.context"
	Service    string
	Namespace  string
	Context    string

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
func NewPodEvent(eventType EventType, service, namespace, context, podName string) Event {
	return Event{
		Type:       eventType,
		Timestamp:  time.Now(),
		ServiceKey: service + "." + namespace + "." + context,
		Service:    service,
		Namespace:  namespace,
		Context:    context,
		PodKey:     podName,
		PodName:    podName,
	}
}

// NewLogEvent creates a new log message event
func NewLogEvent(level logrus.Level, message string, fields map[string]interface{}) Event {
	return Event{
		Type:       LogMessage,
		Timestamp:  time.Now(),
		LogLevel:   level,
		LogMessage: message,
		LogFields:  fields,
	}
}

// NewBandwidthEvent creates a new bandwidth update event
func NewBandwidthEvent(serviceKey, podKey string, bytesIn, bytesOut uint64, rateIn, rateOut float64) Event {
	return Event{
		Type:       BandwidthUpdate,
		Timestamp:  time.Now(),
		ServiceKey: serviceKey,
		PodKey:     podKey,
		BytesIn:    bytesIn,
		BytesOut:   bytesOut,
		RateIn:     rateIn,
		RateOut:    rateOut,
	}
}
