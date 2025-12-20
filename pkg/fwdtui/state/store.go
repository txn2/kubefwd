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
	"sort"
	"strings"
	"sync"
	"time"
)

// Store maintains the current state of all forwards for TUI rendering
type Store struct {
	mu       sync.RWMutex
	forwards map[string]*ForwardSnapshot // keyed by "service.namespace.context.podname"
	services map[string]*ServiceSnapshot // keyed by "service.namespace.context"

	// Display options
	filter    string
	sortField string
	sortAsc   bool

	// Log buffer
	logs       []LogEntry
	maxLogSize int
}

// NewStore creates a new state store
func NewStore(maxLogSize int) *Store {
	if maxLogSize <= 0 {
		maxLogSize = 1000
	}
	return &Store{
		forwards:   make(map[string]*ForwardSnapshot),
		services:   make(map[string]*ServiceSnapshot),
		filter:     "",
		sortField:  "hostname",
		sortAsc:    true,
		logs:       make([]LogEntry, 0, maxLogSize),
		maxLogSize: maxLogSize,
	}
}

// AddForward adds or updates a forward in the store
func (s *Store) AddForward(snapshot ForwardSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.forwards[snapshot.Key] = &snapshot

	// Update or create service aggregate
	if svc, ok := s.services[snapshot.ServiceKey]; ok {
		s.updateServiceAggregate(svc)
	} else {
		s.services[snapshot.ServiceKey] = &ServiceSnapshot{
			Key:         snapshot.ServiceKey,
			ServiceName: snapshot.ServiceName,
			Namespace:   snapshot.Namespace,
			Context:     snapshot.Context,
			Headless:    snapshot.Headless,
		}
		s.updateServiceAggregate(s.services[snapshot.ServiceKey])
	}
}

// RemoveForward removes a forward from the store
func (s *Store) RemoveForward(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if fwd, ok := s.forwards[key]; ok {
		serviceKey := fwd.ServiceKey
		delete(s.forwards, key)

		// Update service aggregate
		if svc, ok := s.services[serviceKey]; ok {
			s.updateServiceAggregate(svc)
			// Remove service if no forwards left
			if len(svc.PortForwards) == 0 {
				delete(s.services, serviceKey)
			}
		}
	}
}

// UpdateMetrics updates bandwidth metrics for a forward
func (s *Store) UpdateMetrics(key string, bytesIn, bytesOut uint64, rateIn, rateOut, avgRateIn, avgRateOut float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if fwd, ok := s.forwards[key]; ok {
		fwd.BytesIn = bytesIn
		fwd.BytesOut = bytesOut
		fwd.RateIn = rateIn
		fwd.RateOut = rateOut
		fwd.AvgRateIn = avgRateIn
		fwd.AvgRateOut = avgRateOut
		fwd.LastActive = time.Now()

		// Update service aggregate
		if svc, ok := s.services[fwd.ServiceKey]; ok {
			s.updateServiceAggregate(svc)
		}
	}
}

// UpdateStatus updates the status of a forward
func (s *Store) UpdateStatus(key string, status ForwardStatus, errorMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if fwd, ok := s.forwards[key]; ok {
		fwd.Status = status
		fwd.Error = errorMsg

		// Update service aggregate
		if svc, ok := s.services[fwd.ServiceKey]; ok {
			s.updateServiceAggregate(svc)
		}
	}
}

// updateServiceAggregate recalculates service-level aggregates
// Must be called with lock held
func (s *Store) updateServiceAggregate(svc *ServiceSnapshot) {
	svc.PortForwards = make([]ForwardSnapshot, 0)
	svc.TotalBytesIn = 0
	svc.TotalBytesOut = 0
	svc.TotalRateIn = 0
	svc.TotalRateOut = 0
	svc.ActiveCount = 0
	svc.ErrorCount = 0

	for _, fwd := range s.forwards {
		if fwd.ServiceKey == svc.Key {
			svc.PortForwards = append(svc.PortForwards, *fwd)
			svc.TotalBytesIn += fwd.BytesIn
			svc.TotalBytesOut += fwd.BytesOut
			svc.TotalRateIn += fwd.RateIn
			svc.TotalRateOut += fwd.RateOut
			if fwd.Status == StatusActive {
				svc.ActiveCount++
			}
			if fwd.Status == StatusError {
				svc.ErrorCount++
			}
		}
	}
}

// GetFiltered returns forwards matching the current filter, sorted
func (s *Store) GetFiltered() []ForwardSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]ForwardSnapshot, 0, len(s.forwards))

	for _, fwd := range s.forwards {
		if s.matchesFilter(fwd) {
			result = append(result, *fwd)
		}
	}

	s.sortForwards(result)
	return result
}

// GetServices returns all services with their forwards
func (s *Store) GetServices() []ServiceSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]ServiceSnapshot, 0, len(s.services))
	for _, svc := range s.services {
		result = append(result, *svc)
	}

	// Sort by service name
	sort.Slice(result, func(i, j int) bool {
		return result[i].ServiceName < result[j].ServiceName
	})

	return result
}

// GetSummary returns overall statistics
func (s *Store) GetSummary() SummaryStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := SummaryStats{
		TotalServices: len(s.services),
		TotalForwards: len(s.forwards),
		LastUpdated:   time.Now(),
	}

	for _, svc := range s.services {
		if svc.ActiveCount > 0 {
			stats.ActiveServices++
		}
	}

	for _, fwd := range s.forwards {
		if fwd.Status == StatusActive {
			stats.ActiveForwards++
		}
		if fwd.Status == StatusError {
			stats.ErrorCount++
		}
		stats.TotalBytesIn += fwd.BytesIn
		stats.TotalBytesOut += fwd.BytesOut
		stats.TotalRateIn += fwd.RateIn
		stats.TotalRateOut += fwd.RateOut
	}

	return stats
}

// SetFilter sets the current filter text
func (s *Store) SetFilter(filter string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.filter = strings.ToLower(filter)
}

// GetFilter returns the current filter text
func (s *Store) GetFilter() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.filter
}

// SetSort sets the sort field and direction
func (s *Store) SetSort(field string, ascending bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sortField = field
	s.sortAsc = ascending
}

// matchesFilter checks if a forward matches the current filter
// Must be called with lock held
func (s *Store) matchesFilter(fwd *ForwardSnapshot) bool {
	if s.filter == "" {
		return true
	}

	// Check hostnames
	for _, h := range fwd.Hostnames {
		if strings.Contains(strings.ToLower(h), s.filter) {
			return true
		}
	}

	// Check service name
	if strings.Contains(strings.ToLower(fwd.ServiceName), s.filter) {
		return true
	}

	// Check namespace
	if strings.Contains(strings.ToLower(fwd.Namespace), s.filter) {
		return true
	}

	// Check pod name
	if strings.Contains(strings.ToLower(fwd.PodName), s.filter) {
		return true
	}

	return false
}

// sortForwards sorts forwards based on current sort settings
// Must be called with lock held
func (s *Store) sortForwards(forwards []ForwardSnapshot) {
	sort.Slice(forwards, func(i, j int) bool {
		var cmp bool
		switch s.sortField {
		case "hostname":
			cmp = forwards[i].PrimaryHostname() < forwards[j].PrimaryHostname()
		case "namespace":
			cmp = forwards[i].Namespace < forwards[j].Namespace
		case "service":
			cmp = forwards[i].ServiceName < forwards[j].ServiceName
		case "status":
			cmp = forwards[i].Status < forwards[j].Status
		case "rateIn":
			cmp = forwards[i].RateIn < forwards[j].RateIn
		case "rateOut":
			cmp = forwards[i].RateOut < forwards[j].RateOut
		default:
			cmp = forwards[i].PrimaryHostname() < forwards[j].PrimaryHostname()
		}

		if s.sortAsc {
			return cmp
		}
		return !cmp
	})
}

// AddLog adds a log entry to the buffer
func (s *Store) AddLog(entry LogEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.logs = append(s.logs, entry)
	// Trim if exceeding max size
	if len(s.logs) > s.maxLogSize {
		s.logs = s.logs[len(s.logs)-s.maxLogSize:]
	}
}

// GetLogs returns recent log entries
func (s *Store) GetLogs(count int) []LogEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if count <= 0 || count > len(s.logs) {
		count = len(s.logs)
	}

	start := len(s.logs) - count
	if start < 0 {
		start = 0
	}

	result := make([]LogEntry, count)
	copy(result, s.logs[start:])
	return result
}

// Count returns the total number of forwards
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.forwards)
}

// ServiceCount returns the total number of services
func (s *Store) ServiceCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.services)
}
