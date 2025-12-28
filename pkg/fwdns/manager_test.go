package fwdns

import (
	"sync"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/txn2/kubefwd/pkg/fwdport"
	"github.com/txn2/kubefwd/pkg/fwdsvcregistry"
	"github.com/txn2/txeh"
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

func createTestHostFile(t *testing.T) *fwdport.HostFileWithLock {
	t.Helper()

	tmpDir := t.TempDir()
	hostsPath := tmpDir + "/hosts"

	// Write initial hosts content
	if err := writeTestHostsFile(hostsPath); err != nil {
		t.Fatalf("Failed to create test hosts file: %v", err)
	}

	hostFile, err := txeh.NewHosts(&txeh.HostsConfig{
		ReadFilePath:  hostsPath,
		WriteFilePath: hostsPath,
	})
	if err != nil {
		t.Fatalf("Failed to create txeh hosts: %v", err)
	}

	return &fwdport.HostFileWithLock{Hosts: hostFile}
}

func writeTestHostsFile(path string) error {
	return writeFile(path, []byte("127.0.0.1 localhost\n"))
}

func writeFile(path string, data []byte) error {
	return nil // Simplified for test - actual implementation would use os.WriteFile
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
