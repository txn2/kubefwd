package fwdmcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/txn2/kubefwd/pkg/fwdapi/types"
	"github.com/txn2/kubefwd/pkg/fwdtui/state"
)

// TestHTTPClient_Get tests the HTTP client Get method
func TestHTTPClient_Get(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET method, got %s", r.Method)
		}
		if r.URL.Path != "/test" {
			t.Errorf("Expected /test path, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL)

	var result struct {
		Status string `json:"status"`
	}
	err := client.Get("/test", &result)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result.Status != "ok" {
		t.Errorf("Expected status 'ok', got '%s'", result.Status)
	}
}

// TestHTTPClient_Get_Error tests error handling
func TestHTTPClient_Get_Error(t *testing.T) {
	// Create test server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "internal error"})
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL)

	var result interface{}
	err := client.Get("/test", &result)
	if err == nil {
		t.Error("Expected error, got nil")
	}
}

// TestHTTPClient_Post tests the HTTP client Post method
func TestHTTPClient_Post(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST method, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"result": "success"})
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL)

	var result struct {
		Result string `json:"result"`
	}
	err := client.Post("/action", &result)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result.Result != "success" {
		t.Errorf("Expected result 'success', got '%s'", result.Result)
	}
}

// TestStateReaderHTTP_GetServices tests getting services
func TestStateReaderHTTP_GetServices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/services" {
			t.Errorf("Expected /v1/services, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool                      `json:"success"`
			Data    types.ServiceListResponse `json:"data"`
		}{
			Success: true,
			Data: types.ServiceListResponse{
				Services: []types.ServiceResponse{
					{
						Key:         "svc1.default.ctx1",
						ServiceName: "svc1",
						Namespace:   "default",
						Context:     "ctx1",
						ActiveCount: 1,
						ErrorCount:  0,
					},
				},
			},
		})
	}))
	defer server.Close()

	reader := NewStateReaderHTTP(server.URL)
	services := reader.GetServices()

	if len(services) != 1 {
		t.Fatalf("Expected 1 service, got %d", len(services))
	}
	if services[0].ServiceName != "svc1" {
		t.Errorf("Expected service name 'svc1', got '%s'", services[0].ServiceName)
	}
}

// TestStateReaderHTTP_GetService tests getting a single service
func TestStateReaderHTTP_GetService(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/services/svc1.default.ctx1" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool                  `json:"success"`
			Data    types.ServiceResponse `json:"data"`
		}{
			Success: true,
			Data: types.ServiceResponse{
				Key:         "svc1.default.ctx1",
				ServiceName: "svc1",
				Namespace:   "default",
				Context:     "ctx1",
			},
		})
	}))
	defer server.Close()

	reader := NewStateReaderHTTP(server.URL)

	// Test existing service
	svc := reader.GetService("svc1.default.ctx1")
	if svc == nil {
		t.Fatal("Expected non-nil service")
	}
	if svc.ServiceName != "svc1" {
		t.Errorf("Expected service name 'svc1', got '%s'", svc.ServiceName)
	}

	// Test non-existing service
	svc = reader.GetService("nonexistent")
	if svc != nil {
		t.Error("Expected nil for non-existent service")
	}
}

// TestStateReaderHTTP_GetSummary tests getting summary stats
func TestStateReaderHTTP_GetSummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool               `json:"success"`
			Data    types.InfoResponse `json:"data"`
		}{
			Success: true,
			Data: types.InfoResponse{
				Version:    "1.0.0",
				TUIEnabled: true,
			},
		})
	}))
	defer server.Close()

	reader := NewStateReaderHTTP(server.URL)
	summary := reader.GetSummary()

	// Summary is built from /info endpoint - just verify it doesn't panic
	_ = summary
}

// TestStateReaderHTTP_GetFiltered tests getting forwards
func TestStateReaderHTTP_GetFiltered(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool                      `json:"success"`
			Data    types.ForwardListResponse `json:"data"`
		}{
			Success: true,
			Data: types.ForwardListResponse{
				Forwards: []types.ForwardResponse{
					{
						Key:         "fwd1",
						ServiceKey:  "svc1.default.ctx1",
						ServiceName: "svc1",
						Namespace:   "default",
						Context:     "ctx1",
						PodName:     "pod1",
						LocalIP:     "127.1.0.1",
						LocalPort:   "80",
						PodPort:     "8080",
						Status:      "active",
					},
				},
			},
		})
	}))
	defer server.Close()

	reader := NewStateReaderHTTP(server.URL)
	forwards := reader.GetFiltered()

	if len(forwards) != 1 {
		t.Fatalf("Expected 1 forward, got %d", len(forwards))
	}
	if forwards[0].PodName != "pod1" {
		t.Errorf("Expected pod name 'pod1', got '%s'", forwards[0].PodName)
	}
}

// TestStateReaderHTTP_GetLogs tests getting logs
func TestStateReaderHTTP_GetLogs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("count") != "10" {
			t.Errorf("Expected count=10, got %s", r.URL.Query().Get("count"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool               `json:"success"`
			Data    types.LogsResponse `json:"data"`
		}{
			Success: true,
			Data: types.LogsResponse{
				Logs: []types.LogEntryResponse{
					{Timestamp: time.Now(), Level: "info", Message: "test log"},
				},
			},
		})
	}))
	defer server.Close()

	reader := NewStateReaderHTTP(server.URL)
	logs := reader.GetLogs(10)

	if len(logs) != 1 {
		t.Fatalf("Expected 1 log, got %d", len(logs))
	}
	if logs[0].Message != "test log" {
		t.Errorf("Expected message 'test log', got '%s'", logs[0].Message)
	}
}

// TestStateReaderHTTP_Count tests counting forwards
func TestStateReaderHTTP_Count(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool                      `json:"success"`
			Data    types.ForwardListResponse `json:"data"`
		}{
			Success: true,
			Data: types.ForwardListResponse{
				Forwards: []types.ForwardResponse{
					{Key: "fwd1"},
					{Key: "fwd2"},
				},
			},
		})
	}))
	defer server.Close()

	reader := NewStateReaderHTTP(server.URL)
	count := reader.Count()

	if count != 2 {
		t.Errorf("Expected count 2, got %d", count)
	}
}

// TestMetricsProviderHTTP_GetAllSnapshots tests getting all metric snapshots
func TestMetricsProviderHTTP_GetAllSnapshots(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool                           `json:"success"`
			Data    []types.ServiceMetricsResponse `json:"data"`
		}{
			Success: true,
			Data: []types.ServiceMetricsResponse{
				{
					Key:           "svc1.default.ctx1",
					ServiceName:   "svc1",
					TotalBytesIn:  1000,
					TotalBytesOut: 2000,
				},
			},
		})
	}))
	defer server.Close()

	provider := NewMetricsProviderHTTP(server.URL)
	snapshots := provider.GetAllSnapshots()

	if len(snapshots) != 1 {
		t.Fatalf("Expected 1 snapshot, got %d", len(snapshots))
	}
	if snapshots[0].TotalBytesIn != 1000 {
		t.Errorf("Expected TotalBytesIn 1000, got %d", snapshots[0].TotalBytesIn)
	}
}

// TestMetricsProviderHTTP_GetTotals tests getting total metrics
func TestMetricsProviderHTTP_GetTotals(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool                         `json:"success"`
			Data    types.MetricsSummaryResponse `json:"data"`
		}{
			Success: true,
			Data: types.MetricsSummaryResponse{
				TotalBytesIn:  5000,
				TotalBytesOut: 10000,
				TotalRateIn:   100.5,
				TotalRateOut:  200.5,
			},
		})
	}))
	defer server.Close()

	provider := NewMetricsProviderHTTP(server.URL)
	bytesIn, bytesOut, rateIn, rateOut := provider.GetTotals()

	if bytesIn != 5000 {
		t.Errorf("Expected bytesIn 5000, got %d", bytesIn)
	}
	if bytesOut != 10000 {
		t.Errorf("Expected bytesOut 10000, got %d", bytesOut)
	}
	if rateIn != 100.5 {
		t.Errorf("Expected rateIn 100.5, got %f", rateIn)
	}
	if rateOut != 200.5 {
		t.Errorf("Expected rateOut 200.5, got %f", rateOut)
	}
}

// TestServiceControllerHTTP_Reconnect tests reconnecting a service
func TestServiceControllerHTTP_Reconnect(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST method, got %s", r.Method)
		}
		if r.URL.Path != "/v1/services/svc1/reconnect" {
			t.Errorf("Expected path /v1/services/svc1/reconnect, got %s", r.URL.Path)
		}
		called = true
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool `json:"success"`
		}{Success: true})
	}))
	defer server.Close()

	controller := NewServiceControllerHTTP(server.URL)
	err := controller.Reconnect("svc1")

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !called {
		t.Error("Expected reconnect endpoint to be called")
	}
}

// TestServiceControllerHTTP_ReconnectAll tests reconnecting all services
func TestServiceControllerHTTP_ReconnectAll(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool                    `json:"success"`
			Data    types.ReconnectResponse `json:"data"`
		}{
			Success: true,
			Data: types.ReconnectResponse{
				Triggered: 3,
			},
		})
	}))
	defer server.Close()

	controller := NewServiceControllerHTTP(server.URL)
	count := controller.ReconnectAll()

	if count != 3 {
		t.Errorf("Expected 3 triggered, got %d", count)
	}
}

// TestServiceControllerHTTP_Sync tests syncing a service
func TestServiceControllerHTTP_Sync(t *testing.T) {
	var receivedForce string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedForce = r.URL.Query().Get("force")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool `json:"success"`
		}{Success: true})
	}))
	defer server.Close()

	controller := NewServiceControllerHTTP(server.URL)

	// Test with force=false
	err := controller.Sync("svc1", false)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if receivedForce != "false" {
		t.Errorf("Expected force=false, got %s", receivedForce)
	}

	// Test with force=true
	err = controller.Sync("svc1", true)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if receivedForce != "true" {
		t.Errorf("Expected force=true, got %s", receivedForce)
	}
}

// TestDiagnosticsProviderHTTP_GetSummary tests getting diagnostic summary
func TestDiagnosticsProviderHTTP_GetSummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool                    `json:"success"`
			Data    types.DiagnosticSummary `json:"data"`
		}{
			Success: true,
			Data: types.DiagnosticSummary{
				Status:          "healthy",
				Recommendations: []string{"Test recommendation"},
			},
		})
	}))
	defer server.Close()

	provider := NewDiagnosticsProviderHTTP(server.URL)
	summary := provider.GetSummary()

	if summary.Status != "healthy" {
		t.Errorf("Expected health status 'healthy', got '%s'", summary.Status)
	}
	if len(summary.Recommendations) != 1 {
		t.Errorf("Expected 1 recommendation, got %d", len(summary.Recommendations))
	}
}

// TestDiagnosticsProviderHTTP_GetErrors tests getting errors
func TestDiagnosticsProviderHTTP_GetErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool                `json:"success"`
			Data    []types.ErrorDetail `json:"data"`
		}{
			Success: true,
			Data: []types.ErrorDetail{
				{
					ServiceKey: "svc1",
					Message:    "connection refused",
				},
			},
		})
	}))
	defer server.Close()

	provider := NewDiagnosticsProviderHTTP(server.URL)
	errors := provider.GetErrors(10)

	if len(errors) != 1 {
		t.Fatalf("Expected 1 error, got %d", len(errors))
	}
	if errors[0].Message != "connection refused" {
		t.Errorf("Expected message 'connection refused', got '%s'", errors[0].Message)
	}
}

// TestManagerInfoHTTP tests the manager info adapter
func TestManagerInfoHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool               `json:"success"`
			Data    types.InfoResponse `json:"data"`
		}{
			Success: true,
			Data: types.InfoResponse{
				Version:    "1.0.0",
				TUIEnabled: true,
				Namespaces: []string{"default", "kube-system"},
				Contexts:   []string{"minikube"},
				StartTime:  time.Now().Add(-5 * time.Minute),
			},
		})
	}))
	defer server.Close()

	manager := NewManagerInfoHTTP(server.URL)

	if manager.Version() != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", manager.Version())
	}
	if !manager.TUIEnabled() {
		t.Error("Expected TUIEnabled to be true")
	}
	if len(manager.Namespaces()) != 2 {
		t.Errorf("Expected 2 namespaces, got %d", len(manager.Namespaces()))
	}
	if len(manager.Contexts()) != 1 {
		t.Errorf("Expected 1 context, got %d", len(manager.Contexts()))
	}

	// Test uptime calculation
	uptime := manager.Uptime()
	if uptime < 0 {
		t.Error("Expected non-negative uptime")
	}

	// Test start time
	startTime := manager.StartTime()
	if startTime.IsZero() {
		t.Error("Expected non-zero start time")
	}
}

// TestStateReaderHTTP_GetFiltered_StatusParsing tests that status strings are parsed correctly
// by the GetFiltered method (indirectly tests parseStatus function)
func TestStateReaderHTTP_GetFiltered_StatusParsing(t *testing.T) {
	testCases := []struct {
		statusStr      string
		expectedStatus state.ForwardStatus
	}{
		{"active", state.StatusActive},
		{"connecting", state.StatusConnecting},
		{"error", state.StatusError},
		{"pending", state.StatusPending},
	}

	for _, tc := range testCases {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(struct {
				Success bool                      `json:"success"`
				Data    types.ForwardListResponse `json:"data"`
			}{
				Success: true,
				Data: types.ForwardListResponse{
					Forwards: []types.ForwardResponse{
						{Key: "fwd1", Status: tc.statusStr},
					},
				},
			})
		}))

		reader := NewStateReaderHTTP(server.URL)
		forwards := reader.GetFiltered()

		if len(forwards) != 1 {
			t.Fatalf("Expected 1 forward, got %d", len(forwards))
		}
		if forwards[0].Status != tc.expectedStatus {
			t.Errorf("For status %q: expected %v, got %v", tc.statusStr, tc.expectedStatus, forwards[0].Status)
		}
		server.Close()
	}
}

// TestHTTPClient_NetworkError tests handling of network errors
func TestHTTPClient_NetworkError(t *testing.T) {
	client := NewHTTPClient("http://localhost:99999") // Invalid port

	var result interface{}
	err := client.Get("/test", &result)
	if err == nil {
		t.Error("Expected network error, got nil")
	}
}

// TestStateReaderHTTP_ServiceCount tests counting services
func TestStateReaderHTTP_ServiceCount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool                      `json:"success"`
			Data    types.ServiceListResponse `json:"data"`
		}{
			Success: true,
			Data: types.ServiceListResponse{
				Services: []types.ServiceResponse{
					{Key: "svc1"},
					{Key: "svc2"},
					{Key: "svc3"},
				},
			},
		})
	}))
	defer server.Close()

	reader := NewStateReaderHTTP(server.URL)
	count := reader.ServiceCount()

	if count != 3 {
		t.Errorf("Expected count 3, got %d", count)
	}
}

// TestMetricsProviderHTTP_ServiceCount tests counting services via metrics
func TestMetricsProviderHTTP_ServiceCount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool                         `json:"success"`
			Data    types.MetricsSummaryResponse `json:"data"`
		}{
			Success: true,
			Data: types.MetricsSummaryResponse{
				TotalServices: 2,
			},
		})
	}))
	defer server.Close()

	provider := NewMetricsProviderHTTP(server.URL)
	count := provider.ServiceCount()

	if count != 2 {
		t.Errorf("Expected count 2, got %d", count)
	}
}

// TestMetricsProviderHTTP_PortForwardCount tests counting port forwards
func TestMetricsProviderHTTP_PortForwardCount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool                         `json:"success"`
			Data    types.MetricsSummaryResponse `json:"data"`
		}{
			Success: true,
			Data: types.MetricsSummaryResponse{
				TotalForwards: 3,
			},
		})
	}))
	defer server.Close()

	provider := NewMetricsProviderHTTP(server.URL)
	count := provider.PortForwardCount()

	if count != 3 {
		t.Errorf("Expected count 3, got %d", count)
	}
}
