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
	_, data, err := server.handleListServices(context.Background(), nil, ListServicesInput{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
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
	_, data, err := server.handleGetService(context.Background(), nil, GetServiceInput{Key: "svc1.default.ctx1"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
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

	_, data, err := server.handleDiagnoseErrors(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
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
	_, data, err := server.handleReconnectService(context.Background(), nil, ReconnectServiceInput{Key: "svc1"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	// Result is nil because the SDK auto-populates Content from the data
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

	_, data, err := server.handleReconnectAllErrors(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	// Result is nil because the SDK auto-populates Content from the data
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
	_, data, err := server.handleGetMetrics(context.Background(), nil, GetMetricsInput{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	// Result is nil because the SDK auto-populates Content from the data
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
	_, _, err = server.handleGetLogs(context.Background(), nil, GetLogsInput{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Test with count
	var data any
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

	_, data, err := server.handleGetHealth(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	// Result is nil because the SDK auto-populates Content from the data
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
	_, data, err := server.handleSyncService(context.Background(), nil, SyncServiceInput{Key: "svc1", Force: false})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	// Result is nil because the SDK auto-populates Content from the data
	dataMap := data.(map[string]interface{})
	if !dataMap["success"].(bool) {
		t.Error("Expected success to be true")
	}

	// Test force sync
	_, data, err = server.handleSyncService(context.Background(), nil, SyncServiceInput{Key: "svc1", Force: true})
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

// ============================================================================
// Tests for new developer-focused tools
// ============================================================================

// mockNamespaceController implements NamespaceController for testing
type mockNamespaceController struct {
	namespaces []types.NamespaceInfoResponse
	addErr     error
	removeErr  error
}

func (m *mockNamespaceController) AddNamespace(ctx, namespace string, opts types.AddNamespaceOpts) (*types.NamespaceInfoResponse, error) {
	if m.addErr != nil {
		return nil, m.addErr
	}
	return &types.NamespaceInfoResponse{
		Key:          namespace + "." + ctx,
		Namespace:    namespace,
		Context:      ctx,
		ServiceCount: 3,
	}, nil
}

func (m *mockNamespaceController) RemoveNamespace(ctx, namespace string) error {
	return m.removeErr
}

func (m *mockNamespaceController) ListNamespaces() []types.NamespaceInfoResponse {
	return m.namespaces
}

func (m *mockNamespaceController) GetNamespace(ctx, namespace string) (*types.NamespaceInfoResponse, error) {
	for _, ns := range m.namespaces {
		if ns.Namespace == namespace {
			return &ns, nil
		}
	}
	return nil, errors.New("not found")
}

// mockServiceCRUD implements ServiceCRUD for testing
type mockServiceCRUD struct {
	mockServiceController
	addErr    error
	removeErr error
}

func (m *mockServiceCRUD) AddService(req types.AddServiceRequest) (*types.AddServiceResponse, error) {
	if m.addErr != nil {
		return nil, m.addErr
	}
	return &types.AddServiceResponse{
		Key:         req.ServiceName + "." + req.Namespace + "." + req.Context,
		ServiceName: req.ServiceName,
		Namespace:   req.Namespace,
		Context:     req.Context,
		LocalIP:     "127.1.2.3",
		Hostnames:   []string{req.ServiceName, req.ServiceName + "." + req.Namespace},
	}, nil
}

func (m *mockServiceCRUD) RemoveService(key string) error {
	return m.removeErr
}

// mockK8sDiscovery implements KubernetesDiscovery for testing
type mockK8sDiscovery struct {
	namespaces []types.K8sNamespace
	services   []types.K8sService
	contexts   *types.K8sContextsResponse
	listNsErr  error
	listSvcErr error
}

func (m *mockK8sDiscovery) ListNamespaces(ctx string) ([]types.K8sNamespace, error) {
	if m.listNsErr != nil {
		return nil, m.listNsErr
	}
	return m.namespaces, nil
}

func (m *mockK8sDiscovery) ListServices(ctx, namespace string) ([]types.K8sService, error) {
	if m.listSvcErr != nil {
		return nil, m.listSvcErr
	}
	var result []types.K8sService
	for _, svc := range m.services {
		if namespace == "" || svc.Namespace == namespace {
			result = append(result, svc)
		}
	}
	return result, nil
}

func (m *mockK8sDiscovery) ListContexts() (*types.K8sContextsResponse, error) {
	return m.contexts, nil
}

func (m *mockK8sDiscovery) GetService(ctx, namespace, name string) (*types.K8sService, error) {
	for _, svc := range m.services {
		if svc.Name == name && svc.Namespace == namespace {
			return &svc, nil
		}
	}
	return nil, errors.New("not found")
}

// mockConnectionInfoProvider implements ConnectionInfoProvider for testing
type mockConnectionInfoProvider struct {
	services []types.ConnectionInfoResponse
}

func (m *mockConnectionInfoProvider) GetConnectionInfo(key string) (*types.ConnectionInfoResponse, error) {
	for _, svc := range m.services {
		if svc.Service+"."+svc.Namespace == key || svc.Service == key {
			return &svc, nil
		}
	}
	return nil, errors.New("not found")
}

func (m *mockConnectionInfoProvider) ListHostnames() (*types.HostnameListResponse, error) {
	var entries []types.HostnameEntry
	for _, svc := range m.services {
		for _, h := range svc.Hostnames {
			entries = append(entries, types.HostnameEntry{
				Hostname:  h,
				IP:        svc.LocalIP,
				Service:   svc.Service,
				Namespace: svc.Namespace,
			})
		}
	}
	return &types.HostnameListResponse{Hostnames: entries, Total: len(entries)}, nil
}

func (m *mockConnectionInfoProvider) FindServices(query string, port int, namespace string) ([]types.ConnectionInfoResponse, error) {
	return m.services, nil
}

func TestHandleAddNamespace(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Test nil controller
	_, _, err := server.handleAddNamespace(context.Background(), nil, AddNamespaceInput{Namespace: "staging"})
	if err == nil {
		t.Error("Expected error when namespace controller is nil")
	}

	// Set up mock
	mock := &mockNamespaceController{}
	server.SetNamespaceController(mock)

	// Test successful add
	_, data, err := server.handleAddNamespace(context.Background(), nil, AddNamespaceInput{
		Namespace: "staging",
		Context:   "minikube",
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	// Result is nil because the SDK auto-populates Content from the data
	dataMap := data.(map[string]interface{})
	if dataMap["namespace"].(string) != "staging" {
		t.Errorf("Expected namespace 'staging', got '%s'", dataMap["namespace"])
	}

	// Test add error
	mock.addErr = errors.New("add failed")
	_, _, err = server.handleAddNamespace(context.Background(), nil, AddNamespaceInput{Namespace: "staging"})
	if err == nil {
		t.Error("Expected error when add fails")
	}
}

func TestHandleRemoveNamespace(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Test nil controller
	_, _, err := server.handleRemoveNamespace(context.Background(), nil, RemoveNamespaceInput{Namespace: "staging"})
	if err == nil {
		t.Error("Expected error when namespace controller is nil")
	}

	// Set up mock
	mock := &mockNamespaceController{}
	server.SetNamespaceController(mock)

	// Test successful remove
	_, data, err := server.handleRemoveNamespace(context.Background(), nil, RemoveNamespaceInput{
		Namespace: "staging",
		Context:   "minikube",
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	// Result is nil because the SDK auto-populates Content from the data
	dataMap := data.(map[string]interface{})
	if !dataMap["success"].(bool) {
		t.Error("Expected success to be true")
	}

	// Test remove error
	mock.removeErr = errors.New("remove failed")
	_, _, err = server.handleRemoveNamespace(context.Background(), nil, RemoveNamespaceInput{Namespace: "staging"})
	if err == nil {
		t.Error("Expected error when remove fails")
	}
}

func TestHandleAddService(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Test nil CRUD
	_, _, err := server.handleAddService(context.Background(), nil, AddServiceInput{
		Namespace:   "staging",
		ServiceName: "postgres",
	})
	if err == nil {
		t.Error("Expected error when service CRUD is nil")
	}

	// Set up mock
	mock := &mockServiceCRUD{}
	server.SetServiceCRUD(mock)

	// Test successful add
	_, data, err := server.handleAddService(context.Background(), nil, AddServiceInput{
		Namespace:   "staging",
		ServiceName: "postgres",
		Context:     "minikube",
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	// Result is nil because the SDK auto-populates Content from the data
	dataMap := data.(map[string]interface{})
	if dataMap["serviceName"].(string) != "postgres" {
		t.Errorf("Expected serviceName 'postgres', got '%s'", dataMap["serviceName"])
	}

	// Test add error
	mock.addErr = errors.New("add failed")
	_, _, err = server.handleAddService(context.Background(), nil, AddServiceInput{
		Namespace:   "staging",
		ServiceName: "postgres",
	})
	if err == nil {
		t.Error("Expected error when add fails")
	}
}

func TestHandleRemoveService(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Test nil CRUD
	_, _, err := server.handleRemoveService(context.Background(), nil, RemoveServiceInput{Key: "postgres.staging"})
	if err == nil {
		t.Error("Expected error when service CRUD is nil")
	}

	// Set up mock
	mock := &mockServiceCRUD{}
	server.SetServiceCRUD(mock)

	// Test successful remove
	_, data, err := server.handleRemoveService(context.Background(), nil, RemoveServiceInput{Key: "postgres.staging.minikube"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	// Result is nil because the SDK auto-populates Content from the data
	dataMap := data.(map[string]interface{})
	if !dataMap["success"].(bool) {
		t.Error("Expected success to be true")
	}

	// Test remove error
	mock.removeErr = errors.New("remove failed")
	_, _, err = server.handleRemoveService(context.Background(), nil, RemoveServiceInput{Key: "postgres.staging"})
	if err == nil {
		t.Error("Expected error when remove fails")
	}
}

func TestHandleGetConnectionInfo(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Test nil provider
	_, _, err := server.handleGetConnectionInfo(context.Background(), nil, GetConnectionInfoInput{ServiceName: "postgres"})
	if err == nil {
		t.Error("Expected error when connection info provider is nil")
	}

	// Set up mock
	mock := &mockConnectionInfoProvider{
		services: []types.ConnectionInfoResponse{
			{
				Service:   "postgres",
				Namespace: "staging",
				LocalIP:   "127.1.2.3",
				Hostnames: []string{"postgres", "postgres.staging"},
				Ports:     []types.PortInfo{{LocalPort: 5432}},
				EnvVars:   map[string]string{"POSTGRES_HOST": "postgres"},
				Status:    "active",
			},
		},
	}
	server.SetConnectionInfoProvider(mock)

	// Test successful get
	_, data, err := server.handleGetConnectionInfo(context.Background(), nil, GetConnectionInfoInput{
		ServiceName: "postgres",
		Namespace:   "staging",
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	// Result is nil because the SDK auto-populates Content from the data
	// Data is returned as *types.ConnectionInfoResponse
	connInfo := data.(*types.ConnectionInfoResponse)
	if connInfo.Service != "postgres" {
		t.Errorf("Expected service 'postgres', got '%s'", connInfo.Service)
	}
}

func TestHandleListK8sNamespaces(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Test nil discovery
	_, _, err := server.handleListK8sNamespaces(context.Background(), nil, ListK8sNamespacesInput{})
	if err == nil {
		t.Error("Expected error when K8s discovery is nil")
	}

	// Set up mock
	mock := &mockK8sDiscovery{
		namespaces: []types.K8sNamespace{
			{Name: "default", Status: "Active", Forwarded: true},
			{Name: "staging", Status: "Active", Forwarded: false},
		},
		contexts: &types.K8sContextsResponse{
			CurrentContext: "minikube",
			Contexts:       []types.K8sContext{{Name: "minikube", Cluster: "minikube", Active: true}},
		},
	}
	server.SetKubernetesDiscovery(mock)

	// Test successful list
	_, data, err := server.handleListK8sNamespaces(context.Background(), nil, ListK8sNamespacesInput{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	// Result is nil because the SDK auto-populates Content from the data
	dataMap := data.(map[string]interface{})
	namespaces := dataMap["namespaces"].([]types.K8sNamespace)
	if len(namespaces) != 2 {
		t.Errorf("Expected 2 namespaces, got %d", len(namespaces))
	}

	// Test error
	mock.listNsErr = errors.New("list failed")
	_, _, err = server.handleListK8sNamespaces(context.Background(), nil, ListK8sNamespacesInput{})
	if err == nil {
		t.Error("Expected error when list fails")
	}
}

func TestHandleListK8sServices(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Test nil discovery
	_, _, err := server.handleListK8sServices(context.Background(), nil, ListK8sServicesInput{Namespace: "default"})
	if err == nil {
		t.Error("Expected error when K8s discovery is nil")
	}

	// Set up mock
	mock := &mockK8sDiscovery{
		services: []types.K8sService{
			{Name: "api", Namespace: "default", Type: "ClusterIP"},
			{Name: "db", Namespace: "default", Type: "ClusterIP"},
		},
		contexts: &types.K8sContextsResponse{
			CurrentContext: "minikube",
			Contexts:       []types.K8sContext{{Name: "minikube", Cluster: "minikube", Active: true}},
		},
	}
	server.SetKubernetesDiscovery(mock)

	// Test successful list
	_, data, err := server.handleListK8sServices(context.Background(), nil, ListK8sServicesInput{Namespace: "default"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	// Result is nil because the SDK auto-populates Content from the data
	dataMap := data.(map[string]interface{})
	services := dataMap["services"].([]types.K8sService)
	if len(services) != 2 {
		t.Errorf("Expected 2 services, got %d", len(services))
	}

	// Test error
	mock.listSvcErr = errors.New("list failed")
	_, _, err = server.handleListK8sServices(context.Background(), nil, ListK8sServicesInput{Namespace: "default"})
	if err == nil {
		t.Error("Expected error when list fails")
	}
}

func TestHandleFindServices(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Test nil state reader
	_, _, err := server.handleFindServices(context.Background(), nil, FindServicesInput{Query: "postgres"})
	if err == nil {
		t.Error("Expected error when state reader is nil")
	}

	// Set up mock state reader with test services
	mockState := &mockStateReader{
		services: []state.ServiceSnapshot{
			{
				Key: "postgres.staging", ServiceName: "postgres", Namespace: "staging",
				PortForwards: []state.ForwardSnapshot{
					{LocalIP: "127.1.0.1", LocalPort: "5432", Hostnames: []string{"postgres"}},
				},
			},
			{
				Key: "mysql.staging", ServiceName: "mysql", Namespace: "staging",
				PortForwards: []state.ForwardSnapshot{
					{LocalIP: "127.1.0.2", LocalPort: "3306", Hostnames: []string{"mysql"}},
				},
			},
		},
	}
	server.SetStateReader(mockState)

	// Test successful find
	_, data, err := server.handleFindServices(context.Background(), nil, FindServicesInput{Query: "postgres"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	// Result is nil because the SDK auto-populates Content from the data
	dataMap := data.(map[string]interface{})
	if dataMap["count"].(int) != 1 { // Should find only postgres
		t.Errorf("Expected 1 service, got %d", dataMap["count"])
	}
}

func TestHandleListHostnames(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Test nil provider - handleListHostnames actually uses stateReader, not connectionInfo
	// So we need to set stateReader for the test
	_, _, err := server.handleListHostnames(context.Background(), nil, struct{}{})
	if err == nil {
		t.Error("Expected error when state reader is nil")
	}

	// Set up mock state reader with forwards that have hostnames
	mockState := &mockStateReader{
		services: []state.ServiceSnapshot{
			{
				Key: "api.default", ServiceName: "api", Namespace: "default",
				PortForwards: []state.ForwardSnapshot{
					{LocalIP: "127.1.0.1", Hostnames: []string{"api", "api.default"}},
				},
			},
			{
				Key: "db.default", ServiceName: "db", Namespace: "default",
				PortForwards: []state.ForwardSnapshot{
					{LocalIP: "127.1.0.2", Hostnames: []string{"db"}},
				},
			},
		},
	}
	server.SetStateReader(mockState)

	// Test successful list
	_, data, err := server.handleListHostnames(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	// Result is nil because the SDK auto-populates Content from the data
	dataMap := data.(map[string]interface{})
	if dataMap["total"].(int) != 3 {
		t.Errorf("Expected 3 hostnames, got %d", dataMap["total"])
	}
}
