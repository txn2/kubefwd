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

package fwdmetrics

import (
	"sync"
	"sync/atomic"
	"time"
)

// PortForwardMetrics tracks bandwidth for a single port forward
type PortForwardMetrics struct {
	// Atomic counters for hot path (Read/Write operations)
	BytesIn      uint64 // Total bytes received from pod
	BytesOut     uint64 // Total bytes sent to pod
	lastActivity int64  // Unix nano timestamp of last activity

	// Immutable after creation
	ServiceName string
	Namespace   string
	Context     string
	PodName     string
	LocalIP     string
	LocalPort   string
	PodPort     string
	ConnectedAt time.Time

	// Rate calculation (protected by RateCalculator's mutex)
	rateCalc *RateCalculator
}

// NewPortForwardMetrics creates a new metrics tracker for a port forward
func NewPortForwardMetrics(serviceName, namespace, context, podName, localIP, localPort, podPort string) *PortForwardMetrics {
	return &PortForwardMetrics{
		ServiceName: serviceName,
		Namespace:   namespace,
		Context:     context,
		PodName:     podName,
		LocalIP:     localIP,
		LocalPort:   localPort,
		PodPort:     podPort,
		ConnectedAt: time.Now(),
		rateCalc:    NewRateCalculator(DefaultMaxSamples),
	}
}

// Key returns a unique key for this port forward
func (m *PortForwardMetrics) Key() string {
	return m.ServiceName + "." + m.Namespace + "." + m.Context + "." + m.PodName
}

// ServiceKey returns the service identifier
func (m *PortForwardMetrics) ServiceKey() string {
	return m.ServiceName + "." + m.Namespace + "." + m.Context
}

// AddBytesIn adds bytes to the incoming counter
func (m *PortForwardMetrics) AddBytesIn(n uint64) {
	atomic.AddUint64(&m.BytesIn, n)
	atomic.StoreInt64(&m.lastActivity, time.Now().UnixNano())
}

// AddBytesOut adds bytes to the outgoing counter
func (m *PortForwardMetrics) AddBytesOut(n uint64) {
	atomic.AddUint64(&m.BytesOut, n)
	atomic.StoreInt64(&m.lastActivity, time.Now().UnixNano())
}

// GetBytesIn returns the total bytes received
func (m *PortForwardMetrics) GetBytesIn() uint64 {
	return atomic.LoadUint64(&m.BytesIn)
}

// GetBytesOut returns the total bytes sent
func (m *PortForwardMetrics) GetBytesOut() uint64 {
	return atomic.LoadUint64(&m.BytesOut)
}

// GetLastActivity returns the time of last activity
func (m *PortForwardMetrics) GetLastActivity() time.Time {
	return time.Unix(0, atomic.LoadInt64(&m.lastActivity))
}

// RecordSample records current counters for rate calculation
func (m *PortForwardMetrics) RecordSample() {
	m.rateCalc.AddSample(m.GetBytesIn(), m.GetBytesOut(), time.Now())
}

// GetInstantRate returns instantaneous rate in bytes/sec
func (m *PortForwardMetrics) GetInstantRate() (rateIn, rateOut float64) {
	return m.rateCalc.GetInstantRate()
}

// GetAverageRate returns average rate over window seconds
func (m *PortForwardMetrics) GetAverageRate(windowSeconds int) (rateIn, rateOut float64) {
	return m.rateCalc.GetAverageRate(windowSeconds)
}

// GetHistory returns recent samples for graphing
func (m *PortForwardMetrics) GetHistory(count int) []RateSample {
	return m.rateCalc.GetHistory(count)
}

// RateSample represents a point-in-time measurement
type RateSample struct {
	Timestamp time.Time
	BytesIn   uint64
	BytesOut  uint64
}

// ServiceMetrics aggregates metrics for all port forwards of a service
type ServiceMetrics struct {
	ServiceName  string
	Namespace    string
	Context      string
	PortForwards map[string]*PortForwardMetrics // key: podName:localPort
	mu           sync.RWMutex
}

// NewServiceMetrics creates a new service metrics aggregator
func NewServiceMetrics(serviceName, namespace, context string) *ServiceMetrics {
	return &ServiceMetrics{
		ServiceName:  serviceName,
		Namespace:    namespace,
		Context:      context,
		PortForwards: make(map[string]*PortForwardMetrics),
	}
}

// Key returns a unique key for this service
func (s *ServiceMetrics) Key() string {
	return s.ServiceName + "." + s.Namespace + "." + s.Context
}

// AddPortForward adds a port forward to this service
func (s *ServiceMetrics) AddPortForward(pf *PortForwardMetrics) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := pf.PodName + ":" + pf.LocalPort
	s.PortForwards[key] = pf
}

// RemovePortForward removes a port forward from this service
func (s *ServiceMetrics) RemovePortForward(podName, localPort string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := podName + ":" + localPort
	delete(s.PortForwards, key)
}

// GetTotals returns aggregated totals for the service
func (s *ServiceMetrics) GetTotals() (bytesIn, bytesOut uint64, rateIn, rateOut float64) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, pf := range s.PortForwards {
		bytesIn += pf.GetBytesIn()
		bytesOut += pf.GetBytesOut()
		ri, ro := pf.GetInstantRate()
		rateIn += ri
		rateOut += ro
	}
	return
}

// Count returns the number of port forwards
func (s *ServiceMetrics) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.PortForwards)
}
