package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

func (m *mockServiceController) Sync(_ string, _ bool) error {
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

// Test helpers

func setupRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

func performRequest(r *gin.Engine, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, http.NoBody)
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
	h := NewLogsHandler(mock, nil)
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
	h := NewLogsHandler(mock, nil)
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

func (m *mockDiagnosticsProvider) GetServiceDiagnostic(_ string) (*types.ServiceDiagnostic, error) {
	if m.serviceErr != nil {
		return nil, m.serviceErr
	}
	return m.serviceDiagnostic, nil
}

func (m *mockDiagnosticsProvider) GetForwardDiagnostic(_ string) (*types.ForwardDiagnostic, error) {
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
				PortForwards: []state.ForwardSnapshot{
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
				PortForwards: []state.ForwardSnapshot{
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

// Additional Metrics Handler Tests

func TestMetricsHandler_SummaryNilState(t *testing.T) {
	r := setupRouter()
	h := NewMetricsHandler(nil, nil, nil)
	r.GET("/v1/metrics", h.Summary)

	w := performRequest(r, "GET", "/v1/metrics")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestMetricsHandler_ByServiceNilProvider(t *testing.T) {
	r := setupRouter()
	h := NewMetricsHandler(nil, nil, nil)
	r.GET("/v1/metrics/services", h.ByService)

	w := performRequest(r, "GET", "/v1/metrics/services")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestMetricsHandler_ServiceDetail(t *testing.T) {
	r := setupRouter()
	mock := &mockMetricsProvider{
		snapshots: []fwdmetrics.ServiceSnapshot{
			{
				ServiceName:   "svc1",
				Namespace:     "default",
				Context:       "ctx",
				TotalBytesIn:  1024,
				TotalBytesOut: 2048,
			},
		},
	}
	h := NewMetricsHandler(nil, mock, nil)
	r.GET("/v1/metrics/services/:key", h.ServiceDetail)

	w := performRequest(r, "GET", "/v1/metrics/services/svc1.default.ctx")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestMetricsHandler_ServiceDetailNotFound(t *testing.T) {
	r := setupRouter()
	mock := &mockMetricsProvider{snapshots: []fwdmetrics.ServiceSnapshot{}}
	h := NewMetricsHandler(nil, mock, nil)
	r.GET("/v1/metrics/services/:key", h.ServiceDetail)

	w := performRequest(r, "GET", "/v1/metrics/services/nonexistent")

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestMetricsHandler_ServiceDetailNilProvider(t *testing.T) {
	r := setupRouter()
	h := NewMetricsHandler(nil, nil, nil)
	r.GET("/v1/metrics/services/:key", h.ServiceDetail)

	w := performRequest(r, "GET", "/v1/metrics/services/test")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestMetricsHandler_ServiceHistory(t *testing.T) {
	r := setupRouter()
	now := time.Now()
	mock := &mockMetricsProvider{
		snapshots: []fwdmetrics.ServiceSnapshot{
			{
				ServiceName: "svc1",
				Namespace:   "default",
				Context:     "ctx",
				PortForwards: []fwdmetrics.PortForwardSnapshot{
					{
						PodName: "pod1",
						History: []fwdmetrics.RateSample{
							{Timestamp: now, BytesIn: 100, BytesOut: 200},
							{Timestamp: now.Add(time.Second), BytesIn: 150, BytesOut: 250},
						},
					},
				},
			},
		},
	}
	h := NewMetricsHandler(nil, mock, nil)
	r.GET("/v1/metrics/services/:key/history", h.ServiceHistory)

	w := performRequest(r, "GET", "/v1/metrics/services/svc1.default.ctx/history")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestMetricsHandler_ServiceHistoryWithPoints(t *testing.T) {
	r := setupRouter()
	now := time.Now()
	mock := &mockMetricsProvider{
		snapshots: []fwdmetrics.ServiceSnapshot{
			{
				ServiceName: "svc1",
				Namespace:   "default",
				Context:     "ctx",
				PortForwards: []fwdmetrics.PortForwardSnapshot{
					{
						PodName: "pod1",
						History: []fwdmetrics.RateSample{
							{Timestamp: now, BytesIn: 100, BytesOut: 200},
						},
					},
				},
			},
		},
	}
	h := NewMetricsHandler(nil, mock, nil)
	r.GET("/v1/metrics/services/:key/history", h.ServiceHistory)

	w := performRequest(r, "GET", "/v1/metrics/services/svc1.default.ctx/history?points=30")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestMetricsHandler_ServiceHistoryNotFound(t *testing.T) {
	r := setupRouter()
	mock := &mockMetricsProvider{snapshots: []fwdmetrics.ServiceSnapshot{}}
	h := NewMetricsHandler(nil, mock, nil)
	r.GET("/v1/metrics/services/:key/history", h.ServiceHistory)

	w := performRequest(r, "GET", "/v1/metrics/services/nonexistent/history")

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestMetricsHandler_ServiceHistoryNilProvider(t *testing.T) {
	r := setupRouter()
	h := NewMetricsHandler(nil, nil, nil)
	r.GET("/v1/metrics/services/:key/history", h.ServiceHistory)

	w := performRequest(r, "GET", "/v1/metrics/services/test/history")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

// Additional Services Handler Tests

func TestServicesHandler_Sync(t *testing.T) {
	r := setupRouter()
	mock := &mockServiceController{}
	h := NewServicesHandler(nil, mock)
	r.POST("/v1/services/:key/sync", h.Sync)

	w := performRequest(r, "POST", "/v1/services/svc1.default.ctx/sync")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestServicesHandler_SyncNilController(t *testing.T) {
	r := setupRouter()
	h := NewServicesHandler(nil, nil)
	r.POST("/v1/services/:key/sync", h.Sync)

	w := performRequest(r, "POST", "/v1/services/svc1.default.ctx/sync")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestServicesHandler_SyncWithError(t *testing.T) {
	r := setupRouter()
	mock := &mockServiceController{syncErr: fmt.Errorf("sync failed")}
	h := NewServicesHandler(nil, mock)
	r.POST("/v1/services/:key/sync", h.Sync)

	w := performRequest(r, "POST", "/v1/services/svc1.default.ctx/sync")

	// Handler returns 404 for errors (treating them as "not found")
	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestServicesHandler_ReconnectNilController(t *testing.T) {
	r := setupRouter()
	h := NewServicesHandler(nil, nil)
	r.POST("/v1/services/:key/reconnect", h.Reconnect)

	w := performRequest(r, "POST", "/v1/services/svc1/reconnect")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestServicesHandler_ReconnectWithError(t *testing.T) {
	r := setupRouter()
	mock := &mockServiceController{reconnectErr: fmt.Errorf("reconnect failed")}
	h := NewServicesHandler(nil, mock)
	r.POST("/v1/services/:key/reconnect", h.Reconnect)

	w := performRequest(r, "POST", "/v1/services/svc1/reconnect")

	// Handler returns 404 for errors (treating them as "not found")
	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestServicesHandler_ReconnectAll(t *testing.T) {
	r := setupRouter()
	mock := &mockServiceController{reconnected: []string{"svc1", "svc2"}}
	h := NewServicesHandler(nil, mock)
	r.POST("/v1/services/reconnect", h.ReconnectAll)

	w := performRequest(r, "POST", "/v1/services/reconnect")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestServicesHandler_ReconnectAllNilController(t *testing.T) {
	r := setupRouter()
	h := NewServicesHandler(nil, nil)
	r.POST("/v1/services/reconnect", h.ReconnectAll)

	w := performRequest(r, "POST", "/v1/services/reconnect")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestServicesHandler_GetNilStateReader(t *testing.T) {
	r := setupRouter()
	h := NewServicesHandler(nil, nil)
	r.GET("/v1/services/:key", h.Get)

	w := performRequest(r, "GET", "/v1/services/test")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

// Additional Forwards Handler Tests

func TestForwardsHandler_ListNilState(t *testing.T) {
	r := setupRouter()
	h := NewForwardsHandler(nil)
	r.GET("/v1/forwards", h.List)

	w := performRequest(r, "GET", "/v1/forwards")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestForwardsHandler_GetNilState(t *testing.T) {
	r := setupRouter()
	h := NewForwardsHandler(nil)
	r.GET("/v1/forwards/:key", h.Get)

	w := performRequest(r, "GET", "/v1/forwards/test")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

// Additional Logs Handler Tests

func TestLogsHandler_RecentNilState(t *testing.T) {
	r := setupRouter()
	h := NewLogsHandler(nil, nil)
	r.GET("/v1/logs", h.Recent)

	w := performRequest(r, "GET", "/v1/logs")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestLogsHandler_RecentInvalidCount(t *testing.T) {
	r := setupRouter()
	mock := &mockStateReader{logs: []state.LogEntry{}}
	h := NewLogsHandler(mock, nil)
	r.GET("/v1/logs", h.Recent)

	w := performRequest(r, "GET", "/v1/logs?count=invalid")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

// Additional Diagnostics Handler Tests

func TestDiagnosticsHandler_ServiceDiagnosticNilProvider(t *testing.T) {
	r := setupRouter()
	h := NewDiagnosticsHandler(nil)
	r.GET("/v1/diagnostics/services/:key", h.ServiceDiagnostic)

	w := performRequest(r, "GET", "/v1/diagnostics/services/test")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestDiagnosticsHandler_ForwardDiagnosticNilProvider(t *testing.T) {
	r := setupRouter()
	h := NewDiagnosticsHandler(nil)
	r.GET("/v1/diagnostics/forwards/:key", h.ForwardDiagnostic)

	w := performRequest(r, "GET", "/v1/diagnostics/forwards/test")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestDiagnosticsHandler_NetworkNilProvider(t *testing.T) {
	r := setupRouter()
	h := NewDiagnosticsHandler(nil)
	r.GET("/v1/diagnostics/network", h.Network)

	w := performRequest(r, "GET", "/v1/diagnostics/network")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestDiagnosticsHandler_ErrorsNilProvider(t *testing.T) {
	r := setupRouter()
	h := NewDiagnosticsHandler(nil)
	r.GET("/v1/diagnostics/errors", h.Errors)

	w := performRequest(r, "GET", "/v1/diagnostics/errors")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestDiagnosticsHandler_ErrorsInvalidCount(t *testing.T) {
	r := setupRouter()
	mock := &mockDiagnosticsProvider{errors: []types.ErrorDetail{}}
	h := NewDiagnosticsHandler(mock)
	r.GET("/v1/diagnostics/errors", h.Errors)

	w := performRequest(r, "GET", "/v1/diagnostics/errors?count=invalid")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestDiagnosticsHandler_ErrorsMaxCount(t *testing.T) {
	r := setupRouter()
	mock := &mockDiagnosticsProvider{errors: []types.ErrorDetail{}}
	h := NewDiagnosticsHandler(mock)
	r.GET("/v1/diagnostics/errors", h.Errors)

	w := performRequest(r, "GET", "/v1/diagnostics/errors?count=1000")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

// Test mapPortForwardMetrics helper

func TestMapPortForwardMetrics(t *testing.T) {
	now := time.Now()
	pfs := []fwdmetrics.PortForwardSnapshot{
		{
			PodName:        "pod1",
			LocalIP:        "127.1.0.1",
			LocalPort:      "8080",
			PodPort:        "80",
			BytesIn:        1024,
			BytesOut:       2048,
			RateIn:         100.0,
			RateOut:        200.0,
			AvgRateIn:      50.0,
			AvgRateOut:     100.0,
			ConnectedAt:    now,
			LastActivityAt: now,
		},
	}

	result := mapPortForwardMetrics(pfs)

	if len(result) != 1 {
		t.Errorf("Expected 1 result, got %d", len(result))
	}
	if result[0].PodName != "pod1" {
		t.Errorf("Expected PodName 'pod1', got '%s'", result[0].PodName)
	}
	if result[0].BytesIn != 1024 {
		t.Errorf("Expected BytesIn 1024, got %d", result[0].BytesIn)
	}
}

func TestMapPortForwardMetricsEmpty(t *testing.T) {
	result := mapPortForwardMetrics(nil)
	if len(result) != 0 {
		t.Errorf("Expected 0 results, got %d", len(result))
	}
}

// More Analyze handler edge cases

func TestAnalyzeHandler_AnalyzeNoServices(t *testing.T) {
	r := setupRouter()
	mock := &mockStateReader{
		services: []state.ServiceSnapshot{},
		summary:  state.SummaryStats{},
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
	if data["status"] != "healthy" {
		t.Errorf("Expected status 'healthy', got '%v'", data["status"])
	}
}

func TestAnalyzeHandler_StatusNoServices(t *testing.T) {
	r := setupRouter()
	mock := &mockStateReader{
		summary: state.SummaryStats{
			TotalServices:  0,
			ActiveServices: 0,
			ErrorCount:     0,
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
	if data["status"] != "ok" {
		t.Errorf("Expected status 'ok', got '%v'", data["status"])
	}
}

// Services List Filtering Tests

func TestServicesHandler_ListWithStatusFilter(t *testing.T) {
	r := setupRouter()
	mock := &mockStateReader{
		services: []state.ServiceSnapshot{
			{Key: "svc1.default.ctx", ServiceName: "svc1", Namespace: "default", Context: "ctx", ActiveCount: 1, ErrorCount: 0},
			{Key: "svc2.default.ctx", ServiceName: "svc2", Namespace: "default", Context: "ctx", ActiveCount: 0, ErrorCount: 1},
			{Key: "svc3.default.ctx", ServiceName: "svc3", Namespace: "default", Context: "ctx", ActiveCount: 1, ErrorCount: 1},
		},
		summary: state.SummaryStats{TotalServices: 3},
	}
	h := NewServicesHandler(mock, nil)
	r.GET("/v1/services", h.List)

	w := performRequest(r, "GET", "/v1/services?status=active")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response types.Response
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response.Meta == nil || response.Meta.Count != 1 {
		t.Errorf("Expected meta count 1 for active status filter, got %v", response.Meta)
	}
}

func TestServicesHandler_ListWithNamespaceFilter(t *testing.T) {
	r := setupRouter()
	mock := &mockStateReader{
		services: []state.ServiceSnapshot{
			{Key: "svc1.default.ctx", ServiceName: "svc1", Namespace: "default", Context: "ctx", ActiveCount: 1},
			{Key: "svc2.kube-system.ctx", ServiceName: "svc2", Namespace: "kube-system", Context: "ctx", ActiveCount: 1},
		},
		summary: state.SummaryStats{TotalServices: 2},
	}
	h := NewServicesHandler(mock, nil)
	r.GET("/v1/services", h.List)

	w := performRequest(r, "GET", "/v1/services?namespace=default")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response types.Response
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response.Meta == nil || response.Meta.Count != 1 {
		t.Errorf("Expected meta count 1 for namespace filter, got %v", response.Meta)
	}
}

func TestServicesHandler_ListWithContextFilter(t *testing.T) {
	r := setupRouter()
	mock := &mockStateReader{
		services: []state.ServiceSnapshot{
			{Key: "svc1.default.ctx1", ServiceName: "svc1", Namespace: "default", Context: "ctx1", ActiveCount: 1},
			{Key: "svc2.default.ctx2", ServiceName: "svc2", Namespace: "default", Context: "ctx2", ActiveCount: 1},
		},
		summary: state.SummaryStats{TotalServices: 2},
	}
	h := NewServicesHandler(mock, nil)
	r.GET("/v1/services", h.List)

	w := performRequest(r, "GET", "/v1/services?context=ctx1")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response types.Response
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response.Meta == nil || response.Meta.Count != 1 {
		t.Errorf("Expected meta count 1 for context filter, got %v", response.Meta)
	}
}

func TestServicesHandler_ListWithSearchFilter(t *testing.T) {
	r := setupRouter()
	mock := &mockStateReader{
		services: []state.ServiceSnapshot{
			{Key: "api-gateway.default.ctx", ServiceName: "api-gateway", Namespace: "default", Context: "ctx", ActiveCount: 1},
			{Key: "database.default.ctx", ServiceName: "database", Namespace: "default", Context: "ctx", ActiveCount: 1},
		},
		summary: state.SummaryStats{TotalServices: 2},
	}
	h := NewServicesHandler(mock, nil)
	r.GET("/v1/services", h.List)

	w := performRequest(r, "GET", "/v1/services?search=api")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response types.Response
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response.Meta == nil || response.Meta.Count != 1 {
		t.Errorf("Expected meta count 1 for search filter, got %v", response.Meta)
	}
}

func TestServicesHandler_ListWithPagination(t *testing.T) {
	r := setupRouter()
	services := make([]state.ServiceSnapshot, 20)
	for i := 0; i < 20; i++ {
		services[i] = state.ServiceSnapshot{
			Key:         fmt.Sprintf("svc%d.default.ctx", i),
			ServiceName: fmt.Sprintf("svc%d", i),
			Namespace:   "default",
			Context:     "ctx",
			ActiveCount: 1,
		}
	}
	mock := &mockStateReader{
		services: services,
		summary:  state.SummaryStats{TotalServices: 20},
	}
	h := NewServicesHandler(mock, nil)
	r.GET("/v1/services", h.List)

	w := performRequest(r, "GET", "/v1/services?limit=5&offset=5")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response types.Response
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response.Meta == nil || response.Meta.Count != 5 {
		t.Errorf("Expected meta count 5 for pagination, got %v", response.Meta)
	}
}

// Services status mapping tests

func TestMapServiceSnapshot_PendingStatus(t *testing.T) {
	svc := state.ServiceSnapshot{
		Key:         "svc.ns.ctx",
		ActiveCount: 0,
		ErrorCount:  0,
	}

	result := mapServiceSnapshot(svc)

	if result.Status != "pending" {
		t.Errorf("Expected status 'pending', got '%s'", result.Status)
	}
}

// History handler edge cases

func TestHistoryHandler_EventsInvalidCount(t *testing.T) {
	r := setupRouter()
	h := NewHistoryHandler()
	r.GET("/v1/history/events", h.Events)

	w := performRequest(r, "GET", "/v1/history/events?count=invalid")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestHistoryHandler_EventsMaxCount(t *testing.T) {
	r := setupRouter()
	h := NewHistoryHandler()
	r.GET("/v1/history/events", h.Events)

	w := performRequest(r, "GET", "/v1/history/events?count=5000")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestHistoryHandler_ReconnectionsWithCount(t *testing.T) {
	r := setupRouter()
	h := NewHistoryHandler()
	r.GET("/v1/services/:key/history/reconnections", h.Reconnections)

	w := performRequest(r, "GET", "/v1/services/svc1/history/reconnections?count=10")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestHistoryHandler_ReconnectionsMaxCount(t *testing.T) {
	r := setupRouter()
	h := NewHistoryHandler()
	r.GET("/v1/services/:key/history/reconnections", h.Reconnections)

	w := performRequest(r, "GET", "/v1/services/svc1/history/reconnections?count=500")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestHistoryHandler_AllReconnectionsMaxCount(t *testing.T) {
	r := setupRouter()
	h := NewHistoryHandler()
	r.GET("/v1/history/reconnections", h.AllReconnections)

	w := performRequest(r, "GET", "/v1/history/reconnections?count=500")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestHistoryHandler_ErrorsMaxCount(t *testing.T) {
	r := setupRouter()
	h := NewHistoryHandler()
	r.GET("/v1/history/errors", h.Errors)

	w := performRequest(r, "GET", "/v1/history/errors?count=1000")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

// HTTP traffic handler edge cases

func TestHTTPTrafficHandler_ForwardHTTPWithCount(t *testing.T) {
	r := setupRouter()
	now := time.Now()
	mockState := &mockStateReader{
		forwards: []state.ForwardSnapshot{
			{Key: "pod1.svc1.default.ctx", ServiceKey: "svc1.default.ctx", PodName: "pod1"},
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
						PodName: "pod1",
						HTTPLogs: []fwdmetrics.HTTPLogEntry{
							{Timestamp: now, Method: "GET", Path: "/test", StatusCode: 200},
						},
					},
				},
			},
		},
	}
	h := NewHTTPTrafficHandler(mockMetrics, mockState)
	r.GET("/v1/forwards/:key/http", h.ForwardHTTP)

	w := performRequest(r, "GET", "/v1/forwards/pod1.svc1.default.ctx/http?count=25")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestHTTPTrafficHandler_ForwardHTTPInvalidCount(t *testing.T) {
	r := setupRouter()
	mockState := &mockStateReader{
		forwards: []state.ForwardSnapshot{
			{Key: "pod1.svc1.default.ctx", ServiceKey: "svc1.default.ctx", PodName: "pod1"},
		},
	}
	mockMetrics := &mockMetricsProvider{
		snapshots: []fwdmetrics.ServiceSnapshot{
			{
				ServiceName:  "svc1",
				Namespace:    "default",
				Context:      "ctx",
				PortForwards: []fwdmetrics.PortForwardSnapshot{{PodName: "pod1"}},
			},
		},
	}
	h := NewHTTPTrafficHandler(mockMetrics, mockState)
	r.GET("/v1/forwards/:key/http", h.ForwardHTTP)

	w := performRequest(r, "GET", "/v1/forwards/pod1.svc1.default.ctx/http?count=invalid")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestHTTPTrafficHandler_ServiceHTTPWithCount(t *testing.T) {
	r := setupRouter()
	mockMetrics := &mockMetricsProvider{
		snapshots: []fwdmetrics.ServiceSnapshot{
			{
				ServiceName:  "svc1",
				Namespace:    "default",
				Context:      "ctx",
				PortForwards: []fwdmetrics.PortForwardSnapshot{{PodName: "pod1"}},
			},
		},
	}
	h := NewHTTPTrafficHandler(mockMetrics, nil)
	r.GET("/v1/services/:key/http", h.ServiceHTTP)

	w := performRequest(r, "GET", "/v1/services/svc1.default.ctx/http?count=25")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

// Metrics handler edge cases

func TestMetricsHandler_ServiceHistoryInvalidPoints(t *testing.T) {
	r := setupRouter()
	mock := &mockMetricsProvider{
		snapshots: []fwdmetrics.ServiceSnapshot{
			{ServiceName: "svc1", Namespace: "default", Context: "ctx"},
		},
	}
	h := NewMetricsHandler(nil, mock, nil)
	r.GET("/v1/metrics/services/:key/history", h.ServiceHistory)

	w := performRequest(r, "GET", "/v1/metrics/services/svc1.default.ctx/history?points=invalid")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestMetricsHandler_ServiceHistoryMaxPoints(t *testing.T) {
	r := setupRouter()
	mock := &mockMetricsProvider{
		snapshots: []fwdmetrics.ServiceSnapshot{
			{ServiceName: "svc1", Namespace: "default", Context: "ctx"},
		},
	}
	h := NewMetricsHandler(nil, mock, nil)
	r.GET("/v1/metrics/services/:key/history", h.ServiceHistory)

	w := performRequest(r, "GET", "/v1/metrics/services/svc1.default.ctx/history?points=500")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

// Forwards filtering tests

func TestForwardsHandler_ListWithStatusFilter(t *testing.T) {
	r := setupRouter()
	mock := &mockStateReader{
		forwards: []state.ForwardSnapshot{
			{Key: "fwd1", ServiceName: "svc1", Namespace: "default", Context: "ctx", Status: state.StatusActive},
			{Key: "fwd2", ServiceName: "svc2", Namespace: "default", Context: "ctx", Status: state.StatusError},
		},
		summary: state.SummaryStats{TotalForwards: 2},
	}
	h := NewForwardsHandler(mock)
	r.GET("/v1/forwards", h.List)

	w := performRequest(r, "GET", "/v1/forwards?status=active")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestForwardsHandler_ListWithNamespaceFilter(t *testing.T) {
	r := setupRouter()
	mock := &mockStateReader{
		forwards: []state.ForwardSnapshot{
			{Key: "fwd1", ServiceName: "svc1", Namespace: "default", Context: "ctx"},
			{Key: "fwd2", ServiceName: "svc2", Namespace: "kube-system", Context: "ctx"},
		},
		summary: state.SummaryStats{TotalForwards: 2},
	}
	h := NewForwardsHandler(mock)
	r.GET("/v1/forwards", h.List)

	w := performRequest(r, "GET", "/v1/forwards?namespace=default")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestForwardsHandler_ListWithContextFilter(t *testing.T) {
	r := setupRouter()
	mock := &mockStateReader{
		forwards: []state.ForwardSnapshot{
			{Key: "fwd1", ServiceName: "svc1", Namespace: "default", Context: "ctx1"},
			{Key: "fwd2", ServiceName: "svc2", Namespace: "default", Context: "ctx2"},
		},
		summary: state.SummaryStats{TotalForwards: 2},
	}
	h := NewForwardsHandler(mock)
	r.GET("/v1/forwards", h.List)

	w := performRequest(r, "GET", "/v1/forwards?context=ctx1")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestForwardsHandler_ListWithSearchFilter(t *testing.T) {
	r := setupRouter()
	mock := &mockStateReader{
		forwards: []state.ForwardSnapshot{
			{Key: "api-pod.svc", ServiceName: "api", Namespace: "default", Context: "ctx"},
			{Key: "db-pod.svc", ServiceName: "database", Namespace: "default", Context: "ctx"},
		},
		summary: state.SummaryStats{TotalForwards: 2},
	}
	h := NewForwardsHandler(mock)
	r.GET("/v1/forwards", h.List)

	w := performRequest(r, "GET", "/v1/forwards?search=api")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestForwardsHandler_ListWithPagination(t *testing.T) {
	r := setupRouter()
	forwards := make([]state.ForwardSnapshot, 20)
	for i := 0; i < 20; i++ {
		forwards[i] = state.ForwardSnapshot{
			Key:         fmt.Sprintf("fwd%d", i),
			ServiceName: fmt.Sprintf("svc%d", i),
			Namespace:   "default",
			Context:     "ctx",
		}
	}
	mock := &mockStateReader{
		forwards: forwards,
		summary:  state.SummaryStats{TotalForwards: 20},
	}
	h := NewForwardsHandler(mock)
	r.GET("/v1/forwards", h.List)

	w := performRequest(r, "GET", "/v1/forwards?limit=5&offset=5")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

// Events handler tests

func TestNewEventsHandler(t *testing.T) {
	h := NewEventsHandler(nil)
	if h == nil {
		t.Error("Expected non-nil handler")
	}
}

// Additional Analyze handler tests for error type branches

func TestAnalyzeHandler_AnalyzeAllErrorTypes(t *testing.T) {
	r := setupRouter()
	mock := &mockStateReader{
		services: []state.ServiceSnapshot{
			{
				Key:         "svc1.default.ctx",
				ServiceName: "svc1",
				ErrorCount:  4,
				PortForwards: []state.ForwardSnapshot{
					{PodName: "pod1", Error: "connection refused"},
					{PodName: "pod2", Error: "request timeout"},
					{PodName: "pod3", Error: "pod not found"},
					{PodName: "pod4", Error: "broken pipe error"},
				},
			},
		},
		summary: state.SummaryStats{
			TotalServices:  1,
			ActiveServices: 0,
			ErrorCount:     4,
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

func TestAnalyzeHandler_AnalyzeDegraded(t *testing.T) {
	r := setupRouter()
	mock := &mockStateReader{
		services: []state.ServiceSnapshot{
			{Key: "svc1.default.ctx", ServiceName: "svc1", ActiveCount: 1, ErrorCount: 0},
			{Key: "svc2.default.ctx", ServiceName: "svc2", ActiveCount: 0, ErrorCount: 1,
				PortForwards: []state.ForwardSnapshot{{PodName: "pod1", Error: "unknown error type"}}},
		},
		summary: state.SummaryStats{
			TotalServices:  2,
			ActiveServices: 1,
			ErrorCount:     1,
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
	// Status should be "degraded" because ErrorCount (1) <= ActiveServices (1)
	if data["status"] != "degraded" {
		t.Errorf("Expected status 'degraded', got '%v'", data["status"])
	}
}

// HTTPTrafficHandler edge case tests

func TestHTTPTrafficHandler_ForwardHTTPMetricsNotFound(t *testing.T) {
	r := setupRouter()
	stateMock := &mockStateReader{
		forwards: []state.ForwardSnapshot{
			{Key: "fwd1.svc.ns.ctx", ServiceKey: "svc.ns.ctx", PodName: "pod1"},
		},
	}
	metricsMock := &mockMetricsProvider{
		// No snapshots - service metrics not found
	}
	h := NewHTTPTrafficHandler(metricsMock, stateMock)
	r.GET("/v1/forwards/:key/http", h.ForwardHTTP)

	w := performRequest(r, "GET", "/v1/forwards/fwd1.svc.ns.ctx/http")

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestHTTPTrafficHandler_ForwardHTTPPortForwardNotFound(t *testing.T) {
	r := setupRouter()
	stateMock := &mockStateReader{
		forwards: []state.ForwardSnapshot{
			{Key: "fwd1.svc.ns.ctx", ServiceKey: "svc.ns.ctx", PodName: "pod1"},
		},
	}
	metricsMock := &mockMetricsProvider{
		snapshots: []fwdmetrics.ServiceSnapshot{
			{
				ServiceName: "svc",
				Namespace:   "ns",
				Context:     "ctx",
				// No PortForwards matching the key
				PortForwards: []fwdmetrics.PortForwardSnapshot{
					{PodName: "other-pod"},
				},
			},
		},
	}
	h := NewHTTPTrafficHandler(metricsMock, stateMock)
	r.GET("/v1/forwards/:key/http", h.ForwardHTTP)

	w := performRequest(r, "GET", "/v1/forwards/fwd1.svc.ns.ctx/http")

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

// Mock implementations for CRUD handlers

type mockNamespaceController struct {
	namespaces []types.NamespaceInfoResponse
	addErr     error
	removeErr  error
	getErr     error
}

func (m *mockNamespaceController) AddNamespace(ctx, namespace string, _ types.AddNamespaceOpts) (*types.NamespaceInfoResponse, error) {
	if m.addErr != nil {
		return nil, m.addErr
	}
	return &types.NamespaceInfoResponse{
		Key:       namespace + "." + ctx,
		Namespace: namespace,
		Context:   ctx,
	}, nil
}

func (m *mockNamespaceController) RemoveNamespace(_, _ string) error {
	return m.removeErr
}

func (m *mockNamespaceController) ListNamespaces() []types.NamespaceInfoResponse {
	return m.namespaces
}

func (m *mockNamespaceController) GetNamespace(ctx, namespace string) (*types.NamespaceInfoResponse, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	for i := range m.namespaces {
		if m.namespaces[i].Namespace == namespace && m.namespaces[i].Context == ctx {
			return &m.namespaces[i], nil
		}
	}
	return nil, fmt.Errorf("namespace not found")
}

type mockServiceCRUD struct {
	addErr    error
	removeErr error
}

func (m *mockServiceCRUD) Reconnect(_ string) error {
	return nil
}

func (m *mockServiceCRUD) ReconnectAll() int {
	return 0
}

func (m *mockServiceCRUD) Sync(_ string, _ bool) error {
	return nil
}

func (m *mockServiceCRUD) AddService(req types.AddServiceRequest) (*types.AddServiceResponse, error) {
	if m.addErr != nil {
		return nil, m.addErr
	}
	return &types.AddServiceResponse{
		Key:         req.ServiceName + "." + req.Namespace + ".default",
		ServiceName: req.ServiceName,
		Namespace:   req.Namespace,
		Context:     "default",
		LocalIP:     "127.1.0.1",
	}, nil
}

func (m *mockServiceCRUD) RemoveService(_ string) error {
	return m.removeErr
}

type mockKubernetesDiscovery struct {
	namespaces []types.K8sNamespace
	services   []types.K8sService
	contexts   *types.K8sContextsResponse
	listNsErr  error
	listSvcErr error
	getErr     error
	listCtxErr error
}

func (m *mockKubernetesDiscovery) ListNamespaces(_ string) ([]types.K8sNamespace, error) {
	if m.listNsErr != nil {
		return nil, m.listNsErr
	}
	return m.namespaces, nil
}

func (m *mockKubernetesDiscovery) ListServices(_, _ string) ([]types.K8sService, error) {
	if m.listSvcErr != nil {
		return nil, m.listSvcErr
	}
	return m.services, nil
}

func (m *mockKubernetesDiscovery) GetService(_, namespace, name string) (*types.K8sService, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	for i := range m.services {
		if m.services[i].Name == name && m.services[i].Namespace == namespace {
			return &m.services[i], nil
		}
	}
	return nil, fmt.Errorf("service not found")
}

func (m *mockKubernetesDiscovery) ListContexts() (*types.K8sContextsResponse, error) {
	if m.listCtxErr != nil {
		return nil, m.listCtxErr
	}
	return m.contexts, nil
}

func (m *mockKubernetesDiscovery) GetPodLogs(ctx, namespace, podName string, opts types.PodLogsOptions) (*types.PodLogsResponse, error) {
	return &types.PodLogsResponse{
		PodName:       podName,
		Namespace:     namespace,
		Context:       ctx,
		ContainerName: opts.Container,
		Logs:          []string{"mock log line 1", "mock log line 2"},
		LineCount:     2,
		Truncated:     false,
	}, nil
}

func (m *mockKubernetesDiscovery) ListPods(_, namespace string, _ types.ListPodsOptions) ([]types.K8sPod, error) {
	return []types.K8sPod{{Name: "test-pod", Namespace: namespace, Phase: "Running"}}, nil
}

func (m *mockKubernetesDiscovery) GetPod(_, namespace, podName string) (*types.K8sPodDetail, error) {
	return &types.K8sPodDetail{Name: podName, Namespace: namespace, Phase: "Running"}, nil
}

func (m *mockKubernetesDiscovery) GetEvents(_, _ string, _ types.GetEventsOptions) ([]types.K8sEvent, error) {
	return []types.K8sEvent{{Type: "Normal", Reason: "Scheduled"}}, nil
}

func (m *mockKubernetesDiscovery) GetEndpoints(_, namespace, serviceName string) (*types.K8sEndpoints, error) {
	return &types.K8sEndpoints{Name: serviceName, Namespace: namespace}, nil
}

// Namespaces Handler Tests

func TestNamespacesHandler_List(t *testing.T) {
	r := setupRouter()
	mock := &mockNamespaceController{
		namespaces: []types.NamespaceInfoResponse{
			{Key: "default.minikube", Namespace: "default", Context: "minikube", ServiceCount: 5},
			{Key: "kube-system.minikube", Namespace: "kube-system", Context: "minikube", ServiceCount: 10},
		},
	}
	h := NewNamespacesHandler(mock)
	r.GET("/v1/namespaces", h.List)

	w := performRequest(r, "GET", "/v1/namespaces")

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

func TestNamespacesHandler_ListNilController(t *testing.T) {
	r := setupRouter()
	h := NewNamespacesHandler(nil)
	r.GET("/v1/namespaces", h.List)

	w := performRequest(r, "GET", "/v1/namespaces")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestNamespacesHandler_Get(t *testing.T) {
	r := setupRouter()
	mock := &mockNamespaceController{
		namespaces: []types.NamespaceInfoResponse{
			{Key: "default.minikube", Namespace: "default", Context: "minikube", ServiceCount: 5},
		},
	}
	h := NewNamespacesHandler(mock)
	r.GET("/v1/namespaces/:key", h.Get)

	w := performRequest(r, "GET", "/v1/namespaces/default.minikube")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestNamespacesHandler_GetNilController(t *testing.T) {
	r := setupRouter()
	h := NewNamespacesHandler(nil)
	r.GET("/v1/namespaces/:key", h.Get)

	w := performRequest(r, "GET", "/v1/namespaces/default.minikube")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestNamespacesHandler_GetInvalidKey(t *testing.T) {
	r := setupRouter()
	mock := &mockNamespaceController{}
	h := NewNamespacesHandler(mock)
	r.GET("/v1/namespaces/:key", h.Get)

	w := performRequest(r, "GET", "/v1/namespaces/invalidkey")

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestNamespacesHandler_GetNotFound(t *testing.T) {
	r := setupRouter()
	mock := &mockNamespaceController{
		getErr: fmt.Errorf("namespace not found"),
	}
	h := NewNamespacesHandler(mock)
	r.GET("/v1/namespaces/:key", h.Get)

	w := performRequest(r, "GET", "/v1/namespaces/nonexistent.ctx")

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestNamespacesHandler_Add(t *testing.T) {
	r := setupRouter()
	mock := &mockNamespaceController{}
	h := NewNamespacesHandler(mock)
	r.POST("/v1/namespaces", h.Add)

	w := performRequestWithBody(r, "POST", "/v1/namespaces", `{"namespace":"default"}`)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status %d, got %d", http.StatusCreated, w.Code)
	}

	var response types.Response
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if !response.Success {
		t.Errorf("Expected success true, got false")
	}
}

func TestNamespacesHandler_AddWithContext(t *testing.T) {
	r := setupRouter()
	mock := &mockNamespaceController{}
	h := NewNamespacesHandler(mock)
	r.POST("/v1/namespaces", h.Add)

	w := performRequestWithBody(r, "POST", "/v1/namespaces", `{"namespace":"staging","context":"prod-cluster"}`)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status %d, got %d", http.StatusCreated, w.Code)
	}
}

func TestNamespacesHandler_AddNilController(t *testing.T) {
	r := setupRouter()
	h := NewNamespacesHandler(nil)
	r.POST("/v1/namespaces", h.Add)

	w := performRequestWithBody(r, "POST", "/v1/namespaces", `{"namespace":"default"}`)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestNamespacesHandler_AddInvalidRequest(t *testing.T) {
	r := setupRouter()
	mock := &mockNamespaceController{}
	h := NewNamespacesHandler(mock)
	r.POST("/v1/namespaces", h.Add)

	w := performRequestWithBody(r, "POST", "/v1/namespaces", `invalid json`)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestNamespacesHandler_AddError(t *testing.T) {
	r := setupRouter()
	mock := &mockNamespaceController{
		addErr: fmt.Errorf("namespace already exists"),
	}
	h := NewNamespacesHandler(mock)
	r.POST("/v1/namespaces", h.Add)

	w := performRequestWithBody(r, "POST", "/v1/namespaces", `{"namespace":"default"}`)

	if w.Code != http.StatusConflict {
		t.Errorf("Expected status %d, got %d", http.StatusConflict, w.Code)
	}
}

func TestNamespacesHandler_Remove(t *testing.T) {
	r := setupRouter()
	mock := &mockNamespaceController{}
	h := NewNamespacesHandler(mock)
	r.DELETE("/v1/namespaces/:key", h.Remove)

	w := performRequest(r, "DELETE", "/v1/namespaces/default.minikube")

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

func TestNamespacesHandler_RemoveNilController(t *testing.T) {
	r := setupRouter()
	h := NewNamespacesHandler(nil)
	r.DELETE("/v1/namespaces/:key", h.Remove)

	w := performRequest(r, "DELETE", "/v1/namespaces/default.minikube")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestNamespacesHandler_RemoveInvalidKey(t *testing.T) {
	r := setupRouter()
	mock := &mockNamespaceController{}
	h := NewNamespacesHandler(mock)
	r.DELETE("/v1/namespaces/:key", h.Remove)

	w := performRequest(r, "DELETE", "/v1/namespaces/invalidkey")

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestNamespacesHandler_RemoveError(t *testing.T) {
	r := setupRouter()
	mock := &mockNamespaceController{
		removeErr: fmt.Errorf("namespace not found"),
	}
	h := NewNamespacesHandler(mock)
	r.DELETE("/v1/namespaces/:key", h.Remove)

	w := performRequest(r, "DELETE", "/v1/namespaces/nonexistent.ctx")

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

// Services CRUD Handler Tests

func TestServicesCRUDHandler_Add(t *testing.T) {
	r := setupRouter()
	mock := &mockServiceCRUD{}
	h := NewServicesCRUDHandler(mock)
	r.POST("/v1/services", h.Add)

	w := performRequestWithBody(r, "POST", "/v1/services", `{"namespace":"default","serviceName":"postgres"}`)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status %d, got %d", http.StatusCreated, w.Code)
	}

	var response types.Response
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if !response.Success {
		t.Errorf("Expected success true, got false")
	}
}

func TestServicesCRUDHandler_AddNilController(t *testing.T) {
	r := setupRouter()
	h := NewServicesCRUDHandler(nil)
	r.POST("/v1/services", h.Add)

	w := performRequestWithBody(r, "POST", "/v1/services", `{"namespace":"default","serviceName":"postgres"}`)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestServicesCRUDHandler_AddInvalidRequest(t *testing.T) {
	r := setupRouter()
	mock := &mockServiceCRUD{}
	h := NewServicesCRUDHandler(mock)
	r.POST("/v1/services", h.Add)

	w := performRequestWithBody(r, "POST", "/v1/services", `invalid json`)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestServicesCRUDHandler_AddError(t *testing.T) {
	r := setupRouter()
	mock := &mockServiceCRUD{
		addErr: fmt.Errorf("service already exists"),
	}
	h := NewServicesCRUDHandler(mock)
	r.POST("/v1/services", h.Add)

	w := performRequestWithBody(r, "POST", "/v1/services", `{"namespace":"default","serviceName":"postgres"}`)

	if w.Code != http.StatusConflict {
		t.Errorf("Expected status %d, got %d", http.StatusConflict, w.Code)
	}
}

func TestServicesCRUDHandler_Remove(t *testing.T) {
	r := setupRouter()
	mock := &mockServiceCRUD{}
	h := NewServicesCRUDHandler(mock)
	r.DELETE("/v1/services/:key", h.Remove)

	w := performRequest(r, "DELETE", "/v1/services/postgres.default.minikube")

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

func TestServicesCRUDHandler_RemoveNilController(t *testing.T) {
	r := setupRouter()
	h := NewServicesCRUDHandler(nil)
	r.DELETE("/v1/services/:key", h.Remove)

	w := performRequest(r, "DELETE", "/v1/services/postgres.default.minikube")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestServicesCRUDHandler_RemoveError(t *testing.T) {
	r := setupRouter()
	mock := &mockServiceCRUD{
		removeErr: fmt.Errorf("service not found"),
	}
	h := NewServicesCRUDHandler(mock)
	r.DELETE("/v1/services/:key", h.Remove)

	w := performRequest(r, "DELETE", "/v1/services/nonexistent")

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestServicesCRUDHandler_RemoveEmptyKey(t *testing.T) {
	r := setupRouter()
	mock := &mockServiceCRUD{}
	h := NewServicesCRUDHandler(mock)
	// Note: gin requires at least one character for :key param, so we use a route without the param
	r.DELETE("/v1/services/", h.Remove)

	w := performRequest(r, "DELETE", "/v1/services/")

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

// Kubernetes Handler Tests

func TestKubernetesHandler_ListNamespaces(t *testing.T) {
	r := setupRouter()
	mock := &mockKubernetesDiscovery{
		namespaces: []types.K8sNamespace{
			{Name: "default", Status: "Active", Forwarded: true},
			{Name: "kube-system", Status: "Active", Forwarded: false},
		},
	}
	h := NewKubernetesHandler(mock)
	r.GET("/v1/kubernetes/namespaces", h.ListNamespaces)

	w := performRequest(r, "GET", "/v1/kubernetes/namespaces")

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

func TestKubernetesHandler_ListNamespacesNilDiscovery(t *testing.T) {
	r := setupRouter()
	h := NewKubernetesHandler(nil)
	r.GET("/v1/kubernetes/namespaces", h.ListNamespaces)

	w := performRequest(r, "GET", "/v1/kubernetes/namespaces")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestKubernetesHandler_ListNamespacesWithContext(t *testing.T) {
	r := setupRouter()
	mock := &mockKubernetesDiscovery{
		namespaces: []types.K8sNamespace{
			{Name: "default", Status: "Active"},
		},
	}
	h := NewKubernetesHandler(mock)
	r.GET("/v1/kubernetes/namespaces", h.ListNamespaces)

	w := performRequest(r, "GET", "/v1/kubernetes/namespaces?context=prod-cluster")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestKubernetesHandler_ListNamespacesError(t *testing.T) {
	r := setupRouter()
	mock := &mockKubernetesDiscovery{
		listNsErr: fmt.Errorf("connection refused"),
	}
	h := NewKubernetesHandler(mock)
	r.GET("/v1/kubernetes/namespaces", h.ListNamespaces)

	w := performRequest(r, "GET", "/v1/kubernetes/namespaces")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}
}

func TestKubernetesHandler_ListServices(t *testing.T) {
	r := setupRouter()
	mock := &mockKubernetesDiscovery{
		services: []types.K8sService{
			{Name: "postgres", Namespace: "default", Type: "ClusterIP", Forwarded: true},
			{Name: "redis", Namespace: "default", Type: "ClusterIP", Forwarded: false},
		},
	}
	h := NewKubernetesHandler(mock)
	r.GET("/v1/kubernetes/services", h.ListServices)

	w := performRequest(r, "GET", "/v1/kubernetes/services?namespace=default")

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

func TestKubernetesHandler_ListServicesNilDiscovery(t *testing.T) {
	r := setupRouter()
	h := NewKubernetesHandler(nil)
	r.GET("/v1/kubernetes/services", h.ListServices)

	w := performRequest(r, "GET", "/v1/kubernetes/services?namespace=default")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestKubernetesHandler_ListServicesMissingNamespace(t *testing.T) {
	r := setupRouter()
	mock := &mockKubernetesDiscovery{}
	h := NewKubernetesHandler(mock)
	r.GET("/v1/kubernetes/services", h.ListServices)

	w := performRequest(r, "GET", "/v1/kubernetes/services")

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestKubernetesHandler_ListServicesError(t *testing.T) {
	r := setupRouter()
	mock := &mockKubernetesDiscovery{
		listSvcErr: fmt.Errorf("permission denied"),
	}
	h := NewKubernetesHandler(mock)
	r.GET("/v1/kubernetes/services", h.ListServices)

	w := performRequest(r, "GET", "/v1/kubernetes/services?namespace=default")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}
}

func TestKubernetesHandler_GetService(t *testing.T) {
	r := setupRouter()
	mock := &mockKubernetesDiscovery{
		services: []types.K8sService{
			{Name: "postgres", Namespace: "default", Type: "ClusterIP"},
		},
	}
	h := NewKubernetesHandler(mock)
	r.GET("/v1/kubernetes/services/:namespace/:name", h.GetService)

	w := performRequest(r, "GET", "/v1/kubernetes/services/default/postgres")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestKubernetesHandler_GetServiceNilDiscovery(t *testing.T) {
	r := setupRouter()
	h := NewKubernetesHandler(nil)
	r.GET("/v1/kubernetes/services/:namespace/:name", h.GetService)

	w := performRequest(r, "GET", "/v1/kubernetes/services/default/postgres")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestKubernetesHandler_GetServiceNotFound(t *testing.T) {
	r := setupRouter()
	mock := &mockKubernetesDiscovery{
		getErr: fmt.Errorf("service not found"),
	}
	h := NewKubernetesHandler(mock)
	r.GET("/v1/kubernetes/services/:namespace/:name", h.GetService)

	w := performRequest(r, "GET", "/v1/kubernetes/services/default/nonexistent")

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestKubernetesHandler_ListContexts(t *testing.T) {
	r := setupRouter()
	mock := &mockKubernetesDiscovery{
		contexts: &types.K8sContextsResponse{
			CurrentContext: "minikube",
			Contexts: []types.K8sContext{
				{Name: "minikube", Cluster: "minikube", Active: true},
				{Name: "prod-cluster", Cluster: "prod", Active: false},
			},
		},
	}
	h := NewKubernetesHandler(mock)
	r.GET("/v1/kubernetes/contexts", h.ListContexts)

	w := performRequest(r, "GET", "/v1/kubernetes/contexts")

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

func TestKubernetesHandler_ListContextsNilDiscovery(t *testing.T) {
	r := setupRouter()
	h := NewKubernetesHandler(nil)
	r.GET("/v1/kubernetes/contexts", h.ListContexts)

	w := performRequest(r, "GET", "/v1/kubernetes/contexts")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestKubernetesHandler_ListContextsError(t *testing.T) {
	r := setupRouter()
	mock := &mockKubernetesDiscovery{
		listCtxErr: fmt.Errorf("kubeconfig not found"),
	}
	h := NewKubernetesHandler(mock)
	r.GET("/v1/kubernetes/contexts", h.ListContexts)

	w := performRequest(r, "GET", "/v1/kubernetes/contexts")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}
}

// Test parseNamespaceKey function

func TestParseNamespaceKey(t *testing.T) {
	tests := []struct {
		key       string
		namespace string
		context   string
	}{
		{"default.minikube", "default", "minikube"},
		{"kube-system.prod-cluster", "kube-system", "prod-cluster"},
		{"my-ns.my-context", "my-ns", "my-context"},
		{"invalidkey", "", ""},
		{"", "", ""},
	}

	for _, tt := range tests {
		ns, ctx := parseNamespaceKey(tt.key)
		if ns != tt.namespace || ctx != tt.context {
			t.Errorf("parseNamespaceKey(%q) = (%q, %q), want (%q, %q)",
				tt.key, ns, ctx, tt.namespace, tt.context)
		}
	}
}

// Helper function for requests with body

func performRequestWithBody(r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// Mock implementation for LogBufferProvider

type mockLogBufferProvider struct {
	entries []types.LogBufferEntry
	cleared bool
}

func (m *mockLogBufferProvider) GetLast(n int) []types.LogBufferEntry {
	if n >= len(m.entries) {
		return m.entries
	}
	return m.entries[:n]
}

func (m *mockLogBufferProvider) Count() int {
	return len(m.entries)
}

func (m *mockLogBufferProvider) Clear() {
	m.cleared = true
	m.entries = nil
}

// LogsHandler System and ClearSystem tests

func TestLogsHandler_System(t *testing.T) {
	r := setupRouter()
	mockBuffer := &mockLogBufferProvider{
		entries: []types.LogBufferEntry{
			{Timestamp: time.Now(), Level: "info", Message: "Test message 1"},
			{Timestamp: time.Now(), Level: "error", Message: "Test error message"},
			{Timestamp: time.Now(), Level: "info", Message: "Test message 2"},
		},
	}
	h := NewLogsHandler(nil, func() types.LogBufferProvider { return mockBuffer })
	r.GET("/v1/logs/system", h.System)

	w := performRequest(r, "GET", "/v1/logs/system")

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

func TestLogsHandler_SystemWithCount(t *testing.T) {
	r := setupRouter()
	mockBuffer := &mockLogBufferProvider{
		entries: []types.LogBufferEntry{
			{Timestamp: time.Now(), Level: "info", Message: "Test message 1"},
			{Timestamp: time.Now(), Level: "info", Message: "Test message 2"},
			{Timestamp: time.Now(), Level: "info", Message: "Test message 3"},
		},
	}
	h := NewLogsHandler(nil, func() types.LogBufferProvider { return mockBuffer })
	r.GET("/v1/logs/system", h.System)

	w := performRequest(r, "GET", "/v1/logs/system?count=2")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestLogsHandler_SystemWithLevelFilter(t *testing.T) {
	r := setupRouter()
	mockBuffer := &mockLogBufferProvider{
		entries: []types.LogBufferEntry{
			{Timestamp: time.Now(), Level: "info", Message: "Test message 1"},
			{Timestamp: time.Now(), Level: "error", Message: "Test error message"},
			{Timestamp: time.Now(), Level: "info", Message: "Test message 2"},
		},
	}
	h := NewLogsHandler(nil, func() types.LogBufferProvider { return mockBuffer })
	r.GET("/v1/logs/system", h.System)

	w := performRequest(r, "GET", "/v1/logs/system?level=error")

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

func TestLogsHandler_SystemNilBuffer(t *testing.T) {
	r := setupRouter()
	h := NewLogsHandler(nil, func() types.LogBufferProvider { return nil })
	r.GET("/v1/logs/system", h.System)

	w := performRequest(r, "GET", "/v1/logs/system")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestLogsHandler_SystemNilBufferGetter(t *testing.T) {
	r := setupRouter()
	h := NewLogsHandler(nil, nil)
	r.GET("/v1/logs/system", h.System)

	w := performRequest(r, "GET", "/v1/logs/system")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestLogsHandler_SystemInvalidCount(t *testing.T) {
	r := setupRouter()
	mockBuffer := &mockLogBufferProvider{
		entries: []types.LogBufferEntry{
			{Timestamp: time.Now(), Level: "info", Message: "Test message"},
		},
	}
	h := NewLogsHandler(nil, func() types.LogBufferProvider { return mockBuffer })
	r.GET("/v1/logs/system", h.System)

	w := performRequest(r, "GET", "/v1/logs/system?count=invalid")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d (should use default count)", http.StatusOK, w.Code)
	}
}

func TestLogsHandler_SystemMaxCount(t *testing.T) {
	r := setupRouter()
	mockBuffer := &mockLogBufferProvider{
		entries: []types.LogBufferEntry{
			{Timestamp: time.Now(), Level: "info", Message: "Test message"},
		},
	}
	h := NewLogsHandler(nil, func() types.LogBufferProvider { return mockBuffer })
	r.GET("/v1/logs/system", h.System)

	w := performRequest(r, "GET", "/v1/logs/system?count=9999")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d (should cap at 1000)", http.StatusOK, w.Code)
	}
}

func TestLogsHandler_ClearSystem(t *testing.T) {
	r := setupRouter()
	mockBuffer := &mockLogBufferProvider{
		entries: []types.LogBufferEntry{
			{Timestamp: time.Now(), Level: "info", Message: "Test message"},
		},
	}
	h := NewLogsHandler(nil, func() types.LogBufferProvider { return mockBuffer })
	r.DELETE("/v1/logs/system", h.ClearSystem)

	w := performRequest(r, "DELETE", "/v1/logs/system")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	if !mockBuffer.cleared {
		t.Errorf("Expected buffer to be cleared")
	}
}

func TestLogsHandler_ClearSystemNilBuffer(t *testing.T) {
	r := setupRouter()
	h := NewLogsHandler(nil, func() types.LogBufferProvider { return nil })
	r.DELETE("/v1/logs/system", h.ClearSystem)

	w := performRequest(r, "DELETE", "/v1/logs/system")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestLogsHandler_ClearSystemNilBufferGetter(t *testing.T) {
	r := setupRouter()
	h := NewLogsHandler(nil, nil)
	r.DELETE("/v1/logs/system", h.ClearSystem)

	w := performRequest(r, "DELETE", "/v1/logs/system")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

// KubernetesHandler additional endpoint tests

func TestKubernetesHandler_GetPodLogs(t *testing.T) {
	r := setupRouter()
	mock := &mockKubernetesDiscovery{}
	h := NewKubernetesHandler(mock)
	r.GET("/v1/kubernetes/pods/:namespace/:podName/logs", h.GetPodLogs)

	w := performRequest(r, "GET", "/v1/kubernetes/pods/default/my-pod/logs")

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

func TestKubernetesHandler_GetPodLogsWithOptions(t *testing.T) {
	r := setupRouter()
	mock := &mockKubernetesDiscovery{}
	h := NewKubernetesHandler(mock)
	r.GET("/v1/kubernetes/pods/:namespace/:podName/logs", h.GetPodLogs)

	w := performRequest(r, "GET", "/v1/kubernetes/pods/default/my-pod/logs?container=app&tail_lines=100&previous=true&timestamps=true")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestKubernetesHandler_GetPodLogsNilDiscovery(t *testing.T) {
	r := setupRouter()
	h := NewKubernetesHandler(nil)
	r.GET("/v1/kubernetes/pods/:namespace/:podName/logs", h.GetPodLogs)

	w := performRequest(r, "GET", "/v1/kubernetes/pods/default/my-pod/logs")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestKubernetesHandler_GetPodLogsMissingParams(t *testing.T) {
	r := setupRouter()
	mock := &mockKubernetesDiscovery{}
	h := NewKubernetesHandler(mock)
	// Missing podName parameter
	r.GET("/v1/kubernetes/pods/:namespace/logs", h.GetPodLogs)

	w := performRequest(r, "GET", "/v1/kubernetes/pods/default/logs")

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestKubernetesHandler_ListPods(t *testing.T) {
	r := setupRouter()
	mock := &mockKubernetesDiscovery{}
	h := NewKubernetesHandler(mock)
	r.GET("/v1/kubernetes/pods/:namespace", h.ListPods)

	w := performRequest(r, "GET", "/v1/kubernetes/pods/default")

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

func TestKubernetesHandler_ListPodsWithOptions(t *testing.T) {
	r := setupRouter()
	mock := &mockKubernetesDiscovery{}
	h := NewKubernetesHandler(mock)
	r.GET("/v1/kubernetes/pods/:namespace", h.ListPods)

	w := performRequest(r, "GET", "/v1/kubernetes/pods/default?label_selector=app%3Dtest&service_name=my-svc")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestKubernetesHandler_ListPodsNilDiscovery(t *testing.T) {
	r := setupRouter()
	h := NewKubernetesHandler(nil)
	r.GET("/v1/kubernetes/pods/:namespace", h.ListPods)

	w := performRequest(r, "GET", "/v1/kubernetes/pods/default")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestKubernetesHandler_ListPodsMissingNamespace(t *testing.T) {
	r := setupRouter()
	mock := &mockKubernetesDiscovery{}
	h := NewKubernetesHandler(mock)
	r.GET("/v1/kubernetes/pods", h.ListPods)

	w := performRequest(r, "GET", "/v1/kubernetes/pods")

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestKubernetesHandler_GetPod(t *testing.T) {
	r := setupRouter()
	mock := &mockKubernetesDiscovery{}
	h := NewKubernetesHandler(mock)
	r.GET("/v1/kubernetes/pods/:namespace/:podName", h.GetPod)

	w := performRequest(r, "GET", "/v1/kubernetes/pods/default/my-pod")

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

func TestKubernetesHandler_GetPodNilDiscovery(t *testing.T) {
	r := setupRouter()
	h := NewKubernetesHandler(nil)
	r.GET("/v1/kubernetes/pods/:namespace/:podName", h.GetPod)

	w := performRequest(r, "GET", "/v1/kubernetes/pods/default/my-pod")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestKubernetesHandler_GetPodMissingParams(t *testing.T) {
	r := setupRouter()
	mock := &mockKubernetesDiscovery{}
	h := NewKubernetesHandler(mock)
	r.GET("/v1/kubernetes/pods/:namespace", h.GetPod)

	w := performRequest(r, "GET", "/v1/kubernetes/pods/default")

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestKubernetesHandler_GetEvents(t *testing.T) {
	r := setupRouter()
	mock := &mockKubernetesDiscovery{}
	h := NewKubernetesHandler(mock)
	r.GET("/v1/kubernetes/events/:namespace", h.GetEvents)

	w := performRequest(r, "GET", "/v1/kubernetes/events/default")

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

func TestKubernetesHandler_GetEventsWithOptions(t *testing.T) {
	r := setupRouter()
	mock := &mockKubernetesDiscovery{}
	h := NewKubernetesHandler(mock)
	r.GET("/v1/kubernetes/events/:namespace", h.GetEvents)

	w := performRequest(r, "GET", "/v1/kubernetes/events/default?resource_kind=Pod&resource_name=my-pod&limit=50")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestKubernetesHandler_GetEventsNilDiscovery(t *testing.T) {
	r := setupRouter()
	h := NewKubernetesHandler(nil)
	r.GET("/v1/kubernetes/events/:namespace", h.GetEvents)

	w := performRequest(r, "GET", "/v1/kubernetes/events/default")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestKubernetesHandler_GetEventsMissingNamespace(t *testing.T) {
	r := setupRouter()
	mock := &mockKubernetesDiscovery{}
	h := NewKubernetesHandler(mock)
	r.GET("/v1/kubernetes/events", h.GetEvents)

	w := performRequest(r, "GET", "/v1/kubernetes/events")

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestKubernetesHandler_GetEndpoints(t *testing.T) {
	r := setupRouter()
	mock := &mockKubernetesDiscovery{}
	h := NewKubernetesHandler(mock)
	r.GET("/v1/kubernetes/endpoints/:namespace/:serviceName", h.GetEndpoints)

	w := performRequest(r, "GET", "/v1/kubernetes/endpoints/default/my-service")

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

func TestKubernetesHandler_GetEndpointsNilDiscovery(t *testing.T) {
	r := setupRouter()
	h := NewKubernetesHandler(nil)
	r.GET("/v1/kubernetes/endpoints/:namespace/:serviceName", h.GetEndpoints)

	w := performRequest(r, "GET", "/v1/kubernetes/endpoints/default/my-service")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestKubernetesHandler_GetEndpointsMissingParams(t *testing.T) {
	r := setupRouter()
	mock := &mockKubernetesDiscovery{}
	h := NewKubernetesHandler(mock)
	r.GET("/v1/kubernetes/endpoints/:namespace", h.GetEndpoints)

	w := performRequest(r, "GET", "/v1/kubernetes/endpoints/default")

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

// LogsHandler Stream tests (partial - SSE is complex to fully test)

func TestLogsHandler_StreamNilState(t *testing.T) {
	r := setupRouter()
	h := NewLogsHandler(nil, nil)
	r.GET("/v1/logs/stream", h.Stream)

	w := performRequest(r, "GET", "/v1/logs/stream")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

// mockEventStreamer for testing SSE event streaming
type mockEventStreamer struct {
	eventCh      chan events.Event
	closed       bool
	subscribedTo events.EventType
}

func newMockEventStreamer() *mockEventStreamer {
	return &mockEventStreamer{
		eventCh: make(chan events.Event, 10),
	}
}

func (m *mockEventStreamer) Subscribe() (<-chan events.Event, func()) {
	return m.eventCh, func() { m.closed = true }
}

func (m *mockEventStreamer) SubscribeType(eventType events.EventType) (<-chan events.Event, func()) {
	m.subscribedTo = eventType
	return m.eventCh, func() { m.closed = true }
}

// EventsHandler tests

func TestEventsHandler_StreamNilStreamer(t *testing.T) {
	r := setupRouter()
	h := NewEventsHandler(nil)
	r.GET("/v1/events/stream", h.Stream)

	w := performRequest(r, "GET", "/v1/events/stream")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}

	var response types.Response
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response.Success {
		t.Error("Expected success=false")
	}
	if response.Error == nil || response.Error.Code != "NOT_READY" {
		t.Error("Expected NOT_READY error code")
	}
}

func TestEventsHandler_StreamClosedChannel(t *testing.T) {
	r := setupRouter()
	streamer := newMockEventStreamer()
	close(streamer.eventCh) // Close channel before request
	h := NewEventsHandler(streamer)
	r.GET("/v1/events/stream", h.Stream)

	w := performRequest(r, "GET", "/v1/events/stream")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}

	var response types.Response
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response.Error == nil || response.Error.Code != "EVENT_BUS_UNAVAILABLE" {
		t.Error("Expected EVENT_BUS_UNAVAILABLE error code")
	}
}

func TestEventsHandler_StreamWithTypeFilter(t *testing.T) {
	// This test verifies that SubscribeType is called with the correct filter.
	// Full SSE streaming tests are complex due to httptest.ResponseRecorder
	// not implementing CloseNotifier. Instead we test via closed channel.
	streamer := newMockEventStreamer()

	// Close the channel immediately to trigger the closed check
	close(streamer.eventCh)

	r := setupRouter()
	h := NewEventsHandler(streamer)
	r.GET("/v1/events/stream", h.Stream)

	req := httptest.NewRequest("GET", "/v1/events/stream?type=ServiceAdded", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Verify subscription type was set before channel closed
	if streamer.subscribedTo != events.ServiceAdded {
		t.Errorf("Expected subscription to ServiceAdded, got %v", streamer.subscribedTo)
	}

	// Should get error response since channel was closed
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

// Test mapEventToResponse function
func TestMapEventToResponse_BasicFields(t *testing.T) {
	now := time.Now()
	e := events.Event{
		Type:       events.ServiceAdded,
		Timestamp:  now,
		ServiceKey: "myservice.default.ctx",
		Service:    "myservice",
		Namespace:  "default",
		Context:    "ctx",
	}

	resp := mapEventToResponse(e)

	if resp.Type != "ServiceAdded" {
		t.Errorf("Expected type 'ServiceAdded', got '%s'", resp.Type)
	}
	if !resp.Timestamp.Equal(now) {
		t.Error("Timestamp mismatch")
	}
	if resp.Data["serviceKey"] != "myservice.default.ctx" {
		t.Errorf("Expected serviceKey 'myservice.default.ctx', got '%v'", resp.Data["serviceKey"])
	}
}

func TestMapEventToResponse_AllOptionalFields(t *testing.T) {
	err := fmt.Errorf("test error")
	e := events.Event{
		Type:          events.PodAdded,
		Timestamp:     time.Now(),
		ServiceKey:    "svc.ns.ctx",
		Service:       "svc",
		Namespace:     "ns",
		Context:       "ctx",
		RegistryKey:   "registry-key",
		PodName:       "pod-1",
		ContainerName: "container-1",
		LocalIP:       "127.0.0.1",
		LocalPort:     "8080",
		PodPort:       "80",
		Hostnames:     []string{"svc", "svc.ns"},
		Status:        "Running",
		Error:         err,
		BytesIn:       1024,
		BytesOut:      2048,
		RateIn:        100.5,
		RateOut:       200.5,
	}

	resp := mapEventToResponse(e)

	if resp.Data["registryKey"] != "registry-key" {
		t.Error("Missing registryKey")
	}
	if resp.Data["podName"] != "pod-1" {
		t.Error("Missing podName")
	}
	if resp.Data["containerName"] != "container-1" {
		t.Error("Missing containerName")
	}
	if resp.Data["localIP"] != "127.0.0.1" {
		t.Error("Missing localIP")
	}
	if resp.Data["localPort"] != "8080" {
		t.Error("Missing localPort")
	}
	if resp.Data["podPort"] != "80" {
		t.Error("Missing podPort")
	}
	hostnames, ok := resp.Data["hostnames"].([]string)
	if !ok || len(hostnames) != 2 {
		t.Error("Missing or invalid hostnames")
	}
	if resp.Data["status"] != "Running" {
		t.Error("Missing status")
	}
	if resp.Data["error"] != "test error" {
		t.Error("Missing error")
	}
	if resp.Data["bytesIn"] != uint64(1024) {
		t.Error("Missing bytesIn")
	}
	if resp.Data["bytesOut"] != uint64(2048) {
		t.Error("Missing bytesOut")
	}
	if resp.Data["rateIn"] != 100.5 {
		t.Error("Missing rateIn")
	}
	if resp.Data["rateOut"] != 200.5 {
		t.Error("Missing rateOut")
	}
}

func TestMapEventToResponse_EmptyOptionalFields(t *testing.T) {
	e := events.Event{
		Type:       events.ServiceRemoved,
		Timestamp:  time.Now(),
		ServiceKey: "svc.ns.ctx",
		Service:    "svc",
		Namespace:  "ns",
		Context:    "ctx",
	}

	resp := mapEventToResponse(e)

	// These fields should not be present when empty/zero
	if _, exists := resp.Data["registryKey"]; exists {
		t.Error("Empty registryKey should not be included")
	}
	if _, exists := resp.Data["podName"]; exists {
		t.Error("Empty podName should not be included")
	}
	if _, exists := resp.Data["error"]; exists {
		t.Error("Nil error should not be included")
	}
	if _, exists := resp.Data["bytesIn"]; exists {
		t.Error("Zero bytesIn should not be included")
	}
	if _, exists := resp.Data["rateIn"]; exists {
		t.Error("Zero rateIn should not be included")
	}
}

// Test parseEventType function
func TestParseEventType_AllTypes(t *testing.T) {
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
		{"UnknownType", events.PodStatusChanged}, // Default
		{"", events.PodStatusChanged},            // Empty defaults
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseEventType(tt.input)
			if result != tt.expected {
				t.Errorf("parseEventType(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

// Note: Full SSE streaming tests for LogsHandler.Stream require a real HTTP server
// because httptest.ResponseRecorder does not implement http.CloseNotifier.
// The nil state path is tested in TestLogsHandler_StreamNilState above.
