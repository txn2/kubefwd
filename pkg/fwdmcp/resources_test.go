package fwdmcp

import (
	"context"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/txn2/kubefwd/pkg/fwdapi/types"
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
	result, err = server.handleSummaryResource(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Test unhealthy state (errors > active)
	mock.summary.ErrorCount = 5
	mock.summary.ActiveServices = 2
	result, err = server.handleSummaryResource(context.Background(), req)
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

	result, err = server.handleSummaryResource(context.Background(), req)
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
