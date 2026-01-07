package fwdmcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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

func (m *mockNamespaceController) AddNamespace(ctx, namespace string, _ types.AddNamespaceOpts) (*types.NamespaceInfoResponse, error) {
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

func (m *mockNamespaceController) RemoveNamespace(_, _ string) error {
	return m.removeErr
}

func (m *mockNamespaceController) ListNamespaces() []types.NamespaceInfoResponse {
	return m.namespaces
}

func (m *mockNamespaceController) GetNamespace(_, namespace string) (*types.NamespaceInfoResponse, error) {
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

func (m *mockServiceCRUD) RemoveService(_ string) error {
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

func (m *mockK8sDiscovery) ListNamespaces(_ string) ([]types.K8sNamespace, error) {
	if m.listNsErr != nil {
		return nil, m.listNsErr
	}
	return m.namespaces, nil
}

func (m *mockK8sDiscovery) ListServices(_, namespace string) ([]types.K8sService, error) {
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

func (m *mockK8sDiscovery) GetService(_, namespace, name string) (*types.K8sService, error) {
	for _, svc := range m.services {
		if svc.Name == name && svc.Namespace == namespace {
			return &svc, nil
		}
	}
	return nil, errors.New("not found")
}

func (m *mockK8sDiscovery) GetPodLogs(ctx, namespace, podName string, opts types.PodLogsOptions) (*types.PodLogsResponse, error) {
	// Return mock pod logs
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

func (m *mockK8sDiscovery) ListPods(_, namespace string, _ types.ListPodsOptions) ([]types.K8sPod, error) {
	return []types.K8sPod{
		{Name: "pod-1", Namespace: namespace, Phase: "Running", Status: "Running", Ready: "1/1"},
		{Name: "pod-2", Namespace: namespace, Phase: "Running", Status: "Running", Ready: "1/1"},
	}, nil
}

func (m *mockK8sDiscovery) GetPod(ctx, namespace, podName string) (*types.K8sPodDetail, error) {
	return &types.K8sPodDetail{
		Name:      podName,
		Namespace: namespace,
		Context:   ctx,
		Phase:     "Running",
		Status:    "Running",
		Containers: []types.K8sContainerInfo{
			{Name: "main", Image: "nginx:latest", Ready: true, State: "Running"},
		},
	}, nil
}

func (m *mockK8sDiscovery) GetEvents(_, _ string, _ types.GetEventsOptions) ([]types.K8sEvent, error) {
	return []types.K8sEvent{
		{Type: "Normal", Reason: "Scheduled", Message: "Successfully assigned pod"},
		{Type: "Normal", Reason: "Pulled", Message: "Container image already present"},
	}, nil
}

func (m *mockK8sDiscovery) GetEndpoints(_, namespace, serviceName string) (*types.K8sEndpoints, error) {
	return &types.K8sEndpoints{
		Name:      serviceName,
		Namespace: namespace,
		Subsets: []types.K8sEndpointSubset{
			{
				Addresses: []types.K8sEndpointAddress{{IP: "10.0.0.1", PodName: "pod-1"}},
				Ports:     []types.K8sEndpointPort{{Port: 80, Protocol: "TCP"}},
			},
		},
	}, nil
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

func (m *mockConnectionInfoProvider) FindServices(_ string, _ int, _ string) ([]types.ConnectionInfoResponse, error) {
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

func TestHandleGetConnectionInfo_StateFallback(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Set up state reader (no connection info provider)
	mock := &mockStateReader{
		services: []state.ServiceSnapshot{
			{
				Key:         "postgres.staging.ctx1",
				ServiceName: "postgres",
				Namespace:   "staging",
				Context:     "ctx1",
				PortForwards: []state.ForwardSnapshot{
					{
						LocalIP:   "127.1.2.3",
						LocalPort: "5432",
						PodPort:   "5432",
						Hostnames: []string{"postgres", "postgres.staging"},
					},
				},
			},
			{
				Key:         "mysql.production.ctx1",
				ServiceName: "mysql",
				Namespace:   "production",
				Context:     "ctx1",
				PortForwards: []state.ForwardSnapshot{
					{
						LocalIP:   "127.1.2.4",
						LocalPort: "3306",
						PodPort:   "3306",
						Hostnames: []string{"mysql", "mysql.production"},
					},
				},
			},
		},
	}
	server.SetStateReader(mock)

	// Test successful get via state fallback
	_, data, err := server.handleGetConnectionInfo(context.Background(), nil, GetConnectionInfoInput{
		ServiceName: "postgres",
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	result := data.(map[string]interface{})
	if result["service"] != "postgres" {
		t.Errorf("Expected service 'postgres', got '%v'", result["service"])
	}
	if result["localIP"] != "127.1.2.3" {
		t.Errorf("Expected localIP '127.1.2.3', got '%v'", result["localIP"])
	}

	// Test with namespace filter
	_, data, err = server.handleGetConnectionInfo(context.Background(), nil, GetConnectionInfoInput{
		ServiceName: "postgres",
		Namespace:   "staging",
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	result = data.(map[string]interface{})
	if result["namespace"] != "staging" {
		t.Errorf("Expected namespace 'staging', got '%v'", result["namespace"])
	}

	// Test with wrong namespace filter - should not find
	_, _, err = server.handleGetConnectionInfo(context.Background(), nil, GetConnectionInfoInput{
		ServiceName: "postgres",
		Namespace:   "nonexistent",
	})
	if err == nil {
		t.Error("Expected error when service not found with namespace filter")
	}

	// Test with context filter
	_, data, err = server.handleGetConnectionInfo(context.Background(), nil, GetConnectionInfoInput{
		ServiceName: "mysql",
		Context:     "ctx1",
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	result = data.(map[string]interface{})
	if result["context"] != "ctx1" {
		t.Errorf("Expected context 'ctx1', got '%v'", result["context"])
	}

	// Test service not found
	_, _, err = server.handleGetConnectionInfo(context.Background(), nil, GetConnectionInfoInput{
		ServiceName: "nonexistent",
	})
	if err == nil {
		t.Error("Expected error when service not found")
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

func TestHandleGetPodLogs(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Test nil k8s discovery
	_, _, err := server.handleGetPodLogs(context.Background(), nil, GetPodLogsInput{
		Namespace: "default",
		PodName:   "api-pod-123",
	})
	if err == nil {
		t.Error("Expected error when k8s discovery is nil")
	}

	// Set up mock
	mock := &mockK8sDiscovery{
		contexts: &types.K8sContextsResponse{
			Contexts:       []types.K8sContext{{Name: "dev", Active: true}},
			CurrentContext: "dev",
		},
	}
	server.SetKubernetesDiscovery(mock)

	// Test missing namespace
	_, _, err = server.handleGetPodLogs(context.Background(), nil, GetPodLogsInput{
		PodName: "api-pod-123",
	})
	if err == nil {
		t.Error("Expected error when namespace is missing")
	}

	// Test missing pod name
	_, _, err = server.handleGetPodLogs(context.Background(), nil, GetPodLogsInput{
		Namespace: "default",
	})
	if err == nil {
		t.Error("Expected error when pod_name is missing")
	}

	// Test successful log retrieval
	_, data, err := server.handleGetPodLogs(context.Background(), nil, GetPodLogsInput{
		Namespace: "default",
		PodName:   "api-pod-123",
		TailLines: 50,
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	dataMap := data.(map[string]interface{})
	if dataMap["podName"].(string) != "api-pod-123" {
		t.Errorf("Expected podName 'api-pod-123', got %v", dataMap["podName"])
	}
	if dataMap["namespace"].(string) != "default" {
		t.Errorf("Expected namespace 'default', got %v", dataMap["namespace"])
	}
	logs := dataMap["logs"].([]string)
	if len(logs) != 2 {
		t.Errorf("Expected 2 log lines, got %d", len(logs))
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

	// Test successful find by query
	_, data, err := server.handleFindServices(context.Background(), nil, FindServicesInput{Query: "postgres"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	// Result is nil because the SDK auto-populates Content from the data
	dataMap := data.(map[string]interface{})
	if dataMap["count"].(int) != 1 { // Should find only postgres
		t.Errorf("Expected 1 service, got %d", dataMap["count"])
	}

	// Test namespace filter
	_, data, err = server.handleFindServices(context.Background(), nil, FindServicesInput{Namespace: "staging"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	dataMap = data.(map[string]interface{})
	if dataMap["count"].(int) != 2 { // Should find both services in staging
		t.Errorf("Expected 2 services in staging namespace, got %d", dataMap["count"])
	}

	// Test namespace filter with non-matching namespace
	_, data, err = server.handleFindServices(context.Background(), nil, FindServicesInput{Namespace: "nonexistent"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	dataMap = data.(map[string]interface{})
	if dataMap["count"].(int) != 0 { // Should find nothing
		t.Errorf("Expected 0 services in nonexistent namespace, got %d", dataMap["count"])
	}

	// Test port filter
	_, data, err = server.handleFindServices(context.Background(), nil, FindServicesInput{Port: 5432})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	dataMap = data.(map[string]interface{})
	if dataMap["count"].(int) != 1 { // Should find only postgres on port 5432
		t.Errorf("Expected 1 service on port 5432, got %d", dataMap["count"])
	}

	// Test port filter with no match
	_, data, err = server.handleFindServices(context.Background(), nil, FindServicesInput{Port: 9999})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	dataMap = data.(map[string]interface{})
	if dataMap["count"].(int) != 0 { // Should find nothing on port 9999
		t.Errorf("Expected 0 services on port 9999, got %d", dataMap["count"])
	}

	// Test combined filters (query + namespace)
	_, data, err = server.handleFindServices(context.Background(), nil, FindServicesInput{Query: "sql", Namespace: "staging"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	dataMap = data.(map[string]interface{})
	if dataMap["count"].(int) != 1 { // Should find only mysql
		t.Errorf("Expected 1 service with 'sql' in staging, got %d", dataMap["count"])
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

func TestHandleListPods(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Test nil k8s discovery
	_, _, err := server.handleListPods(context.Background(), nil, ListPodsInput{
		Namespace: "default",
	})
	if err == nil {
		t.Error("Expected error when k8s discovery is nil")
	}

	// Set up mock
	mock := &mockK8sDiscovery{
		contexts: &types.K8sContextsResponse{
			Contexts:       []types.K8sContext{{Name: "dev", Active: true}},
			CurrentContext: "dev",
		},
	}
	server.SetKubernetesDiscovery(mock)

	// Test missing namespace
	_, _, err = server.handleListPods(context.Background(), nil, ListPodsInput{})
	if err == nil {
		t.Error("Expected error when namespace is missing")
	}

	// Test successful list
	_, data, err := server.handleListPods(context.Background(), nil, ListPodsInput{
		Namespace: "default",
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	dataMap := data.(map[string]interface{})
	pods := dataMap["pods"].([]types.K8sPod)
	if len(pods) != 2 {
		t.Errorf("Expected 2 pods, got %d", len(pods))
	}

	// Test with label selector
	_, data, err = server.handleListPods(context.Background(), nil, ListPodsInput{
		Namespace:     "default",
		LabelSelector: "app=api",
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	dataMap = data.(map[string]interface{})
	if dataMap["namespace"].(string) != "default" {
		t.Errorf("Expected namespace 'default', got %v", dataMap["namespace"])
	}

	// Test with service name filter
	_, data, err = server.handleListPods(context.Background(), nil, ListPodsInput{
		Namespace:   "default",
		ServiceName: "api-service",
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	dataMap = data.(map[string]interface{})
	if dataMap["count"].(int) != 2 {
		t.Errorf("Expected count 2, got %v", dataMap["count"])
	}
}

func TestHandleGetPod(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Test nil k8s discovery
	_, _, err := server.handleGetPod(context.Background(), nil, GetPodInput{
		Namespace: "default",
		PodName:   "api-pod-123",
	})
	if err == nil {
		t.Error("Expected error when k8s discovery is nil")
	}

	// Set up mock
	mock := &mockK8sDiscovery{
		contexts: &types.K8sContextsResponse{
			Contexts:       []types.K8sContext{{Name: "dev", Active: true}},
			CurrentContext: "dev",
		},
	}
	server.SetKubernetesDiscovery(mock)

	// Test missing namespace
	_, _, err = server.handleGetPod(context.Background(), nil, GetPodInput{
		PodName: "api-pod-123",
	})
	if err == nil {
		t.Error("Expected error when namespace is missing")
	}

	// Test missing pod name
	_, _, err = server.handleGetPod(context.Background(), nil, GetPodInput{
		Namespace: "default",
	})
	if err == nil {
		t.Error("Expected error when pod_name is missing")
	}

	// Test successful get
	_, data, err := server.handleGetPod(context.Background(), nil, GetPodInput{
		Namespace: "default",
		PodName:   "api-pod-123",
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	podDetail := data.(*types.K8sPodDetail)
	if podDetail.Name != "api-pod-123" {
		t.Errorf("Expected pod name 'api-pod-123', got %v", podDetail.Name)
	}
	if podDetail.Namespace != "default" {
		t.Errorf("Expected namespace 'default', got %v", podDetail.Namespace)
	}
	if len(podDetail.Containers) != 1 {
		t.Errorf("Expected 1 container, got %d", len(podDetail.Containers))
	}
}

func TestHandleGetEvents(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Test nil k8s discovery
	_, _, err := server.handleGetEvents(context.Background(), nil, GetEventsInput{
		Namespace: "default",
	})
	if err == nil {
		t.Error("Expected error when k8s discovery is nil")
	}

	// Set up mock
	mock := &mockK8sDiscovery{
		contexts: &types.K8sContextsResponse{
			Contexts:       []types.K8sContext{{Name: "dev", Active: true}},
			CurrentContext: "dev",
		},
	}
	server.SetKubernetesDiscovery(mock)

	// Test missing namespace
	_, _, err = server.handleGetEvents(context.Background(), nil, GetEventsInput{})
	if err == nil {
		t.Error("Expected error when namespace is missing")
	}

	// Test successful get
	_, data, err := server.handleGetEvents(context.Background(), nil, GetEventsInput{
		Namespace: "default",
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	dataMap := data.(map[string]interface{})
	events := dataMap["events"].([]types.K8sEvent)
	if len(events) != 2 {
		t.Errorf("Expected 2 events, got %d", len(events))
	}

	// Test with resource filter
	_, data, err = server.handleGetEvents(context.Background(), nil, GetEventsInput{
		Namespace:    "default",
		ResourceKind: "Pod",
		ResourceName: "api-pod-123",
		Limit:        10,
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	dataMap = data.(map[string]interface{})
	if dataMap["namespace"].(string) != "default" {
		t.Errorf("Expected namespace 'default', got %v", dataMap["namespace"])
	}
}

func TestHandleGetEndpoints(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Test nil k8s discovery
	_, _, err := server.handleGetEndpoints(context.Background(), nil, GetEndpointsInput{
		Namespace:   "default",
		ServiceName: "api-service",
	})
	if err == nil {
		t.Error("Expected error when k8s discovery is nil")
	}

	// Set up mock
	mock := &mockK8sDiscovery{
		contexts: &types.K8sContextsResponse{
			Contexts:       []types.K8sContext{{Name: "dev", Active: true}},
			CurrentContext: "dev",
		},
	}
	server.SetKubernetesDiscovery(mock)

	// Test missing namespace
	_, _, err = server.handleGetEndpoints(context.Background(), nil, GetEndpointsInput{
		ServiceName: "api-service",
	})
	if err == nil {
		t.Error("Expected error when namespace is missing")
	}

	// Test missing service name
	_, _, err = server.handleGetEndpoints(context.Background(), nil, GetEndpointsInput{
		Namespace: "default",
	})
	if err == nil {
		t.Error("Expected error when service_name is missing")
	}

	// Test successful get
	_, data, err := server.handleGetEndpoints(context.Background(), nil, GetEndpointsInput{
		Namespace:   "default",
		ServiceName: "api-service",
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	endpoints := data.(*types.K8sEndpoints)
	if endpoints.Name != "api-service" {
		t.Errorf("Expected service name 'api-service', got %v", endpoints.Name)
	}
	if endpoints.Namespace != "default" {
		t.Errorf("Expected namespace 'default', got %v", endpoints.Namespace)
	}
	if len(endpoints.Subsets) != 1 {
		t.Errorf("Expected 1 subset, got %d", len(endpoints.Subsets))
	}
	if len(endpoints.Subsets[0].Addresses) != 1 {
		t.Errorf("Expected 1 address, got %d", len(endpoints.Subsets[0].Addresses))
	}
}

func TestHandleListContexts(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Test nil k8s discovery
	_, _, err := server.handleListContexts(context.Background(), nil, struct{}{})
	if err == nil {
		t.Error("Expected error when k8s discovery is nil")
	}

	// Set up mock
	mock := &mockK8sDiscovery{
		contexts: &types.K8sContextsResponse{
			Contexts: []types.K8sContext{
				{Name: "dev", Cluster: "dev-cluster", Active: true},
				{Name: "prod", Cluster: "prod-cluster", Active: false},
			},
			CurrentContext: "dev",
		},
	}
	server.SetKubernetesDiscovery(mock)

	// Test successful list
	_, data, err := server.handleListContexts(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	dataMap := data.(map[string]interface{})
	if dataMap["current"].(string) != "dev" {
		t.Errorf("Expected current context 'dev', got %v", dataMap["current"])
	}
	if dataMap["count"].(int) != 2 {
		t.Errorf("Expected 2 contexts, got %v", dataMap["count"])
	}
	contexts := dataMap["contexts"].([]types.K8sContext)
	if len(contexts) != 2 {
		t.Errorf("Expected 2 contexts in list, got %d", len(contexts))
	}
}

func TestHandleGetAnalysis(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Test nil analysis provider - this covers the handler entry point
	_, _, err := server.handleGetAnalysis(context.Background(), nil, struct{}{})
	if err == nil {
		t.Error("Expected error when analysis provider is nil")
	}
}

func TestHandleGetQuickStatus(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Test nil analysis provider - this covers the handler entry point
	_, _, err := server.handleGetQuickStatus(context.Background(), nil, struct{}{})
	if err == nil {
		t.Error("Expected error when analysis provider is nil")
	}
}

func TestHandleGetHistory(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Test nil history provider - this covers the handler entry point
	_, _, err := server.handleGetHistory(context.Background(), nil, GetHistoryInput{Type: "events"})
	if err == nil {
		t.Error("Expected error when history provider is nil")
	}
}

func TestHandleGetHTTPTraffic(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Test nil HTTP traffic provider
	_, _, err := server.handleGetHTTPTraffic(context.Background(), nil, GetHTTPTrafficInput{})
	if err == nil {
		t.Error("Expected error when HTTP traffic provider is nil")
	}

	// Test missing keys
	_, _, err = server.handleGetHTTPTraffic(context.Background(), nil, GetHTTPTrafficInput{
		ServiceKey: "",
		ForwardKey: "",
	})
	if err == nil {
		t.Error("Expected error when both service_key and forward_key are missing")
	}
}

func TestHandleGetAnalysis_WithProvider(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Create mock HTTP server that returns analysis data
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := map[string]interface{}{
			"status":          "healthy",
			"services":        10,
			"errors":          0,
			"issues":          []interface{}{},
			"recommendations": []interface{}{},
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	// Create real provider with mock server
	provider := NewAnalysisProviderHTTP(mockServer.URL)
	server.SetAnalysisProvider(provider)

	// Test successful analysis
	_, data, err := server.handleGetAnalysis(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if data == nil {
		t.Fatal("Expected non-nil data")
	}
}

func TestHandleGetQuickStatus_WithProvider(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Create mock HTTP server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := map[string]interface{}{
			"status":   "ok",
			"message":  "All services healthy",
			"errors":   0,
			"services": 5,
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	provider := NewAnalysisProviderHTTP(mockServer.URL)
	server.SetAnalysisProvider(provider)

	// Test successful status
	_, data, err := server.handleGetQuickStatus(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if data == nil {
		t.Fatal("Expected non-nil data")
	}
}

func TestHandleGetHistory_WithProvider(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Create mock HTTP server that handles different history types with proper response format
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Check URL path to determine response - must use /v1/ path prefix
		switch r.URL.Path {
		case "/v1/history/events":
			response := map[string]interface{}{
				"success": true,
				"data": []map[string]interface{}{
					{"id": 1, "type": "service_added", "service_key": "svc1.ns1.ctx1", "timestamp": time.Now().Format(time.RFC3339)},
					{"id": 2, "type": "forward_started", "service_key": "svc1.ns1.ctx1", "timestamp": time.Now().Format(time.RFC3339)},
				},
			}
			_ = json.NewEncoder(w).Encode(response)

		case "/v1/history/errors":
			response := map[string]interface{}{
				"success": true,
				"data": []map[string]interface{}{
					{"id": 1, "error_type": "connection_refused", "service_key": "svc1.ns1.ctx1", "timestamp": time.Now().Format(time.RFC3339)},
				},
			}
			_ = json.NewEncoder(w).Encode(response)

		case "/v1/history/reconnections":
			response := map[string]interface{}{
				"success": true,
				"data": []map[string]interface{}{
					{"id": 1, "trigger": "auto", "service_key": "svc1.ns1.ctx1", "success": true, "timestamp": time.Now().Format(time.RFC3339)},
				},
			}
			_ = json.NewEncoder(w).Encode(response)

		default:
			response := map[string]interface{}{
				"success": true,
				"data":    []interface{}{},
			}
			_ = json.NewEncoder(w).Encode(response)
		}
	}))
	defer mockServer.Close()

	provider := NewHistoryProviderHTTP(mockServer.URL)
	server.SetHistoryProvider(provider)

	// Test events history
	_, data, err := server.handleGetHistory(context.Background(), nil, GetHistoryInput{Type: "events", Count: 10})
	if err != nil {
		t.Fatalf("Unexpected error for events: %v", err)
	}
	if data == nil {
		t.Fatal("Expected non-nil data for events")
	}
	dataMap := data.(map[string]interface{})
	if dataMap["type"].(string) != "events" {
		t.Errorf("Expected type 'events', got %v", dataMap["type"])
	}

	// Test errors history
	_, data, err = server.handleGetHistory(context.Background(), nil, GetHistoryInput{Type: "errors", Count: 10})
	if err != nil {
		t.Fatalf("Unexpected error for errors: %v", err)
	}
	dataMap = data.(map[string]interface{})
	if dataMap["type"].(string) != "errors" {
		t.Errorf("Expected type 'errors', got %v", dataMap["type"])
	}

	// Test reconnections history
	_, data, err = server.handleGetHistory(context.Background(), nil, GetHistoryInput{Type: "reconnections", Count: 10})
	if err != nil {
		t.Fatalf("Unexpected error for reconnections: %v", err)
	}
	dataMap = data.(map[string]interface{})
	if dataMap["type"].(string) != "reconnections" {
		t.Errorf("Expected type 'reconnections', got %v", dataMap["type"])
	}

	// Test with empty type (should default to events)
	_, data, err = server.handleGetHistory(context.Background(), nil, GetHistoryInput{Type: "", Count: 10})
	if err != nil {
		t.Fatalf("Unexpected error for empty type: %v", err)
	}
	dataMap = data.(map[string]interface{})
	if dataMap["type"].(string) != "events" {
		t.Errorf("Expected default type 'events', got %v", dataMap["type"])
	}

	// Test with default count (zero count)
	_, data, err = server.handleGetHistory(context.Background(), nil, GetHistoryInput{Type: "errors", Count: 0})
	if err != nil {
		t.Fatalf("Unexpected error with zero count: %v", err)
	}
	if data == nil {
		t.Fatal("Expected non-nil data with zero count")
	}
}

func TestHandleGetHTTPTraffic_WithProvider(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Create mock HTTP server with proper response format
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// All traffic endpoints return the same format - wrapped response
		response := map[string]interface{}{
			"success": true,
			"data": map[string]interface{}{
				"forward_key": "test-fwd",
				"requests": []map[string]interface{}{
					{
						"method":      "GET",
						"path":        "/api/health",
						"status_code": 200,
						"duration_ms": 15,
						"timestamp":   time.Now().Format(time.RFC3339),
					},
					{
						"method":      "POST",
						"path":        "/api/users",
						"status_code": 201,
						"duration_ms": 45,
						"timestamp":   time.Now().Format(time.RFC3339),
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	provider := NewHTTPTrafficProviderHTTP(mockServer.URL)
	server.SetHTTPTrafficProvider(provider)

	// Test with service_key
	_, data, err := server.handleGetHTTPTraffic(context.Background(), nil, GetHTTPTrafficInput{
		ServiceKey: "svc1.ns1.ctx1",
		Count:      10,
	})
	if err != nil {
		t.Fatalf("Unexpected error with service_key: %v", err)
	}
	if data == nil {
		t.Fatal("Expected non-nil data with service_key")
	}

	// Test with forward_key
	_, data, err = server.handleGetHTTPTraffic(context.Background(), nil, GetHTTPTrafficInput{
		ForwardKey: "fwd-key",
		Count:      10,
	})
	if err != nil {
		t.Fatalf("Unexpected error with forward_key: %v", err)
	}
	if data == nil {
		t.Fatal("Expected non-nil data with forward_key")
	}

	// Test with default count
	_, data, err = server.handleGetHTTPTraffic(context.Background(), nil, GetHTTPTrafficInput{
		ServiceKey: "svc1.ns1.ctx1",
		Count:      0,
	})
	if err != nil {
		t.Fatalf("Unexpected error with zero count: %v", err)
	}
	if data == nil {
		t.Fatal("Expected non-nil data with zero count")
	}
}

// Tests for extracted helper functions

func TestBuildConnectionInfoFromService(t *testing.T) {
	svc := &state.ServiceSnapshot{
		ServiceName: "test-service",
		Namespace:   "default",
		Context:     "test-context",
		PortForwards: []state.ForwardSnapshot{
			{LocalIP: "127.1.1.1", LocalPort: "8080", PodPort: "80", Hostnames: []string{"test-host"}},
		},
	}

	result := buildConnectionInfoFromService(svc)

	if result["service"] != "test-service" {
		t.Errorf("Expected service name 'test-service', got %v", result["service"])
	}
	if result["namespace"] != "default" {
		t.Errorf("Expected namespace 'default', got %v", result["namespace"])
	}
	if result["localIP"] != "127.1.1.1" {
		t.Errorf("Expected localIP '127.1.1.1', got %v", result["localIP"])
	}
}

func TestFindServiceInState(t *testing.T) {
	mock := &mockStateReader{
		services: []state.ServiceSnapshot{
			{Key: "svc1.default.ctx1", ServiceName: "svc1", Namespace: "default", Context: "ctx1"},
			{Key: "svc2.kube-system.ctx1", ServiceName: "svc2", Namespace: "kube-system", Context: "ctx1"},
		},
	}

	// Test finding existing service
	svc, err := findServiceInState(mock, "svc1", "", "")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if svc.ServiceName != "svc1" {
		t.Errorf("Expected svc1, got %s", svc.ServiceName)
	}

	// Test with namespace filter
	svc, err = findServiceInState(mock, "svc1", "default", "")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if svc.ServiceName != "svc1" {
		t.Errorf("Expected svc1, got %s", svc.ServiceName)
	}

	// Test service not found
	_, err = findServiceInState(mock, "nonexistent", "", "")
	if err == nil {
		t.Error("Expected error for nonexistent service")
	}

	// Test namespace mismatch
	_, err = findServiceInState(mock, "svc1", "kube-system", "")
	if err == nil {
		t.Error("Expected error for namespace mismatch")
	}
}

// mockSearchProvider implements ConnectionInfoProvider for searchServicesByName tests
type mockSearchProvider struct {
	findResult []types.ConnectionInfoResponse
	findErr    error
}

func (m *mockSearchProvider) GetConnectionInfo(_ string) (*types.ConnectionInfoResponse, error) {
	return nil, nil
}

func (m *mockSearchProvider) FindServices(_ string, _ int, _ string) ([]types.ConnectionInfoResponse, error) {
	return m.findResult, m.findErr
}

func (m *mockSearchProvider) ListHostnames() (*types.HostnameListResponse, error) {
	return &types.HostnameListResponse{}, nil
}

func TestSearchServicesByName(t *testing.T) {
	// Test with exact match
	mock := &mockSearchProvider{
		findResult: []types.ConnectionInfoResponse{
			{Service: "my-service", Namespace: "default"},
			{Service: "my-service-extra", Namespace: "default"},
		},
	}

	results, err := searchServicesByName(mock, "my-service", 0)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Expected 1 exact match, got %d", len(results))
	}

	// Test with error
	mock.findErr = errors.New("test error")
	_, err = searchServicesByName(mock, "my-service", 0)
	if err == nil {
		t.Error("Expected error")
	}
}

func TestCalculateMCPServiceStatus(t *testing.T) {
	tests := []struct {
		name        string
		activeCount int
		errorCount  int
		want        string
	}{
		{"active only", 2, 0, "active"},
		{"error only", 0, 2, "error"},
		{"partial", 1, 1, "partial"},
		{"pending", 0, 0, "pending"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateMCPServiceStatus(tt.activeCount, tt.errorCount)
			if got != tt.want {
				t.Errorf("calculateMCPServiceStatus(%d, %d) = %q, want %q", tt.activeCount, tt.errorCount, got, tt.want)
			}
		})
	}
}
