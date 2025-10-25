package fwdport

import (
	"sync"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestGlobalPodInformerManager_AddPod(t *testing.T) {
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
	_, exists := globalInformer.activePods[types.UID("test-uid-123")]
	globalInformer.mu.RUnlock()

	if !exists {
		t.Error("Pod should be in the active pods map")
	}
}

func TestGlobalPodInformerManager_GetPod(t *testing.T) {
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
	retrievedPfo, exists := globalInformer.getPod(types.UID("test-uid-123"))

	if !exists {
		t.Error("Pod should exist in the map")
	}

	if retrievedPfo != pfo {
		t.Error("Retrieved PortForwardOpts should match the original")
	}

	// Test getting a non-existent pod
	_, exists = globalInformer.getPod(types.UID("non-existent-uid"))
	if exists {
		t.Error("Non-existent pod should not exist in the map")
	}
}

func TestGlobalPodInformerManager_RemovePod(t *testing.T) {
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
	_, exists := globalInformer.activePods[types.UID("test-uid-123")]
	globalInformer.mu.RUnlock()

	if !exists {
		t.Error("Pod should be in the active pods map")
	}

	// Test removing the pod
	globalInformer.RemovePodByUID(types.UID("test-uid-123"))

	// Verify the pod was removed
	globalInformer.mu.RLock()
	_, exists = globalInformer.activePods[types.UID("test-uid-123")]
	globalInformer.mu.RUnlock()

	if exists {
		t.Error("Pod should not be in the active pods map after removal")
	}
}

func TestGlobalPodInformerManager_ConcurrentAccess(t *testing.T) {
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
