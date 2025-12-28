package fwdmcp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/txn2/kubefwd/pkg/fwdapi/types"
	"github.com/txn2/kubefwd/pkg/fwdmetrics"
	"github.com/txn2/kubefwd/pkg/fwdtui/state"
)

func TestHandleListServices(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Test nil state reader
	_, _, err := server.handleListServices(context.Background(), nil, ListServicesInput{})
	if err == nil {
		t.Error("Expected error when state reader is nil")
	}

	// Set up mock state reader with test data
	mock := &mockStateReader{
		services: []state.ServiceSnapshot{
			{Key: "svc1.default.ctx1", ServiceName: "svc1", Namespace: "default", Context: "ctx1", ActiveCount: 1, ErrorCount: 0},
			{Key: "svc2.kube-system.ctx1", ServiceName: "svc2", Namespace: "kube-system", Context: "ctx1", ActiveCount: 0, ErrorCount: 1},
			{Key: "svc3.default.ctx1", ServiceName: "svc3", Namespace: "default", Context: "ctx1", ActiveCount: 1, ErrorCount: 1},
		},
		summary: state.SummaryStats{TotalServices: 3, ActiveServices: 2, TotalForwards: 3, ActiveForwards: 2, ErrorCount: 2},
	}
	server.SetStateReader(mock)

	// Test basic list
	result, data, err := server.handleListServices(context.Background(), nil, ListServicesInput{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if data == nil {
		t.Fatal("Expected non-nil data")
	}

	// Test namespace filter
	_, data, err = server.handleListServices(context.Background(), nil, ListServicesInput{Namespace: "default"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	dataMap := data.(map[string]interface{})
	services := dataMap["services"].([]map[string]interface{})
	if len(services) != 2 {
		t.Errorf("Expected 2 services in default namespace, got %d", len(services))
	}

	// Test status filter - active
	_, data, err = server.handleListServices(context.Background(), nil, ListServicesInput{Status: "active"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	dataMap = data.(map[string]interface{})
	services = dataMap["services"].([]map[string]interface{})
	if len(services) != 1 {
		t.Errorf("Expected 1 active service, got %d", len(services))
	}

	// Test status filter - error
	_, data, err = server.handleListServices(context.Background(), nil, ListServicesInput{Status: "error"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	dataMap = data.(map[string]interface{})
	services = dataMap["services"].([]map[string]interface{})
	if len(services) != 1 {
		t.Errorf("Expected 1 error service, got %d", len(services))
	}

	// Test status filter - partial
	_, data, err = server.handleListServices(context.Background(), nil, ListServicesInput{Status: "partial"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	dataMap = data.(map[string]interface{})
	services = dataMap["services"].([]map[string]interface{})
	if len(services) != 1 {
		t.Errorf("Expected 1 partial service, got %d", len(services))
	}

	// Test include forwards
	mock.services[0].PortForwards = []state.ForwardSnapshot{
		{PodName: "pod1", LocalIP: "127.1.0.1", LocalPort: "80", PodPort: "8080", Status: state.StatusActive},
	}
	_, data, err = server.handleListServices(context.Background(), nil, ListServicesInput{IncludeForwards: true})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	dataMap = data.(map[string]interface{})
	services = dataMap["services"].([]map[string]interface{})
	if len(services) > 0 {
		if _, ok := services[0]["forwards"]; !ok {
			t.Error("Expected forwards field when IncludeForwards is true")
		}
	}
}

func TestHandleGetService(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Test nil state reader
	_, _, err := server.handleGetService(context.Background(), nil, GetServiceInput{Key: "test"})
	if err == nil {
		t.Error("Expected error when state reader is nil")
	}

	// Set up mock
	mock := &mockStateReader{
		services: []state.ServiceSnapshot{
			{
				Key: "svc1.default.ctx1", ServiceName: "svc1", Namespace: "default", Context: "ctx1",
				ActiveCount: 1, ErrorCount: 0,
				PortForwards: []state.ForwardSnapshot{
					{Key: "fwd1", PodName: "pod1", LocalIP: "127.1.0.1", LocalPort: "80", PodPort: "8080", Status: state.StatusActive},
				},
			},
		},
	}
	server.SetStateReader(mock)

	// Test service not found
	_, _, err = server.handleGetService(context.Background(), nil, GetServiceInput{Key: "nonexistent"})
	if err == nil {
		t.Error("Expected error for nonexistent service")
	}

	// Test valid service
	result, data, err := server.handleGetService(context.Background(), nil, GetServiceInput{Key: "svc1.default.ctx1"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Error("Expected non-nil result")
	}
	if data == nil {
		t.Error("Expected non-nil data")
	}
}

func TestHandleDiagnoseErrors(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Test nil state reader
	_, _, err := server.handleDiagnoseErrors(context.Background(), nil, struct{}{})
	if err == nil {
		t.Error("Expected error when state reader is nil")
	}

	// Set up mock with errors
	mock := &mockStateReader{
		services: []state.ServiceSnapshot{
			{
				Key: "svc1.default.ctx1", ServiceName: "svc1", Namespace: "default",
				ErrorCount: 1,
				PortForwards: []state.ForwardSnapshot{
					{PodName: "pod1", Error: "connection refused"},
				},
			},
			{
				Key: "svc2.default.ctx1", ServiceName: "svc2", Namespace: "default",
				ErrorCount: 1,
				PortForwards: []state.ForwardSnapshot{
					{PodName: "pod2", Error: "timeout error"},
				},
			},
			{
				Key: "svc3.default.ctx1", ServiceName: "svc3", Namespace: "default",
				ErrorCount: 1,
				PortForwards: []state.ForwardSnapshot{
					{PodName: "pod3", Error: "pod not found"},
				},
			},
		},
		logs: []state.LogEntry{
			{Timestamp: time.Now(), Level: "error", Message: "test error"},
		},
	}
	server.SetStateReader(mock)

	result, data, err := server.handleDiagnoseErrors(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Error("Expected non-nil result")
	}

	dataMap := data.(map[string]interface{})
	if dataMap["errorCount"].(int) != 3 {
		t.Errorf("Expected 3 errors, got %d", dataMap["errorCount"])
	}

	// Test healthy state (more than 5 errors = unhealthy)
	mock.services = append(mock.services,
		state.ServiceSnapshot{Key: "svc4", ErrorCount: 1, PortForwards: []state.ForwardSnapshot{{Error: "err1"}}},
		state.ServiceSnapshot{Key: "svc5", ErrorCount: 1, PortForwards: []state.ForwardSnapshot{{Error: "err2"}}},
		state.ServiceSnapshot{Key: "svc6", ErrorCount: 1, PortForwards: []state.ForwardSnapshot{{Error: "err3"}}},
	)
	_, data, _ = server.handleDiagnoseErrors(context.Background(), nil, struct{}{})
	dataMap = data.(map[string]interface{})
	if dataMap["overallHealth"].(string) != "unhealthy" {
		t.Errorf("Expected unhealthy status with 6 errors, got %s", dataMap["overallHealth"])
	}
}

func TestHandleReconnectService(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Test nil controller
	_, _, err := server.handleReconnectService(context.Background(), nil, ReconnectServiceInput{Key: "test"})
	if err == nil {
		t.Error("Expected error when controller is nil")
	}

	// Set up mock controller
	mock := &mockServiceController{}
	server.SetServiceController(mock)

	// Test successful reconnect
	result, data, err := server.handleReconnectService(context.Background(), nil, ReconnectServiceInput{Key: "svc1"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Error("Expected non-nil result")
	}
	dataMap := data.(map[string]interface{})
	if !dataMap["success"].(bool) {
		t.Error("Expected success to be true")
	}

	// Test reconnect error
	mock.reconnectErr = errors.New("reconnect failed")
	_, _, err = server.handleReconnectService(context.Background(), nil, ReconnectServiceInput{Key: "svc1"})
	if err == nil {
		t.Error("Expected error when reconnect fails")
	}
}

func TestHandleReconnectAllErrors(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Test nil controller
	_, _, err := server.handleReconnectAllErrors(context.Background(), nil, struct{}{})
	if err == nil {
		t.Error("Expected error when controller is nil")
	}

	// Set up mock controller
	mock := &mockServiceController{reconnected: 5}
	server.SetServiceController(mock)

	result, data, err := server.handleReconnectAllErrors(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Error("Expected non-nil result")
	}
	dataMap := data.(map[string]interface{})
	if dataMap["triggered"].(int) != 5 {
		t.Errorf("Expected 5 triggered, got %d", dataMap["triggered"])
	}
}

func TestHandleGetMetrics(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Test nil metrics/state
	_, _, err := server.handleGetMetrics(context.Background(), nil, GetMetricsInput{})
	if err == nil {
		t.Error("Expected error when metrics/state is nil")
	}

	// Set up mocks
	stateReader := &mockStateReader{
		summary: state.SummaryStats{TotalServices: 3, ActiveServices: 2, TotalForwards: 5, ActiveForwards: 4, ErrorCount: 1},
	}
	metricsProvider := &mockMetricsProvider{
		bytesIn: 1000, bytesOut: 2000, rateIn: 100, rateOut: 200,
		snapshots: []fwdmetrics.ServiceSnapshot{
			{ServiceName: "svc1", Namespace: "default", Context: "ctx1", TotalBytesIn: 500, TotalBytesOut: 1000},
			{ServiceName: "svc2", Namespace: "default", Context: "ctx1", TotalBytesIn: 500, TotalBytesOut: 1000},
		},
	}
	server.SetStateReader(stateReader)
	server.SetMetricsProvider(metricsProvider)

	// Test summary scope
	result, data, err := server.handleGetMetrics(context.Background(), nil, GetMetricsInput{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Error("Expected non-nil result")
	}
	dataMap := data.(map[string]interface{})
	if dataMap["totalBytesIn"].(uint64) != 1000 {
		t.Errorf("Expected 1000 bytesIn, got %d", dataMap["totalBytesIn"])
	}

	// Test by_service scope
	_, data, err = server.handleGetMetrics(context.Background(), nil, GetMetricsInput{Scope: "by_service"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	dataMap = data.(map[string]interface{})
	if _, ok := dataMap["services"]; !ok {
		t.Error("Expected services field for by_service scope")
	}

	// Test with manager info
	mockMgr := &mockManagerInfo{uptime: 5 * time.Minute}
	server.SetManagerInfo(func() types.ManagerInfo { return mockMgr })
	_, data, err = server.handleGetMetrics(context.Background(), nil, GetMetricsInput{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	dataMap = data.(map[string]interface{})
	if dataMap["uptime"].(string) == "" {
		t.Error("Expected uptime to be set when manager is available")
	}
}

func TestHandleGetLogs(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Test nil state reader
	_, _, err := server.handleGetLogs(context.Background(), nil, GetLogsInput{})
	if err == nil {
		t.Error("Expected error when state reader is nil")
	}

	// Set up mock with logs
	mock := &mockStateReader{
		logs: []state.LogEntry{
			{Timestamp: time.Now(), Level: "info", Message: "info message"},
			{Timestamp: time.Now(), Level: "error", Message: "error message"},
			{Timestamp: time.Now(), Level: "warn", Message: "warn message with keyword"},
			{Timestamp: time.Now(), Level: "debug", Message: "debug message"},
		},
	}
	server.SetStateReader(mock)

	// Test default count
	result, data, err := server.handleGetLogs(context.Background(), nil, GetLogsInput{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Error("Expected non-nil result")
	}

	// Test with count
	_, data, err = server.handleGetLogs(context.Background(), nil, GetLogsInput{Count: 2})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	dataMap := data.(map[string]interface{})
	// Note: mock returns all logs regardless of count, filtering happens in real implementation
	_ = dataMap

	// Test count cap at 500
	_, _, err = server.handleGetLogs(context.Background(), nil, GetLogsInput{Count: 1000})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Test level filter
	_, data, err = server.handleGetLogs(context.Background(), nil, GetLogsInput{Level: "error"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	dataMap = data.(map[string]interface{})
	logs := dataMap["logs"].([]map[string]interface{})
	if len(logs) != 1 {
		t.Errorf("Expected 1 error log, got %d", len(logs))
	}

	// Test search filter
	_, data, err = server.handleGetLogs(context.Background(), nil, GetLogsInput{Search: "keyword"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	dataMap = data.(map[string]interface{})
	logs = dataMap["logs"].([]map[string]interface{})
	if len(logs) != 1 {
		t.Errorf("Expected 1 log with keyword, got %d", len(logs))
	}
}

func TestHandleGetHealth(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Test nil state reader
	_, _, err := server.handleGetHealth(context.Background(), nil, struct{}{})
	if err == nil {
		t.Error("Expected error when state reader is nil")
	}

	// Test healthy state
	mock := &mockStateReader{
		summary: state.SummaryStats{TotalServices: 3, ActiveServices: 3, ErrorCount: 0},
	}
	server.SetStateReader(mock)

	result, data, err := server.handleGetHealth(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Error("Expected non-nil result")
	}
	dataMap := data.(map[string]interface{})
	if dataMap["status"].(string) != "healthy" {
		t.Errorf("Expected healthy status, got %s", dataMap["status"])
	}

	// Test degraded state
	mock.summary = state.SummaryStats{TotalServices: 3, ActiveServices: 2, ErrorCount: 1}
	_, data, err = server.handleGetHealth(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	dataMap = data.(map[string]interface{})
	if dataMap["status"].(string) != "degraded" {
		t.Errorf("Expected degraded status, got %s", dataMap["status"])
	}

	// Test unhealthy state (errors > active)
	mock.summary = state.SummaryStats{TotalServices: 3, ActiveServices: 1, ErrorCount: 2}
	_, data, err = server.handleGetHealth(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	dataMap = data.(map[string]interface{})
	if dataMap["status"].(string) != "unhealthy" {
		t.Errorf("Expected unhealthy status, got %s", dataMap["status"])
	}

	// Test with manager info
	mockMgr := &mockManagerInfo{
		version:    "1.0.0",
		uptime:     10 * time.Minute,
		startTime:  time.Now().Add(-10 * time.Minute),
		namespaces: []string{"default", "kube-system"},
		contexts:   []string{"minikube"},
		tuiEnabled: true,
	}
	server.SetManagerInfo(func() types.ManagerInfo { return mockMgr })
	_, data, err = server.handleGetHealth(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	dataMap = data.(map[string]interface{})
	if dataMap["tuiEnabled"].(bool) != true {
		t.Error("Expected tuiEnabled to be true")
	}
}

func TestHandleSyncService(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Test nil controller
	_, _, err := server.handleSyncService(context.Background(), nil, SyncServiceInput{Key: "test"})
	if err == nil {
		t.Error("Expected error when controller is nil")
	}

	// Set up mock controller
	mock := &mockServiceController{}
	server.SetServiceController(mock)

	// Test successful sync
	result, data, err := server.handleSyncService(context.Background(), nil, SyncServiceInput{Key: "svc1", Force: false})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Error("Expected non-nil result")
	}
	dataMap := data.(map[string]interface{})
	if !dataMap["success"].(bool) {
		t.Error("Expected success to be true")
	}

	// Test force sync
	result, data, err = server.handleSyncService(context.Background(), nil, SyncServiceInput{Key: "svc1", Force: true})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	dataMap = data.(map[string]interface{})
	if !dataMap["force"].(bool) {
		t.Error("Expected force to be true")
	}

	// Test sync error
	mock.syncErr = errors.New("sync failed")
	_, _, err = server.handleSyncService(context.Background(), nil, SyncServiceInput{Key: "svc1"})
	if err == nil {
		t.Error("Expected error when sync fails")
	}
}
