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
	"time"
)

// PortForwardSnapshot is an immutable snapshot for TUI rendering
type PortForwardSnapshot struct {
	ServiceName    string
	Namespace      string
	Context        string
	PodName        string
	LocalIP        string
	LocalPort      string
	PodPort        string
	BytesIn        uint64
	BytesOut       uint64
	RateIn         float64 // bytes/sec instantaneous
	RateOut        float64 // bytes/sec instantaneous
	AvgRateIn      float64 // bytes/sec 10-second average
	AvgRateOut     float64 // bytes/sec 10-second average
	ConnectedAt    time.Time
	LastActivityAt time.Time
	History        []RateSample // for graphing
}

// ServiceSnapshot aggregates all port forwards for a service
type ServiceSnapshot struct {
	ServiceName   string
	Namespace     string
	Context       string
	TotalBytesIn  uint64
	TotalBytesOut uint64
	TotalRateIn   float64
	TotalRateOut  float64
	PortForwards  []PortForwardSnapshot
}

// GetSnapshot creates an immutable snapshot of port forward metrics
func (pf *PortForwardMetrics) GetSnapshot() PortForwardSnapshot {
	rateIn, rateOut := pf.GetInstantRate()
	avgRateIn, avgRateOut := pf.GetAverageRate(10)

	return PortForwardSnapshot{
		ServiceName:    pf.ServiceName,
		Namespace:      pf.Namespace,
		Context:        pf.Context,
		PodName:        pf.PodName,
		LocalIP:        pf.LocalIP,
		LocalPort:      pf.LocalPort,
		PodPort:        pf.PodPort,
		BytesIn:        pf.GetBytesIn(),
		BytesOut:       pf.GetBytesOut(),
		RateIn:         rateIn,
		RateOut:        rateOut,
		AvgRateIn:      avgRateIn,
		AvgRateOut:     avgRateOut,
		ConnectedAt:    pf.ConnectedAt,
		LastActivityAt: pf.GetLastActivity(),
		History:        pf.GetHistory(30),
	}
}

// GetSnapshot creates an immutable snapshot of service metrics
func (sm *ServiceMetrics) GetSnapshot() ServiceSnapshot {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	snapshot := ServiceSnapshot{
		ServiceName:  sm.ServiceName,
		Namespace:    sm.Namespace,
		Context:      sm.Context,
		PortForwards: make([]PortForwardSnapshot, 0, len(sm.PortForwards)),
	}

	for _, pf := range sm.PortForwards {
		pfSnapshot := pf.GetSnapshot()
		snapshot.PortForwards = append(snapshot.PortForwards, pfSnapshot)
		snapshot.TotalBytesIn += pfSnapshot.BytesIn
		snapshot.TotalBytesOut += pfSnapshot.BytesOut
		snapshot.TotalRateIn += pfSnapshot.RateIn
		snapshot.TotalRateOut += pfSnapshot.RateOut
	}

	return snapshot
}

// GetAllSnapshots returns current state for TUI rendering
func (r *Registry) GetAllSnapshots() []ServiceSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()

	snapshots := make([]ServiceSnapshot, 0, len(r.services))

	for _, svc := range r.services {
		snapshots = append(snapshots, svc.GetSnapshot())
	}

	return snapshots
}

// GetServiceSnapshot returns snapshot for single service
func (r *Registry) GetServiceSnapshot(key string) *ServiceSnapshot {
	r.mu.RLock()
	svc, ok := r.services[key]
	r.mu.RUnlock()

	if !ok {
		return nil
	}

	snapshot := svc.GetSnapshot()
	return &snapshot
}

// Subscribe returns a channel that receives updates at specified interval
func (r *Registry) Subscribe(interval time.Duration) (<-chan []ServiceSnapshot, func()) {
	ch := make(chan []ServiceSnapshot, 1)
	stopCh := make(chan struct{})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		defer close(ch)

		for {
			select {
			case <-ticker.C:
				select {
				case ch <- r.GetAllSnapshots():
				default:
					// Channel full, skip this update
				}
			case <-stopCh:
				return
			case <-r.stopCh:
				return
			}
		}
	}()

	cancel := func() { close(stopCh) }
	return ch, cancel
}
