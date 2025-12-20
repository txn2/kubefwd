/*
Copyright 2018-2024 Craig Johnston <cjimti@gmail.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
	Key        string // unique key: "service.namespace.context.podname"
	ServiceKey string // "service.namespace.context"

	// Service info
	ServiceName string
	Namespace   string
	Context     string
	Headless    bool

	// Pod info
	PodName string

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

// PrimaryHostname returns the first hostname or service name if no hostnames
func (f *ForwardSnapshot) PrimaryHostname() string {
	if len(f.Hostnames) > 0 {
		return f.Hostnames[0]
	}
	return f.ServiceName
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
