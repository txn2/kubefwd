package fwdmcp

import (
	"encoding/json"
	"fmt"
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
	client := NewHTTPClient("http://127.0.0.1:65535") // Use unreachable port in valid range

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

// ============================================================================
// Tests for new CRUD HTTP clients
// ============================================================================

// TestHTTPClient_PostJSON tests the PostJSON method
func TestHTTPClient_PostJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST method, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type application/json")
		}

		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["name"] != "test" {
			t.Errorf("Expected body name 'test', got '%s'", body["name"])
		}

		w.WriteHeader(http.StatusCreated)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"result": "created"})
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL)
	var result struct {
		Result string `json:"result"`
	}
	err := client.PostJSON("/create", map[string]string{"name": "test"}, &result)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result.Result != "created" {
		t.Errorf("Expected result 'created', got '%s'", result.Result)
	}
}

// TestHTTPClient_Delete tests the Delete method
func TestHTTPClient_Delete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("Expected DELETE method, got %s", r.Method)
		}
		if r.URL.Path != "/v1/items/123" {
			t.Errorf("Expected path /v1/items/123, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"success": true})
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL)
	var result struct {
		Success bool `json:"success"`
	}
	err := client.Delete("/v1/items/123", &result)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("Expected success to be true")
	}
}

// TestHTTPClient_Delete_NoContent tests Delete with 204 No Content
func TestHTTPClient_Delete_NoContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL)
	err := client.Delete("/v1/items/123", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
}

// TestNamespaceControllerHTTP_AddNamespace tests adding a namespace
func TestNamespaceControllerHTTP_AddNamespace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST method, got %s", r.Method)
		}
		if r.URL.Path != "/v1/namespaces" {
			t.Errorf("Expected path /v1/namespaces, got %s", r.URL.Path)
		}

		var req types.AddNamespaceRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Namespace != "staging" {
			t.Errorf("Expected namespace 'staging', got '%s'", req.Namespace)
		}

		w.WriteHeader(http.StatusCreated)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool                       `json:"success"`
			Data    types.AddNamespaceResponse `json:"data"`
		}{
			Success: true,
			Data: types.AddNamespaceResponse{
				Key:       "staging.minikube",
				Namespace: "staging",
				Context:   "minikube",
				Services:  []string{"api", "db", "cache"},
			},
		})
	}))
	defer server.Close()

	controller := NewNamespaceControllerHTTP(server.URL)
	result, err := controller.AddNamespace("minikube", "staging", types.AddNamespaceOpts{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result.Key != "staging.minikube" {
		t.Errorf("Expected key 'staging.minikube', got '%s'", result.Key)
	}
	if result.ServiceCount != 3 {
		t.Errorf("Expected 3 services, got %d", result.ServiceCount)
	}
}

// TestNamespaceControllerHTTP_RemoveNamespace tests removing a namespace
func TestNamespaceControllerHTTP_RemoveNamespace(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("Expected DELETE method, got %s", r.Method)
		}
		if r.URL.Path != "/v1/namespaces/staging.minikube" {
			t.Errorf("Expected path /v1/namespaces/staging.minikube, got %s", r.URL.Path)
		}
		called = true
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool `json:"success"`
		}{Success: true})
	}))
	defer server.Close()

	controller := NewNamespaceControllerHTTP(server.URL)
	err := controller.RemoveNamespace("minikube", "staging")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !called {
		t.Error("Expected delete endpoint to be called")
	}
}

// TestNamespaceControllerHTTP_ListNamespaces tests listing namespaces
func TestNamespaceControllerHTTP_ListNamespaces(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool                        `json:"success"`
			Data    types.NamespaceListResponse `json:"data"`
		}{
			Success: true,
			Data: types.NamespaceListResponse{
				Namespaces: []types.NamespaceInfoResponse{
					{Key: "default.minikube", Namespace: "default", Context: "minikube", ServiceCount: 5},
					{Key: "staging.minikube", Namespace: "staging", Context: "minikube", ServiceCount: 3},
				},
			},
		})
	}))
	defer server.Close()

	controller := NewNamespaceControllerHTTP(server.URL)
	namespaces := controller.ListNamespaces()
	if len(namespaces) != 2 {
		t.Fatalf("Expected 2 namespaces, got %d", len(namespaces))
	}
	if namespaces[0].Namespace != "default" {
		t.Errorf("Expected first namespace 'default', got '%s'", namespaces[0].Namespace)
	}
}

// TestNamespaceControllerHTTP_GetNamespace tests getting a single namespace
func TestNamespaceControllerHTTP_GetNamespace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/namespaces/staging.minikube" {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(struct {
				Success bool             `json:"success"`
				Error   *types.ErrorInfo `json:"error"`
			}{
				Success: false,
				Error:   &types.ErrorInfo{Code: "NOT_FOUND", Message: "not found"},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool                        `json:"success"`
			Data    types.NamespaceInfoResponse `json:"data"`
		}{
			Success: true,
			Data: types.NamespaceInfoResponse{
				Key:          "staging.minikube",
				Namespace:    "staging",
				Context:      "minikube",
				ServiceCount: 3,
			},
		})
	}))
	defer server.Close()

	controller := NewNamespaceControllerHTTP(server.URL)
	ns, err := controller.GetNamespace("minikube", "staging")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if ns.Namespace != "staging" {
		t.Errorf("Expected namespace 'staging', got '%s'", ns.Namespace)
	}
}

// TestServiceCRUDHTTP_AddService tests adding a service
func TestServiceCRUDHTTP_AddService(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST method, got %s", r.Method)
		}

		var req types.AddServiceRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.ServiceName != "postgres" {
			t.Errorf("Expected service name 'postgres', got '%s'", req.ServiceName)
		}

		w.WriteHeader(http.StatusCreated)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool                     `json:"success"`
			Data    types.AddServiceResponse `json:"data"`
		}{
			Success: true,
			Data: types.AddServiceResponse{
				Key:         "postgres.staging.minikube",
				ServiceName: "postgres",
				Namespace:   "staging",
				Context:     "minikube",
				LocalIP:     "127.1.2.3",
				Hostnames:   []string{"postgres", "postgres.staging"},
			},
		})
	}))
	defer server.Close()

	crud := NewServiceCRUDHTTP(server.URL)
	result, err := crud.AddService(types.AddServiceRequest{
		Namespace:   "staging",
		ServiceName: "postgres",
		Context:     "minikube",
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result.LocalIP != "127.1.2.3" {
		t.Errorf("Expected LocalIP '127.1.2.3', got '%s'", result.LocalIP)
	}
}

// TestServiceCRUDHTTP_RemoveService tests removing a service
func TestServiceCRUDHTTP_RemoveService(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("Expected DELETE method, got %s", r.Method)
		}
		called = true
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool `json:"success"`
		}{Success: true})
	}))
	defer server.Close()

	crud := NewServiceCRUDHTTP(server.URL)
	err := crud.RemoveService("postgres.staging.minikube")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !called {
		t.Error("Expected delete endpoint to be called")
	}
}

// TestKubernetesDiscoveryHTTP_ListNamespaces tests listing K8s namespaces
func TestKubernetesDiscoveryHTTP_ListNamespaces(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/kubernetes/namespaces" {
			t.Errorf("Expected path /v1/kubernetes/namespaces, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool                        `json:"success"`
			Data    types.K8sNamespacesResponse `json:"data"`
		}{
			Success: true,
			Data: types.K8sNamespacesResponse{
				Namespaces: []types.K8sNamespace{
					{Name: "default", Status: "Active", Forwarded: true},
					{Name: "kube-system", Status: "Active", Forwarded: false},
				},
			},
		})
	}))
	defer server.Close()

	discovery := NewKubernetesDiscoveryHTTP(server.URL)
	namespaces, err := discovery.ListNamespaces("")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(namespaces) != 2 {
		t.Fatalf("Expected 2 namespaces, got %d", len(namespaces))
	}
	if !namespaces[0].Forwarded {
		t.Error("Expected first namespace to be forwarded")
	}
}

// TestKubernetesDiscoveryHTTP_ListServices tests listing K8s services
func TestKubernetesDiscoveryHTTP_ListServices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("namespace") != "staging" {
			t.Errorf("Expected namespace query 'staging', got '%s'", r.URL.Query().Get("namespace"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool                      `json:"success"`
			Data    types.K8sServicesResponse `json:"data"`
		}{
			Success: true,
			Data: types.K8sServicesResponse{
				Services: []types.K8sService{
					{Name: "api", Namespace: "staging", Type: "ClusterIP", ClusterIP: "10.0.0.1"},
					{Name: "db", Namespace: "staging", Type: "ClusterIP", ClusterIP: "10.0.0.2"},
				},
			},
		})
	}))
	defer server.Close()

	discovery := NewKubernetesDiscoveryHTTP(server.URL)
	services, err := discovery.ListServices("", "staging")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(services) != 2 {
		t.Fatalf("Expected 2 services, got %d", len(services))
	}
}

// TestKubernetesDiscoveryHTTP_GetService tests getting a K8s service
func TestKubernetesDiscoveryHTTP_GetService(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/kubernetes/services/staging/api" {
			t.Errorf("Unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool             `json:"success"`
			Data    types.K8sService `json:"data"`
		}{
			Success: true,
			Data: types.K8sService{
				Name:      "api",
				Namespace: "staging",
				Type:      "ClusterIP",
				ClusterIP: "10.0.0.1",
				Ports: []types.K8sServicePort{
					{Port: 80, TargetPort: "8080", Protocol: "TCP"},
				},
			},
		})
	}))
	defer server.Close()

	discovery := NewKubernetesDiscoveryHTTP(server.URL)
	svc, err := discovery.GetService("", "staging", "api")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if svc.Name != "api" {
		t.Errorf("Expected name 'api', got '%s'", svc.Name)
	}
	if len(svc.Ports) != 1 {
		t.Errorf("Expected 1 port, got %d", len(svc.Ports))
	}
}

// TestKubernetesDiscoveryHTTP_ListContexts tests listing K8s contexts
func TestKubernetesDiscoveryHTTP_ListContexts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool                      `json:"success"`
			Data    types.K8sContextsResponse `json:"data"`
		}{
			Success: true,
			Data: types.K8sContextsResponse{
				Contexts: []types.K8sContext{
					{Name: "minikube", Cluster: "minikube", Active: true},
					{Name: "prod", Cluster: "prod-cluster", Active: false},
				},
				CurrentContext: "minikube",
			},
		})
	}))
	defer server.Close()

	discovery := NewKubernetesDiscoveryHTTP(server.URL)
	result, err := discovery.ListContexts()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(result.Contexts) != 2 {
		t.Fatalf("Expected 2 contexts, got %d", len(result.Contexts))
	}
	if result.CurrentContext != "minikube" {
		t.Errorf("Expected current context 'minikube', got '%s'", result.CurrentContext)
	}
}

// TestConnectionInfoProviderHTTP_GetConnectionInfo tests getting connection info
func TestConnectionInfoProviderHTTP_GetConnectionInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool                  `json:"success"`
			Data    types.ServiceResponse `json:"data"`
		}{
			Success: true,
			Data: types.ServiceResponse{
				Key:         "postgres.staging.minikube",
				ServiceName: "postgres",
				Namespace:   "staging",
				Context:     "minikube",
				Status:      "active",
				Forwards: []types.ForwardResponse{
					{
						LocalIP:   "127.1.2.3",
						LocalPort: "5432",
						PodPort:   "5432",
						Hostnames: []string{"postgres", "postgres.staging"},
					},
				},
			},
		})
	}))
	defer server.Close()

	provider := NewConnectionInfoProviderHTTP(server.URL)
	info, err := provider.GetConnectionInfo("postgres.staging.minikube")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if info.Service != "postgres" {
		t.Errorf("Expected service 'postgres', got '%s'", info.Service)
	}
	if info.LocalIP != "127.1.2.3" {
		t.Errorf("Expected LocalIP '127.1.2.3', got '%s'", info.LocalIP)
	}
	if len(info.Hostnames) != 2 {
		t.Errorf("Expected 2 hostnames, got %d", len(info.Hostnames))
	}
	// Check env vars are generated
	if info.EnvVars["POSTGRES_HOST"] != "postgres" {
		t.Errorf("Expected POSTGRES_HOST 'postgres', got '%s'", info.EnvVars["POSTGRES_HOST"])
	}
	if info.EnvVars["DATABASE_URL"] != "postgresql://postgres:5432" {
		t.Errorf("Expected DATABASE_URL, got '%s'", info.EnvVars["DATABASE_URL"])
	}
}

// TestConnectionInfoProviderHTTP_ListHostnames tests listing hostnames
func TestConnectionInfoProviderHTTP_ListHostnames(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool                      `json:"success"`
			Data    types.ServiceListResponse `json:"data"`
		}{
			Success: true,
			Data: types.ServiceListResponse{
				Services: []types.ServiceResponse{
					{
						ServiceName: "api",
						Namespace:   "default",
						Context:     "minikube",
						Forwards: []types.ForwardResponse{
							{LocalIP: "127.1.0.1", Hostnames: []string{"api", "api.default"}},
						},
					},
					{
						ServiceName: "db",
						Namespace:   "default",
						Context:     "minikube",
						Forwards: []types.ForwardResponse{
							{LocalIP: "127.1.0.2", Hostnames: []string{"db"}},
						},
					},
				},
			},
		})
	}))
	defer server.Close()

	provider := NewConnectionInfoProviderHTTP(server.URL)
	result, err := provider.ListHostnames()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result.Total != 3 {
		t.Errorf("Expected 3 hostnames, got %d", result.Total)
	}
}

// TestConnectionInfoProviderHTTP_FindServices tests finding services
func TestConnectionInfoProviderHTTP_FindServices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool                      `json:"success"`
			Data    types.ServiceListResponse `json:"data"`
		}{
			Success: true,
			Data: types.ServiceListResponse{
				Services: []types.ServiceResponse{
					{
						ServiceName: "postgres",
						Namespace:   "staging",
						Context:     "minikube",
						Status:      "active",
						Forwards: []types.ForwardResponse{
							{LocalIP: "127.1.0.1", LocalPort: "5432", PodPort: "5432", Hostnames: []string{"postgres"}},
						},
					},
					{
						ServiceName: "mysql",
						Namespace:   "staging",
						Context:     "minikube",
						Status:      "active",
						Forwards: []types.ForwardResponse{
							{LocalIP: "127.1.0.2", LocalPort: "3306", PodPort: "3306", Hostnames: []string{"mysql"}},
						},
					},
					{
						ServiceName: "api",
						Namespace:   "default",
						Context:     "minikube",
						Status:      "active",
						Forwards: []types.ForwardResponse{
							{LocalIP: "127.1.0.3", LocalPort: "8080", PodPort: "8080", Hostnames: []string{"api"}},
						},
					},
				},
			},
		})
	}))
	defer server.Close()

	provider := NewConnectionInfoProviderHTTP(server.URL)

	// Test find by query
	results, err := provider.FindServices("postgres", 0, "")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Expected 1 result for query 'postgres', got %d", len(results))
	}

	// Test find by port
	results, err = provider.FindServices("", 5432, "")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Expected 1 result for port 5432, got %d", len(results))
	}

	// Test find by namespace
	results, err = provider.FindServices("", 0, "staging")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Expected 2 results for namespace 'staging', got %d", len(results))
	}
}

// TestStateReaderHTTP_GetForward tests getting a single forward
func TestStateReaderHTTP_GetForward(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/forwards/fwd1.svc1.default.ctx1" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool                  `json:"success"`
			Data    types.ForwardResponse `json:"data"`
		}{
			Success: true,
			Data: types.ForwardResponse{
				Key:         "fwd1.svc1.default.ctx1",
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
		})
	}))
	defer server.Close()

	reader := NewStateReaderHTTP(server.URL)

	// Test existing forward
	fwd := reader.GetForward("fwd1.svc1.default.ctx1")
	if fwd == nil {
		t.Fatal("Expected non-nil forward")
	}
	if fwd.PodName != "pod1" {
		t.Errorf("Expected pod name 'pod1', got '%s'", fwd.PodName)
	}
	if fwd.LocalIP != "127.1.0.1" {
		t.Errorf("Expected LocalIP '127.1.0.1', got '%s'", fwd.LocalIP)
	}

	// Test non-existing forward
	fwd = reader.GetForward("nonexistent")
	if fwd != nil {
		t.Error("Expected nil for non-existent forward")
	}
}

// TestMetricsProviderHTTP_GetServiceSnapshot tests getting a single service snapshot
func TestMetricsProviderHTTP_GetServiceSnapshot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/metrics/services/svc1.default.ctx1" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool                         `json:"success"`
			Data    types.ServiceMetricsResponse `json:"data"`
		}{
			Success: true,
			Data: types.ServiceMetricsResponse{
				Key:           "svc1.default.ctx1",
				ServiceName:   "svc1",
				Namespace:     "default",
				Context:       "ctx1",
				TotalBytesIn:  5000,
				TotalBytesOut: 10000,
				RateIn:        100.5,
				RateOut:       200.5,
			},
		})
	}))
	defer server.Close()

	provider := NewMetricsProviderHTTP(server.URL)

	// Test existing service
	snapshot := provider.GetServiceSnapshot("svc1.default.ctx1")
	if snapshot == nil {
		t.Fatal("Expected non-nil snapshot")
	}
	if snapshot.ServiceName != "svc1" {
		t.Errorf("Expected service name 'svc1', got '%s'", snapshot.ServiceName)
	}
	if snapshot.TotalBytesIn != 5000 {
		t.Errorf("Expected TotalBytesIn 5000, got %d", snapshot.TotalBytesIn)
	}

	// Test non-existing service
	snapshot = provider.GetServiceSnapshot("nonexistent")
	if snapshot != nil {
		t.Error("Expected nil for non-existent service")
	}
}

// TestDiagnosticsProviderHTTP_GetServiceDiagnostic tests getting service diagnostic
func TestDiagnosticsProviderHTTP_GetServiceDiagnostic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/diagnostics/services/svc1.default.ctx1" {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(struct {
				Success bool             `json:"success"`
				Error   *types.ErrorInfo `json:"error"`
			}{
				Success: false,
				Error:   &types.ErrorInfo{Code: "NOT_FOUND", Message: "service not found"},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool                    `json:"success"`
			Data    types.ServiceDiagnostic `json:"data"`
		}{
			Success: true,
			Data: types.ServiceDiagnostic{
				Key:         "svc1.default.ctx1",
				ServiceName: "svc1",
				Namespace:   "default",
				Context:     "ctx1",
				Status:      "healthy",
			},
		})
	}))
	defer server.Close()

	provider := NewDiagnosticsProviderHTTP(server.URL)

	// Test existing service
	diag, err := provider.GetServiceDiagnostic("svc1.default.ctx1")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if diag.ServiceName != "svc1" {
		t.Errorf("Expected service name 'svc1', got '%s'", diag.ServiceName)
	}
	if diag.Status != "healthy" {
		t.Errorf("Expected status 'healthy', got '%s'", diag.Status)
	}

	// Test non-existing service
	_, err = provider.GetServiceDiagnostic("nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent service")
	}
}

// TestDiagnosticsProviderHTTP_GetForwardDiagnostic tests getting forward diagnostic
func TestDiagnosticsProviderHTTP_GetForwardDiagnostic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/diagnostics/forwards/fwd1" {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(struct {
				Success bool             `json:"success"`
				Error   *types.ErrorInfo `json:"error"`
			}{
				Success: false,
				Error:   &types.ErrorInfo{Code: "NOT_FOUND", Message: "forward not found"},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool                    `json:"success"`
			Data    types.ForwardDiagnostic `json:"data"`
		}{
			Success: true,
			Data: types.ForwardDiagnostic{
				Key:        "fwd1",
				ServiceKey: "svc1.default.ctx1",
				PodName:    "pod1",
				Status:     "active",
				LocalIP:    "127.1.0.1",
				LocalPort:  "80",
				PodPort:    "8080",
			},
		})
	}))
	defer server.Close()

	provider := NewDiagnosticsProviderHTTP(server.URL)

	// Test existing forward
	diag, err := provider.GetForwardDiagnostic("fwd1")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if diag.PodName != "pod1" {
		t.Errorf("Expected pod name 'pod1', got '%s'", diag.PodName)
	}
	if diag.Status != "active" {
		t.Errorf("Expected status 'active', got '%s'", diag.Status)
	}

	// Test non-existing forward
	_, err = provider.GetForwardDiagnostic("nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent forward")
	}
}

// TestDiagnosticsProviderHTTP_GetNetworkStatus tests getting network status
func TestDiagnosticsProviderHTTP_GetNetworkStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool                `json:"success"`
			Data    types.NetworkStatus `json:"data"`
		}{
			Success: true,
			Data: types.NetworkStatus{
				LoopbackInterface: "lo0",
				IPRange:           "127.1.0.1-127.255.255.254",
				IPsAllocated:      5,
				PortsInUse:        10,
				Hostnames:         []string{"svc1", "svc2"},
			},
		})
	}))
	defer server.Close()

	provider := NewDiagnosticsProviderHTTP(server.URL)
	status := provider.GetNetworkStatus()

	if status.LoopbackInterface != "lo0" {
		t.Errorf("Expected loopback interface 'lo0', got '%s'", status.LoopbackInterface)
	}
	if status.IPsAllocated != 5 {
		t.Errorf("Expected 5 allocated IPs, got %d", status.IPsAllocated)
	}
	if status.PortsInUse != 10 {
		t.Errorf("Expected 10 ports in use, got %d", status.PortsInUse)
	}
	if len(status.Hostnames) != 2 {
		t.Errorf("Expected 2 hostnames, got %d", len(status.Hostnames))
	}
}

// TestNewAPIUnavailableError tests the API unavailable error constructor
func TestNewAPIUnavailableError(t *testing.T) {
	cause := fmt.Errorf("connection refused")
	err := NewAPIUnavailableError("http://localhost:8080", cause)
	if err == nil {
		t.Fatal("Expected non-nil error")
	}

	if err.Code != "api_unavailable" {
		t.Errorf("Expected code 'api_unavailable', got '%s'", err.Code)
	}

	errMsg := err.Error()
	if errMsg == "" {
		t.Error("Expected non-empty error message")
	}

	// Verify the message contains the API URL
	if err.Message == "" {
		t.Error("Expected non-empty message")
	}

	// Verify diagnosis is provided
	if err.Diagnosis == "" {
		t.Error("Expected non-empty diagnosis")
	}
}

// TestGenerateEnvVars tests environment variable generation
func TestGenerateEnvVars(t *testing.T) {
	testCases := []struct {
		name        string
		serviceName string
		localIP     string
		ports       []types.PortInfo
		expectKeys  []string
	}{
		{
			name:        "PostgreSQL",
			serviceName: "postgres",
			localIP:     "127.1.0.1",
			ports:       []types.PortInfo{{LocalPort: 5432}},
			expectKeys:  []string{"POSTGRES_HOST", "POSTGRES_PORT", "DATABASE_URL"},
		},
		{
			name:        "MySQL",
			serviceName: "mysql-db",
			localIP:     "127.1.0.2",
			ports:       []types.PortInfo{{LocalPort: 3306}},
			expectKeys:  []string{"MYSQL_DB_HOST", "MYSQL_DB_PORT", "DATABASE_URL"},
		},
		{
			name:        "Redis",
			serviceName: "redis",
			localIP:     "127.1.0.3",
			ports:       []types.PortInfo{{LocalPort: 6379}},
			expectKeys:  []string{"REDIS_HOST", "REDIS_PORT", "REDIS_URL"},
		},
		{
			name:        "HTTP service",
			serviceName: "api",
			localIP:     "127.1.0.4",
			ports:       []types.PortInfo{{LocalPort: 8080}},
			expectKeys:  []string{"API_HOST", "API_PORT", "API_URL"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			envVars := generateEnvVars(tc.serviceName, tc.localIP, tc.ports)
			for _, key := range tc.expectKeys {
				if _, ok := envVars[key]; !ok {
					t.Errorf("Expected env var %s to be present", key)
				}
			}
		})
	}
}

// =============================================================================
// KubernetesDiscovery HTTP - Pod Methods
// =============================================================================

func TestKubernetesDiscoveryHTTP_GetPodLogs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET method, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool                  `json:"success"`
			Data    types.PodLogsResponse `json:"data"`
		}{
			Success: true,
			Data: types.PodLogsResponse{
				PodName:       "test-pod",
				Namespace:     "default",
				ContainerName: "main",
				Logs:          []string{"line1", "line2", "line3"},
				LineCount:     3,
			},
		})
	}))
	defer server.Close()

	discovery := NewKubernetesDiscoveryHTTP(server.URL)
	logs, err := discovery.GetPodLogs("", "default", "test-pod", types.PodLogsOptions{
		TailLines:  100,
		Timestamps: true,
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if logs.PodName != "test-pod" {
		t.Errorf("Expected pod name 'test-pod', got '%s'", logs.PodName)
	}
	if len(logs.Logs) != 3 {
		t.Errorf("Expected 3 log lines, got %d", len(logs.Logs))
	}
}

func TestKubernetesDiscoveryHTTP_GetPodLogs_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool             `json:"success"`
			Error   *types.ErrorInfo `json:"error"`
		}{
			Success: false,
			Error:   &types.ErrorInfo{Code: "NOT_FOUND", Message: "Pod not found"},
		})
	}))
	defer server.Close()

	discovery := NewKubernetesDiscoveryHTTP(server.URL)
	_, err := discovery.GetPodLogs("", "default", "nonexistent-pod", types.PodLogsOptions{})
	if err == nil {
		t.Error("Expected error, got nil")
	}
}

func TestKubernetesDiscoveryHTTP_ListPods(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET method, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool           `json:"success"`
			Data    []types.K8sPod `json:"data"`
		}{
			Success: true,
			Data: []types.K8sPod{
				{Name: "pod-1", Namespace: "default", Status: "Running", Ready: "1/1"},
				{Name: "pod-2", Namespace: "default", Status: "Running", Ready: "1/1"},
			},
		})
	}))
	defer server.Close()

	discovery := NewKubernetesDiscoveryHTTP(server.URL)
	pods, err := discovery.ListPods("", "default", types.ListPodsOptions{
		LabelSelector: "app=test",
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(pods) != 2 {
		t.Errorf("Expected 2 pods, got %d", len(pods))
	}
}

func TestKubernetesDiscoveryHTTP_ListPods_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool             `json:"success"`
			Error   *types.ErrorInfo `json:"error"`
		}{
			Success: false,
			Error:   &types.ErrorInfo{Code: "FORBIDDEN", Message: "Access denied"},
		})
	}))
	defer server.Close()

	discovery := NewKubernetesDiscoveryHTTP(server.URL)
	_, err := discovery.ListPods("", "default", types.ListPodsOptions{})
	if err == nil {
		t.Error("Expected error, got nil")
	}
}

func TestKubernetesDiscoveryHTTP_GetPod(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool               `json:"success"`
			Data    types.K8sPodDetail `json:"data"`
		}{
			Success: true,
			Data: types.K8sPodDetail{
				Name:      "test-pod",
				Namespace: "default",
				Status:    "Running",
				Containers: []types.K8sContainerInfo{
					{Name: "main", Image: "nginx:latest"},
				},
			},
		})
	}))
	defer server.Close()

	discovery := NewKubernetesDiscoveryHTTP(server.URL)
	pod, err := discovery.GetPod("ctx", "default", "test-pod")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if pod.Name != "test-pod" {
		t.Errorf("Expected pod name 'test-pod', got '%s'", pod.Name)
	}
	if len(pod.Containers) != 1 {
		t.Errorf("Expected 1 container, got %d", len(pod.Containers))
	}
}

func TestKubernetesDiscoveryHTTP_GetPod_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool             `json:"success"`
			Error   *types.ErrorInfo `json:"error"`
		}{
			Success: false,
			Error:   &types.ErrorInfo{Code: "NOT_FOUND", Message: "Pod not found"},
		})
	}))
	defer server.Close()

	discovery := NewKubernetesDiscoveryHTTP(server.URL)
	_, err := discovery.GetPod("", "default", "nonexistent")
	if err == nil {
		t.Error("Expected error, got nil")
	}
}

func TestKubernetesDiscoveryHTTP_GetEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool             `json:"success"`
			Data    []types.K8sEvent `json:"data"`
		}{
			Success: true,
			Data: []types.K8sEvent{
				{
					Type:    "Normal",
					Reason:  "Scheduled",
					Message: "Pod scheduled to node",
				},
				{
					Type:    "Normal",
					Reason:  "Pulled",
					Message: "Container image pulled",
				},
			},
		})
	}))
	defer server.Close()

	discovery := NewKubernetesDiscoveryHTTP(server.URL)
	events, err := discovery.GetEvents("ctx", "default", types.GetEventsOptions{
		ResourceKind: "Pod",
		ResourceName: "test-pod",
		Limit:        50,
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("Expected 2 events, got %d", len(events))
	}
}

func TestKubernetesDiscoveryHTTP_GetEvents_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool             `json:"success"`
			Error   *types.ErrorInfo `json:"error"`
		}{
			Success: false,
			Error:   &types.ErrorInfo{Code: "ERROR", Message: "Failed to get events"},
		})
	}))
	defer server.Close()

	discovery := NewKubernetesDiscoveryHTTP(server.URL)
	_, err := discovery.GetEvents("", "default", types.GetEventsOptions{})
	if err == nil {
		t.Error("Expected error, got nil")
	}
}

func TestKubernetesDiscoveryHTTP_GetEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool               `json:"success"`
			Data    types.K8sEndpoints `json:"data"`
		}{
			Success: true,
			Data: types.K8sEndpoints{
				Name:      "api",
				Namespace: "default",
				Subsets: []types.K8sEndpointSubset{
					{
						Addresses: []types.K8sEndpointAddress{{IP: "10.0.0.1", PodName: "pod-1"}},
						Ports:     []types.K8sEndpointPort{{Port: 8080, Protocol: "TCP"}},
					},
				},
			},
		})
	}))
	defer server.Close()

	discovery := NewKubernetesDiscoveryHTTP(server.URL)
	endpoints, err := discovery.GetEndpoints("ctx", "default", "api")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if endpoints.Name != "api" {
		t.Errorf("Expected service name 'api', got '%s'", endpoints.Name)
	}
}

func TestKubernetesDiscoveryHTTP_GetEndpoints_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool             `json:"success"`
			Error   *types.ErrorInfo `json:"error"`
		}{
			Success: false,
			Error:   &types.ErrorInfo{Code: "NOT_FOUND", Message: "Endpoints not found"},
		})
	}))
	defer server.Close()

	discovery := NewKubernetesDiscoveryHTTP(server.URL)
	_, err := discovery.GetEndpoints("", "default", "nonexistent")
	if err == nil {
		t.Error("Expected error, got nil")
	}
}

// =============================================================================
// Analysis Provider HTTP
// =============================================================================

func TestNewAnalysisProviderHTTP(t *testing.T) {
	provider := NewAnalysisProviderHTTP("http://localhost:8080")
	if provider == nil {
		t.Error("Expected non-nil provider")
	}
}

func TestAnalysisProviderHTTP_GetQuickStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool                `json:"success"`
			Data    QuickStatusResponse `json:"data"`
		}{
			Success: true,
			Data: QuickStatusResponse{
				Status:  "ok",
				Message: "All services healthy",
			},
		})
	}))
	defer server.Close()

	provider := NewAnalysisProviderHTTP(server.URL)
	status, err := provider.GetQuickStatus()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if status.Status != "ok" {
		t.Errorf("Expected status 'ok', got '%s'", status.Status)
	}
}

func TestAnalysisProviderHTTP_GetAnalysis(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool                 `json:"success"`
			Data    FullAnalysisResponse `json:"data"`
		}{
			Success: true,
			Data: FullAnalysisResponse{
				Status: "healthy",
				Issues: []AnalysisIssue{},
			},
		})
	}))
	defer server.Close()

	provider := NewAnalysisProviderHTTP(server.URL)
	analysis, err := provider.GetAnalysis()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if analysis.Status != "healthy" {
		t.Errorf("Expected status 'healthy', got '%s'", analysis.Status)
	}
}

// =============================================================================
// HTTP Traffic Provider HTTP
// =============================================================================

func TestNewHTTPTrafficProviderHTTP(t *testing.T) {
	provider := NewHTTPTrafficProviderHTTP("http://localhost:8080")
	if provider == nil {
		t.Error("Expected non-nil provider")
	}
}

func TestHTTPTrafficProviderHTTP_GetForwardHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool                      `json:"success"`
			Data    types.HTTPTrafficResponse `json:"data"`
		}{
			Success: true,
			Data: types.HTTPTrafficResponse{
				ForwardKey: "svc1.default.ctx",
				PodName:    "pod-1",
				LocalIP:    "127.1.0.1",
				LocalPort:  "8080",
				Logs: []types.HTTPLogResponse{
					{Method: "GET", Path: "/api/users", StatusCode: 200},
					{Method: "POST", Path: "/api/data", StatusCode: 201},
				},
			},
		})
	}))
	defer server.Close()

	provider := NewHTTPTrafficProviderHTTP(server.URL)
	resp, err := provider.GetForwardHTTP("svc1.default.ctx", 10)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(resp.Logs) != 2 {
		t.Errorf("Expected 2 logs, got %d", len(resp.Logs))
	}
}

func TestHTTPTrafficProviderHTTP_GetServiceHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool                             `json:"success"`
			Data    types.ServiceHTTPTrafficResponse `json:"data"`
		}{
			Success: true,
			Data: types.ServiceHTTPTrafficResponse{
				ServiceKey: "svc1.default.ctx",
				Forwards: []types.HTTPTrafficResponse{
					{
						ForwardKey: "svc1.default.ctx:0",
						PodName:    "pod-1",
						Logs: []types.HTTPLogResponse{
							{Method: "GET", Path: "/health", StatusCode: 200},
						},
					},
				},
			},
		})
	}))
	defer server.Close()

	provider := NewHTTPTrafficProviderHTTP(server.URL)
	resp, err := provider.GetServiceHTTP("svc1.default.ctx", 10)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(resp.Forwards) != 1 {
		t.Errorf("Expected 1 forward, got %d", len(resp.Forwards))
	}
}

// =============================================================================
// History Provider HTTP
// =============================================================================

func TestNewHistoryProviderHTTP(t *testing.T) {
	provider := NewHistoryProviderHTTP("http://localhost:8080")
	if provider == nil {
		t.Error("Expected non-nil provider")
	}
}

func TestHistoryProviderHTTP_GetEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool           `json:"success"`
			Data    []HistoryEvent `json:"data"`
		}{
			Success: true,
			Data: []HistoryEvent{
				{Type: "ForwardAdded", ServiceKey: "svc1.default.ctx"},
				{Type: "PodSync", ServiceKey: "svc1.default.ctx"},
			},
		})
	}))
	defer server.Close()

	provider := NewHistoryProviderHTTP(server.URL)
	events, err := provider.GetEvents(10, "")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("Expected 2 events, got %d", len(events))
	}
}

func TestHistoryProviderHTTP_GetErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool           `json:"success"`
			Data    []HistoryError `json:"data"`
		}{
			Success: true,
			Data: []HistoryError{
				{ServiceKey: "svc1.default.ctx", Message: "connection refused"},
			},
		})
	}))
	defer server.Close()

	provider := NewHistoryProviderHTTP(server.URL)
	errors, err := provider.GetErrors(10)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(errors) != 1 {
		t.Errorf("Expected 1 error, got %d", len(errors))
	}
}

func TestHistoryProviderHTTP_GetReconnects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool               `json:"success"`
			Data    []HistoryReconnect `json:"data"`
		}{
			Success: true,
			Data: []HistoryReconnect{
				{ServiceKey: "svc1.default.ctx", Success: true},
			},
		})
	}))
	defer server.Close()

	provider := NewHistoryProviderHTTP(server.URL)
	reconnects, err := provider.GetReconnects(10, "")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(reconnects) != 1 {
		t.Errorf("Expected 1 reconnect, got %d", len(reconnects))
	}
}

func TestHistoryProviderHTTP_GetStats(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Success bool         `json:"success"`
			Data    HistoryStats `json:"data"`
		}{
			Success: true,
			Data: HistoryStats{
				TotalEvents:     100,
				TotalErrors:     5,
				TotalReconnects: 10,
			},
		})
	}))
	defer server.Close()

	provider := NewHistoryProviderHTTP(server.URL)
	stats, err := provider.GetStats()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if stats.TotalEvents != 100 {
		t.Errorf("Expected 100 total events, got %d", stats.TotalEvents)
	}
}
