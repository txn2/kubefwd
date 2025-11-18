package fwdport

import (
	"fmt"
	"sync"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	k8stest "k8s.io/client-go/testing"
)

// mockServiceFWD implements the ServiceFWD interface for testing
type mockServiceFWD struct {
	syncCalled bool
	syncMutex  sync.Mutex
}

func (m *mockServiceFWD) String() string {
	return "mock-service"
}

func (m *mockServiceFWD) SyncPodForwards(force bool) {
	m.syncMutex.Lock()
	defer m.syncMutex.Unlock()
	m.syncCalled = true
}

func (m *mockServiceFWD) wasSyncCalled() bool {
	m.syncMutex.Lock()
	defer m.syncMutex.Unlock()
	return m.syncCalled
}

// TestListenUntilPodDeleted_DeleteEvent tests that pod deletion triggers reconnection
func TestListenUntilPodDeleted_DeleteEvent(t *testing.T) {
	// Create test pod
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Status: v1.PodStatus{
			Phase: v1.PodRunning,
		},
	}

	// Create fake clientset with the pod
	clientset := fake.NewSimpleClientset(pod)
	watcher := watch.NewFake()

	// Configure watch reactor
	clientset.PrependWatchReactor("pods", k8stest.DefaultWatchReactor(watcher, nil))

	// Create mock service
	mockSvc := &mockServiceFWD{}

	// Create port forward options
	stopChan := make(chan struct{})
	manualStopChan := make(chan struct{})
	pfo := &PortForwardOpts{
		ClientSet:      clientset,
		Namespace:      "default",
		PodName:        "test-pod",
		ServiceFwd:     mockSvc,
		ManualStopChan: manualStopChan,
	}

	// Start listening in goroutine
	done := make(chan struct{})
	go func() {
		pfo.ListenUntilPodDeleted(stopChan, pod)
		close(done)
	}()

	// Give it time to start watching
	time.Sleep(100 * time.Millisecond)

	// Simulate pod deletion
	watcher.Delete(pod)

	// Give time for deletion event to be processed
	time.Sleep(200 * time.Millisecond)

	// Verify SyncPodForwards was called
	if !mockSvc.wasSyncCalled() {
		t.Error("Expected SyncPodForwards to be called after pod deletion")
	}

	// Note: The watch loop continues running after deletion (by design for auto-reconnect)
	// We need to explicitly stop it
	close(stopChan)

	// Wait for watch to stop
	select {
	case <-done:
		// Success - watch stopped after stopChan closed
	case <-time.After(2 * time.Second):
		t.Fatal("ListenUntilPodDeleted did not exit after stop signal")
	}
}

// TestListenUntilPodDeleted_DeletionTimestamp tests that pod marked for deletion triggers reconnection
func TestListenUntilPodDeleted_DeletionTimestamp(t *testing.T) {
	now := metav1.Now()
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Status: v1.PodStatus{
			Phase: v1.PodRunning,
		},
	}

	clientset := fake.NewSimpleClientset(pod)
	watcher := watch.NewFake()
	clientset.PrependWatchReactor("pods", k8stest.DefaultWatchReactor(watcher, nil))

	mockSvc := &mockServiceFWD{}
	stopChan := make(chan struct{})
	manualStopChan := make(chan struct{})

	pfo := &PortForwardOpts{
		ClientSet:      clientset,
		Namespace:      "default",
		PodName:        "test-pod",
		ServiceFwd:     mockSvc,
		ManualStopChan: manualStopChan,
	}

	// Start listening in goroutine
	go func() {
		pfo.ListenUntilPodDeleted(stopChan, pod)
	}()

	// Give it time to start watching
	time.Sleep(100 * time.Millisecond)

	// Simulate pod being marked for deletion (DeletionTimestamp set)
	modifiedPod := pod.DeepCopy()
	modifiedPod.DeletionTimestamp = &now
	watcher.Modify(modifiedPod)

	// Give Stop() and SyncPodForwards() time to be called
	time.Sleep(200 * time.Millisecond)

	// Verify SyncPodForwards was called
	if !mockSvc.wasSyncCalled() {
		t.Error("Expected SyncPodForwards to be called after pod marked for deletion")
	}

	// Cleanup
	close(stopChan)
}

// TestListenUntilPodDeleted_StopChannel tests that stopChannel properly stops the watch
func TestListenUntilPodDeleted_StopChannel(t *testing.T) {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Status: v1.PodStatus{
			Phase: v1.PodRunning,
		},
	}

	clientset := fake.NewSimpleClientset(pod)
	watcher := watch.NewFake()
	clientset.PrependWatchReactor("pods", k8stest.DefaultWatchReactor(watcher, nil))

	mockSvc := &mockServiceFWD{}
	stopChan := make(chan struct{})
	manualStopChan := make(chan struct{})

	pfo := &PortForwardOpts{
		ClientSet:      clientset,
		Namespace:      "default",
		PodName:        "test-pod",
		ServiceFwd:     mockSvc,
		ManualStopChan: manualStopChan,
	}

	// Start listening in goroutine
	done := make(chan struct{})
	go func() {
		pfo.ListenUntilPodDeleted(stopChan, pod)
		close(done)
	}()

	// Give it time to start watching
	time.Sleep(100 * time.Millisecond)

	// Send stop signal
	close(stopChan)

	// Wait for watch to stop with timeout
	select {
	case <-done:
		// Success - watch stopped
	case <-time.After(2 * time.Second):
		t.Fatal("ListenUntilPodDeleted did not exit after stop signal")
	}

	// SyncPodForwards should NOT have been called (no deletion event)
	if mockSvc.wasSyncCalled() {
		t.Error("SyncPodForwards should not be called when stopped via stopChannel")
	}
}

// TestListenUntilPodDeleted_WatchChannelClosed tests graceful handling of closed watch channel
func TestListenUntilPodDeleted_WatchChannelClosed(t *testing.T) {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Status: v1.PodStatus{
			Phase: v1.PodRunning,
		},
	}

	clientset := fake.NewSimpleClientset(pod)
	watcher := watch.NewFake()
	clientset.PrependWatchReactor("pods", k8stest.DefaultWatchReactor(watcher, nil))

	mockSvc := &mockServiceFWD{}
	stopChan := make(chan struct{})
	manualStopChan := make(chan struct{})

	pfo := &PortForwardOpts{
		ClientSet:      clientset,
		Namespace:      "default",
		PodName:        "test-pod",
		ServiceFwd:     mockSvc,
		ManualStopChan: manualStopChan,
	}

	// Start listening in goroutine
	done := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("ListenUntilPodDeleted panicked: %v", r)
			}
			close(done)
		}()
		pfo.ListenUntilPodDeleted(stopChan, pod)
	}()

	// Give it time to start watching
	time.Sleep(100 * time.Millisecond)

	// Close the watch channel (simulates connection loss)
	watcher.Stop()

	// Wait for watch to exit gracefully with timeout
	select {
	case <-done:
		// Success - watch exited without panic
	case <-time.After(2 * time.Second):
		t.Fatal("ListenUntilPodDeleted did not exit after watch channel closed")
	}

	// Cleanup
	close(stopChan)
}

// TestListenUntilPodDeleted_ModifiedWithoutDeletionTimestamp tests that normal modifications don't trigger sync
func TestListenUntilPodDeleted_ModifiedWithoutDeletionTimestamp(t *testing.T) {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Status: v1.PodStatus{
			Phase: v1.PodRunning,
		},
	}

	clientset := fake.NewSimpleClientset(pod)
	watcher := watch.NewFake()
	clientset.PrependWatchReactor("pods", k8stest.DefaultWatchReactor(watcher, nil))

	mockSvc := &mockServiceFWD{}
	stopChan := make(chan struct{})
	manualStopChan := make(chan struct{})

	pfo := &PortForwardOpts{
		ClientSet:      clientset,
		Namespace:      "default",
		PodName:        "test-pod",
		ServiceFwd:     mockSvc,
		ManualStopChan: manualStopChan,
	}

	// Start listening in goroutine
	go func() {
		pfo.ListenUntilPodDeleted(stopChan, pod)
	}()

	// Give it time to start watching
	time.Sleep(100 * time.Millisecond)

	// Simulate pod modification without DeletionTimestamp (e.g., status change)
	modifiedPod := pod.DeepCopy()
	modifiedPod.Status.Phase = v1.PodFailed // Changed but not marked for deletion
	watcher.Modify(modifiedPod)

	// Give it time to process
	time.Sleep(200 * time.Millisecond)

	// SyncPodForwards should NOT have been called
	if mockSvc.wasSyncCalled() {
		t.Error("SyncPodForwards should not be called for modifications without DeletionTimestamp")
	}

	// Cleanup
	close(stopChan)
}

// TestListenUntilPodDeleted_WatchError tests handling of watch errors
func TestListenUntilPodDeleted_WatchError(t *testing.T) {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Status: v1.PodStatus{
			Phase: v1.PodRunning,
		},
	}

	clientset := fake.NewSimpleClientset(pod)
	watcher := watch.NewFake()
	clientset.PrependWatchReactor("pods", k8stest.DefaultWatchReactor(watcher, nil))

	mockSvc := &mockServiceFWD{}
	stopChan := make(chan struct{})
	manualStopChan := make(chan struct{})

	pfo := &PortForwardOpts{
		ClientSet:      clientset,
		Namespace:      "default",
		PodName:        "test-pod",
		ServiceFwd:     mockSvc,
		ManualStopChan: manualStopChan,
	}

	// Start listening in goroutine
	done := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("ListenUntilPodDeleted panicked on error: %v", r)
			}
			close(done)
		}()
		pfo.ListenUntilPodDeleted(stopChan, pod)
	}()

	// Give it time to start watching
	time.Sleep(100 * time.Millisecond)

	// Send an error event
	watcher.Error(&metav1.Status{
		Status:  metav1.StatusFailure,
		Message: "test error",
		Reason:  metav1.StatusReasonInternalError,
		Code:    500,
	})

	// Give it time to process
	time.Sleep(200 * time.Millisecond)

	// The watch should continue or exit gracefully (implementation dependent)
	// Most importantly, it should not panic

	// Cleanup
	close(stopChan)

	// Wait a bit for cleanup
	select {
	case <-done:
		// Exited - OK
	case <-time.After(1 * time.Second):
		// Still running - also OK (watch continues after error)
	}
}

// TestListenUntilPodDeleted_RapidEvents tests handling of rapid deletion events
func TestListenUntilPodDeleted_RapidEvents(t *testing.T) {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Status: v1.PodStatus{
			Phase: v1.PodRunning,
		},
	}

	clientset := fake.NewSimpleClientset(pod)
	watcher := watch.NewFake()
	clientset.PrependWatchReactor("pods", k8stest.DefaultWatchReactor(watcher, nil))

	mockSvc := &mockServiceFWD{}
	stopChan := make(chan struct{})
	manualStopChan := make(chan struct{})

	pfo := &PortForwardOpts{
		ClientSet:      clientset,
		Namespace:      "default",
		PodName:        "test-pod",
		ServiceFwd:     mockSvc,
		ManualStopChan: manualStopChan,
	}

	// Start listening in goroutine
	done := make(chan struct{})
	go func() {
		pfo.ListenUntilPodDeleted(stopChan, pod)
		close(done)
	}()

	// Give it time to start watching
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

	// Give time for all events to be processed
	time.Sleep(300 * time.Millisecond)

	// SyncPodForwards should have been called at least once
	if !mockSvc.wasSyncCalled() {
		t.Error("Expected SyncPodForwards to be called after rapid deletion events")
	}

	// Stop the watch explicitly
	close(stopChan)

	// Wait for watch to stop
	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("ListenUntilPodDeleted did not exit after stop signal")
	}
}

// TestListenUntilPodDeleted_WatchCreationError tests handling when watch cannot be created
func TestListenUntilPodDeleted_WatchCreationError(t *testing.T) {
	// Create clientset that will fail to create watch
	clientset := fake.NewSimpleClientset()

	// Inject a reactor that returns an error for watch operations
	clientset.PrependWatchReactor("pods", func(action k8stest.Action) (handled bool, ret watch.Interface, err error) {
		return true, nil, fmt.Errorf("simulated watch creation error")
	})

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
	}

	mockSvc := &mockServiceFWD{}
	stopChan := make(chan struct{})
	manualStopChan := make(chan struct{})

	pfo := &PortForwardOpts{
		ClientSet:      clientset,
		Namespace:      "default",
		PodName:        "test-pod",
		ServiceFwd:     mockSvc,
		ManualStopChan: manualStopChan,
	}

	// Start listening - should return immediately due to watch creation error
	done := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("ListenUntilPodDeleted panicked on watch creation error: %v", r)
			}
			close(done)
		}()
		pfo.ListenUntilPodDeleted(stopChan, pod)
	}()

	// Should return quickly
	select {
	case <-done:
		// Success - function returned gracefully
	case <-time.After(1 * time.Second):
		t.Fatal("ListenUntilPodDeleted did not return after watch creation error")
	}

	// SyncPodForwards should NOT have been called
	if mockSvc.wasSyncCalled() {
		t.Error("SyncPodForwards should not be called when watch creation fails")
	}

	// Cleanup
	close(stopChan)
}
