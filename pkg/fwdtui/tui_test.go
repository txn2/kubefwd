package fwdtui

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	log "github.com/sirupsen/logrus"
	"github.com/txn2/kubefwd/pkg/fwdapi/types"
	"github.com/txn2/kubefwd/pkg/fwdmetrics"
	"github.com/txn2/kubefwd/pkg/fwdtui/components"
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

// =============================================================================
// RootModel Tests
// =============================================================================

// createTestRootModel creates a RootModel for testing
func createTestRootModel() *RootModel {
	store := state.NewStore(100)
	eventBus := events.NewBus(100)
	eventBus.Start()

	eventCh := make(chan events.Event, 10)
	metricsCh := make(chan []fwdmetrics.ServiceSnapshot, 10)
	logCh := make(chan LogEntryMsg, 10)
	stopCh := make(chan struct{})

	m := &RootModel{
		header:      components.NewHeaderModel("test"),
		services:    components.NewServicesModel(store),
		logs:        components.NewLogsModel(),
		statusBar:   components.NewStatusBarModel(),
		help:        components.NewHelpModel(),
		detail:      components.NewDetailModel(store, state.NewRateHistory(60)),
		browse:      components.NewBrowseModel(),
		store:       store,
		eventBus:    eventBus,
		rateHistory: state.NewRateHistory(60),
		focus:       FocusServices,
		width:       100,
		height:      40,
		eventCh:     eventCh,
		metricsCh:   metricsCh,
		logCh:       logCh,
		stopCh:      stopCh,
	}

	m.updateSizes()
	return m
}

func TestRootModel_CycleFocus(t *testing.T) {
	m := createTestRootModel()

	// Initial focus is Services
	if m.focus != FocusServices {
		t.Errorf("Expected initial focus FocusServices, got %d", m.focus)
	}

	// Cycle to Logs
	m.cycleFocus()
	if m.focus != FocusLogs {
		t.Errorf("Expected focus FocusLogs after first cycle, got %d", m.focus)
	}

	// Verify services lost focus, logs gained focus
	// Note: We can't directly check component focus, but we can verify the model focus changed

	// Cycle back to Services
	m.cycleFocus()
	if m.focus != FocusServices {
		t.Errorf("Expected focus FocusServices after second cycle, got %d", m.focus)
	}
}

func TestRootModel_UpdateSizes(t *testing.T) {
	m := createTestRootModel()

	m.width = 120
	m.height = 50
	m.updateSizes()

	// Verify heights are calculated (non-zero and within bounds)
	if m.servicesHeight <= 0 {
		t.Error("Expected positive servicesHeight")
	}
	if m.logsHeight <= 0 {
		t.Error("Expected positive logsHeight")
	}
	if m.servicesHeight+m.logsHeight > m.height {
		t.Error("Combined heights exceed terminal height")
	}
}

func TestRootModel_UpdateSizes_SmallTerminal(t *testing.T) {
	m := createTestRootModel()

	m.width = 40
	m.height = 15
	m.updateSizes()

	// Should still have minimum sizes
	if m.servicesHeight < 3 {
		t.Errorf("Expected minimum servicesHeight of 3, got %d", m.servicesHeight)
	}
	if m.logsHeight < 3 {
		t.Errorf("Expected minimum logsHeight of 3, got %d", m.logsHeight)
	}
}

func TestRootModel_HandleKubefwdEvent_PodAdded(t *testing.T) {
	m := createTestRootModel()

	// NewPodEvent signature: (eventType, service, namespace, context, podName, registryKey)
	event := events.NewPodEvent(
		events.PodAdded,
		"myservice",
		"default",
		"mycontext",
		"mypod",
		"myservice.default",
	)
	event.LocalIP = "127.1.0.1"
	event.LocalPort = "8080"
	event.PodPort = "80"
	event.Hostnames = []string{"myservice", "myservice.default"}

	m.handleKubefwdEvent(event)

	// Check that forward was added to store
	forwards := m.store.GetFiltered()
	if len(forwards) != 1 {
		t.Fatalf("Expected 1 forward, got %d", len(forwards))
	}

	fwd := forwards[0]
	if fwd.ServiceName != "myservice" {
		t.Errorf("Expected service name 'myservice', got '%s'", fwd.ServiceName)
	}
	if fwd.PodName != "mypod" {
		t.Errorf("Expected pod name 'mypod', got '%s'", fwd.PodName)
	}
	if fwd.Status != state.StatusConnecting {
		t.Errorf("Expected status Connecting, got %v", fwd.Status)
	}
}

func TestRootModel_HandleKubefwdEvent_PodRemoved(t *testing.T) {
	m := createTestRootModel()

	// First add a pod
	addEvent := events.NewPodEvent(
		events.PodAdded,
		"myservice",
		"default",
		"mycontext",
		"mypod",
		"myservice.default",
	)
	addEvent.LocalPort = "8080"
	m.handleKubefwdEvent(addEvent)

	// Verify it was added
	if m.store.Count() != 1 {
		t.Fatalf("Expected 1 forward after add, got %d", m.store.Count())
	}

	// Now remove it
	removeEvent := events.NewPodEvent(
		events.PodRemoved,
		"myservice",
		"default",
		"mycontext",
		"mypod",
		"myservice.default",
	)
	removeEvent.LocalPort = "8080"
	m.handleKubefwdEvent(removeEvent)

	// Verify it was removed
	if m.store.Count() != 0 {
		t.Errorf("Expected 0 forwards after remove, got %d", m.store.Count())
	}
}

func TestRootModel_HandleKubefwdEvent_PodStatusChanged(t *testing.T) {
	m := createTestRootModel()

	// First add a pod
	addEvent := events.NewPodEvent(
		events.PodAdded,
		"myservice",
		"default",
		"mycontext",
		"mypod",
		"myservice.default",
	)
	addEvent.LocalPort = "8080"
	m.handleKubefwdEvent(addEvent)

	// Change status to active
	statusEvent := events.Event{
		Type:       events.PodStatusChanged,
		Service:    "myservice",
		PodName:    "mypod",
		Namespace:  "default",
		Context:    "mycontext",
		LocalPort:  "8080",
		ServiceKey: "myservice.default.mycontext",
		Status:     "active",
		Hostnames:  []string{"myservice", "myservice.default"},
	}
	m.handleKubefwdEvent(statusEvent)

	forwards := m.store.GetFiltered()
	if len(forwards) != 1 {
		t.Fatalf("Expected 1 forward, got %d", len(forwards))
	}
	if forwards[0].Status != state.StatusActive {
		t.Errorf("Expected status Active, got %v", forwards[0].Status)
	}
}

func TestRootModel_HandleKubefwdEvent_StatusError(t *testing.T) {
	m := createTestRootModel()

	// First add a pod
	addEvent := events.NewPodEvent(
		events.PodAdded,
		"myservice",
		"default",
		"mycontext",
		"mypod",
		"myservice.default",
	)
	addEvent.LocalPort = "8080"
	m.handleKubefwdEvent(addEvent)

	// Change status to error
	statusEvent := events.Event{
		Type:       events.PodStatusChanged,
		Service:    "myservice",
		PodName:    "mypod",
		Namespace:  "default",
		Context:    "mycontext",
		LocalPort:  "8080",
		ServiceKey: "myservice.default.mycontext",
		Status:     "error",
		Error:      fmt.Errorf("connection refused"),
	}
	m.handleKubefwdEvent(statusEvent)

	forwards := m.store.GetFiltered()
	if len(forwards) != 1 {
		t.Fatalf("Expected 1 forward, got %d", len(forwards))
	}
	if forwards[0].Status != state.StatusError {
		t.Errorf("Expected status Error, got %v", forwards[0].Status)
	}
	if forwards[0].Error != "connection refused" {
		t.Errorf("Expected error message 'connection refused', got '%s'", forwards[0].Error)
	}
}

func TestRootModel_HandleKubefwdEvent_ServiceRemoved(t *testing.T) {
	m := createTestRootModel()

	// Add multiple pods for same service
	for i := 1; i <= 3; i++ {
		addEvent := events.NewPodEvent(
			events.PodAdded,
			"myservice",
			"default",
			"mycontext",
			fmt.Sprintf("pod%d", i),
			"myservice.default",
		)
		addEvent.LocalPort = "8080"
		m.handleKubefwdEvent(addEvent)
	}

	// Add a pod for a different service
	addEvent := events.NewPodEvent(
		events.PodAdded,
		"otherservice",
		"default",
		"mycontext",
		"otherpod",
		"otherservice.default",
	)
	addEvent.LocalPort = "9090"
	m.handleKubefwdEvent(addEvent)

	// Verify initial count
	if m.store.Count() != 4 {
		t.Fatalf("Expected 4 forwards, got %d", m.store.Count())
	}

	// Remove the first service
	removeEvent := events.Event{
		Type:       events.ServiceRemoved,
		Service:    "myservice",
		Namespace:  "default",
		Context:    "mycontext",
		ServiceKey: "myservice.default.mycontext",
	}
	m.handleKubefwdEvent(removeEvent)

	// Should have only the other service remaining
	if m.store.Count() != 1 {
		t.Errorf("Expected 1 forward after service removal, got %d", m.store.Count())
	}
}

func TestRootModel_WaitForLogLine_NilChannel(t *testing.T) {
	m := createTestRootModel()
	m.logStreamCh = nil

	cmd := m.waitForLogLine()
	if cmd != nil {
		t.Error("Expected nil command when logStreamCh is nil")
	}
}

func TestRootModel_WaitForLogLine_ReceiveLine(t *testing.T) {
	m := createTestRootModel()
	ch := make(chan string, 1)
	m.logStreamCh = ch

	ch <- "test log line"

	cmd := m.waitForLogLine()
	if cmd == nil {
		t.Fatal("Expected non-nil command")
	}

	msg := cmd()
	lineMsg, ok := msg.(components.PodLogLineMsg)
	if !ok {
		t.Fatalf("Expected PodLogLineMsg, got %T", msg)
	}
	if lineMsg.Line != "test log line" {
		t.Errorf("Expected 'test log line', got '%s'", lineMsg.Line)
	}
}

func TestRootModel_WaitForLogLine_ChannelClosed(t *testing.T) {
	m := createTestRootModel()
	ch := make(chan string)
	m.logStreamCh = ch
	close(ch)

	cmd := m.waitForLogLine()
	if cmd == nil {
		t.Fatal("Expected non-nil command")
	}

	msg := cmd()
	_, ok := msg.(components.PodLogsStopMsg)
	if !ok {
		t.Errorf("Expected PodLogsStopMsg when channel closed, got %T", msg)
	}
}

func TestRootModel_View_Quitting(t *testing.T) {
	m := createTestRootModel()
	m.quitting = true

	view := m.View()
	if view != "Shutting down...\n" {
		t.Errorf("Expected 'Shutting down...' view when quitting, got '%s'", view)
	}
}

func TestRootModel_View_HelpVisible(t *testing.T) {
	m := createTestRootModel()
	m.help.Show()

	view := m.View()
	// Help view should contain keyboard shortcuts
	if !strings.Contains(view, "help") && !strings.Contains(view, "Help") {
		t.Error("Expected help content in view when help is visible")
	}
}

func TestRootModel_View_Normal(t *testing.T) {
	m := createTestRootModel()

	view := m.View()

	// Should contain section titles
	if !strings.Contains(view, "Services") {
		t.Error("Expected 'Services' in normal view")
	}
	if !strings.Contains(view, "Logs") {
		t.Error("Expected 'Logs' in normal view")
	}
}

// TestListenMetrics tests the metrics listening command
func TestListenMetrics(t *testing.T) {
	ch := make(chan []fwdmetrics.ServiceSnapshot, 1)
	snapshots := []fwdmetrics.ServiceSnapshot{
		{ServiceName: "svc1", Namespace: "ns", Context: "ctx", TotalBytesIn: 100, TotalBytesOut: 200},
	}
	ch <- snapshots

	cmd := ListenMetrics(ch)
	msg := cmd()

	metricsMsg, ok := msg.(MetricsUpdateMsg)
	if !ok {
		t.Errorf("Expected MetricsUpdateMsg, got %T", msg)
	}

	if len(metricsMsg.Snapshots) != 1 {
		t.Errorf("Expected 1 snapshot, got %d", len(metricsMsg.Snapshots))
	}
	if metricsMsg.Snapshots[0].TotalBytesIn != 100 {
		t.Errorf("Expected TotalBytesIn 100, got %d", metricsMsg.Snapshots[0].TotalBytesIn)
	}
}

// TestListenMetrics_ClosedChannel tests behavior when channel is closed
func TestListenMetrics_ClosedChannel(t *testing.T) {
	ch := make(chan []fwdmetrics.ServiceSnapshot)
	close(ch)

	cmd := ListenMetrics(ch)
	msg := cmd()

	// Should return nil when channel is closed
	if msg != nil {
		t.Errorf("Expected nil when channel closed, got %T", msg)
	}
}

// =============================================================================
// Additional Event Handler Tests
// =============================================================================

func TestHandleEventForStore_PodAdded(t *testing.T) {
	store := state.NewStore(100)

	// NewPodEvent signature: (eventType, service, namespace, context, podName, registryKey)
	event := events.NewPodEvent(
		events.PodAdded,
		"svc",
		"ns",
		"ctx",
		"pod1",
		"svc.ns",
	)
	event.LocalIP = "127.1.0.1"
	event.LocalPort = "8080"
	event.PodPort = "80"

	handleEventForStore(store, event)

	if store.Count() != 1 {
		t.Errorf("Expected 1 forward, got %d", store.Count())
	}
}

func TestHandleEventForStore_PodRemoved(t *testing.T) {
	store := state.NewStore(100)

	// Add first
	addEvent := events.NewPodEvent(events.PodAdded, "svc", "ns", "ctx", "pod1", "svc.ns")
	addEvent.LocalPort = "8080"
	handleEventForStore(store, addEvent)

	// Remove
	removeEvent := events.NewPodEvent(events.PodRemoved, "svc", "ns", "ctx", "pod1", "svc.ns")
	removeEvent.LocalPort = "8080"
	handleEventForStore(store, removeEvent)

	if store.Count() != 0 {
		t.Errorf("Expected 0 forwards, got %d", store.Count())
	}
}

func TestHandleEventForStore_PodStatusChanged(t *testing.T) {
	store := state.NewStore(100)

	// Add first
	addEvent := events.NewPodEvent(events.PodAdded, "svc", "ns", "ctx", "pod1", "svc.ns")
	addEvent.LocalPort = "8080"
	handleEventForStore(store, addEvent)

	// Change status
	statusEvent := events.Event{
		Type:       events.PodStatusChanged,
		Service:    "svc",
		PodName:    "pod1",
		Namespace:  "ns",
		Context:    "ctx",
		LocalPort:  "8080",
		ServiceKey: "svc.ns.ctx",
		Status:     "active",
	}
	handleEventForStore(store, statusEvent)

	forwards := store.GetFiltered()
	if len(forwards) != 1 {
		t.Fatalf("Expected 1 forward, got %d", len(forwards))
	}
	if forwards[0].Status != state.StatusActive {
		t.Errorf("Expected StatusActive, got %v", forwards[0].Status)
	}
}

// =============================================================================
// TUI Manager Configuration Tests
// =============================================================================

func TestEventsEnabled(t *testing.T) {
	// Before enabling TUI with events, EventsEnabled should return false
	// Just test that the function doesn't panic
	result := EventsEnabled()
	// The result depends on global state - just verify it returns a bool
	_ = result
}

func TestManager_SetHeaderContext(t *testing.T) {
	m := createTestRootModel()
	manager := &Manager{
		model: m,
	}

	manager.SetHeaderContext("test-context")

	// Verify the context was set in header
	if m.header.GetContext() != "test-context" {
		t.Errorf("Expected context 'test-context', got '%s'", m.header.GetContext())
	}
}

// =============================================================================
// Detail Model Tests
// =============================================================================

func TestDetailModel_SetHTTPLogs(t *testing.T) {
	store := state.NewStore(100)
	rateHistory := state.NewRateHistory(60)
	detail := components.NewDetailModel(store, rateHistory)

	logs := []components.HTTPLogEntry{
		{Method: "GET", Path: "/api", StatusCode: 200},
		{Method: "POST", Path: "/api", StatusCode: 201},
	}

	detail.SetHTTPLogs(logs)

	// The logs should be set (we can't directly verify, but the call should not panic)
}

func TestDetailModel_SetPodLogs(t *testing.T) {
	store := state.NewStore(100)
	rateHistory := state.NewRateHistory(60)
	detail := components.NewDetailModel(store, rateHistory)

	logs := []string{"log line 1", "log line 2", "log line 3"}

	detail.SetPodLogs(logs)

	// The logs should be set
}

func TestDetailModel_SetLogsLoading(t *testing.T) {
	store := state.NewStore(100)
	rateHistory := state.NewRateHistory(60)
	detail := components.NewDetailModel(store, rateHistory)

	detail.SetLogsLoading(true)

	// Loading state should be set
}

// =============================================================================
// Header Model Tests
// =============================================================================

func TestHeaderModel_SetContext(t *testing.T) {
	header := components.NewHeaderModel("1.0.0")

	header.SetContext("my-cluster")

	if header.GetContext() != "my-cluster" {
		t.Errorf("Expected context 'my-cluster', got '%s'", header.GetContext())
	}
}

func TestHeaderModel_GetContext_Empty(t *testing.T) {
	header := components.NewHeaderModel("1.0.0")

	if header.GetContext() != "" {
		t.Errorf("Expected empty context, got '%s'", header.GetContext())
	}
}

// =============================================================================
// Services Model Tests
// =============================================================================

func TestServicesModel_GetSelectedRegistryKey(t *testing.T) {
	store := state.NewStore(100)
	model := components.NewServicesModel(store)

	// Add a forward with registry key
	store.AddForward(state.ForwardSnapshot{
		Key:         "svc.ns.ctx.pod.8080",
		ServiceKey:  "svc.ns.ctx",
		RegistryKey: "svc.ns",
		ServiceName: "svc",
		Namespace:   "ns",
		Context:     "ctx",
		Status:      state.StatusActive,
	})

	// Refresh the model
	model.Refresh()

	// Get registry key should work - we just verify it doesn't panic
	_ = model.GetSelectedRegistryKey()
}

// =============================================================================
// RootModel Update() Message Handler Tests
// =============================================================================

func TestRootModel_Update_WindowSizeMsg(t *testing.T) {
	m := createTestRootModel()

	msg := tea.WindowSizeMsg{Width: 150, Height: 60}
	model, _ := m.Update(msg)
	result := model.(*RootModel)

	if result.width != 150 {
		t.Errorf("Expected width 150, got %d", result.width)
	}
	if result.height != 60 {
		t.Errorf("Expected height 60, got %d", result.height)
	}
	if result.servicesHeight <= 0 {
		t.Error("Expected positive servicesHeight after resize")
	}
	if result.logsHeight <= 0 {
		t.Error("Expected positive logsHeight after resize")
	}
}

func TestRootModel_Update_KeyMsg_CtrlC(t *testing.T) {
	m := createTestRootModel()

	msg := tea.KeyMsg{Type: tea.KeyCtrlC}
	model, cmd := m.Update(msg)
	result := model.(*RootModel)

	if !result.quitting {
		t.Error("Expected quitting to be true after ctrl+c")
	}
	if cmd == nil {
		t.Error("Expected tea.Quit command after ctrl+c")
	}
}

func TestRootModel_Update_KeyMsg_Q(t *testing.T) {
	m := createTestRootModel()

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	model, cmd := m.Update(msg)
	result := model.(*RootModel)

	if !result.quitting {
		t.Error("Expected quitting to be true after q")
	}
	if cmd == nil {
		t.Error("Expected tea.Quit command after q")
	}
}

func TestRootModel_Update_KeyMsg_Q_NotWhileFiltering(t *testing.T) {
	m := createTestRootModel()

	// Start filtering mode
	slashMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}
	m.Update(slashMsg)

	// Now try q - should NOT quit because we're filtering
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	model, _ := m.Update(msg)
	result := model.(*RootModel)

	if result.quitting {
		t.Error("Should not quit when filtering mode is active")
	}
}

func TestRootModel_Update_KeyMsg_Question(t *testing.T) {
	m := createTestRootModel()

	if m.help.IsVisible() {
		t.Error("Help should be hidden initially")
	}

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}}
	m.Update(msg)

	if !m.help.IsVisible() {
		t.Error("Help should be visible after pressing ?")
	}
}

func TestRootModel_Update_KeyMsg_Tab(t *testing.T) {
	m := createTestRootModel()

	if m.focus != FocusServices {
		t.Error("Initial focus should be FocusServices")
	}

	msg := tea.KeyMsg{Type: tea.KeyTab}
	m.Update(msg)

	if m.focus != FocusLogs {
		t.Errorf("Expected focus FocusLogs after tab, got %d", m.focus)
	}

	// Tab again
	m.Update(msg)
	if m.focus != FocusServices {
		t.Errorf("Expected focus FocusServices after second tab, got %d", m.focus)
	}
}

func TestRootModel_Update_KeyMsg_R_ReconnectErrored(t *testing.T) {
	m := createTestRootModel()

	reconnectCalled := false
	m.reconnectErrored = func() int {
		reconnectCalled = true
		return 3
	}

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}
	_, cmd := m.Update(msg)

	if !reconnectCalled {
		t.Error("Expected reconnectErrored to be called")
	}
	if cmd == nil {
		t.Error("Expected log command after reconnect")
	}
}

func TestRootModel_Update_KeyMsg_R_NoErrors(t *testing.T) {
	m := createTestRootModel()

	m.reconnectErrored = func() int {
		return 0 // No errors to reconnect
	}

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}
	_, cmd := m.Update(msg)

	if cmd == nil {
		t.Error("Expected log command even when no errors")
	}
}

func TestRootModel_Update_KeyMsg_F_ShowBrowse(t *testing.T) {
	m := createTestRootModel()

	if m.browse.IsVisible() {
		t.Error("Browse should be hidden initially")
	}

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}}
	m.Update(msg)

	if !m.browse.IsVisible() {
		t.Error("Browse should be visible after pressing f")
	}
}

func TestRootModel_Update_KeyMsg_Enter_OpenDetail(t *testing.T) {
	m := createTestRootModel()

	// Add a service so there's something to select
	m.store.AddForward(state.ForwardSnapshot{
		Key:         "svc.ns.ctx.pod.8080",
		ServiceKey:  "svc.ns.ctx",
		ServiceName: "svc",
		Namespace:   "ns",
		Context:     "ctx",
		PodName:     "pod",
		Status:      state.StatusActive,
	})
	m.services.Refresh()

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	m.Update(msg)

	if m.focus != FocusDetail {
		t.Errorf("Expected focus to be FocusDetail after Enter, got %d", m.focus)
	}
}

func TestRootModel_Update_HelpCapturesInput(t *testing.T) {
	m := createTestRootModel()
	m.help.Show()

	// When help is visible, any key should be handled by help
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	model, _ := m.Update(msg)
	result := model.(*RootModel)

	// q should not quit when help is visible (help closes first)
	if result.quitting {
		t.Error("Should not quit when help is visible")
	}
}

func TestRootModel_Update_DetailCapturesInput(t *testing.T) {
	m := createTestRootModel()

	// Add a service and open detail
	m.store.AddForward(state.ForwardSnapshot{
		Key:         "svc.ns.ctx.pod.8080",
		ServiceKey:  "svc.ns.ctx",
		ServiceName: "svc",
		Namespace:   "ns",
		Context:     "ctx",
		PodName:     "pod",
		Status:      state.StatusActive,
	})
	m.detail.Show("svc.ns.ctx.pod.8080")

	// Esc should close detail
	msg := tea.KeyMsg{Type: tea.KeyEsc}
	m.Update(msg)

	if m.detail.IsVisible() {
		t.Error("Detail should be hidden after Esc")
	}
	if m.focus != FocusServices {
		t.Error("Focus should return to services after closing detail")
	}
}

func TestRootModel_Update_MouseWheelServices(t *testing.T) {
	m := createTestRootModel()
	m.focus = FocusServices

	msg := tea.MouseMsg{Button: tea.MouseButtonWheelDown}
	model, _ := m.Update(msg)

	// Should not panic and model should be returned
	if model == nil {
		t.Error("Expected non-nil model after mouse wheel")
	}
}

func TestRootModel_Update_MouseWheelLogs(t *testing.T) {
	m := createTestRootModel()
	m.focus = FocusLogs

	msg := tea.MouseMsg{Button: tea.MouseButtonWheelUp}
	model, _ := m.Update(msg)

	if model == nil {
		t.Error("Expected non-nil model after mouse wheel")
	}
}

func TestRootModel_Update_MouseClickServices(t *testing.T) {
	m := createTestRootModel()
	m.focus = FocusLogs // Start with logs focus

	// Click in services area
	msg := tea.MouseMsg{
		Button: tea.MouseButtonLeft,
		Y:      m.servicesStartY + 1,
	}
	m.Update(msg)

	if m.focus != FocusServices {
		t.Error("Focus should switch to services after clicking in services area")
	}
}

func TestRootModel_Update_MouseClickLogs(t *testing.T) {
	m := createTestRootModel()
	m.focus = FocusServices // Start with services focus

	// Click in logs area
	msg := tea.MouseMsg{
		Button: tea.MouseButtonLeft,
		Y:      m.logsStartY + 1,
	}
	m.Update(msg)

	if m.focus != FocusLogs {
		t.Error("Focus should switch to logs after clicking in logs area")
	}
}

func TestRootModel_Update_MetricsUpdateMsg(t *testing.T) {
	m := createTestRootModel()

	// Add a forward first
	m.store.AddForward(state.ForwardSnapshot{
		Key:         "svc.ns.ctx.pod.8080",
		ServiceKey:  "svc.ns.ctx",
		ServiceName: "svc",
		Namespace:   "ns",
		Context:     "ctx",
		PodName:     "pod",
		LocalPort:   "8080",
		Status:      state.StatusActive,
	})

	msg := MetricsUpdateMsg{
		Snapshots: []fwdmetrics.ServiceSnapshot{
			{
				ServiceName: "svc",
				Namespace:   "ns",
				Context:     "ctx",
				PortForwards: []fwdmetrics.PortForwardSnapshot{
					{
						PodName:   "pod",
						LocalPort: "8080",
						BytesIn:   1024,
						BytesOut:  2048,
						RateIn:    100.0,
						RateOut:   200.0,
					},
				},
			},
		},
	}
	model, cmd := m.Update(msg)

	if model == nil {
		t.Error("Expected non-nil model after metrics update")
	}
	if cmd == nil {
		t.Error("Expected non-nil command (resubscribe) after metrics update")
	}

	// Verify metrics were stored
	forwards := m.store.GetFiltered()
	if len(forwards) != 1 {
		t.Fatalf("Expected 1 forward, got %d", len(forwards))
	}
	if forwards[0].BytesIn != 1024 {
		t.Errorf("Expected BytesIn 1024, got %d", forwards[0].BytesIn)
	}
}

func TestRootModel_Update_LogEntryMsg(t *testing.T) {
	m := createTestRootModel()

	msg := LogEntryMsg{
		Level:   log.InfoLevel,
		Message: "test log message",
		Time:    time.Now(),
	}
	model, cmd := m.Update(msg)

	if model == nil {
		t.Error("Expected non-nil model after log entry")
	}
	if cmd == nil {
		t.Error("Expected non-nil command (resubscribe) after log entry")
	}
}

func TestRootModel_Update_ShutdownMsg(t *testing.T) {
	m := createTestRootModel()

	msg := ShutdownMsg{}
	model, cmd := m.Update(msg)
	result := model.(*RootModel)

	if !result.quitting {
		t.Error("Expected quitting to be true after ShutdownMsg")
	}
	if cmd == nil {
		t.Error("Expected tea.Quit command after ShutdownMsg")
	}
}

func TestRootModel_Update_RefreshMsg(t *testing.T) {
	m := createTestRootModel()

	msg := RefreshMsg{}
	model, _ := m.Update(msg)

	if model == nil {
		t.Error("Expected non-nil model after RefreshMsg")
	}
}

func TestRootModel_Update_OpenDetailMsg(t *testing.T) {
	m := createTestRootModel()

	// Add a service first
	m.store.AddForward(state.ForwardSnapshot{
		Key:         "svc.ns.ctx.pod.8080",
		ServiceKey:  "svc.ns.ctx",
		ServiceName: "svc",
		Namespace:   "ns",
		Context:     "ctx",
		PodName:     "pod",
		Status:      state.StatusActive,
	})

	msg := components.OpenDetailMsg{Key: "svc.ns.ctx.pod.8080"}
	m.Update(msg)

	if !m.detail.IsVisible() {
		t.Error("Detail should be visible after OpenDetailMsg")
	}
	if m.focus != FocusDetail {
		t.Error("Focus should be FocusDetail after OpenDetailMsg")
	}
}

func TestRootModel_Update_PodLogsStopMsg(t *testing.T) {
	m := createTestRootModel()

	// Set up a log stream
	ch := make(chan string)
	m.logStreamCh = ch
	cancelCalled := false
	m.logStreamCancel = func() { cancelCalled = true }

	msg := components.PodLogsStopMsg{}
	m.Update(msg)

	if !cancelCalled {
		t.Error("Expected cancel function to be called")
	}
	if m.logStreamCh != nil {
		t.Error("Expected logStreamCh to be nil after stop")
	}
}

func TestRootModel_Update_PodLogLineMsg(t *testing.T) {
	m := createTestRootModel()

	// Set up detail view with streaming
	m.store.AddForward(state.ForwardSnapshot{
		Key:         "svc.ns.ctx.pod.8080",
		ServiceKey:  "svc.ns.ctx",
		ServiceName: "svc",
		Namespace:   "ns",
		Context:     "ctx",
		PodName:     "pod",
		Status:      state.StatusActive,
	})
	m.detail.Show("svc.ns.ctx.pod.8080")
	m.detail.SetLogsStreaming(true)

	// Create channel for continued streaming
	ch := make(chan string, 1)
	m.logStreamCh = ch
	ch <- "next line"

	msg := components.PodLogLineMsg{Line: "test log line"}
	model, cmd := m.Update(msg)

	if model == nil {
		t.Error("Expected non-nil model")
	}
	if cmd == nil {
		t.Error("Expected non-nil command to continue streaming")
	}
}

func TestRootModel_Update_PodLogLineMsg_Error(t *testing.T) {
	m := createTestRootModel()

	m.store.AddForward(state.ForwardSnapshot{
		Key:         "svc.ns.ctx.pod.8080",
		ServiceKey:  "svc.ns.ctx",
		ServiceName: "svc",
		Namespace:   "ns",
		Context:     "ctx",
		PodName:     "pod",
		Status:      state.StatusActive,
	})
	m.detail.Show("svc.ns.ctx.pod.8080")
	m.detail.SetLogsStreaming(true)

	// Send error marker
	msg := components.PodLogLineMsg{Line: "\x00ERROR:connection refused"}
	m.Update(msg)

	if m.detail.IsLogsStreaming() {
		t.Error("Streaming should be stopped after error")
	}
}

func TestRootModel_Update_PodLogsErrorMsg(t *testing.T) {
	m := createTestRootModel()

	m.store.AddForward(state.ForwardSnapshot{
		Key:         "svc.ns.ctx.pod.8080",
		ServiceKey:  "svc.ns.ctx",
		ServiceName: "svc",
		Namespace:   "ns",
		Context:     "ctx",
		PodName:     "pod",
		Status:      state.StatusActive,
	})
	m.detail.Show("svc.ns.ctx.pod.8080")
	m.detail.SetLogsStreaming(true)

	msg := components.PodLogsErrorMsg{Error: fmt.Errorf("test error")}
	m.Update(msg)

	if m.detail.IsLogsStreaming() {
		t.Error("Streaming should be stopped after error")
	}
}

func TestRootModel_Update_ReconnectErroredMsg(t *testing.T) {
	m := createTestRootModel()

	reconnectCalled := false
	m.reconnectErrored = func() int {
		reconnectCalled = true
		return 2
	}

	msg := components.ReconnectErroredMsg{}
	_, cmd := m.Update(msg)

	if !reconnectCalled {
		t.Error("Expected reconnectErrored to be called")
	}
	if cmd == nil {
		t.Error("Expected log command")
	}
}

func TestRootModel_Update_RemoveForwardMsg(t *testing.T) {
	m := createTestRootModel()

	removedKey := ""
	m.removeForward = func(key string) error {
		removedKey = key
		return nil
	}

	msg := components.RemoveForwardMsg{RegistryKey: "svc.ns"}
	_, cmd := m.Update(msg)

	if removedKey != "svc.ns" {
		t.Errorf("Expected removed key 'svc.ns', got '%s'", removedKey)
	}
	if cmd == nil {
		t.Error("Expected log command")
	}
}

func TestRootModel_Update_RemoveForwardMsg_Error(t *testing.T) {
	m := createTestRootModel()

	m.removeForward = func(key string) error {
		return fmt.Errorf("remove failed")
	}

	msg := components.RemoveForwardMsg{RegistryKey: "svc.ns"}
	_, cmd := m.Update(msg)

	// Should return error log command
	if cmd == nil {
		t.Error("Expected error log command")
	}
}

func TestRootModel_Update_ServiceForwardedMsg(t *testing.T) {
	m := createTestRootModel()

	msg := components.ServiceForwardedMsg{
		ServiceName: "myservice",
		LocalIP:     "127.1.0.1",
		Error:       nil,
	}
	_, cmd := m.Update(msg)

	if cmd == nil {
		t.Error("Expected log command for successful forward")
	}
}

func TestRootModel_Update_NamespaceForwardedMsg(t *testing.T) {
	m := createTestRootModel()

	msg := components.NamespaceForwardedMsg{
		Namespace:    "default",
		ServiceCount: 5,
		Error:        nil,
	}
	_, cmd := m.Update(msg)

	if cmd == nil {
		t.Error("Expected log command for successful namespace forward")
	}
}

// =============================================================================
// handleMetricsUpdate Tests
// =============================================================================

func TestHandleMetricsUpdate_AggregatesByPodKey(t *testing.T) {
	m := createTestRootModel()

	// Add a forward
	m.store.AddForward(state.ForwardSnapshot{
		Key:         "svc.ns.ctx.pod.8080",
		ServiceKey:  "svc.ns.ctx",
		ServiceName: "svc",
		Namespace:   "ns",
		Context:     "ctx",
		PodName:     "pod",
		LocalPort:   "8080",
		Status:      state.StatusActive,
	})

	msg := MetricsUpdateMsg{
		Snapshots: []fwdmetrics.ServiceSnapshot{
			{
				ServiceName: "svc",
				Namespace:   "ns",
				Context:     "ctx",
				PortForwards: []fwdmetrics.PortForwardSnapshot{
					{
						PodName:    "pod",
						LocalPort:  "8080",
						BytesIn:    1000,
						BytesOut:   2000,
						RateIn:     50.0,
						RateOut:    100.0,
						AvgRateIn:  45.0,
						AvgRateOut: 95.0,
					},
				},
			},
		},
	}

	m.handleMetricsUpdate(msg)

	// Verify the metrics were stored
	forwards := m.store.GetFiltered()
	if len(forwards) != 1 {
		t.Fatalf("Expected 1 forward, got %d", len(forwards))
	}

	if forwards[0].BytesIn != 1000 {
		t.Errorf("Expected BytesIn 1000, got %d", forwards[0].BytesIn)
	}
	if forwards[0].BytesOut != 2000 {
		t.Errorf("Expected BytesOut 2000, got %d", forwards[0].BytesOut)
	}
}

func TestHandleMetricsUpdate_RateHistory(t *testing.T) {
	m := createTestRootModel()

	// Add a forward
	m.store.AddForward(state.ForwardSnapshot{
		Key:         "svc.ns.ctx.pod.8080",
		ServiceKey:  "svc.ns.ctx",
		ServiceName: "svc",
		Namespace:   "ns",
		Context:     "ctx",
		PodName:     "pod",
		LocalPort:   "8080",
		Status:      state.StatusActive,
	})

	msg := MetricsUpdateMsg{
		Snapshots: []fwdmetrics.ServiceSnapshot{
			{
				ServiceName: "svc",
				Namespace:   "ns",
				Context:     "ctx",
				PortForwards: []fwdmetrics.PortForwardSnapshot{
					{
						PodName:   "pod",
						LocalPort: "8080",
						RateIn:    100.0,
						RateOut:   200.0,
					},
				},
			},
		},
	}

	m.handleMetricsUpdate(msg)

	// Verify rate history was updated
	key := "svc.ns.ctx.pod.8080"
	rateIn, _ := m.rateHistory.GetHistory(key, 10)
	if len(rateIn) == 0 {
		t.Error("Expected rate history to have samples")
	}
}

func TestHandleMetricsUpdate_DetailViewHTTPLogs(t *testing.T) {
	m := createTestRootModel()

	// Add a forward and show detail
	m.store.AddForward(state.ForwardSnapshot{
		Key:         "svc.ns.ctx.pod.8080",
		ServiceKey:  "svc.ns.ctx",
		ServiceName: "svc",
		Namespace:   "ns",
		Context:     "ctx",
		PodName:     "pod",
		LocalPort:   "8080",
		Status:      state.StatusActive,
	})
	m.detail.Show("svc.ns.ctx.pod.8080")

	msg := MetricsUpdateMsg{
		Snapshots: []fwdmetrics.ServiceSnapshot{
			{
				ServiceName: "svc",
				Namespace:   "ns",
				Context:     "ctx",
				PortForwards: []fwdmetrics.PortForwardSnapshot{
					{
						PodName:   "pod",
						LocalPort: "8080",
						HTTPLogs: []fwdmetrics.HTTPLogEntry{
							{
								Method:     "GET",
								Path:       "/api/v1/users",
								StatusCode: 200,
								Duration:   time.Millisecond * 50,
								Size:       1024,
								Timestamp:  time.Now(),
							},
						},
					},
				},
			},
		},
	}

	m.handleMetricsUpdate(msg)

	// Should not panic, HTTP logs should be set on detail view
}

func TestHandleMetricsUpdate_MetricsAggregation(t *testing.T) {
	m := createTestRootModel()

	// Add a forward
	m.store.AddForward(state.ForwardSnapshot{
		Key:         "svc.ns.ctx.pod.8080",
		ServiceKey:  "svc.ns.ctx",
		ServiceName: "svc",
		Namespace:   "ns",
		Context:     "ctx",
		PodName:     "pod",
		LocalPort:   "8080",
		Status:      state.StatusActive,
	})

	// Send metrics update with multiple snapshots for the same key
	// This tests the aggregation logic where podTotals[key] already exists
	msg := MetricsUpdateMsg{
		Snapshots: []fwdmetrics.ServiceSnapshot{
			{
				ServiceName: "svc",
				Namespace:   "ns",
				Context:     "ctx",
				PortForwards: []fwdmetrics.PortForwardSnapshot{
					{
						PodName:   "pod",
						LocalPort: "8080",
						BytesIn:   1000,
						BytesOut:  2000,
						RateIn:    50.0,
						RateOut:   100.0,
					},
				},
			},
			// Second snapshot for the same service (e.g., from different collectors)
			{
				ServiceName: "svc",
				Namespace:   "ns",
				Context:     "ctx",
				PortForwards: []fwdmetrics.PortForwardSnapshot{
					{
						PodName:   "pod",
						LocalPort: "8080",
						BytesIn:   500,
						BytesOut:  750,
						RateIn:    25.0,
						RateOut:   50.0,
					},
				},
			},
		},
	}

	m.handleMetricsUpdate(msg)

	// Verify the metrics were aggregated
	forwards := m.store.GetFiltered()
	if len(forwards) != 1 {
		t.Fatalf("Expected 1 forward, got %d", len(forwards))
	}

	// Aggregated: 1000 + 500 = 1500
	if forwards[0].BytesIn != 1500 {
		t.Errorf("Expected BytesIn 1500, got %d", forwards[0].BytesIn)
	}
	// Aggregated: 2000 + 750 = 2750
	if forwards[0].BytesOut != 2750 {
		t.Errorf("Expected BytesOut 2750, got %d", forwards[0].BytesOut)
	}
}

func TestHandleMetricsUpdate_NilRateHistory(t *testing.T) {
	m := createTestRootModel()
	m.rateHistory = nil // Set to nil to test the nil check

	// Add a forward
	m.store.AddForward(state.ForwardSnapshot{
		Key:         "svc.ns.ctx.pod.8080",
		ServiceKey:  "svc.ns.ctx",
		ServiceName: "svc",
		Namespace:   "ns",
		Context:     "ctx",
		PodName:     "pod",
		LocalPort:   "8080",
		Status:      state.StatusActive,
	})

	msg := MetricsUpdateMsg{
		Snapshots: []fwdmetrics.ServiceSnapshot{
			{
				ServiceName: "svc",
				Namespace:   "ns",
				Context:     "ctx",
				PortForwards: []fwdmetrics.PortForwardSnapshot{
					{
						PodName:   "pod",
						LocalPort: "8080",
						RateIn:    100.0,
						RateOut:   200.0,
					},
				},
			},
		},
	}

	// Should not panic even with nil rateHistory
	m.handleMetricsUpdate(msg)
}

func TestHandleMetricsUpdate_DetailNotVisible(t *testing.T) {
	m := createTestRootModel()

	// Add a forward but don't show detail
	m.store.AddForward(state.ForwardSnapshot{
		Key:         "svc.ns.ctx.pod.8080",
		ServiceKey:  "svc.ns.ctx",
		ServiceName: "svc",
		Namespace:   "ns",
		Context:     "ctx",
		PodName:     "pod",
		LocalPort:   "8080",
		Status:      state.StatusActive,
	})

	// Detail is not visible by default
	msg := MetricsUpdateMsg{
		Snapshots: []fwdmetrics.ServiceSnapshot{
			{
				ServiceName: "svc",
				Namespace:   "ns",
				Context:     "ctx",
				PortForwards: []fwdmetrics.PortForwardSnapshot{
					{
						PodName:   "pod",
						LocalPort: "8080",
						BytesIn:   1000,
						BytesOut:  2000,
						HTTPLogs: []fwdmetrics.HTTPLogEntry{
							{
								Method:     "GET",
								Path:       "/test",
								StatusCode: 200,
							},
						},
					},
				},
			},
		},
	}

	// Should update metrics without updating detail view
	m.handleMetricsUpdate(msg)

	// Verify metrics were still updated
	forwards := m.store.GetFiltered()
	if len(forwards) != 1 {
		t.Fatalf("Expected 1 forward, got %d", len(forwards))
	}
	if forwards[0].BytesIn != 1000 {
		t.Errorf("Expected BytesIn 1000, got %d", forwards[0].BytesIn)
	}
}

// =============================================================================
// tuiLogHook Tests
// =============================================================================

func TestTuiLogHook_Levels(t *testing.T) {
	hook := &tuiLogHook{}
	levels := hook.Levels()

	if len(levels) == 0 {
		t.Error("Expected non-empty levels")
	}

	// Should include all levels
	if len(levels) != len(log.AllLevels) {
		t.Errorf("Expected %d levels, got %d", len(log.AllLevels), len(levels))
	}
}

func TestTuiLogHook_Fire_Success(t *testing.T) {
	logCh := make(chan LogEntryMsg, 10)
	stopCh := make(chan struct{})

	hook := &tuiLogHook{logCh: logCh, stopCh: stopCh}

	entry := &log.Entry{
		Level:   log.InfoLevel,
		Message: "test message",
		Time:    time.Now(),
	}

	err := hook.Fire(entry)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	select {
	case msg := <-logCh:
		if msg.Message != "test message" {
			t.Errorf("Expected 'test message', got '%s'", msg.Message)
		}
		if msg.Level != log.InfoLevel {
			t.Errorf("Expected InfoLevel, got %v", msg.Level)
		}
	case <-time.After(time.Second):
		t.Error("Message not received")
	}
}

func TestTuiLogHook_Fire_BufferFull(t *testing.T) {
	logCh := make(chan LogEntryMsg) // Unbuffered - will be full
	stopCh := make(chan struct{})

	hook := &tuiLogHook{logCh: logCh, stopCh: stopCh}

	entry := &log.Entry{
		Level:   log.InfoLevel,
		Message: "test message",
		Time:    time.Now(),
	}

	// Should not block even when buffer is full
	done := make(chan bool)
	go func() {
		_ = hook.Fire(entry)
		done <- true
	}()

	select {
	case <-done:
		// Success - didn't block
	case <-time.After(time.Millisecond * 100):
		t.Error("Fire blocked when buffer was full")
	}
}

func TestTuiLogHook_Fire_DuringShutdown(t *testing.T) {
	logCh := make(chan LogEntryMsg) // Unbuffered - will be full
	stopCh := make(chan struct{})
	close(stopCh) // Simulate shutdown

	hook := &tuiLogHook{logCh: logCh, stopCh: stopCh}

	entry := &log.Entry{
		Level:   log.InfoLevel,
		Message: "test message",
		Time:    time.Now(),
	}

	// Should not block during shutdown
	done := make(chan bool)
	go func() {
		_ = hook.Fire(entry)
		done <- true
	}()

	select {
	case <-done:
		// Success - didn't block
	case <-time.After(time.Millisecond * 100):
		t.Error("Fire blocked during shutdown")
	}
}

// =============================================================================
// Manager Method Tests
// =============================================================================

func TestManager_SetPodLogsStreamer(t *testing.T) {
	m := createTestRootModel()
	manager := &Manager{model: m}

	manager.SetPodLogsStreamer(func(ctx context.Context, ns, pod, container, k8sCtx string, tail int64) (io.ReadCloser, error) {
		return nil, fmt.Errorf("test")
	})

	if m.streamPodLogs == nil {
		t.Error("Expected streamPodLogs to be set")
	}
}

func TestManager_SetErroredServicesReconnector(t *testing.T) {
	m := createTestRootModel()
	manager := &Manager{model: m}

	reconnectorCalled := false
	manager.SetErroredServicesReconnector(func() int {
		reconnectorCalled = true
		return 0
	})

	if m.reconnectErrored == nil {
		t.Error("Expected reconnectErrored to be set")
	}

	m.reconnectErrored()
	if !reconnectorCalled {
		t.Error("Expected reconnector to be called")
	}
}

func TestManager_SetRemoveForwardCallback(t *testing.T) {
	m := createTestRootModel()
	manager := &Manager{model: m}

	manager.SetRemoveForwardCallback(func(key string) error {
		return nil
	})

	if m.removeForward == nil {
		t.Error("Expected removeForward to be set")
	}
}

func TestManager_Done(t *testing.T) {
	manager := &Manager{
		doneChan: make(chan struct{}),
	}

	doneCh := manager.Done()
	if doneCh == nil {
		t.Error("Expected non-nil done channel")
	}

	// Close and verify channel is closed
	close(manager.doneChan)
	select {
	case <-doneCh:
		// Success
	case <-time.After(time.Millisecond * 100):
		t.Error("Done channel not closed")
	}
}

// =============================================================================
// InitEventInfrastructure Tests
// =============================================================================

func TestInitEventInfrastructure(t *testing.T) {
	// InitEventInfrastructure uses sync.Once so we can only test idempotency
	// Call it multiple times - should not panic
	InitEventInfrastructure()
	InitEventInfrastructure()
	InitEventInfrastructure()

	// After initialization, EventsEnabled should return true
	// (or false if it was already initialized before this test)
	// We just verify it doesn't panic
	_ = EventsEnabled()
}

// =============================================================================
// handleEventForStore All Statuses Tests
// =============================================================================

func TestHandleEventForStore_AllStatuses(t *testing.T) {
	tests := []struct {
		status       string
		expectedStat state.ForwardStatus
	}{
		{"connecting", state.StatusConnecting},
		{"active", state.StatusActive},
		{"error", state.StatusError},
		{"stopping", state.StatusStopping},
		{"unknown_status", state.StatusPending}, // default
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			store := state.NewStore(100)

			// Add first
			addEvent := events.NewPodEvent(events.PodAdded, "svc", "ns", "ctx", "pod1", "svc.ns")
			addEvent.LocalPort = "8080"
			handleEventForStore(store, addEvent)

			// Change status
			statusEvent := events.Event{
				Type:       events.PodStatusChanged,
				Service:    "svc",
				PodName:    "pod1",
				Namespace:  "ns",
				Context:    "ctx",
				LocalPort:  "8080",
				ServiceKey: "svc.ns.ctx",
				Status:     tt.status,
			}
			handleEventForStore(store, statusEvent)

			forwards := store.GetFiltered()
			if len(forwards) != 1 {
				t.Fatalf("Expected 1 forward, got %d", len(forwards))
			}
			if forwards[0].Status != tt.expectedStat {
				t.Errorf("Expected status %v, got %v", tt.expectedStat, forwards[0].Status)
			}
		})
	}
}

func TestHandleEventForStore_ServiceRemoved(t *testing.T) {
	store := state.NewStore(100)

	// Add multiple pods for same service
	for i := 1; i <= 3; i++ {
		addEvent := events.NewPodEvent(events.PodAdded, "svc", "ns", "ctx", fmt.Sprintf("pod%d", i), "svc.ns")
		addEvent.LocalPort = "8080"
		handleEventForStore(store, addEvent)
	}

	// Add a different service
	addEvent := events.NewPodEvent(events.PodAdded, "other", "ns", "ctx", "otherpod", "other.ns")
	addEvent.LocalPort = "9090"
	handleEventForStore(store, addEvent)

	if store.Count() != 4 {
		t.Fatalf("Expected 4 forwards, got %d", store.Count())
	}

	// Remove first service
	removeEvent := events.Event{
		Type:       events.ServiceRemoved,
		Service:    "svc",
		Namespace:  "ns",
		Context:    "ctx",
		ServiceKey: "svc.ns.ctx",
	}
	handleEventForStore(store, removeEvent)

	if store.Count() != 1 {
		t.Errorf("Expected 1 forward after removal, got %d", store.Count())
	}
}

func TestHandleEventForStore_UpdateHostnamesOnActive(t *testing.T) {
	store := state.NewStore(100)

	// Add first
	addEvent := events.NewPodEvent(events.PodAdded, "svc", "ns", "ctx", "pod1", "svc.ns")
	addEvent.LocalPort = "8080"
	handleEventForStore(store, addEvent)

	// Change to active with hostnames
	statusEvent := events.Event{
		Type:       events.PodStatusChanged,
		Service:    "svc",
		PodName:    "pod1",
		Namespace:  "ns",
		Context:    "ctx",
		LocalPort:  "8080",
		ServiceKey: "svc.ns.ctx",
		Status:     "active",
		Hostnames:  []string{"svc", "svc.ns"},
	}
	handleEventForStore(store, statusEvent)

	forwards := store.GetFiltered()
	if len(forwards) != 1 {
		t.Fatalf("Expected 1 forward, got %d", len(forwards))
	}
	if len(forwards[0].Hostnames) != 2 {
		t.Errorf("Expected 2 hostnames, got %d", len(forwards[0].Hostnames))
	}
}

// =============================================================================
// RootModel.Init Tests
// =============================================================================

func TestRootModel_Init(t *testing.T) {
	m := createTestRootModel()

	cmd := m.Init()

	// Init should return a batch command
	if cmd == nil {
		t.Error("Expected non-nil command from Init()")
	}
}

// =============================================================================
// Update Panic Recovery Tests
// =============================================================================

func TestRootModel_Update_PanicRecovery(t *testing.T) {
	m := createTestRootModel()

	// This should not crash the test - panic is recovered
	// We can't easily inject a panic, but we can verify the Update method exists
	// and handles nil messages gracefully

	// Pass nil message type that doesn't match any case
	type unknownMsg struct{}
	model, _ := m.Update(unknownMsg{})

	if model == nil {
		t.Error("Expected non-nil model even for unknown message type")
	}
}

// =============================================================================
// Manager with nil model Tests
// =============================================================================

func TestManager_SetCallbacks_NilModel(t *testing.T) {
	manager := &Manager{model: nil}

	// Should not panic with nil model
	manager.SetPodLogsStreamer(nil)
	manager.SetErroredServicesReconnector(nil)
	manager.SetBrowseDiscovery(nil)
	manager.SetBrowseNamespaceController(nil)
	manager.SetBrowseServiceCRUD(nil)
	manager.SetHeaderContext("test")
	manager.SetRemoveForwardCallback(nil)
}

func TestManager_SetCallbacks_WithModel(t *testing.T) {
	model := createTestRootModel()
	manager := &Manager{model: model}

	// Should set callbacks without panic (nil values are acceptable)
	manager.SetBrowseDiscovery(nil)
	manager.SetBrowseNamespaceController(nil)
	manager.SetBrowseServiceCRUD(nil)
	manager.SetHeaderContext("test-context")

	// Test with non-nil callbacks
	manager.SetErroredServicesReconnector(func() int { return 0 })
	manager.SetRemoveForwardCallback(func(key string) error { return nil })
}

// =============================================================================
// Additional Update Path Tests
// =============================================================================

func TestRootModel_Update_ServiceForwardedMsg_WithError(t *testing.T) {
	m := createTestRootModel()

	msg := components.ServiceForwardedMsg{
		ServiceName: "myservice",
		LocalIP:     "127.1.0.1",
		Error:       fmt.Errorf("failed to forward"),
	}
	updated, _ := m.Update(msg)

	// With error, model should still be returned (browse modal updates)
	if updated == nil {
		t.Error("Expected non-nil model even with error")
	}
}

func TestRootModel_Update_NamespaceForwardedMsg_WithError(t *testing.T) {
	m := createTestRootModel()

	msg := components.NamespaceForwardedMsg{
		Namespace:    "default",
		ServiceCount: 0,
		Error:        fmt.Errorf("failed to forward namespace"),
	}
	updated, _ := m.Update(msg)

	// With error, model should still be returned (browse modal updates)
	if updated == nil {
		t.Error("Expected non-nil model even with error")
	}
}

func TestRootModel_Update_BrowseContextsLoadedMsg(t *testing.T) {
	m := createTestRootModel()

	msg := components.BrowseContextsLoadedMsg{
		Contexts:       []types.K8sContext{{Name: "ctx1"}, {Name: "ctx2"}},
		CurrentContext: "ctx1",
	}
	updated, _ := m.Update(msg)

	if updated == nil {
		t.Error("Expected non-nil model after BrowseContextsLoadedMsg")
	}

	// Header should be updated with current context
	rootModel := updated.(*RootModel)
	if rootModel.header.GetContext() != "ctx1" {
		t.Errorf("Expected header context 'ctx1', got '%s'", rootModel.header.GetContext())
	}
}

func TestRootModel_Update_BrowseNamespacesLoadedMsg_Forwarding(t *testing.T) {
	m := createTestRootModel()

	msg := components.BrowseNamespacesLoadedMsg{
		Namespaces: []types.K8sNamespace{
			{Name: "default", Status: "Active"},
			{Name: "kube-system", Status: "Active"},
		},
	}
	updated, _ := m.Update(msg)

	if updated == nil {
		t.Error("Expected non-nil model after BrowseNamespacesLoadedMsg")
	}
}

func TestRootModel_Update_BrowseServicesLoadedMsg_Forwarding(t *testing.T) {
	m := createTestRootModel()

	msg := components.BrowseServicesLoadedMsg{
		Services: []types.K8sService{
			{Name: "api", Namespace: "default", Type: "ClusterIP"},
			{Name: "db", Namespace: "default", Type: "ClusterIP"},
		},
	}
	updated, _ := m.Update(msg)

	if updated == nil {
		t.Error("Expected non-nil model after BrowseServicesLoadedMsg")
	}
}

func TestRootModel_Update_PodLogLineMsg_WithErrorMarker(t *testing.T) {
	m := createTestRootModel()

	// Show detail first
	m.store.AddForward(state.ForwardSnapshot{
		Key:         "svc.ns.ctx.pod.8080",
		ServiceKey:  "svc.ns.ctx",
		ServiceName: "svc",
		Namespace:   "ns",
		Context:     "ctx",
		PodName:     "pod",
		LocalPort:   "8080",
		Status:      state.StatusActive,
	})
	m.detail.Show("svc.ns.ctx.pod.8080")
	m.detail.SetLogsStreaming(true)

	// Send error marker
	msg := components.PodLogLineMsg{Line: "\x00ERROR:connection refused"}
	m.Update(msg)

	// Should have set error and stopped streaming
	if m.detail.IsLogsStreaming() {
		t.Error("Expected streaming to stop after error marker")
	}
}

func TestRootModel_Update_PodLogLineMsg_NormalLine(t *testing.T) {
	m := createTestRootModel()

	// Show detail first
	m.store.AddForward(state.ForwardSnapshot{
		Key:         "svc.ns.ctx.pod.8080",
		ServiceKey:  "svc.ns.ctx",
		ServiceName: "svc",
		Namespace:   "ns",
		Context:     "ctx",
		PodName:     "pod",
		LocalPort:   "8080",
		Status:      state.StatusActive,
	})
	m.detail.Show("svc.ns.ctx.pod.8080")
	m.detail.SetLogsStreaming(true)

	// Create a log stream channel
	m.logStreamCh = make(chan string, 10)

	// Send normal log line
	msg := components.PodLogLineMsg{Line: "2024-01-15 INFO: Server started"}
	updated, cmd := m.Update(msg)

	if updated == nil {
		t.Error("Expected non-nil model")
	}
	if m.detail.IsLogsStreaming() && cmd == nil {
		t.Error("Expected command to continue listening when streaming")
	}
}

func TestRootModel_Update_PodLogsRequestMsg_NilStreamer(t *testing.T) {
	m := createTestRootModel()

	// Ensure streamPodLogs is nil
	m.streamPodLogs = nil

	msg := components.PodLogsRequestMsg{
		Namespace:     "default",
		PodName:       "my-pod",
		ContainerName: "main",
		Context:       "ctx",
		TailLines:     100,
	}
	m.Update(msg)

	// Should have set error when no streamer available
	// We can't easily check the error message, but it shouldn't panic
}

func TestRootModel_Update_MouseClick_InLogsArea(t *testing.T) {
	m := createTestRootModel()

	// Set up sizes first
	m.width = 100
	m.height = 30
	m.updateSizes()

	// Start with services focused
	m.focus = FocusServices
	m.services.SetFocus(true)
	m.logs.SetFocus(false)

	// Simulate click in logs area
	msg := tea.MouseMsg{
		Button: tea.MouseButtonLeft,
		Y:      m.logsStartY + 1, // Click inside logs area
	}
	m.Update(msg)

	// Focus should switch to logs
	if m.focus != FocusLogs {
		t.Errorf("Expected FocusLogs, got %v", m.focus)
	}
}

func TestRootModel_Update_MouseWheel_InLogs(t *testing.T) {
	m := createTestRootModel()

	// Set up sizes first
	m.width = 100
	m.height = 30
	m.updateSizes()

	// Focus on logs
	m.focus = FocusLogs
	m.logs.SetFocus(true)

	// Simulate wheel scroll
	msg := tea.MouseMsg{
		Button: tea.MouseButtonWheelDown,
	}
	_, cmd := m.Update(msg)

	// Should have processed the event (cmd may be nil for wheel events in logs)
	_ = cmd // wheel scroll in logs shouldn't error
}

func TestRootModel_Update_KeyMsg_R_NoReconnector(t *testing.T) {
	m := createTestRootModel()
	m.reconnectErrored = nil

	// Press 'r' with no reconnector set
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")}
	_, cmd := m.Update(msg)

	// Should return nil cmd when no reconnector
	if cmd != nil {
		t.Error("Expected nil command when reconnector not set")
	}
}

func TestRootModel_Update_KeyMsg_Tab_CycleFocus(t *testing.T) {
	m := createTestRootModel()
	m.focus = FocusServices

	// Press tab to cycle focus
	msg := tea.KeyMsg{Type: tea.KeyTab}
	m.Update(msg)

	// Focus should change
	if m.focus != FocusLogs {
		t.Errorf("Expected FocusLogs after tab, got %v", m.focus)
	}
}

func TestRootModel_Update_KeyMsg_HelpToggle(t *testing.T) {
	m := createTestRootModel()

	// Press '?' to toggle help
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")}
	m.Update(msg)

	if !m.help.IsVisible() {
		t.Error("Help should be visible after '?' press")
	}

	// Press '?' again to hide
	m.Update(msg)
	if m.help.IsVisible() {
		t.Error("Help should be hidden after second '?' press")
	}
}

func TestRootModel_Update_KeyMsg_Enter_DetailView_NoSelection(t *testing.T) {
	m := createTestRootModel()
	m.focus = FocusServices

	// No forwards added, so no selection
	msg := tea.KeyMsg{Type: tea.KeyEnter}
	m.Update(msg)

	// Detail should not be visible
	if m.detail.IsVisible() {
		t.Error("Detail should not be visible when no selection")
	}
}

func TestRootModel_Update_KeyMsg_BrowseVisible(t *testing.T) {
	m := createTestRootModel()

	// Make browse visible
	m.browse.SetSize(100, 30)
	// We can't easily show browse, but we can test the path where it's visible

	// Just verify the code path works without error
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}
	_, _ = m.Update(msg)
}

func TestRootModel_Update_KeyMsg_InLogsPanel(t *testing.T) {
	m := createTestRootModel()
	m.focus = FocusLogs
	m.logs.SetFocus(true)
	m.logs.SetSize(100, 10)

	// Send key to logs panel
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}
	updated, _ := m.Update(msg)

	if updated == nil {
		t.Error("Expected non-nil model")
	}
}

func TestRootModel_Update_ReconnectErroredMsg_WithCount(t *testing.T) {
	m := createTestRootModel()

	// Set up reconnect function that returns count > 0
	m.reconnectErrored = func() int { return 3 }

	msg := components.ReconnectErroredMsg{}
	_, cmd := m.Update(msg)

	// Should return a log command
	if cmd == nil {
		t.Error("Expected non-nil command for reconnect with count > 0")
	}
}

func TestRootModel_Update_ReconnectErroredMsg_NoErrors(t *testing.T) {
	m := createTestRootModel()

	// Set up reconnect function that returns 0
	m.reconnectErrored = func() int { return 0 }

	msg := components.ReconnectErroredMsg{}
	_, cmd := m.Update(msg)

	// Should return a log command indicating no errors
	if cmd == nil {
		t.Error("Expected non-nil command for reconnect with 0 count")
	}
}

func TestRootModel_Update_OpenDetailMsg_ValidKey(t *testing.T) {
	m := createTestRootModel()

	// Add a forward so the key is valid
	m.store.AddForward(state.ForwardSnapshot{
		Key:         "svc.ns.ctx.pod.8080",
		ServiceKey:  "svc.ns.ctx",
		ServiceName: "svc",
		Namespace:   "ns",
		Context:     "ctx",
		PodName:     "pod",
		LocalPort:   "8080",
		Status:      state.StatusActive,
	})

	msg := components.OpenDetailMsg{Key: "svc.ns.ctx.pod.8080"}
	m.Update(msg)

	if !m.detail.IsVisible() {
		t.Error("Expected detail to be visible after OpenDetailMsg")
	}
	if m.focus != FocusDetail {
		t.Errorf("Expected focus to be FocusDetail, got %v", m.focus)
	}
}

func TestRootModel_Update_OpenDetailMsg_EmptyKey(t *testing.T) {
	m := createTestRootModel()

	// Empty key should not open detail
	msg := components.OpenDetailMsg{Key: ""}
	m.Update(msg)

	if m.detail.IsVisible() {
		t.Error("Detail should not be visible for empty key")
	}
}

func TestRootModel_Update_RemoveForwardMsg_NoCallback(t *testing.T) {
	m := createTestRootModel()
	m.removeForward = nil

	msg := components.RemoveForwardMsg{RegistryKey: "svc.ns.ctx"}
	_, cmd := m.Update(msg)

	// Should return nil cmd when no callback
	if cmd != nil {
		t.Error("Expected nil command when removeForward not set")
	}
}

func TestRootModel_Update_RemoveForwardMsg_EmptyKey(t *testing.T) {
	m := createTestRootModel()

	removeCalled := false
	m.removeForward = func(key string) error {
		removeCalled = true
		return nil
	}

	// Empty key should not call callback
	msg := components.RemoveForwardMsg{RegistryKey: ""}
	_, _ = m.Update(msg)

	if removeCalled {
		t.Error("removeForward should not be called for empty key")
	}
}

func TestRootModel_Update_KeyMsg_R_WithReconnector_CountZero(t *testing.T) {
	m := createTestRootModel()
	m.reconnectErrored = func() int { return 0 }

	// Press 'r' when no services have errors
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")}
	_, cmd := m.Update(msg)

	// Should return a log command indicating no errors
	if cmd == nil {
		t.Error("Expected non-nil command for 'r' with no errors")
	}
}

func TestRootModel_Update_KeyMsg_R_WithReconnector_CountPositive(t *testing.T) {
	m := createTestRootModel()
	m.reconnectErrored = func() int { return 5 }

	// Press 'r' when services have errors
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")}
	_, cmd := m.Update(msg)

	// Should return a log command indicating reconnection
	if cmd == nil {
		t.Error("Expected non-nil command for 'r' with errors")
	}
}

func TestRootModel_Update_KeyMsg_Q_WhileFiltering(t *testing.T) {
	m := createTestRootModel()
	m.services.SetSize(100, 20)

	// Enter filter mode by simulating '/'
	filterMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")}
	m.Update(filterMsg)

	// Now press 'q' - should NOT quit because we're filtering
	qMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}
	_, cmd := m.Update(qMsg)

	// Should not be quitting
	if m.quitting {
		t.Error("Should not quit while filtering")
	}

	// cmd should not be tea.Quit
	if cmd != nil {
		// Execute the cmd to see if it's tea.Quit
		result := cmd()
		if _, ok := result.(tea.QuitMsg); ok {
			t.Error("Should not return tea.Quit while filtering")
		}
	}
}
