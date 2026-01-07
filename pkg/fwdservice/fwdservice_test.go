package fwdservice

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/txn2/kubefwd/pkg/fwdnet"
	"github.com/txn2/kubefwd/pkg/fwdport"
	"github.com/txn2/kubefwd/pkg/fwdtui"
	"github.com/txn2/kubefwd/pkg/fwdtui/events"
	"github.com/txn2/txeh"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
)

// setupMockInterface sets up the mock interface manager for tests
// and returns a cleanup function to restore the original
func setupMockInterface() func() {
	mock := fwdnet.NewMockInterfaceManager()
	fwdnet.SetManager(mock)
	return func() {
		fwdnet.ResetManager()
	}
}

// setupMockRESTClient creates a mock REST client that blocks on requests.
// This allows tests to verify that pods are added to the PortForwards map
// before PortForward() returns. Returns the RESTClient and a cleanup function.
func setupMockRESTClient() (*restclient.RESTClient, func()) {
	// Create a test server that blocks indefinitely on port-forward requests
	// This simulates a real port-forward connection staying open
	blockChan := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block until cleanup is called
		<-blockChan
	}))

	config := &restclient.Config{
		Host: server.URL,
		ContentConfig: restclient.ContentConfig{
			GroupVersion:         &v1.SchemeGroupVersion,
			NegotiatedSerializer: scheme.Codecs.WithoutConversion(),
		},
	}

	client, err := restclient.RESTClientFor(config)
	if err != nil {
		// If we can't create a client, return nil - tests should handle this
		server.Close()
		close(blockChan)
		return nil, func() {}
	}

	cleanup := func() {
		close(blockChan) // Unblock any waiting handlers
		server.Close()
	}

	return client, cleanup
}

// mockDebouncer for testing debouncing behavior
type mockDebouncer struct {
	calls     int
	lastFunc  func()
	mutex     sync.Mutex
	immediate bool // If true, execute immediately instead of debouncing
}

func (m *mockDebouncer) debounce(f func()) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.calls++
	m.lastFunc = f
	if m.immediate {
		f()
	}
}

func (m *mockDebouncer) getCalls() int {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.calls
}

// createTestService creates a test Kubernetes service
func createTestService(name, namespace string, ports []v1.ServicePort, headless bool) *v1.Service {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1.ServiceSpec{
			Ports: ports,
		},
	}
	if headless {
		svc.Spec.ClusterIP = "None"
	} else {
		svc.Spec.ClusterIP = "10.0.0.1"
	}
	return svc
}

// createTestPod creates a test Kubernetes pod
func createTestPod(name, namespace string, phase v1.PodPhase, labels map[string]string) *v1.Pod {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Status: v1.PodStatus{
			Phase: phase,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name: "test-container",
					Ports: []v1.ContainerPort{
						{Name: "http", ContainerPort: 8080},
					},
				},
			},
		},
	}

	// For Running pods, add Ready container status
	if phase == v1.PodRunning {
		pod.Status.ContainerStatuses = []v1.ContainerStatus{
			{
				Name:  "test-container",
				Ready: true,
			},
		}
	}

	return pod
}

// TestGetPodsForService_FiltersByPhase tests that only Pending/Running pods are returned
func TestGetPodsForService_FiltersByPhase(t *testing.T) {
	namespace := "default"
	labels := map[string]string{"app": "test"}

	// Create pods in various phases
	runningPod := createTestPod("running-pod", namespace, v1.PodRunning, labels)
	pendingPod := createTestPod("pending-pod", namespace, v1.PodPending, labels)
	succeededPod := createTestPod("succeeded-pod", namespace, v1.PodSucceeded, labels)
	failedPod := createTestPod("failed-pod", namespace, v1.PodFailed, labels)

	clientset := fake.NewClientset(runningPod, pendingPod, succeededPod, failedPod)

	svc := createTestService("test-svc", namespace, []v1.ServicePort{
		{Port: 80, TargetPort: intstr.FromInt32(8080), Protocol: v1.ProtocolTCP},
	}, false)

	svcFwd := &ServiceFWD{
		ClientSet:        clientset,
		Svc:              svc,
		PodLabelSelector: "app=test",
	}

	pods := svcFwd.GetPodsForService()

	// Should only return Running and Pending pods
	if len(pods) != 2 {
		t.Errorf("Expected 2 eligible pods, got %d", len(pods))
	}

	// Verify correct pods are returned
	foundRunning := false
	foundPending := false
	for _, pod := range pods {
		if pod.Name == "running-pod" {
			foundRunning = true
		}
		if pod.Name == "pending-pod" {
			foundPending = true
		}
		// Should NOT find succeeded or failed pods
		if pod.Name == "succeeded-pod" || pod.Name == "failed-pod" {
			t.Errorf("Found pod %s which should have been filtered out", pod.Name)
		}
	}

	if !foundRunning {
		t.Error("Running pod was not returned")
	}
	if !foundPending {
		t.Error("Pending pod was not returned")
	}
}

// TestGetPodsForService_NoPodsFound tests handling when no pods match the selector
func TestGetPodsForService_NoPodsFound(t *testing.T) {
	namespace := "default"
	clientset := fake.NewClientset()

	svc := createTestService("test-svc", namespace, []v1.ServicePort{
		{Port: 80, TargetPort: intstr.FromInt32(8080), Protocol: v1.ProtocolTCP},
	}, false)

	svcFwd := &ServiceFWD{
		ClientSet:        clientset,
		Svc:              svc,
		PodLabelSelector: "app=nonexistent",
	}

	pods := svcFwd.GetPodsForService()

	// Should return empty slice, not nil
	if pods == nil {
		t.Error("Expected empty slice, got nil")
	}
	if len(pods) != 0 {
		t.Errorf("Expected 0 pods, got %d", len(pods))
	}
}

// TestGetPodsForService_OnlyRunningPods tests filtering with only running pods
func TestGetPodsForService_OnlyRunningPods(t *testing.T) {
	namespace := "default"
	labels := map[string]string{"app": "test"}

	runningPod1 := createTestPod("running-pod-1", namespace, v1.PodRunning, labels)
	runningPod2 := createTestPod("running-pod-2", namespace, v1.PodRunning, labels)

	clientset := fake.NewClientset(runningPod1, runningPod2)

	svc := createTestService("test-svc", namespace, []v1.ServicePort{
		{Port: 80, TargetPort: intstr.FromInt32(8080), Protocol: v1.ProtocolTCP},
	}, false)

	svcFwd := &ServiceFWD{
		ClientSet:        clientset,
		Svc:              svc,
		PodLabelSelector: "app=test",
	}

	pods := svcFwd.GetPodsForService()

	if len(pods) != 2 {
		t.Errorf("Expected 2 running pods, got %d", len(pods))
	}
}

// TestSyncPodForwards_NormalService tests syncing for a normal (non-headless) service
//
//goland:noinspection DuplicatedCode
func TestSyncPodForwards_NormalService(t *testing.T) {
	cleanup := setupMockInterface()
	defer cleanup()

	restClient, restCleanup := setupMockRESTClient()
	defer restCleanup()

	namespace := "default"
	labels := map[string]string{"app": "test"}

	runningPod1 := createTestPod("running-pod-1", namespace, v1.PodRunning, labels)
	runningPod2 := createTestPod("running-pod-2", namespace, v1.PodRunning, labels)

	clientset := fake.NewClientset(runningPod1, runningPod2)

	svc := createTestService("test-svc", namespace, []v1.ServicePort{
		{Port: 80, TargetPort: intstr.FromInt32(8080), Protocol: v1.ProtocolTCP},
	}, false)

	// Create hosts file mock
	hosts, err := txeh.NewHosts(&txeh.HostsConfig{})
	if err != nil {
		t.Fatalf("Failed to create txeh.Hosts: %v", err)
	}
	hostFile := &fwdport.HostFileWithLock{Hosts: hosts}

	debouncer := &mockDebouncer{immediate: true}

	svcFwd := &ServiceFWD{
		ClientSet:            clientset,
		Svc:                  svc,
		PodLabelSelector:     "app=test",
		Headless:             false,
		Context:              "test-context",
		Namespace:            namespace,
		PortForwards:         make(map[string]*fwdport.PortForwardOpts),
		NamespaceServiceLock: &sync.Mutex{},
		Hostfile:             hostFile,
		SyncDebouncer:        debouncer.debounce,
		LastSyncedAt:         time.Now().Add(-10 * time.Minute), // Old sync time
		RESTClient:           restClient,
	}

	// Sync with force=true to bypass debouncer
	svcFwd.SyncPodForwards(true)

	// Give time for goroutines to add pods to PortForwards map
	time.Sleep(200 * time.Millisecond)

	svcFwd.NamespaceServiceLock.Lock()
	numPods := len(svcFwd.PortForwards)
	svcFwd.NamespaceServiceLock.Unlock()

	// For normal service, should only forward ONE pod
	if numPods != 1 {
		t.Errorf("Normal service should forward 1 pod, got %d", numPods)
	}

	// Verify it's one of our pods (key format: service.podname.localport)
	foundPod := false
	svcFwd.NamespaceServiceLock.Lock()
	for key := range svcFwd.PortForwards {
		if key == "test-svc.running-pod-1.80" || key == "test-svc.running-pod-2.80" {
			foundPod = true
		}
	}
	svcFwd.NamespaceServiceLock.Unlock()

	if !foundPod {
		t.Error("Did not find expected pod in PortForwards map")
	}
}

// TestSyncPodForwards_HeadlessService tests syncing for a headless service
//
//goland:noinspection DuplicatedCode
func TestSyncPodForwards_HeadlessService(t *testing.T) {
	cleanup := setupMockInterface()
	defer cleanup()

	restClient, restCleanup := setupMockRESTClient()
	defer restCleanup()

	namespace := "default"
	labels := map[string]string{"app": "test"}

	runningPod1 := createTestPod("running-pod-1", namespace, v1.PodRunning, labels)
	runningPod2 := createTestPod("running-pod-2", namespace, v1.PodRunning, labels)

	clientset := fake.NewClientset(runningPod1, runningPod2)

	svc := createTestService("test-svc", namespace, []v1.ServicePort{
		{Port: 80, TargetPort: intstr.FromInt32(8080), Protocol: v1.ProtocolTCP},
	}, true) // Headless service

	hosts, err := txeh.NewHosts(&txeh.HostsConfig{})
	if err != nil {
		t.Fatalf("Failed to create txeh.Hosts: %v", err)
	}
	hostFile := &fwdport.HostFileWithLock{Hosts: hosts}

	debouncer := &mockDebouncer{immediate: true}

	svcFwd := &ServiceFWD{
		ClientSet:            clientset,
		Svc:                  svc,
		PodLabelSelector:     "app=test",
		Headless:             true,
		Context:              "test-context",
		Namespace:            namespace,
		PortForwards:         make(map[string]*fwdport.PortForwardOpts),
		NamespaceServiceLock: &sync.Mutex{},
		Hostfile:             hostFile,
		SyncDebouncer:        debouncer.debounce,
		LastSyncedAt:         time.Now().Add(-10 * time.Minute),
		RESTClient:           restClient,
	}

	svcFwd.SyncPodForwards(true)

	// Give time for goroutines to add pods to PortForwards map
	time.Sleep(200 * time.Millisecond)

	svcFwd.NamespaceServiceLock.Lock()
	numPods := len(svcFwd.PortForwards)
	svcFwd.NamespaceServiceLock.Unlock()

	// For headless service, should forward ALL pods (2 + 1 for service name)
	// First pod gets service name, then all pods get individual names
	if numPods < 2 {
		t.Errorf("Headless service should forward at least 2 pods, got %d", numPods)
	}
}

// TestSyncPodForwards_Debouncing tests debouncing behavior
//
//goland:noinspection DuplicatedCode
func TestSyncPodForwards_Debouncing(t *testing.T) {
	namespace := "default"
	labels := map[string]string{"app": "test"}

	runningPod := createTestPod("running-pod", namespace, v1.PodRunning, labels)
	clientset := fake.NewClientset(runningPod)

	svc := createTestService("test-svc", namespace, []v1.ServicePort{
		{Port: 80, TargetPort: intstr.FromInt32(8080), Protocol: v1.ProtocolTCP},
	}, false)

	hosts, err := txeh.NewHosts(&txeh.HostsConfig{})
	if err != nil {
		t.Fatalf("Failed to create txeh.Hosts: %v", err)
	}
	hostFile := &fwdport.HostFileWithLock{Hosts: hosts}

	debouncer := &mockDebouncer{immediate: false} // Don't execute immediately

	svcFwd := &ServiceFWD{
		ClientSet:            clientset,
		Svc:                  svc,
		PodLabelSelector:     "app=test",
		Headless:             false,
		Context:              "test-context",
		Namespace:            namespace,
		PortForwards:         make(map[string]*fwdport.PortForwardOpts),
		NamespaceServiceLock: &sync.Mutex{},
		Hostfile:             hostFile,
		SyncDebouncer:        debouncer.debounce,
		LastSyncedAt:         time.Now(), // Recent sync
	}

	// Call without force - should be debounced
	svcFwd.SyncPodForwards(false)
	svcFwd.SyncPodForwards(false)
	svcFwd.SyncPodForwards(false)

	// Should have 3 debouncer calls
	if debouncer.getCalls() != 3 {
		t.Errorf("Expected 3 debouncer calls, got %d", debouncer.getCalls())
	}

	// But no pods forwarded yet (debounced)
	if len(svcFwd.PortForwards) != 0 {
		t.Errorf("Expected 0 pods forwarded (debounced), got %d", len(svcFwd.PortForwards))
	}
}

// TestSyncPodForwards_ForceBypassesDebouncer tests that force=true bypasses debouncing
//
//goland:noinspection DuplicatedCode
func TestSyncPodForwards_ForceBypassesDebouncer(t *testing.T) {
	cleanup := setupMockInterface()
	defer cleanup()

	restClient, restCleanup := setupMockRESTClient()
	defer restCleanup()

	namespace := "default"
	labels := map[string]string{"app": "test"}

	runningPod := createTestPod("running-pod", namespace, v1.PodRunning, labels)
	clientset := fake.NewClientset(runningPod)

	svc := createTestService("test-svc", namespace, []v1.ServicePort{
		{Port: 80, TargetPort: intstr.FromInt32(8080), Protocol: v1.ProtocolTCP},
	}, false)

	hosts, err := txeh.NewHosts(&txeh.HostsConfig{})
	if err != nil {
		t.Fatalf("Failed to create txeh.Hosts: %v", err)
	}
	hostFile := &fwdport.HostFileWithLock{Hosts: hosts}

	debouncer := &mockDebouncer{immediate: true}

	svcFwd := &ServiceFWD{
		ClientSet:            clientset,
		Svc:                  svc,
		PodLabelSelector:     "app=test",
		Headless:             false,
		Context:              "test-context",
		Namespace:            namespace,
		PortForwards:         make(map[string]*fwdport.PortForwardOpts),
		NamespaceServiceLock: &sync.Mutex{},
		Hostfile:             hostFile,
		SyncDebouncer:        debouncer.debounce,
		LastSyncedAt:         time.Now(), // Recent sync
		RESTClient:           restClient,
	}

	// Call with force=true - should bypass debouncer
	svcFwd.SyncPodForwards(true)

	// Debouncer should be replaced with no-op, so getCalls() should be 0
	// But sync should still have happened
	time.Sleep(100 * time.Millisecond) // Give time for goroutine to start

	// Lock before reading PortForwards to avoid race condition
	svcFwd.NamespaceServiceLock.Lock()
	count := len(svcFwd.PortForwards)
	svcFwd.NamespaceServiceLock.Unlock()

	if count == 0 {
		t.Error("Expected pod to be forwarded with force=true")
	}
}

// TestSyncPodForwards_ForceSyncAfter5Minutes tests automatic force sync after 5 minutes
//
//goland:noinspection DuplicatedCode
func TestSyncPodForwards_ForceSyncAfter5Minutes(t *testing.T) {
	cleanup := setupMockInterface()
	defer cleanup()

	restClient, restCleanup := setupMockRESTClient()
	defer restCleanup()

	namespace := "default"
	labels := map[string]string{"app": "test"}

	runningPod := createTestPod("running-pod", namespace, v1.PodRunning, labels)
	clientset := fake.NewClientset(runningPod)

	svc := createTestService("test-svc", namespace, []v1.ServicePort{
		{Port: 80, TargetPort: intstr.FromInt32(8080), Protocol: v1.ProtocolTCP},
	}, false)

	hosts, err := txeh.NewHosts(&txeh.HostsConfig{})
	if err != nil {
		t.Fatalf("Failed to create txeh.Hosts: %v", err)
	}
	hostFile := &fwdport.HostFileWithLock{Hosts: hosts}

	debouncer := &mockDebouncer{immediate: true}

	svcFwd := &ServiceFWD{
		ClientSet:            clientset,
		Svc:                  svc,
		PodLabelSelector:     "app=test",
		Headless:             false,
		Context:              "test-context",
		Namespace:            namespace,
		PortForwards:         make(map[string]*fwdport.PortForwardOpts),
		NamespaceServiceLock: &sync.Mutex{},
		Hostfile:             hostFile,
		SyncDebouncer:        debouncer.debounce,
		LastSyncedAt:         time.Now().Add(-6 * time.Minute), // 6 minutes ago
		RESTClient:           restClient,
	}

	// Call without force, but LastSyncedAt is >5 minutes ago
	// Should force sync anyway
	svcFwd.SyncPodForwards(false)

	time.Sleep(100 * time.Millisecond) // Give time for goroutine

	// Lock before reading PortForwards to avoid race condition
	svcFwd.NamespaceServiceLock.Lock()
	count := len(svcFwd.PortForwards)
	svcFwd.NamespaceServiceLock.Unlock()

	if count == 0 {
		t.Error("Expected pod to be forwarded after 5 minute threshold")
	}
}

// TestSyncPodForwards_RemovesStoppedPods tests that stopped pods are removed
//
//goland:noinspection DuplicatedCode
func TestSyncPodForwards_RemovesStoppedPods(t *testing.T) {
	cleanup := setupMockInterface()
	defer cleanup()

	restClient, restCleanup := setupMockRESTClient()
	defer restCleanup()

	namespace := "default"
	labels := map[string]string{"app": "test"}

	// Start with one running pod
	runningPod := createTestPod("running-pod", namespace, v1.PodRunning, labels)
	clientset := fake.NewClientset(runningPod)

	svc := createTestService("test-svc", namespace, []v1.ServicePort{
		{Port: 80, TargetPort: intstr.FromInt32(8080), Protocol: v1.ProtocolTCP},
	}, false)

	hosts, err := txeh.NewHosts(&txeh.HostsConfig{})
	if err != nil {
		t.Fatalf("Failed to create txeh.Hosts: %v", err)
	}
	hostFile := &fwdport.HostFileWithLock{Hosts: hosts}

	debouncer := &mockDebouncer{immediate: true}

	svcFwd := &ServiceFWD{
		ClientSet:            clientset,
		Svc:                  svc,
		PodLabelSelector:     "app=test",
		Headless:             false,
		Context:              "test-context",
		Namespace:            namespace,
		PortForwards:         make(map[string]*fwdport.PortForwardOpts),
		NamespaceServiceLock: &sync.Mutex{},
		Hostfile:             hostFile,
		SyncDebouncer:        debouncer.debounce,
		LastSyncedAt:         time.Now().Add(-10 * time.Minute),
		RESTClient:           restClient,
	}

	// Add a mock pod forward entry for a pod that no longer exists
	mockPfo := &fwdport.PortForwardOpts{
		PodName:        "deleted-pod",
		Service:        "test-svc",
		ManualStopChan: make(chan struct{}),
		DoneChan:       make(chan struct{}),
	}
	close(mockPfo.DoneChan) // Already stopped
	svcFwd.PortForwards["test-svc.deleted-pod"] = mockPfo

	// Sync should remove the deleted pod and add the running pod
	svcFwd.SyncPodForwards(true)

	time.Sleep(100 * time.Millisecond)

	// Lock before reading PortForwards to avoid race condition
	svcFwd.NamespaceServiceLock.Lock()
	count := len(svcFwd.PortForwards)
	_, foundDeleted := svcFwd.PortForwards["test-svc.deleted-pod"]
	svcFwd.NamespaceServiceLock.Unlock()

	// Should only have the running pod now
	if count != 1 {
		t.Errorf("Expected 1 pod after sync, got %d", count)
	}

	// Should NOT have the deleted pod
	if foundDeleted {
		t.Error("Deleted pod should have been removed from PortForwards")
	}
}

// TestSyncPodForwards_RemovesStoppedPods_FullKeyFormat is a regression test for the bug
// where stale forwards were not being cleaned up because the cleanup logic incorrectly
// compared map keys (format: "service.podname.port") against pod names ("podname").
// This test verifies that forwards with the full key format are properly cleaned up.
//
//goland:noinspection DuplicatedCode
func TestSyncPodForwards_RemovesStoppedPods_FullKeyFormat(t *testing.T) {
	cleanup := setupMockInterface()
	defer cleanup()

	restClient, restCleanup := setupMockRESTClient()
	defer restCleanup()

	namespace := "default"
	labels := map[string]string{"app": "test"}

	// Only have running-pod in k8s (deleted-pod is gone)
	runningPod := createTestPod("running-pod", namespace, v1.PodRunning, labels)
	clientset := fake.NewClientset(runningPod)

	svc := createTestService("test-svc", namespace, []v1.ServicePort{
		{Port: 80, TargetPort: intstr.FromInt32(8080), Protocol: v1.ProtocolTCP},
	}, false)

	hosts, err := txeh.NewHosts(&txeh.HostsConfig{})
	if err != nil {
		t.Fatalf("Failed to create txeh.Hosts: %v", err)
	}
	hostFile := &fwdport.HostFileWithLock{Hosts: hosts}

	debouncer := &mockDebouncer{immediate: true}

	svcFwd := &ServiceFWD{
		ClientSet:            clientset,
		Svc:                  svc,
		PodLabelSelector:     "app=test",
		Headless:             false,
		Context:              "test-context",
		Namespace:            namespace,
		PortForwards:         make(map[string]*fwdport.PortForwardOpts),
		NamespaceServiceLock: &sync.Mutex{},
		Hostfile:             hostFile,
		SyncDebouncer:        debouncer.debounce,
		LastSyncedAt:         time.Now().Add(-10 * time.Minute),
		RESTClient:           restClient,
	}

	// Add a mock pod forward entry with FULL KEY FORMAT (service.podname.port)
	// for a pod that no longer exists in k8s
	mockPfo := &fwdport.PortForwardOpts{
		PodName:        "deleted-pod",
		Service:        "test-svc",
		LocalPort:      "80",
		ManualStopChan: make(chan struct{}),
		DoneChan:       make(chan struct{}),
	}
	close(mockPfo.DoneChan) // Already stopped

	// Use the full key format: service.podname.port
	fullKey := "test-svc.deleted-pod.80"
	svcFwd.PortForwards[fullKey] = mockPfo

	// Sync should remove the deleted pod (even with full key format)
	svcFwd.SyncPodForwards(true)

	time.Sleep(100 * time.Millisecond)

	// Lock before reading PortForwards
	svcFwd.NamespaceServiceLock.Lock()
	count := len(svcFwd.PortForwards)
	_, foundDeletedByFullKey := svcFwd.PortForwards[fullKey]
	svcFwd.NamespaceServiceLock.Unlock()

	// Should only have the running pod now (1 forward)
	if count != 1 {
		t.Errorf("Expected 1 pod after sync, got %d", count)
	}

	// Should NOT have the deleted pod entry (with full key format)
	if foundDeletedByFullKey {
		t.Errorf("Deleted pod with key '%s' should have been removed from PortForwards. "+
			"This is a regression of the bug where cleanup compared map keys against pod names instead of "+
			"extracting the actual pod name from PortForwardOpts.", fullKey)
	}
}

// TestAddServicePod tests adding a pod to the PortForwards map
func TestAddServicePod(t *testing.T) {
	svcFwd := &ServiceFWD{
		PortForwards:         make(map[string]*fwdport.PortForwardOpts),
		NamespaceServiceLock: &sync.Mutex{},
	}

	pfo := &fwdport.PortForwardOpts{
		Service:   "test-svc",
		PodName:   "test-pod",
		LocalPort: "8080",
	}

	isNew := svcFwd.AddServicePod(pfo)

	// Should return true for new pod
	if !isNew {
		t.Error("AddServicePod should return true for new pod")
	}

	// Should be in map with key "service.podname.localport"
	if _, found := svcFwd.PortForwards["test-svc.test-pod.8080"]; !found {
		t.Error("Pod was not added to PortForwards map")
	}

	if len(svcFwd.PortForwards) != 1 {
		t.Errorf("Expected 1 entry in PortForwards, got %d", len(svcFwd.PortForwards))
	}
}

// TestAddServicePod_Duplicate tests that adding same pod twice doesn't create duplicates
func TestAddServicePod_Duplicate(t *testing.T) {
	svcFwd := &ServiceFWD{
		PortForwards:         make(map[string]*fwdport.PortForwardOpts),
		NamespaceServiceLock: &sync.Mutex{},
	}

	pfo1 := &fwdport.PortForwardOpts{
		Service:   "test-svc",
		PodName:   "test-pod",
		LocalPort: "8080",
	}

	pfo2 := &fwdport.PortForwardOpts{
		Service:   "test-svc",
		PodName:   "test-pod",
		LocalPort: "8080",
	}

	isNew1 := svcFwd.AddServicePod(pfo1)
	isNew2 := svcFwd.AddServicePod(pfo2)

	// First should return true, second should return false
	if !isNew1 {
		t.Error("First AddServicePod should return true")
	}
	if isNew2 {
		t.Error("Second AddServicePod should return false for duplicate")
	}

	// Should only have one entry (second one doesn't add duplicate)
	if len(svcFwd.PortForwards) != 1 {
		t.Errorf("Expected 1 entry (no duplicates), got %d", len(svcFwd.PortForwards))
	}
}

// TestAddServicePod_ConcurrentDuplicates tests that concurrent adds of the same pod
// only result in one entry and only one returns true (the winner of the race)
func TestAddServicePod_ConcurrentDuplicates(t *testing.T) {
	svcFwd := &ServiceFWD{
		PortForwards:         make(map[string]*fwdport.PortForwardOpts),
		NamespaceServiceLock: &sync.Mutex{},
	}

	var wg sync.WaitGroup
	numGoroutines := 50
	successCount := 0
	var mu sync.Mutex

	// All goroutines try to add the SAME pod
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pfo := &fwdport.PortForwardOpts{
				Service:        "test-svc",
				PodName:        "same-pod", // Same pod for all
				LocalPort:      "8080",
				ManualStopChan: make(chan struct{}),
				DoneChan:       make(chan struct{}),
			}
			if svcFwd.AddServicePod(pfo) {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// Only ONE should succeed (win the race)
	if successCount != 1 {
		t.Errorf("Expected exactly 1 successful AddServicePod for same pod, got %d", successCount)
	}

	// Should only have one entry
	svcFwd.NamespaceServiceLock.Lock()
	count := len(svcFwd.PortForwards)
	svcFwd.NamespaceServiceLock.Unlock()

	if count != 1 {
		t.Errorf("Expected 1 entry in PortForwards, got %d", count)
	}
}

// TestListServicePodNames tests listing pod names
func TestListServicePodNames(t *testing.T) {
	svcFwd := &ServiceFWD{
		PortForwards:         make(map[string]*fwdport.PortForwardOpts),
		NamespaceServiceLock: &sync.Mutex{},
	}

	pfo1 := &fwdport.PortForwardOpts{Service: "svc", PodName: "pod1", LocalPort: "80"}
	pfo2 := &fwdport.PortForwardOpts{Service: "svc", PodName: "pod2", LocalPort: "80"}

	svcFwd.AddServicePod(pfo1)
	svcFwd.AddServicePod(pfo2)

	names := svcFwd.ListServicePodNames()

	if len(names) != 2 {
		t.Errorf("Expected 2 pod names, got %d", len(names))
	}

	// Should contain both keys (format: service.podname.localport)
	foundPod1 := false
	foundPod2 := false
	for _, name := range names {
		if name == "svc.pod1.80" {
			foundPod1 = true
		}
		if name == "svc.pod2.80" {
			foundPod2 = true
		}
	}

	if !foundPod1 || !foundPod2 {
		t.Error("ListServicePodNames did not return expected pod names")
	}
}

// TestRemoveServicePod tests removing a pod from forwarding
func TestRemoveServicePod(t *testing.T) {
	svcFwd := &ServiceFWD{
		PortForwards:         make(map[string]*fwdport.PortForwardOpts),
		NamespaceServiceLock: &sync.Mutex{},
	}

	pfo := &fwdport.PortForwardOpts{
		Service:        "test-svc",
		PodName:        "test-pod",
		LocalPort:      "8080",
		ManualStopChan: make(chan struct{}),
		DoneChan:       make(chan struct{}),
	}

	// Simulate the done channel being closed (pod forwarding finished)
	go func() {
		<-pfo.ManualStopChan
		close(pfo.DoneChan)
	}()

	svcFwd.AddServicePod(pfo)

	// Verify it was added
	if len(svcFwd.PortForwards) != 1 {
		t.Fatal("Pod was not added")
	}

	// Remove it (key format: service.podname.localport)
	svcFwd.RemoveServicePod("test-svc.test-pod.8080")

	// Should be removed from map
	if len(svcFwd.PortForwards) != 0 {
		t.Errorf("Expected 0 pods after removal, got %d", len(svcFwd.PortForwards))
	}

	// ManualStopChan should be closed
	select {
	case <-pfo.ManualStopChan:
		// Good - channel was closed
	default:
		t.Error("ManualStopChan was not closed")
	}
}

// TestRemoveServicePod_NonExistent tests removing a pod that doesn't exist
func TestRemoveServicePod_NonExistent(t *testing.T) {
	svcFwd := &ServiceFWD{
		PortForwards:         make(map[string]*fwdport.PortForwardOpts),
		NamespaceServiceLock: &sync.Mutex{},
	}

	// Should not panic when removing non-existent pod
	svcFwd.RemoveServicePod("nonexistent.pod")

	if len(svcFwd.PortForwards) != 0 {
		t.Errorf("Expected 0 pods, got %d", len(svcFwd.PortForwards))
	}
}

// TestString tests the String() method
func TestString(t *testing.T) {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-service",
			Namespace: "my-namespace",
		},
	}

	svcFwd := &ServiceFWD{
		Svc:       svc,
		Namespace: "my-namespace",
		Context:   "my-context",
	}

	expected := "my-service.my-namespace.my-context"
	if svcFwd.String() != expected {
		t.Errorf("Expected %s, got %s", expected, svcFwd.String())
	}
}

// TestGetPortMap tests port mapping functionality
func TestGetPortMap(t *testing.T) {
	portMap := []PortMap{
		{SourcePort: "80", TargetPort: "8080"},
		{SourcePort: "443", TargetPort: "8443"},
	}

	svcFwd := &ServiceFWD{
		PortMap: &portMap,
	}

	// Test mapped port
	result := svcFwd.getPortMap(80)
	if result != "8080" {
		t.Errorf("Expected mapped port 8080, got %s", result)
	}

	result = svcFwd.getPortMap(443)
	if result != "8443" {
		t.Errorf("Expected mapped port 8443, got %s", result)
	}

	// Test unmapped port (should return same port)
	result = svcFwd.getPortMap(9000)
	if result != "9000" {
		t.Errorf("Expected unmapped port 9000, got %s", result)
	}
}

// TestGetPortMap_Nil tests port mapping with nil PortMap
func TestGetPortMap_Nil(t *testing.T) {
	svcFwd := &ServiceFWD{
		PortMap: nil,
	}

	// Should return the same port when no mapping exists
	result := svcFwd.getPortMap(8080)
	if result != "8080" {
		t.Errorf("Expected port 8080, got %s", result)
	}
}

// TestPortSearch tests searching for named container ports
func TestPortSearch(t *testing.T) {
	containers := []v1.Container{
		{
			Name: "container1",
			Ports: []v1.ContainerPort{
				{Name: "http", ContainerPort: 8080},
				{Name: "metrics", ContainerPort: 9090},
			},
		},
		{
			Name: "container2",
			Ports: []v1.ContainerPort{
				{Name: "grpc", ContainerPort: 50051},
			},
		},
	}

	// Test finding existing port
	port, containerName, found := portSearch("http", containers)
	if !found {
		t.Error("Expected to find 'http' port")
	}
	if port != "8080" {
		t.Errorf("Expected port 8080, got %s", port)
	}
	if containerName != "container1" {
		t.Errorf("Expected container container1, got %s", containerName)
	}

	// Test finding port in second container
	port, containerName, found = portSearch("grpc", containers)
	if !found {
		t.Error("Expected to find 'grpc' port")
	}
	if port != "50051" {
		t.Errorf("Expected port 50051, got %s", port)
	}
	if containerName != "container2" {
		t.Errorf("Expected container container2, got %s", containerName)
	}

	// Test non-existent port
	_, _, found = portSearch("nonexistent", containers)
	if found {
		t.Error("Should not find 'nonexistent' port")
	}
}

// TestFindContainerForPort tests finding container by numeric port
func TestFindContainerForPort(t *testing.T) {
	containers := []v1.Container{
		{
			Name: "web",
			Ports: []v1.ContainerPort{
				{ContainerPort: 8080},
				{ContainerPort: 8443},
			},
		},
		{
			Name: "sidecar",
			Ports: []v1.ContainerPort{
				{ContainerPort: 9090},
			},
		},
	}

	// Test finding port in first container
	containerName := findContainerForPort(8080, containers)
	if containerName != "web" {
		t.Errorf("Expected container 'web', got '%s'", containerName)
	}

	// Test finding port in second container
	containerName = findContainerForPort(9090, containers)
	if containerName != "sidecar" {
		t.Errorf("Expected container 'sidecar', got '%s'", containerName)
	}

	// Test non-existent port - should return first container
	containerName = findContainerForPort(12345, containers)
	if containerName != "web" {
		t.Errorf("Expected first container 'web' for non-existent port, got '%s'", containerName)
	}

	// Test empty container list
	containerName = findContainerForPort(8080, []v1.Container{})
	if containerName != "" {
		t.Errorf("Expected empty string for empty container list, got '%s'", containerName)
	}
}

// TestConcurrentAddRemovePods tests thread safety of pod management
func TestConcurrentAddRemovePods(t *testing.T) {
	svcFwd := &ServiceFWD{
		PortForwards:         make(map[string]*fwdport.PortForwardOpts),
		NamespaceServiceLock: &sync.Mutex{},
	}

	var wg sync.WaitGroup
	numGoroutines := 10

	// Concurrently add pods
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			pfo := &fwdport.PortForwardOpts{
				Service:        "svc",
				PodName:        string(rune('a' + n)),
				LocalPort:      "80",
				ManualStopChan: make(chan struct{}),
				DoneChan:       make(chan struct{}),
			}
			svcFwd.AddServicePod(pfo)
		}(i)
	}

	wg.Wait()

	// Should have all pods
	svcFwd.NamespaceServiceLock.Lock()
	numAdded := len(svcFwd.PortForwards)
	svcFwd.NamespaceServiceLock.Unlock()

	if numAdded != numGoroutines {
		t.Errorf("Expected %d pods, got %d", numGoroutines, numAdded)
	}

	// Concurrently list and remove
	for i := 0; i < numGoroutines; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			svcFwd.ListServicePodNames()
		}()
		go func(n int) {
			defer wg.Done()
			// Key format: service.podname.localport
			podKey := "svc." + string(rune('a'+n)) + ".80"
			// Close DoneChan before removing to simulate stopped forwarding
			svcFwd.NamespaceServiceLock.Lock()
			pfo, found := svcFwd.PortForwards[podKey]
			svcFwd.NamespaceServiceLock.Unlock()
			if found {
				close(pfo.DoneChan)
				svcFwd.RemoveServicePod(podKey)
			}
		}(i)
	}

	wg.Wait()

	// All should be removed
	svcFwd.NamespaceServiceLock.Lock()
	numRemaining := len(svcFwd.PortForwards)
	svcFwd.NamespaceServiceLock.Unlock()

	if numRemaining != 0 {
		t.Errorf("Expected 0 pods after removal, got %d", numRemaining)
	}
}

// ============================================================================
// Auto-Reconnect Logic Tests
// ============================================================================

// TestScheduleReconnect_DisabledByDefault tests that auto-reconnect is disabled when not enabled
func TestScheduleReconnect_DisabledByDefault(t *testing.T) {
	svc := createTestService("test-svc", "default", []v1.ServicePort{
		{Port: 80},
	}, false)

	svcFwd := &ServiceFWD{
		Svc:           svc,
		AutoReconnect: false, // Disabled
		DoneChannel:   make(chan struct{}),
	}

	result := svcFwd.scheduleReconnect()

	if result {
		t.Error("scheduleReconnect should return false when AutoReconnect is disabled")
	}
}

// TestScheduleReconnect_EnabledTriggersReconnect tests that auto-reconnect schedules reconnection
func TestScheduleReconnect_EnabledTriggersReconnect(t *testing.T) {
	cleanup := setupMockInterface()

	namespace := "default"
	labels := map[string]string{"app": "test"}

	runningPod := createTestPod("running-pod", namespace, v1.PodRunning, labels)
	clientset := fake.NewClientset(runningPod)

	svc := createTestService("test-svc", namespace, []v1.ServicePort{
		{Port: 80},
	}, false)

	hosts, _ := txeh.NewHosts(&txeh.HostsConfig{})
	hostFile := &fwdport.HostFileWithLock{Hosts: hosts}

	debouncer := &mockDebouncer{immediate: true}
	doneChannel := make(chan struct{})

	svcFwd := &ServiceFWD{
		ClientSet:            clientset,
		Svc:                  svc,
		PodLabelSelector:     "app=test",
		Namespace:            namespace,
		Context:              "test-context",
		PortForwards:         make(map[string]*fwdport.PortForwardOpts),
		NamespaceServiceLock: &sync.Mutex{},
		Hostfile:             hostFile,
		SyncDebouncer:        debouncer.debounce,
		AutoReconnect:        true,
		DoneChannel:          doneChannel,
	}

	result := svcFwd.scheduleReconnect()

	if !result {
		t.Error("scheduleReconnect should return true when AutoReconnect is enabled")
	}

	// Wait a bit then close DoneChannel to stop the reconnect goroutine before cleanup
	time.Sleep(100 * time.Millisecond)
	close(doneChannel)
	// Give goroutine time to see the closed channel
	time.Sleep(50 * time.Millisecond)

	// Now safe to cleanup
	cleanup()
}

// TestScheduleReconnect_ExponentialBackoff tests that backoff increases exponentially
func TestScheduleReconnect_ExponentialBackoff(t *testing.T) {
	svc := createTestService("test-svc", "default", []v1.ServicePort{
		{Port: 80},
	}, false)

	doneChannel := make(chan struct{})
	defer close(doneChannel) // Stop all goroutines spawned by scheduleReconnect

	svcFwd := &ServiceFWD{
		Svc:                  svc,
		AutoReconnect:        true,
		DoneChannel:          doneChannel,
		NamespaceServiceLock: &sync.Mutex{},
		PortForwards:         make(map[string]*fwdport.PortForwardOpts),
	}

	// First call should set backoff to initial (1s) and double it for next
	svcFwd.scheduleReconnect()

	svcFwd.reconnectMu.Lock()
	firstBackoff := svcFwd.reconnectBackoff
	svcFwd.reconnecting = false // Reset so we can call again
	svcFwd.reconnectMu.Unlock()

	if firstBackoff != 2*time.Second {
		t.Errorf("Expected backoff to be 2s after first call, got %v", firstBackoff)
	}

	// Second call should double again
	svcFwd.scheduleReconnect()

	svcFwd.reconnectMu.Lock()
	secondBackoff := svcFwd.reconnectBackoff
	svcFwd.reconnecting = false
	svcFwd.reconnectMu.Unlock()

	if secondBackoff != 4*time.Second {
		t.Errorf("Expected backoff to be 4s after second call, got %v", secondBackoff)
	}

	// Verify backoff caps at maxReconnectBackoff (5 minutes)
	svcFwd.reconnectMu.Lock()
	svcFwd.reconnectBackoff = 4 * time.Minute
	svcFwd.reconnectMu.Unlock()

	svcFwd.scheduleReconnect()

	svcFwd.reconnectMu.Lock()
	cappedBackoff := svcFwd.reconnectBackoff
	svcFwd.reconnectMu.Unlock()

	if cappedBackoff != 5*time.Minute {
		t.Errorf("Expected backoff to be capped at 5m, got %v", cappedBackoff)
	}
}

// TestScheduleReconnect_AlreadyReconnecting tests that duplicate reconnects are prevented
func TestScheduleReconnect_AlreadyReconnecting(t *testing.T) {
	svc := createTestService("test-svc", "default", []v1.ServicePort{
		{Port: 80},
	}, false)

	doneChannel := make(chan struct{})
	defer close(doneChannel) // Stop all goroutines spawned by scheduleReconnect

	svcFwd := &ServiceFWD{
		Svc:                  svc,
		AutoReconnect:        true,
		DoneChannel:          doneChannel,
		NamespaceServiceLock: &sync.Mutex{},
		PortForwards:         make(map[string]*fwdport.PortForwardOpts),
	}

	// First call starts reconnecting
	result1 := svcFwd.scheduleReconnect()
	if !result1 {
		t.Error("First scheduleReconnect should succeed")
	}

	// Second call while still reconnecting should be rejected
	result2 := svcFwd.scheduleReconnect()
	if result2 {
		t.Error("Second scheduleReconnect should be rejected while reconnecting")
	}
}

// TestScheduleReconnect_ShuttingDown tests that reconnect is prevented during shutdown
func TestScheduleReconnect_ShuttingDown(t *testing.T) {
	svc := createTestService("test-svc", "default", []v1.ServicePort{
		{Port: 80},
	}, false)

	doneChannel := make(chan struct{})
	close(doneChannel) // Simulate shutdown

	svcFwd := &ServiceFWD{
		Svc:           svc,
		AutoReconnect: true,
		DoneChannel:   doneChannel,
	}

	result := svcFwd.scheduleReconnect()

	if result {
		t.Error("scheduleReconnect should return false when service is shutting down")
	}
}

// TestResetReconnectBackoff tests that backoff is reset after successful connection
func TestResetReconnectBackoff(t *testing.T) {
	svc := createTestService("test-svc", "default", []v1.ServicePort{
		{Port: 80},
	}, false)

	svcFwd := &ServiceFWD{
		Svc: svc,
	}

	// Set some backoff
	svcFwd.reconnectMu.Lock()
	svcFwd.reconnectBackoff = 2 * time.Minute
	svcFwd.reconnectMu.Unlock()

	// Reset backoff
	svcFwd.ResetReconnectBackoff()

	svcFwd.reconnectMu.Lock()
	backoff := svcFwd.reconnectBackoff
	svcFwd.reconnectMu.Unlock()

	if backoff != 0 {
		t.Errorf("Expected backoff to be reset to 0, got %v", backoff)
	}
}

// TestForceReconnect_ResetsState tests that ForceReconnect resets all reconnect state
func TestForceReconnect_ResetsState(t *testing.T) {
	cleanup := setupMockInterface()
	defer cleanup()

	namespace := "default"
	labels := map[string]string{"app": "test"}

	runningPod := createTestPod("running-pod", namespace, v1.PodRunning, labels)
	clientset := fake.NewClientset(runningPod)

	svc := createTestService("test-svc", namespace, []v1.ServicePort{
		{Port: 80},
	}, false)

	hosts, _ := txeh.NewHosts(&txeh.HostsConfig{})
	hostFile := &fwdport.HostFileWithLock{Hosts: hosts}

	debouncer := &mockDebouncer{immediate: true}

	svcFwd := &ServiceFWD{
		ClientSet:            clientset,
		Svc:                  svc,
		PodLabelSelector:     "app=test",
		Namespace:            namespace,
		Context:              "test-context",
		PortForwards:         make(map[string]*fwdport.PortForwardOpts),
		NamespaceServiceLock: &sync.Mutex{},
		Hostfile:             hostFile,
		SyncDebouncer:        debouncer.debounce,
		DoneChannel:          make(chan struct{}),
	}

	// Set some state
	svcFwd.reconnectMu.Lock()
	svcFwd.reconnectBackoff = 2 * time.Minute
	svcFwd.reconnecting = true
	svcFwd.reconnectMu.Unlock()

	// Force reconnect
	svcFwd.ForceReconnect()

	// Verify state was reset
	svcFwd.reconnectMu.Lock()
	backoff := svcFwd.reconnectBackoff
	reconnecting := svcFwd.reconnecting
	svcFwd.reconnectMu.Unlock()

	if backoff != 0 {
		t.Errorf("Expected backoff to be reset to 0, got %v", backoff)
	}

	if reconnecting {
		t.Error("Expected reconnecting to be reset to false")
	}
}

// TestStopAllPortForwards tests that all port forwards are stopped and map is cleared
func TestStopAllPortForwards(t *testing.T) {
	svc := createTestService("test-svc", "default", []v1.ServicePort{
		{Port: 80},
	}, false)

	svcFwd := &ServiceFWD{
		Svc:                  svc,
		PortForwards:         make(map[string]*fwdport.PortForwardOpts),
		NamespaceServiceLock: &sync.Mutex{},
		DoneChannel:          make(chan struct{}),
	}

	// Add some port forwards
	for i := 0; i < 5; i++ {
		pfo := &fwdport.PortForwardOpts{
			Service:        "test-svc",
			PodName:        string(rune('a' + i)),
			LocalPort:      "80",
			ManualStopChan: make(chan struct{}),
			DoneChan:       make(chan struct{}),
		}
		key := "test-svc." + string(rune('a'+i)) + ".80"
		svcFwd.PortForwards[key] = pfo
	}

	// Verify we have 5 forwards
	svcFwd.NamespaceServiceLock.Lock()
	initialCount := len(svcFwd.PortForwards)
	svcFwd.NamespaceServiceLock.Unlock()

	if initialCount != 5 {
		t.Fatalf("Expected 5 port forwards, got %d", initialCount)
	}

	// Stop all
	svcFwd.StopAllPortForwards()

	// Verify map is cleared
	svcFwd.NamespaceServiceLock.Lock()
	finalCount := len(svcFwd.PortForwards)
	svcFwd.NamespaceServiceLock.Unlock()

	if finalCount != 0 {
		t.Errorf("Expected 0 port forwards after StopAllPortForwards, got %d", finalCount)
	}
}

// TestIsPodReady tests the isPodReady function
func TestIsPodReady(t *testing.T) {
	tests := []struct {
		name     string
		pod      *v1.Pod
		expected bool
	}{
		{
			name: "pending pod is ready",
			pod: &v1.Pod{
				Status: v1.PodStatus{
					Phase: v1.PodPending,
				},
			},
			expected: true,
		},
		{
			name: "running pod with ready container",
			pod: &v1.Pod{
				Status: v1.PodStatus{
					Phase: v1.PodRunning,
					ContainerStatuses: []v1.ContainerStatus{
						{Ready: true},
					},
				},
			},
			expected: true,
		},
		{
			name: "running pod with no ready containers",
			pod: &v1.Pod{
				Status: v1.PodStatus{
					Phase: v1.PodRunning,
					ContainerStatuses: []v1.ContainerStatus{
						{Ready: false},
					},
				},
			},
			expected: false,
		},
		{
			name: "running pod with empty container statuses",
			pod: &v1.Pod{
				Status: v1.PodStatus{
					Phase:             v1.PodRunning,
					ContainerStatuses: []v1.ContainerStatus{},
				},
			},
			expected: false,
		},
		{
			name: "running pod with multiple containers one ready",
			pod: &v1.Pod{
				Status: v1.PodStatus{
					Phase: v1.PodRunning,
					ContainerStatuses: []v1.ContainerStatus{
						{Ready: false},
						{Ready: true},
						{Ready: false},
					},
				},
			},
			expected: true,
		},
		{
			name: "running pod with multiple containers none ready",
			pod: &v1.Pod{
				Status: v1.PodStatus{
					Phase: v1.PodRunning,
					ContainerStatuses: []v1.ContainerStatus{
						{Ready: false},
						{Ready: false},
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isPodReady(tt.pod)
			if result != tt.expected {
				t.Errorf("isPodReady() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestCloseIdleHTTPConnections tests that the method doesn't panic
func TestCloseIdleHTTPConnections(t *testing.T) {
	svc := createTestService("test-svc", "default", []v1.ServicePort{
		{Port: 80},
	}, false)

	svcFwd := &ServiceFWD{
		Svc: svc,
	}

	// Should not panic even with nil transport
	svcFwd.CloseIdleHTTPConnections()
}

// mockTransportWithIdleCloser implements http.RoundTripper and CloseIdleConnections
type mockTransportWithIdleCloser struct {
	closeIdleCalled bool
}

func (m *mockTransportWithIdleCloser) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, nil
}

func (m *mockTransportWithIdleCloser) CloseIdleConnections() {
	m.closeIdleCalled = true
}

// mockTransportWithoutIdleCloser only implements http.RoundTripper
type mockTransportWithoutIdleCloser struct{}

func (m *mockTransportWithoutIdleCloser) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, nil
}

// TestCloseIdleHTTPConnections_WithIdleCloser tests that CloseIdleConnections is called
func TestCloseIdleHTTPConnections_WithIdleCloser(t *testing.T) {
	svc := createTestService("test-svc", "default", []v1.ServicePort{
		{Port: 80},
	}, false)

	mockTransport := &mockTransportWithIdleCloser{}

	svcFwd := &ServiceFWD{
		Svc: svc,
		ClientConfig: restclient.Config{
			Transport: mockTransport,
		},
	}

	svcFwd.CloseIdleHTTPConnections()

	if !mockTransport.closeIdleCalled {
		t.Error("Expected CloseIdleConnections to be called on transport")
	}
}

// TestCloseIdleHTTPConnections_WithoutIdleCloser tests transport without CloseIdleConnections
func TestCloseIdleHTTPConnections_WithoutIdleCloser(t *testing.T) {
	svc := createTestService("test-svc", "default", []v1.ServicePort{
		{Port: 80},
	}, false)

	mockTransport := &mockTransportWithoutIdleCloser{}

	svcFwd := &ServiceFWD{
		Svc: svc,
		ClientConfig: restclient.Config{
			Transport: mockTransport,
		},
	}

	// Should not panic when transport doesn't implement CloseIdleConnections
	svcFwd.CloseIdleHTTPConnections()
}

// TestScheduleReconnect_Concurrent tests thread safety of scheduleReconnect
func TestScheduleReconnect_Concurrent(t *testing.T) {
	svc := createTestService("test-svc", "default", []v1.ServicePort{
		{Port: 80},
	}, false)

	doneChannel := make(chan struct{})
	defer close(doneChannel) // Stop all goroutines spawned by scheduleReconnect

	svcFwd := &ServiceFWD{
		Svc:                  svc,
		AutoReconnect:        true,
		DoneChannel:          doneChannel,
		NamespaceServiceLock: &sync.Mutex{},
		PortForwards:         make(map[string]*fwdport.PortForwardOpts),
	}

	var wg sync.WaitGroup
	numGoroutines := 20
	successCount := 0
	var mu sync.Mutex

	// Call scheduleReconnect from many goroutines
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if svcFwd.scheduleReconnect() {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// Only one should succeed (the first one)
	if successCount != 1 {
		t.Errorf("Expected exactly 1 successful scheduleReconnect, got %d", successCount)
	}
}

// TestForceReconnect_ClearsPortForwardsBeforeSync tests that ForceReconnect clears port forwards
func TestForceReconnect_ClearsPortForwardsBeforeSync(t *testing.T) {
	cleanup := setupMockInterface()
	defer cleanup()

	namespace := "default"
	labels := map[string]string{"app": "test"}

	runningPod := createTestPod("running-pod", namespace, v1.PodRunning, labels)
	clientset := fake.NewClientset(runningPod)

	svc := createTestService("test-svc", namespace, []v1.ServicePort{
		{Port: 80},
	}, false)

	hosts, _ := txeh.NewHosts(&txeh.HostsConfig{})
	hostFile := &fwdport.HostFileWithLock{Hosts: hosts}

	syncCalled := false
	var syncCalledWithForwardsCleared bool
	var forwardsAtSyncTime int

	// Create svcFwd first without the debouncer
	svcFwd := &ServiceFWD{
		ClientSet:            clientset,
		Svc:                  svc,
		PodLabelSelector:     "app=test",
		Namespace:            namespace,
		Context:              "test-context",
		PortForwards:         make(map[string]*fwdport.PortForwardOpts),
		NamespaceServiceLock: &sync.Mutex{},
		Hostfile:             hostFile,
		DoneChannel:          make(chan struct{}),
	}

	// Now set the debouncer that captures svcFwd
	svcFwd.SyncDebouncer = func(f func()) {
		syncCalled = true
		// Check if port forwards were cleared before sync
		svcFwd.NamespaceServiceLock.Lock()
		forwardsAtSyncTime = len(svcFwd.PortForwards)
		syncCalledWithForwardsCleared = forwardsAtSyncTime == 0
		svcFwd.NamespaceServiceLock.Unlock()
		f()
	}

	// Add a port forward
	pfo := &fwdport.PortForwardOpts{
		Service:        "test-svc",
		PodName:        "old-pod",
		LocalPort:      "80",
		ManualStopChan: make(chan struct{}),
		DoneChan:       make(chan struct{}),
	}
	svcFwd.PortForwards["test-svc.old-pod.80"] = pfo

	// Force reconnect
	svcFwd.ForceReconnect()

	if !syncCalled {
		t.Error("Expected SyncPodForwards to be called")
	}

	if !syncCalledWithForwardsCleared {
		t.Errorf("Expected port forwards to be cleared before sync was called, found %d", forwardsAtSyncTime)
	}
}

// ============================================================================
// TUI Event Emission Tests
// ============================================================================

// TestAddServicePod_EmitsPodAdded verifies that AddServicePod emits a PodAdded event
func TestAddServicePod_EmitsPodAdded(t *testing.T) {
	// Enable TUI test mode with event collection
	collector, cleanup := fwdtui.EnableTestMode()
	defer cleanup()

	svc := createTestService("test-svc", "default", []v1.ServicePort{
		{Port: 80},
	}, false)

	svcFwd := &ServiceFWD{
		Svc:                  svc,
		Namespace:            "default",
		Context:              "test-context",
		PortForwards:         make(map[string]*fwdport.PortForwardOpts),
		NamespaceServiceLock: &sync.Mutex{},
	}

	pfo := &fwdport.PortForwardOpts{
		Service:   "test-svc",
		PodName:   "test-pod-abc123",
		Namespace: "default",
		Context:   "test-context",
		LocalPort: "8080",
	}

	// Clear collector before the action we want to test (avoid pollution from parallel tests)
	collector.Clear()

	// Add the pod
	isNew := svcFwd.AddServicePod(pfo)

	if !isNew {
		t.Fatal("Expected AddServicePod to return true for new pod")
	}

	// Wait for event to be processed (async)
	if !collector.WaitForEventType(events.PodAdded, 1, 500*time.Millisecond) {
		t.Fatal("Expected PodAdded event to be emitted")
	}

	// Filter events for this specific service (extra safety against parallel test pollution)
	allPodAddedEvents := collector.EventsOfType(events.PodAdded)
	var podAddedEvents []events.Event
	for _, e := range allPodAddedEvents {
		if e.Service == "test-svc" && e.PodName == "test-pod-abc123" {
			podAddedEvents = append(podAddedEvents, e)
		}
	}

	if len(podAddedEvents) != 1 {
		t.Fatalf("Expected 1 PodAdded event for test-svc/test-pod-abc123, got %d", len(podAddedEvents))
	}

	event := podAddedEvents[0]
	if event.Namespace != "default" {
		t.Errorf("Expected Namespace 'default', got '%s'", event.Namespace)
	}
	if event.LocalPort != "8080" {
		t.Errorf("Expected LocalPort '8080', got '%s'", event.LocalPort)
	}
}

// TestAddServicePod_DuplicateDoesNotEmit verifies duplicate adds don't emit events
func TestAddServicePod_DuplicateDoesNotEmit(t *testing.T) {
	collector, cleanup := fwdtui.EnableTestMode()
	defer cleanup()

	svc := createTestService("test-svc", "default", []v1.ServicePort{
		{Port: 80},
	}, false)

	svcFwd := &ServiceFWD{
		Svc:                  svc,
		Namespace:            "default",
		Context:              "test-context",
		PortForwards:         make(map[string]*fwdport.PortForwardOpts),
		NamespaceServiceLock: &sync.Mutex{},
	}

	pfo1 := &fwdport.PortForwardOpts{
		Service:   "test-svc",
		PodName:   "test-pod",
		Namespace: "default",
		Context:   "test-context",
		LocalPort: "8080",
	}

	pfo2 := &fwdport.PortForwardOpts{
		Service:   "test-svc",
		PodName:   "test-pod",
		Namespace: "default",
		Context:   "test-context",
		LocalPort: "8080",
	}

	// Clear collector before the action we want to test (avoid pollution from parallel tests)
	collector.Clear()

	// Add first pod
	svcFwd.AddServicePod(pfo1)
	// Try to add duplicate
	isNew := svcFwd.AddServicePod(pfo2)

	if isNew {
		t.Error("Expected AddServicePod to return false for duplicate")
	}

	// Wait a bit for any events
	time.Sleep(100 * time.Millisecond)

	// Filter events for this specific service (extra safety against parallel test pollution)
	allPodAddedEvents := collector.EventsOfType(events.PodAdded)
	var podAddedEvents []events.Event
	for _, e := range allPodAddedEvents {
		if e.Service == "test-svc" && e.PodName == "test-pod" {
			podAddedEvents = append(podAddedEvents, e)
		}
	}

	// Should only have 1 PodAdded event (from first add)
	if len(podAddedEvents) != 1 {
		t.Errorf("Expected 1 PodAdded event for test-svc/test-pod (no duplicate), got %d", len(podAddedEvents))
	}
}

// TestStopAllPortForwards_EmitsPodRemoved verifies that StopAllPortForwards emits
// PodRemoved events for each stopped forward
func TestStopAllPortForwards_EmitsPodRemoved(t *testing.T) {
	collector, cleanup := fwdtui.EnableTestMode()
	defer cleanup()

	svc := createTestService("test-svc", "default", []v1.ServicePort{
		{Port: 80},
	}, false)

	svcFwd := &ServiceFWD{
		Svc:                  svc,
		Namespace:            "default",
		Context:              "test-context",
		PortForwards:         make(map[string]*fwdport.PortForwardOpts),
		NamespaceServiceLock: &sync.Mutex{},
		DoneChannel:          make(chan struct{}),
	}

	// Add multiple port forwards directly to the map
	numForwards := 3
	for i := 0; i < numForwards; i++ {
		pfo := &fwdport.PortForwardOpts{
			Service:        "test-svc",
			PodName:        "pod-" + string(rune('a'+i)),
			Namespace:      "default",
			Context:        "test-context",
			LocalPort:      "80",
			ManualStopChan: make(chan struct{}),
			DoneChan:       make(chan struct{}),
		}
		key := "test-svc.pod-" + string(rune('a'+i)) + ".80"
		svcFwd.PortForwards[key] = pfo
	}

	// Clear collector before the action we want to test (avoid pollution from parallel tests)
	collector.Clear()

	// Stop all port forwards
	svcFwd.StopAllPortForwards()

	// Wait for events to be processed
	if !collector.WaitForEventType(events.PodRemoved, numForwards, 500*time.Millisecond) {
		t.Fatalf("Expected %d PodRemoved events, got %d", numForwards, collector.CountOfType(events.PodRemoved))
	}

	// Filter events for this specific test's pods (extra safety against parallel test pollution)
	// This test uses pod-a, pod-b, pod-c
	validPods := map[string]bool{"pod-a": true, "pod-b": true, "pod-c": true}
	allPodRemovedEvents := collector.EventsOfType(events.PodRemoved)
	var podRemovedEvents []events.Event
	for _, e := range allPodRemovedEvents {
		if e.Service == "test-svc" && validPods[e.PodName] {
			podRemovedEvents = append(podRemovedEvents, e)
		}
	}

	if len(podRemovedEvents) != numForwards {
		t.Errorf("Expected %d PodRemoved events for 'test-svc', got %d", numForwards, len(podRemovedEvents))
	}

	// Verify each event has correct service info including LocalPort
	for _, event := range podRemovedEvents {
		if event.Namespace != "default" {
			t.Errorf("Expected Namespace 'default', got '%s'", event.Namespace)
		}
		// Critical: LocalPort must be set for TUI to match the removal key
		if event.LocalPort != "80" {
			t.Errorf("Expected LocalPort '80', got '%s'", event.LocalPort)
		}
	}
}

func TestBuildServiceHostname(t *testing.T) {
	tests := []struct {
		name       string
		baseName   string
		namespace  string
		context    string
		namespaceN int
		clusterN   int
		want       string
	}{
		{
			name:       "first namespace first cluster",
			baseName:   "my-service",
			namespace:  "default",
			context:    "minikube",
			namespaceN: 0,
			clusterN:   0,
			want:       "my-service",
		},
		{
			name:       "second namespace first cluster",
			baseName:   "my-service",
			namespace:  "kube-system",
			context:    "minikube",
			namespaceN: 1,
			clusterN:   0,
			want:       "my-service.kube-system",
		},
		{
			name:       "first namespace second cluster",
			baseName:   "my-service",
			namespace:  "default",
			context:    "production",
			namespaceN: 0,
			clusterN:   1,
			want:       "my-service.production",
		},
		{
			name:       "second namespace second cluster",
			baseName:   "my-service",
			namespace:  "staging",
			context:    "prod-cluster",
			namespaceN: 1,
			clusterN:   1,
			want:       "my-service.staging.prod-cluster",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildServiceHostname(tt.baseName, tt.namespace, tt.context, tt.namespaceN, tt.clusterN)
			if got != tt.want {
				t.Errorf("buildServiceHostname() = %q, want %q", got, tt.want)
			}
		})
	}
}
