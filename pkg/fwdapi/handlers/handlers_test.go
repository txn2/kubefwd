package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/txn2/kubefwd/pkg/fwdapi/types"
	"github.com/txn2/kubefwd/pkg/fwdmetrics"
	"github.com/txn2/kubefwd/pkg/fwdtui/events"
	"github.com/txn2/kubefwd/pkg/fwdtui/state"
)

// Mock implementations

type mockStateReader struct {
	services []state.ServiceSnapshot
	forwards []state.ForwardSnapshot
	logs     []state.LogEntry
	summary  state.SummaryStats
}

func (m *mockStateReader) GetServices() []state.ServiceSnapshot {
	return m.services
}

func (m *mockStateReader) GetService(key string) *state.ServiceSnapshot {
	for i := range m.services {
		if m.services[i].Key == key {
			return &m.services[i]
		}
	}
	return nil
}

func (m *mockStateReader) GetSummary() state.SummaryStats {
	return m.summary
}

func (m *mockStateReader) GetFiltered() []state.ForwardSnapshot {
	return m.forwards
}

func (m *mockStateReader) GetForward(key string) *state.ForwardSnapshot {
	for i := range m.forwards {
		if m.forwards[i].Key == key {
			return &m.forwards[i]
		}
	}
	return nil
}

func (m *mockStateReader) GetLogs(count int) []state.LogEntry {
	if count >= len(m.logs) {
		return m.logs
	}
	return m.logs[:count]
}

func (m *mockStateReader) Count() int {
	return len(m.forwards)
}

func (m *mockStateReader) ServiceCount() int {
	return len(m.services)
}

type mockServiceController struct {
	reconnectErr error
	syncErr      error
	reconnected  []string
}

func (m *mockServiceController) Reconnect(key string) error {
	if m.reconnectErr != nil {
		return m.reconnectErr
	}
	m.reconnected = append(m.reconnected, key)
	return nil
}

func (m *mockServiceController) ReconnectAll() int {
	return len(m.reconnected)
}

func (m *mockServiceController) Sync(key string, force bool) error {
	return m.syncErr
}

type mockMetricsProvider struct {
	snapshots []fwdmetrics.ServiceSnapshot
}

func (m *mockMetricsProvider) GetAllSnapshots() []fwdmetrics.ServiceSnapshot {
	return m.snapshots
}

func (m *mockMetricsProvider) GetServiceSnapshot(key string) *fwdmetrics.ServiceSnapshot {
	for i := range m.snapshots {
		if m.snapshots[i].ServiceName+"."+m.snapshots[i].Namespace+"."+m.snapshots[i].Context == key {
			return &m.snapshots[i]
		}
	}
	return nil
}

func (m *mockMetricsProvider) GetTotals() (uint64, uint64, float64, float64) {
	var bytesIn, bytesOut uint64
	var rateIn, rateOut float64
	for _, s := range m.snapshots {
		bytesIn += s.TotalBytesIn
		bytesOut += s.TotalBytesOut
		rateIn += s.TotalRateIn
		rateOut += s.TotalRateOut
	}
	return bytesIn, bytesOut, rateIn, rateOut
}

func (m *mockMetricsProvider) ServiceCount() int {
	return len(m.snapshots)
}

func (m *mockMetricsProvider) PortForwardCount() int {
	count := 0
	for _, s := range m.snapshots {
		count += len(s.PortForwards)
	}
	return count
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

type mockEventStreamer struct {
	eventCh chan events.Event
}

func (m *mockEventStreamer) Subscribe() (<-chan events.Event, func()) {
	return m.eventCh, func() { close(m.eventCh) }
}

func (m *mockEventStreamer) SubscribeType(eventType events.EventType) (<-chan events.Event, func()) {
	return m.eventCh, func() { close(m.eventCh) }
}

// Test helpers

func setupRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

func performRequest(r *gin.Engine, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// Health Handler Tests

func TestHealthHandler_Root(t *testing.T) {
	r := setupRouter()
	h := NewHealthHandler("1.0.0", time.Now(), nil)
	r.GET("/", h.Root)

	w := performRequest(r, "GET", "/")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["name"] != "kubefwd API" {
		t.Errorf("Expected name 'kubefwd API', got '%v'", response["name"])
	}
	if response["version"] != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%v'", response["version"])
	}
}

func TestHealthHandler_Health(t *testing.T) {
	r := setupRouter()
	startTime := time.Now().Add(-time.Hour)
	h := NewHealthHandler("1.0.0", startTime, nil)
	r.GET("/health", h.Health)

	w := performRequest(r, "GET", "/health")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response types.HealthResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response.Status != "healthy" {
		t.Errorf("Expected status 'healthy', got '%s'", response.Status)
	}
	if response.Version != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", response.Version)
	}
}

func TestHealthHandler_Info(t *testing.T) {
	r := setupRouter()
	startTime := time.Now()
	mockMgr := &mockManagerInfo{
		version:    "1.0.0",
		uptime:     time.Hour,
		startTime:  startTime,
		namespaces: []string{"default", "kube-system"},
		contexts:   []string{"minikube"},
		tuiEnabled: true,
	}
	h := NewHealthHandler("1.0.0", startTime, func() types.ManagerInfo { return mockMgr })
	r.GET("/info", h.Info)

	w := performRequest(r, "GET", "/info")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response types.Response
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if !response.Success {
		t.Errorf("Expected success true, got false")
	}
}

// Services Handler Tests

func TestServicesHandler_List(t *testing.T) {
	r := setupRouter()
	mock := &mockStateReader{
		services: []state.ServiceSnapshot{
			{
				Key:         "svc1.default.ctx",
				ServiceName: "svc1",
				Namespace:   "default",
				Context:     "ctx",
				ActiveCount: 1,
			},
			{
				Key:         "svc2.default.ctx",
				ServiceName: "svc2",
				Namespace:   "default",
				Context:     "ctx",
				ErrorCount:  1,
			},
		},
		summary: state.SummaryStats{
			TotalServices:  2,
			ActiveServices: 1,
		},
	}
	h := NewServicesHandler(mock, nil)
	r.GET("/v1/services", h.List)

	w := performRequest(r, "GET", "/v1/services")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response types.Response
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if !response.Success {
		t.Errorf("Expected success true, got false")
	}
	if response.Meta == nil || response.Meta.Count != 2 {
		t.Errorf("Expected meta count 2, got %v", response.Meta)
	}
}

func TestServicesHandler_ListNilState(t *testing.T) {
	r := setupRouter()
	h := NewServicesHandler(nil, nil)
	r.GET("/v1/services", h.List)

	w := performRequest(r, "GET", "/v1/services")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestServicesHandler_Get(t *testing.T) {
	r := setupRouter()
	mock := &mockStateReader{
		services: []state.ServiceSnapshot{
			{
				Key:         "svc1.default.ctx",
				ServiceName: "svc1",
				Namespace:   "default",
				Context:     "ctx",
				ActiveCount: 1,
			},
		},
	}
	h := NewServicesHandler(mock, nil)
	r.GET("/v1/services/:key", h.Get)

	w := performRequest(r, "GET", "/v1/services/svc1.default.ctx")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestServicesHandler_GetNotFound(t *testing.T) {
	r := setupRouter()
	mock := &mockStateReader{services: []state.ServiceSnapshot{}}
	h := NewServicesHandler(mock, nil)
	r.GET("/v1/services/:key", h.Get)

	w := performRequest(r, "GET", "/v1/services/nonexistent")

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestServicesHandler_Reconnect(t *testing.T) {
	r := setupRouter()
	mock := &mockServiceController{}
	h := NewServicesHandler(nil, mock)
	r.POST("/v1/services/:key/reconnect", h.Reconnect)

	w := performRequest(r, "POST", "/v1/services/svc1.default.ctx/reconnect")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	if len(mock.reconnected) != 1 || mock.reconnected[0] != "svc1.default.ctx" {
		t.Errorf("Expected reconnected ['svc1.default.ctx'], got %v", mock.reconnected)
	}
}

// Forwards Handler Tests

func TestForwardsHandler_List(t *testing.T) {
	r := setupRouter()
	mock := &mockStateReader{
		forwards: []state.ForwardSnapshot{
			{
				Key:         "fwd1",
				ServiceName: "svc1",
				Namespace:   "default",
				LocalIP:     "127.1.0.1",
				LocalPort:   "8080",
				Status:      state.StatusActive,
			},
		},
		summary: state.SummaryStats{
			TotalForwards:  1,
			ActiveForwards: 1,
		},
	}
	h := NewForwardsHandler(mock)
	r.GET("/v1/forwards", h.List)

	w := performRequest(r, "GET", "/v1/forwards")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response types.Response
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if !response.Success {
		t.Errorf("Expected success true, got false")
	}
}

func TestForwardsHandler_Get(t *testing.T) {
	r := setupRouter()
	mock := &mockStateReader{
		forwards: []state.ForwardSnapshot{
			{
				Key:         "fwd1",
				ServiceName: "svc1",
				Status:      state.StatusActive,
			},
		},
	}
	h := NewForwardsHandler(mock)
	r.GET("/v1/forwards/:key", h.Get)

	w := performRequest(r, "GET", "/v1/forwards/fwd1")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestForwardsHandler_GetNotFound(t *testing.T) {
	r := setupRouter()
	mock := &mockStateReader{forwards: []state.ForwardSnapshot{}}
	h := NewForwardsHandler(mock)
	r.GET("/v1/forwards/:key", h.Get)

	w := performRequest(r, "GET", "/v1/forwards/nonexistent")

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

// Metrics Handler Tests

func TestMetricsHandler_Summary(t *testing.T) {
	r := setupRouter()
	mock := &mockStateReader{
		summary: state.SummaryStats{
			TotalServices:  5,
			ActiveServices: 4,
			TotalForwards:  10,
			ActiveForwards: 8,
			TotalBytesIn:   1024,
			TotalBytesOut:  2048,
		},
	}
	mockMgr := &mockManagerInfo{uptime: time.Hour}
	h := NewMetricsHandler(mock, nil, func() types.ManagerInfo { return mockMgr })
	r.GET("/v1/metrics", h.Summary)

	w := performRequest(r, "GET", "/v1/metrics")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestMetricsHandler_ByService(t *testing.T) {
	r := setupRouter()
	mock := &mockMetricsProvider{
		snapshots: []fwdmetrics.ServiceSnapshot{
			{
				ServiceName:   "svc1",
				Namespace:     "default",
				Context:       "ctx",
				TotalBytesIn:  512,
				TotalBytesOut: 1024,
			},
		},
	}
	h := NewMetricsHandler(nil, mock, nil)
	r.GET("/v1/metrics/services", h.ByService)

	w := performRequest(r, "GET", "/v1/metrics/services")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

// Logs Handler Tests

func TestLogsHandler_Recent(t *testing.T) {
	r := setupRouter()
	now := time.Now()
	mock := &mockStateReader{
		logs: []state.LogEntry{
			{Timestamp: now, Level: "info", Message: "Test log 1"},
			{Timestamp: now.Add(time.Second), Level: "warn", Message: "Test log 2"},
		},
	}
	h := NewLogsHandler(mock)
	r.GET("/v1/logs", h.Recent)

	w := performRequest(r, "GET", "/v1/logs")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response types.Response
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if !response.Success {
		t.Errorf("Expected success true, got false")
	}
}

func TestLogsHandler_RecentWithCount(t *testing.T) {
	r := setupRouter()
	now := time.Now()
	logs := make([]state.LogEntry, 50)
	for i := 0; i < 50; i++ {
		logs[i] = state.LogEntry{
			Timestamp: now.Add(time.Duration(i) * time.Second),
			Level:     "info",
			Message:   "Log message",
		}
	}
	mock := &mockStateReader{logs: logs}
	h := NewLogsHandler(mock)
	r.GET("/v1/logs", h.Recent)

	w := performRequest(r, "GET", "/v1/logs?count=10")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response types.Response
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response.Meta == nil || response.Meta.Count != 10 {
		t.Errorf("Expected meta count 10, got %v", response.Meta)
	}
}

// Helper function tests

func TestMapServiceSnapshot(t *testing.T) {
	svc := state.ServiceSnapshot{
		Key:           "svc.ns.ctx",
		ServiceName:   "svc",
		Namespace:     "ns",
		Context:       "ctx",
		Headless:      false,
		ActiveCount:   2,
		ErrorCount:    0,
		TotalBytesIn:  1024,
		TotalBytesOut: 2048,
		TotalRateIn:   100.0,
		TotalRateOut:  200.0,
	}

	result := mapServiceSnapshot(svc)

	if result.Key != "svc.ns.ctx" {
		t.Errorf("Expected key 'svc.ns.ctx', got '%s'", result.Key)
	}
	if result.Status != "active" {
		t.Errorf("Expected status 'active', got '%s'", result.Status)
	}
}

func TestMapServiceSnapshot_ErrorStatus(t *testing.T) {
	svc := state.ServiceSnapshot{
		Key:         "svc.ns.ctx",
		ActiveCount: 0,
		ErrorCount:  1,
	}

	result := mapServiceSnapshot(svc)

	if result.Status != "error" {
		t.Errorf("Expected status 'error', got '%s'", result.Status)
	}
}

func TestMapServiceSnapshot_PartialStatus(t *testing.T) {
	svc := state.ServiceSnapshot{
		Key:         "svc.ns.ctx",
		ActiveCount: 1,
		ErrorCount:  1,
	}

	result := mapServiceSnapshot(svc)

	if result.Status != "partial" {
		t.Errorf("Expected status 'partial', got '%s'", result.Status)
	}
}

func TestEscapeJSON(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"hello\nworld", "hello\\nworld"},
		{"tab\there", "tab\\there"},
		{`quote"here`, `quote\"here`},
		{"back\\slash", "back\\\\slash"},
		{"\r\n", "\\r\\n"},
	}

	for _, test := range tests {
		result := escapeJSON(test.input)
		if result != test.expected {
			t.Errorf("escapeJSON(%q) = %q, expected %q", test.input, result, test.expected)
		}
	}
}

func TestParseEventType(t *testing.T) {
	tests := []struct {
		input    string
		expected events.EventType
	}{
		{"ServiceAdded", events.ServiceAdded},
		{"ServiceRemoved", events.ServiceRemoved},
		{"ServiceUpdated", events.ServiceUpdated},
		{"PodAdded", events.PodAdded},
		{"PodRemoved", events.PodRemoved},
		{"PodStatusChanged", events.PodStatusChanged},
		{"BandwidthUpdate", events.BandwidthUpdate},
		{"LogMessage", events.LogMessage},
		{"ShutdownStarted", events.ShutdownStarted},
		{"ShutdownComplete", events.ShutdownComplete},
		{"unknown", events.PodStatusChanged}, // default
	}

	for _, test := range tests {
		result := parseEventType(test.input)
		if result != test.expected {
			t.Errorf("parseEventType(%q) = %v, expected %v", test.input, result, test.expected)
		}
	}
}

// Mock Diagnostics Provider

type mockDiagnosticsProvider struct {
	summary           types.DiagnosticSummary
	serviceDiagnostic *types.ServiceDiagnostic
	forwardDiagnostic *types.ForwardDiagnostic
	networkStatus     types.NetworkStatus
	errors            []types.ErrorDetail
	serviceErr        error
	forwardErr        error
}

func (m *mockDiagnosticsProvider) GetSummary() types.DiagnosticSummary {
	return m.summary
}

func (m *mockDiagnosticsProvider) GetServiceDiagnostic(key string) (*types.ServiceDiagnostic, error) {
	if m.serviceErr != nil {
		return nil, m.serviceErr
	}
	return m.serviceDiagnostic, nil
}

func (m *mockDiagnosticsProvider) GetForwardDiagnostic(key string) (*types.ForwardDiagnostic, error) {
	if m.forwardErr != nil {
		return nil, m.forwardErr
	}
	return m.forwardDiagnostic, nil
}

func (m *mockDiagnosticsProvider) GetNetworkStatus() types.NetworkStatus {
	return m.networkStatus
}

func (m *mockDiagnosticsProvider) GetErrors(count int) []types.ErrorDetail {
	if count >= len(m.errors) {
		return m.errors
	}
	return m.errors[:count]
}

// Diagnostics Handler Tests

func TestDiagnosticsHandler_Summary(t *testing.T) {
	r := setupRouter()
	mock := &mockDiagnosticsProvider{
		summary: types.DiagnosticSummary{
			Status:  "healthy",
			Version: "1.0.0",
			Uptime:  "1h0m0s",
			Services: types.ServicesSummaryDiag{
				Total:  5,
				Active: 4,
				Error:  1,
			},
		},
	}
	h := NewDiagnosticsHandler(mock)
	r.GET("/v1/diagnostics", h.Summary)

	w := performRequest(r, "GET", "/v1/diagnostics")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response types.Response
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if !response.Success {
		t.Errorf("Expected success true, got false")
	}
}

func TestDiagnosticsHandler_SummaryNilProvider(t *testing.T) {
	r := setupRouter()
	h := NewDiagnosticsHandler(nil)
	r.GET("/v1/diagnostics", h.Summary)

	w := performRequest(r, "GET", "/v1/diagnostics")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestDiagnosticsHandler_ServiceDiagnostic(t *testing.T) {
	r := setupRouter()
	mock := &mockDiagnosticsProvider{
		serviceDiagnostic: &types.ServiceDiagnostic{
			Key:         "svc1.default.ctx",
			ServiceName: "svc1",
			Namespace:   "default",
			Context:     "ctx",
			Status:      "active",
			ActiveCount: 2,
		},
	}
	h := NewDiagnosticsHandler(mock)
	r.GET("/v1/diagnostics/services/:key", h.ServiceDiagnostic)

	w := performRequest(r, "GET", "/v1/diagnostics/services/svc1.default.ctx")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestDiagnosticsHandler_ServiceDiagnosticNotFound(t *testing.T) {
	r := setupRouter()
	mock := &mockDiagnosticsProvider{
		serviceErr: fmt.Errorf("service not found"),
	}
	h := NewDiagnosticsHandler(mock)
	r.GET("/v1/diagnostics/services/:key", h.ServiceDiagnostic)

	w := performRequest(r, "GET", "/v1/diagnostics/services/nonexistent")

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestDiagnosticsHandler_ForwardDiagnostic(t *testing.T) {
	r := setupRouter()
	mock := &mockDiagnosticsProvider{
		forwardDiagnostic: &types.ForwardDiagnostic{
			Key:        "pod1.svc1.default.ctx",
			ServiceKey: "svc1.default.ctx",
			PodName:    "pod1",
			Status:     "connected",
			LocalIP:    "127.1.0.1",
			LocalPort:  "8080",
		},
	}
	h := NewDiagnosticsHandler(mock)
	r.GET("/v1/diagnostics/forwards/:key", h.ForwardDiagnostic)

	w := performRequest(r, "GET", "/v1/diagnostics/forwards/pod1.svc1.default.ctx")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestDiagnosticsHandler_ForwardDiagnosticNotFound(t *testing.T) {
	r := setupRouter()
	mock := &mockDiagnosticsProvider{
		forwardErr: fmt.Errorf("forward not found"),
	}
	h := NewDiagnosticsHandler(mock)
	r.GET("/v1/diagnostics/forwards/:key", h.ForwardDiagnostic)

	w := performRequest(r, "GET", "/v1/diagnostics/forwards/nonexistent")

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestDiagnosticsHandler_Network(t *testing.T) {
	r := setupRouter()
	mock := &mockDiagnosticsProvider{
		networkStatus: types.NetworkStatus{
			LoopbackInterface: "lo0",
			IPsAllocated:      5,
			IPRange:           "127.1.0.0/24",
			PortsInUse:        10,
		},
	}
	h := NewDiagnosticsHandler(mock)
	r.GET("/v1/diagnostics/network", h.Network)

	w := performRequest(r, "GET", "/v1/diagnostics/network")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestDiagnosticsHandler_Errors(t *testing.T) {
	r := setupRouter()
	now := time.Now()
	mock := &mockDiagnosticsProvider{
		errors: []types.ErrorDetail{
			{
				Timestamp:  now,
				Component:  "connection",
				ServiceKey: "svc1.default.ctx",
				Message:    "connection refused",
			},
		},
	}
	h := NewDiagnosticsHandler(mock)
	r.GET("/v1/diagnostics/errors", h.Errors)

	w := performRequest(r, "GET", "/v1/diagnostics/errors")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response types.Response
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response.Meta == nil || response.Meta.Count != 1 {
		t.Errorf("Expected meta count 1, got %v", response.Meta)
	}
}

func TestDiagnosticsHandler_ErrorsWithCount(t *testing.T) {
	r := setupRouter()
	now := time.Now()
	errors := make([]types.ErrorDetail, 10)
	for i := 0; i < 10; i++ {
		errors[i] = types.ErrorDetail{
			Timestamp: now,
			Component: "connection",
			Message:   "error",
		}
	}
	mock := &mockDiagnosticsProvider{errors: errors}
	h := NewDiagnosticsHandler(mock)
	r.GET("/v1/diagnostics/errors", h.Errors)

	w := performRequest(r, "GET", "/v1/diagnostics/errors?count=5")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response types.Response
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response.Meta == nil || response.Meta.Count != 5 {
		t.Errorf("Expected meta count 5, got %v", response.Meta)
	}
}

// Analyze Handler Tests

func TestAnalyzeHandler_Status(t *testing.T) {
	r := setupRouter()
	mock := &mockStateReader{
		summary: state.SummaryStats{
			TotalServices:  5,
			ActiveServices: 5,
			ActiveForwards: 10,
			ErrorCount:     0,
		},
	}
	mockMgr := &mockManagerInfo{uptime: time.Hour}
	h := NewAnalyzeHandler(mock, nil, func() types.ManagerInfo { return mockMgr })
	r.GET("/v1/status", h.Status)

	w := performRequest(r, "GET", "/v1/status")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response types.Response
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if !response.Success {
		t.Errorf("Expected success true, got false")
	}

	// Check the status data
	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected data to be a map")
	}
	if data["status"] != "ok" {
		t.Errorf("Expected status 'ok', got '%v'", data["status"])
	}
}

func TestAnalyzeHandler_StatusWithIssues(t *testing.T) {
	r := setupRouter()
	mock := &mockStateReader{
		summary: state.SummaryStats{
			TotalServices:  5,
			ActiveServices: 3,
			ErrorCount:     2,
		},
	}
	h := NewAnalyzeHandler(mock, nil, nil)
	r.GET("/v1/status", h.Status)

	w := performRequest(r, "GET", "/v1/status")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response types.Response
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected data to be a map")
	}
	if data["status"] != "issues" {
		t.Errorf("Expected status 'issues', got '%v'", data["status"])
	}
}

func TestAnalyzeHandler_StatusWithErrors(t *testing.T) {
	r := setupRouter()
	mock := &mockStateReader{
		summary: state.SummaryStats{
			TotalServices:  5,
			ActiveServices: 1,
			ErrorCount:     4,
		},
	}
	h := NewAnalyzeHandler(mock, nil, nil)
	r.GET("/v1/status", h.Status)

	w := performRequest(r, "GET", "/v1/status")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response types.Response
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected data to be a map")
	}
	if data["status"] != "error" {
		t.Errorf("Expected status 'error', got '%v'", data["status"])
	}
}

func TestAnalyzeHandler_StatusNilStateReader(t *testing.T) {
	r := setupRouter()
	h := NewAnalyzeHandler(nil, nil, nil)
	r.GET("/v1/status", h.Status)

	w := performRequest(r, "GET", "/v1/status")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestAnalyzeHandler_Analyze(t *testing.T) {
	r := setupRouter()
	mock := &mockStateReader{
		services: []state.ServiceSnapshot{
			{
				Key:         "svc1.default.ctx",
				ServiceName: "svc1",
				Namespace:   "default",
				Context:     "ctx",
				ActiveCount: 1,
			},
		},
		summary: state.SummaryStats{
			TotalServices:  1,
			ActiveServices: 1,
			TotalForwards:  1,
			ActiveForwards: 1,
		},
	}
	mockMgr := &mockManagerInfo{uptime: time.Hour}
	h := NewAnalyzeHandler(mock, nil, func() types.ManagerInfo { return mockMgr })
	r.GET("/v1/analyze", h.Analyze)

	w := performRequest(r, "GET", "/v1/analyze")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response types.Response
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if !response.Success {
		t.Errorf("Expected success true, got false")
	}
}

func TestAnalyzeHandler_AnalyzeWithErrors(t *testing.T) {
	r := setupRouter()
	mock := &mockStateReader{
		services: []state.ServiceSnapshot{
			{
				Key:         "svc1.default.ctx",
				ServiceName: "svc1",
				ErrorCount:  1,
				PortForwards: []state.PortForwardInfo{
					{
						PodName: "pod1",
						Error:   "connection refused",
					},
				},
			},
			{
				Key:         "svc2.default.ctx",
				ServiceName: "svc2",
				ErrorCount:  1,
				PortForwards: []state.PortForwardInfo{
					{
						PodName: "pod2",
						Error:   "timeout error",
					},
				},
			},
		},
		summary: state.SummaryStats{
			TotalServices: 2,
			ErrorCount:    2,
		},
	}
	h := NewAnalyzeHandler(mock, nil, nil)
	r.GET("/v1/analyze", h.Analyze)

	w := performRequest(r, "GET", "/v1/analyze")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response types.Response
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected data to be a map")
	}
	if data["status"] != "unhealthy" {
		t.Errorf("Expected status 'unhealthy', got '%v'", data["status"])
	}
}

func TestAnalyzeHandler_AnalyzeNilStateReader(t *testing.T) {
	r := setupRouter()
	h := NewAnalyzeHandler(nil, nil, nil)
	r.GET("/v1/analyze", h.Analyze)

	w := performRequest(r, "GET", "/v1/analyze")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

// HTTP Traffic Handler Tests

func TestHTTPTrafficHandler_ForwardHTTP(t *testing.T) {
	r := setupRouter()
	now := time.Now()
	mockState := &mockStateReader{
		forwards: []state.ForwardSnapshot{
			{
				Key:        "pod1.svc1.default.ctx",
				ServiceKey: "svc1.default.ctx",
				PodName:    "pod1",
				LocalIP:    "127.1.0.1",
				LocalPort:  "8080",
			},
		},
	}
	mockMetrics := &mockMetricsProvider{
		snapshots: []fwdmetrics.ServiceSnapshot{
			{
				ServiceName: "svc1",
				Namespace:   "default",
				Context:     "ctx",
				PortForwards: []fwdmetrics.PortForwardSnapshot{
					{
						PodName:   "pod1",
						LocalIP:   "127.1.0.1",
						LocalPort: "8080",
						HTTPLogs: []fwdmetrics.HTTPLogEntry{
							{
								Timestamp:  now,
								Method:     "GET",
								Path:       "/api/v1/health",
								StatusCode: 200,
								Duration:   time.Millisecond * 50,
								Size:       1024,
							},
						},
					},
				},
			},
		},
	}
	h := NewHTTPTrafficHandler(mockMetrics, mockState)
	r.GET("/v1/forwards/:key/http", h.ForwardHTTP)

	w := performRequest(r, "GET", "/v1/forwards/pod1.svc1.default.ctx/http")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response types.Response
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if !response.Success {
		t.Errorf("Expected success true, got false")
	}
	if response.Meta == nil || response.Meta.Count != 1 {
		t.Errorf("Expected meta count 1, got %v", response.Meta)
	}
}

func TestHTTPTrafficHandler_ForwardHTTPNotFound(t *testing.T) {
	r := setupRouter()
	mockState := &mockStateReader{forwards: []state.ForwardSnapshot{}}
	mockMetrics := &mockMetricsProvider{}
	h := NewHTTPTrafficHandler(mockMetrics, mockState)
	r.GET("/v1/forwards/:key/http", h.ForwardHTTP)

	w := performRequest(r, "GET", "/v1/forwards/nonexistent/http")

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestHTTPTrafficHandler_ForwardHTTPNilProvider(t *testing.T) {
	r := setupRouter()
	h := NewHTTPTrafficHandler(nil, nil)
	r.GET("/v1/forwards/:key/http", h.ForwardHTTP)

	w := performRequest(r, "GET", "/v1/forwards/test/http")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestHTTPTrafficHandler_ServiceHTTP(t *testing.T) {
	r := setupRouter()
	now := time.Now()
	mockMetrics := &mockMetricsProvider{
		snapshots: []fwdmetrics.ServiceSnapshot{
			{
				ServiceName: "svc1",
				Namespace:   "default",
				Context:     "ctx",
				PortForwards: []fwdmetrics.PortForwardSnapshot{
					{
						PodName:   "pod1",
						LocalIP:   "127.1.0.1",
						LocalPort: "8080",
						HTTPLogs: []fwdmetrics.HTTPLogEntry{
							{
								Timestamp:  now,
								Method:     "GET",
								Path:       "/health",
								StatusCode: 200,
							},
							{
								Timestamp:  now,
								Method:     "POST",
								Path:       "/api/data",
								StatusCode: 201,
							},
						},
					},
				},
			},
		},
	}
	h := NewHTTPTrafficHandler(mockMetrics, nil)
	r.GET("/v1/services/:key/http", h.ServiceHTTP)

	w := performRequest(r, "GET", "/v1/services/svc1.default.ctx/http")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response types.Response
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if !response.Success {
		t.Errorf("Expected success true, got false")
	}
}

func TestHTTPTrafficHandler_ServiceHTTPNotFound(t *testing.T) {
	r := setupRouter()
	mockMetrics := &mockMetricsProvider{snapshots: []fwdmetrics.ServiceSnapshot{}}
	h := NewHTTPTrafficHandler(mockMetrics, nil)
	r.GET("/v1/services/:key/http", h.ServiceHTTP)

	w := performRequest(r, "GET", "/v1/services/nonexistent/http")

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestHTTPTrafficHandler_ServiceHTTPNilProvider(t *testing.T) {
	r := setupRouter()
	h := NewHTTPTrafficHandler(nil, nil)
	r.GET("/v1/services/:key/http", h.ServiceHTTP)

	w := performRequest(r, "GET", "/v1/services/test/http")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

// History Handler Tests

func TestHistoryHandler_Events(t *testing.T) {
	r := setupRouter()
	h := NewHistoryHandler()
	r.GET("/v1/history/events", h.Events)

	w := performRequest(r, "GET", "/v1/history/events")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response types.Response
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if !response.Success {
		t.Errorf("Expected success true, got false")
	}
}

func TestHistoryHandler_EventsWithParams(t *testing.T) {
	r := setupRouter()
	h := NewHistoryHandler()
	r.GET("/v1/history/events", h.Events)

	w := performRequest(r, "GET", "/v1/history/events?count=50&type=ServiceAdded")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestHistoryHandler_Errors(t *testing.T) {
	r := setupRouter()
	h := NewHistoryHandler()
	r.GET("/v1/history/errors", h.Errors)

	w := performRequest(r, "GET", "/v1/history/errors")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestHistoryHandler_ErrorsWithCount(t *testing.T) {
	r := setupRouter()
	h := NewHistoryHandler()
	r.GET("/v1/history/errors", h.Errors)

	w := performRequest(r, "GET", "/v1/history/errors?count=25")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestHistoryHandler_Reconnections(t *testing.T) {
	r := setupRouter()
	h := NewHistoryHandler()
	r.GET("/v1/services/:key/history/reconnections", h.Reconnections)

	w := performRequest(r, "GET", "/v1/services/svc1.default.ctx/history/reconnections")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestHistoryHandler_AllReconnections(t *testing.T) {
	r := setupRouter()
	h := NewHistoryHandler()
	r.GET("/v1/history/reconnections", h.AllReconnections)

	w := performRequest(r, "GET", "/v1/history/reconnections")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestHistoryHandler_Stats(t *testing.T) {
	r := setupRouter()
	h := NewHistoryHandler()
	r.GET("/v1/history/stats", h.Stats)

	w := performRequest(r, "GET", "/v1/history/stats")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response types.Response
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if !response.Success {
		t.Errorf("Expected success true, got false")
	}
}

// Helper function tests for HTTP traffic

func TestBuildHTTPActivitySummary(t *testing.T) {
	now := time.Now()
	logs := []fwdmetrics.HTTPLogEntry{
		{Timestamp: now, Method: "GET", Path: "/api/health", StatusCode: 200},
		{Timestamp: now.Add(time.Second), Method: "GET", Path: "/api/data", StatusCode: 200},
		{Timestamp: now.Add(2 * time.Second), Method: "POST", Path: "/api/data", StatusCode: 201},
		{Timestamp: now.Add(3 * time.Second), Method: "GET", Path: "/api/data", StatusCode: 500},
		{Timestamp: now.Add(4 * time.Second), Method: "GET", Path: "/api/error", StatusCode: 404},
	}

	summary := buildHTTPActivitySummary(logs)

	if summary.TotalRequests != 5 {
		t.Errorf("Expected TotalRequests 5, got %d", summary.TotalRequests)
	}
	if summary.StatusCodes["2xx"] != 3 {
		t.Errorf("Expected 2xx count 3, got %d", summary.StatusCodes["2xx"])
	}
	if summary.StatusCodes["4xx"] != 1 {
		t.Errorf("Expected 4xx count 1, got %d", summary.StatusCodes["4xx"])
	}
	if summary.StatusCodes["5xx"] != 1 {
		t.Errorf("Expected 5xx count 1, got %d", summary.StatusCodes["5xx"])
	}
}

func TestBuildHTTPActivitySummaryEmpty(t *testing.T) {
	summary := buildHTTPActivitySummary(nil)

	if summary.TotalRequests != 0 {
		t.Errorf("Expected TotalRequests 0, got %d", summary.TotalRequests)
	}
}

func TestBuildHTTPTrafficResponse(t *testing.T) {
	now := time.Now()
	logs := []fwdmetrics.HTTPLogEntry{
		{
			Timestamp:  now,
			Method:     "GET",
			Path:       "/test",
			StatusCode: 200,
			Duration:   time.Millisecond * 100,
			Size:       512,
		},
	}

	response := buildHTTPTrafficResponse("key1", "pod1", "127.1.0.1", "8080", logs)

	if response.ForwardKey != "key1" {
		t.Errorf("Expected ForwardKey 'key1', got '%s'", response.ForwardKey)
	}
	if response.PodName != "pod1" {
		t.Errorf("Expected PodName 'pod1', got '%s'", response.PodName)
	}
	if len(response.Logs) != 1 {
		t.Errorf("Expected 1 log, got %d", len(response.Logs))
	}
	if response.Logs[0].Method != "GET" {
		t.Errorf("Expected method 'GET', got '%s'", response.Logs[0].Method)
	}
}
