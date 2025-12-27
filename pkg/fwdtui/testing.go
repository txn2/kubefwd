package fwdtui

import (
	"sync"
	"time"

	"github.com/txn2/kubefwd/pkg/fwdtui/events"
)

// EventCollector captures events for test assertions.
// It is thread-safe and can be used to verify event emission in tests.
type EventCollector struct {
	mu     sync.Mutex
	events []events.Event
}

// NewEventCollector creates a new event collector
func NewEventCollector() *EventCollector {
	return &EventCollector{
		events: make([]events.Event, 0),
	}
}

// Collect adds an event to the collector (implements events.Handler)
func (c *EventCollector) Collect(e events.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
}

// Events returns a copy of all collected events
func (c *EventCollector) Events() []events.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]events.Event, len(c.events))
	copy(result, c.events)
	return result
}

// EventsOfType returns all events of a specific type
func (c *EventCollector) EventsOfType(t events.EventType) []events.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]events.Event, 0)
	for _, e := range c.events {
		if e.Type == t {
			result = append(result, e)
		}
	}
	return result
}

// Count returns the total number of collected events
func (c *EventCollector) Count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.events)
}

// CountOfType returns the count of events of a specific type
func (c *EventCollector) CountOfType(t events.EventType) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	count := 0
	for _, e := range c.events {
		if e.Type == t {
			count++
		}
	}
	return count
}

// Clear removes all collected events
func (c *EventCollector) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = make([]events.Event, 0)
}

// WaitForEvents waits until at least n events are collected or timeout
func (c *EventCollector) WaitForEvents(n int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if c.Count() >= n {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return c.Count() >= n
}

// WaitForEventType waits until at least n events of type t are collected or timeout
func (c *EventCollector) WaitForEventType(t events.EventType, n int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if c.CountOfType(t) >= n {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return c.CountOfType(t) >= n
}

// EnableTestMode initializes the TUI system in test mode with event collection.
// It sets up a minimal event bus and enables TUI mode so that Emit() calls work.
// Returns an EventCollector that captures all emitted events and a cleanup function.
func EnableTestMode() (*EventCollector, func()) {
	collector := NewEventCollector()

	// Create a minimal event bus for testing
	bus := events.NewBus(1000)
	bus.SubscribeAll(collector.Collect)
	bus.Start()

	// Set up the global TUI state for testing
	mu.Lock()
	oldEnabled := tuiEnabled
	oldManager := tuiManager
	tuiEnabled = true
	tuiManager = &Manager{
		eventBus: bus,
	}
	mu.Unlock()

	// Return cleanup function
	cleanup := func() {
		bus.Stop()
		mu.Lock()
		tuiEnabled = oldEnabled
		tuiManager = oldManager
		mu.Unlock()
	}

	return collector, cleanup
}
