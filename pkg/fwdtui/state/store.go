package state

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// Maximum allocation limit for logs (CodeQL CWE-770 compliance)
const maxLogsAllocation = 10000

// boundedSize returns size bounded to limit for memory safety
func boundedSize(size, limit int) int {
	if size <= 0 {
		return 0
	}
	if size > limit {
		return limit
	}
	return size
}

// Store maintains the current state of all forwards for TUI rendering
type Store struct {
	mu       sync.RWMutex
	forwards map[string]*ForwardSnapshot // keyed by "service.namespace.context.podname"
	services map[string]*ServiceSnapshot // keyed by "service.namespace.context"

	// Blocked namespaces - prevents race condition where port forward events
	// arrive after namespace removal. Key format: "namespace.context"
	blockedNamespaces map[string]struct{}

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
		forwards:          make(map[string]*ForwardSnapshot),
		services:          make(map[string]*ServiceSnapshot),
		blockedNamespaces: make(map[string]struct{}),
		filter:            "",
		sortField:         "hostname",
		sortAsc:           true,
		logs:              make([]LogEntry, 0, maxLogSize),
		maxLogSize:        maxLogSize,
	}
}

// AddForward adds or updates a forward in the store
func (s *Store) AddForward(snapshot ForwardSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if this namespace is blocked (recently removed)
	// This prevents race condition where port forward events arrive after namespace removal
	nsKey := snapshot.Namespace + "." + snapshot.Context
	if _, blocked := s.blockedNamespaces[nsKey]; blocked {
		return // Silently ignore updates for blocked namespaces
	}

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

// compareStrings returns -1 if a < b, 1 if a > b, 0 if equal
func compareStrings(a, b string) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// compareInts returns -1 if a < b, 1 if a > b, 0 if equal
func compareInts[T ~int | ~int32 | ~int64 | ~uint32](a, b T) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// compareFloats returns -1 if a < b, 1 if a > b, 0 if equal
func compareFloats(a, b float64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// compareForwardsByField compares two forwards by the given field
func compareForwardsByField(a, b *ForwardSnapshot, field string) int {
	switch field {
	case "hostname":
		return compareStrings(a.PrimaryHostname(), b.PrimaryHostname())
	case "namespace":
		return compareStrings(a.Namespace, b.Namespace)
	case "service":
		return compareStrings(a.ServiceName, b.ServiceName)
	case "status":
		return compareInts(a.Status, b.Status)
	case "rateIn":
		return compareFloats(a.RateIn, b.RateIn)
	case "rateOut":
		return compareFloats(a.RateOut, b.RateOut)
	default:
		return compareStrings(a.PrimaryHostname(), b.PrimaryHostname())
	}
}

// sortForwards sorts forwards based on current sort settings
// Must be called with lock held
func (s *Store) sortForwards(forwards []ForwardSnapshot) {
	sort.Slice(forwards, func(i, j int) bool {
		cmp := compareForwardsByField(&forwards[i], &forwards[j], s.sortField)

		// Tiebreaker: sort by port for stable ordering
		if cmp == 0 {
			cmp = compareStrings(forwards[i].LocalPort, forwards[j].LocalPort)
		}

		if s.sortAsc {
			return cmp < 0
		}
		return cmp > 0
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

	// Explicit upper bound for memory safety (CodeQL CWE-770)
	allocSize := boundedSize(count, maxLogsAllocation)
	if allocSize == 0 {
		return nil
	}

	start := len(s.logs) - allocSize
	if start < 0 {
		start = 0
	}

	result := make([]LogEntry, allocSize)
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

// GetForward returns a forward by key, or nil if not found
func (s *Store) GetForward(key string) *ForwardSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if fwd, ok := s.forwards[key]; ok {
		// Return a copy to prevent concurrent modification
		snapshot := *fwd
		return &snapshot
	}
	return nil
}

// GetService returns a service by key, or nil if not found
func (s *Store) GetService(key string) *ServiceSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if svc, ok := s.services[key]; ok {
		// Return a copy to prevent concurrent modification
		snapshot := *svc
		return &snapshot
	}
	return nil
}

// UpdateHostnames updates the hostnames for a forward
// Called when PodStatusChanged "active" event arrives with populated hostnames
func (s *Store) UpdateHostnames(key string, hostnames []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if fwd, ok := s.forwards[key]; ok {
		fwd.Hostnames = hostnames
	}
}

// RemoveByNamespace removes all forwards and services for a given namespace/context
// This is used when a namespace watcher is stopped to clean up orphaned state
func (s *Store) RemoveByNamespace(namespace, context string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Block this namespace FIRST to prevent race condition with in-flight events
	// Any AddForward calls for this namespace will be ignored after this point
	nsKey := namespace + "." + context
	s.blockedNamespaces[nsKey] = struct{}{}

	removedCount := 0

	// Find and remove all forwards matching the namespace/context
	forwardsToRemove := make([]string, 0)
	for key, fwd := range s.forwards {
		if fwd.Namespace == namespace && fwd.Context == context {
			forwardsToRemove = append(forwardsToRemove, key)
		}
	}

	for _, key := range forwardsToRemove {
		delete(s.forwards, key)
		removedCount++
	}

	// Then, find and remove all services matching the namespace/context
	servicesToRemove := make([]string, 0)
	for key, svc := range s.services {
		if svc.Namespace == namespace && svc.Context == context {
			servicesToRemove = append(servicesToRemove, key)
		}
	}

	for _, key := range servicesToRemove {
		delete(s.services, key)
	}

	return removedCount
}

// UnblockNamespace removes a namespace from the blocked list, allowing new forwards to be added.
// This should be called when a namespace is being re-added after previous removal.
func (s *Store) UnblockNamespace(namespace, context string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	nsKey := namespace + "." + context
	delete(s.blockedNamespaces, nsKey)
}
