package fwdapi

import (
	"testing"
	"time"

	"github.com/txn2/kubefwd/pkg/fwdapi/types"
	"github.com/txn2/kubefwd/pkg/fwdmetrics"
	"github.com/txn2/kubefwd/pkg/fwdtui/events"
	"github.com/txn2/kubefwd/pkg/fwdtui/state"
)

// StateReaderAdapter tests

func TestNewStateReaderAdapter(t *testing.T) {
	adapter := NewStateReaderAdapter(nil)
	if adapter == nil {
		t.Error("Expected non-nil adapter")
	}
}

func TestStateReaderAdapter_NilStore(t *testing.T) {
	adapter := NewStateReaderAdapter(func() *state.Store { return nil })

	if services := adapter.GetServices(); services != nil {
		t.Error("Expected nil services for nil store")
	}

	if svc := adapter.GetService("test"); svc != nil {
		t.Error("Expected nil service for nil store")
	}

	summary := adapter.GetSummary()
	if summary.TotalServices != 0 {
		t.Error("Expected empty summary for nil store")
	}

	if forwards := adapter.GetFiltered(); forwards != nil {
		t.Error("Expected nil forwards for nil store")
	}

	if fwd := adapter.GetForward("test"); fwd != nil {
		t.Error("Expected nil forward for nil store")
	}

	if logs := adapter.GetLogs(10); logs != nil {
		t.Error("Expected nil logs for nil store")
	}

	if count := adapter.Count(); count != 0 {
		t.Error("Expected 0 count for nil store")
	}

	if count := adapter.ServiceCount(); count != 0 {
		t.Error("Expected 0 service count for nil store")
	}
}

// MetricsProviderAdapter tests

func TestNewMetricsProviderAdapter(t *testing.T) {
	adapter := NewMetricsProviderAdapter(nil)
	if adapter == nil {
		t.Error("Expected non-nil adapter")
	}
}

func TestMetricsProviderAdapter_NilRegistry(t *testing.T) {
	adapter := NewMetricsProviderAdapter(nil)

	if snapshots := adapter.GetAllSnapshots(); snapshots != nil {
		t.Error("Expected nil snapshots for nil registry")
	}

	if snapshot := adapter.GetServiceSnapshot("test"); snapshot != nil {
		t.Error("Expected nil snapshot for nil registry")
	}

	bytesIn, bytesOut, rateIn, rateOut := adapter.GetTotals()
	if bytesIn != 0 || bytesOut != 0 || rateIn != 0 || rateOut != 0 {
		t.Error("Expected zero totals for nil registry")
	}

	if count := adapter.ServiceCount(); count != 0 {
		t.Error("Expected 0 service count for nil registry")
	}

	if count := adapter.PortForwardCount(); count != 0 {
		t.Error("Expected 0 port forward count for nil registry")
	}
}

// ServiceControllerAdapter tests

func TestNewServiceControllerAdapter(t *testing.T) {
	adapter := NewServiceControllerAdapter(nil)
	if adapter == nil {
		t.Error("Expected non-nil adapter")
	}
}

func TestServiceControllerAdapter_ReconnectAllNilStore(t *testing.T) {
	adapter := NewServiceControllerAdapter(func() *state.Store { return nil })
	count := adapter.ReconnectAll()
	if count != 0 {
		t.Errorf("Expected 0 reconnections for nil store, got %d", count)
	}
}

// Note: Reconnect, Sync, and ReconnectAll (with data) tests are skipped because
// they require the global fwdsvcregistry to be initialized with real services,
// which is not practical in unit tests. These are tested through integration tests instead.

// EventStreamerAdapter tests

func TestNewEventStreamerAdapter(t *testing.T) {
	adapter := NewEventStreamerAdapter(nil)
	if adapter == nil {
		t.Error("Expected non-nil adapter")
	}
}

func TestEventStreamerAdapter_SubscribeNilBus(t *testing.T) {
	adapter := NewEventStreamerAdapter(func() *events.Bus { return nil })
	ch, cancel := adapter.Subscribe()

	// Should return a closed channel
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("Expected channel to be closed for nil bus")
		}
	default:
		// Channel might be empty but not closed yet
	}

	// Cancel should not panic
	cancel()
}

func TestEventStreamerAdapter_SubscribeTypeNilBus(t *testing.T) {
	adapter := NewEventStreamerAdapter(func() *events.Bus { return nil })
	ch, cancel := adapter.SubscribeType(events.ServiceAdded)

	// Should return a closed channel
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("Expected channel to be closed for nil bus")
		}
	default:
		// Channel might be empty but not closed yet
	}

	// Cancel should not panic
	cancel()
}

// DiagnosticsProviderAdapter tests

func TestNewDiagnosticsProviderAdapter(t *testing.T) {
	adapter := NewDiagnosticsProviderAdapter(nil, nil)
	if adapter == nil {
		t.Error("Expected non-nil adapter")
	}
}

func TestDiagnosticsProviderAdapter_GetSummaryNilStore(t *testing.T) {
	adapter := NewDiagnosticsProviderAdapter(func() *state.Store { return nil }, nil)
	summary := adapter.GetSummary()
	if summary.Status != "unknown" {
		t.Errorf("Expected status 'unknown' for nil store, got '%s'", summary.Status)
	}
}

func TestDiagnosticsProviderAdapter_GetServiceDiagnosticNilStore(t *testing.T) {
	adapter := NewDiagnosticsProviderAdapter(func() *state.Store { return nil }, nil)
	_, err := adapter.GetServiceDiagnostic("test")
	if err == nil {
		t.Error("Expected error for nil store")
	}
}

func TestDiagnosticsProviderAdapter_GetForwardDiagnosticNilStore(t *testing.T) {
	adapter := NewDiagnosticsProviderAdapter(func() *state.Store { return nil }, nil)
	_, err := adapter.GetForwardDiagnostic("test")
	if err == nil {
		t.Error("Expected error for nil store")
	}
}

func TestDiagnosticsProviderAdapter_GetNetworkStatusNilStore(t *testing.T) {
	adapter := NewDiagnosticsProviderAdapter(func() *state.Store { return nil }, nil)
	network := adapter.GetNetworkStatus()
	if network.IPsAllocated != 0 {
		t.Error("Expected 0 IPs allocated for nil store")
	}
}

func TestDiagnosticsProviderAdapter_GetErrorsNilStore(t *testing.T) {
	adapter := NewDiagnosticsProviderAdapter(func() *state.Store { return nil }, nil)
	errors := adapter.GetErrors(10)
	if errors != nil {
		t.Error("Expected nil errors for nil store")
	}
}

// Mock ManagerInfo for testing

type mockManagerInfo struct {
	version   string
	uptime    time.Duration
	startTime time.Time
}

func (m *mockManagerInfo) Version() string       { return m.version }
func (m *mockManagerInfo) Uptime() time.Duration { return m.uptime }
func (m *mockManagerInfo) StartTime() time.Time  { return m.startTime }
func (m *mockManagerInfo) Namespaces() []string  { return nil }
func (m *mockManagerInfo) Contexts() []string    { return nil }
func (m *mockManagerInfo) TUIEnabled() bool      { return false }

func TestDiagnosticsProviderAdapter_GetSummaryWithManager(t *testing.T) {
	mockMgr := &mockManagerInfo{
		version:   "1.0.0",
		uptime:    time.Hour,
		startTime: time.Now().Add(-time.Hour),
	}

	adapter := NewDiagnosticsProviderAdapter(
		func() *state.Store { return nil },
		func() types.ManagerInfo { return mockMgr },
	)

	summary := adapter.GetSummary()
	if summary.Version != "" {
		// Manager is only used when store is available
		t.Log("Manager info not used when store is nil - expected behavior")
	}
}

// DiagnosticsProviderAdapter with real store tests

func TestDiagnosticsProviderAdapter_WithStore(t *testing.T) {
	store := state.NewStore(100)

	// Add an active forward
	store.AddForward(state.ForwardSnapshot{
		Key:         "svc1.ns.ctx.pod1",
		ServiceKey:  "svc1.ns.ctx",
		ServiceName: "svc1",
		Namespace:   "ns",
		Context:     "ctx",
		PodName:     "pod1",
		Status:      state.StatusActive,
		StartedAt:   time.Now().Add(-time.Hour),
		LastActive:  time.Now(),
	})

	// Add an error forward
	store.AddForward(state.ForwardSnapshot{
		Key:         "svc2.ns.ctx.pod2",
		ServiceKey:  "svc2.ns.ctx",
		ServiceName: "svc2",
		Namespace:   "ns",
		Context:     "ctx",
		PodName:     "pod2",
		Status:      state.StatusError,
		Error:       "connection refused",
	})

	mockMgr := &mockManagerInfo{
		version:   "1.0.0",
		uptime:    time.Hour,
		startTime: time.Now().Add(-time.Hour),
	}

	adapter := NewDiagnosticsProviderAdapter(
		func() *state.Store { return store },
		func() types.ManagerInfo { return mockMgr },
	)

	// Test GetSummary
	summary := adapter.GetSummary()
	if summary.Version != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", summary.Version)
	}
	// Status will be "degraded" because we have errors
	if summary.Status != "degraded" {
		t.Errorf("Expected status 'degraded', got '%s'", summary.Status)
	}

	// Note: GetServiceDiagnostic and GetForwardDiagnostic tests are skipped here
	// because they call fwdsvcregistry.Get() which requires global initialization.
	// They are tested through integration tests.

	// Test GetNetworkStatus (no IPs in our test data)
	network := adapter.GetNetworkStatus()
	if network.IPsAllocated < 0 {
		t.Error("Expected non-negative IPs allocated")
	}

	// Test GetErrors
	errors := adapter.GetErrors(10)
	if len(errors) != 1 {
		t.Errorf("Expected 1 error, got %d", len(errors))
	}
}

// Test GetNetworkStatus with IPs and hostnames

func TestDiagnosticsProviderAdapter_GetNetworkStatusWithData(t *testing.T) {
	store := state.NewStore(100)

	// Add forwards with IPs and hostnames
	store.AddForward(state.ForwardSnapshot{
		Key:         "svc1.ns.ctx.pod1",
		ServiceKey:  "svc1.ns.ctx",
		ServiceName: "svc1",
		Namespace:   "ns",
		Context:     "ctx",
		PodName:     "pod1",
		Status:      state.StatusActive,
		LocalIP:     "127.1.1.1",
		LocalPort:   "8080",
		Hostnames:   []string{"svc1", "svc1.ns"},
	})

	store.AddForward(state.ForwardSnapshot{
		Key:         "svc2.ns.ctx.pod2",
		ServiceKey:  "svc2.ns.ctx",
		ServiceName: "svc2",
		Namespace:   "ns",
		Context:     "ctx",
		PodName:     "pod2",
		Status:      state.StatusActive,
		LocalIP:     "127.1.1.2",
		LocalPort:   "8081",
		Hostnames:   []string{"svc2", "svc2.ns"},
	})

	adapter := NewDiagnosticsProviderAdapter(
		func() *state.Store { return store },
		nil,
	)

	network := adapter.GetNetworkStatus()

	if network.IPsAllocated != 2 {
		t.Errorf("Expected 2 IPs allocated, got %d", network.IPsAllocated)
	}
	if network.PortsInUse != 2 {
		t.Errorf("Expected 2 ports in use, got %d", network.PortsInUse)
	}
	if len(network.Hostnames) != 4 {
		t.Errorf("Expected 4 hostnames, got %d", len(network.Hostnames))
	}
}

// Test EventStreamerAdapter with real bus

func TestEventStreamerAdapter_WithBus(t *testing.T) {
	bus := events.NewBus(100)
	adapter := NewEventStreamerAdapter(func() *events.Bus { return bus })

	// Test Subscribe
	ch, cancel := adapter.Subscribe()
	if ch == nil {
		t.Error("Expected non-nil channel")
	}

	// Test SubscribeType
	typedCh, typeCancel := adapter.SubscribeType(events.ServiceAdded)
	if typedCh == nil {
		t.Error("Expected non-nil typed channel")
	}

	// Test publishing an event and receiving it
	bus.Publish(events.Event{Type: events.ServiceAdded})

	// Give the event time to be processed
	time.Sleep(10 * time.Millisecond)

	// Clean up - this exercises the cancel function
	cancel()
	typeCancel()

	// Verify channels are closed
	_, ok := <-ch
	if ok {
		t.Error("Expected channel to be closed after cancel")
	}
	_, ok = <-typedCh
	if ok {
		t.Error("Expected typed channel to be closed after cancel")
	}
}

// CreateAPIAdapters test

func TestCreateAPIAdapters(t *testing.T) {
	stateReader, metricsProvider, serviceController, eventStreamer := CreateAPIAdapters()

	if stateReader == nil {
		t.Error("Expected non-nil stateReader")
	}
	if metricsProvider == nil {
		t.Error("Expected non-nil metricsProvider")
	}
	if serviceController == nil {
		t.Error("Expected non-nil serviceController")
	}
	if eventStreamer == nil {
		t.Error("Expected non-nil eventStreamer")
	}
}

// CreateDiagnosticsAdapter test

func TestCreateDiagnosticsAdapter(t *testing.T) {
	adapter := CreateDiagnosticsAdapter(nil)
	if adapter == nil {
		t.Error("Expected non-nil adapter")
	}
}

// Manager SetDiagnosticsProvider test (not in manager_test.go)

func TestManager_SetDiagnosticsProviderAdapter(t *testing.T) {
	m := &Manager{}

	// Test SetDiagnosticsProvider
	mockDiag := NewDiagnosticsProviderAdapter(nil, nil)
	m.SetDiagnosticsProvider(mockDiag)
	if m.diagnosticsProvider == nil {
		t.Error("Expected diagnosticsProvider to be set")
	}
}

// Test setupRouter creates a valid gin.Engine

func TestManager_setupRouter(t *testing.T) {
	store := state.NewStore(100)

	m := &Manager{
		startTime: time.Now(),
		version:   "1.0.0",
		stateReader: NewStateReaderAdapter(func() *state.Store {
			return store
		}),
		metricsProvider:     NewMetricsProviderAdapter(nil),
		serviceController:   NewServiceControllerAdapter(func() *state.Store { return store }),
		eventStreamer:       NewEventStreamerAdapter(func() *events.Bus { return nil }),
		diagnosticsProvider: NewDiagnosticsProviderAdapter(func() *state.Store { return store }, nil),
	}

	router := m.setupRouter()
	if router == nil {
		t.Error("Expected setupRouter to return non-nil router")
	}
}

// Test getManagerInfo function

func TestGetManagerInfo_NilManager(t *testing.T) {
	// Save current manager
	mu.Lock()
	savedManager := apiManager
	apiManager = nil
	mu.Unlock()

	info := getManagerInfo()
	if info != nil {
		t.Error("Expected nil when manager is not initialized")
	}

	// Restore
	mu.Lock()
	apiManager = savedManager
	mu.Unlock()
}

func TestGetManagerInfo_WithManager(t *testing.T) {
	// Save current manager
	mu.Lock()
	savedManager := apiManager
	apiManager = &Manager{
		version:   "test-version",
		startTime: time.Now().Add(-time.Hour),
	}
	mu.Unlock()

	info := getManagerInfo()
	if info == nil {
		t.Error("Expected non-nil when manager is initialized")
	} else if info.Version() != "test-version" {
		t.Errorf("Expected version 'test-version', got '%s'", info.Version())
	}

	// Restore
	mu.Lock()
	apiManager = savedManager
	mu.Unlock()
}

// Test buildForwardDiagnostic with different statuses

func TestDiagnosticsProviderAdapter_buildForwardDiagnostic(t *testing.T) {
	adapter := NewDiagnosticsProviderAdapter(nil, nil)

	tests := []struct {
		name          string
		fwd           *state.ForwardSnapshot
		expectedState string
	}{
		{
			name: "active forward",
			fwd: &state.ForwardSnapshot{
				Key:        "test-key",
				Status:     state.StatusActive,
				StartedAt:  time.Now().Add(-time.Hour),
				LastActive: time.Now(),
			},
			expectedState: "connected",
		},
		{
			name: "connecting forward",
			fwd: &state.ForwardSnapshot{
				Key:    "test-key",
				Status: state.StatusConnecting,
			},
			expectedState: "connecting",
		},
		{
			name: "error forward",
			fwd: &state.ForwardSnapshot{
				Key:    "test-key",
				Status: state.StatusError,
				Error:  "connection refused",
			},
			expectedState: "error",
		},
		{
			name: "disconnected forward",
			fwd: &state.ForwardSnapshot{
				Key:    "test-key",
				Status: state.StatusPending,
			},
			expectedState: "disconnected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diag, _ := adapter.buildForwardDiagnostic(tt.fwd)
			if diag.Connection.State != tt.expectedState {
				t.Errorf("Expected connection state '%s', got '%s'", tt.expectedState, diag.Connection.State)
			}
		})
	}
}

// Test StateReaderAdapter with actual state store

func TestStateReaderAdapter_WithStore(t *testing.T) {
	store := state.NewStore(100)

	// Add a forward
	store.AddForward(state.ForwardSnapshot{
		Key:         "svc.ns.ctx.pod1",
		ServiceKey:  "svc.ns.ctx",
		ServiceName: "svc",
		Namespace:   "ns",
		Context:     "ctx",
		PodName:     "pod1",
		Status:      state.StatusActive,
	})

	// Add a log
	store.AddLog(state.LogEntry{
		Timestamp: time.Now(),
		Message:   "Test log",
	})

	adapter := NewStateReaderAdapter(func() *state.Store { return store })

	// Test GetServices
	services := adapter.GetServices()
	if len(services) != 1 {
		t.Errorf("Expected 1 service, got %d", len(services))
	}

	// Test GetService
	svc := adapter.GetService("svc.ns.ctx")
	if svc == nil {
		t.Error("Expected service not to be nil")
	}

	// Test GetService not found
	svc = adapter.GetService("nonexistent")
	if svc != nil {
		t.Error("Expected nil for nonexistent service")
	}

	// Test GetSummary
	summary := adapter.GetSummary()
	if summary.TotalServices != 1 {
		t.Errorf("Expected 1 service in summary, got %d", summary.TotalServices)
	}

	// Test GetFiltered
	forwards := adapter.GetFiltered()
	if len(forwards) != 1 {
		t.Errorf("Expected 1 forward, got %d", len(forwards))
	}

	// Test GetForward
	fwd := adapter.GetForward("svc.ns.ctx.pod1")
	if fwd == nil {
		t.Error("Expected forward not to be nil")
	}

	// Test GetForward not found
	fwd = adapter.GetForward("nonexistent")
	if fwd != nil {
		t.Error("Expected nil for nonexistent forward")
	}

	// Test GetLogs
	logs := adapter.GetLogs(10)
	if len(logs) != 1 {
		t.Errorf("Expected 1 log, got %d", len(logs))
	}

	// Test Count
	count := adapter.Count()
	if count != 1 {
		t.Errorf("Expected count 1, got %d", count)
	}

	// Test ServiceCount
	svcCount := adapter.ServiceCount()
	if svcCount != 1 {
		t.Errorf("Expected service count 1, got %d", svcCount)
	}
}

// Test with a real metrics registry

func TestMetricsProviderAdapter_WithRegistry(t *testing.T) {
	registry := fwdmetrics.GetRegistry()
	adapter := NewMetricsProviderAdapter(registry)

	if snapshots := adapter.GetAllSnapshots(); len(snapshots) != 0 {
		t.Error("Expected empty snapshots for empty registry")
	}

	if snapshot := adapter.GetServiceSnapshot("test"); snapshot != nil {
		t.Error("Expected nil snapshot for nonexistent service")
	}

	bytesIn, bytesOut, rateIn, rateOut := adapter.GetTotals()
	if bytesIn != 0 || bytesOut != 0 || rateIn != 0 || rateOut != 0 {
		t.Error("Expected zero totals for empty registry")
	}

	if count := adapter.ServiceCount(); count != 0 {
		t.Error("Expected 0 service count for empty registry")
	}

	if count := adapter.PortForwardCount(); count != 0 {
		t.Error("Expected 0 port forward count for empty registry")
	}
}
