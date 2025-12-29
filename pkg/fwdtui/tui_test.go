package fwdtui

import (
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/txn2/kubefwd/pkg/fwdtui/events"
	"github.com/txn2/kubefwd/pkg/fwdtui/state"
)

func TestEventBus(t *testing.T) {
	bus := events.NewBus(100)
	bus.Start()
	defer bus.Stop()

	received := make(chan bool, 1)
	bus.Subscribe(events.ServiceAdded, func(e events.Event) {
		if e.Service == "test-svc" {
			received <- true
		}
	})

	bus.Publish(events.NewServiceEvent(events.ServiceAdded, "test-svc", "default", "ctx"))

	select {
	case <-received:
		// Success
	case <-time.After(time.Second):
		t.Error("Event not received within timeout")
	}
}

func TestStateStore(t *testing.T) {
	store := state.NewStore(100)

	snapshot := state.ForwardSnapshot{
		Key:         "test-key",
		ServiceName: "test-svc",
		Namespace:   "default",
		Context:     "ctx",
		PodName:     "pod-1",
		LocalIP:     "127.1.27.1",
		LocalPort:   "8080",
		Status:      state.StatusActive,
	}

	store.AddForward(snapshot)

	forwards := store.GetFiltered()
	if len(forwards) != 1 {
		t.Errorf("Expected 1 forward, got %d", len(forwards))
	}

	if forwards[0].ServiceName != "test-svc" {
		t.Errorf("Expected service name 'test-svc', got '%s'", forwards[0].ServiceName)
	}

	// Test filter
	store.SetFilter("test")
	filtered := store.GetFiltered()
	if len(filtered) != 1 {
		t.Errorf("Expected 1 filtered forward, got %d", len(filtered))
	}

	store.SetFilter("nomatch")
	filtered = store.GetFiltered()
	if len(filtered) != 0 {
		t.Errorf("Expected 0 filtered forwards, got %d", len(filtered))
	}

	// Test remove
	store.SetFilter("")
	store.RemoveForward("test-key")
	forwards = store.GetFiltered()
	if len(forwards) != 0 {
		t.Errorf("Expected 0 forwards after remove, got %d", len(forwards))
	}
}

func TestEnableDisable(t *testing.T) {
	// Reset state for test
	mu.Lock()
	tuiEnabled = false
	mu.Unlock()

	if IsEnabled() {
		t.Error("Expected TUI to be disabled initially")
	}

	Enable()

	if !IsEnabled() {
		t.Error("Expected TUI to be enabled after Enable()")
	}
}

// TestGetEventBus tests getting the event bus
func TestGetEventBus(t *testing.T) {
	// Reset state
	mu.Lock()
	oldManager := tuiManager
	tuiManager = nil
	mu.Unlock()
	defer func() {
		mu.Lock()
		tuiManager = oldManager
		mu.Unlock()
	}()

	// Should return nil when not initialized
	if GetEventBus() != nil {
		t.Error("Expected nil event bus when not initialized")
	}
}

// TestGetStore tests getting the state store
func TestGetStore(t *testing.T) {
	// Reset state
	mu.Lock()
	oldManager := tuiManager
	tuiManager = nil
	mu.Unlock()
	defer func() {
		mu.Lock()
		tuiManager = oldManager
		mu.Unlock()
	}()

	// Should return nil when not initialized
	if GetStore() != nil {
		t.Error("Expected nil store when not initialized")
	}
}

// TestEmit tests emitting events
func TestEmit(t *testing.T) {
	// Reset state
	mu.Lock()
	oldManager := tuiManager
	tuiManager = nil
	mu.Unlock()
	defer func() {
		mu.Lock()
		tuiManager = oldManager
		mu.Unlock()
	}()

	// Should not panic when not initialized
	Emit(events.NewServiceEvent(events.ServiceAdded, "test", "default", "ctx"))
}

// TestFocusType tests the Focus type constants
func TestFocusType(t *testing.T) {
	// Test Focus values are distinct
	if FocusServices == FocusLogs {
		t.Error("FocusServices should not equal FocusLogs")
	}
	if FocusLogs == FocusDetail {
		t.Error("FocusLogs should not equal FocusDetail")
	}
	if FocusServices == FocusDetail {
		t.Error("FocusServices should not equal FocusDetail")
	}

	// Test they have expected values
	if FocusServices != 0 {
		t.Errorf("Expected FocusServices=0, got %d", FocusServices)
	}
	if FocusLogs != 1 {
		t.Errorf("Expected FocusLogs=1, got %d", FocusLogs)
	}
	if FocusDetail != 2 {
		t.Errorf("Expected FocusDetail=2, got %d", FocusDetail)
	}
}

// TestLogEntryMsg tests the LogEntryMsg type
func TestLogEntryMsg(t *testing.T) {
	now := time.Now()
	msg := LogEntryMsg{
		Level:   log.InfoLevel,
		Message: "test message",
		Time:    now,
	}

	if msg.Level != log.InfoLevel {
		t.Errorf("Expected level InfoLevel, got '%v'", msg.Level)
	}
	if msg.Message != "test message" {
		t.Errorf("Expected message 'test message', got '%s'", msg.Message)
	}
	if msg.Time != now {
		t.Error("Time mismatch")
	}
}

// TestMetricsUpdateMsg tests the MetricsUpdateMsg type
func TestMetricsUpdateMsg(t *testing.T) {
	msg := MetricsUpdateMsg{
		Snapshots: nil,
	}

	if msg.Snapshots != nil {
		t.Error("Expected nil snapshots")
	}
}

// TestKubefwdEventMsg tests the KubefwdEventMsg type
func TestKubefwdEventMsg(t *testing.T) {
	event := events.NewServiceEvent(events.ServiceAdded, "svc", "ns", "ctx")
	msg := KubefwdEventMsg{
		Event: event,
	}

	if msg.Event.Type != events.ServiceAdded {
		t.Errorf("Expected ServiceAdded event type, got %v", msg.Event.Type)
	}
	if msg.Event.Service != "svc" {
		t.Errorf("Expected service 'svc', got '%s'", msg.Event.Service)
	}
}

// TestShutdownMsg tests the ShutdownMsg type
func TestShutdownMsg(t *testing.T) {
	msg := ShutdownMsg{}
	_ = msg // Just verify the type exists
}

// TestRefreshMsg tests the RefreshMsg type
func TestRefreshMsg(t *testing.T) {
	msg := RefreshMsg{}
	_ = msg // Just verify the type exists
}

// TestListenEvents tests the event listening command
func TestListenEvents(t *testing.T) {
	ch := make(chan events.Event, 1)
	event := events.NewServiceEvent(events.ServiceAdded, "test", "default", "ctx")
	ch <- event

	cmd := ListenEvents(ch)
	msg := cmd()

	kubefwdMsg, ok := msg.(KubefwdEventMsg)
	if !ok {
		t.Errorf("Expected KubefwdEventMsg, got %T", msg)
	}

	if kubefwdMsg.Event.Service != "test" {
		t.Errorf("Expected service 'test', got '%s'", kubefwdMsg.Event.Service)
	}
}

// TestListenLogs tests the log listening command
func TestListenLogs(t *testing.T) {
	ch := make(chan LogEntryMsg, 1)
	now := time.Now()
	logEntry := LogEntryMsg{
		Level:   log.InfoLevel,
		Message: "test",
		Time:    now,
	}
	ch <- logEntry

	cmd := ListenLogs(ch)
	msg := cmd()

	logMsg, ok := msg.(LogEntryMsg)
	if !ok {
		t.Errorf("Expected LogEntryMsg, got %T", msg)
	}

	if logMsg.Message != "test" {
		t.Errorf("Expected message 'test', got '%s'", logMsg.Message)
	}
}

// TestListenShutdown tests the shutdown listening command
func TestListenShutdown(t *testing.T) {
	ch := make(chan struct{})
	close(ch) // Simulate shutdown signal

	cmd := ListenShutdown(ch)
	msg := cmd()

	_, ok := msg.(ShutdownMsg)
	if !ok {
		t.Errorf("Expected ShutdownMsg, got %T", msg)
	}
}

// TestStateStoreSummary tests getting summary stats
func TestStateStoreSummary(t *testing.T) {
	store := state.NewStore(100)

	// Add some forwards
	store.AddForward(state.ForwardSnapshot{
		Key:         "key1",
		ServiceKey:  "svc1.ns.ctx",
		ServiceName: "svc1",
		Namespace:   "ns",
		Status:      state.StatusActive,
	})

	store.AddForward(state.ForwardSnapshot{
		Key:         "key2",
		ServiceKey:  "svc2.ns.ctx",
		ServiceName: "svc2",
		Namespace:   "ns",
		Status:      state.StatusError,
		Error:       "connection failed",
	})

	summary := store.GetSummary()

	if summary.TotalForwards != 2 {
		t.Errorf("Expected 2 total forwards, got %d", summary.TotalForwards)
	}

	if summary.ActiveForwards != 1 {
		t.Errorf("Expected 1 active forward, got %d", summary.ActiveForwards)
	}

	if summary.ErrorCount != 1 {
		t.Errorf("Expected 1 error, got %d", summary.ErrorCount)
	}
}

// TestSendLog tests the SendLog command
func TestSendLog(t *testing.T) {
	cmd := SendLog(log.InfoLevel, "test message")
	msg := cmd()

	logMsg, ok := msg.(LogEntryMsg)
	if !ok {
		t.Errorf("Expected LogEntryMsg, got %T", msg)
	}

	if logMsg.Message != "test message" {
		t.Errorf("Expected message 'test message', got '%s'", logMsg.Message)
	}

	if logMsg.Level != log.InfoLevel {
		t.Errorf("Expected InfoLevel, got %v", logMsg.Level)
	}
}

// TestConcurrentEnableDisable tests thread safety of Enable/IsEnabled
func TestConcurrentEnableDisable(t *testing.T) {
	// Reset state
	mu.Lock()
	tuiEnabled = false
	mu.Unlock()

	done := make(chan bool, 100)

	// Concurrent enables
	for i := 0; i < 50; i++ {
		go func() {
			Enable()
			done <- true
		}()
	}

	// Concurrent reads
	for i := 0; i < 50; i++ {
		go func() {
			_ = IsEnabled()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}

	// Should be enabled after all enables
	if !IsEnabled() {
		t.Error("Expected TUI to be enabled after concurrent enables")
	}
}

// TestHandleEventForStore_NamespaceRemoved tests that NamespaceRemoved events clean up the store
func TestHandleEventForStore_NamespaceRemoved(t *testing.T) {
	store := state.NewStore(100)

	// Add forwards from different namespaces
	store.AddForward(state.ForwardSnapshot{
		Key:         "svc1.ns1.ctx1.pod1.8080",
		ServiceKey:  "svc1.ns1.ctx1",
		ServiceName: "svc1",
		Namespace:   "ns1",
		Context:     "ctx1",
		PodName:     "pod1",
		Status:      state.StatusActive,
	})
	store.AddForward(state.ForwardSnapshot{
		Key:         "svc2.ns1.ctx1.pod2.8080",
		ServiceKey:  "svc2.ns1.ctx1",
		ServiceName: "svc2",
		Namespace:   "ns1",
		Context:     "ctx1",
		PodName:     "pod2",
		Status:      state.StatusActive,
	})
	store.AddForward(state.ForwardSnapshot{
		Key:         "svc3.ns2.ctx1.pod3.8080",
		ServiceKey:  "svc3.ns2.ctx1",
		ServiceName: "svc3",
		Namespace:   "ns2",
		Context:     "ctx1",
		PodName:     "pod3",
		Status:      state.StatusActive,
	})

	// Verify initial state
	if store.Count() != 3 {
		t.Fatalf("Expected 3 forwards, got %d", store.Count())
	}

	// Handle NamespaceRemoved event for ns1.ctx1
	event := events.NewNamespaceRemovedEvent("ns1", "ctx1")
	handleEventForStore(store, event)

	// Should have removed 2 forwards (ns1.ctx1)
	if store.Count() != 1 {
		t.Errorf("Expected 1 forward remaining, got %d", store.Count())
	}

	// Verify the correct service remains
	if store.GetService("svc3.ns2.ctx1") == nil {
		t.Error("svc3.ns2.ctx1 should NOT have been removed")
	}
	if store.GetService("svc1.ns1.ctx1") != nil {
		t.Error("svc1.ns1.ctx1 should have been removed")
	}
}
