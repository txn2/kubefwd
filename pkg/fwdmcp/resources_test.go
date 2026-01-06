package fwdmcp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/txn2/kubefwd/pkg/fwdapi/types"
	"github.com/txn2/kubefwd/pkg/fwdmetrics"
	"github.com/txn2/kubefwd/pkg/fwdtui/state"
)

func TestHandleServicesResource(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	req := &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{
			URI: "kubefwd://services",
		},
	}

	// Test nil state reader
	_, err := server.handleServicesResource(context.Background(), req)
	if err == nil {
		t.Error("Expected error when state reader is nil")
	}

	// Set up mock with various statuses
	mock := &mockStateReader{
		services: []state.ServiceSnapshot{
			{Key: "svc1.default.ctx1", ServiceName: "svc1", Namespace: "default", Context: "ctx1", ActiveCount: 1, ErrorCount: 0},
			{Key: "svc2.default.ctx1", ServiceName: "svc2", Namespace: "default", Context: "ctx1", ActiveCount: 0, ErrorCount: 1},
			{Key: "svc3.default.ctx1", ServiceName: "svc3", Namespace: "default", Context: "ctx1", ActiveCount: 1, ErrorCount: 1},
			{Key: "svc4.default.ctx1", ServiceName: "svc4", Namespace: "default", Context: "ctx1", ActiveCount: 0, ErrorCount: 0},
		},
	}
	server.SetStateReader(mock)

	result, err := server.handleServicesResource(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if len(result.Contents) != 1 {
		t.Errorf("Expected 1 content, got %d", len(result.Contents))
	}
	if result.Contents[0].MIMEType != "application/json" {
		t.Errorf("Expected application/json MIME type, got %s", result.Contents[0].MIMEType)
	}
	if result.Contents[0].Text == "" {
		t.Error("Expected non-empty text content")
	}
}

func TestHandleForwardsResource(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	req := &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{
			URI: "kubefwd://forwards",
		},
	}

	// Test nil state reader
	_, err := server.handleForwardsResource(context.Background(), req)
	if err == nil {
		t.Error("Expected error when state reader is nil")
	}

	// Set up mock
	mock := &mockStateReader{
		forwards: []state.ForwardSnapshot{
			{
				Key:           "fwd1",
				ServiceKey:    "svc1.default.ctx1",
				ServiceName:   "svc1",
				Namespace:     "default",
				Context:       "ctx1",
				PodName:       "pod1",
				ContainerName: "container1",
				LocalIP:       "127.1.0.1",
				LocalPort:     "80",
				PodPort:       "8080",
				Hostnames:     []string{"svc1", "svc1.default"},
				Status:        state.StatusActive,
				StartedAt:     time.Now(),
				LastActive:    time.Now(),
				BytesIn:       1000,
				BytesOut:      2000,
			},
		},
	}
	server.SetStateReader(mock)

	result, err := server.handleForwardsResource(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if result.Contents[0].Text == "" {
		t.Error("Expected non-empty text content")
	}
}

func TestHandleMetricsResource(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	req := &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{
			URI: "kubefwd://metrics",
		},
	}

	// Test nil metrics/state
	_, err := server.handleMetricsResource(context.Background(), req)
	if err == nil {
		t.Error("Expected error when metrics/state is nil")
	}

	// Set up only state - should still fail
	stateReader := &mockStateReader{
		summary: state.SummaryStats{TotalServices: 2, ActiveServices: 1},
	}
	server.SetStateReader(stateReader)
	_, err = server.handleMetricsResource(context.Background(), req)
	if err == nil {
		t.Error("Expected error when metrics is nil")
	}

	// Set up both state and metrics
	metricsProvider := &mockMetricsProvider{
		bytesIn: 1000, bytesOut: 2000, rateIn: 100, rateOut: 200,
	}
	server.SetMetricsProvider(metricsProvider)

	result, err := server.handleMetricsResource(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	// Test with manager info
	mockMgr := &mockManagerInfo{uptime: 5 * time.Minute}
	server.SetManagerInfo(func() types.ManagerInfo { return mockMgr })

	result, err = server.handleMetricsResource(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
}

func TestHandleSummaryResource(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	req := &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{
			URI: "kubefwd://summary",
		},
	}

	// Test nil state reader
	_, err := server.handleSummaryResource(context.Background(), req)
	if err == nil {
		t.Error("Expected error when state reader is nil")
	}

	// Test healthy state
	mock := &mockStateReader{
		summary: state.SummaryStats{
			TotalServices:  3,
			ActiveServices: 3,
			TotalForwards:  5,
			ActiveForwards: 5,
			ErrorCount:     0,
			TotalBytesIn:   10000,
			TotalBytesOut:  20000,
		},
	}
	server.SetStateReader(mock)

	result, err := server.handleSummaryResource(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	// Test degraded state
	mock.summary.ErrorCount = 1
	_, err = server.handleSummaryResource(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Test unhealthy state (errors > active)
	mock.summary.ErrorCount = 5
	mock.summary.ActiveServices = 2
	_, err = server.handleSummaryResource(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Test with manager info
	mockMgr := &mockManagerInfo{
		uptime:     10 * time.Minute,
		startTime:  time.Now().Add(-10 * time.Minute),
		namespaces: []string{"default"},
		contexts:   []string{"minikube"},
	}
	server.SetManagerInfo(func() types.ManagerInfo { return mockMgr })

	_, err = server.handleSummaryResource(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
}

func TestHandleErrorsResource(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	req := &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{
			URI: "kubefwd://errors",
		},
	}

	// Test nil state reader
	_, err := server.handleErrorsResource(context.Background(), req)
	if err == nil {
		t.Error("Expected error when state reader is nil")
	}

	// Test no errors
	mock := &mockStateReader{
		services: []state.ServiceSnapshot{
			{Key: "svc1", ServiceName: "svc1", ErrorCount: 0},
		},
	}
	server.SetStateReader(mock)

	result, err := server.handleErrorsResource(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	// Test with errors
	mock.services = []state.ServiceSnapshot{
		{
			Key:         "svc1.default.ctx1",
			ServiceName: "svc1",
			Namespace:   "default",
			Context:     "ctx1",
			ErrorCount:  2,
			PortForwards: []state.ForwardSnapshot{
				{PodName: "pod1", LocalIP: "127.1.0.1", LocalPort: "80", Error: "connection refused", Status: state.StatusError},
				{PodName: "pod2", LocalIP: "127.1.0.2", LocalPort: "80", Error: ""}, // no error
				{PodName: "pod3", LocalIP: "127.1.0.3", LocalPort: "80", Error: "timeout", Status: state.StatusError},
			},
		},
		{
			Key:         "svc2.default.ctx1",
			ServiceName: "svc2",
			ErrorCount:  0, // no errors in this service
		},
	}

	result, err = server.handleErrorsResource(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	// Should contain 2 errors from svc1
}

// mockK8sDiscoveryResource implements KubernetesDiscovery for resource tests
type mockK8sDiscoveryResource struct {
	contextsResp *types.K8sContextsResponse
	contextsErr  error
}

func (m *mockK8sDiscoveryResource) ListNamespaces(ctx string) ([]types.K8sNamespace, error) {
	return nil, nil
}
func (m *mockK8sDiscoveryResource) ListServices(ctx, namespace string) ([]types.K8sService, error) {
	return nil, nil
}
func (m *mockK8sDiscoveryResource) ListContexts() (*types.K8sContextsResponse, error) {
	return m.contextsResp, m.contextsErr
}
func (m *mockK8sDiscoveryResource) GetService(ctx, namespace, name string) (*types.K8sService, error) {
	return nil, nil
}
func (m *mockK8sDiscoveryResource) GetPodLogs(ctx, namespace, podName string, opts types.PodLogsOptions) (*types.PodLogsResponse, error) {
	return nil, nil
}
func (m *mockK8sDiscoveryResource) ListPods(ctx, namespace string, opts types.ListPodsOptions) ([]types.K8sPod, error) {
	return nil, nil
}
func (m *mockK8sDiscoveryResource) GetPod(ctx, namespace, podName string) (*types.K8sPodDetail, error) {
	return nil, nil
}
func (m *mockK8sDiscoveryResource) GetEvents(ctx, namespace string, opts types.GetEventsOptions) ([]types.K8sEvent, error) {
	return nil, nil
}
func (m *mockK8sDiscoveryResource) GetEndpoints(ctx, namespace, serviceName string) (*types.K8sEndpoints, error) {
	return nil, nil
}

// mockMetricsProviderWithSnapshots extends mockMetricsProvider to support GetServiceSnapshot
type mockMetricsProviderWithSnapshots struct {
	snapshots      []fwdmetrics.ServiceSnapshot
	snapshotsByKey map[string]*fwdmetrics.ServiceSnapshot
	bytesIn        uint64
	bytesOut       uint64
	rateIn         float64
	rateOut        float64
}

func (m *mockMetricsProviderWithSnapshots) GetAllSnapshots() []fwdmetrics.ServiceSnapshot {
	return m.snapshots
}
func (m *mockMetricsProviderWithSnapshots) GetServiceSnapshot(key string) *fwdmetrics.ServiceSnapshot {
	if m.snapshotsByKey != nil {
		return m.snapshotsByKey[key]
	}
	return nil
}
func (m *mockMetricsProviderWithSnapshots) GetTotals() (uint64, uint64, float64, float64) {
	return m.bytesIn, m.bytesOut, m.rateIn, m.rateOut
}
func (m *mockMetricsProviderWithSnapshots) ServiceCount() int     { return len(m.snapshots) }
func (m *mockMetricsProviderWithSnapshots) PortForwardCount() int { return 0 }

func TestResourcesMIMEType(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Set up required mocks
	mock := &mockStateReader{
		services: []state.ServiceSnapshot{{Key: "svc1"}},
		forwards: []state.ForwardSnapshot{{Key: "fwd1"}},
		summary:  state.SummaryStats{},
	}
	metricsProvider := &mockMetricsProvider{}
	server.SetStateReader(mock)
	server.SetMetricsProvider(metricsProvider)

	testCases := []struct {
		name    string
		uri     string
		handler func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error)
	}{
		{"services", "kubefwd://services", server.handleServicesResource},
		{"forwards", "kubefwd://forwards", server.handleForwardsResource},
		{"metrics", "kubefwd://metrics", server.handleMetricsResource},
		{"summary", "kubefwd://summary", server.handleSummaryResource},
		{"errors", "kubefwd://errors", server.handleErrorsResource},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := &mcp.ReadResourceRequest{
				Params: &mcp.ReadResourceParams{
					URI: tc.uri,
				},
			}

			result, err := tc.handler(context.Background(), req)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if result.Contents[0].MIMEType != "application/json" {
				t.Errorf("Expected application/json MIME type for %s, got %s", tc.name, result.Contents[0].MIMEType)
			}
			if result.Contents[0].URI != tc.uri {
				t.Errorf("Expected URI %s, got %s", tc.uri, result.Contents[0].URI)
			}
		})
	}
}

func TestHandleStatusResource(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	req := &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{
			URI: "kubefwd://status",
		},
	}

	// Test with no state or analysis provider - should fail
	_, err := server.handleStatusResource(context.Background(), req)
	if err == nil {
		t.Error("Expected error when both analysis and state are nil")
	}

	// Test with state-based fallback (no analysis provider)
	// Test healthy state
	mock := &mockStateReader{
		summary: state.SummaryStats{
			TotalServices:  5,
			ActiveServices: 5,
			ErrorCount:     0,
		},
	}
	server.SetStateReader(mock)

	result, err := server.handleStatusResource(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if result.Contents[0].MIMEType != "application/json" {
		t.Errorf("Expected application/json MIME type, got %s", result.Contents[0].MIMEType)
	}

	// Test state with issues (some errors, but not all)
	mock.summary.ErrorCount = 2
	result, err = server.handleStatusResource(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	// Test state with errors (errors >= active)
	mock.summary.ErrorCount = 6
	result, err = server.handleStatusResource(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	// Test state with no services
	mock.summary = state.SummaryStats{
		TotalServices:  0,
		ActiveServices: 0,
		ErrorCount:     0,
	}
	result, err = server.handleStatusResource(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
}

func TestHandleHTTPTrafficResource(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	req := &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{
			URI: "kubefwd://http-traffic",
		},
	}

	// Test with no state or metrics - should fail
	_, err := server.handleHTTPTrafficResource(context.Background(), req)
	if err == nil {
		t.Error("Expected error when state or metrics is nil")
	}

	// Set up state reader
	mock := &mockStateReader{
		services: []state.ServiceSnapshot{
			{Key: "svc1.default.ctx1", ServiceName: "svc1", Namespace: "default", Context: "ctx1"},
			{Key: "svc2.default.ctx1", ServiceName: "svc2", Namespace: "default", Context: "ctx1"},
		},
	}
	server.SetStateReader(mock)

	// Still no metrics - should fail
	_, err = server.handleHTTPTrafficResource(context.Background(), req)
	if err == nil {
		t.Error("Expected error when metrics is nil")
	}

	// Set up metrics provider with HTTP logs
	metricsProvider := &mockMetricsProviderWithSnapshots{
		snapshotsByKey: map[string]*fwdmetrics.ServiceSnapshot{
			"svc1.default.ctx1": {
				ServiceName: "svc1",
				Namespace:   "default",
				Context:     "ctx1",
				PortForwards: []fwdmetrics.PortForwardSnapshot{
					{
						PodName:   "pod1",
						LocalIP:   "127.1.0.1",
						LocalPort: "80",
						HTTPLogs: []fwdmetrics.HTTPLogEntry{
							{Timestamp: time.Now(), Method: "GET", Path: "/api/health", StatusCode: 200, Duration: 10 * time.Millisecond},
							{Timestamp: time.Now(), Method: "POST", Path: "/api/data", StatusCode: 201, Duration: 50 * time.Millisecond},
						},
					},
				},
			},
		},
	}
	server.SetMetricsProvider(metricsProvider)

	result, err := server.handleHTTPTrafficResource(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if result.Contents[0].MIMEType != "application/json" {
		t.Errorf("Expected application/json MIME type, got %s", result.Contents[0].MIMEType)
	}

	// Test with no HTTP traffic (nil snapshot)
	metricsProvider.snapshotsByKey = nil
	result, err = server.handleHTTPTrafficResource(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
}

func TestHandleContextsResource(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	req := &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{
			URI: "kubefwd://contexts",
		},
	}

	// Test with no K8s discovery - should fail
	_, err := server.handleContextsResource(context.Background(), req)
	if err == nil {
		t.Error("Expected error when K8s discovery is nil")
	}

	// Set up K8s discovery mock with error
	mockK8s := &mockK8sDiscoveryResource{
		contextsErr: errors.New("failed to get contexts"),
	}
	server.SetKubernetesDiscovery(mockK8s)

	_, err = server.handleContextsResource(context.Background(), req)
	if err == nil {
		t.Error("Expected error when ListContexts fails")
	}

	// Set up K8s discovery mock with success
	mockK8s = &mockK8sDiscoveryResource{
		contextsResp: &types.K8sContextsResponse{
			CurrentContext: "minikube",
			Contexts: []types.K8sContext{
				{Name: "minikube", Cluster: "minikube", Active: true},
				{Name: "docker-desktop", Cluster: "docker-desktop", Active: false},
				{Name: "kind-kind", Cluster: "kind-kind", Active: false},
			},
		},
	}
	server.SetKubernetesDiscovery(mockK8s)

	result, err := server.handleContextsResource(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if result.Contents[0].MIMEType != "application/json" {
		t.Errorf("Expected application/json MIME type, got %s", result.Contents[0].MIMEType)
	}
	if result.Contents[0].URI != req.Params.URI {
		t.Errorf("Expected URI %s, got %s", req.Params.URI, result.Contents[0].URI)
	}
}
