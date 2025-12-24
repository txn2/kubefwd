package fwdport

//goland:noinspection DuplicatedCode
import (
	"fmt"
	"sync"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	testing2 "k8s.io/client-go/testing"
)

// mockServiceFWD implements the ServiceFWD interface for testing
type mockServiceFWD struct {
	syncCalled bool
	syncMutex  sync.Mutex
}

func (m *mockServiceFWD) String() string {
	return "mock-service"
}

func (m *mockServiceFWD) SyncPodForwards(_ bool) {
	m.syncMutex.Lock()
	defer m.syncMutex.Unlock()
	m.syncCalled = true
}

func (m *mockServiceFWD) wasSyncCalled() bool {
	m.syncMutex.Lock()
	defer m.syncMutex.Unlock()
	return m.syncCalled
}

func setUpTestPod(namespace string, podName string) *v1.Pod {
	podUID := types.UID(fmt.Sprintf("test-pod-%s", podName))

	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
			UID:       podUID,
		},
		Status: v1.PodStatus{
			Phase: v1.PodRunning,
		},
	}
}

func setUpTestPFO(clientset *fake.Clientset, namespace string, podName string) *PortForwardOpts {
	mockSvc := &mockServiceFWD{}
	podUID := types.UID(fmt.Sprintf("test-%s", podName))

	return &PortForwardOpts{
		ClientSet:      clientset,
		Namespace:      namespace,
		PodName:        podName,
		PodUID:         podUID,
		ServiceFwd:     mockSvc,
		ManualStopChan: make(chan struct{}),
	}
}

//goland:noinspection DuplicatedCode
func TestGlobalPodInformerManager_AddPod(t *testing.T) {
	t.Cleanup(ResetGlobalPodInformer)
	// Create a test pod
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
			UID:       types.UID("test-uid-123"),
		},
		Status: v1.PodStatus{
			Phase: v1.PodRunning,
		},
	}

	// Create a mock PortForwardOpts
	pfo := &PortForwardOpts{
		PodName: "test-pod",
		PodUID:  types.UID("test-uid-123"),
	}

	// Create a global informer manager directly for testing
	globalInformer := &GlobalPodInformer{
		activePods: make(map[types.UID]*PortForwardOpts),
	}

	// Test adding a pod
	globalInformer.addPod(pod, pfo)

	// Verify the pod was added
	globalInformer.mu.RLock()
	_, exists := globalInformer.activePods[("test-uid-123")]
	globalInformer.mu.RUnlock()

	if !exists {
		t.Error("Pod should be in the active pods map")
	}
}

func TestGlobalPodInformerManager_GetPod(t *testing.T) {
	t.Cleanup(ResetGlobalPodInformer)
	// Create a test pod
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
			UID:       types.UID("test-uid-123"),
		},
		Status: v1.PodStatus{
			Phase: v1.PodRunning,
		},
	}

	// Create a mock PortForwardOpts
	pfo := &PortForwardOpts{
		PodName: "test-pod",
		PodUID:  types.UID("test-uid-123"),
	}

	// Create a global informer manager directly for testing
	globalInformer := &GlobalPodInformer{
		activePods: make(map[types.UID]*PortForwardOpts),
	}

	// Add the pod
	globalInformer.addPod(pod, pfo)

	// Test getting the pod
	retrievedPfo, exists := globalInformer.getPod("test-uid-123")

	if !exists {
		t.Error("Pod should exist in the map")
	}

	if retrievedPfo != pfo {
		t.Error("Retrieved PortForwardOpts should match the original")
	}

	// Test getting a non-existent pod
	_, exists = globalInformer.getPod("non-existent-uid")
	if exists {
		t.Error("Non-existent pod should not exist in the map")
	}
}

//goland:noinspection DuplicatedCode
func TestGlobalPodInformerManager_RemovePod(t *testing.T) {
	t.Cleanup(ResetGlobalPodInformer)
	// Create a test pod
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
			UID:       types.UID("test-uid-123"),
		},
		Status: v1.PodStatus{
			Phase: v1.PodRunning,
		},
	}

	// Create a mock PortForwardOpts
	pfo := &PortForwardOpts{
		PodName: "test-pod",
		PodUID:  types.UID("test-uid-123"),
	}

	// Create a global informer manager directly for testing
	globalInformer := &GlobalPodInformer{
		activePods: make(map[types.UID]*PortForwardOpts),
	}

	// Add the pod
	globalInformer.addPod(pod, pfo)

	// Verify the pod was added
	globalInformer.mu.RLock()
	_, exists := globalInformer.activePods[("test-uid-123")]
	globalInformer.mu.RUnlock()

	if !exists {
		t.Error("Pod should be in the active pods map")
	}

	// Test removing the pod
	globalInformer.RemovePodByUID("test-uid-123")

	// Verify the pod was removed
	globalInformer.mu.RLock()
	_, exists = globalInformer.activePods[("test-uid-123")]
	globalInformer.mu.RUnlock()

	if exists {
		t.Error("Pod should not be in the active pods map after removal")
	}
}

func TestGlobalPodInformerManager_ConcurrentAccess(t *testing.T) {
	t.Cleanup(ResetGlobalPodInformer)
	// Create a global informer manager directly for testing
	globalInformer := &GlobalPodInformer{
		activePods: make(map[types.UID]*PortForwardOpts),
	}

	// Test concurrent access
	var wg sync.WaitGroup
	numGoroutines := 10

	// Start multiple goroutines that add pods
	for i := range numGoroutines {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			pod := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod-" + string(rune(i)),
					Namespace: "test-namespace",
					UID:       types.UID("test-uid-" + string(rune(i))),
				},
				Status: v1.PodStatus{
					Phase: v1.PodRunning,
				},
			}

			pfo := &PortForwardOpts{
				PodName: "test-pod-" + string(rune(i)),
				PodUID:  types.UID("test-uid-" + string(rune(i))),
			}

			globalInformer.addPod(pod, pfo)
		}(i)
	}

	wg.Wait()

	// Verify all pods were added
	globalInformer.mu.RLock()
	podCount := len(globalInformer.activePods)
	globalInformer.mu.RUnlock()

	if podCount != numGoroutines {
		t.Errorf("Expected %d pods, got %d", numGoroutines, podCount)
	}
}

func TestGlobalPodInformerManager_Stop(t *testing.T) {
	t.Cleanup(ResetGlobalPodInformer)
	// Create a global informer manager directly for testing
	globalInformer := &GlobalPodInformer{
		activePods:  make(map[types.UID]*PortForwardOpts),
		stopChannel: make(chan struct{}),
	}

	// Test stopping the informer
	globalInformer.Stop()

	// Test stopping an already stopped informer (should not panic)
	globalInformer.Stop()
}

// TestGlobalPodInformer_DeleteEvent tests that pod deletion triggers reconnection
//
//goland:noinspection DuplicatedCode
func TestGlobalPodInformer_DeleteEvent(t *testing.T) {
	t.Cleanup(ResetGlobalPodInformer)
	// Create test pod
	pod := setUpTestPod("default", "test-pod")

	// Create fake clientset with the pod
	clientset := fake.NewSimpleClientset(pod)
	watcher := watch.NewFake()

	// Configure watch reactor
	clientset.PrependWatchReactor("pods", testing2.DefaultWatchReactor(watcher, nil))

	// Create GlobalPodInformer
	gpi := GetGlobalPodInformer(clientset, "default")
	defer gpi.Stop()

	// Create port forward options
	pfo := setUpTestPFO(clientset, "default", "test-pod")

	// Add pod to informer
	gpi.addPod(pod, pfo)

	// Give it time to sync
	time.Sleep(100 * time.Millisecond)

	// Simulate pod deletion
	watcher.Delete(pod)

	// Give time for deletion event to be processed
	time.Sleep(200 * time.Millisecond)

	// Verify SyncPodForwards was called
	if !pfo.ServiceFwd.(*mockServiceFWD).wasSyncCalled() {
		t.Error("Expected SyncPodForwards to be called after pod deletion")
	}
}

// TestGlobalPodInformer_DeletionTimestamp tests that pod marked for deletion triggers reconnection
//
//goland:noinspection DuplicatedCode
func TestGlobalPodInformer_DeletionTimestamp(t *testing.T) {
	t.Cleanup(ResetGlobalPodInformer)
	now := metav1.Now()
	pod := setUpTestPod("default", "test-pod")

	clientset := fake.NewSimpleClientset(pod)
	watcher := watch.NewFake()
	clientset.PrependWatchReactor("pods", testing2.DefaultWatchReactor(watcher, nil))

	gpi := GetGlobalPodInformer(clientset, "default")
	defer gpi.Stop()

	pfo := setUpTestPFO(clientset, "default", "test-pod")

	gpi.addPod(pod, pfo)
	time.Sleep(100 * time.Millisecond)

	// Simulate pod being marked for deletion (DeletionTimestamp set)
	modifiedPod := pod.DeepCopy()
	modifiedPod.DeletionTimestamp = &now
	watcher.Modify(modifiedPod)

	// Give time to be called
	time.Sleep(200 * time.Millisecond)

	// Verify SyncPodForwards was called
	if !pfo.ServiceFwd.(*mockServiceFWD).wasSyncCalled() {
		t.Error("Expected SyncPodForwards to be called after pod marked for deletion")
	}
}

// TestGlobalPodInformer_ModifiedWithoutDeletionTimestamp tests that normal modifications don't trigger sync
//
//goland:noinspection DuplicatedCode
func TestGlobalPodInformer_ModifiedWithoutDeletionTimestamp(t *testing.T) {
	t.Cleanup(ResetGlobalPodInformer)
	pod := setUpTestPod("default", "test-pod")

	clientset := fake.NewSimpleClientset(pod)
	watcher := watch.NewFake()
	clientset.PrependWatchReactor("pods", testing2.DefaultWatchReactor(watcher, nil))

	gpi := GetGlobalPodInformer(clientset, "default")
	defer gpi.Stop()

	pfo := setUpTestPFO(clientset, "default", "test-pod")

	gpi.addPod(pod, pfo)
	time.Sleep(100 * time.Millisecond)

	// Simulate pod modification without DeletionTimestamp
	modifiedPod := pod.DeepCopy()
	modifiedPod.Status.Phase = v1.PodFailed
	watcher.Modify(modifiedPod)

	time.Sleep(200 * time.Millisecond)

	// SyncPodForwards should NOT have been called
	if pfo.ServiceFwd.(*mockServiceFWD).wasSyncCalled() {
		t.Error("SyncPodForwards should not be called for modifications without DeletionTimestamp")
	}
}

// TestGlobalPodInformer_RapidEvents tests handling of rapid deletion events
//
//goland:noinspection DuplicatedCode
func TestGlobalPodInformer_RapidEvents(t *testing.T) {
	t.Cleanup(ResetGlobalPodInformer)
	pod := setUpTestPod("default", "test-pod")

	clientset := fake.NewSimpleClientset(pod)
	watcher := watch.NewFake()
	clientset.PrependWatchReactor("pods", testing2.DefaultWatchReactor(watcher, nil))

	gpi := GetGlobalPodInformer(clientset, "default")
	defer gpi.Stop()

	pfo := setUpTestPFO(clientset, "default", "test-pod")

	gpi.addPod(pod, pfo)
	time.Sleep(100 * time.Millisecond)

	// Send rapid events
	now := metav1.Now()
	modifiedPod := pod.DeepCopy()
	modifiedPod.DeletionTimestamp = &now

	// Rapid modifications
	for i := 0; i < 3; i++ {
		watcher.Modify(modifiedPod)
		time.Sleep(10 * time.Millisecond)
	}

	// Then deletion
	watcher.Delete(pod)

	time.Sleep(300 * time.Millisecond)

	// SyncPodForwards should have been called at least once
	if !pfo.ServiceFwd.(*mockServiceFWD).wasSyncCalled() {
		t.Error("Expected SyncPodForwards to be called after rapid deletion events")
	}
}

// TestGlobalPodInformer_MultipleNamespaces tests handling of events while port forwarding multiple namespaces
func TestGlobalPodInformer_MultipleNamespaces(t *testing.T) {
	t.Cleanup(ResetGlobalPodInformer)
	amount := 5

	mockSvcs := make([]*mockServiceFWD, 0, amount)
	pods := make([]runtime.Object, 0, amount)
	var stopFunc func()
	defer func() {
		stopFunc()
	}()

	for i := range amount {
		namespace := fmt.Sprintf("test-namespace-%d", i)
		pod := setUpTestPod(namespace, fmt.Sprintf("pod-%d", i))
		pods = append(pods, pod)
	}

	clientset := fake.NewSimpleClientset(pods...)
	watcher := watch.NewFake()
	clientset.PrependWatchReactor("pods", testing2.DefaultWatchReactor(watcher, nil))

	for i := range amount {
		namespace := fmt.Sprintf("test-namespace-%d", i)
		gpi := GetGlobalPodInformer(clientset, namespace)
		if stopFunc == nil {
			stopFunc = gpi.Stop
		}
		podName := fmt.Sprintf("pod-%d", i)
		pfo := setUpTestPFO(clientset, namespace, podName)
		mockSvcs = append(mockSvcs, pfo.ServiceFwd.(*mockServiceFWD))
		gpi.addPod(pods[i].(*v1.Pod), pfo)
	}

	time.Sleep(100 * time.Millisecond)

	for i := range amount {
		watcher.Delete(pods[i])
	}

	// Give some time to sync
	time.Sleep(300 * time.Millisecond)

	for i := range amount {
		// SyncPodForwards should have been called at least once
		if !mockSvcs[i].wasSyncCalled() {
			t.Errorf("Expected SyncPodForwards to be called for pod %d", i)
		}
	}
}
