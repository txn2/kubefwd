package fwdapi

import (
	"sync"
	"testing"
	"time"

	"github.com/txn2/kubefwd/pkg/fwdapi/types"
	"github.com/txn2/kubefwd/pkg/fwdmetrics"
	"github.com/txn2/kubefwd/pkg/fwdtui/events"
	"github.com/txn2/kubefwd/pkg/fwdtui/state"
)

// Reset global state between tests
func resetGlobalState() {
	mu.Lock()
	defer mu.Unlock()
	apiEnabled = false
	apiManager = nil
	once = sync.Once{} // This allows Init to run again
}

func TestEnable(t *testing.T) {
	resetGlobalState()

	if IsEnabled() {
		t.Error("Expected API to be disabled initially")
	}

	Enable()

	if !IsEnabled() {
		t.Error("Expected API to be enabled after Enable()")
	}
}

func TestIsEnabled(t *testing.T) {
	resetGlobalState()

	if IsEnabled() {
		t.Error("Expected IsEnabled to return false initially")
	}

	Enable()

	if !IsEnabled() {
		t.Error("Expected IsEnabled to return true after Enable()")
	}
}

func TestGetManager(t *testing.T) {
	resetGlobalState()

	if GetManager() != nil {
		t.Error("Expected GetManager to return nil before Init")
	}

	shutdownChan := make(chan struct{})
	triggerShutdown := func() {}

	Init(shutdownChan, triggerShutdown, "1.0.0")

	if GetManager() == nil {
		t.Error("Expected GetManager to return non-nil after Init")
	}
}

func TestInit(t *testing.T) {
	resetGlobalState()

	shutdownChan := make(chan struct{})
	triggerShutdown := func() {}

	manager := Init(shutdownChan, triggerShutdown, "1.0.0")

	if manager == nil {
		t.Fatal("Expected Init to return non-nil Manager")
	}

	if manager.version != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", manager.version)
	}

	if manager.stopChan == nil {
		t.Error("Expected stopChan to be initialized")
	}

	if manager.doneChan == nil {
		t.Error("Expected doneChan to be initialized")
	}
}

func TestInit_OnlyOnce(t *testing.T) {
	resetGlobalState()

	shutdownChan := make(chan struct{})
	triggerShutdown := func() {}

	manager1 := Init(shutdownChan, triggerShutdown, "1.0.0")
	manager2 := Init(shutdownChan, triggerShutdown, "2.0.0")

	if manager1 != manager2 {
		t.Error("Expected Init to return the same manager on subsequent calls")
	}

	if manager2.version != "1.0.0" {
		t.Errorf("Expected version to remain '1.0.0', got '%s'", manager2.version)
	}
}

func TestManager_SetStateReader(t *testing.T) {
	resetGlobalState()

	manager := &Manager{}
	mock := &mockStateReader{}

	manager.SetStateReader(mock)

	if manager.stateReader != mock {
		t.Error("Expected stateReader to be set")
	}
}

func TestManager_SetMetricsProvider(t *testing.T) {
	resetGlobalState()

	manager := &Manager{}
	mock := &mockMetricsProvider{}

	manager.SetMetricsProvider(mock)

	if manager.metricsProvider != mock {
		t.Error("Expected metricsProvider to be set")
	}
}

func TestManager_SetServiceController(t *testing.T) {
	resetGlobalState()

	manager := &Manager{}
	mock := &mockServiceController{}

	manager.SetServiceController(mock)

	if manager.serviceController != mock {
		t.Error("Expected serviceController to be set")
	}
}

func TestManager_SetEventStreamer(t *testing.T) {
	resetGlobalState()

	manager := &Manager{}
	mock := &mockEventStreamer{}

	manager.SetEventStreamer(mock)

	if manager.eventStreamer != mock {
		t.Error("Expected eventStreamer to be set")
	}
}

func TestManager_SetNamespaces(t *testing.T) {
	manager := &Manager{}
	namespaces := []string{"default", "kube-system"}

	manager.SetNamespaces(namespaces)

	if len(manager.namespaces) != 2 {
		t.Errorf("Expected 2 namespaces, got %d", len(manager.namespaces))
	}
}

func TestManager_SetContexts(t *testing.T) {
	manager := &Manager{}
	contexts := []string{"minikube", "prod"}

	manager.SetContexts(contexts)

	if len(manager.contexts) != 2 {
		t.Errorf("Expected 2 contexts, got %d", len(manager.contexts))
	}
}

func TestManager_SetTUIEnabled(t *testing.T) {
	manager := &Manager{}

	manager.SetTUIEnabled(true)

	if !manager.tuiEnabled {
		t.Error("Expected tuiEnabled to be true")
	}
}

func TestManager_Stop(t *testing.T) {
	manager := &Manager{
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}

	manager.Stop()

	select {
	case <-manager.stopChan:
		// Expected
	default:
		t.Error("Expected stopChan to be closed")
	}

	// Stop should be idempotent
	manager.Stop()
}

func TestManager_Done(t *testing.T) {
	manager := &Manager{
		doneChan: make(chan struct{}),
	}

	doneChan := manager.Done()

	if doneChan == nil {
		t.Error("Expected Done() to return non-nil channel")
	}
}

func TestManager_Uptime(t *testing.T) {
	startTime := time.Now().Add(-time.Hour)
	manager := &Manager{
		startTime: startTime,
	}

	uptime := manager.Uptime()

	if uptime < time.Hour {
		t.Errorf("Expected uptime >= 1 hour, got %v", uptime)
	}
}

func TestManager_StartTime(t *testing.T) {
	now := time.Now()
	manager := &Manager{
		startTime: now,
	}

	if !manager.StartTime().Equal(now) {
		t.Error("Expected StartTime to match")
	}
}

func TestManager_Version(t *testing.T) {
	manager := &Manager{
		version: "1.2.3",
	}

	if manager.Version() != "1.2.3" {
		t.Errorf("Expected version '1.2.3', got '%s'", manager.Version())
	}
}

func TestManager_Namespaces(t *testing.T) {
	manager := &Manager{
		namespaces: []string{"ns1", "ns2"},
	}

	ns := manager.Namespaces()

	if len(ns) != 2 {
		t.Errorf("Expected 2 namespaces, got %d", len(ns))
	}
}

func TestManager_Contexts(t *testing.T) {
	manager := &Manager{
		contexts: []string{"ctx1"},
	}

	ctx := manager.Contexts()

	if len(ctx) != 1 {
		t.Errorf("Expected 1 context, got %d", len(ctx))
	}
}

func TestManager_TUIEnabled(t *testing.T) {
	manager := &Manager{
		tuiEnabled: true,
	}

	if !manager.TUIEnabled() {
		t.Error("Expected TUIEnabled to return true")
	}
}

func TestManager_RunWithoutStateReader(t *testing.T) {
	manager := &Manager{
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}

	err := manager.Run()

	if err == nil {
		t.Error("Expected error when stateReader is not configured")
	}

	if err.Error() != "state reader not configured" {
		t.Errorf("Unexpected error message: %s", err.Error())
	}
}

// Mock implementations for manager tests

type mockStateReader struct{}

func (m *mockStateReader) GetServices() []state.ServiceSnapshot         { return nil }
func (m *mockStateReader) GetService(key string) *state.ServiceSnapshot { return nil }
func (m *mockStateReader) GetSummary() state.SummaryStats               { return state.SummaryStats{} }
func (m *mockStateReader) GetFiltered() []state.ForwardSnapshot         { return nil }
func (m *mockStateReader) GetForward(key string) *state.ForwardSnapshot { return nil }
func (m *mockStateReader) GetLogs(count int) []state.LogEntry           { return nil }
func (m *mockStateReader) Count() int                                   { return 0 }
func (m *mockStateReader) ServiceCount() int                            { return 0 }

type mockMetricsProvider struct{}

func (m *mockMetricsProvider) GetAllSnapshots() []fwdmetrics.ServiceSnapshot             { return nil }
func (m *mockMetricsProvider) GetServiceSnapshot(key string) *fwdmetrics.ServiceSnapshot { return nil }
func (m *mockMetricsProvider) GetTotals() (uint64, uint64, float64, float64)             { return 0, 0, 0, 0 }
func (m *mockMetricsProvider) ServiceCount() int                                         { return 0 }
func (m *mockMetricsProvider) PortForwardCount() int                                     { return 0 }

type mockServiceController struct{}

func (m *mockServiceController) Reconnect(key string) error        { return nil }
func (m *mockServiceController) ReconnectAll() int                 { return 0 }
func (m *mockServiceController) Sync(key string, force bool) error { return nil }

type mockEventStreamer struct{}

func (m *mockEventStreamer) Subscribe() (<-chan events.Event, func()) {
	ch := make(chan events.Event)
	return ch, func() { close(ch) }
}
func (m *mockEventStreamer) SubscribeType(eventType events.EventType) (<-chan events.Event, func()) {
	ch := make(chan events.Event)
	return ch, func() { close(ch) }
}

// Ensure mocks implement the interfaces
var (
	_ types.StateReader       = (*mockStateReader)(nil)
	_ types.MetricsProvider   = (*mockMetricsProvider)(nil)
	_ types.ServiceController = (*mockServiceController)(nil)
	_ types.EventStreamer     = (*mockEventStreamer)(nil)
)
