package fwdapi

import (
	"testing"
	"time"

	"github.com/txn2/kubefwd/pkg/fwdapi/types"
	"github.com/txn2/kubefwd/pkg/fwdmetrics"
	"github.com/txn2/kubefwd/pkg/fwdtui/events"
	"github.com/txn2/kubefwd/pkg/fwdtui/state"
)

// Reset global state between tests
func resetGlobalState() {
	mu.Lock()
	defer mu.Unlock()
	apiEnabled = false
	apiManager = nil
	initialized = false
}

func TestEnable(t *testing.T) {
	resetGlobalState()

	if IsEnabled() {
		t.Error("Expected API to be disabled initially")
	}

	Enable()

	if !IsEnabled() {
		t.Error("Expected API to be enabled after Enable()")
	}
}

func TestIsEnabled(t *testing.T) {
	resetGlobalState()

	if IsEnabled() {
		t.Error("Expected IsEnabled to return false initially")
	}

	Enable()

	if !IsEnabled() {
		t.Error("Expected IsEnabled to return true after Enable()")
	}
}

func TestGetManager(t *testing.T) {
	resetGlobalState()

	if GetManager() != nil {
		t.Error("Expected GetManager to return nil before Init")
	}

	shutdownChan := make(chan struct{})
	triggerShutdown := func() {}

	Init(shutdownChan, triggerShutdown, "1.0.0")

	if GetManager() == nil {
		t.Error("Expected GetManager to return non-nil after Init")
	}
}

func TestInit(t *testing.T) {
	resetGlobalState()

	shutdownChan := make(chan struct{})
	triggerShutdown := func() {}

	manager := Init(shutdownChan, triggerShutdown, "1.0.0")

	if manager == nil {
		t.Fatal("Expected Init to return non-nil Manager")
	}

	if manager.version != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", manager.version)
	}

	if manager.stopChan == nil {
		t.Error("Expected stopChan to be initialized")
	}

	if manager.doneChan == nil {
		t.Error("Expected doneChan to be initialized")
	}
}

func TestInit_OnlyOnce(t *testing.T) {
	resetGlobalState()

	shutdownChan := make(chan struct{})
	triggerShutdown := func() {}

	manager1 := Init(shutdownChan, triggerShutdown, "1.0.0")
	manager2 := Init(shutdownChan, triggerShutdown, "2.0.0")

	if manager1 != manager2 {
		t.Error("Expected Init to return the same manager on subsequent calls")
	}

	if manager2.version != "1.0.0" {
		t.Errorf("Expected version to remain '1.0.0', got '%s'", manager2.version)
	}
}

func TestManager_SetStateReader(t *testing.T) {
	resetGlobalState()

	manager := &Manager{}
	mock := &mockStateReader{}

	manager.SetStateReader(mock)

	if manager.stateReader != mock {
		t.Error("Expected stateReader to be set")
	}
}

func TestManager_SetMetricsProvider(t *testing.T) {
	resetGlobalState()

	manager := &Manager{}
	mock := &mockMetricsProvider{}

	manager.SetMetricsProvider(mock)

	if manager.metricsProvider != mock {
		t.Error("Expected metricsProvider to be set")
	}
}

func TestManager_SetServiceController(t *testing.T) {
	resetGlobalState()

	manager := &Manager{}
	mock := &mockServiceController{}

	manager.SetServiceController(mock)

	if manager.serviceController != mock {
		t.Error("Expected serviceController to be set")
	}
}

func TestManager_SetEventStreamer(t *testing.T) {
	resetGlobalState()

	manager := &Manager{}
	mock := &mockEventStreamer{}

	manager.SetEventStreamer(mock)

	if manager.eventStreamer != mock {
		t.Error("Expected eventStreamer to be set")
	}
}

func TestManager_SetNamespaces(t *testing.T) {
	manager := &Manager{}
	namespaces := []string{"default", "kube-system"}

	manager.SetNamespaces(namespaces)

	if len(manager.namespaces) != 2 {
		t.Errorf("Expected 2 namespaces, got %d", len(manager.namespaces))
	}
}

func TestManager_SetContexts(t *testing.T) {
	manager := &Manager{}
	contexts := []string{"minikube", "prod"}

	manager.SetContexts(contexts)

	if len(manager.contexts) != 2 {
		t.Errorf("Expected 2 contexts, got %d", len(manager.contexts))
	}
}

func TestManager_SetTUIEnabled(t *testing.T) {
	manager := &Manager{}

	manager.SetTUIEnabled(true)

	if !manager.tuiEnabled {
		t.Error("Expected tuiEnabled to be true")
	}
}

func TestManager_Stop(t *testing.T) {
	manager := &Manager{
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}

	manager.Stop()

	select {
	case <-manager.stopChan:
		// Expected
	default:
		t.Error("Expected stopChan to be closed")
	}

	// Stop should be idempotent
	manager.Stop()
}

func TestManager_Done(t *testing.T) {
	manager := &Manager{
		doneChan: make(chan struct{}),
	}

	doneChan := manager.Done()

	if doneChan == nil {
		t.Error("Expected Done() to return non-nil channel")
	}
}

func TestManager_Uptime(t *testing.T) {
	startTime := time.Now().Add(-time.Hour)
	manager := &Manager{
		startTime: startTime,
	}

	uptime := manager.Uptime()

	if uptime < time.Hour {
		t.Errorf("Expected uptime >= 1 hour, got %v", uptime)
	}
}

func TestManager_StartTime(t *testing.T) {
	now := time.Now()
	manager := &Manager{
		startTime: now,
	}

	if !manager.StartTime().Equal(now) {
		t.Error("Expected StartTime to match")
	}
}

func TestManager_Version(t *testing.T) {
	manager := &Manager{
		version: "1.2.3",
	}

	if manager.Version() != "1.2.3" {
		t.Errorf("Expected version '1.2.3', got '%s'", manager.Version())
	}
}

func TestManager_Namespaces(t *testing.T) {
	manager := &Manager{
		namespaces: []string{"ns1", "ns2"},
	}

	ns := manager.Namespaces()

	if len(ns) != 2 {
		t.Errorf("Expected 2 namespaces, got %d", len(ns))
	}
}

func TestManager_Contexts(t *testing.T) {
	manager := &Manager{
		contexts: []string{"ctx1"},
	}

	ctx := manager.Contexts()

	if len(ctx) != 1 {
		t.Errorf("Expected 1 context, got %d", len(ctx))
	}
}

func TestManager_TUIEnabled(t *testing.T) {
	manager := &Manager{
		tuiEnabled: true,
	}

	if !manager.TUIEnabled() {
		t.Error("Expected TUIEnabled to return true")
	}
}

func TestManager_RunWithoutStateReader(t *testing.T) {
	manager := &Manager{
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}

	err := manager.Run()

	if err == nil {
		t.Error("Expected error when stateReader is not configured")
	}

	if err.Error() != "state reader not configured" {
		t.Errorf("Unexpected error message: %s", err.Error())
	}
}

// Mock implementations for manager tests

type mockStateReader struct{}

func (m *mockStateReader) GetServices() []state.ServiceSnapshot         { return nil }
func (m *mockStateReader) GetService(key string) *state.ServiceSnapshot { return nil }
func (m *mockStateReader) GetSummary() state.SummaryStats               { return state.SummaryStats{} }
func (m *mockStateReader) GetFiltered() []state.ForwardSnapshot         { return nil }
func (m *mockStateReader) GetForward(key string) *state.ForwardSnapshot { return nil }
func (m *mockStateReader) GetLogs(count int) []state.LogEntry           { return nil }
func (m *mockStateReader) Count() int                                   { return 0 }
func (m *mockStateReader) ServiceCount() int                            { return 0 }

type mockMetricsProvider struct{}

func (m *mockMetricsProvider) GetAllSnapshots() []fwdmetrics.ServiceSnapshot             { return nil }
func (m *mockMetricsProvider) GetServiceSnapshot(key string) *fwdmetrics.ServiceSnapshot { return nil }
func (m *mockMetricsProvider) GetTotals() (uint64, uint64, float64, float64)             { return 0, 0, 0, 0 }
func (m *mockMetricsProvider) ServiceCount() int                                         { return 0 }
func (m *mockMetricsProvider) PortForwardCount() int                                     { return 0 }

type mockServiceController struct{}

func (m *mockServiceController) Reconnect(key string) error        { return nil }
func (m *mockServiceController) ReconnectAll() int                 { return 0 }
func (m *mockServiceController) Sync(key string, force bool) error { return nil }

type mockEventStreamer struct{}

func (m *mockEventStreamer) Subscribe() (<-chan events.Event, func()) {
	ch := make(chan events.Event)
	return ch, func() { close(ch) }
}
func (m *mockEventStreamer) SubscribeType(eventType events.EventType) (<-chan events.Event, func()) {
	ch := make(chan events.Event)
	return ch, func() { close(ch) }
}

// Additional mock implementations

type mockDiagnosticsProvider struct{}

func (m *mockDiagnosticsProvider) GetSummary() types.DiagnosticSummary {
	return types.DiagnosticSummary{}
}
func (m *mockDiagnosticsProvider) GetServiceDiagnostic(key string) (*types.ServiceDiagnostic, error) {
	return nil, nil
}
func (m *mockDiagnosticsProvider) GetForwardDiagnostic(key string) (*types.ForwardDiagnostic, error) {
	return nil, nil
}
func (m *mockDiagnosticsProvider) GetNetworkStatus() types.NetworkStatus {
	return types.NetworkStatus{}
}
func (m *mockDiagnosticsProvider) GetErrors(count int) []types.ErrorDetail { return nil }

type mockNamespaceController struct {
	namespaces []types.NamespaceInfoResponse
}

func (m *mockNamespaceController) AddNamespace(ctx, namespace string, opts types.AddNamespaceOpts) (*types.NamespaceInfoResponse, error) {
	info := types.NamespaceInfoResponse{
		Key:       namespace + "." + ctx,
		Namespace: namespace,
		Context:   ctx,
	}
	m.namespaces = append(m.namespaces, info)
	return &info, nil
}

func (m *mockNamespaceController) RemoveNamespace(ctx, namespace string) error {
	return nil
}

func (m *mockNamespaceController) ListNamespaces() []types.NamespaceInfoResponse {
	return m.namespaces
}

func (m *mockNamespaceController) GetNamespace(ctx, namespace string) (*types.NamespaceInfoResponse, error) {
	for _, ns := range m.namespaces {
		if ns.Context == ctx && ns.Namespace == namespace {
			return &ns, nil
		}
	}
	return nil, nil
}

type mockServiceCRUD struct {
	mockServiceController
}

func (m *mockServiceCRUD) AddService(req types.AddServiceRequest) (*types.AddServiceResponse, error) {
	return &types.AddServiceResponse{
		Key:         req.ServiceName + "." + req.Namespace + "." + req.Context,
		ServiceName: req.ServiceName,
		Namespace:   req.Namespace,
		Context:     req.Context,
	}, nil
}

func (m *mockServiceCRUD) RemoveService(key string) error {
	return nil
}

type mockKubernetesDiscovery struct{}

func (m *mockKubernetesDiscovery) ListNamespaces(ctx string) ([]types.K8sNamespace, error) {
	return []types.K8sNamespace{{Name: "default"}}, nil
}

func (m *mockKubernetesDiscovery) ListServices(ctx, namespace string) ([]types.K8sService, error) {
	return []types.K8sService{{Name: "test-svc", Namespace: namespace}}, nil
}

func (m *mockKubernetesDiscovery) ListContexts() (*types.K8sContextsResponse, error) {
	return &types.K8sContextsResponse{
		CurrentContext: "minikube",
		Contexts:       []types.K8sContext{{Name: "minikube", Cluster: "minikube"}},
	}, nil
}

func (m *mockKubernetesDiscovery) GetService(ctx, namespace, name string) (*types.K8sService, error) {
	return &types.K8sService{Name: name, Namespace: namespace}, nil
}

func (m *mockKubernetesDiscovery) GetPodLogs(ctx, namespace, podName string, opts types.PodLogsOptions) (*types.PodLogsResponse, error) {
	return &types.PodLogsResponse{
		PodName:       podName,
		Namespace:     namespace,
		Context:       ctx,
		ContainerName: opts.Container,
		Logs:          []string{"mock log line"},
		LineCount:     1,
		Truncated:     false,
	}, nil
}

func (m *mockKubernetesDiscovery) ListPods(ctx, namespace string, opts types.ListPodsOptions) ([]types.K8sPod, error) {
	return []types.K8sPod{{Name: "test-pod", Namespace: namespace, Phase: "Running"}}, nil
}

func (m *mockKubernetesDiscovery) GetPod(ctx, namespace, podName string) (*types.K8sPodDetail, error) {
	return &types.K8sPodDetail{Name: podName, Namespace: namespace, Phase: "Running"}, nil
}

func (m *mockKubernetesDiscovery) GetEvents(ctx, namespace string, opts types.GetEventsOptions) ([]types.K8sEvent, error) {
	return []types.K8sEvent{{Type: "Normal", Reason: "Scheduled"}}, nil
}

func (m *mockKubernetesDiscovery) GetEndpoints(ctx, namespace, serviceName string) (*types.K8sEndpoints, error) {
	return &types.K8sEndpoints{Name: serviceName, Namespace: namespace}, nil
}

// Tests for additional manager methods

func TestManager_SetDiagnosticsProvider(t *testing.T) {
	manager := &Manager{}
	mock := &mockDiagnosticsProvider{}

	manager.SetDiagnosticsProvider(mock)

	if manager.diagnosticsProvider != mock {
		t.Error("Expected diagnosticsProvider to be set")
	}
}

func TestManager_SetNamespaceController(t *testing.T) {
	manager := &Manager{}
	mock := &mockNamespaceController{}

	manager.SetNamespaceController(mock)

	if manager.namespaceController != mock {
		t.Error("Expected namespaceController to be set")
	}
}

func TestManager_SetServiceCRUD(t *testing.T) {
	manager := &Manager{}
	mock := &mockServiceCRUD{}

	manager.SetServiceCRUD(mock)

	if manager.serviceCRUD != mock {
		t.Error("Expected serviceCRUD to be set")
	}
}

func TestManager_SetKubernetesDiscovery(t *testing.T) {
	manager := &Manager{}
	mock := &mockKubernetesDiscovery{}

	manager.SetKubernetesDiscovery(mock)

	if manager.k8sDiscovery != mock {
		t.Error("Expected k8sDiscovery to be set")
	}
}

func TestManager_GetNamespaceManager(t *testing.T) {
	manager := &Manager{}

	// Initially nil
	if manager.GetNamespaceManager() != nil {
		t.Error("Expected nil namespace manager initially")
	}
}

// TestNamespaceControllerMethods tests the namespace controller interface methods
func TestNamespaceControllerMethods(t *testing.T) {
	mock := &mockNamespaceController{}

	// AddNamespace
	info, err := mock.AddNamespace("minikube", "default", types.AddNamespaceOpts{})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("Expected namespace info")
	}
	if info.Namespace != "default" {
		t.Errorf("Expected namespace 'default', got '%s'", info.Namespace)
	}

	// ListNamespaces
	namespaces := mock.ListNamespaces()
	if len(namespaces) != 1 {
		t.Errorf("Expected 1 namespace, got %d", len(namespaces))
	}

	// GetNamespace
	ns, err := mock.GetNamespace("minikube", "default")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if ns == nil {
		t.Fatal("Expected namespace info")
	}

	// RemoveNamespace
	err = mock.RemoveNamespace("minikube", "default")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

// TestServiceCRUDMethods tests the service CRUD interface methods
func TestServiceCRUDMethods(t *testing.T) {
	mock := &mockServiceCRUD{}

	// AddService
	resp, err := mock.AddService(types.AddServiceRequest{
		ServiceName: "test-svc",
		Namespace:   "default",
		Context:     "minikube",
	})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("Expected response")
	}
	if resp.ServiceName != "test-svc" {
		t.Errorf("Expected ServiceName 'test-svc', got '%s'", resp.ServiceName)
	}

	// RemoveService
	err = mock.RemoveService("test-svc.default.minikube")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

// TestKubernetesDiscoveryMethods tests the k8s discovery interface methods
func TestKubernetesDiscoveryMethods(t *testing.T) {
	mock := &mockKubernetesDiscovery{}

	// ListNamespaces
	namespaces, err := mock.ListNamespaces("minikube")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(namespaces) != 1 {
		t.Errorf("Expected 1 namespace, got %d", len(namespaces))
	}

	// ListServices
	services, err := mock.ListServices("minikube", "default")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(services) != 1 {
		t.Errorf("Expected 1 service, got %d", len(services))
	}

	// ListContexts
	ctxResp, err := mock.ListContexts()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if ctxResp.CurrentContext != "minikube" {
		t.Errorf("Expected current context 'minikube', got '%s'", ctxResp.CurrentContext)
	}

	// GetService
	svc, err := mock.GetService("minikube", "default", "test-svc")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if svc.Name != "test-svc" {
		t.Errorf("Expected service name 'test-svc', got '%s'", svc.Name)
	}
}

// Ensure mocks implement the interfaces
var (
	_ types.StateReader         = (*mockStateReader)(nil)
	_ types.MetricsProvider     = (*mockMetricsProvider)(nil)
	_ types.ServiceController   = (*mockServiceController)(nil)
	_ types.EventStreamer       = (*mockEventStreamer)(nil)
	_ types.DiagnosticsProvider = (*mockDiagnosticsProvider)(nil)
	_ types.NamespaceController = (*mockNamespaceController)(nil)
	_ types.ServiceCRUD         = (*mockServiceCRUD)(nil)
	_ types.KubernetesDiscovery = (*mockKubernetesDiscovery)(nil)
)
