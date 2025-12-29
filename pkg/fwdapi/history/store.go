// Package history provides a ring buffer-based store for tracking
// events, errors, and reconnection history in kubefwd.
package history

import (
	"sync"
	"time"
)

// EventType categorizes events
type EventType string

const (
	EventServiceAdded     EventType = "ServiceAdded"
	EventServiceRemoved   EventType = "ServiceRemoved"
	EventForwardStarted   EventType = "ForwardStarted"
	EventForwardStopped   EventType = "ForwardStopped"
	EventForwardError     EventType = "ForwardError"
	EventPodStatusChanged EventType = "PodStatusChanged"
	EventReconnectStarted EventType = "ReconnectStarted"
	EventReconnectSuccess EventType = "ReconnectSuccess"
	EventReconnectFailed  EventType = "ReconnectFailed"
	EventSyncTriggered    EventType = "SyncTriggered"
)

// Event represents a historical event
type Event struct {
	ID         int64                  `json:"id"`
	Type       EventType              `json:"type"`
	Timestamp  time.Time              `json:"timestamp"`
	ServiceKey string                 `json:"serviceKey,omitempty"`
	ForwardKey string                 `json:"forwardKey,omitempty"`
	PodName    string                 `json:"podName,omitempty"`
	Message    string                 `json:"message"`
	Data       map[string]interface{} `json:"data,omitempty"`
}

// ErrorRecord represents a historical error
type ErrorRecord struct {
	ID         int64     `json:"id"`
	Timestamp  time.Time `json:"timestamp"`
	ServiceKey string    `json:"serviceKey"`
	ForwardKey string    `json:"forwardKey,omitempty"`
	PodName    string    `json:"podName,omitempty"`
	ErrorType  string    `json:"errorType"`
	Message    string    `json:"message"`
	Resolved   bool      `json:"resolved"`
	ResolvedAt time.Time `json:"resolvedAt,omitempty"`
}

// ReconnectRecord represents a reconnection attempt
type ReconnectRecord struct {
	ID           int64         `json:"id"`
	Timestamp    time.Time     `json:"timestamp"`
	ServiceKey   string        `json:"serviceKey"`
	Trigger      string        `json:"trigger"` // "auto", "manual", "sync"
	AttemptCount int           `json:"attemptCount"`
	Success      bool          `json:"success"`
	Duration     time.Duration `json:"duration"`
	Error        string        `json:"error,omitempty"`
}

// Store provides thread-safe storage for historical data
type Store struct {
	mu sync.RWMutex

	// Ring buffers with indices
	events      []Event
	eventIndex  int
	eventCount  int
	eventNextID int64

	errors      []ErrorRecord
	errorIndex  int
	errorCount  int
	errorNextID int64

	reconnects      []ReconnectRecord
	reconnectIndex  int
	reconnectCount  int
	reconnectNextID int64

	// Configuration
	maxEvents     int
	maxErrors     int
	maxReconnects int
}

// Config configures the history store
type Config struct {
	MaxEvents     int
	MaxErrors     int
	MaxReconnects int
}

// Maximum allocation limits (for CodeQL CWE-770 compliance)
const (
	maxEventsLimit     = 10000
	maxErrorsLimit     = 5000
	maxReconnectsLimit = 2000
)

// boundedSize returns size bounded to limit, providing explicit flow control
// that static analyzers can verify. Returns 0 for negative inputs.
func boundedSize(size, limit int) int {
	if size <= 0 {
		return 0
	}
	if size > limit {
		return limit
	}
	return size
}

// DefaultConfig returns default configuration
func DefaultConfig() Config {
	return Config{
		MaxEvents:     1000,
		MaxErrors:     500,
		MaxReconnects: 200,
	}
}

var (
	globalStore *Store
	storeOnce   sync.Once
)

// GetStore returns the global history store
func GetStore() *Store {
	storeOnce.Do(func() {
		globalStore = NewStore(DefaultConfig())
	})
	return globalStore
}

// NewStore creates a new history store
func NewStore(cfg Config) *Store {
	if cfg.MaxEvents <= 0 {
		cfg.MaxEvents = 1000
	}
	if cfg.MaxErrors <= 0 {
		cfg.MaxErrors = 500
	}
	if cfg.MaxReconnects <= 0 {
		cfg.MaxReconnects = 200
	}

	return &Store{
		events:          make([]Event, cfg.MaxEvents),
		errors:          make([]ErrorRecord, cfg.MaxErrors),
		reconnects:      make([]ReconnectRecord, cfg.MaxReconnects),
		maxEvents:       cfg.MaxEvents,
		maxErrors:       cfg.MaxErrors,
		maxReconnects:   cfg.MaxReconnects,
		eventNextID:     1,
		errorNextID:     1,
		reconnectNextID: 1,
	}
}

// RecordEvent adds an event to history
func (s *Store) RecordEvent(eventType EventType, serviceKey, forwardKey, podName, message string, data map[string]interface{}) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := s.eventNextID
	s.eventNextID++

	event := Event{
		ID:         id,
		Type:       eventType,
		Timestamp:  time.Now(),
		ServiceKey: serviceKey,
		ForwardKey: forwardKey,
		PodName:    podName,
		Message:    message,
		Data:       data,
	}

	s.events[s.eventIndex] = event
	s.eventIndex = (s.eventIndex + 1) % s.maxEvents
	if s.eventCount < s.maxEvents {
		s.eventCount++
	}

	return id
}

// RecordError adds an error to history
func (s *Store) RecordError(serviceKey, forwardKey, podName, errorType, message string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := s.errorNextID
	s.errorNextID++

	record := ErrorRecord{
		ID:         id,
		Timestamp:  time.Now(),
		ServiceKey: serviceKey,
		ForwardKey: forwardKey,
		PodName:    podName,
		ErrorType:  errorType,
		Message:    message,
		Resolved:   false,
	}

	s.errors[s.errorIndex] = record
	s.errorIndex = (s.errorIndex + 1) % s.maxErrors
	if s.errorCount < s.maxErrors {
		s.errorCount++
	}

	return id
}

// RecordReconnect adds a reconnection attempt to history
func (s *Store) RecordReconnect(serviceKey, trigger string, attemptCount int, success bool, duration time.Duration, errMsg string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := s.reconnectNextID
	s.reconnectNextID++

	record := ReconnectRecord{
		ID:           id,
		Timestamp:    time.Now(),
		ServiceKey:   serviceKey,
		Trigger:      trigger,
		AttemptCount: attemptCount,
		Success:      success,
		Duration:     duration,
		Error:        errMsg,
	}

	s.reconnects[s.reconnectIndex] = record
	s.reconnectIndex = (s.reconnectIndex + 1) % s.maxReconnects
	if s.reconnectCount < s.maxReconnects {
		s.reconnectCount++
	}

	return id
}

// GetEvents returns recent events
func (s *Store) GetEvents(count int, eventType EventType) []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if count <= 0 || s.eventCount == 0 {
		return nil
	}
	if count > s.eventCount {
		count = s.eventCount
	}

	// Collect matching events
	var result []Event
	for i := 0; i < s.eventCount && len(result) < count; i++ {
		idx := (s.eventIndex - 1 - i + s.maxEvents) % s.maxEvents
		event := s.events[idx]
		if event.ID == 0 {
			continue
		}
		if eventType != "" && event.Type != eventType {
			continue
		}
		result = append(result, event)
	}

	return result
}

// GetErrors returns recent errors
func (s *Store) GetErrors(count int) []ErrorRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if count <= 0 || s.errorCount == 0 {
		return nil
	}
	if count > s.errorCount {
		count = s.errorCount
	}

	// Explicit upper bound for memory safety (CodeQL CWE-770)
	allocSize := boundedSize(count, maxErrorsLimit)
	if allocSize == 0 {
		return nil
	}

	result := make([]ErrorRecord, 0, allocSize)
	for i := 0; i < count && i < s.errorCount; i++ {
		idx := (s.errorIndex - 1 - i + s.maxErrors) % s.maxErrors
		record := s.errors[idx]
		if record.ID == 0 {
			continue
		}
		result = append(result, record)
	}

	return result
}

// GetReconnects returns recent reconnection attempts
func (s *Store) GetReconnects(count int, serviceKey string) []ReconnectRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if count <= 0 || s.reconnectCount == 0 {
		return nil
	}
	if count > s.reconnectCount {
		count = s.reconnectCount
	}

	var result []ReconnectRecord
	for i := 0; i < s.reconnectCount && len(result) < count; i++ {
		idx := (s.reconnectIndex - 1 - i + s.maxReconnects) % s.maxReconnects
		record := s.reconnects[idx]
		if record.ID == 0 {
			continue
		}
		if serviceKey != "" && record.ServiceKey != serviceKey {
			continue
		}
		result = append(result, record)
	}

	return result
}

// GetStats returns statistics about stored history
func (s *Store) GetStats() HistoryStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return HistoryStats{
		TotalEvents:     s.eventCount,
		TotalErrors:     s.errorCount,
		TotalReconnects: s.reconnectCount,
		MaxEvents:       s.maxEvents,
		MaxErrors:       s.maxErrors,
		MaxReconnects:   s.maxReconnects,
	}
}

// HistoryStats provides statistics about stored history
type HistoryStats struct {
	TotalEvents     int `json:"totalEvents"`
	TotalErrors     int `json:"totalErrors"`
	TotalReconnects int `json:"totalReconnects"`
	MaxEvents       int `json:"maxEvents"`
	MaxErrors       int `json:"maxErrors"`
	MaxReconnects   int `json:"maxReconnects"`
}

// Clear removes all history
func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.events = make([]Event, s.maxEvents)
	s.eventIndex = 0
	s.eventCount = 0

	s.errors = make([]ErrorRecord, s.maxErrors)
	s.errorIndex = 0
	s.errorCount = 0

	s.reconnects = make([]ReconnectRecord, s.maxReconnects)
	s.reconnectIndex = 0
	s.reconnectCount = 0
}
