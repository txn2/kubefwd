package fwdtui

import (
	"sync"
	"testing"
	"time"

	"github.com/txn2/kubefwd/pkg/fwdtui/events"
)

func TestEventCollector_Collect(t *testing.T) {
	collector := NewEventCollector()

	event := events.Event{
		Type:      events.PodAdded,
		Service:   "test-svc",
		PodName:   "test-pod",
		Namespace: "default",
	}

	collector.Collect(event)

	if collector.Count() != 1 {
		t.Errorf("Expected count 1, got %d", collector.Count())
	}
}

func TestEventCollector_Events(t *testing.T) {
	collector := NewEventCollector()

	event1 := events.Event{Type: events.PodAdded, Service: "svc1"}
	event2 := events.Event{Type: events.PodRemoved, Service: "svc2"}

	collector.Collect(event1)
	collector.Collect(event2)

	allEvents := collector.Events()
	if len(allEvents) != 2 {
		t.Errorf("Expected 2 events, got %d", len(allEvents))
	}

	// Verify it returns a copy (modifying returned slice doesn't affect collector)
	allEvents[0] = events.Event{Service: "modified"}
	originalEvents := collector.Events()
	if originalEvents[0].Service == "modified" {
		t.Error("Events() should return a copy, not the original slice")
	}
}

func TestEventCollector_EventsOfType(t *testing.T) {
	collector := NewEventCollector()

	collector.Collect(events.Event{Type: events.PodAdded, Service: "svc1"})
	collector.Collect(events.Event{Type: events.PodRemoved, Service: "svc2"})
	collector.Collect(events.Event{Type: events.PodAdded, Service: "svc3"})

	podAddedEvents := collector.EventsOfType(events.PodAdded)
	if len(podAddedEvents) != 2 {
		t.Errorf("Expected 2 PodAdded events, got %d", len(podAddedEvents))
	}

	podRemovedEvents := collector.EventsOfType(events.PodRemoved)
	if len(podRemovedEvents) != 1 {
		t.Errorf("Expected 1 PodRemoved event, got %d", len(podRemovedEvents))
	}
}

func TestEventCollector_Count(t *testing.T) {
	collector := NewEventCollector()

	if collector.Count() != 0 {
		t.Errorf("Expected count 0 for new collector, got %d", collector.Count())
	}

	collector.Collect(events.Event{Type: events.PodAdded})
	collector.Collect(events.Event{Type: events.PodAdded})
	collector.Collect(events.Event{Type: events.PodAdded})

	if collector.Count() != 3 {
		t.Errorf("Expected count 3, got %d", collector.Count())
	}
}

func TestEventCollector_CountOfType(t *testing.T) {
	collector := NewEventCollector()

	collector.Collect(events.Event{Type: events.PodAdded})
	collector.Collect(events.Event{Type: events.PodRemoved})
	collector.Collect(events.Event{Type: events.PodAdded})
	collector.Collect(events.Event{Type: events.PodStatusChanged})

	if collector.CountOfType(events.PodAdded) != 2 {
		t.Errorf("Expected 2 PodAdded, got %d", collector.CountOfType(events.PodAdded))
	}

	if collector.CountOfType(events.PodRemoved) != 1 {
		t.Errorf("Expected 1 PodRemoved, got %d", collector.CountOfType(events.PodRemoved))
	}
}

func TestEventCollector_Clear(t *testing.T) {
	collector := NewEventCollector()

	collector.Collect(events.Event{Type: events.PodAdded})
	collector.Collect(events.Event{Type: events.PodAdded})

	if collector.Count() != 2 {
		t.Fatalf("Expected count 2 before clear, got %d", collector.Count())
	}

	collector.Clear()

	if collector.Count() != 0 {
		t.Errorf("Expected count 0 after clear, got %d", collector.Count())
	}
}

func TestEventCollector_WaitForEvents(t *testing.T) {
	collector := NewEventCollector()

	// Already has enough events
	collector.Collect(events.Event{Type: events.PodAdded})
	collector.Collect(events.Event{Type: events.PodAdded})

	if !collector.WaitForEvents(2, 100*time.Millisecond) {
		t.Error("WaitForEvents should return true when events are already present")
	}

	// Events arrive during wait
	collector.Clear()
	go func() {
		time.Sleep(20 * time.Millisecond)
		collector.Collect(events.Event{Type: events.PodAdded})
	}()

	if !collector.WaitForEvents(1, 500*time.Millisecond) {
		t.Error("WaitForEvents should return true when event arrives during wait")
	}

	// Timeout when not enough events
	collector.Clear()
	if collector.WaitForEvents(5, 50*time.Millisecond) {
		t.Error("WaitForEvents should return false on timeout")
	}
}

func TestEventCollector_WaitForEventType(t *testing.T) {
	collector := NewEventCollector()

	// Already has enough events of type
	collector.Collect(events.Event{Type: events.PodAdded})
	collector.Collect(events.Event{Type: events.PodRemoved})
	collector.Collect(events.Event{Type: events.PodAdded})

	if !collector.WaitForEventType(events.PodAdded, 2, 100*time.Millisecond) {
		t.Error("WaitForEventType should return true when events are already present")
	}

	// Events arrive during wait
	collector.Clear()
	go func() {
		time.Sleep(20 * time.Millisecond)
		collector.Collect(events.Event{Type: events.PodRemoved})
	}()

	if !collector.WaitForEventType(events.PodRemoved, 1, 500*time.Millisecond) {
		t.Error("WaitForEventType should return true when event arrives during wait")
	}

	// Timeout when not enough events of type
	collector.Clear()
	collector.Collect(events.Event{Type: events.PodAdded}) // Wrong type
	if collector.WaitForEventType(events.PodRemoved, 1, 50*time.Millisecond) {
		t.Error("WaitForEventType should return false on timeout")
	}
}

func TestEventCollector_ThreadSafety(t *testing.T) {
	collector := NewEventCollector()

	var wg sync.WaitGroup
	numGoroutines := 10
	eventsPerGoroutine := 100

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				collector.Collect(events.Event{Type: events.PodAdded})
			}
		}()
	}

	// Concurrent reads while writing
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = collector.Count()
				_ = collector.Events()
				_ = collector.EventsOfType(events.PodAdded)
				time.Sleep(time.Millisecond)
			}
		}()
	}

	wg.Wait()

	expectedCount := numGoroutines * eventsPerGoroutine
	if collector.Count() != expectedCount {
		t.Errorf("Expected %d events, got %d", expectedCount, collector.Count())
	}
}

func TestEnableTestMode(t *testing.T) {
	// Store original state
	mu.Lock()
	originalEnabled := tuiEnabled
	originalManager := tuiManager
	mu.Unlock()

	collector, cleanup := EnableTestMode()

	// Verify TUI is enabled
	if !IsEnabled() {
		t.Error("Expected TUI to be enabled after EnableTestMode")
	}

	// Verify events can be emitted and collected
	Emit(events.NewPodEvent(events.PodAdded, "test-svc", "test-pod", "default", "ctx", "8080"))

	if !collector.WaitForEventType(events.PodAdded, 1, 500*time.Millisecond) {
		t.Error("Expected to receive PodAdded event")
	}

	// Cleanup
	cleanup()

	// Verify state is restored
	mu.Lock()
	restoredEnabled := tuiEnabled
	restoredManager := tuiManager
	mu.Unlock()

	if restoredEnabled != originalEnabled {
		t.Error("Expected tuiEnabled to be restored after cleanup")
	}
	if restoredManager != originalManager {
		t.Error("Expected tuiManager to be restored after cleanup")
	}
}
