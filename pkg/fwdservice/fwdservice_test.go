package fwdservice

import (
	"sync"
	"testing"
	"time"

	"github.com/txn2/kubefwd/pkg/fwdport"
	"github.com/txn2/txeh"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
)

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

func (m *mockDebouncer) executeLast() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.lastFunc != nil {
		m.lastFunc()
	}
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
	return &v1.Pod{
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

	clientset := fake.NewSimpleClientset(runningPod, pendingPod, succeededPod, failedPod)

	svc := createTestService("test-svc", namespace, []v1.ServicePort{
		{Port: 80, TargetPort: intstr.FromInt(8080), Protocol: v1.ProtocolTCP},
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
	clientset := fake.NewSimpleClientset()

	svc := createTestService("test-svc", namespace, []v1.ServicePort{
		{Port: 80, TargetPort: intstr.FromInt(8080), Protocol: v1.ProtocolTCP},
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

	clientset := fake.NewSimpleClientset(runningPod1, runningPod2)

	svc := createTestService("test-svc", namespace, []v1.ServicePort{
		{Port: 80, TargetPort: intstr.FromInt(8080), Protocol: v1.ProtocolTCP},
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
	namespace := "default"
	labels := map[string]string{"app": "test"}

	runningPod1 := createTestPod("running-pod-1", namespace, v1.PodRunning, labels)
	runningPod2 := createTestPod("running-pod-2", namespace, v1.PodRunning, labels)

	clientset := fake.NewSimpleClientset(runningPod1, runningPod2)

	svc := createTestService("test-svc", namespace, []v1.ServicePort{
		{Port: 80, TargetPort: intstr.FromInt(8080), Protocol: v1.ProtocolTCP},
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

	// Verify it's one of our pods
	foundPod := false
	svcFwd.NamespaceServiceLock.Lock()
	for key := range svcFwd.PortForwards {
		if key == "test-svc.running-pod-1" || key == "test-svc.running-pod-2" {
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
	namespace := "default"
	labels := map[string]string{"app": "test"}

	runningPod1 := createTestPod("running-pod-1", namespace, v1.PodRunning, labels)
	runningPod2 := createTestPod("running-pod-2", namespace, v1.PodRunning, labels)

	clientset := fake.NewSimpleClientset(runningPod1, runningPod2)

	svc := createTestService("test-svc", namespace, []v1.ServicePort{
		{Port: 80, TargetPort: intstr.FromInt(8080), Protocol: v1.ProtocolTCP},
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
	clientset := fake.NewSimpleClientset(runningPod)

	svc := createTestService("test-svc", namespace, []v1.ServicePort{
		{Port: 80, TargetPort: intstr.FromInt(8080), Protocol: v1.ProtocolTCP},
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
	namespace := "default"
	labels := map[string]string{"app": "test"}

	runningPod := createTestPod("running-pod", namespace, v1.PodRunning, labels)
	clientset := fake.NewSimpleClientset(runningPod)

	svc := createTestService("test-svc", namespace, []v1.ServicePort{
		{Port: 80, TargetPort: intstr.FromInt(8080), Protocol: v1.ProtocolTCP},
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
	}

	// Call with force=true - should bypass debouncer
	svcFwd.SyncPodForwards(true)

	// Debouncer should be replaced with no-op, so getCalls() should be 0
	// But sync should still have happened
	time.Sleep(100 * time.Millisecond) // Give time for goroutine to start

	if len(svcFwd.PortForwards) == 0 {
		t.Error("Expected pod to be forwarded with force=true")
	}
}

// TestSyncPodForwards_ForceSyncAfter5Minutes tests automatic force sync after 5 minutes
//
//goland:noinspection DuplicatedCode
func TestSyncPodForwards_ForceSyncAfter5Minutes(t *testing.T) {
	namespace := "default"
	labels := map[string]string{"app": "test"}

	runningPod := createTestPod("running-pod", namespace, v1.PodRunning, labels)
	clientset := fake.NewSimpleClientset(runningPod)

	svc := createTestService("test-svc", namespace, []v1.ServicePort{
		{Port: 80, TargetPort: intstr.FromInt(8080), Protocol: v1.ProtocolTCP},
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
	}

	// Call without force, but LastSyncedAt is >5 minutes ago
	// Should force sync anyway
	svcFwd.SyncPodForwards(false)

	time.Sleep(100 * time.Millisecond) // Give time for goroutine

	if len(svcFwd.PortForwards) == 0 {
		t.Error("Expected pod to be forwarded after 5 minute threshold")
	}
}

// TestSyncPodForwards_RemovesStoppedPods tests that stopped pods are removed
//
//goland:noinspection DuplicatedCode
func TestSyncPodForwards_RemovesStoppedPods(t *testing.T) {
	namespace := "default"
	labels := map[string]string{"app": "test"}

	// Start with one running pod
	runningPod := createTestPod("running-pod", namespace, v1.PodRunning, labels)
	clientset := fake.NewSimpleClientset(runningPod)

	svc := createTestService("test-svc", namespace, []v1.ServicePort{
		{Port: 80, TargetPort: intstr.FromInt(8080), Protocol: v1.ProtocolTCP},
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

	// Should only have the running pod now
	if len(svcFwd.PortForwards) != 1 {
		t.Errorf("Expected 1 pod after sync, got %d", len(svcFwd.PortForwards))
	}

	// Should NOT have the deleted pod
	if _, found := svcFwd.PortForwards["test-svc.deleted-pod"]; found {
		t.Error("Deleted pod should have been removed from PortForwards")
	}
}

// TestAddServicePod tests adding a pod to the PortForwards map
func TestAddServicePod(t *testing.T) {
	svcFwd := &ServiceFWD{
		PortForwards:         make(map[string]*fwdport.PortForwardOpts),
		NamespaceServiceLock: &sync.Mutex{},
	}

	pfo := &fwdport.PortForwardOpts{
		Service: "test-svc",
		PodName: "test-pod",
	}

	svcFwd.AddServicePod(pfo)

	// Should be in map with key "service.podname"
	if _, found := svcFwd.PortForwards["test-svc.test-pod"]; !found {
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
		Service: "test-svc",
		PodName: "test-pod",
	}

	pfo2 := &fwdport.PortForwardOpts{
		Service: "test-svc",
		PodName: "test-pod",
	}

	svcFwd.AddServicePod(pfo1)
	svcFwd.AddServicePod(pfo2)

	// Should only have one entry (second one doesn't add duplicate)
	if len(svcFwd.PortForwards) != 1 {
		t.Errorf("Expected 1 entry (no duplicates), got %d", len(svcFwd.PortForwards))
	}
}

// TestListServicePodNames tests listing pod names
func TestListServicePodNames(t *testing.T) {
	svcFwd := &ServiceFWD{
		PortForwards:         make(map[string]*fwdport.PortForwardOpts),
		NamespaceServiceLock: &sync.Mutex{},
	}

	pfo1 := &fwdport.PortForwardOpts{Service: "svc", PodName: "pod1"}
	pfo2 := &fwdport.PortForwardOpts{Service: "svc", PodName: "pod2"}

	svcFwd.AddServicePod(pfo1)
	svcFwd.AddServicePod(pfo2)

	names := svcFwd.ListServicePodNames()

	if len(names) != 2 {
		t.Errorf("Expected 2 pod names, got %d", len(names))
	}

	// Should contain both keys
	foundPod1 := false
	foundPod2 := false
	for _, name := range names {
		if name == "svc.pod1" {
			foundPod1 = true
		}
		if name == "svc.pod2" {
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

	// Remove it
	svcFwd.RemoveServicePod("test-svc.test-pod")

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
			podKey := "svc." + string(rune('a'+n))
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
