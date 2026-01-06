package components

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/txn2/kubefwd/pkg/fwdapi/types"
)

// =============================================================================
// Mock Implementations for Testing
// =============================================================================

type mockDiscovery struct {
	contexts       []types.K8sContext
	currentContext string
	namespaces     []types.K8sNamespace
	services       []types.K8sService
	listContextErr error
	listNsErr      error
	listSvcErr     error
}

func (m *mockDiscovery) ListContexts() (*types.K8sContextsResponse, error) {
	if m.listContextErr != nil {
		return nil, m.listContextErr
	}
	return &types.K8sContextsResponse{
		Contexts:       m.contexts,
		CurrentContext: m.currentContext,
	}, nil
}

func (m *mockDiscovery) ListNamespaces(_ string) ([]types.K8sNamespace, error) {
	if m.listNsErr != nil {
		return nil, m.listNsErr
	}
	return m.namespaces, nil
}

func (m *mockDiscovery) ListServices(_, _ string) ([]types.K8sService, error) {
	if m.listSvcErr != nil {
		return nil, m.listSvcErr
	}
	return m.services, nil
}

// Implement remaining interface methods with no-ops
func (m *mockDiscovery) GetService(_, _, _ string) (*types.K8sService, error) {
	return nil, nil
}
func (m *mockDiscovery) ListPods(_, _ string, _ types.ListPodsOptions) ([]types.K8sPod, error) {
	return nil, nil
}
func (m *mockDiscovery) GetPod(_, _, _ string) (*types.K8sPodDetail, error) {
	return nil, nil
}
func (m *mockDiscovery) GetPodLogs(_, _, _ string, _ types.PodLogsOptions) (*types.PodLogsResponse, error) {
	return nil, nil
}
func (m *mockDiscovery) GetEvents(_, _ string, _ types.GetEventsOptions) ([]types.K8sEvent, error) {
	return nil, nil
}
func (m *mockDiscovery) GetEndpoints(_, _, _ string) (*types.K8sEndpoints, error) {
	return nil, nil
}

type mockNamespaceController struct {
	addErr    error
	removeErr error
	addedNs   []string
}

func (m *mockNamespaceController) AddNamespace(ctx, namespace string, _ types.AddNamespaceOpts) (*types.NamespaceInfoResponse, error) {
	if m.addErr != nil {
		return nil, m.addErr
	}
	m.addedNs = append(m.addedNs, namespace)
	return &types.NamespaceInfoResponse{
		Namespace:    namespace,
		Context:      ctx,
		ServiceCount: 5,
	}, nil
}

func (m *mockNamespaceController) RemoveNamespace(_, _ string) error {
	return m.removeErr
}

func (m *mockNamespaceController) ListNamespaces() []types.NamespaceInfoResponse {
	return nil
}

func (m *mockNamespaceController) GetNamespace(_, _ string) (*types.NamespaceInfoResponse, error) {
	return nil, nil
}

type mockServiceCRUD struct {
	addErr       error
	removeErr    error
	addedSvcs    []string
	reconnectErr error
	syncErr      error
}

// ServiceController interface methods
func (m *mockServiceCRUD) Reconnect(_ string) error {
	return m.reconnectErr
}

func (m *mockServiceCRUD) ReconnectAll() int {
	return 0
}

func (m *mockServiceCRUD) Sync(_ string, _ bool) error {
	return m.syncErr
}

// ServiceCRUD interface methods
func (m *mockServiceCRUD) AddService(req types.AddServiceRequest) (*types.AddServiceResponse, error) {
	if m.addErr != nil {
		return nil, m.addErr
	}
	m.addedSvcs = append(m.addedSvcs, req.ServiceName)
	return &types.AddServiceResponse{
		Key:         req.ServiceName + "." + req.Namespace + "." + req.Context,
		ServiceName: req.ServiceName,
		Namespace:   req.Namespace,
		LocalIP:     "127.1.0.1",
		Hostnames:   []string{req.ServiceName, req.ServiceName + "." + req.Namespace},
	}, nil
}

func (m *mockServiceCRUD) RemoveService(_ string) error {
	return m.removeErr
}

// =============================================================================
// Helper Functions
// =============================================================================

func createTestBrowseModel() BrowseModel {
	m := NewBrowseModel()
	m.SetSize(100, 40)
	return m
}

func createBrowseModelWithData() BrowseModel {
	m := createTestBrowseModel()
	m.contexts = []types.K8sContext{
		{Name: "ctx1", Active: true},
		{Name: "ctx2", Active: false},
		{Name: "ctx3", Active: false},
	}
	m.currentContext = "ctx1"
	m.namespaces = []types.K8sNamespace{
		{Name: "default", Forwarded: false},
		{Name: "kube-system", Forwarded: true},
		{Name: "production", Forwarded: false},
	}
	m.services = []types.K8sService{
		{Name: "api", Type: "ClusterIP", Namespace: "default", Ports: []types.K8sServicePort{{Port: 80}}, Forwarded: false},
		{Name: "db", Type: "ClusterIP", Namespace: "default", Ports: []types.K8sServicePort{{Port: 5432}}, Forwarded: true},
		{Name: "cache", Type: "ClusterIP", Namespace: "default", Ports: []types.K8sServicePort{{Port: 6379}}, Forwarded: false},
	}
	return m
}

func keyMsg(key string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

// =============================================================================
// Constructor Tests
// =============================================================================

func TestBrowseModel_NewBrowseModel(t *testing.T) {
	m := NewBrowseModel()

	if m.visible {
		t.Error("Expected model to be hidden initially")
	}
	if m.currentView != NamespacesView {
		t.Errorf("Expected initial view to be NamespacesView, got %v", m.currentView)
	}
	if m.loading {
		t.Error("Expected loading to be false initially")
	}
	if m.selectedIndex != 0 {
		t.Error("Expected selectedIndex to be 0 initially")
	}
}

func TestBrowseModel_Init(t *testing.T) {
	m := NewBrowseModel()
	cmd := m.Init()

	if cmd != nil {
		t.Error("Expected Init to return nil cmd")
	}
}

// =============================================================================
// Visibility Tests
// =============================================================================

func TestBrowseModel_ShowHide(t *testing.T) {
	m := createTestBrowseModel()
	m.SetDiscovery(&mockDiscovery{
		contexts:       []types.K8sContext{{Name: "ctx1", Active: true}},
		currentContext: "ctx1",
	})

	if m.IsVisible() {
		t.Error("Expected model to be hidden initially")
	}

	// Show returns a command to load contexts
	cmd := m.Show()
	if !m.IsVisible() {
		t.Error("Expected model to be visible after Show()")
	}
	if !m.loading {
		t.Error("Expected loading to be true after Show()")
	}
	if cmd == nil {
		t.Error("Expected Show to return a command")
	}

	m.Hide()
	if m.IsVisible() {
		t.Error("Expected model to be hidden after Hide()")
	}
	if m.loading {
		t.Error("Expected loading to be false after Hide()")
	}
}

// =============================================================================
// Size Tests
// =============================================================================

func TestBrowseModel_SetSize(t *testing.T) {
	m := NewBrowseModel()
	m.SetSize(80, 24)

	if m.width != 80 {
		t.Errorf("Expected width 80, got %d", m.width)
	}
	if m.height != 24 {
		t.Errorf("Expected height 24, got %d", m.height)
	}
}

func TestBrowseModel_GetVisibleItemCount(t *testing.T) {
	tests := []struct {
		name           string
		height         int
		expectedMinVal int
	}{
		{"Normal height", 40, 3},
		{"Small height", 15, 3},
		{"Very small height", 10, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewBrowseModel()
			m.SetSize(80, tt.height)
			count := m.getVisibleItemCount()
			if count < tt.expectedMinVal {
				t.Errorf("Expected at least %d visible items for height %d, got %d", tt.expectedMinVal, tt.height, count)
			}
		})
	}
}

// =============================================================================
// Dependency Injection Tests
// =============================================================================

func TestBrowseModel_SetDiscovery(t *testing.T) {
	m := NewBrowseModel()
	discovery := &mockDiscovery{}
	m.SetDiscovery(discovery)

	if m.discovery == nil {
		t.Error("Expected discovery to be set")
	}
}

func TestBrowseModel_SetNamespaceController(t *testing.T) {
	m := NewBrowseModel()
	nc := &mockNamespaceController{}
	m.SetNamespaceController(nc)

	if m.namespaceController == nil {
		t.Error("Expected namespaceController to be set")
	}
}

func TestBrowseModel_SetServiceCRUD(t *testing.T) {
	m := NewBrowseModel()
	crud := &mockServiceCRUD{}
	m.SetServiceCRUD(crud)

	if m.serviceCRUD == nil {
		t.Error("Expected serviceCRUD to be set")
	}
}

// =============================================================================
// Navigation Tests - Namespaces View
// =============================================================================

func TestBrowseModel_NavigationNamespacesView_JK(t *testing.T) {
	m := createBrowseModelWithData()
	m.visible = true
	m.currentView = NamespacesView

	// Initial index is 0
	if m.selectedIndex != 0 {
		t.Errorf("Expected initial index 0, got %d", m.selectedIndex)
	}

	// Move down with 'j'
	m, _ = m.Update(keyMsg("j"))
	if m.selectedIndex != 1 {
		t.Errorf("Expected index 1 after 'j', got %d", m.selectedIndex)
	}

	// Move down again
	m, _ = m.Update(keyMsg("j"))
	if m.selectedIndex != 2 {
		t.Errorf("Expected index 2 after second 'j', got %d", m.selectedIndex)
	}

	// Can't go past end
	m, _ = m.Update(keyMsg("j"))
	if m.selectedIndex != 2 {
		t.Errorf("Expected index to stay at 2, got %d", m.selectedIndex)
	}

	// Move up with 'k'
	m, _ = m.Update(keyMsg("k"))
	if m.selectedIndex != 1 {
		t.Errorf("Expected index 1 after 'k', got %d", m.selectedIndex)
	}
}

func TestBrowseModel_NavigationNamespacesView_Arrows(t *testing.T) {
	m := createBrowseModelWithData()
	m.visible = true
	m.currentView = NamespacesView

	// Move down with arrow
	m, _ = m.Update(keyMsg("down"))
	if m.selectedIndex != 1 {
		t.Errorf("Expected index 1 after 'down', got %d", m.selectedIndex)
	}

	// Move up with arrow
	m, _ = m.Update(keyMsg("up"))
	if m.selectedIndex != 0 {
		t.Errorf("Expected index 0 after 'up', got %d", m.selectedIndex)
	}
}

func TestBrowseModel_NavigationNamespacesView_GShift(t *testing.T) {
	m := createBrowseModelWithData()
	m.visible = true
	m.currentView = NamespacesView
	m.selectedIndex = 1

	// Jump to top with 'g'
	m, _ = m.Update(keyMsg("g"))
	if m.selectedIndex != 0 {
		t.Errorf("Expected index 0 after 'g', got %d", m.selectedIndex)
	}

	// Jump to bottom with 'G'
	m, _ = m.Update(keyMsg("G"))
	if m.selectedIndex != 2 {
		t.Errorf("Expected index 2 (last) after 'G', got %d", m.selectedIndex)
	}
}

func TestBrowseModel_NavigationNamespacesView_Enter(t *testing.T) {
	m := createBrowseModelWithData()
	m.visible = true
	m.currentView = NamespacesView
	m.SetDiscovery(&mockDiscovery{})

	// Select first namespace
	m, cmd := m.Update(keyMsg("enter"))
	if m.currentView != ServicesView {
		t.Errorf("Expected ServicesView after enter, got %v", m.currentView)
	}
	if m.selectedNamespace != "default" {
		t.Errorf("Expected selectedNamespace 'default', got %s", m.selectedNamespace)
	}
	if !m.loading {
		t.Error("Expected loading to be true after enter")
	}
	if cmd == nil {
		t.Error("Expected a command to load services")
	}
}

func TestBrowseModel_NavigationNamespacesView_Close(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{"esc key", "esc"},
		{"q key", "q"},
		{"f key", "f"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := createBrowseModelWithData()
			m.visible = true
			m.currentView = NamespacesView

			m, _ = m.Update(keyMsg(tt.key))
			if m.visible {
				t.Errorf("Expected modal to be hidden after %s", tt.key)
			}
		})
	}
}

func TestBrowseModel_NavigationNamespacesView_SwitchContext(t *testing.T) {
	m := createBrowseModelWithData()
	m.visible = true
	m.currentView = NamespacesView

	// Press 'x' to switch to contexts view
	m, _ = m.Update(keyMsg("x"))
	if m.currentView != ContextsView {
		t.Errorf("Expected ContextsView after 'x', got %v", m.currentView)
	}
}

// =============================================================================
// Navigation Tests - Contexts View
// =============================================================================

func TestBrowseModel_NavigationContextsView_JK(t *testing.T) {
	m := createBrowseModelWithData()
	m.visible = true
	m.currentView = ContextsView
	m.selectedIndex = 0

	m, _ = m.Update(keyMsg("j"))
	if m.selectedIndex != 1 {
		t.Errorf("Expected index 1 after 'j', got %d", m.selectedIndex)
	}

	m, _ = m.Update(keyMsg("k"))
	if m.selectedIndex != 0 {
		t.Errorf("Expected index 0 after 'k', got %d", m.selectedIndex)
	}
}

func TestBrowseModel_NavigationContextsView_Back(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{"esc key", "esc"},
		{"left arrow", "left"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := createBrowseModelWithData()
			m.visible = true
			m.currentView = ContextsView

			m, _ = m.Update(keyMsg(tt.key))
			if m.currentView != NamespacesView {
				t.Errorf("Expected NamespacesView after %s, got %v", tt.key, m.currentView)
			}
		})
	}
}

func TestBrowseModel_NavigationContextsView_Enter(t *testing.T) {
	m := createBrowseModelWithData()
	m.visible = true
	m.currentView = ContextsView
	m.selectedIndex = 1 // Select ctx2
	m.SetDiscovery(&mockDiscovery{})

	m, cmd := m.Update(keyMsg("enter"))
	if m.currentContext != "ctx2" {
		t.Errorf("Expected currentContext 'ctx2', got %s", m.currentContext)
	}
	if m.currentView != NamespacesView {
		t.Error("Expected to return to NamespacesView")
	}
	if !m.loading {
		t.Error("Expected loading to be true after selecting context")
	}
	if cmd == nil {
		t.Error("Expected a command to load namespaces")
	}
}

func TestBrowseModel_NavigationContextsView_Close(t *testing.T) {
	m := createBrowseModelWithData()
	m.visible = true
	m.currentView = ContextsView

	m, _ = m.Update(keyMsg("q"))
	if m.visible {
		t.Error("Expected modal to be hidden after 'q'")
	}
}

// =============================================================================
// Navigation Tests - Services View
// =============================================================================

func TestBrowseModel_NavigationServicesView_JK(t *testing.T) {
	m := createBrowseModelWithData()
	m.visible = true
	m.currentView = ServicesView
	m.selectedIndex = 0 // "Forward All" is at 0

	// Move down
	m, _ = m.Update(keyMsg("j"))
	if m.selectedIndex != 1 {
		t.Errorf("Expected index 1 after 'j', got %d", m.selectedIndex)
	}

	// Move to last service
	m, _ = m.Update(keyMsg("j"))
	m, _ = m.Update(keyMsg("j"))
	if m.selectedIndex != 3 { // 0=ForwardAll, 1=api, 2=db, 3=cache
		t.Errorf("Expected index 3, got %d", m.selectedIndex)
	}

	// Can't go past end
	m, _ = m.Update(keyMsg("j"))
	if m.selectedIndex != 3 {
		t.Error("Expected index to stay at 3")
	}

	// Move up
	m, _ = m.Update(keyMsg("k"))
	if m.selectedIndex != 2 {
		t.Errorf("Expected index 2 after 'k', got %d", m.selectedIndex)
	}
}

func TestBrowseModel_NavigationServicesView_GShift(t *testing.T) {
	m := createBrowseModelWithData()
	m.visible = true
	m.currentView = ServicesView
	m.selectedIndex = 2

	// Jump to top with 'g'
	m, _ = m.Update(keyMsg("g"))
	if m.selectedIndex != 0 {
		t.Errorf("Expected index 0 after 'g', got %d", m.selectedIndex)
	}

	// Jump to bottom with 'G'
	m, _ = m.Update(keyMsg("G"))
	if m.selectedIndex != 3 { // ForwardAll + 3 services
		t.Errorf("Expected index 3 (last) after 'G', got %d", m.selectedIndex)
	}
}

func TestBrowseModel_NavigationServicesView_Back(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{"esc key", "esc"},
		{"left arrow", "left"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := createBrowseModelWithData()
			m.visible = true
			m.currentView = ServicesView

			m, _ = m.Update(keyMsg(tt.key))
			if m.currentView != NamespacesView {
				t.Errorf("Expected NamespacesView after %s, got %v", tt.key, m.currentView)
			}
		})
	}
}

func TestBrowseModel_NavigationServicesView_ForwardAll(t *testing.T) {
	m := createBrowseModelWithData()
	m.visible = true
	m.currentView = ServicesView
	m.selectedIndex = 0 // "Forward All"
	m.selectedNamespace = "default"
	nc := &mockNamespaceController{}
	m.SetNamespaceController(nc)

	m, cmd := m.Update(keyMsg("enter"))
	if !m.loading {
		t.Error("Expected loading to be true after forwarding namespace")
	}
	if cmd == nil {
		t.Error("Expected a command to forward namespace")
	}
}

func TestBrowseModel_NavigationServicesView_ForwardService(t *testing.T) {
	m := createBrowseModelWithData()
	m.visible = true
	m.currentView = ServicesView
	m.selectedIndex = 1 // First service (api)
	m.currentContext = "ctx1"
	crud := &mockServiceCRUD{}
	m.SetServiceCRUD(crud)

	m, cmd := m.Update(keyMsg("enter"))
	if !m.loading {
		t.Error("Expected loading to be true after forwarding service")
	}
	if cmd == nil {
		t.Error("Expected a command to forward service")
	}
}

func TestBrowseModel_NavigationServicesView_AlreadyForwarded(t *testing.T) {
	m := createBrowseModelWithData()
	m.visible = true
	m.currentView = ServicesView
	m.selectedIndex = 2 // db service which is forwarded

	m, _ = m.Update(keyMsg("enter"))
	if m.errorMsg == "" {
		t.Error("Expected error message for already forwarded service")
	}
	if !strings.Contains(m.errorMsg, "already forwarded") {
		t.Errorf("Expected 'already forwarded' in error, got: %s", m.errorMsg)
	}
}

// =============================================================================
// Message Handling Tests
// =============================================================================

func TestBrowseModel_ContextsLoadedMsg_Success(t *testing.T) {
	m := createTestBrowseModel()
	m.visible = true
	m.loading = true

	msg := BrowseContextsLoadedMsg{
		Contexts: []types.K8sContext{
			{Name: "ctx1", Active: true},
			{Name: "ctx2", Active: false},
		},
		CurrentContext: "ctx1",
	}

	m.SetDiscovery(&mockDiscovery{})
	m, cmd := m.Update(msg)

	// Should immediately trigger loading namespaces
	if !m.loading {
		t.Error("Expected loading to be true (loading namespaces)")
	}
	if m.currentContext != "ctx1" {
		t.Errorf("Expected currentContext 'ctx1', got %s", m.currentContext)
	}
	if len(m.contexts) != 2 {
		t.Errorf("Expected 2 contexts, got %d", len(m.contexts))
	}
	if cmd == nil {
		t.Error("Expected a command to load namespaces")
	}
}

func TestBrowseModel_ContextsLoadedMsg_Error(t *testing.T) {
	m := createTestBrowseModel()
	m.visible = true
	m.loading = true

	m, _ = m.Update(BrowseContextsLoadedMsg{Error: errForTest("connection refused")})
	if m.loading {
		t.Error("Expected loading to be false after error")
	}
	if !strings.Contains(m.errorMsg, "connection refused") {
		t.Errorf("Expected error message, got: %s", m.errorMsg)
	}
}

func TestBrowseModel_NamespacesLoadedMsg_Success(t *testing.T) {
	m := createTestBrowseModel()
	m.visible = true
	m.loading = true
	m.selectedIndex = 5 // Will be reset

	msg := BrowseNamespacesLoadedMsg{
		Namespaces: []types.K8sNamespace{
			{Name: "default", Forwarded: false},
			{Name: "kube-system", Forwarded: true},
		},
	}

	m, _ = m.Update(msg)
	if m.loading {
		t.Error("Expected loading to be false")
	}
	if len(m.namespaces) != 2 {
		t.Errorf("Expected 2 namespaces, got %d", len(m.namespaces))
	}
	if m.selectedIndex != 0 {
		t.Error("Expected selectedIndex to be reset to 0")
	}
	if m.scrollOffset != 0 {
		t.Error("Expected scrollOffset to be reset to 0")
	}
}

func TestBrowseModel_NamespacesLoadedMsg_Error(t *testing.T) {
	m := createTestBrowseModel()
	m.visible = true
	m.loading = true

	m, _ = m.Update(BrowseNamespacesLoadedMsg{Error: errForTest("forbidden")})
	if m.loading {
		t.Error("Expected loading to be false after error")
	}
	if !strings.Contains(m.errorMsg, "forbidden") {
		t.Errorf("Expected error message, got: %s", m.errorMsg)
	}
}

func TestBrowseModel_ServicesLoadedMsg_Success(t *testing.T) {
	m := createTestBrowseModel()
	m.visible = true
	m.loading = true
	m.currentView = ServicesView
	m.selectedIndex = 3

	msg := BrowseServicesLoadedMsg{
		Services: []types.K8sService{
			{Name: "api", Type: "ClusterIP"},
			{Name: "db", Type: "ClusterIP"},
		},
	}

	m, _ = m.Update(msg)
	if m.loading {
		t.Error("Expected loading to be false")
	}
	if len(m.services) != 2 {
		t.Errorf("Expected 2 services, got %d", len(m.services))
	}
	if m.selectedIndex != 0 {
		t.Error("Expected selectedIndex to be reset to 0")
	}
}

func TestBrowseModel_ServiceForwardedMsg_Success(t *testing.T) {
	m := createTestBrowseModel()
	m.visible = true
	m.loading = true
	m.currentView = ServicesView
	m.selectedNamespace = "default"
	m.SetDiscovery(&mockDiscovery{})

	msg := ServiceForwardedMsg{
		Key:         "api.default.ctx1",
		ServiceName: "api",
		Namespace:   "default",
		LocalIP:     "127.1.0.1",
		Hostnames:   []string{"api", "api.default"},
	}

	m, cmd := m.Update(msg)
	// Should reload services
	if !m.loading {
		t.Error("Expected loading to be true (reloading services)")
	}
	if cmd == nil {
		t.Error("Expected a command to reload services")
	}
}

func TestBrowseModel_ServiceForwardedMsg_Error(t *testing.T) {
	m := createTestBrowseModel()
	m.visible = true
	m.loading = true

	m, _ = m.Update(ServiceForwardedMsg{Error: errForTest("no pods available")})
	if m.loading {
		t.Error("Expected loading to be false after error")
	}
	if !strings.Contains(m.errorMsg, "no pods available") {
		t.Errorf("Expected error message, got: %s", m.errorMsg)
	}
}

func TestBrowseModel_NamespaceForwardedMsg_Success(t *testing.T) {
	m := createTestBrowseModel()
	m.visible = true
	m.loading = true

	msg := NamespaceForwardedMsg{
		Namespace:    "default",
		Context:      "ctx1",
		ServiceCount: 5,
	}

	m, _ = m.Update(msg)
	// Should close modal after success
	if m.visible {
		t.Error("Expected modal to be hidden after forwarding namespace")
	}
}

func TestBrowseModel_NamespaceForwardedMsg_Error(t *testing.T) {
	m := createTestBrowseModel()
	m.visible = true
	m.loading = true

	m, _ = m.Update(NamespaceForwardedMsg{Error: errForTest("namespace not found")})
	if m.loading {
		t.Error("Expected loading to be false after error")
	}
	if !strings.Contains(m.errorMsg, "namespace not found") {
		t.Errorf("Expected error message, got: %s", m.errorMsg)
	}
}

// =============================================================================
// Loading State Tests
// =============================================================================

func TestBrowseModel_LoadingState_BlocksNavigation(t *testing.T) {
	m := createBrowseModelWithData()
	m.visible = true
	m.loading = true
	m.selectedIndex = 0

	// Navigation keys should be blocked
	m, _ = m.Update(keyMsg("j"))
	if m.selectedIndex != 0 {
		t.Error("Expected navigation to be blocked during loading")
	}

	m, _ = m.Update(keyMsg("enter"))
	// Still loading, no view change
	if m.currentView != NamespacesView {
		t.Error("Expected view to not change during loading")
	}
}

func TestBrowseModel_LoadingState_AllowsClose(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{"q key", "q"},
		{"esc key", "esc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := createBrowseModelWithData()
			m.visible = true
			m.loading = true

			m, _ = m.Update(keyMsg(tt.key))
			if m.visible {
				t.Errorf("Expected modal to close with %s during loading", tt.key)
			}
		})
	}
}

// =============================================================================
// Error State Tests
// =============================================================================

func TestBrowseModel_ErrorState_ClearsOnKeypress(t *testing.T) {
	m := createBrowseModelWithData()
	m.visible = true
	m.errorMsg = "Some error"

	// Any keypress should clear the error
	m, _ = m.Update(keyMsg("j"))
	if m.errorMsg != "" {
		t.Error("Expected error to be cleared on keypress")
	}
}

// =============================================================================
// Scroll Offset Tests
// =============================================================================

func TestBrowseModel_AdjustScrollOffset_ScrollDown(t *testing.T) {
	m := NewBrowseModel()
	m.SetSize(80, 20) // Small height for testing
	m.namespaces = make([]types.K8sNamespace, 20)
	for i := range m.namespaces {
		m.namespaces[i] = types.K8sNamespace{Name: "ns" + string(rune('a'+i))}
	}

	visibleItems := m.getVisibleItemCount()

	// Move to just beyond visible area
	m.selectedIndex = visibleItems
	m.adjustScrollOffset()

	if m.scrollOffset == 0 {
		t.Error("Expected scrollOffset to increase when selection is beyond visible area")
	}
}

func TestBrowseModel_AdjustScrollOffset_ScrollUp(t *testing.T) {
	m := NewBrowseModel()
	m.SetSize(80, 20)
	m.scrollOffset = 5
	m.selectedIndex = 3

	m.adjustScrollOffset()
	if m.scrollOffset != 3 {
		t.Errorf("Expected scrollOffset to decrease to 3, got %d", m.scrollOffset)
	}
}

func TestBrowseModel_AdjustScrollOffset_ZeroVisibleItems(_ *testing.T) {
	m := NewBrowseModel()
	m.SetSize(80, 5) // Very small, will result in minimum visible items

	// Should not panic
	m.selectedIndex = 0
	m.adjustScrollOffset()
}

// =============================================================================
// FindCurrentContextIndex Tests
// =============================================================================

func TestBrowseModel_FindCurrentContextIndex_Found(t *testing.T) {
	m := createBrowseModelWithData()
	m.currentContext = "ctx2"

	index := m.findCurrentContextIndex()
	if index != 1 {
		t.Errorf("Expected index 1 for ctx2, got %d", index)
	}
}

func TestBrowseModel_FindCurrentContextIndex_NotFound(t *testing.T) {
	m := createBrowseModelWithData()
	m.currentContext = "nonexistent"

	index := m.findCurrentContextIndex()
	if index != 0 {
		t.Errorf("Expected index 0 for not found, got %d", index)
	}
}

func TestBrowseModel_FindCurrentContextIndex_EmptyContexts(t *testing.T) {
	m := NewBrowseModel()
	m.currentContext = "ctx1"

	index := m.findCurrentContextIndex()
	if index != 0 {
		t.Errorf("Expected index 0 for empty contexts, got %d", index)
	}
}

// =============================================================================
// View Rendering Tests
// =============================================================================

func TestBrowseModel_View_Hidden(t *testing.T) {
	m := NewBrowseModel()
	m.visible = false

	view := m.View()
	if view != "" {
		t.Error("Expected empty string when modal is hidden")
	}
}

func TestBrowseModel_View_Loading(t *testing.T) {
	m := createTestBrowseModel()
	m.visible = true
	m.loading = true
	m.currentContext = "ctx1"

	view := m.View()
	if !strings.Contains(view, "Loading") {
		t.Error("Expected 'Loading' in view during loading state")
	}
}

func TestBrowseModel_View_NamespacesView(t *testing.T) {
	m := createBrowseModelWithData()
	m.visible = true
	m.currentView = NamespacesView

	view := m.View()
	if !strings.Contains(view, "Browse Namespaces") {
		t.Error("Expected 'Browse Namespaces' title")
	}
	if !strings.Contains(view, "default") {
		t.Error("Expected 'default' namespace in view")
	}
}

func TestBrowseModel_View_ContextsView(t *testing.T) {
	m := createBrowseModelWithData()
	m.visible = true
	m.currentView = ContextsView

	view := m.View()
	if !strings.Contains(view, "Select Context") {
		t.Error("Expected 'Select Context' title")
	}
	if !strings.Contains(view, "ctx1") {
		t.Error("Expected 'ctx1' context in view")
	}
}

func TestBrowseModel_View_ServicesView(t *testing.T) {
	m := createBrowseModelWithData()
	m.visible = true
	m.currentView = ServicesView
	m.selectedNamespace = "default"

	view := m.View()
	if !strings.Contains(view, "Services in default") {
		t.Error("Expected 'Services in default' title")
	}
	if !strings.Contains(view, "Forward All") {
		t.Error("Expected 'Forward All' option in view")
	}
	if !strings.Contains(view, "api") {
		t.Error("Expected 'api' service in view")
	}
}

func TestBrowseModel_View_EmptyNamespaces(t *testing.T) {
	m := createTestBrowseModel()
	m.visible = true
	m.currentView = NamespacesView
	m.namespaces = nil

	view := m.View()
	if !strings.Contains(view, "No namespaces found") {
		t.Error("Expected 'No namespaces found' message")
	}
}

func TestBrowseModel_View_EmptyServices(t *testing.T) {
	m := createTestBrowseModel()
	m.visible = true
	m.currentView = ServicesView
	m.selectedNamespace = "default"
	m.services = nil

	view := m.View()
	if !strings.Contains(view, "No services found") {
		t.Error("Expected 'No services found' message")
	}
}

func TestBrowseModel_View_ErrorState(t *testing.T) {
	m := createBrowseModelWithData()
	m.visible = true
	m.errorMsg = "Something went wrong"

	view := m.View()
	if !strings.Contains(view, "Something went wrong") {
		t.Error("Expected error message in view")
	}
}

func TestBrowseModel_View_ForwardedStatus(t *testing.T) {
	m := createBrowseModelWithData()
	m.visible = true
	m.currentView = NamespacesView

	view := m.View()
	if !strings.Contains(view, "forwarded") {
		t.Error("Expected 'forwarded' indicator for forwarded namespace")
	}
}

// =============================================================================
// Port Formatting Tests
// =============================================================================

func TestBrowseModel_FormatPorts_Empty(t *testing.T) {
	m := NewBrowseModel()
	result := m.formatPorts(nil)
	if result != "" {
		t.Errorf("Expected empty string for nil ports, got %s", result)
	}
}

func TestBrowseModel_FormatPorts_Single(t *testing.T) {
	m := NewBrowseModel()
	ports := []types.K8sServicePort{{Port: 80}}
	result := m.formatPorts(ports)
	if result != "[80]" {
		t.Errorf("Expected '[80]', got %s", result)
	}
}

func TestBrowseModel_FormatPorts_Multiple(t *testing.T) {
	m := NewBrowseModel()
	ports := []types.K8sServicePort{{Port: 80}, {Port: 443}, {Port: 8080}}
	result := m.formatPorts(ports)
	if result != "[80,443,8080]" {
		t.Errorf("Expected '[80,443,8080]', got %s", result)
	}
}

// =============================================================================
// GetCurrentContext Test
// =============================================================================

func TestBrowseModel_GetCurrentContext(t *testing.T) {
	m := NewBrowseModel()
	m.currentContext = "my-context"

	if m.GetCurrentContext() != "my-context" {
		t.Errorf("Expected 'my-context', got %s", m.GetCurrentContext())
	}
}

// =============================================================================
// Command Generation Tests
// =============================================================================

func TestBrowseModel_LoadContexts_NoDiscovery(t *testing.T) {
	m := NewBrowseModel()
	m.discovery = nil

	cmd := m.loadContexts()
	msg := cmd()
	ctxMsg, ok := msg.(BrowseContextsLoadedMsg)
	if !ok {
		t.Fatal("Expected BrowseContextsLoadedMsg")
	}
	if ctxMsg.Error == nil {
		t.Error("Expected error when discovery is nil")
	}
}

func TestBrowseModel_LoadNamespaces_NoDiscovery(t *testing.T) {
	m := NewBrowseModel()
	m.discovery = nil

	cmd := m.loadNamespaces()
	msg := cmd()
	nsMsg, ok := msg.(BrowseNamespacesLoadedMsg)
	if !ok {
		t.Fatal("Expected BrowseNamespacesLoadedMsg")
	}
	if nsMsg.Error == nil {
		t.Error("Expected error when discovery is nil")
	}
}

func TestBrowseModel_LoadServices_NoDiscovery(t *testing.T) {
	m := NewBrowseModel()
	m.discovery = nil

	cmd := m.loadServices("default")
	msg := cmd()
	svcMsg, ok := msg.(BrowseServicesLoadedMsg)
	if !ok {
		t.Fatal("Expected BrowseServicesLoadedMsg")
	}
	if svcMsg.Error == nil {
		t.Error("Expected error when discovery is nil")
	}
}

func TestBrowseModel_ForwardService_NoCRUD(t *testing.T) {
	m := NewBrowseModel()
	m.serviceCRUD = nil

	svc := types.K8sService{Name: "api", Namespace: "default"}
	cmd := m.forwardService(svc)
	msg := cmd()
	fwdMsg, ok := msg.(ServiceForwardedMsg)
	if !ok {
		t.Fatal("Expected ServiceForwardedMsg")
	}
	if fwdMsg.Error == nil {
		t.Error("Expected error when serviceCRUD is nil")
	}
}

func TestBrowseModel_ForwardNamespace_NoController(t *testing.T) {
	m := NewBrowseModel()
	m.namespaceController = nil

	cmd := m.forwardNamespace("default")
	msg := cmd()
	fwdMsg, ok := msg.(NamespaceForwardedMsg)
	if !ok {
		t.Fatal("Expected NamespaceForwardedMsg")
	}
	if fwdMsg.Error == nil {
		t.Error("Expected error when namespaceController is nil")
	}
}

// =============================================================================
// Integration-style Tests
// =============================================================================

func TestBrowseModel_FullWorkflow_NamespaceBrowsing(t *testing.T) {
	m := createTestBrowseModel()
	discovery := &mockDiscovery{
		contexts:       []types.K8sContext{{Name: "ctx1", Active: true}},
		currentContext: "ctx1",
		namespaces:     []types.K8sNamespace{{Name: "default"}, {Name: "production"}},
	}
	m.SetDiscovery(discovery)

	// Show modal
	cmd := m.Show()
	if cmd == nil {
		t.Fatal("Expected command from Show()")
	}

	// Execute the load contexts command
	msg := cmd()
	m, cmd = m.Update(msg)

	// Should have loaded contexts and be loading namespaces
	if m.currentContext != "ctx1" {
		t.Error("Expected current context to be set")
	}

	// Execute load namespaces command
	if cmd != nil {
		msg = cmd()
		m, _ = m.Update(msg)
	}

	// Should have namespaces loaded
	if len(m.namespaces) != 2 {
		t.Errorf("Expected 2 namespaces, got %d", len(m.namespaces))
	}
	if m.loading {
		t.Error("Expected loading to be false after namespaces loaded")
	}
}

// =============================================================================
// Helper for creating errors
// =============================================================================

type testError string

func (e testError) Error() string { return string(e) }

func errForTest(msg string) error {
	return testError(msg)
}

// =============================================================================
// Success Path Tests for loadServices, forwardService, forwardNamespace
// =============================================================================

func TestBrowseModel_LoadServices_Success(t *testing.T) {
	m := createTestBrowseModel()
	m.currentContext = "test-ctx"
	m.SetDiscovery(&mockDiscovery{
		services: []types.K8sService{
			{Name: "svc1", Namespace: "default", Type: "ClusterIP"},
			{Name: "svc2", Namespace: "default", Type: "LoadBalancer"},
		},
	})

	cmd := m.loadServices("default")
	if cmd == nil {
		t.Fatal("Expected non-nil command")
	}

	msg := cmd()
	loadedMsg, ok := msg.(BrowseServicesLoadedMsg)
	if !ok {
		t.Fatalf("Expected BrowseServicesLoadedMsg, got %T", msg)
	}

	if loadedMsg.Error != nil {
		t.Errorf("Expected no error, got %v", loadedMsg.Error)
	}
	if len(loadedMsg.Services) != 2 {
		t.Errorf("Expected 2 services, got %d", len(loadedMsg.Services))
	}
}

func TestBrowseModel_LoadServices_Error(t *testing.T) {
	m := createTestBrowseModel()
	m.currentContext = "test-ctx"
	m.SetDiscovery(&mockDiscovery{
		listSvcErr: errForTest("connection refused"),
	})

	cmd := m.loadServices("default")
	msg := cmd()
	loadedMsg := msg.(BrowseServicesLoadedMsg)

	if loadedMsg.Error == nil {
		t.Error("Expected error from discovery")
	}
}

func TestBrowseModel_ForwardService_Success(t *testing.T) {
	m := createTestBrowseModel()
	m.currentContext = "test-ctx"
	m.SetServiceCRUD(&mockServiceCRUD{})

	svc := types.K8sService{Name: "api", Namespace: "default", Type: "ClusterIP"}
	cmd := m.forwardService(svc)
	if cmd == nil {
		t.Fatal("Expected non-nil command")
	}

	msg := cmd()
	fwdMsg, ok := msg.(ServiceForwardedMsg)
	if !ok {
		t.Fatalf("Expected ServiceForwardedMsg, got %T", msg)
	}

	if fwdMsg.Error != nil {
		t.Errorf("Expected no error, got %v", fwdMsg.Error)
	}
	if fwdMsg.ServiceName != "api" {
		t.Errorf("Expected ServiceName 'api', got '%s'", fwdMsg.ServiceName)
	}
	if fwdMsg.LocalIP != "127.1.0.1" {
		t.Errorf("Expected LocalIP '127.1.0.1', got '%s'", fwdMsg.LocalIP)
	}
}

func TestBrowseModel_ForwardService_Error(t *testing.T) {
	m := createTestBrowseModel()
	m.currentContext = "test-ctx"
	m.SetServiceCRUD(&mockServiceCRUD{
		addErr: errForTest("service already forwarded"),
	})

	svc := types.K8sService{Name: "api", Namespace: "default", Type: "ClusterIP"}
	cmd := m.forwardService(svc)
	msg := cmd()
	fwdMsg := msg.(ServiceForwardedMsg)

	if fwdMsg.Error == nil {
		t.Error("Expected error from serviceCRUD")
	}
}

func TestBrowseModel_ForwardNamespace_Success(t *testing.T) {
	m := createTestBrowseModel()
	m.currentContext = "test-ctx"
	m.SetNamespaceController(&mockNamespaceController{})

	cmd := m.forwardNamespace("default")
	if cmd == nil {
		t.Fatal("Expected non-nil command")
	}

	msg := cmd()
	fwdMsg, ok := msg.(NamespaceForwardedMsg)
	if !ok {
		t.Fatalf("Expected NamespaceForwardedMsg, got %T", msg)
	}

	if fwdMsg.Error != nil {
		t.Errorf("Expected no error, got %v", fwdMsg.Error)
	}
	if fwdMsg.Namespace != "default" {
		t.Errorf("Expected Namespace 'default', got '%s'", fwdMsg.Namespace)
	}
	// Default mock returns ServiceCount 5
	if fwdMsg.ServiceCount != 5 {
		t.Errorf("Expected ServiceCount 5, got %d", fwdMsg.ServiceCount)
	}
}

func TestBrowseModel_ForwardNamespace_Error(t *testing.T) {
	m := createTestBrowseModel()
	m.currentContext = "test-ctx"
	m.SetNamespaceController(&mockNamespaceController{
		addErr: errForTest("namespace already watching"),
	})

	cmd := m.forwardNamespace("default")
	msg := cmd()
	fwdMsg := msg.(NamespaceForwardedMsg)

	if fwdMsg.Error == nil {
		t.Error("Expected error from namespaceController")
	}
}
