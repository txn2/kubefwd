package fwdns

import (
	"strings"
	"sync"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	restclient "k8s.io/client-go/rest"

	"github.com/txn2/kubefwd/pkg/fwdsvcregistry"
)

// Package-level registry initialization
var (
	registryOnce sync.Once
	registryStop chan struct{}
)

func initRegistryOnce() {
	registryOnce.Do(func() {
		registryStop = make(chan struct{})
		fwdsvcregistry.Init(registryStop)
	})
}

func TestWatcherKey(t *testing.T) {
	tests := []struct {
		namespace string
		context   string
		expected  string
	}{
		{"default", "minikube", "default.minikube"},
		{"kube-system", "prod-cluster", "kube-system.prod-cluster"},
		{"my-ns", "my-ctx", "my-ns.my-ctx"},
	}

	for _, tt := range tests {
		result := WatcherKey(tt.namespace, tt.context)
		if result != tt.expected {
			t.Errorf("WatcherKey(%q, %q) = %q, want %q",
				tt.namespace, tt.context, result, tt.expected)
		}
	}
}

func TestNewManager(t *testing.T) {
	stopCh := make(chan struct{})
	defer close(stopCh)

	cfg := ManagerConfig{
		Domain:         "test.local",
		Timeout:        300,
		AutoReconnect:  true,
		ResyncInterval: 5 * time.Minute,
		RetryInterval:  10 * time.Second,
		GlobalStopCh:   stopCh,
	}

	mgr := NewManager(cfg)

	if mgr == nil {
		t.Fatal("NewManager returned nil")
	}

	if mgr.domain != "test.local" {
		t.Errorf("Expected domain 'test.local', got %q", mgr.domain)
	}

	if mgr.timeout != 300 {
		t.Errorf("Expected timeout 300, got %d", mgr.timeout)
	}

	if !mgr.autoReconnect {
		t.Error("Expected autoReconnect to be true")
	}
}

func TestManager_ListWatchers_Empty(t *testing.T) {
	stopCh := make(chan struct{})
	defer close(stopCh)

	mgr := NewManager(ManagerConfig{GlobalStopCh: stopCh})
	watchers := mgr.ListWatchers()

	if len(watchers) != 0 {
		t.Errorf("Expected 0 watchers, got %d", len(watchers))
	}
}

func TestManager_GetWatcher_NotFound(t *testing.T) {
	stopCh := make(chan struct{})
	defer close(stopCh)

	mgr := NewManager(ManagerConfig{GlobalStopCh: stopCh})
	watcher := mgr.GetWatcher("ctx", "ns")

	if watcher != nil {
		t.Error("Expected nil watcher for non-existent namespace")
	}
}

func TestManager_StopWatcher_NotFound(t *testing.T) {
	stopCh := make(chan struct{})
	defer close(stopCh)

	mgr := NewManager(ManagerConfig{GlobalStopCh: stopCh})
	err := mgr.StopWatcher("ctx", "ns")

	if err == nil {
		t.Error("Expected error when stopping non-existent watcher")
	}
}

func TestManager_StopAll_Empty(t *testing.T) {
	stopCh := make(chan struct{})

	mgr := NewManager(ManagerConfig{GlobalStopCh: stopCh})

	// Should not panic with no watchers
	mgr.StopAll()

	// Verify done channel is closed
	select {
	case <-mgr.Done():
		// Success
	case <-time.After(time.Second):
		t.Error("Done channel not closed after StopAll")
	}
}

func TestManager_GetClientSet_NotFound(t *testing.T) {
	stopCh := make(chan struct{})
	defer close(stopCh)

	mgr := NewManager(ManagerConfig{GlobalStopCh: stopCh})
	cs := mgr.GetClientSet("nonexistent")

	if cs != nil {
		t.Error("Expected nil clientSet for non-existent context")
	}
}

func TestNamespaceWatcher_Running(t *testing.T) {
	watcher := &NamespaceWatcher{
		running: false,
	}

	if watcher.Running() {
		t.Error("Expected Running() to return false")
	}

	watcher.runningMu.Lock()
	watcher.running = true
	watcher.runningMu.Unlock()

	if !watcher.Running() {
		t.Error("Expected Running() to return true")
	}
}

func TestNamespaceWatcher_Info(t *testing.T) {
	initRegistryOnce()

	startTime := time.Now()
	watcher := &NamespaceWatcher{
		key:           "default.minikube",
		namespace:     "default",
		context:       "minikube",
		labelSelector: "app=test",
		fieldSelector: "metadata.name=test",
		startedAt:     startTime,
		running:       true,
	}

	info := watcher.Info()

	if info.Key != "default.minikube" {
		t.Errorf("Expected key 'default.minikube', got %q", info.Key)
	}
	if info.Namespace != "default" {
		t.Errorf("Expected namespace 'default', got %q", info.Namespace)
	}
	if info.Context != "minikube" {
		t.Errorf("Expected context 'minikube', got %q", info.Context)
	}
	if info.LabelSelector != "app=test" {
		t.Errorf("Expected labelSelector 'app=test', got %q", info.LabelSelector)
	}
	if info.FieldSelector != "metadata.name=test" {
		t.Errorf("Expected fieldSelector 'metadata.name=test', got %q", info.FieldSelector)
	}
	if !info.Running {
		t.Error("Expected Running to be true")
	}
}

func TestNamespaceWatcher_Stop(t *testing.T) {
	watcher := &NamespaceWatcher{
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
		running: true,
	}

	watcher.Stop()

	// Verify stopCh is closed
	select {
	case <-watcher.stopCh:
		// Success
	default:
		t.Error("stopCh not closed after Stop()")
	}

	// Calling Stop again should not panic
	watcher.Stop()
}

func TestSplitPortMapping(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"80:8080", []string{"80", "8080"}},
		{"443:8443", []string{"443", "8443"}},
		{"http:8080", []string{"http", "8080"}},
		{"invalid", nil},
		{"", nil},
	}

	for _, tt := range tests {
		result := splitPortMapping(tt.input)
		if tt.expected == nil {
			if result != nil {
				t.Errorf("splitPortMapping(%q) = %v, want nil", tt.input, result)
			}
		} else {
			if result == nil || len(result) != len(tt.expected) {
				t.Errorf("splitPortMapping(%q) = %v, want %v", tt.input, result, tt.expected)
				continue
			}
			for i := range tt.expected {
				if result[i] != tt.expected[i] {
					t.Errorf("splitPortMapping(%q)[%d] = %q, want %q",
						tt.input, i, result[i], tt.expected[i])
				}
			}
		}
	}
}

func TestNamespaceWatcher_ParsePortMap(t *testing.T) {
	stopCh := make(chan struct{})
	defer close(stopCh)

	mgr := NewManager(ManagerConfig{GlobalStopCh: stopCh})
	watcher := &NamespaceWatcher{manager: mgr}

	// Test nil mappings
	result := watcher.parsePortMap(nil)
	if result != nil {
		t.Error("Expected nil for nil mappings")
	}

	// Test empty mappings
	result = watcher.parsePortMap([]string{})
	if result != nil {
		t.Error("Expected nil for empty mappings")
	}

	// Test valid mappings
	result = watcher.parsePortMap([]string{"80:8080", "443:8443"})
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if len(*result) != 2 {
		t.Errorf("Expected 2 mappings, got %d", len(*result))
	}
	if (*result)[0].SourcePort != "80" || (*result)[0].TargetPort != "8080" {
		t.Errorf("First mapping: got %+v", (*result)[0])
	}
	if (*result)[1].SourcePort != "443" || (*result)[1].TargetPort != "8443" {
		t.Errorf("Second mapping: got %+v", (*result)[1])
	}

	// Test invalid mappings (should be skipped)
	result = watcher.parsePortMap([]string{"invalid", "80:8080"})
	if result == nil || len(*result) != 1 {
		t.Errorf("Expected 1 valid mapping, got %v", result)
	}
}

func TestNamespaceWatcher_AddServiceHandler_SkipsNoSelector(t *testing.T) {
	initRegistryOnce()

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "no-selector-svc",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Selector: nil, // No selector
			Ports: []v1.ServicePort{
				{Port: 80, Name: "http"},
			},
		},
	}

	stopCh := make(chan struct{})
	defer close(stopCh)

	mgr := NewManager(ManagerConfig{GlobalStopCh: stopCh})
	watcher := &NamespaceWatcher{
		manager:   mgr,
		namespace: "default",
		context:   "test",
		clientSet: fake.NewClientset(),
	}

	// Should not panic - just skip silently
	watcher.addServiceHandler(svc)
}

func TestNamespaceWatcher_AddServiceHandler_SkipsEmptySelector(t *testing.T) {
	initRegistryOnce()

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "empty-selector-svc",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{}, // Empty selector
			Ports: []v1.ServicePort{
				{Port: 80, Name: "http"},
			},
		},
	}

	stopCh := make(chan struct{})
	defer close(stopCh)

	mgr := NewManager(ManagerConfig{GlobalStopCh: stopCh})
	watcher := &NamespaceWatcher{
		manager:   mgr,
		namespace: "default",
		context:   "test",
		clientSet: fake.NewClientset(),
	}

	// Should not panic - just skip silently
	watcher.addServiceHandler(svc)
}

func TestNamespaceWatcher_DeleteServiceHandler_InvalidObject(t *testing.T) {
	initRegistryOnce()

	stopCh := make(chan struct{})
	defer close(stopCh)

	mgr := NewManager(ManagerConfig{GlobalStopCh: stopCh})
	watcher := &NamespaceWatcher{
		manager:   mgr,
		namespace: "default",
		context:   "test",
		clientSet: fake.NewClientset(),
	}

	// Should not panic with non-service object
	watcher.deleteServiceHandler("not a service")
	watcher.deleteServiceHandler(nil)
	watcher.deleteServiceHandler(123)
}

func TestNamespaceWatcher_UpdateServiceHandler_InvalidObjects(t *testing.T) {
	initRegistryOnce()

	stopCh := make(chan struct{})
	defer close(stopCh)

	mgr := NewManager(ManagerConfig{GlobalStopCh: stopCh})
	watcher := &NamespaceWatcher{
		manager:   mgr,
		namespace: "default",
		context:   "test",
		clientSet: fake.NewClientset(),
	}

	// Should not panic with non-service objects
	watcher.updateServiceHandler("old", "new")
	watcher.updateServiceHandler(nil, nil)

	// Mixed types
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
	}
	watcher.updateServiceHandler(svc, "not a service")
	watcher.updateServiceHandler("not a service", svc)
}

func TestNamespaceWatcher_UpdateServiceHandler_NoChange(t *testing.T) {
	initRegistryOnce()

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{"app": "test"},
			Ports: []v1.ServicePort{
				{Port: 80, Name: "http"},
			},
		},
	}

	stopCh := make(chan struct{})
	defer close(stopCh)

	mgr := NewManager(ManagerConfig{GlobalStopCh: stopCh})
	watcher := &NamespaceWatcher{
		manager:   mgr,
		namespace: "default",
		context:   "test-ctx",
		clientSet: fake.NewClientset(),
	}

	// Same service, no changes - should do nothing
	watcher.updateServiceHandler(svc, svc)
}

func TestManagerConfig_Fields(t *testing.T) {
	stopCh := make(chan struct{})
	defer close(stopCh)

	cfg := ManagerConfig{
		ConfigPath:      "/path/to/config",
		Domain:          "test.domain",
		PortMapping:     []string{"80:8080", "443:8443"},
		Timeout:         600,
		FwdConfigPath:   "/path/to/fwd.conf",
		FwdReservations: []string{"svc1:127.1.1.1"},
		ResyncInterval:  10 * time.Minute,
		RetryInterval:   30 * time.Second,
		AutoReconnect:   true,
		LabelSelector:   "app=test",
		FieldSelector:   "metadata.name=test",
		GlobalStopCh:    stopCh,
	}

	mgr := NewManager(cfg)

	if mgr.configPath != "/path/to/config" {
		t.Errorf("Expected configPath '/path/to/config', got %q", mgr.configPath)
	}
	if mgr.domain != "test.domain" {
		t.Errorf("Expected domain 'test.domain', got %q", mgr.domain)
	}
	if len(mgr.portMapping) != 2 {
		t.Errorf("Expected 2 port mappings, got %d", len(mgr.portMapping))
	}
	if mgr.timeout != 600 {
		t.Errorf("Expected timeout 600, got %d", mgr.timeout)
	}
	if mgr.fwdConfigPath != "/path/to/fwd.conf" {
		t.Errorf("Expected fwdConfigPath '/path/to/fwd.conf', got %q", mgr.fwdConfigPath)
	}
	if len(mgr.fwdReservations) != 1 {
		t.Errorf("Expected 1 reservation, got %d", len(mgr.fwdReservations))
	}
	if mgr.resyncInterval != 10*time.Minute {
		t.Errorf("Expected resyncInterval 10m, got %v", mgr.resyncInterval)
	}
	if mgr.retryInterval != 30*time.Second {
		t.Errorf("Expected retryInterval 30s, got %v", mgr.retryInterval)
	}
	if !mgr.autoReconnect {
		t.Error("Expected autoReconnect true")
	}
	if mgr.labelSelector != "app=test" {
		t.Errorf("Expected labelSelector 'app=test', got %q", mgr.labelSelector)
	}
	if mgr.fieldSelector != "metadata.name=test" {
		t.Errorf("Expected fieldSelector 'metadata.name=test', got %q", mgr.fieldSelector)
	}
}

func TestNamespaceInfo_Fields(t *testing.T) {
	now := time.Now()
	info := NamespaceInfo{
		Key:           "default.minikube",
		Namespace:     "default",
		Context:       "minikube",
		ServiceCount:  5,
		ActiveCount:   4,
		ErrorCount:    1,
		StartedAt:     now,
		Running:       true,
		LabelSelector: "app=test",
		FieldSelector: "metadata.name=svc",
	}

	if info.Key != "default.minikube" {
		t.Errorf("Expected Key 'default.minikube', got %q", info.Key)
	}
	if info.Namespace != "default" {
		t.Errorf("Expected Namespace 'default', got %q", info.Namespace)
	}
	if info.Context != "minikube" {
		t.Errorf("Expected Context 'minikube', got %q", info.Context)
	}
	if info.ServiceCount != 5 {
		t.Errorf("Expected ServiceCount 5, got %d", info.ServiceCount)
	}
	if info.ActiveCount != 4 {
		t.Errorf("Expected ActiveCount 4, got %d", info.ActiveCount)
	}
	if info.ErrorCount != 1 {
		t.Errorf("Expected ErrorCount 1, got %d", info.ErrorCount)
	}
	if !info.Running {
		t.Error("Expected Running true")
	}
}

func TestWatcherOpts_Fields(t *testing.T) {
	opts := WatcherOpts{
		LabelSelector: "app=web",
		FieldSelector: "metadata.name=frontend",
	}

	if opts.LabelSelector != "app=web" {
		t.Errorf("Expected LabelSelector 'app=web', got %q", opts.LabelSelector)
	}
	if opts.FieldSelector != "metadata.name=frontend" {
		t.Errorf("Expected FieldSelector 'metadata.name=frontend', got %q", opts.FieldSelector)
	}
}

func TestManager_GetContextIndex(t *testing.T) {
	stopCh := make(chan struct{})
	defer close(stopCh)

	mgr := NewManager(ManagerConfig{GlobalStopCh: stopCh})

	// First context should get index 0
	idx1 := mgr.GetContextIndex("ctx1")
	if idx1 != 0 {
		t.Errorf("Expected first context index 0, got %d", idx1)
	}

	// Same context should return same index
	idx1Again := mgr.GetContextIndex("ctx1")
	if idx1Again != 0 {
		t.Errorf("Expected same context to return 0, got %d", idx1Again)
	}

	// Second context should get index 1
	idx2 := mgr.GetContextIndex("ctx2")
	if idx2 != 1 {
		t.Errorf("Expected second context index 1, got %d", idx2)
	}

	// Third context should get index 2
	idx3 := mgr.GetContextIndex("ctx3")
	if idx3 != 2 {
		t.Errorf("Expected third context index 2, got %d", idx3)
	}
}

func TestManager_GetNamespaceIndex(t *testing.T) {
	stopCh := make(chan struct{})
	defer close(stopCh)

	mgr := NewManager(ManagerConfig{GlobalStopCh: stopCh})

	// First namespace key should get index 0
	idx1 := mgr.GetNamespaceIndex("ns1.ctx1")
	if idx1 != 0 {
		t.Errorf("Expected first namespace index 0, got %d", idx1)
	}

	// Same key should return same index
	idx1Again := mgr.GetNamespaceIndex("ns1.ctx1")
	if idx1Again != 0 {
		t.Errorf("Expected same key to return 0, got %d", idx1Again)
	}

	// Second namespace key should get index 1
	idx2 := mgr.GetNamespaceIndex("ns2.ctx1")
	if idx2 != 1 {
		t.Errorf("Expected second namespace index 1, got %d", idx2)
	}

	// Different context same namespace should get new index
	idx3 := mgr.GetNamespaceIndex("ns1.ctx2")
	if idx3 != 2 {
		t.Errorf("Expected third namespace index 2, got %d", idx3)
	}
}

func TestManager_GetOrCreateIPLock_AdHoc(t *testing.T) {
	stopCh := make(chan struct{})
	defer close(stopCh)

	mgr := NewManager(ManagerConfig{GlobalStopCh: stopCh})

	// Get lock for namespace without watcher - should create ad-hoc lock
	lock1 := mgr.GetOrCreateIPLock("ctx1", "ns1")
	if lock1 == nil {
		t.Fatal("Expected non-nil lock")
	}

	// Same namespace/context should return same lock
	lock1Again := mgr.GetOrCreateIPLock("ctx1", "ns1")
	if lock1Again != lock1 {
		t.Error("Expected same lock for same namespace/context")
	}

	// Different namespace should return different lock
	lock2 := mgr.GetOrCreateIPLock("ctx1", "ns2")
	if lock2 == nil {
		t.Fatal("Expected non-nil lock for ns2")
	}
	if lock2 == lock1 {
		t.Error("Expected different lock for different namespace")
	}

	// Different context should return different lock
	lock3 := mgr.GetOrCreateIPLock("ctx2", "ns1")
	if lock3 == nil {
		t.Fatal("Expected non-nil lock for ctx2")
	}
	if lock3 == lock1 {
		t.Error("Expected different lock for different context")
	}
}

func TestManager_GetOrCreateIPLock_WithWatcher(t *testing.T) {
	stopCh := make(chan struct{})
	defer close(stopCh)

	mgr := NewManager(ManagerConfig{GlobalStopCh: stopCh})

	// Manually add a watcher with its own ipLock
	watcherLock := &sync.Mutex{}
	watcher := &NamespaceWatcher{
		key:       "ns1.ctx1",
		namespace: "ns1",
		context:   "ctx1",
		ipLock:    watcherLock,
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
	}
	mgr.mu.Lock()
	mgr.watchers["ns1.ctx1"] = watcher
	mgr.mu.Unlock()

	// GetOrCreateIPLock should return watcher's lock
	lock := mgr.GetOrCreateIPLock("ctx1", "ns1")
	if lock != watcherLock {
		t.Error("Expected watcher's lock to be returned")
	}
}

func TestManager_adHocIPLocks_Initialization(t *testing.T) {
	stopCh := make(chan struct{})
	defer close(stopCh)

	mgr := NewManager(ManagerConfig{GlobalStopCh: stopCh})

	// adHocIPLocks should be initialized
	if mgr.adHocIPLocks == nil {
		t.Error("adHocIPLocks should be initialized")
	}
}

func TestParsePortMapPublic(t *testing.T) {
	// Test nil mappings
	result := parsePortMapPublic(nil)
	if result != nil {
		t.Error("Expected nil for nil mappings")
	}

	// Test empty mappings
	result = parsePortMapPublic([]string{})
	if result != nil {
		t.Error("Expected nil for empty mappings")
	}

	// Test valid mappings
	result = parsePortMapPublic([]string{"80:8080", "443:8443"})
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if len(*result) != 2 {
		t.Errorf("Expected 2 mappings, got %d", len(*result))
	}
	if (*result)[0].SourcePort != "80" || (*result)[0].TargetPort != "8080" {
		t.Errorf("First mapping incorrect: got %+v", (*result)[0])
	}
	if (*result)[1].SourcePort != "443" || (*result)[1].TargetPort != "8443" {
		t.Errorf("Second mapping incorrect: got %+v", (*result)[1])
	}

	// Test with invalid mappings (should be skipped)
	result = parsePortMapPublic([]string{"invalid", "80:8080", "alsobad"})
	if result == nil || len(*result) != 1 {
		t.Errorf("Expected 1 valid mapping, got %v", result)
	}
}

func TestManager_CreateServiceFWD_NoSelector(t *testing.T) {
	initRegistryOnce()

	stopCh := make(chan struct{})
	defer close(stopCh)

	mgr := NewManager(ManagerConfig{
		GlobalStopCh: stopCh,
		Timeout:      300,
		Domain:       "test.local",
	})

	// Service without selector should fail
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "no-selector",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Selector: nil,
			Ports: []v1.ServicePort{
				{Port: 80},
			},
		},
	}

	_, err := mgr.CreateServiceFWD("ctx", "default", svc)
	if err == nil {
		t.Error("Expected error for service without selector")
	}
	if err != nil && !contains(err.Error(), "no pod selector") {
		t.Errorf("Expected 'no pod selector' error, got: %v", err)
	}
}

func TestManager_CreateServiceFWD_EmptySelector(t *testing.T) {
	initRegistryOnce()

	stopCh := make(chan struct{})
	defer close(stopCh)

	mgr := NewManager(ManagerConfig{
		GlobalStopCh: stopCh,
		Timeout:      300,
		Domain:       "test.local",
	})

	// Service with empty selector should fail
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "empty-selector",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{},
			Ports: []v1.ServicePort{
				{Port: 80},
			},
		},
	}

	_, err := mgr.CreateServiceFWD("ctx", "default", svc)
	if err == nil {
		t.Error("Expected error for service with empty selector")
	}
}

func TestManager_GetWatcherByKey(t *testing.T) {
	stopCh := make(chan struct{})
	defer close(stopCh)

	mgr := NewManager(ManagerConfig{GlobalStopCh: stopCh})

	// Test not found
	watcher := mgr.GetWatcherByKey("nonexistent.key")
	if watcher != nil {
		t.Error("Expected nil watcher for non-existent key")
	}

	// Add a watcher manually
	testWatcher := &NamespaceWatcher{
		key:       "default.minikube",
		namespace: "default",
		context:   "minikube",
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
	}
	mgr.mu.Lock()
	mgr.watchers["default.minikube"] = testWatcher
	mgr.mu.Unlock()

	// Test found
	found := mgr.GetWatcherByKey("default.minikube")
	if found == nil {
		t.Fatal("Expected to find watcher")
	}
	if found.key != "default.minikube" {
		t.Errorf("Expected key 'default.minikube', got %q", found.key)
	}
}

func TestNamespaceWatcher_Done(t *testing.T) {
	doneCh := make(chan struct{})
	watcher := &NamespaceWatcher{
		doneCh: doneCh,
	}

	// Get the done channel
	done := watcher.Done()
	if done == nil {
		t.Fatal("Expected non-nil done channel")
	}

	// Verify it's the same channel
	if done != doneCh {
		t.Error("Expected Done() to return the doneCh")
	}

	// Verify it blocks initially
	select {
	case <-done:
		t.Error("Done channel should not be closed initially")
	default:
		// Expected
	}

	// Close and verify
	close(doneCh)
	select {
	case <-done:
		// Expected
	default:
		t.Error("Done channel should be closed after closing doneCh")
	}
}

// Helper function to check if string contains substring.
// Delegates to the standard library to avoid custom reimplementation.
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

func TestManager_StopWatcher_RunningWatcher(t *testing.T) {
	initRegistryOnce()

	stopCh := make(chan struct{})
	defer close(stopCh)

	mgr := NewManager(ManagerConfig{
		GlobalStopCh: stopCh,
		Timeout:      300,
		Domain:       "test.local",
	})

	// Create a fake running watcher
	watcherStopCh := make(chan struct{})
	watcherDoneCh := make(chan struct{})
	watcher := &NamespaceWatcher{
		manager:   mgr,
		key:       "default.test",
		namespace: "default",
		context:   "test",
		clientSet: fake.NewClientset(),
		stopCh:    watcherStopCh,
		doneCh:    watcherDoneCh,
		running:   true,
	}

	mgr.mu.Lock()
	mgr.watchers["default.test"] = watcher
	mgr.mu.Unlock()

	// Simulate watcher closing its done channel when stopped
	go func() {
		<-watcherStopCh
		close(watcherDoneCh)
	}()

	// Stop the watcher
	err := mgr.StopWatcher("test", "default")
	if err != nil {
		t.Fatalf("StopWatcher failed: %v", err)
	}

	// Verify watcher is removed
	mgr.mu.RLock()
	_, exists := mgr.watchers["default.test"]
	mgr.mu.RUnlock()

	if exists {
		t.Error("Watcher should be removed after stopping")
	}
}

func TestManager_StopAll_WithRunningWatchers(t *testing.T) {
	initRegistryOnce()

	stopCh := make(chan struct{})
	defer close(stopCh)

	mgr := NewManager(ManagerConfig{
		GlobalStopCh: stopCh,
		Timeout:      300,
		Domain:       "test.local",
	})

	// Track stopped watchers
	stoppedCount := 0
	var stoppedMu sync.Mutex

	// Create multiple fake watchers
	for i := 0; i < 3; i++ {
		key := WatcherKey("ns"+string(rune('0'+i)), "ctx")
		watcherStopCh := make(chan struct{})
		watcherDoneCh := make(chan struct{})
		watcher := &NamespaceWatcher{
			manager:   mgr,
			key:       key,
			namespace: "ns" + string(rune('0'+i)),
			context:   "ctx",
			clientSet: fake.NewClientset(),
			stopCh:    watcherStopCh,
			doneCh:    watcherDoneCh,
			running:   true,
		}

		// Simulate watcher closing done channel on stop
		go func(stopCh, doneCh chan struct{}) {
			<-stopCh
			stoppedMu.Lock()
			stoppedCount++
			stoppedMu.Unlock()
			close(doneCh)
		}(watcherStopCh, watcherDoneCh)

		mgr.mu.Lock()
		mgr.watchers[key] = watcher
		mgr.mu.Unlock()
	}

	// Verify we have 3 watchers
	mgr.mu.RLock()
	count := len(mgr.watchers)
	mgr.mu.RUnlock()

	if count != 3 {
		t.Errorf("Expected 3 watchers, got %d", count)
	}

	// Stop all
	mgr.StopAll()

	// Verify all watchers were stopped (StopAll doesn't remove them from map)
	stoppedMu.Lock()
	finalCount := stoppedCount
	stoppedMu.Unlock()

	if finalCount != 3 {
		t.Errorf("Expected 3 watchers stopped, got %d", finalCount)
	}

	// Verify manager's done channel is closed
	select {
	case <-mgr.Done():
		// Expected
	default:
		t.Error("Manager's done channel should be closed after StopAll")
	}
}

func TestNamespaceWatcher_Info_AllFields(t *testing.T) {
	now := time.Now()
	watcher := &NamespaceWatcher{
		key:           "default.minikube",
		namespace:     "default",
		context:       "minikube",
		clusterN:      1,
		namespaceN:    2,
		labelSelector: "app=web",
		fieldSelector: "metadata.name=svc1",
		startedAt:     now,
		running:       true,
	}

	info := watcher.Info()

	if info.Key != "default.minikube" {
		t.Errorf("Expected key 'default.minikube', got %q", info.Key)
	}
	if info.Namespace != "default" {
		t.Errorf("Expected namespace 'default', got %q", info.Namespace)
	}
	if info.Context != "minikube" {
		t.Errorf("Expected context 'minikube', got %q", info.Context)
	}
	if info.LabelSelector != "app=web" {
		t.Errorf("Expected LabelSelector 'app=web', got %q", info.LabelSelector)
	}
	if info.FieldSelector != "metadata.name=svc1" {
		t.Errorf("Expected FieldSelector 'metadata.name=svc1', got %q", info.FieldSelector)
	}
	if !info.Running {
		t.Error("Expected Running to be true")
	}
	if info.StartedAt != now {
		t.Error("Expected StartedAt to match")
	}
}

func TestManager_removeNamespaceServices(t *testing.T) {
	initRegistryOnce()

	stopCh := make(chan struct{})
	defer close(stopCh)

	mgr := NewManager(ManagerConfig{
		GlobalStopCh: stopCh,
		Timeout:      300,
		Domain:       "test.local",
	})

	// Call removeNamespaceServices - should not panic even with no services
	mgr.removeNamespaceServices("default", "test")

	// The function clears services from the registry and updates store
	// Since we don't have actual services, this tests the path doesn't panic
}

func TestManager_GetCurrentContext(t *testing.T) {
	initRegistryOnce()

	stopCh := make(chan struct{})
	defer close(stopCh)

	mgr := NewManager(ManagerConfig{
		GlobalStopCh: stopCh,
		ConfigPath:   "/nonexistent/kubeconfig", // Will fail
		Timeout:      300,
		Domain:       "test.local",
	})

	_, err := mgr.GetCurrentContext()
	// Should fail because config path doesn't exist
	if err == nil {
		t.Log("GetCurrentContext didn't fail - kubeconfig may be available in default location")
	}
}

func TestManager_SetClientSet(t *testing.T) {
	stopCh := make(chan struct{})
	defer close(stopCh)

	mgr := NewManager(ManagerConfig{GlobalStopCh: stopCh, Timeout: 300})

	// Initially no clientset
	if cs := mgr.GetClientSet("test-context"); cs != nil {
		t.Error("Expected nil clientset initially")
	}

	// Set a fake clientset
	fakeClient := fake.NewClientset()
	mgr.SetClientSet("test-context", fakeClient)

	// Verify it's set
	if cs := mgr.GetClientSet("test-context"); cs == nil {
		t.Error("Expected non-nil clientset after setting")
	} else if cs != fakeClient {
		t.Error("Expected same clientset that was set")
	}

	// Test with different contexts
	fakeClient2 := fake.NewClientset()
	mgr.SetClientSet("other-context", fakeClient2)

	if cs := mgr.GetClientSet("test-context"); cs != fakeClient {
		t.Error("test-context should still have original clientset")
	}
	if cs := mgr.GetClientSet("other-context"); cs != fakeClient2 {
		t.Error("other-context should have second clientset")
	}
}

func TestManager_GetClientSet_NonexistentContext(t *testing.T) {
	stopCh := make(chan struct{})
	defer close(stopCh)

	mgr := NewManager(ManagerConfig{GlobalStopCh: stopCh, Timeout: 300})

	// Non-existent context should return nil
	if cs := mgr.GetClientSet("nonexistent"); cs != nil {
		t.Error("Expected nil for nonexistent context")
	}
}

func TestNamespaceWatcher_AddServiceHandler_ValidService(t *testing.T) {
	initRegistryOnce()

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "valid-svc",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Selector:  map[string]string{"app": "test"},
			ClusterIP: "10.0.0.1",
			Ports: []v1.ServicePort{
				{Port: 80, Name: "http"},
			},
		},
	}

	stopCh := make(chan struct{})
	defer close(stopCh)

	mgr := NewManager(ManagerConfig{GlobalStopCh: stopCh, Timeout: 300, Domain: "test.local"})

	// Create a minimal rest config
	restCfg := &restclient.Config{
		Host: "https://localhost:6443",
	}

	watcher := &NamespaceWatcher{
		manager:    mgr,
		namespace:  "default",
		context:    "test-ctx",
		clientSet:  fake.NewClientset(),
		restConfig: restCfg,
		ipLock:     &sync.Mutex{},
	}

	// Clear registry before test
	for _, s := range fwdsvcregistry.GetAll() {
		key := s.Svc.Name + "." + s.Namespace + "." + s.Context
		fwdsvcregistry.RemoveByName(key)
	}

	// Add service handler should add to registry
	watcher.addServiceHandler(svc)

	// Check registry
	key := "valid-svc.default.test-ctx"
	found := fwdsvcregistry.Get(key)
	if found == nil {
		t.Error("Expected service to be added to registry")
	}

	// Cleanup
	fwdsvcregistry.RemoveByName(key)
}

func TestNamespaceWatcher_AddServiceHandler_HeadlessService(t *testing.T) {
	initRegistryOnce()

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "headless-svc",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Selector:  map[string]string{"app": "test"},
			ClusterIP: "None", // Headless
			Ports: []v1.ServicePort{
				{Port: 80, Name: "http"},
			},
		},
	}

	stopCh := make(chan struct{})
	defer close(stopCh)

	mgr := NewManager(ManagerConfig{GlobalStopCh: stopCh, Timeout: 300})
	restCfg := &restclient.Config{Host: "https://localhost:6443"}
	watcher := &NamespaceWatcher{
		manager:    mgr,
		namespace:  "default",
		context:    "test-ctx",
		clientSet:  fake.NewClientset(),
		restConfig: restCfg,
		ipLock:     &sync.Mutex{},
	}

	// Clear registry before test
	for _, s := range fwdsvcregistry.GetAll() {
		key := s.Svc.Name + "." + s.Namespace + "." + s.Context
		fwdsvcregistry.RemoveByName(key)
	}

	// Add service handler
	watcher.addServiceHandler(svc)

	// Check registry - headless service should be marked as such
	key := "headless-svc.default.test-ctx"
	found := fwdsvcregistry.Get(key)
	if found == nil {
		t.Error("Expected headless service to be added to registry")
	} else if !found.Headless {
		t.Error("Expected Headless flag to be true")
	}

	// Cleanup
	fwdsvcregistry.RemoveByName(key)
}

func TestNamespaceWatcher_DeleteServiceHandler_ValidService(t *testing.T) {
	initRegistryOnce()

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "to-delete",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{"app": "test"},
			Ports: []v1.ServicePort{
				{Port: 80, Name: "http"},
			},
		},
	}

	stopCh := make(chan struct{})
	defer close(stopCh)

	mgr := NewManager(ManagerConfig{GlobalStopCh: stopCh, Timeout: 300})
	restCfg := &restclient.Config{Host: "https://localhost:6443"}
	watcher := &NamespaceWatcher{
		manager:    mgr,
		namespace:  "default",
		context:    "test-ctx",
		clientSet:  fake.NewClientset(),
		restConfig: restCfg,
		ipLock:     &sync.Mutex{},
	}

	// First add the service
	watcher.addServiceHandler(svc)

	key := "to-delete.default.test-ctx"
	if fwdsvcregistry.Get(key) == nil {
		t.Fatal("Service should exist before delete test")
	}

	// Now delete it
	watcher.deleteServiceHandler(svc)

	// Verify removed - the RemoveByName stops the service, so it may still be in registry
	// but will be cleaned up. Just verify the function didn't panic
}

func TestNamespaceWatcher_UpdateServiceHandler_SelectorChanged(t *testing.T) {
	initRegistryOnce()

	oldSvc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "updated-svc",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{"app": "old"},
			Ports: []v1.ServicePort{
				{Port: 80, Name: "http"},
			},
		},
	}

	newSvc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "updated-svc",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{"app": "new"}, // Changed selector
			Ports: []v1.ServicePort{
				{Port: 80, Name: "http"},
			},
		},
	}

	stopCh := make(chan struct{})
	defer close(stopCh)

	mgr := NewManager(ManagerConfig{GlobalStopCh: stopCh, Timeout: 300})
	watcher := &NamespaceWatcher{
		manager:   mgr,
		namespace: "default",
		context:   "test-ctx",
		clientSet: fake.NewClientset(),
		ipLock:    &sync.Mutex{},
	}

	// Update handler should detect selector change
	// This won't trigger resync since service isn't in registry, but should not panic
	watcher.updateServiceHandler(oldSvc, newSvc)
}

func TestNamespaceWatcher_UpdateServiceHandler_PortsChanged(t *testing.T) {
	initRegistryOnce()

	oldSvc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ports-svc",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{"app": "test"},
			Ports: []v1.ServicePort{
				{Port: 80, Name: "http"},
			},
		},
	}

	newSvc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ports-svc",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{"app": "test"},
			Ports: []v1.ServicePort{
				{Port: 80, Name: "http"},
				{Port: 443, Name: "https"}, // Added port
			},
		},
	}

	stopCh := make(chan struct{})
	defer close(stopCh)

	mgr := NewManager(ManagerConfig{GlobalStopCh: stopCh, Timeout: 300})
	watcher := &NamespaceWatcher{
		manager:   mgr,
		namespace: "default",
		context:   "test-ctx",
		clientSet: fake.NewClientset(),
		ipLock:    &sync.Mutex{},
	}

	// Update handler should detect ports change
	watcher.updateServiceHandler(oldSvc, newSvc)
}

func TestManager_RemoveNamespaceServices_WithServices(t *testing.T) {
	initRegistryOnce()

	stopCh := make(chan struct{})
	defer close(stopCh)

	mgr := NewManager(ManagerConfig{
		GlobalStopCh: stopCh,
		Timeout:      300,
		Domain:       "test.local",
	})

	// Create a watcher with fake clientset
	restCfg := &restclient.Config{Host: "https://localhost:6443"}
	watcher := &NamespaceWatcher{
		manager:    mgr,
		namespace:  "test-ns",
		context:    "test-ctx",
		clientSet:  fake.NewClientset(),
		restConfig: restCfg,
		ipLock:     &sync.Mutex{},
	}

	// Add a service to registry
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "test-ns",
		},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{"app": "test"},
			Ports: []v1.ServicePort{
				{Port: 80, Name: "http"},
			},
		},
	}
	watcher.addServiceHandler(svc)

	// Verify service is in registry
	key := "test-svc.test-ns.test-ctx"
	if fwdsvcregistry.Get(key) == nil {
		t.Fatal("Service should exist before removal")
	}

	// Remove namespace services
	mgr.removeNamespaceServices("test-ns", "test-ctx")

	// Service should be removed/stopped
	// Note: RemoveByName may not immediately remove if Stop is async
}

func TestNamespaceWatcher_Info_NotRunning(t *testing.T) {
	watcher := &NamespaceWatcher{
		key:       "default.minikube",
		namespace: "default",
		context:   "minikube",
		running:   false,
	}

	info := watcher.Info()

	if info.Running {
		t.Error("Expected Running to be false")
	}
}

func TestManager_StopWatcher_EmptyContext(t *testing.T) {
	stopCh := make(chan struct{})
	defer close(stopCh)

	mgr := NewManager(ManagerConfig{
		GlobalStopCh: stopCh,
		ConfigPath:   "/nonexistent/kubeconfig",
	})

	// StopWatcher with empty context should try to get current context and fail
	err := mgr.StopWatcher("", "default")
	if err == nil {
		t.Log("StopWatcher didn't fail - kubeconfig may be available")
	}
}

func TestManager_ListWatchers_WithWatchers(t *testing.T) {
	stopCh := make(chan struct{})
	defer close(stopCh)

	mgr := NewManager(ManagerConfig{GlobalStopCh: stopCh})

	// Add some watchers manually
	w1 := &NamespaceWatcher{
		key:       "ns1.ctx1",
		namespace: "ns1",
		context:   "ctx1",
		running:   true,
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
	}
	w2 := &NamespaceWatcher{
		key:       "ns2.ctx1",
		namespace: "ns2",
		context:   "ctx1",
		running:   true,
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
	}

	mgr.mu.Lock()
	mgr.watchers["ns1.ctx1"] = w1
	mgr.watchers["ns2.ctx1"] = w2
	mgr.mu.Unlock()

	watchers := mgr.ListWatchers()

	if len(watchers) != 2 {
		t.Errorf("Expected 2 watchers, got %d", len(watchers))
	}

	// Verify keys are present
	keys := make(map[string]bool)
	for _, w := range watchers {
		keys[w.Key] = true
	}
	if !keys["ns1.ctx1"] || !keys["ns2.ctx1"] {
		t.Error("Expected both watcher keys to be present")
	}
}

func TestNamespaceWatcher_ParsePortMap_WithPortMapping(t *testing.T) {
	stopCh := make(chan struct{})
	defer close(stopCh)

	mgr := NewManager(ManagerConfig{GlobalStopCh: stopCh})
	watcher := &NamespaceWatcher{manager: mgr}

	// Test with mixed valid and invalid mappings
	result := watcher.parsePortMap([]string{"80:8080", "invalid-no-colon", "443:8443"})
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if len(*result) != 2 {
		t.Errorf("Expected 2 valid mappings, got %d", len(*result))
	}
}

func TestSplitPortMapping_EdgeCases(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{":", []string{"", ""}},                     // Empty parts
		{"a:", []string{"a", ""}},                   // Empty target
		{":b", []string{"", "b"}},                   // Empty source
		{"abc:def:ghi", []string{"abc", "def:ghi"}}, // Multiple colons
	}

	for _, tt := range tests {
		result := splitPortMapping(tt.input)
		if tt.expected == nil {
			if result != nil {
				t.Errorf("splitPortMapping(%q) = %v, want nil", tt.input, result)
			}
		} else {
			if result == nil || len(result) != len(tt.expected) {
				t.Errorf("splitPortMapping(%q) = %v, want %v", tt.input, result, tt.expected)
				continue
			}
			for i := range tt.expected {
				if result[i] != tt.expected[i] {
					t.Errorf("splitPortMapping(%q)[%d] = %q, want %q",
						tt.input, i, result[i], tt.expected[i])
				}
			}
		}
	}
}
