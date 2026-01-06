package events

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestNewBus tests creating a new event bus
func TestNewBus(t *testing.T) {
	bus := NewBus(100)

	if bus == nil {
		t.Fatal("Expected non-nil bus")
	}

	if bus.handlers == nil {
		t.Error("Expected handlers map to be initialized")
	}

	if bus.allHandle == nil {
		t.Error("Expected allHandle slice to be initialized")
	}
}

// TestNewBus_DefaultBufferSize tests default buffer size
func TestNewBus_DefaultBufferSize(t *testing.T) {
	bus := NewBus(0) // Should default to 1000

	if bus == nil {
		t.Fatal("Expected non-nil bus")
	}

	bus2 := NewBus(-10) // Negative should also use default

	if bus2 == nil {
		t.Fatal("Expected non-nil bus for negative buffer size")
	}
}

// TestSubscribe tests subscribing to specific event types
func TestSubscribe(t *testing.T) {
	bus := NewBus(100)

	var received int32
	bus.Subscribe(PodAdded, func(_ Event) {
		atomic.StoreInt32(&received, 1)
	})

	bus.Start()
	defer bus.Stop()

	bus.Publish(NewPodEvent(PodAdded, "svc", "ns", "ctx", "pod", "svc.ns.ctx"))

	// Wait for event to be processed
	time.Sleep(50 * time.Millisecond)

	if atomic.LoadInt32(&received) != 1 {
		t.Error("Expected handler to be called")
	}
}

// TestSubscribe_OnlyReceivesSubscribedType tests that handlers only receive subscribed types
func TestSubscribe_OnlyReceivesSubscribedType(t *testing.T) {
	bus := NewBus(100)

	var podAddedCount int32
	bus.Subscribe(PodAdded, func(_ Event) {
		atomic.AddInt32(&podAddedCount, 1)
	})

	var podRemovedCount int32
	bus.Subscribe(PodRemoved, func(_ Event) {
		atomic.AddInt32(&podRemovedCount, 1)
	})

	bus.Start()
	defer bus.Stop()

	bus.Publish(NewPodEvent(PodAdded, "svc", "ns", "ctx", "pod", "svc.ns.ctx"))
	bus.Publish(NewPodEvent(PodAdded, "svc", "ns", "ctx", "pod2", "svc.ns.ctx"))
	bus.Publish(NewPodEvent(PodRemoved, "svc", "ns", "ctx", "pod", "svc.ns.ctx"))

	time.Sleep(50 * time.Millisecond)

	if atomic.LoadInt32(&podAddedCount) != 2 {
		t.Errorf("Expected 2 PodAdded events, got %d", atomic.LoadInt32(&podAddedCount))
	}

	if atomic.LoadInt32(&podRemovedCount) != 1 {
		t.Errorf("Expected 1 PodRemoved event, got %d", atomic.LoadInt32(&podRemovedCount))
	}
}

// TestSubscribeAll tests subscribing to all event types
func TestSubscribeAll(t *testing.T) {
	bus := NewBus(100)

	var count int32
	bus.SubscribeAll(func(_ Event) {
		atomic.AddInt32(&count, 1)
	})

	bus.Start()
	defer bus.Stop()

	bus.Publish(NewPodEvent(PodAdded, "svc", "ns", "ctx", "pod", "svc.ns.ctx"))
	bus.Publish(NewServiceEvent(ServiceAdded, "svc", "ns", "ctx"))
	bus.Publish(NewPodEvent(PodRemoved, "svc", "ns", "ctx", "pod", "svc.ns.ctx"))

	time.Sleep(50 * time.Millisecond)

	if atomic.LoadInt32(&count) != 3 {
		t.Errorf("Expected 3 events received, got %d", count)
	}
}

// TestPublish_NonBlocking tests that Publish doesn't block when buffer is full
func TestPublish_NonBlocking(t *testing.T) {
	bus := NewBus(1) // Very small buffer

	// Don't start the bus - events will accumulate

	done := make(chan bool, 1)

	go func() {
		// Publish many events - should not block
		for i := 0; i < 100; i++ {
			bus.Publish(NewPodEvent(PodAdded, "svc", "ns", "ctx", "pod", "svc.ns.ctx"))
		}
		done <- true
	}()

	select {
	case <-done:
		// Good - didn't block
	case <-time.After(1 * time.Second):
		t.Error("Publish blocked when buffer was full")
	}
}

// TestStop tests stopping the event bus
func TestStop(t *testing.T) {
	bus := NewBus(100)

	bus.Start()

	// Publish some events
	bus.Publish(NewPodEvent(PodAdded, "svc", "ns", "ctx", "pod", "svc.ns.ctx"))

	// Stop should complete without hanging
	done := make(chan bool, 1)
	go func() {
		bus.Stop()
		done <- true
	}()

	select {
	case <-done:
		// Good
	case <-time.After(1 * time.Second):
		t.Error("Stop hung")
	}
}

// TestStop_DrainsEvents tests that Stop drains pending events
func TestStop_DrainsEvents(t *testing.T) {
	bus := NewBus(100)

	var count int32
	bus.SubscribeAll(func(_ Event) {
		atomic.AddInt32(&count, 1)
	})

	bus.Start()

	// Publish events
	for i := 0; i < 10; i++ {
		bus.Publish(NewPodEvent(PodAdded, "svc", "ns", "ctx", "pod", "svc.ns.ctx"))
	}

	// Stop and wait - should drain events
	bus.Stop()

	if atomic.LoadInt32(&count) != 10 {
		t.Errorf("Expected 10 events to be drained, got %d", count)
	}
}

// TestHandlerPanicRecovery tests that a panicking handler doesn't crash the bus
func TestHandlerPanicRecovery(t *testing.T) {
	bus := NewBus(100)

	var panicHandlerCalled int32
	bus.Subscribe(PodAdded, func(_ Event) {
		atomic.StoreInt32(&panicHandlerCalled, 1)
		panic("test panic")
	})

	var normalHandlerCalled int32
	bus.Subscribe(PodAdded, func(_ Event) {
		atomic.StoreInt32(&normalHandlerCalled, 1)
	})

	bus.Start()
	defer bus.Stop()

	bus.Publish(NewPodEvent(PodAdded, "svc", "ns", "ctx", "pod", "svc.ns.ctx"))

	time.Sleep(50 * time.Millisecond)

	if atomic.LoadInt32(&panicHandlerCalled) != 1 {
		t.Error("Expected panic handler to be called")
	}

	if atomic.LoadInt32(&normalHandlerCalled) != 1 {
		t.Error("Expected normal handler to still be called after panic")
	}
}

// TestConcurrentSubscribe tests thread safety of Subscribe
func TestConcurrentSubscribe(t *testing.T) {
	bus := NewBus(100)

	var wg sync.WaitGroup
	numGoroutines := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bus.Subscribe(PodAdded, func(_ Event) {})
		}()
	}

	wg.Wait()

	// Should have all handlers registered
	bus.mu.RLock()
	count := len(bus.handlers[PodAdded])
	bus.mu.RUnlock()

	if count != numGoroutines {
		t.Errorf("Expected %d handlers, got %d", numGoroutines, count)
	}
}

// TestConcurrentPublish tests thread safety of Publish
func TestConcurrentPublish(t *testing.T) {
	bus := NewBus(1000)

	var count int32
	bus.SubscribeAll(func(_ Event) {
		atomic.AddInt32(&count, 1)
	})

	bus.Start()
	defer bus.Stop()

	var wg sync.WaitGroup
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bus.Publish(NewPodEvent(PodAdded, "svc", "ns", "ctx", "pod", "svc.ns.ctx"))
		}()
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond)

	if atomic.LoadInt32(&count) != int32(numGoroutines) {
		t.Errorf("Expected %d events, got %d", numGoroutines, count)
	}
}

// TestEventChan tests direct access to event channel
func TestEventChan(t *testing.T) {
	bus := NewBus(100)

	ch := bus.EventChan()
	if ch == nil {
		t.Error("Expected non-nil event channel")
	}

	// Publish without starting - should be buffered
	bus.Publish(NewPodEvent(PodAdded, "svc", "ns", "ctx", "pod", "svc.ns.ctx"))

	select {
	case event := <-ch:
		if event.Type != PodAdded {
			t.Errorf("Expected PodAdded event, got %v", event.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected to receive event from channel")
	}
}

// =============================================================================
// Event Type Tests
// =============================================================================

// TestEventType_String tests string representation of event types
func TestEventType_String(t *testing.T) {
	tests := []struct {
		eventType EventType
		expected  string
	}{
		{ServiceAdded, "ServiceAdded"},
		{ServiceRemoved, "ServiceRemoved"},
		{ServiceUpdated, "ServiceUpdated"},
		{PodAdded, "PodAdded"},
		{PodRemoved, "PodRemoved"},
		{PodStatusChanged, "PodStatusChanged"},
		{BandwidthUpdate, "BandwidthUpdate"},
		{LogMessage, "LogMessage"},
		{ShutdownStarted, "ShutdownStarted"},
		{ShutdownComplete, "ShutdownComplete"},
		{EventType(999), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.eventType.String()
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

// TestNewServiceEvent tests creating service events
func TestNewServiceEvent(t *testing.T) {
	event := NewServiceEvent(ServiceAdded, "my-service", "default", "minikube")

	if event.Type != ServiceAdded {
		t.Errorf("Expected ServiceAdded type, got %v", event.Type)
	}

	if event.Service != "my-service" {
		t.Errorf("Expected service 'my-service', got %s", event.Service)
	}

	if event.Namespace != "default" {
		t.Errorf("Expected namespace 'default', got %s", event.Namespace)
	}

	if event.Context != "minikube" {
		t.Errorf("Expected context 'minikube', got %s", event.Context)
	}

	expectedKey := "my-service.default.minikube"
	if event.ServiceKey != expectedKey {
		t.Errorf("Expected service key %s, got %s", expectedKey, event.ServiceKey)
	}

	if event.Timestamp.IsZero() {
		t.Error("Expected non-zero timestamp")
	}
}

// TestNewPodEvent tests creating pod events
func TestNewPodEvent(t *testing.T) {
	event := NewPodEvent(PodAdded, "my-service", "default", "minikube", "pod-123", "my-service.default.minikube")

	if event.Type != PodAdded {
		t.Errorf("Expected PodAdded type, got %v", event.Type)
	}

	if event.Service != "my-service" {
		t.Errorf("Expected service 'my-service', got %s", event.Service)
	}

	if event.PodName != "pod-123" {
		t.Errorf("Expected pod name 'pod-123', got %s", event.PodName)
	}

	if event.PodKey != "pod-123" {
		t.Errorf("Expected pod key 'pod-123', got %s", event.PodKey)
	}

	if event.RegistryKey != "my-service.default.minikube" {
		t.Errorf("Expected registry key 'my-service.default.minikube', got %s", event.RegistryKey)
	}

	expectedKey := "my-service.default.minikube"
	if event.ServiceKey != expectedKey {
		t.Errorf("Expected service key %s, got %s", expectedKey, event.ServiceKey)
	}
}

// TestEvent_FieldsAccessible tests that all event fields are accessible
func TestEvent_FieldsAccessible(t *testing.T) {
	event := Event{
		Type:          PodStatusChanged,
		Timestamp:     time.Now(),
		ServiceKey:    "svc.ns.ctx",
		RegistryKey:   "svc.ns.ctx",
		Service:       "svc",
		Namespace:     "ns",
		Context:       "ctx",
		PodKey:        "pod",
		PodName:       "pod",
		ContainerName: "container",
		LocalIP:       "127.0.0.1",
		LocalPort:     "8080",
		PodPort:       "80",
		Hostnames:     []string{"svc", "svc.ns"},
		Status:        "active",
		Error:         nil,
		BytesIn:       100,
		BytesOut:      200,
		RateIn:        10.0,
		RateOut:       20.0,
	}

	// Just verify fields are accessible
	if event.Type != PodStatusChanged {
		t.Error("Type field not accessible")
	}

	if len(event.Hostnames) != 2 {
		t.Error("Hostnames field not accessible")
	}

	if event.BytesIn != 100 {
		t.Error("BytesIn field not accessible")
	}
}

// TestNewNamespaceRemovedEvent tests creating namespace removal events
func TestNewNamespaceRemovedEvent(t *testing.T) {
	event := NewNamespaceRemovedEvent("default", "minikube")

	if event.Type != NamespaceRemoved {
		t.Errorf("Expected type NamespaceRemoved, got %v", event.Type)
	}

	if event.Namespace != "default" {
		t.Errorf("Expected namespace 'default', got '%s'", event.Namespace)
	}

	if event.Context != "minikube" {
		t.Errorf("Expected context 'minikube', got '%s'", event.Context)
	}

	if event.Timestamp.IsZero() {
		t.Error("Expected non-zero timestamp")
	}
}

// TestNamespaceRemovedEventType_String tests the String() method for NamespaceRemoved
func TestNamespaceRemovedEventType_String(t *testing.T) {
	eventType := NamespaceRemoved

	if eventType.String() != "NamespaceRemoved" {
		t.Errorf("Expected 'NamespaceRemoved', got '%s'", eventType.String())
	}
}

// TestNamespaceRemovedEvent_Subscribe tests subscribing to NamespaceRemoved events
func TestNamespaceRemovedEvent_Subscribe(t *testing.T) {
	bus := NewBus(100)

	var received int32
	var receivedNamespace string
	var receivedContext string
	var mu sync.Mutex

	bus.Subscribe(NamespaceRemoved, func(e Event) {
		mu.Lock()
		defer mu.Unlock()
		atomic.StoreInt32(&received, 1)
		receivedNamespace = e.Namespace
		receivedContext = e.Context
	})

	bus.Start()
	defer bus.Stop()

	bus.Publish(NewNamespaceRemovedEvent("staging", "prod-cluster"))

	// Wait for event to be processed
	time.Sleep(50 * time.Millisecond)

	if atomic.LoadInt32(&received) != 1 {
		t.Error("Expected handler to be called")
	}

	mu.Lock()
	defer mu.Unlock()
	if receivedNamespace != "staging" {
		t.Errorf("Expected namespace 'staging', got '%s'", receivedNamespace)
	}
	if receivedContext != "prod-cluster" {
		t.Errorf("Expected context 'prod-cluster', got '%s'", receivedContext)
	}
}

// TestUnsubscribe tests that unsubscribing removes the handler
func TestUnsubscribe(t *testing.T) {
	bus := NewBus(100)

	var callCount int32

	unsubscribe := bus.Subscribe(PodAdded, func(_ Event) {
		atomic.AddInt32(&callCount, 1)
	})

	bus.Start()
	defer bus.Stop()

	// First event should be received
	bus.Publish(NewPodEvent(PodAdded, "svc", "ns", "ctx", "pod", "svc.ns.ctx"))
	time.Sleep(50 * time.Millisecond)

	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("Expected 1 call before unsubscribe, got %d", atomic.LoadInt32(&callCount))
	}

	// Unsubscribe
	unsubscribe()

	// Second event should NOT be received
	bus.Publish(NewPodEvent(PodAdded, "svc", "ns", "ctx", "pod2", "svc.ns.ctx"))
	time.Sleep(50 * time.Millisecond)

	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("Expected still 1 call after unsubscribe, got %d", atomic.LoadInt32(&callCount))
	}
}

// TestUnsubscribeAll tests that unsubscribing from SubscribeAll works
func TestUnsubscribeAll(t *testing.T) {
	bus := NewBus(100)

	var callCount int32

	unsubscribe := bus.SubscribeAll(func(e Event) {
		atomic.AddInt32(&callCount, 1)
	})

	bus.Start()
	defer bus.Stop()

	// First event should be received
	bus.Publish(NewPodEvent(PodAdded, "svc", "ns", "ctx", "pod", "svc.ns.ctx"))
	time.Sleep(50 * time.Millisecond)

	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("Expected 1 call before unsubscribe, got %d", atomic.LoadInt32(&callCount))
	}

	// Unsubscribe
	unsubscribe()

	// Second event should NOT be received
	bus.Publish(NewPodEvent(PodRemoved, "svc", "ns", "ctx", "pod", "svc.ns.ctx"))
	time.Sleep(50 * time.Millisecond)

	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("Expected still 1 call after unsubscribe, got %d", atomic.LoadInt32(&callCount))
	}
}

// TestMultipleUnsubscribe tests that multiple handlers can be independently unsubscribed
func TestMultipleUnsubscribe(t *testing.T) {
	bus := NewBus(100)

	var count1, count2, count3 int32

	unsub1 := bus.Subscribe(PodAdded, func(e Event) {
		atomic.AddInt32(&count1, 1)
	})
	_ = bus.Subscribe(PodAdded, func(e Event) {
		atomic.AddInt32(&count2, 1)
	})
	unsub3 := bus.Subscribe(PodAdded, func(e Event) {
		atomic.AddInt32(&count3, 1)
	})

	bus.Start()
	defer bus.Stop()

	// All three should receive first event
	bus.Publish(NewPodEvent(PodAdded, "svc", "ns", "ctx", "pod", "svc.ns.ctx"))
	time.Sleep(50 * time.Millisecond)

	if atomic.LoadInt32(&count1) != 1 || atomic.LoadInt32(&count2) != 1 || atomic.LoadInt32(&count3) != 1 {
		t.Errorf("Expected all handlers to be called once, got %d, %d, %d",
			atomic.LoadInt32(&count1), atomic.LoadInt32(&count2), atomic.LoadInt32(&count3))
	}

	// Unsubscribe first and third
	unsub1()
	unsub3()

	// Only second should receive second event
	bus.Publish(NewPodEvent(PodAdded, "svc", "ns", "ctx", "pod2", "svc.ns.ctx"))
	time.Sleep(50 * time.Millisecond)

	if atomic.LoadInt32(&count1) != 1 {
		t.Errorf("Expected handler 1 still at 1 after unsubscribe, got %d", atomic.LoadInt32(&count1))
	}
	if atomic.LoadInt32(&count2) != 2 {
		t.Errorf("Expected handler 2 at 2 (still subscribed), got %d", atomic.LoadInt32(&count2))
	}
	if atomic.LoadInt32(&count3) != 1 {
		t.Errorf("Expected handler 3 still at 1 after unsubscribe, got %d", atomic.LoadInt32(&count3))
	}
}
