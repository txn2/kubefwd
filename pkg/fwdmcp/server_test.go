package fwdmcp

import (
	"sync"
	"testing"
	"time"

	"github.com/txn2/kubefwd/pkg/fwdapi/types"
	"github.com/txn2/kubefwd/pkg/fwdmetrics"
	"github.com/txn2/kubefwd/pkg/fwdtui/state"
)

// Reset global state between tests
func resetGlobalState() {
	enabledMu.Lock()
	enabled = false
	enabledMu.Unlock()

	globalServer = nil
	serverOnce = sync.Once{}
}

func TestEnable(t *testing.T) {
	resetGlobalState()

	if IsEnabled() {
		t.Error("Expected MCP to be disabled initially")
	}

	Enable()

	if !IsEnabled() {
		t.Error("Expected MCP to be enabled after Enable()")
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

func TestInit(t *testing.T) {
	resetGlobalState()

	server := Init("1.0.0")

	if server == nil {
		t.Fatal("Expected Init to return non-nil Server")
	}

	if server.version != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", server.version)
	}

	if server.stopCh == nil {
		t.Error("Expected stopCh to be initialized")
	}

	if server.doneCh == nil {
		t.Error("Expected doneCh to be initialized")
	}

	if server.mcpServer == nil {
		t.Error("Expected mcpServer to be initialized")
	}
}

func TestInit_OnlyOnce(t *testing.T) {
	resetGlobalState()

	server1 := Init("1.0.0")
	server2 := Init("2.0.0")

	if server1 != server2 {
		t.Error("Expected Init to return the same server on subsequent calls")
	}

	if server2.version != "1.0.0" {
		t.Errorf("Expected version to remain '1.0.0', got '%s'", server2.version)
	}
}

func TestGetServer(t *testing.T) {
	resetGlobalState()

	if GetServer() != nil {
		t.Error("Expected GetServer to return nil before Init")
	}

	Init("1.0.0")

	if GetServer() == nil {
		t.Error("Expected GetServer to return non-nil after Init")
	}
}

func TestServer_SetStateReader(t *testing.T) {
	resetGlobalState()

	server := Init("1.0.0")
	mock := &mockStateReader{}

	server.SetStateReader(mock)

	if server.getState() != mock {
		t.Error("Expected stateReader to be set")
	}
}

func TestServer_SetMetricsProvider(t *testing.T) {
	resetGlobalState()

	server := Init("1.0.0")
	mock := &mockMetricsProvider{}

	server.SetMetricsProvider(mock)

	if server.getMetrics() != mock {
		t.Error("Expected metricsProvider to be set")
	}
}

func TestServer_SetServiceController(t *testing.T) {
	resetGlobalState()

	server := Init("1.0.0")
	mock := &mockServiceController{}

	server.SetServiceController(mock)

	if server.getController() != mock {
		t.Error("Expected serviceController to be set")
	}
}

func TestServer_SetDiagnosticsProvider(t *testing.T) {
	resetGlobalState()

	server := Init("1.0.0")
	mock := &mockDiagnosticsProvider{}

	server.SetDiagnosticsProvider(mock)

	if server.getDiagnostics() != mock {
		t.Error("Expected diagnosticsProvider to be set")
	}
}

func TestServer_SetManagerInfo(t *testing.T) {
	resetGlobalState()

	server := Init("1.0.0")
	mockMgr := &mockManagerInfo{version: "1.0.0"}

	server.SetManagerInfo(func() types.ManagerInfo {
		return mockMgr
	})

	if server.getManager() == nil {
		t.Error("Expected managerInfo to be set")
	}
	if server.getManager().Version() != "1.0.0" {
		t.Errorf("Expected manager version '1.0.0', got '%s'", server.getManager().Version())
	}
}

func TestServer_GetManagerNilGetter(t *testing.T) {
	resetGlobalState()

	server := Init("1.0.0")

	if server.getManager() != nil {
		t.Error("Expected getManager to return nil when getter not set")
	}
}

func TestServer_Done(t *testing.T) {
	resetGlobalState()

	server := Init("1.0.0")

	doneCh := server.Done()

	if doneCh == nil {
		t.Error("Expected Done() to return non-nil channel")
	}
}

func TestServer_SetAnalysisProvider(t *testing.T) {
	resetGlobalState()

	server := Init("1.0.0")

	// Initially nil
	if server.getAnalysisProvider() != nil {
		t.Error("Expected analysisProvider to be nil initially")
	}

	// Set it (using nil pointer since we can't create a real one without HTTP)
	var provider *AnalysisProviderHTTP
	server.SetAnalysisProvider(provider)

	// Should still be nil since we set nil
	if server.getAnalysisProvider() != nil {
		t.Error("Expected analysisProvider to remain nil")
	}
}

func TestServer_SetHTTPTrafficProvider(t *testing.T) {
	resetGlobalState()

	server := Init("1.0.0")

	// Initially nil
	if server.getHTTPTrafficProvider() != nil {
		t.Error("Expected httpTrafficProvider to be nil initially")
	}

	// Set it (using nil pointer)
	var provider *HTTPTrafficProviderHTTP
	server.SetHTTPTrafficProvider(provider)

	// Should still be nil
	if server.getHTTPTrafficProvider() != nil {
		t.Error("Expected httpTrafficProvider to remain nil")
	}
}

func TestServer_SetHistoryProvider(t *testing.T) {
	resetGlobalState()

	server := Init("1.0.0")

	// Initially nil
	if server.getHistoryProvider() != nil {
		t.Error("Expected historyProvider to be nil initially")
	}

	// Set it (using nil pointer)
	var provider *HistoryProviderHTTP
	server.SetHistoryProvider(provider)

	// Should still be nil
	if server.getHistoryProvider() != nil {
		t.Error("Expected historyProvider to remain nil")
	}
}

// Mock implementations

type mockStateReader struct {
	services []state.ServiceSnapshot
	forwards []state.ForwardSnapshot
	logs     []state.LogEntry
	summary  state.SummaryStats
}

func (m *mockStateReader) GetServices() []state.ServiceSnapshot { return m.services }
func (m *mockStateReader) GetService(key string) *state.ServiceSnapshot {
	for i := range m.services {
		if m.services[i].Key == key {
			return &m.services[i]
		}
	}
	return nil
}
func (m *mockStateReader) GetSummary() state.SummaryStats               { return m.summary }
func (m *mockStateReader) GetFiltered() []state.ForwardSnapshot         { return m.forwards }
func (m *mockStateReader) GetForward(key string) *state.ForwardSnapshot { return nil }
func (m *mockStateReader) GetLogs(count int) []state.LogEntry           { return m.logs }
func (m *mockStateReader) Count() int                                   { return len(m.forwards) }
func (m *mockStateReader) ServiceCount() int                            { return len(m.services) }

type mockMetricsProvider struct {
	snapshots []fwdmetrics.ServiceSnapshot
	bytesIn   uint64
	bytesOut  uint64
	rateIn    float64
	rateOut   float64
}

func (m *mockMetricsProvider) GetAllSnapshots() []fwdmetrics.ServiceSnapshot { return m.snapshots }
func (m *mockMetricsProvider) GetServiceSnapshot(key string) *fwdmetrics.ServiceSnapshot {
	return nil
}
func (m *mockMetricsProvider) GetTotals() (uint64, uint64, float64, float64) {
	return m.bytesIn, m.bytesOut, m.rateIn, m.rateOut
}
func (m *mockMetricsProvider) ServiceCount() int     { return len(m.snapshots) }
func (m *mockMetricsProvider) PortForwardCount() int { return 0 }

type mockServiceController struct {
	reconnectErr error
	syncErr      error
	reconnected  int
}

func (m *mockServiceController) Reconnect(key string) error { return m.reconnectErr }
func (m *mockServiceController) ReconnectAll() int          { return m.reconnected }
func (m *mockServiceController) Sync(key string, force bool) error {
	return m.syncErr
}

type mockDiagnosticsProvider struct{}

func (m *mockDiagnosticsProvider) GetSummary() types.DiagnosticSummary {
	return types.DiagnosticSummary{}
}
func (m *mockDiagnosticsProvider) GetServiceDiagnostic(key string) (*types.ServiceDiagnostic, error) {
	return nil, nil
}
func (m *mockDiagnosticsProvider) GetForwardDiagnostic(key string) (*types.ForwardDiagnostic, error) {
	return nil, nil
}
func (m *mockDiagnosticsProvider) GetNetworkStatus() types.NetworkStatus {
	return types.NetworkStatus{}
}
func (m *mockDiagnosticsProvider) GetErrors(count int) []types.ErrorDetail {
	return nil
}

type mockManagerInfo struct {
	version    string
	uptime     time.Duration
	startTime  time.Time
	namespaces []string
	contexts   []string
	tuiEnabled bool
}

func (m *mockManagerInfo) Version() string       { return m.version }
func (m *mockManagerInfo) Uptime() time.Duration { return m.uptime }
func (m *mockManagerInfo) StartTime() time.Time  { return m.startTime }
func (m *mockManagerInfo) Namespaces() []string  { return m.namespaces }
func (m *mockManagerInfo) Contexts() []string    { return m.contexts }
func (m *mockManagerInfo) TUIEnabled() bool      { return m.tuiEnabled }

// Ensure mocks implement the interfaces
var (
	_ types.StateReader         = (*mockStateReader)(nil)
	_ types.MetricsProvider     = (*mockMetricsProvider)(nil)
	_ types.ServiceController   = (*mockServiceController)(nil)
	_ types.DiagnosticsProvider = (*mockDiagnosticsProvider)(nil)
	_ types.ManagerInfo         = (*mockManagerInfo)(nil)
)
