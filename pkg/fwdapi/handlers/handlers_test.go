package handlers

import (
	"encoding/json"
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
