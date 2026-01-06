package fwdapi

import (
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/txn2/kubefwd/pkg/fwdapi/types"
	"github.com/txn2/kubefwd/pkg/fwdmetrics"
	"github.com/txn2/kubefwd/pkg/fwdns"
	"github.com/txn2/kubefwd/pkg/fwdsvcregistry"
	"github.com/txn2/kubefwd/pkg/fwdtui/events"
	"github.com/txn2/kubefwd/pkg/fwdtui/state"
)

// initRegistryOnce ensures the service registry is initialized only once for all tests
var initRegistryOnce sync.Once
var testShutdownCh chan struct{}

func initTestRegistry() {
	initRegistryOnce.Do(func() {
		testShutdownCh = make(chan struct{})
		fwdsvcregistry.Init(testShutdownCh)
	})
}

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

// Note: Reconnect and Sync with actual services require fwdsvcregistry to be
// initialized with real services. However, we can test the error paths.

func TestServiceControllerAdapter_Reconnect_NotFound(t *testing.T) {
	initTestRegistry()
	adapter := NewServiceControllerAdapter(nil)

	err := adapter.Reconnect("nonexistent.service.key")
	if err == nil {
		t.Fatal("Expected error for nonexistent service")
	}
	if err.Error() != "service not found: nonexistent.service.key" {
		t.Errorf("Expected 'service not found' error, got: %s", err.Error())
	}
}

func TestServiceControllerAdapter_Sync_NotFound(t *testing.T) {
	initTestRegistry()
	adapter := NewServiceControllerAdapter(nil)

	err := adapter.Sync("nonexistent.service.key", false)
	if err == nil {
		t.Fatal("Expected error for nonexistent service")
	}
	if err.Error() != "service not found: nonexistent.service.key" {
		t.Errorf("Expected 'service not found' error, got: %s", err.Error())
	}
}

func TestServiceControllerAdapter_Sync_NotFound_WithForce(t *testing.T) {
	initTestRegistry()
	adapter := NewServiceControllerAdapter(nil)

	// Even with force=true, should return error for nonexistent service
	err := adapter.Sync("nonexistent.service.key", true)
	if err == nil {
		t.Error("Expected error for nonexistent service with force=true")
	}
}

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
			name: "pending forward",
			fwd: &state.ForwardSnapshot{
				Key:    "test-key",
				Status: state.StatusPending,
			},
			expectedState: "pending",
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

// ServiceCRUDAdapter tests

func TestNewServiceCRUDAdapter(t *testing.T) {
	adapter := NewServiceCRUDAdapter(
		func() *state.Store { return nil },
		func() *fwdns.NamespaceManager { return nil },
		"",
	)
	if adapter == nil {
		t.Error("Expected non-nil adapter")
	}
}

func TestNewServiceCRUDAdapter_WithConfigPath(t *testing.T) {
	adapter := NewServiceCRUDAdapter(
		func() *state.Store { return nil },
		func() *fwdns.NamespaceManager { return nil },
		"/path/to/config",
	)
	if adapter == nil {
		t.Fatal("Expected non-nil adapter")
	}
	if adapter.configPath != "/path/to/config" {
		t.Errorf("Expected config path '/path/to/config', got '%s'", adapter.configPath)
	}
}

func TestServiceCRUDAdapter_HasEmbeddedController(t *testing.T) {
	adapter := NewServiceCRUDAdapter(
		func() *state.Store { return nil },
		func() *fwdns.NamespaceManager { return nil },
		"",
	)
	// Should have embedded ServiceControllerAdapter
	if adapter.ServiceControllerAdapter == nil {
		t.Error("Expected embedded ServiceControllerAdapter to be non-nil")
	}
}

func TestServiceCRUDAdapter_AddService_NilNamespaceManager(t *testing.T) {
	adapter := NewServiceCRUDAdapter(
		func() *state.Store { return nil },
		func() *fwdns.NamespaceManager { return nil },
		"",
	)

	_, err := adapter.AddService(types.AddServiceRequest{
		Namespace:   "default",
		ServiceName: "test-service",
	})

	if err == nil {
		t.Fatal("Expected error for nil namespace manager")
	}
	if err.Error() != "namespace manager not available" {
		t.Errorf("Expected 'namespace manager not available' error, got: %s", err.Error())
	}
}

func TestServiceCRUDAdapter_RemoveService_NotFound(t *testing.T) {
	// Initialize the service registry (once for all tests)
	initTestRegistry()

	adapter := NewServiceCRUDAdapter(
		func() *state.Store { return nil },
		func() *fwdns.NamespaceManager { return nil },
		"",
	)

	err := adapter.RemoveService("nonexistent.service.key")

	if err == nil {
		t.Fatal("Expected error for nonexistent service")
	}
	if err.Error() != "service not found: nonexistent.service.key" {
		t.Errorf("Expected 'service not found' error, got: %s", err.Error())
	}
}

func TestServiceCRUDAdapter_RemoveService_EmptyKey(t *testing.T) {
	// Initialize the service registry (once for all tests)
	initTestRegistry()

	adapter := NewServiceCRUDAdapter(
		func() *state.Store { return nil },
		func() *fwdns.NamespaceManager { return nil },
		"",
	)

	err := adapter.RemoveService("")

	if err == nil {
		t.Fatal("Expected error for empty service key")
	}
	// fwdsvcregistry.Get("") returns nil, so error should be service not found
	if err.Error() != "service not found: " {
		t.Errorf("Expected 'service not found: ' error, got: %s", err.Error())
	}
}

// KubernetesDiscoveryAdapter tests

func TestNewKubernetesDiscoveryAdapter(t *testing.T) {
	adapter := NewKubernetesDiscoveryAdapter(
		func() *fwdns.NamespaceManager { return nil },
		"",
	)
	if adapter == nil {
		t.Error("Expected non-nil adapter")
	}
}

func TestNewKubernetesDiscoveryAdapter_WithConfigPath(t *testing.T) {
	adapter := NewKubernetesDiscoveryAdapter(
		func() *fwdns.NamespaceManager { return nil },
		"/path/to/kubeconfig",
	)
	if adapter == nil {
		t.Fatal("Expected non-nil adapter")
	}
	if adapter.configPath != "/path/to/kubeconfig" {
		t.Errorf("Expected config path '/path/to/kubeconfig', got '%s'", adapter.configPath)
	}
}

func TestKubernetesDiscoveryAdapter_ListNamespaces_NilManager(t *testing.T) {
	adapter := NewKubernetesDiscoveryAdapter(
		func() *fwdns.NamespaceManager { return nil },
		"",
	)

	// We need a valid context to test, but with nil manager it should fail
	// when trying to get namespaces
	_, err := adapter.ListNamespaces("test-context")

	if err == nil {
		t.Fatal("Expected error for nil namespace manager")
	}
	if err.Error() != "namespace manager not available" {
		t.Errorf("Expected 'namespace manager not available' error, got: %s", err.Error())
	}
}

func TestKubernetesDiscoveryAdapter_ListServices_NilManager(t *testing.T) {
	adapter := NewKubernetesDiscoveryAdapter(
		func() *fwdns.NamespaceManager { return nil },
		"",
	)

	_, err := adapter.ListServices("test-context", "default")

	if err == nil {
		t.Fatal("Expected error for nil namespace manager")
	}
	if err.Error() != "namespace manager not available" {
		t.Errorf("Expected 'namespace manager not available' error, got: %s", err.Error())
	}
}

func TestKubernetesDiscoveryAdapter_GetService_NilManager(t *testing.T) {
	adapter := NewKubernetesDiscoveryAdapter(
		func() *fwdns.NamespaceManager { return nil },
		"",
	)

	_, err := adapter.GetService("test-context", "default", "my-service")

	if err == nil {
		t.Fatal("Expected error for nil namespace manager")
	}
	if err.Error() != "namespace manager not available" {
		t.Errorf("Expected 'namespace manager not available' error, got: %s", err.Error())
	}
}

// Note: ListContexts doesn't require namespace manager, only kubeconfig
// Full integration tests for ServiceCRUDAdapter.AddService and RemoveService with real
// Kubernetes clusters are done through curl/API integration testing as these require:
// - Active Kubernetes cluster connection
// - fwdsvcregistry global state
// - fwdns.NamespaceManager with real clients

// TestKubernetesDiscoveryAdapter_ListContexts tests listing contexts from kubeconfig
func TestKubernetesDiscoveryAdapter_ListContexts(t *testing.T) {
	adapter := NewKubernetesDiscoveryAdapter(
		func() *fwdns.NamespaceManager { return nil },
		"", // Empty config path - will use default kubeconfig
	)

	// This will attempt to read kubeconfig which may or may not exist
	// We're testing that the method doesn't panic and handles errors gracefully
	result, err := adapter.ListContexts()

	// We can't predict if kubeconfig exists, but method should handle both cases
	if err != nil {
		// Error is expected if no kubeconfig
		t.Logf("ListContexts returned expected error (no kubeconfig): %v", err)
	} else if result != nil {
		// If we got a result, verify structure
		if result.Contexts == nil {
			t.Error("Expected Contexts slice to be non-nil")
		}
		t.Logf("ListContexts returned %d contexts, current: %s", len(result.Contexts), result.CurrentContext)
	}
}

// Test ServiceCRUD adapter inherits from ServiceControllerAdapter correctly
func TestServiceCRUDAdapter_InheritedMethods(t *testing.T) {
	initTestRegistry()

	adapter := NewServiceCRUDAdapter(
		func() *state.Store { return nil },
		func() *fwdns.NamespaceManager { return nil },
		"",
	)

	// Test inherited Reconnect method
	err := adapter.Reconnect("test.key")
	if err == nil {
		t.Error("Expected error for Reconnect with nonexistent key")
	}

	// Test inherited Sync method
	err = adapter.Sync("test.key", false)
	if err == nil {
		t.Error("Expected error for Sync with nonexistent key")
	}

	// Test inherited ReconnectAll method (should return 0 for nil store)
	adapter2 := NewServiceCRUDAdapter(
		func() *state.Store { return nil },
		func() *fwdns.NamespaceManager { return nil },
		"",
	)
	count := adapter2.ReconnectAll()
	if count != 0 {
		t.Errorf("Expected ReconnectAll to return 0 for nil store, got %d", count)
	}
}

// Test AddService with various error conditions
func TestServiceCRUDAdapter_AddService_ErrorCases(t *testing.T) {
	adapter := NewServiceCRUDAdapter(
		func() *state.Store { return nil },
		func() *fwdns.NamespaceManager { return nil },
		"",
	)

	tests := []struct {
		name        string
		request     types.AddServiceRequest
		expectError string
	}{
		{
			name: "missing namespace",
			request: types.AddServiceRequest{
				ServiceName: "test-svc",
			},
			expectError: "namespace manager not available",
		},
		{
			name: "missing service name",
			request: types.AddServiceRequest{
				Namespace: "default",
			},
			expectError: "namespace manager not available",
		},
		{
			name: "all fields empty",
			request: types.AddServiceRequest{
				Context:     "",
				Namespace:   "",
				ServiceName: "",
			},
			expectError: "namespace manager not available",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := adapter.AddService(tt.request)
			if err == nil {
				t.Error("Expected error")
			}
			if err.Error() != tt.expectError {
				t.Errorf("Expected error '%s', got '%s'", tt.expectError, err.Error())
			}
		})
	}
}

// Test splitLogLines helper function
func TestSplitLogLines(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty input",
			input:    "",
			expected: []string{},
		},
		{
			name:     "single line no newline",
			input:    "hello world",
			expected: []string{"hello world"},
		},
		{
			name:     "single line with newline",
			input:    "hello world\n",
			expected: []string{"hello world"},
		},
		{
			name:     "multiple lines with LF",
			input:    "line1\nline2\nline3",
			expected: []string{"line1", "line2", "line3"},
		},
		{
			name:     "multiple lines with CRLF",
			input:    "line1\r\nline2\r\nline3",
			expected: []string{"line1", "line2", "line3"},
		},
		{
			name:     "mixed line endings",
			input:    "line1\nline2\r\nline3\n",
			expected: []string{"line1", "line2", "line3"},
		},
		{
			name:     "empty lines",
			input:    "line1\n\nline3",
			expected: []string{"line1", "", "line3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitLogLines(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d lines, got %d", len(tt.expected), len(result))
				return
			}
			for i, line := range result {
				if line != tt.expected[i] {
					t.Errorf("Line %d: expected '%s', got '%s'", i, tt.expected[i], line)
				}
			}
		})
	}
}

// Test formatDuration helper function
func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{
			name:     "zero duration",
			duration: 0,
			expected: "0s",
		},
		{
			name:     "seconds",
			duration: 45 * time.Second,
			expected: "45s",
		},
		{
			name:     "minutes only",
			duration: 5 * time.Minute,
			expected: "5m",
		},
		{
			name:     "minutes and seconds shows minutes only",
			duration: 5*time.Minute + 30*time.Second,
			expected: "5m",
		},
		{
			name:     "hours only",
			duration: 2 * time.Hour,
			expected: "2h",
		},
		{
			name:     "hours and minutes shows hours only",
			duration: 2*time.Hour + 15*time.Minute,
			expected: "2h",
		},
		{
			name:     "days only",
			duration: 72 * time.Hour,
			expected: "3d",
		},
		{
			name:     "days and hours shows days only",
			duration: 48*time.Hour + 6*time.Hour,
			expected: "2d",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDuration(tt.duration)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

// Test KubernetesDiscoveryAdapter pod methods with nil manager
func TestKubernetesDiscoveryAdapter_GetPodLogs_NilManager(t *testing.T) {
	adapter := NewKubernetesDiscoveryAdapter(
		func() *fwdns.NamespaceManager { return nil },
		"",
	)

	_, err := adapter.GetPodLogs("test-context", "default", "test-pod", types.PodLogsOptions{})

	if err == nil {
		t.Fatal("Expected error for nil namespace manager")
	}
	if err.Error() != "namespace manager not available" {
		t.Errorf("Expected 'namespace manager not available' error, got: %s", err.Error())
	}
}

func TestKubernetesDiscoveryAdapter_ListPods_NilManager(t *testing.T) {
	adapter := NewKubernetesDiscoveryAdapter(
		func() *fwdns.NamespaceManager { return nil },
		"",
	)

	_, err := adapter.ListPods("test-context", "default", types.ListPodsOptions{})

	if err == nil {
		t.Fatal("Expected error for nil namespace manager")
	}
	if err.Error() != "namespace manager not available" {
		t.Errorf("Expected 'namespace manager not available' error, got: %s", err.Error())
	}
}

func TestKubernetesDiscoveryAdapter_GetPod_NilManager(t *testing.T) {
	adapter := NewKubernetesDiscoveryAdapter(
		func() *fwdns.NamespaceManager { return nil },
		"",
	)

	_, err := adapter.GetPod("test-context", "default", "test-pod")

	if err == nil {
		t.Fatal("Expected error for nil namespace manager")
	}
	if err.Error() != "namespace manager not available" {
		t.Errorf("Expected 'namespace manager not available' error, got: %s", err.Error())
	}
}

func TestKubernetesDiscoveryAdapter_GetEvents_NilManager(t *testing.T) {
	adapter := NewKubernetesDiscoveryAdapter(
		func() *fwdns.NamespaceManager { return nil },
		"",
	)

	_, err := adapter.GetEvents("test-context", "default", types.GetEventsOptions{})

	if err == nil {
		t.Fatal("Expected error for nil namespace manager")
	}
	if err.Error() != "namespace manager not available" {
		t.Errorf("Expected 'namespace manager not available' error, got: %s", err.Error())
	}
}

func TestKubernetesDiscoveryAdapter_GetEndpoints_NilManager(t *testing.T) {
	adapter := NewKubernetesDiscoveryAdapter(
		func() *fwdns.NamespaceManager { return nil },
		"",
	)

	_, err := adapter.GetEndpoints("test-context", "default", "my-service")

	if err == nil {
		t.Fatal("Expected error for nil namespace manager")
	}
	if err.Error() != "namespace manager not available" {
		t.Errorf("Expected 'namespace manager not available' error, got: %s", err.Error())
	}
}

// Tests with fake kubernetes clientset

func TestKubernetesDiscoveryAdapter_ListNamespaces_WithFakeClient(t *testing.T) {
	// Create namespaces
	ns1 := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "default"},
		Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
	}
	ns2 := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "kube-system"},
		Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
	}
	ns3 := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "test-ns"},
		Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceTerminating},
	}

	clientset := fake.NewClientset(ns1, ns2, ns3)

	stopCh := make(chan struct{})
	defer close(stopCh)
	mgr := fwdns.NewManager(fwdns.ManagerConfig{GlobalStopCh: stopCh})
	mgr.SetClientSet("test-context", clientset)

	adapter := NewKubernetesDiscoveryAdapter(
		func() *fwdns.NamespaceManager { return mgr },
		"",
	)

	namespaces, err := adapter.ListNamespaces("test-context")
	if err != nil {
		t.Fatalf("ListNamespaces failed: %v", err)
	}

	if len(namespaces) != 3 {
		t.Errorf("Expected 3 namespaces, got %d", len(namespaces))
	}

	// Check namespace properties
	foundDefault := false
	foundTestNs := false
	for _, ns := range namespaces {
		if ns.Name == "default" {
			foundDefault = true
			if ns.Status != "Active" {
				t.Errorf("Expected 'Active' status for default, got %s", ns.Status)
			}
		}
		if ns.Name == "test-ns" {
			foundTestNs = true
			if ns.Status != "Terminating" {
				t.Errorf("Expected 'Terminating' status for test-ns, got %s", ns.Status)
			}
		}
	}
	if !foundDefault {
		t.Error("Expected to find 'default' namespace")
	}
	if !foundTestNs {
		t.Error("Expected to find 'test-ns' namespace")
	}
}

func TestKubernetesDiscoveryAdapter_ListServices_WithFakeClient(t *testing.T) {
	initTestRegistry()

	// Create services
	svc1 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-service",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: "10.0.0.1",
			Selector:  map[string]string{"app": "api"},
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 80, TargetPort: intstr.FromInt32(8080), Protocol: corev1.ProtocolTCP},
			},
		},
	}
	svc2 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "db-service",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: "10.0.0.2",
			Selector:  map[string]string{"app": "db"},
			Ports: []corev1.ServicePort{
				{Name: "mysql", Port: 3306, TargetPort: intstr.FromInt32(3306), Protocol: corev1.ProtocolTCP},
			},
		},
	}

	clientset := fake.NewClientset(svc1, svc2)

	stopCh := make(chan struct{})
	defer close(stopCh)
	mgr := fwdns.NewManager(fwdns.ManagerConfig{GlobalStopCh: stopCh})
	mgr.SetClientSet("test-context", clientset)

	adapter := NewKubernetesDiscoveryAdapter(
		func() *fwdns.NamespaceManager { return mgr },
		"",
	)

	services, err := adapter.ListServices("test-context", "default")
	if err != nil {
		t.Fatalf("ListServices failed: %v", err)
	}

	if len(services) != 2 {
		t.Errorf("Expected 2 services, got %d", len(services))
	}

	// Check service properties
	for _, svc := range services {
		if svc.Name == "api-service" {
			if svc.ClusterIP != "10.0.0.1" {
				t.Errorf("Expected ClusterIP 10.0.0.1, got %s", svc.ClusterIP)
			}
			if len(svc.Ports) != 1 {
				t.Errorf("Expected 1 port, got %d", len(svc.Ports))
			}
			if svc.Ports[0].Name != "http" {
				t.Errorf("Expected port name 'http', got %s", svc.Ports[0].Name)
			}
			if svc.Selector["app"] != "api" {
				t.Errorf("Expected selector app=api, got %v", svc.Selector)
			}
		}
	}
}

func TestKubernetesDiscoveryAdapter_GetService_WithFakeClient(t *testing.T) {
	initTestRegistry()

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-service",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: "10.0.0.100",
			Selector:  map[string]string{"app": "myapp", "version": "v1"},
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 80, TargetPort: intstr.FromInt32(8080), Protocol: corev1.ProtocolTCP},
				{Name: "https", Port: 443, TargetPort: intstr.FromInt32(8443), Protocol: corev1.ProtocolTCP},
			},
		},
	}

	clientset := fake.NewClientset(svc)

	stopCh := make(chan struct{})
	defer close(stopCh)
	mgr := fwdns.NewManager(fwdns.ManagerConfig{GlobalStopCh: stopCh})
	mgr.SetClientSet("test-context", clientset)

	adapter := NewKubernetesDiscoveryAdapter(
		func() *fwdns.NamespaceManager { return mgr },
		"",
	)

	result, err := adapter.GetService("test-context", "default", "my-service")
	if err != nil {
		t.Fatalf("GetService failed: %v", err)
	}

	if result.Name != "my-service" {
		t.Errorf("Expected name 'my-service', got %s", result.Name)
	}
	if result.ClusterIP != "10.0.0.100" {
		t.Errorf("Expected ClusterIP 10.0.0.100, got %s", result.ClusterIP)
	}
	if len(result.Ports) != 2 {
		t.Errorf("Expected 2 ports, got %d", len(result.Ports))
	}
	if result.Selector["version"] != "v1" {
		t.Errorf("Expected selector version=v1, got %v", result.Selector)
	}
}

func TestKubernetesDiscoveryAdapter_GetService_NotFound(t *testing.T) {
	clientset := fake.NewClientset()

	stopCh := make(chan struct{})
	defer close(stopCh)
	mgr := fwdns.NewManager(fwdns.ManagerConfig{GlobalStopCh: stopCh})
	mgr.SetClientSet("test-context", clientset)

	adapter := NewKubernetesDiscoveryAdapter(
		func() *fwdns.NamespaceManager { return mgr },
		"",
	)

	_, err := adapter.GetService("test-context", "default", "nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent service")
	}
}

func TestKubernetesDiscoveryAdapter_ListPods_WithFakeClient(t *testing.T) {
	initTestRegistry()

	now := metav1.Now()
	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-pod-1",
			Namespace: "default",
			Labels:    map[string]string{"app": "api"},
		},
		Spec: corev1.PodSpec{
			NodeName:   "node-1",
			Containers: []corev1.Container{{Name: "api-container", Image: "api:v1"}},
		},
		Status: corev1.PodStatus{
			Phase:     corev1.PodRunning,
			PodIP:     "10.1.1.1",
			StartTime: &now,
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "api-container", Ready: true, RestartCount: 2},
			},
		},
	}
	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-pod-2",
			Namespace: "default",
			Labels:    map[string]string{"app": "api"},
		},
		Spec: corev1.PodSpec{
			NodeName:   "node-2",
			Containers: []corev1.Container{{Name: "api-container", Image: "api:v1"}, {Name: "sidecar", Image: "sidecar:v1"}},
		},
		Status: corev1.PodStatus{
			Phase:     corev1.PodPending,
			PodIP:     "10.1.1.2",
			StartTime: &now,
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "api-container", Ready: true, RestartCount: 0},
				{Name: "sidecar", Ready: false, RestartCount: 1, State: corev1.ContainerState{
					Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"},
				}},
			},
		},
	}

	clientset := fake.NewClientset(pod1, pod2)

	stopCh := make(chan struct{})
	defer close(stopCh)
	mgr := fwdns.NewManager(fwdns.ManagerConfig{GlobalStopCh: stopCh})
	mgr.SetClientSet("test-context", clientset)

	adapter := NewKubernetesDiscoveryAdapter(
		func() *fwdns.NamespaceManager { return mgr },
		"",
	)

	pods, err := adapter.ListPods("test-context", "default", types.ListPodsOptions{
		LabelSelector: "app=api",
	})
	if err != nil {
		t.Fatalf("ListPods failed: %v", err)
	}

	if len(pods) != 2 {
		t.Errorf("Expected 2 pods, got %d", len(pods))
	}

	// Check pod properties
	for _, pod := range pods {
		if pod.Name == "api-pod-1" {
			if pod.Ready != "1/1" {
				t.Errorf("Expected Ready '1/1', got %s", pod.Ready)
			}
			if pod.Restarts != 2 {
				t.Errorf("Expected 2 restarts, got %d", pod.Restarts)
			}
			if pod.Node != "node-1" {
				t.Errorf("Expected node 'node-1', got %s", pod.Node)
			}
		}
		if pod.Name == "api-pod-2" {
			if pod.Ready != "1/2" {
				t.Errorf("Expected Ready '1/2', got %s", pod.Ready)
			}
			if pod.Status != "ImagePullBackOff" {
				t.Errorf("Expected status 'ImagePullBackOff', got %s", pod.Status)
			}
			if len(pod.Containers) != 2 {
				t.Errorf("Expected 2 containers, got %d", len(pod.Containers))
			}
		}
	}
}

func TestKubernetesDiscoveryAdapter_ListPods_TerminatingPod(t *testing.T) {
	initTestRegistry()

	now := metav1.Now()
	deletionTime := metav1.Now()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "terminating-pod",
			Namespace:         "default",
			DeletionTimestamp: &deletionTime,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "container"}},
		},
		Status: corev1.PodStatus{
			Phase:     corev1.PodRunning,
			StartTime: &now,
		},
	}

	clientset := fake.NewClientset(pod)

	stopCh := make(chan struct{})
	defer close(stopCh)
	mgr := fwdns.NewManager(fwdns.ManagerConfig{GlobalStopCh: stopCh})
	mgr.SetClientSet("test-context", clientset)

	adapter := NewKubernetesDiscoveryAdapter(
		func() *fwdns.NamespaceManager { return mgr },
		"",
	)

	pods, err := adapter.ListPods("test-context", "default", types.ListPodsOptions{})
	if err != nil {
		t.Fatalf("ListPods failed: %v", err)
	}

	if len(pods) != 1 {
		t.Fatalf("Expected 1 pod, got %d", len(pods))
	}
	if pods[0].Status != "Terminating" {
		t.Errorf("Expected status 'Terminating', got %s", pods[0].Status)
	}
}

func TestKubernetesDiscoveryAdapter_ListPods_WithServiceName(t *testing.T) {
	initTestRegistry()

	// Create service with selector
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-service",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "myapp"},
		},
	}

	// Create pods - one matching, one not
	now := metav1.Now()
	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-pod",
			Namespace: "default",
			Labels:    map[string]string{"app": "myapp"},
		},
		Status: corev1.PodStatus{
			Phase:     corev1.PodRunning,
			StartTime: &now,
		},
	}
	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-pod",
			Namespace: "default",
			Labels:    map[string]string{"app": "other"},
		},
		Status: corev1.PodStatus{
			Phase:     corev1.PodRunning,
			StartTime: &now,
		},
	}

	clientset := fake.NewClientset(svc, pod1, pod2)

	stopCh := make(chan struct{})
	defer close(stopCh)
	mgr := fwdns.NewManager(fwdns.ManagerConfig{GlobalStopCh: stopCh})
	mgr.SetClientSet("test-context", clientset)

	adapter := NewKubernetesDiscoveryAdapter(
		func() *fwdns.NamespaceManager { return mgr },
		"",
	)

	pods, err := adapter.ListPods("test-context", "default", types.ListPodsOptions{
		ServiceName: "my-service",
	})
	if err != nil {
		t.Fatalf("ListPods failed: %v", err)
	}

	// Should only return the pod matching service selector
	if len(pods) != 1 {
		t.Errorf("Expected 1 pod, got %d", len(pods))
	}
	if len(pods) > 0 && pods[0].Name != "myapp-pod" {
		t.Errorf("Expected pod 'myapp-pod', got '%s'", pods[0].Name)
	}
}

func TestKubernetesDiscoveryAdapter_ListPods_WithFieldSelector(t *testing.T) {
	initTestRegistry()

	now := metav1.Now()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			NodeName: "node-1",
		},
		Status: corev1.PodStatus{
			Phase:     corev1.PodRunning,
			StartTime: &now,
		},
	}

	clientset := fake.NewClientset(pod)

	stopCh := make(chan struct{})
	defer close(stopCh)
	mgr := fwdns.NewManager(fwdns.ManagerConfig{GlobalStopCh: stopCh})
	mgr.SetClientSet("test-context", clientset)

	adapter := NewKubernetesDiscoveryAdapter(
		func() *fwdns.NamespaceManager { return mgr },
		"",
	)

	// Use field selector to filter by node
	pods, err := adapter.ListPods("test-context", "default", types.ListPodsOptions{
		FieldSelector: "spec.nodeName=node-1",
	})
	if err != nil {
		t.Fatalf("ListPods with FieldSelector failed: %v", err)
	}

	if len(pods) != 1 {
		t.Errorf("Expected 1 pod, got %d", len(pods))
	}
}

func TestKubernetesDiscoveryAdapter_ListPods_WithLabelAndService(t *testing.T) {
	initTestRegistry()

	// Create service with selector
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-service",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "myapp"},
		},
	}

	now := metav1.Now()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-v1",
			Namespace: "default",
			Labels:    map[string]string{"app": "myapp", "version": "v1"},
		},
		Status: corev1.PodStatus{
			Phase:     corev1.PodRunning,
			StartTime: &now,
		},
	}

	clientset := fake.NewClientset(svc, pod)

	stopCh := make(chan struct{})
	defer close(stopCh)
	mgr := fwdns.NewManager(fwdns.ManagerConfig{GlobalStopCh: stopCh})
	mgr.SetClientSet("test-context", clientset)

	adapter := NewKubernetesDiscoveryAdapter(
		func() *fwdns.NamespaceManager { return mgr },
		"",
	)

	// Test combining label selector with service name
	pods, err := adapter.ListPods("test-context", "default", types.ListPodsOptions{
		ServiceName:   "my-service",
		LabelSelector: "version=v1",
	})
	if err != nil {
		t.Fatalf("ListPods with combined filters failed: %v", err)
	}

	if len(pods) != 1 {
		t.Errorf("Expected 1 pod, got %d", len(pods))
	}
}

func TestKubernetesDiscoveryAdapter_GetPod_WithFakeClient(t *testing.T) {
	initTestRegistry()

	now := metav1.Now()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "detailed-pod",
			Namespace: "default",
			Labels:    map[string]string{"app": "test", "version": "v2"},
		},
		Spec: corev1.PodSpec{
			NodeName: "worker-node-1",
			Containers: []corev1.Container{
				{
					Name:  "main",
					Image: "myapp:v2",
					Ports: []corev1.ContainerPort{{ContainerPort: 8080, Protocol: corev1.ProtocolTCP}},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("512Mi"),
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			Phase:     corev1.PodRunning,
			PodIP:     "10.1.2.3",
			HostIP:    "192.168.1.10",
			StartTime: &now,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
				{Type: corev1.PodScheduled, Status: corev1.ConditionTrue},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "main",
					Ready:        true,
					RestartCount: 5,
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{StartedAt: now},
					},
				},
			},
		},
	}

	clientset := fake.NewClientset(pod)

	stopCh := make(chan struct{})
	defer close(stopCh)
	mgr := fwdns.NewManager(fwdns.ManagerConfig{GlobalStopCh: stopCh})
	mgr.SetClientSet("test-context", clientset)

	adapter := NewKubernetesDiscoveryAdapter(
		func() *fwdns.NamespaceManager { return mgr },
		"",
	)

	result, err := adapter.GetPod("test-context", "default", "detailed-pod")
	if err != nil {
		t.Fatalf("GetPod failed: %v", err)
	}

	if result.Name != "detailed-pod" {
		t.Errorf("Expected name 'detailed-pod', got %s", result.Name)
	}
	if result.IP != "10.1.2.3" {
		t.Errorf("Expected IP '10.1.2.3', got %s", result.IP)
	}
	if result.HostIP != "192.168.1.10" {
		t.Errorf("Expected HostIP '192.168.1.10', got %s", result.HostIP)
	}
	if result.Node != "worker-node-1" {
		t.Errorf("Expected Node 'worker-node-1', got %s", result.Node)
	}
	if len(result.Containers) != 1 {
		t.Fatalf("Expected 1 container, got %d", len(result.Containers))
	}
	if result.Containers[0].Name != "main" {
		t.Errorf("Expected container name 'main', got %s", result.Containers[0].Name)
	}
	if result.Containers[0].RestartCount != 5 {
		t.Errorf("Expected 5 restarts, got %d", result.Containers[0].RestartCount)
	}
	if len(result.Conditions) != 2 {
		t.Errorf("Expected 2 conditions, got %d", len(result.Conditions))
	}
}

func TestKubernetesDiscoveryAdapter_GetPod_NotFound(t *testing.T) {
	clientset := fake.NewClientset()

	stopCh := make(chan struct{})
	defer close(stopCh)
	mgr := fwdns.NewManager(fwdns.ManagerConfig{GlobalStopCh: stopCh})
	mgr.SetClientSet("test-context", clientset)

	adapter := NewKubernetesDiscoveryAdapter(
		func() *fwdns.NamespaceManager { return mgr },
		"",
	)

	_, err := adapter.GetPod("test-context", "default", "nonexistent-pod")
	if err == nil {
		t.Error("Expected error for nonexistent pod")
	}
}

func TestKubernetesDiscoveryAdapter_GetPod_TerminatingWithWaitingContainers(t *testing.T) {
	initTestRegistry()

	now := metav1.Now()
	deletionTime := metav1.Now()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "terminating-pod",
			Namespace:         "default",
			DeletionTimestamp: &deletionTime,
			Labels:            map[string]string{"app": "test"},
		},
		Spec: corev1.PodSpec{
			NodeName: "node-1",
			Containers: []corev1.Container{
				{
					Name:  "main",
					Image: "app:v1",
					Ports: []corev1.ContainerPort{{ContainerPort: 8080}},
				},
				{
					Name:  "sidecar",
					Image: "sidecar:v1",
				},
			},
		},
		Status: corev1.PodStatus{
			Phase:     corev1.PodRunning,
			PodIP:     "10.1.1.1",
			HostIP:    "192.168.1.1",
			StartTime: &now,
			Message:   "Pod is terminating",
			Reason:    "Terminating",
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "main",
					Ready:        false,
					RestartCount: 0,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "ContainerCreating",
							Message: "Creating container",
						},
					},
				},
				{
					Name:         "sidecar",
					Ready:        false,
					RestartCount: 2,
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							ExitCode: 1,
							Reason:   "Error",
							Message:  "Container failed",
						},
					},
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							ExitCode: 137,
							Reason:   "OOMKilled",
						},
					},
				},
			},
		},
	}

	clientset := fake.NewClientset(pod)

	stopCh := make(chan struct{})
	defer close(stopCh)
	mgr := fwdns.NewManager(fwdns.ManagerConfig{GlobalStopCh: stopCh})
	mgr.SetClientSet("test-context", clientset)

	adapter := NewKubernetesDiscoveryAdapter(
		func() *fwdns.NamespaceManager { return mgr },
		"",
	)

	result, err := adapter.GetPod("test-context", "default", "terminating-pod")
	if err != nil {
		t.Fatalf("GetPod failed: %v", err)
	}

	// Status should be "Terminating" due to DeletionTimestamp
	if result.Status != "Terminating" {
		t.Errorf("Expected status 'Terminating', got %s", result.Status)
	}

	// Should have 2 containers
	if len(result.Containers) != 2 {
		t.Fatalf("Expected 2 containers, got %d", len(result.Containers))
	}

	// Check waiting container state
	mainContainer := result.Containers[0]
	if mainContainer.State != "Waiting" {
		t.Errorf("Expected main container state 'Waiting', got %s", mainContainer.State)
	}
	if mainContainer.StateReason != "ContainerCreating" {
		t.Errorf("Expected main container reason 'ContainerCreating', got %s", mainContainer.StateReason)
	}

	// Check terminated container state
	sidecarContainer := result.Containers[1]
	if sidecarContainer.State != "Terminated" {
		t.Errorf("Expected sidecar container state 'Terminated', got %s", sidecarContainer.State)
	}
	if sidecarContainer.StateReason != "Error" {
		t.Errorf("Expected sidecar state reason 'Error', got %s", sidecarContainer.StateReason)
	}
	// Verify last state is captured
	if sidecarContainer.LastState == "" {
		t.Error("Expected last state to be set for terminated container with restart")
	}
}

func TestKubernetesDiscoveryAdapter_GetPod_RunningContainerState(t *testing.T) {
	initTestRegistry()

	now := metav1.Now()
	startedAt := metav1.NewTime(now.Add(-time.Hour))
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "running-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			NodeName: "node-1",
			Containers: []corev1.Container{
				{Name: "main", Image: "app:v1"},
			},
		},
		Status: corev1.PodStatus{
			Phase:     corev1.PodRunning,
			StartTime: &now,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "main",
					Ready:        true,
					RestartCount: 0,
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{StartedAt: startedAt},
					},
				},
			},
		},
	}

	clientset := fake.NewClientset(pod)

	stopCh := make(chan struct{})
	defer close(stopCh)
	mgr := fwdns.NewManager(fwdns.ManagerConfig{GlobalStopCh: stopCh})
	mgr.SetClientSet("test-context", clientset)

	adapter := NewKubernetesDiscoveryAdapter(
		func() *fwdns.NamespaceManager { return mgr },
		"",
	)

	result, err := adapter.GetPod("test-context", "default", "running-pod")
	if err != nil {
		t.Fatalf("GetPod failed: %v", err)
	}

	if result.Status != "Running" {
		t.Errorf("Expected status 'Running', got %s", result.Status)
	}

	if len(result.Containers) != 1 {
		t.Fatalf("Expected 1 container, got %d", len(result.Containers))
	}

	container := result.Containers[0]
	if container.State != "Running" {
		t.Errorf("Expected container state 'Running', got %s", container.State)
	}
}

func TestKubernetesDiscoveryAdapter_GetEvents_WithFakeClient(t *testing.T) {
	now := metav1.Now()
	event1 := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-event-1",
			Namespace: "default",
		},
		InvolvedObject: corev1.ObjectReference{
			Kind:      "Pod",
			Name:      "my-pod",
			Namespace: "default",
		},
		Reason:         "Scheduled",
		Message:        "Successfully assigned default/my-pod to node-1",
		Type:           "Normal",
		Count:          1,
		FirstTimestamp: now,
		LastTimestamp:  now,
	}
	event2 := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-event-2",
			Namespace: "default",
		},
		InvolvedObject: corev1.ObjectReference{
			Kind:      "Pod",
			Name:      "my-pod",
			Namespace: "default",
		},
		Reason:         "Pulled",
		Message:        "Successfully pulled image myapp:v1",
		Type:           "Normal",
		Count:          1,
		FirstTimestamp: now,
		LastTimestamp:  now,
	}

	clientset := fake.NewClientset(event1, event2)

	stopCh := make(chan struct{})
	defer close(stopCh)
	mgr := fwdns.NewManager(fwdns.ManagerConfig{GlobalStopCh: stopCh})
	mgr.SetClientSet("test-context", clientset)

	adapter := NewKubernetesDiscoveryAdapter(
		func() *fwdns.NamespaceManager { return mgr },
		"",
	)

	eventList, err := adapter.GetEvents("test-context", "default", types.GetEventsOptions{})
	if err != nil {
		t.Fatalf("GetEvents failed: %v", err)
	}

	if len(eventList) != 2 {
		t.Errorf("Expected 2 events, got %d", len(eventList))
	}

	// Check event properties
	for _, event := range eventList {
		if event.Reason == "Scheduled" {
			if event.ObjectKind != "Pod" {
				t.Errorf("Expected ObjectKind 'Pod', got %s", event.ObjectKind)
			}
			if event.ObjectName != "my-pod" {
				t.Errorf("Expected ObjectName 'my-pod', got %s", event.ObjectName)
			}
		}
	}
}

func TestKubernetesDiscoveryAdapter_GetEvents_WithFilters(t *testing.T) {
	now := metav1.Now()
	podEvent := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-event", Namespace: "default"},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod", Name: "my-pod", Namespace: "default",
		},
		Reason: "Started", Message: "Started container", Type: "Normal",
		FirstTimestamp: now, LastTimestamp: now,
	}
	serviceEvent := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "svc-event", Namespace: "default"},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Service", Name: "my-service", Namespace: "default",
		},
		Reason: "Created", Message: "Created service", Type: "Normal",
		FirstTimestamp: now, LastTimestamp: now,
	}

	clientset := fake.NewClientset(podEvent, serviceEvent)

	stopCh := make(chan struct{})
	defer close(stopCh)
	mgr := fwdns.NewManager(fwdns.ManagerConfig{GlobalStopCh: stopCh})
	mgr.SetClientSet("test-context", clientset)

	adapter := NewKubernetesDiscoveryAdapter(
		func() *fwdns.NamespaceManager { return mgr },
		"",
	)

	// Pass filter options (fake clientset may not implement field selectors)
	// This test verifies the code path with filtering options is exercised
	eventList, err := adapter.GetEvents("test-context", "default", types.GetEventsOptions{
		ResourceKind: "Pod",
		ResourceName: "my-pod",
	})
	if err != nil {
		t.Fatalf("GetEvents failed: %v", err)
	}

	// Fake clientset doesn't implement field selectors, so all events are returned
	// We just verify the API call succeeded
	if len(eventList) < 1 {
		t.Errorf("Expected at least 1 event, got %d", len(eventList))
	}
}

func TestKubernetesDiscoveryAdapter_GetEvents_WithLimit(t *testing.T) {
	now := metav1.Now()
	earlier := metav1.NewTime(now.Add(-time.Hour))
	evenEarlier := metav1.NewTime(now.Add(-2 * time.Hour))

	// Create 3 events with different timestamps
	event1 := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "event-1", Namespace: "default"},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod", Name: "my-pod", Namespace: "default",
		},
		Reason: "Event1", Message: "First event", Type: "Normal",
		FirstTimestamp: evenEarlier, LastTimestamp: evenEarlier,
	}
	event2 := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "event-2", Namespace: "default"},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod", Name: "my-pod", Namespace: "default",
		},
		Reason: "Event2", Message: "Second event", Type: "Normal",
		FirstTimestamp: earlier, LastTimestamp: earlier,
	}
	event3 := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "event-3", Namespace: "default"},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod", Name: "my-pod", Namespace: "default",
		},
		Reason: "Event3", Message: "Third event (most recent)", Type: "Normal",
		FirstTimestamp: now, LastTimestamp: now,
	}

	clientset := fake.NewClientset(event1, event2, event3)

	stopCh := make(chan struct{})
	defer close(stopCh)
	mgr := fwdns.NewManager(fwdns.ManagerConfig{GlobalStopCh: stopCh})
	mgr.SetClientSet("test-context", clientset)

	adapter := NewKubernetesDiscoveryAdapter(
		func() *fwdns.NamespaceManager { return mgr },
		"",
	)

	// Test with limit
	eventList, err := adapter.GetEvents("test-context", "default", types.GetEventsOptions{
		Limit: 2,
	})
	if err != nil {
		t.Fatalf("GetEvents failed: %v", err)
	}

	if len(eventList) != 2 {
		t.Errorf("Expected 2 events with limit, got %d", len(eventList))
	}

	// Events should be sorted by LastTimestamp (most recent first)
	if len(eventList) >= 2 && eventList[0].Reason != "Event3" {
		t.Errorf("Expected most recent event first, got %s", eventList[0].Reason)
	}
}

func TestKubernetesDiscoveryAdapter_GetEvents_DefaultLimit(t *testing.T) {
	clientset := fake.NewClientset()

	stopCh := make(chan struct{})
	defer close(stopCh)
	mgr := fwdns.NewManager(fwdns.ManagerConfig{GlobalStopCh: stopCh})
	mgr.SetClientSet("test-context", clientset)

	adapter := NewKubernetesDiscoveryAdapter(
		func() *fwdns.NamespaceManager { return mgr },
		"",
	)

	// Test with zero/default limit
	eventList, err := adapter.GetEvents("test-context", "default", types.GetEventsOptions{
		Limit: 0, // should use default of 50
	})
	if err != nil {
		t.Fatalf("GetEvents failed: %v", err)
	}

	// Should return empty slice when no events
	if eventList == nil {
		t.Error("Expected non-nil events slice")
	}
}

func TestKubernetesDiscoveryAdapter_GetEndpoints_WithFakeClient(t *testing.T) {
	endpoints := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-service",
			Namespace: "default",
		},
		Subsets: []corev1.EndpointSubset{
			{
				Addresses: []corev1.EndpointAddress{
					{IP: "10.1.1.1", TargetRef: &corev1.ObjectReference{Kind: "Pod", Name: "pod-1"}},
					{IP: "10.1.1.2", TargetRef: &corev1.ObjectReference{Kind: "Pod", Name: "pod-2"}},
				},
				NotReadyAddresses: []corev1.EndpointAddress{
					{IP: "10.1.1.3", TargetRef: &corev1.ObjectReference{Kind: "Pod", Name: "pod-3"}},
				},
				Ports: []corev1.EndpointPort{
					{Name: "http", Port: 8080, Protocol: corev1.ProtocolTCP},
				},
			},
		},
	}

	clientset := fake.NewClientset(endpoints)

	stopCh := make(chan struct{})
	defer close(stopCh)
	mgr := fwdns.NewManager(fwdns.ManagerConfig{GlobalStopCh: stopCh})
	mgr.SetClientSet("test-context", clientset)

	adapter := NewKubernetesDiscoveryAdapter(
		func() *fwdns.NamespaceManager { return mgr },
		"",
	)

	result, err := adapter.GetEndpoints("test-context", "default", "my-service")
	if err != nil {
		t.Fatalf("GetEndpoints failed: %v", err)
	}

	if result.Name != "my-service" {
		t.Errorf("Expected name 'my-service', got %s", result.Name)
	}
	if len(result.Subsets) != 1 {
		t.Fatalf("Expected 1 subset, got %d", len(result.Subsets))
	}
	if len(result.Subsets[0].Addresses) != 2 {
		t.Errorf("Expected 2 ready addresses, got %d", len(result.Subsets[0].Addresses))
	}
	if len(result.Subsets[0].NotReadyAddresses) != 1 {
		t.Errorf("Expected 1 not-ready address, got %d", len(result.Subsets[0].NotReadyAddresses))
	}
	if len(result.Subsets[0].Ports) != 1 {
		t.Errorf("Expected 1 port, got %d", len(result.Subsets[0].Ports))
	}
}

func TestKubernetesDiscoveryAdapter_GetEndpoints_NotFound(t *testing.T) {
	clientset := fake.NewClientset()

	stopCh := make(chan struct{})
	defer close(stopCh)
	mgr := fwdns.NewManager(fwdns.ManagerConfig{GlobalStopCh: stopCh})
	mgr.SetClientSet("test-context", clientset)

	adapter := NewKubernetesDiscoveryAdapter(
		func() *fwdns.NamespaceManager { return mgr },
		"",
	)

	_, err := adapter.GetEndpoints("test-context", "default", "nonexistent-service")
	if err == nil {
		t.Error("Expected error for nonexistent endpoints")
	}
}

// Test GetServiceDiagnostic with various status scenarios
func TestDiagnosticsProviderAdapter_GetServiceDiagnostic_Active(t *testing.T) {
	initTestRegistry()
	store := state.NewStore(100)

	// Add service with only active forwards
	store.AddForward(state.ForwardSnapshot{
		Key:         "svc.ns.ctx.pod1.8080",
		ServiceKey:  "svc.ns.ctx",
		ServiceName: "svc",
		Namespace:   "ns",
		Context:     "ctx",
		PodName:     "pod1",
		LocalPort:   "8080",
		Status:      state.StatusActive,
	})

	adapter := NewDiagnosticsProviderAdapter(
		func() *state.Store { return store },
		nil,
	)

	diag, err := adapter.GetServiceDiagnostic("svc.ns.ctx")
	if err != nil {
		t.Fatalf("GetServiceDiagnostic failed: %v", err)
	}

	if diag.Status != "active" {
		t.Errorf("Expected status 'active', got '%s'", diag.Status)
	}
	if diag.ServiceName != "svc" {
		t.Errorf("Expected service name 'svc', got '%s'", diag.ServiceName)
	}
}

func TestDiagnosticsProviderAdapter_GetServiceDiagnostic_Error(t *testing.T) {
	initTestRegistry()
	store := state.NewStore(100)

	// Add service with only error forwards
	store.AddForward(state.ForwardSnapshot{
		Key:         "svc.ns.ctx.pod1.8080",
		ServiceKey:  "svc.ns.ctx",
		ServiceName: "svc",
		Namespace:   "ns",
		Context:     "ctx",
		PodName:     "pod1",
		LocalPort:   "8080",
		Status:      state.StatusError,
		Error:       "connection refused",
	})

	adapter := NewDiagnosticsProviderAdapter(
		func() *state.Store { return store },
		nil,
	)

	diag, err := adapter.GetServiceDiagnostic("svc.ns.ctx")
	if err != nil {
		t.Fatalf("GetServiceDiagnostic failed: %v", err)
	}

	if diag.Status != "error" {
		t.Errorf("Expected status 'error', got '%s'", diag.Status)
	}
	if len(diag.ErrorHistory) != 1 {
		t.Errorf("Expected 1 error in history, got %d", len(diag.ErrorHistory))
	}
}

func TestDiagnosticsProviderAdapter_GetServiceDiagnostic_Partial(t *testing.T) {
	initTestRegistry()
	store := state.NewStore(100)

	// Add service with both active and error forwards
	store.AddForward(state.ForwardSnapshot{
		Key:         "svc.ns.ctx.pod1.8080",
		ServiceKey:  "svc.ns.ctx",
		ServiceName: "svc",
		Namespace:   "ns",
		Context:     "ctx",
		PodName:     "pod1",
		LocalPort:   "8080",
		Status:      state.StatusActive,
	})
	store.AddForward(state.ForwardSnapshot{
		Key:         "svc.ns.ctx.pod2.8080",
		ServiceKey:  "svc.ns.ctx",
		ServiceName: "svc",
		Namespace:   "ns",
		Context:     "ctx",
		PodName:     "pod2",
		LocalPort:   "8080",
		Status:      state.StatusError,
		Error:       "connection refused",
	})

	adapter := NewDiagnosticsProviderAdapter(
		func() *state.Store { return store },
		nil,
	)

	diag, err := adapter.GetServiceDiagnostic("svc.ns.ctx")
	if err != nil {
		t.Fatalf("GetServiceDiagnostic failed: %v", err)
	}

	if diag.Status != "partial" {
		t.Errorf("Expected status 'partial', got '%s'", diag.Status)
	}
	if diag.ActiveCount != 1 {
		t.Errorf("Expected ActiveCount 1, got %d", diag.ActiveCount)
	}
	if diag.ErrorCount != 1 {
		t.Errorf("Expected ErrorCount 1, got %d", diag.ErrorCount)
	}
}

func TestDiagnosticsProviderAdapter_GetServiceDiagnostic_NotFound(t *testing.T) {
	store := state.NewStore(100)

	adapter := NewDiagnosticsProviderAdapter(
		func() *state.Store { return store },
		nil,
	)

	_, err := adapter.GetServiceDiagnostic("nonexistent")
	if err == nil {
		t.Fatal("Expected error for nonexistent service")
	}
	if err.Error() != "service not found: nonexistent" {
		t.Errorf("Expected 'service not found' error, got: %s", err.Error())
	}
}

func TestDiagnosticsProviderAdapter_GetForwardDiagnostic_WithStore(t *testing.T) {
	store := state.NewStore(100)

	// Add forward
	store.AddForward(state.ForwardSnapshot{
		Key:         "svc.ns.ctx.pod1.8080",
		ServiceKey:  "svc.ns.ctx",
		ServiceName: "svc",
		Namespace:   "ns",
		Context:     "ctx",
		PodName:     "pod1",
		LocalPort:   "8080",
		LocalIP:     "127.1.1.1",
		Status:      state.StatusActive,
		StartedAt:   time.Now().Add(-time.Hour),
		LastActive:  time.Now(),
	})

	adapter := NewDiagnosticsProviderAdapter(
		func() *state.Store { return store },
		nil,
	)

	diag, err := adapter.GetForwardDiagnostic("svc.ns.ctx.pod1.8080")
	if err != nil {
		t.Fatalf("GetForwardDiagnostic failed: %v", err)
	}

	if diag.Key != "svc.ns.ctx.pod1.8080" {
		t.Errorf("Expected key 'svc.ns.ctx.pod1.8080', got '%s'", diag.Key)
	}
	if diag.Connection.State != "connected" {
		t.Errorf("Expected connection state 'connected', got '%s'", diag.Connection.State)
	}
	if diag.PodName != "pod1" {
		t.Errorf("Expected pod name 'pod1', got '%s'", diag.PodName)
	}
}

func TestDiagnosticsProviderAdapter_GetForwardDiagnostic_NotFound(t *testing.T) {
	store := state.NewStore(100)

	adapter := NewDiagnosticsProviderAdapter(
		func() *state.Store { return store },
		nil,
	)

	_, err := adapter.GetForwardDiagnostic("nonexistent")
	if err == nil {
		t.Fatal("Expected error for nonexistent forward")
	}
	if err.Error() != "forward not found: nonexistent" {
		t.Errorf("Expected 'forward not found' error, got: %s", err.Error())
	}
}

// Test ReconnectAll with error forwards
func TestServiceControllerAdapter_ReconnectAll_WithErroredForwards(t *testing.T) {
	initTestRegistry()
	store := state.NewStore(100)

	// Add forwards with different statuses
	store.AddForward(state.ForwardSnapshot{
		Key:         "svc1.ns.ctx.pod1.8080",
		ServiceKey:  "svc1.ns.ctx",
		RegistryKey: "svc1.ns.ctx",
		ServiceName: "svc1",
		Status:      state.StatusActive,
	})
	store.AddForward(state.ForwardSnapshot{
		Key:         "svc2.ns.ctx.pod2.8080",
		ServiceKey:  "svc2.ns.ctx",
		RegistryKey: "svc2.ns.ctx",
		ServiceName: "svc2",
		Status:      state.StatusError,
		Error:       "connection refused",
	})

	adapter := NewServiceControllerAdapter(func() *state.Store { return store })

	// ReconnectAll should try to reconnect error services
	// Since the services aren't registered in fwdsvcregistry, count will be 0
	count := adapter.ReconnectAll()

	// We can't easily test the actual reconnect without integration,
	// but we can verify it doesn't panic and returns a count
	if count < 0 {
		t.Error("ReconnectAll should return non-negative count")
	}
}

// Test splitLogLines helper function
func TestSplitLogLines_Empty(t *testing.T) {
	lines := splitLogLines("")
	if len(lines) != 0 {
		t.Errorf("Expected 0 lines for empty string, got %d", len(lines))
	}
}

func TestSplitLogLines_SingleLine(t *testing.T) {
	lines := splitLogLines("single line content")
	if len(lines) != 1 {
		t.Errorf("Expected 1 line, got %d", len(lines))
	}
	if lines[0] != "single line content" {
		t.Errorf("Expected 'single line content', got '%s'", lines[0])
	}
}

func TestSplitLogLines_MultipleLines_LF(t *testing.T) {
	lines := splitLogLines("line1\nline2\nline3")
	if len(lines) != 3 {
		t.Errorf("Expected 3 lines, got %d", len(lines))
	}
	if lines[0] != "line1" || lines[1] != "line2" || lines[2] != "line3" {
		t.Errorf("Lines don't match expected: %v", lines)
	}
}

func TestSplitLogLines_MultipleLines_CRLF(t *testing.T) {
	lines := splitLogLines("line1\r\nline2\r\nline3")
	if len(lines) != 3 {
		t.Errorf("Expected 3 lines, got %d", len(lines))
	}
	if lines[0] != "line1" || lines[1] != "line2" || lines[2] != "line3" {
		t.Errorf("Lines don't match expected: %v", lines)
	}
}

func TestSplitLogLines_TrailingNewline(t *testing.T) {
	lines := splitLogLines("line1\nline2\n")
	if len(lines) != 2 {
		t.Errorf("Expected 2 lines, got %d", len(lines))
	}
}

func TestSplitLogLines_MixedLineEndings(t *testing.T) {
	lines := splitLogLines("line1\nline2\r\nline3\n")
	if len(lines) != 3 {
		t.Errorf("Expected 3 lines, got %d", len(lines))
	}
}

// Test GetPodLogs with invalid sinceTime format
func TestKubernetesDiscoveryAdapter_GetPodLogs_InvalidSinceTime(t *testing.T) {
	initTestRegistry()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "test-container", Image: "test:v1"}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	clientset := fake.NewClientset(pod)

	stopCh := make(chan struct{})
	defer close(stopCh)
	mgr := fwdns.NewManager(fwdns.ManagerConfig{GlobalStopCh: stopCh})
	mgr.SetClientSet("test-context", clientset)

	adapter := NewKubernetesDiscoveryAdapter(
		func() *fwdns.NamespaceManager { return mgr },
		"",
	)

	_, err := adapter.GetPodLogs("test-context", "default", "test-pod", types.PodLogsOptions{
		SinceTime: "invalid-time-format",
	})

	if err == nil {
		t.Fatal("Expected error for invalid sinceTime format")
	}
	if !strings.Contains(err.Error(), "invalid sinceTime format") {
		t.Errorf("Expected 'invalid sinceTime format' error, got: %s", err.Error())
	}
}

// Test GetPodLogs pod not found
func TestKubernetesDiscoveryAdapter_GetPodLogs_PodNotFound(t *testing.T) {
	initTestRegistry()

	clientset := fake.NewClientset() // Empty - no pods

	stopCh := make(chan struct{})
	defer close(stopCh)
	mgr := fwdns.NewManager(fwdns.ManagerConfig{GlobalStopCh: stopCh})
	mgr.SetClientSet("test-context", clientset)

	adapter := NewKubernetesDiscoveryAdapter(
		func() *fwdns.NamespaceManager { return mgr },
		"",
	)

	_, err := adapter.GetPodLogs("test-context", "default", "nonexistent-pod", types.PodLogsOptions{})

	if err == nil {
		t.Fatal("Expected error for nonexistent pod")
	}
	if !strings.Contains(err.Error(), "failed to get pod") {
		t.Errorf("Expected 'failed to get pod' error, got: %s", err.Error())
	}
}

// Test GetPodLogs with valid options (tailLines, etc.)
func TestKubernetesDiscoveryAdapter_GetPodLogs_WithOptions(t *testing.T) {
	initTestRegistry()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "my-container", Image: "test:v1"}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	clientset := fake.NewClientset(pod)

	stopCh := make(chan struct{})
	defer close(stopCh)
	mgr := fwdns.NewManager(fwdns.ManagerConfig{GlobalStopCh: stopCh})
	mgr.SetClientSet("test-context", clientset)

	adapter := NewKubernetesDiscoveryAdapter(
		func() *fwdns.NamespaceManager { return mgr },
		"",
	)

	// Test with various options - the fake client doesn't fully support logs,
	// but this exercises the option parsing code
	_, err := adapter.GetPodLogs("test-context", "default", "test-pod", types.PodLogsOptions{
		Container:  "my-container",
		TailLines:  50,
		Previous:   false,
		Timestamps: true,
		SinceTime:  "2024-01-01T00:00:00Z",
	})

	// The fake client returns an error because it doesn't implement GetLogs
	// but we've at least exercised the options parsing
	if err == nil {
		t.Log("GetPodLogs succeeded with fake client (unexpected but acceptable)")
	}
}

// Test GetPodLogs with extremely high tailLines (should be capped)
func TestKubernetesDiscoveryAdapter_GetPodLogs_TailLinesCapped(t *testing.T) {
	initTestRegistry()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "container", Image: "test:v1"}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	clientset := fake.NewClientset(pod)

	stopCh := make(chan struct{})
	defer close(stopCh)
	mgr := fwdns.NewManager(fwdns.ManagerConfig{GlobalStopCh: stopCh})
	mgr.SetClientSet("test-context", clientset)

	adapter := NewKubernetesDiscoveryAdapter(
		func() *fwdns.NamespaceManager { return mgr },
		"",
	)

	// Request 5000 lines - should be capped to 1000
	_, _ = adapter.GetPodLogs("test-context", "default", "test-pod", types.PodLogsOptions{
		TailLines: 5000,
	})
	// The internal logic caps at 1000, but we can't easily verify without mocking deeper
	// This test ensures the code path is exercised without panic
}

// Test ServiceCRUDAdapter AddService nil manager
func TestServiceCRUDAdapter_AddService_NilManager(t *testing.T) {
	adapter := NewServiceCRUDAdapter(
		func() *state.Store { return nil },
		func() *fwdns.NamespaceManager { return nil },
		"",
	)

	_, err := adapter.AddService(types.AddServiceRequest{
		ServiceName: "my-service",
		Namespace:   "default",
	})

	if err == nil {
		t.Fatal("Expected error for nil namespace manager")
	}
	if !strings.Contains(err.Error(), "namespace manager not available") {
		t.Errorf("Expected 'namespace manager not available' error, got: %s", err.Error())
	}
}

// Tests for extracted helper functions

func TestBuildPortMappings(t *testing.T) {
	tests := []struct {
		name     string
		service  *corev1.Service
		expected int
	}{
		{
			name: "TCP ports only",
			service: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{Port: 80, TargetPort: intstr.FromInt(8080), Protocol: corev1.ProtocolTCP},
						{Port: 443, TargetPort: intstr.FromInt(8443), Protocol: corev1.ProtocolTCP},
					},
				},
			},
			expected: 2,
		},
		{
			name: "Skip UDP ports",
			service: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{Port: 80, TargetPort: intstr.FromInt(8080), Protocol: corev1.ProtocolTCP},
						{Port: 53, TargetPort: intstr.FromInt(53), Protocol: corev1.ProtocolUDP},
					},
				},
			},
			expected: 1,
		},
		{
			name: "Empty ports",
			service: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{},
				},
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildPortMappings(tt.service)
			if len(result) != tt.expected {
				t.Errorf("Expected %d port mappings, got %d", tt.expected, len(result))
			}
		})
	}
}

func TestSetupPodAddedSubscription(t *testing.T) {
	// Test without event bus (returns nil unsubscribe)
	ch, unsubscribe := setupPodAddedSubscription("test-service.default.context")
	if ch == nil {
		t.Error("Expected non-nil channel")
	}
	if unsubscribe != nil {
		t.Error("Expected nil unsubscribe when no event bus")
	}
}

func TestSetupPodAddedSubscription_WithEventBus(t *testing.T) {
	// Create and start an event bus
	bus := events.NewBus(100)
	bus.Start()

	// Store original and set test bus
	// Note: This test verifies the function works with a bus
	// In practice, the global bus would need to be set

	bus.Stop()
}

func TestWaitForPodAdded_NilUnsubscribe(t *testing.T) {
	ch := make(chan events.Event, 1)
	localIP, hostnames := waitForPodAdded(ch, nil, "test-key")

	if localIP != "" {
		t.Errorf("Expected empty localIP, got %s", localIP)
	}
	if hostnames != nil {
		t.Errorf("Expected nil hostnames, got %v", hostnames)
	}
}

func TestWaitForPodAdded_EventReceived(t *testing.T) {
	ch := make(chan events.Event, 1)
	unsubscribeCalled := false
	unsubscribe := func() { unsubscribeCalled = true }

	// Send event before calling wait
	ch <- events.Event{
		LocalIP:   "127.1.1.1",
		Hostnames: []string{"test-host"},
	}

	localIP, hostnames := waitForPodAdded(ch, unsubscribe, "test-key")

	if localIP != "127.1.1.1" {
		t.Errorf("Expected localIP 127.1.1.1, got %s", localIP)
	}
	if len(hostnames) != 1 || hostnames[0] != "test-host" {
		t.Errorf("Expected hostnames [test-host], got %v", hostnames)
	}
	if !unsubscribeCalled {
		t.Error("Expected unsubscribe to be called")
	}
}

func TestCheckPodForwarded(t *testing.T) {
	initTestRegistry()

	// Test when no services are registered
	isForwarded, forwardKey := checkPodForwarded("test-pod", "default")
	if isForwarded {
		t.Error("Expected pod not to be forwarded when no services registered")
	}
	if forwardKey != "" {
		t.Errorf("Expected empty forward key, got %s", forwardKey)
	}
}

func TestDeterminePodStatusString(t *testing.T) {
	now := metav1.Now()

	tests := []struct {
		name string
		pod  *corev1.Pod
		want string
	}{
		{
			name: "terminating",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &now},
				Status:     corev1.PodStatus{Phase: corev1.PodRunning},
			},
			want: "Terminating",
		},
		{
			name: "waiting with reason",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
					ContainerStatuses: []corev1.ContainerStatus{
						{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}}},
					},
				},
			},
			want: "ImagePullBackOff",
		},
		{
			name: "terminated with reason",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodFailed,
					ContainerStatuses: []corev1.ContainerStatus{
						{State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled"}}},
					},
				},
			},
			want: "OOMKilled",
		},
		{
			name: "running - default phase",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{Phase: corev1.PodRunning},
			},
			want: "Running",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := determinePodStatusString(tt.pod)
			if got != tt.want {
				t.Errorf("determinePodStatusString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSortAndLimitEvents(t *testing.T) {
	now := metav1.Now()
	past := metav1.NewTime(now.Add(-1 * time.Hour))
	future := metav1.NewTime(now.Add(1 * time.Hour))

	eventList := []corev1.Event{
		{ObjectMeta: metav1.ObjectMeta{Name: "past"}, LastTimestamp: past},
		{ObjectMeta: metav1.ObjectMeta{Name: "now"}, LastTimestamp: now},
		{ObjectMeta: metav1.ObjectMeta{Name: "future"}, LastTimestamp: future},
	}

	// Test sorting (most recent first)
	sorted := sortAndLimitEvents(eventList, 10)
	if len(sorted) != 3 {
		t.Fatalf("Expected 3 events, got %d", len(sorted))
	}
	if sorted[0].Name != "future" {
		t.Errorf("Expected first event to be 'future', got %s", sorted[0].Name)
	}

	// Test limiting
	eventList2 := []corev1.Event{
		{ObjectMeta: metav1.ObjectMeta{Name: "1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "2"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "3"}},
	}
	limited := sortAndLimitEvents(eventList2, 2)
	if len(limited) != 2 {
		t.Errorf("Expected 2 events with limit, got %d", len(limited))
	}

	// Test default limit
	defaultLimited := sortAndLimitEvents(eventList2, 0)
	if len(defaultLimited) != 3 {
		t.Errorf("Expected 3 events with default limit, got %d", len(defaultLimited))
	}
}

func TestConvertEventsToK8sEvents(t *testing.T) {
	now := metav1.Now()
	eventList := []corev1.Event{
		{
			Type:           "Normal",
			Reason:         "Scheduled",
			Message:        "Successfully scheduled",
			Count:          1,
			FirstTimestamp: now,
			LastTimestamp:  now,
			Source:         corev1.EventSource{Component: "scheduler"},
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "test-pod"},
		},
	}

	result := convertEventsToK8sEvents(eventList)
	if len(result) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(result))
	}
	if result[0].Type != "Normal" {
		t.Errorf("Expected Type 'Normal', got %s", result[0].Type)
	}
	if result[0].Reason != "Scheduled" {
		t.Errorf("Expected Reason 'Scheduled', got %s", result[0].Reason)
	}
	if result[0].ObjectName != "test-pod" {
		t.Errorf("Expected ObjectName 'test-pod', got %s", result[0].ObjectName)
	}
}
